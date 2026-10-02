package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// childOf starts the fake agent, makes it start a child with its own console
// and returns both pids. The child is killed when the test ends whatever
// happens.
func childOf(t *testing.T) (PTY, *output, int) {
	t.Helper()
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	p.Write([]byte("spawn\r"))
	re := regexp.MustCompile(`child:(\d+);`)
	deadline := time.Now().Add(10 * time.Second)
	var m []string
	for m = re.FindStringSubmatch(out.String()); m == nil; m = re.FindStringSubmatch(out.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("no child pid; output:\n%q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	pid, _ := strconv.Atoi(m[1])
	t.Cleanup(func() {
		if alive(pid) {
			if pr, err := os.FindProcess(pid); err == nil {
				pr.Kill()
			}
		}
	})
	if !alive(pid) {
		t.Fatalf("child %d not running", pid)
	}
	return p, out, pid
}

func waitDead(t *testing.T, pid int, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("%s %d still running 10 s later", what, pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// V25: the agent ended but left a child with its own console. Kill used to
// find no tree under the gone pid and the child survived.
func TestKillEndsTheChildrenOfAnEndedAgent(t *testing.T) {
	p, _, child := childOf(t)
	p.Write([]byte("exit\r"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if code, err := p.Wait(ctx); err != nil || code != 7 {
		t.Fatalf("Wait = %d, %v; want 7", code, err)
	}
	if !alive(child) {
		t.Skip("the child ended with the agent: nothing to prove")
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	waitDead(t, child, "child")
}

// jobPids lists the processes in the job (JobObjectBasicProcessIdList).
func jobPids(t *testing.T, job windows.Handle) []int {
	t.Helper()
	const max = 64
	buf := make([]uint64, 2+max) // header: two DWORDs, then the pid list
	size := uint32(len(buf) * 8)
	if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&buf[0])), size, nil); err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	n := *(*uint32)(unsafe.Add(unsafe.Pointer(&buf[0]), 4)) // NumberOfProcessIdsInList
	if n > max {
		n = max
	}
	pids := make([]int, n)
	for i := range pids {
		pids[i] = int(*(*uintptr)(unsafe.Add(unsafe.Pointer(&buf[0]), 8+i*8)))
	}
	return pids
}

func contains(pids []int, pid int) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}

// V25: the program and what it started (also with its own console) are in
// the job, and closing the job ends them all, with no help from taskkill.
func TestJobHoldsTheTreeAndClosingItKillsIt(t *testing.T) {
	p, _, child := childOf(t)
	w := p.(*winPTY)
	if w.job == 0 {
		t.Fatal("no job object")
	}
	t.Cleanup(func() { _ = windows.TerminateJobObject(w.job, 1) }) // never leave processes behind
	pids := jobPids(t, w.job)
	if !contains(pids, w.Pid()) || !contains(pids, child) {
		t.Fatalf("job holds %v; want the agent %d and its child %d", pids, w.Pid(), child)
	}
	// Closing the last handle is what happens if this process dies.
	if err := windows.CloseHandle(w.job); err != nil {
		t.Fatal(err)
	}
	waitDead(t, child, "child")
	waitDead(t, w.Pid(), "agent")
	w.job = 0 // already closed: Kill must not close it again
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
}

// V25: if the job cannot be made or the program cannot be put in it, the
// program still runs (orphans are no hole: the shim fails closed without the
// host), with a warning, and Kill still ends its tree with taskkill.
func TestWithoutJobItWarnsAndGoesOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func()
	}{
		{"create", func() {
			createJobObject = func(*windows.SecurityAttributes, *uint16) (windows.Handle, error) {
				return 0, errors.New("injected")
			}
		}},
		{"assign", func() {
			assignProcessToJob = func(windows.Handle, windows.Handle) error { return errors.New("injected") }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Replaced before the PTY starts its goroutines, restored at the end.
			oc, oa, ow := createJobObject, assignProcessToJob, warnf
			var warned []string
			warnf = func(f string, a ...any) { warned = append(warned, fmt.Sprintf(f, a...)) }
			t.Cleanup(func() { createJobObject, assignProcessToJob, warnf = oc, oa, ow })
			tc.set()
			p, out := startAgent(t)
			out.waitFor(t, "ready")
			if p.(*winPTY).job != 0 {
				t.Fatal("a job despite the failure")
			}
			if len(warned) != 1 || !strings.Contains(warned[0], "injected") {
				t.Fatalf("warnings = %q; want one with the cause", warned)
			}
			// It works: types, and Kill ends it.
			p.Write([]byte("hi\r"))
			out.waitFor(t, "got:HI")
			if err := p.Kill(); err != nil {
				t.Fatal(err)
			}
			waitDead(t, p.Pid(), "agent")
		})
	}
}
