package pty

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The ConPTY is built here rather than with github.com/UserExistsError/conpty
// (validated in spike S3), which keeps the pseudo console's ends of the pipes
// open in this process: its output pipe never broke, so Read did not end when
// the program did (fix round 2, C1). As Microsoft's samples do, those ends are
// closed once the pseudo console has them; closing the console then ends
// conhost and Read gets the rest of the output and io.EOF.

var procUpdateProcThreadAttribute = windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")

// Seams for the tests of the "no job" decision (replaced before a PTY starts,
// restored when the test ends) and the channel for that warning.
var (
	createJobObject    = windows.CreateJobObject
	assignProcessToJob = windows.AssignProcessToJobObject
	warnf              = log.Printf
)

// newJob makes the job object that holds the program and everything it
// starts: closing its last handle (this process dying included) kills them
// all. It is not inheritable (nil SecurityAttributes) and breakaway is not
// allowed, so no child can leave it. It returns 0 when it cannot.
func newJob() windows.Handle {
	job, err := createJobObject(nil, nil)
	if err != nil {
		warnf("pty: no job object, the program's children may outlive it: %v", err)
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		warnf("pty: no job object, the program's children may outlive it: %v", err)
		_ = windows.CloseHandle(job)
		return 0
	}
	return job
}

type winPTY struct {
	jmu    sync.Mutex     // job: Kill closes it while Contains may read it
	job    windows.Handle // 0 if there is none
	hpc    windows.Handle // the pseudo console
	proc   windows.Handle
	thread windows.Handle
	pid    int
	in     *os.File // what is typed
	inH    windows.Handle
	out    *os.File // the console's output

	last atomic.Int64 // when output was last read (UnixNano)

	done chan struct{} // closed when the process has ended; code and err set
	code int
	err  error

	cmu       sync.Mutex // the console handle: Resize and closing
	conClosed bool

	killOnce sync.Once
}

// Start starts the program in a ConPTY.
func Start(o Options) (PTY, error) {
	cmdLine := quote(o.Path)
	for _, a := range o.Args {
		cmdLine += " " + quote(a)
	}
	var ptyIn, cmdIn, cmdOut, ptyOut windows.Handle
	if err := windows.CreatePipe(&ptyIn, &cmdIn, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&cmdOut, &ptyOut, nil, 0); err != nil {
		closeAll(ptyIn, cmdIn)
		return nil, err
	}
	var hpc windows.Handle
	err := windows.CreatePseudoConsole(windows.Coord{X: int16(o.Cols), Y: int16(o.Rows)}, ptyIn, ptyOut, 0, &hpc)
	// The pseudo console has its own copies of its ends of the pipes.
	closeAll(ptyIn, ptyOut)
	if err != nil {
		closeAll(cmdIn, cmdOut)
		return nil, err
	}
	// The program starts suspended, goes into the job and only then runs,
	// so nothing it starts can be outside it.
	job := newJob()
	pi, err := spawn(hpc, cmdLine, o.Dir, o.Env, job != 0)
	if err != nil {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		windows.ClosePseudoConsole(hpc)
		closeAll(cmdIn, cmdOut)
		return nil, err
	}
	if job != 0 {
		if err := assignProcessToJob(job, pi.Process); err != nil {
			warnf("pty: could not put the program in a job object, its children may outlive it: %v", err)
			_ = windows.CloseHandle(job)
			job = 0
		}
		if _, err := windows.ResumeThread(pi.Thread); err != nil {
			if job != 0 {
				_ = windows.TerminateJobObject(job, 1)
				_ = windows.CloseHandle(job)
			}
			_ = windows.TerminateProcess(pi.Process, 1)
			closeAll(pi.Thread, pi.Process)
			windows.ClosePseudoConsole(hpc)
			closeAll(cmdIn, cmdOut)
			return nil, err
		}
	}
	p := &winPTY{
		job:    job,
		hpc:    hpc,
		proc:   pi.Process,
		thread: pi.Thread,
		pid:    int(pi.ProcessId),
		in:     os.NewFile(uintptr(cmdIn), "conpty-in"),
		inH:    cmdIn,
		out:    os.NewFile(uintptr(cmdOut), "conpty-out"),
		done:   make(chan struct{}),
	}
	go p.watch()
	return p, nil
}

// spawn starts the program attached to the pseudo console.
func spawn(hpc windows.Handle, cmdLine, dir string, env []string, suspended bool) (*windows.ProcessInformation, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself, not a pointer to
	// it: called directly so that no handle is held as a Go pointer.
	if r, _, e := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(attrs.List())), 0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(hpc), unsafe.Sizeof(hpc), 0, 0); r == 0 {
		return nil, e
	}
	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	// No standard handles of this process: the console's only.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.ProcThreadAttributeList = attrs.List()

	line, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, err
	}
	var wd *uint16
	if dir != "" {
		if wd, err = windows.UTF16PtrFromString(dir); err != nil {
			return nil, err
		}
	}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT)
	if suspended {
		flags |= windows.CREATE_SUSPENDED
	}
	var block *uint16
	if env != nil {
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
		if block, err = envBlock(env); err != nil {
			return nil, err
		}
	}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(nil, line, nil, nil, false, flags, block, wd, &si.StartupInfo, &pi); err != nil {
		return nil, err
	}
	return &pi, nil
}

