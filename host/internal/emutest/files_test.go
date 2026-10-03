package emutest

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// Files over the channel (validation 3 of M3b, a and b): the upload of a file
// chosen in the phone's picker, with the SHA-256 of the bytes the host got.
//
// Mutations. Each test must go red when the host's side misbehaves; the
// variable ADBUDDY_EMUTEST_MUTATE makes the harness misbehave on purpose, so
// the proof can be repeated (one value per run):
//
//	corrupt-byte  the host damages one byte of one frame it received   (TestUploadSendsTheRealBytes)
//	skip-frame    the host drops one frame it received                  (TestUploadSendsTheRealBytes)
//	ready-zero    the host answers attach.ready{offset:0} after a cut   (TestUploadResumesAfterACut)
//
//	ADBUDDY_EMUTEST_MUTATE=corrupt-byte go test -count=1 -run TestUploadSendsTheRealBytes ./internal/emutest
//
// A test run with a mutation it does not take skips; without the variable
// the tests are the real ones.
const mutateEnv = "ADBUDDY_EMUTEST_MUTATE"

// mutation returns the value of ADBUDDY_EMUTEST_MUTATE, skipping the test
// when it is not one of mine (taken) and failing on an unknown value.
func mutation(t *testing.T, taken ...string) string {
	t.Helper()
	m := os.Getenv(mutateEnv)
	switch m {
	case "":
		return ""
	case "corrupt-byte", "skip-frame", "ready-zero":
	default:
		t.Fatalf("%s=%q: unknown mutation", mutateEnv, m)
	}
	for _, v := range taken {
		if v == m {
			t.Logf("MUTATION %s: this test must fail", m)
			return m
		}
	}
	t.Skipf("mutation %s is not for this test", m)
	return ""
}

