package cli

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// V22: outbox/ has a quota, like inbox/. A send that would take it over is
// refused before anything is copied, and deleting files from outbox/ (what
// the phone already has) makes room again.
func TestSendOutboxQuota(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 6))
	if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("first send: %+v", e)
	}
	_, e := l.sendFile(ws, sendReq(p, "")) // 6 + 6 > 10
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	got := outboxNames(t, ws)
	if len(got) != 1 {
		t.Fatalf("outbox %v", got)
	}
	if err := os.Remove(filepath.Join(ws, "outbox", got[0])); err != nil {
		t.Fatal(err)
	}
	if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("send after making room: %+v", e)
	}
}

// What the agent writes in outbox/ itself counts too (a report written there
// and then sent, as the skills do), and nothing is copied over the quota.
func TestSendOutboxQuotaCountsTheAgentsFiles(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "outbox/informe.md", make([]byte, 6))
	_, e := l.sendFile(ws, sendReq(p, "")) // 6 (the report) + 6 (its copy) > 10
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	if got := outboxNames(t, ws); len(got) != 1 || got[0] != "informe.md" {
		t.Fatalf("outbox %v", got)
	}
}

// A folder that cannot be measured is not an empty one: the send fails and
// nothing is copied (the quota fails closed).
func TestSendOutboxQuotaFailsClosedIfItCannotMeasure(t *testing.T) {
	setVar(t, &readOutbox, func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission })
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 6))
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "SEND_FAILED", "could not measure the outbox folder")
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("copied %v", got)
	}
}

// A file that grew after it was measured is counted by what was copied: the
// copy over the quota is removed.
func TestSendOutboxQuotaCountsWhatWasCopied(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	setVar(t, &statSize, func(os.FileInfo) int64 { return 0 }) // measured empty, it is not
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 11))
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("left %v", got)
	}
}

// Sends take turns from the quota check to the end of the copy: several at
// once cannot all pass a check that only one of them fits in. The pause after
// the check, once every send has begun, would let them all pass it unless
// they take turns: then only the second check would stop them, and maybe
// none would be sent.
func TestSendOutboxQuotaHoldsUnderConcurrentSends(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 6))
	exits := make([]ipc.Exit, 8)
	var started, wg sync.WaitGroup
	var passed atomic.Int32
	started.Add(len(exits))
	setVar(t, &quotaPassed, func() {
		passed.Add(1)
		started.Wait() // every send has begun
		time.Sleep(20 * time.Millisecond)
	})
	for i := range exits {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			_, exits[i] = l.sendFile(ws, sendReq(p, ""))
		}()
	}
	wg.Wait()
	sent := 0
	for _, e := range exits {
		if e.Code == 0 {
			sent++
			continue
		}
		wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	}
	if got := outboxNames(t, ws); sent != 1 || len(got) != 1 || passed.Load() != 1 {
		t.Fatalf("%d sent, %d passed the first check, outbox %v", sent, passed.Load(), got)
	}
}