// envBlock is env as CreateProcess takes it: each entry ended by a NUL, and a
// NUL after the last one.
func envBlock(env []string) (*uint16, error) {
	var b []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			return nil, syscall.EINVAL
		}
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	b = append(b, 0)
	return &b[0], nil
}

func closeAll(hs ...windows.Handle) {
	for _, h := range hs {
		_ = windows.CloseHandle(h)
	}
}

// quote quotes one argument for a Windows command line (CommandLineToArgvW rules).
func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return syscall.EscapeArg(s)
}

// watch waits for the process, keeps its exit code and, once its last
// output had time to come out, closes the console: conhost ends and Read
// returns io.EOF after the rest of the output.
func (p *winPTY) watch() {
	_, err := windows.WaitForSingleObject(p.proc, windows.INFINITE)
	var code uint32
	if err == nil {
		err = windows.GetExitCodeProcess(p.proc, &code)
	}
	p.code, p.err = int(code), err
	close(p.done)
	drain(&p.last)
	p.closeConsole()
}

// closeConsole closes the pseudo console, which ends the programs still
// attached to it and then conhost. It may wait until conhost has written
// its last frame, so Read must keep reading meanwhile.
func (p *winPTY) closeConsole() {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	if p.conClosed {
		return
	}
	p.conClosed = true
	windows.ClosePseudoConsole(p.hpc)
}

func (p *winPTY) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	if n > 0 {
		p.last.Store(time.Now().UnixNano())
	}
	if errors.Is(err, os.ErrClosed) {
		err = io.EOF // Kill closed it
	}
	return n, err
}

func (p *winPTY) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *winPTY) Pid() int                    { return p.pid }

func (p *winPTY) Resize(cols, rows int) error {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	if p.conClosed {
		return os.ErrClosed
	}
	return windows.ResizePseudoConsole(p.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

// Wait returns the exit code kept when the process ended: the same on every
// call, also after Kill.
func (p *winPTY) Wait(ctx context.Context) (int, error) {
	select {
	case <-p.done:
		return p.code, p.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Hangup does nothing: a pseudo console has no hangup that the program
// could catch, and closing it ends the program at once. Stop waits a little
// and then kills the tree.
func (p *winPTY) Hangup() error { return nil }

// Kill ends the whole process tree (the agent starts node, shells…), closes
// the console and releases the handles. Read returns io.EOF.
func (p *winPTY) Kill() error {
	p.killOnce.Do(func() {
		if p.job != 0 {
			// The whole tree, also what is left after the program ended
			// (children with their own console: taskkill finds no tree under
			// a pid that is gone).
			_ = windows.TerminateJobObject(p.job, 1)
		}
		select {
		case <-p.done:
			// Ended: closing the console ends what is still attached to it.
		default:
			if p.job == 0 {
				kill := exec.Command(taskkill(), "/T", "/F", "/PID", strconv.Itoa(p.pid))
				kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
				_ = kill.Run() // it may have ended meanwhile
			}
			// The process itself, if the rest could not.
			_ = windows.TerminateProcess(p.proc, 1)
		}
		<-p.done // watch is done with the process handle
		// A Write stuck in the input (the session's writer, V08) must not
		// hold Kill: cancel it, then close without waiting for it long.
		_ = windows.CancelIoEx(p.inH, nil)
		closeNoWait(p.in, inCloseWait)
		p.closeConsole()
		_ = p.out.Close() // a blocked Read gets io.EOF
		closeAll(p.thread, p.proc)
		p.jmu.Lock()
		if p.job != 0 {
			_ = windows.CloseHandle(p.job)
			p.job = 0
		}
		p.jmu.Unlock()
	})
	return nil
}

// inCloseWait is how long Kill waits for the input to close.
var inCloseWait = 2 * time.Second

// closeNoWait closes f, waiting d at most, and reports whether it closed in
// time. Close waits for a Write in progress, and a Write into a console that
// does not take its input may never end. os.NewFile sees that the input is a
// pipe (Go 1.27), and Close then cancels such a Write (CancelIoEx), as Kill
// does just before; should neither end it, Kill goes on and the handle
// closes when the Write ends (a goroutine and a handle left until then).
func closeNoWait(f *os.File, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		_ = f.Close()
		close(done)
	}()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	}
}

// taskkill is System32's, not the first one on PATH.
func taskkill() string {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		dir = filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	return filepath.Join(dir, "taskkill.exe")
}