// writeDeterministic writes size pseudo-random bytes (the same ones for the
// same seed) to path and returns their SHA-256, computed as they are written:
// the file is never in memory.
func writeDeterministic(t *testing.T, path string, size int64, seed byte) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	rng := rand.NewChaCha8([32]byte{seed, 'e', 'm', 'u'})
	buf := make([]byte, 1<<20)
	for left := size; left > 0; {
		n := min(int64(len(buf)), left)
		rng.Read(buf[:n])
		h.Write(buf[:n])
		if _, err := f.Write(buf[:n]); err != nil {
			t.Fatal(err)
		}
		left -= n
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pushRandomFile makes a file of size bytes, pushes it to /sdcard/Download of
// the emulator (removed at cleanup) and returns the SHA-256 of what is there:
// the one the push left, read by the emulator itself, which must be the
// generated one. It is the truth the upload is compared with, never what the
// app announces.
func (e *emu) pushRandomFile(t *testing.T, name string, size int64, seed byte) string {
	t.Helper()
	local := filepath.Join(t.TempDir(), name)
	want := writeDeterministic(t, local, size, seed)
	remote := "/sdcard/Download/" + name
	e.atCleanup(t, "shell", "rm -f "+remote)
	e.run(t, "push", local, remote)
	_ = os.Remove(local)
	out := e.shell(t, "sha256sum "+remote+" && stat -c %s "+remote)
	lines := strings.Fields(out)
	if len(lines) != 3 {
		t.Fatalf("sha256sum/stat of %s: %q", remote, out)
	}
	if lines[0] != want {
		t.Fatalf("the pushed file has sha256 %s, generated %s", lines[0], want)
	}
	if lines[2] != fmtN(size) {
		t.Fatalf("the pushed file has %s bytes, generated %d", lines[2], size)
	}
	return lines[0]
}

// pickFile attaches the file called name from /sdcard/Download through the
// system's picker (DocumentsUI of android-34, T1): Attach, File, then the
// Downloads root (the picker opens on "Recent", empty for a file nobody
// indexed; the second time it opens on Downloads itself) and the file's name.
func (e *emu) pickFile(t *testing.T, name string) {
	t.Helper()
	e.tap(t, e.waitNode(t, "Attach", 10*time.Second, func(n uiNode) bool { return n.Desc == "Attach" }))
	e.tap(t, e.waitNode(t, "File", 10*time.Second, func(n uiNode) bool { return n.Text == "File" }))
	var last []uiNode
	if !poll(40*time.Second, func() bool {
		last = e.screen(t)
		var roots, downloads *uiNode
		for i, n := range last {
			switch {
			case n.ID == "android:id/title" && n.Text == name:
				e.tap(t, n)
				return true
			case n.Desc == "Show roots":
				roots = &last[i]
			case n.ID == "android:id/title" && n.Text == "Downloads":
				downloads = &last[i]
			}
		}
		switch {
		case downloads != nil:
			e.tap(t, *downloads)
		case roots != nil:
			e.tap(t, *roots)
		}
		return false
	}) {
		t.Fatalf("the picker never showed %s: %+v", name, last)
	}
}

var idRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// uploadRecv is the host's side of an upload: what attach.begin announced
// and, frame by frame, what really arrived.
type uploadRecv struct {
	mutate string // see mutateEnv

	mu        sync.Mutex
	id        bridge.FrameID
	known     bool
	recv      int64 // bytes taken in order: the offset the host can say it has
	delivered int64 // bytes of every frame that came, taken or not
	h         hash.Hash
	nframes   int
	firstOff  int64 // offset of the first frame since mark; -1: none yet
	sinceMark int64 // bytes delivered since mark
	bad       []string
	// frozen: frames that come now are ignored (not counted, not taken); see freeze.
	frozen bool

	// slow is waited on each frame while it is set: the host is slow, so the
	// whole file does not arrive before the test cuts.
	slow time.Duration
	// stopAt closes reached the first time recv gets to it (0: never).
	stopAt  int64
	reached chan struct{}
}

func newUploadRecv(mutate string) *uploadRecv {
	return &uploadRecv{mutate: mutate, h: sha256.New(), firstOff: -1, reached: make(chan struct{})}
}

// begin starts (or, after a cut, goes on with) the upload id and returns
// where the host says it is: its recv, or 0 on the ready-zero mutation, which
// forgets what it had, as a host that restarts the file does.
func (u *uploadRecv) begin(t *testing.T, b bridge.AttachBegin, resumed bool) int64 {
	t.Helper()
	id, err := bridge.ParseFrameID(b.ID)
	if err != nil {
		t.Fatalf("attach.begin id %q: %v", b.ID, err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.id, u.known = id, true
	if resumed && u.mutate == "ready-zero" {
		u.recv, u.h = 0, sha256.New()
	}
	return u.recv
}

// freeze stops the host taking frames and returns what it has taken until now
// (recv): from here every frame that is still in flight is ignored, so the
// offset the host will give after the cut is exact and cannot change, whatever
// the reader was doing (stop does not wait for a frame inside onFrame to be
// counted; this does not depend on that timing). The app sends again from that
// offset, so what is ignored comes back. mark lifts it.
func (u *uploadRecv) freeze() int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.frozen = true
	return u.recv
}

// mark forgets the "since" counters: what comes next is counted from here.
func (u *uploadRecv) mark() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.firstOff, u.sinceMark = -1, 0
	u.slow = 0
	u.frozen = false
}

// onFrame is the link's frame handler (it never cuts).
func (u *uploadRecv) onFrame(id bridge.FrameID, off int64, data []byte) {
	u.mu.Lock()
	slow := u.slow
	u.mu.Unlock()
	if slow > 0 {
		time.Sleep(slow)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.frozen {
		return
	}
	if !u.known || id != u.id {
		u.bad = append(u.bad, "frame of "+id.String()+" nobody asked for")
		return
	}
	u.nframes++
	u.delivered += int64(len(data))
	u.sinceMark += int64(len(data))
	if u.firstOff < 0 {
		u.firstOff = off
	}
	// The mutations are the host's: they change what it keeps, not what came.
	switch {
	case u.mutate == "skip-frame" && u.nframes == 5:
		return
	case u.mutate == "corrupt-byte" && u.nframes == 5:
		data[len(data)/2] ^= 0xff
	}
	if off != u.recv {
		u.bad = append(u.bad, "frame at "+fmtN(off)+", wanted "+fmtN(u.recv))
		return
	}
	u.h.Write(data)
	u.recv += int64(len(data))
	if u.stopAt > 0 && u.recv >= u.stopAt {
		select {
		case <-u.reached:
		default:
			close(u.reached)
		}
	}
}

// state returns recv, what was delivered, and the SHA-256 of what was taken.
func (u *uploadRecv) state() (recv, delivered int64, sum string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.recv, u.delivered, hex.EncodeToString(u.h.Sum(nil))
}

// finishUpload checks the upload at attach.end against the truth (size and sha of
// the file the test pushed, never the app's announcement alone) and answers
// attach.done with what the host has. Every difference is an error.
func (l *link) finishUpload(t *testing.T, u *uploadRecv, b bridge.AttachBegin, size int64, pushedSHA string) {
	t.Helper()
	endMsg := l.waitFor(t, bridge.TypeAttachEnd, 90*time.Second)
	end := payload[bridge.AttachEnd](t, endMsg)
	if end.ID != b.ID {
		t.Errorf("attach.end of %s, the upload is %s", end.ID, b.ID)
	}
	recv, delivered, sum := u.state()
	u.mu.Lock()
	bad := append([]string(nil), u.bad...)
	u.mu.Unlock()
	if len(bad) > 0 {
		if len(bad) > 3 {
			bad = append(bad[:3:3], "... "+fmtN(int64(len(bad)))+" in all")
		}
		t.Errorf("frames out of order or foreign: %v", bad)
	}
	if recv != size {
		t.Errorf("the host took %d bytes in order, the file has %d", recv, size)
	}
	if delivered != size {
		t.Errorf("the app sent %d bytes in all, the file has %d (duplicates or holes)", delivered, size)
	}
	if sum != pushedSHA {
		t.Errorf("sha256 of what the host received %s != sha256 of the pushed bytes %s", sum, pushedSHA)
	}
	// (pickAndBegin already checked that the app announced the pushed bytes' sha256.)
	if sum != b.SHA256 {
		t.Errorf("received %s != announced %s", sum, b.SHA256)
	}
	l.reply(t, endMsg, bridge.TypeAttachDone, bridge.AttachDone{Attachment: bridge.Attachment{
		ID: b.ID, Name: b.Name, MIME: b.MIME, Size: recv, SHA256: sum,
	}})
}

func fmtN(n int64) string { return strconv.FormatInt(n, 10) }

// pickAndBegin picks name in the chat's picker and waits for the app's
// attach.begin, which it checks against the pushed file: the id the app chose,
// the size and the SHA-256 of the real bytes.
func (l *link) pickAndBegin(t *testing.T, name string, size int64, pushedSHA string) (bridge.AttachBegin, bridge.Message) {
	t.Helper()
	l.drain()
	l.e.pickFile(t, name)
	m := l.waitFor(t, bridge.TypeAttachBegin, 90*time.Second)
	b := payload[bridge.AttachBegin](t, m)
	if !idRe.MatchString(b.ID) {
		t.Errorf("attach.begin id %q is not 32 hex digits", b.ID)
	}
	if b.Size != size || b.SHA256 != pushedSHA {
		t.Fatalf("attach.begin announces %d bytes sha256 %s, the pushed file has %d bytes sha256 %s", b.Size, b.SHA256, size, pushedSHA)
	}
	return b, m
}

// Validation 3 of M3b (a): a file chosen in the picker is uploaded by the
// channel and the host gets exactly its bytes: the SHA-256 of what arrived is
// that of the bytes pushed to the phone (computed apart, as it was written),
// not only the one the app announces, and no byte is missing, damaged or
// repeated.
func TestUploadSendsTheRealBytes(t *testing.T) {
	mutate := mutation(t, "corrupt-byte", "skip-frame")
	l := openLink(t)
	e := l.e
	const name = "emutest-upload-a.bin"
	const size = 20 << 20
	pushedSHA := e.pushRandomFile(t, name, size, 1)
	l.openChat(t)

	u := newUploadRecv(mutate)
	l.setOnFrame(u.onFrame)
	b, m := l.pickAndBegin(t, name, size, pushedSHA)
	if b.Name != name {
		t.Errorf("attach.begin name %q, want %q", b.Name, name)
	}
	off := u.begin(t, b, false)
	if off != 0 {
		t.Fatalf("a first upload starts at %d", off)
	}
	l.reply(t, m, bridge.TypeAttachReady, bridge.AttachReady{ID: b.ID, Offset: off})
	l.finishUpload(t, u, b, size, pushedSHA)
	e.waitNode(t, "the sent file's chip", 15*time.Second, func(n uiNode) bool { return strings.Contains(n.Text+n.Desc, name) })
}

// Validation 3 of M3b (b): the channel drops in the middle of a big upload
// (100 MiB; T1 measured 15 s for the whole case). The app reconnects, sends
// attach.begin again with the same id and the same hash, the host says how
// many bytes it has, and the app sends only the rest: the first frame is at
// that offset, what comes after the cut is exactly size - offset, the file
// is size bytes in all with no byte twice, and its SHA-256 is that of the
// bytes pushed.
//
// The host is slowed down (10 ms a frame) until the cut, or the whole file
// would be here in 2.4 s, before the test could cut; the cut is asked from
// the test's goroutine once the threshold passed, never from the frame
// handler, which stop would wait for.
func TestUploadResumesAfterACut(t *testing.T) {
	mutate := mutation(t, "ready-zero")
	l := openLink(t)
	e := l.e
	const name = "emutest-upload-b.bin"
	const size = 100 << 20
	const cutAfter = 30 << 20
	pushedSHA := e.pushRandomFile(t, name, size, 2)
	l.openChat(t)

	u := newUploadRecv(mutate)
	u.slow, u.stopAt = 10*time.Millisecond, cutAfter
	l.setOnFrame(u.onFrame)
	b, m := l.pickAndBegin(t, name, size, pushedSHA)
	if off := u.begin(t, b, false); off != 0 {
		t.Fatalf("a first upload starts at %d", off)
	}
	l.reply(t, m, bridge.TypeAttachReady, bridge.AttachReady{ID: b.ID, Offset: 0})

	select {
	case <-u.reached:
	case <-time.After(60 * time.Second):
		recv, _, _ := u.state()
		t.Fatalf("the upload did not get to %d bytes in 60 s (%d)", cutAfter, recv)
	case <-e.ctx.Done():
		t.Fatal("test context done")
	}
	// Frozen before the cut: atCut is the offset the host will answer, and no
	// frame still in flight can move it (the cut is asked from here, not from
	// onFrame).
	atCut := u.freeze()
	l.cut()
	if atCut < cutAfter || atCut >= size {
		t.Fatalf("cut with %d of %d bytes: not in the middle of the upload", atCut, size)
	}
	u.mark()
	hellos := len(l.Hellos())
	l.resume(t)
	l.waitHellos(t, hellos+1, 60*time.Second)

	m2 := l.waitFor(t, bridge.TypeAttachBegin, 90*time.Second)
	b2 := payload[bridge.AttachBegin](t, m2)
	if b2.ID != b.ID || b2.SHA256 != b.SHA256 || b2.Size != size {
		t.Fatalf("the app began again with %+v, want the same upload %+v", b2, b)
	}
	offset := u.begin(t, b2, true)
	l.reply(t, m2, bridge.TypeAttachReady, bridge.AttachReady{ID: b.ID, Offset: offset})
	// What the host says is what it had: the exact bytes it took before the cut.
	if offset != atCut {
		t.Errorf("the host answered attach.ready at %d, it had taken %d bytes", offset, atCut)
	}

	l.finishUpload(t, u, b, size, pushedSHA)
	u.mu.Lock()
	first, since := u.firstOff, u.sinceMark
	u.mu.Unlock()
	if first != offset {
		t.Errorf("the first frame after the cut is at %d, the host said %d", first, offset)
	}
	if since != size-atCut {
		t.Errorf("%d bytes came after the cut, want size - the %d taken before = %d", since, atCut, size-atCut)
	}
}
