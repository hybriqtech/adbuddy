package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// shotPhone is the phone of appPhone with an adb that can return bytes for
// exec-out, which appPhone cannot.
type shotPhone struct {
	*appPhone
	mu   sync.Mutex
	args [][]string
	// shot answers `exec-out screencap -p`.
	shot func(ctx context.Context, stdout, stderr io.Writer) (int, error)
}

func (s *shotPhone) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) > 0 && args[0] == "exec-out" {
		s.mu.Lock()
		s.args = append(s.args, args)
		s.mu.Unlock()
		return s.shot(ctx, stdout, stderr)
	}
	return s.appPhone.Run(ctx, dir, args, stdin, stdout, stderr)
}

func (s *shotPhone) calls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]string{}, s.args...)
}

// filesRig is a link with an environment and an app connected.
type filesRig struct {
	l     *phoneLink
	c     *websocket.Conn
	ws    string
	port  int
	token string
	phone *shotPhone
}

func newFilesRig(t *testing.T, shot func(ctx context.Context, stdout, stderr io.Writer) (int, error)) *filesRig {
	t.Helper()
	e := linkEnv(t)
	f := &appPhone{}
	sp := &shotPhone{appPhone: f, shot: shot}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app := &App{Stderr: &syncBuffer{}, Stdout: io.Discard, Getenv: func(string) string { return "" }}
	l, err := startPhoneLink(ctx, app, e, sp, "s_1", "claude", &recording{})
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	l.useWorkspace(ws)
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	r := &filesRig{l: l, ws: ws, port: e.cfg.ChatPort, token: f.handedTokens()[0], phone: sp}
	r.c = r.connect(t)
	return r
}

// connect opens a connection as the app (replacing the one before).
func (r *filesRig) connect(t *testing.T) *websocket.Conn {
	t.Helper()
	c := openApp(t, r.port, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: r.token, AppVersion: "0.3.0"})
	c.SetReadLimit(1 << 20) // a frame of 256 KiB
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	return c
}

func fileID(n int) string { return fmt.Sprintf("%032x", n) }

func sumOf(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func testBytes(n int) []byte {
	b := make([]byte, n)
	seed := uint32(7)
	for i := range b {
		seed = seed*1664525 + 1013904223
		b[i] = byte(seed >> 24)
	}
	return b
}

func sendFrame(t *testing.T, c *websocket.Conn, id string, off int64, data []byte) {
	t.Helper()
	fid, err := bridge.ParseFrameID(id)
	if err != nil {
		t.Fatal(err)
	}
	f, err := bridge.EncodeFrame(fid, off, data)
	if err != nil {
		t.Fatal(err)
	}
	sendRaw(t, c, websocket.MessageBinary, f)
}

func sendRaw(t *testing.T, c *websocket.Conn, typ websocket.MessageType, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, typ, data); err != nil {
		t.Fatal(err)
	}
}

// sendData sends data as frames of chunk bytes from offset from.
func sendData(t *testing.T, c *websocket.Conn, id string, data []byte, from, chunk int) {
	t.Helper()
	for off := from; off < len(data); off += chunk {
		sendFrame(t, c, id, int64(off), data[off:min(off+chunk, len(data))])
	}
}

func (r *filesRig) begin(t *testing.T, rid, id, name string, data []byte) {
	t.Helper()
	sendApp(t, r.c, bridge.TypeAttachBegin, rid, bridge.AttachBegin{ID: id, Name: name, MIME: "application/octet-stream", Size: int64(len(data)), SHA256: sumOf(data)})
}

func wantType(t *testing.T, m bridge.Message, rid, typ string) {
	t.Helper()
	if m.Type != typ || m.RID != rid {
		t.Fatalf("got %s rid %q %s, want %s rid %q", m.Type, m.RID, m.Payload, typ, rid)
	}
}

func wantReady(t *testing.T, m bridge.Message, rid string, offset int64) {
	t.Helper()
	wantType(t, m, rid, bridge.TypeAttachReady)
	var ar bridge.AttachReady
	if err := json.Unmarshal(m.Payload, &ar); err != nil || ar.Offset != offset {
		t.Fatalf("ready %s (%v), want offset %d", m.Payload, err, offset)
	}
}

func wantFileError(t *testing.T, m bridge.Message, rid, code string) bridge.ErrorReply {
	t.Helper()
	wantType(t, m, rid, bridge.TypeError)
	var er bridge.ErrorReply
	if err := json.Unmarshal(m.Payload, &er); err != nil || er.Code != code {
		t.Fatalf("error %s (%v), want %q", m.Payload, err, code)
	}
	return er
}

func wantDone(t *testing.T, m bridge.Message, rid string) bridge.Attachment {
	t.Helper()
	wantType(t, m, rid, bridge.TypeAttachDone)
	var ad bridge.AttachDone
	if err := json.Unmarshal(m.Payload, &ad); err != nil {
		t.Fatal(err)
	}
	return ad.Attachment
}

// upload does a whole upload and returns the attachment.
func (r *filesRig) upload(t *testing.T, id, name string, data []byte) bridge.Attachment {
	t.Helper()
	r.begin(t, "r_b"+id[28:], id, name, data)
	wantReady(t, readApp(t, r.c), "r_b"+id[28:], 0)
	sendData(t, r.c, id, data, 0, 100)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_e"+id[28:], bridge.AttachEnd{ID: id})
	return wantDone(t, readApp(t, r.c), "r_e"+id[28:])
}

func (r *filesRig) inboxFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(r.ws, "inbox"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func (r *filesRig) partials(t *testing.T) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(r.ws, "inbox", ".partial"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func noMessage(t *testing.T, c *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, data, err := c.Read(ctx); err == nil {
		t.Fatalf("got %.200s", data)
	}
}

func pngData(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// ---- uploads ----

// A whole upload: ready at 0, frames, end, done with the file in inbox/ named
// with the date and the cleaned name, registered, and no .partial left.
func TestUploadCompletes(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 30, 45, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	r := newFilesRig(t, nil)
	data := pngData(300, 200)
	att := r.upload(t, fileID(1), "../mi foto.png", data)

	if att.ID != fileID(1) || att.Name != "mi foto.png" || att.Size != int64(len(data)) || att.SHA256 != sumOf(data) || att.MIME != "application/octet-stream" || att.Thumb == "" {
		t.Fatalf("attachment %+v", att)
	}
	want := "20261001-123045-mi foto.png"
	got, err := os.ReadFile(filepath.Join(r.ws, "inbox", want))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("file %s: %v", want, err)
	}
	stored, ok := r.l.chat.attachments.get(fileID(1))
	if !ok || stored.Path != "inbox/"+want {
		t.Fatalf("registry %+v %v", stored, ok)
	}
	if p := r.partials(t); len(p) != 0 {
		t.Fatalf("partial files left: %v", p)
	}
	if st, err := os.Stat(filepath.Join(r.ws, "inbox")); err != nil || runtime.GOOS != "windows" && st.Mode().Perm() != 0o700 {
		t.Fatalf("inbox %v %v", st, err)
	}
}

// An empty file is a valid upload.
func TestUploadOfAnEmptyFile(t *testing.T) {
	r := newFilesRig(t, nil)
	att := r.upload(t, fileID(1), "empty.txt", nil)
	if att.Size != 0 || att.SHA256 != sumOf(nil) {
		t.Fatalf("%+v", att)
	}
}

// The app resumes from the offset the host has: on the same connection and
// after another one replaced it. Another hash starts over.
func TestUploadResumes(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(1000)
	id := fileID(2)
	r.begin(t, "r_1", id, "big.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data[:600], 0, 200)

	// The same connection asks again (the app lost the ready).
	r.begin(t, "r_2", id, "big.bin", data)
	wantReady(t, readApp(t, r.c), "r_2", 600)

	// A new connection: what the old one began is forgotten, the bytes stay.
	r.c = r.connect(t)
	r.begin(t, "r_3", id, "big.bin", data)
	wantReady(t, readApp(t, r.c), "r_3", 600)
	sendData(t, r.c, id, data, 600, 150)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_4", bridge.AttachEnd{ID: id})
	att := wantDone(t, readApp(t, r.c), "r_4")
	if att.SHA256 != sumOf(data) {
		t.Fatalf("%+v", att)
	}
	files := r.inboxFiles(t)
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	if got, _ := os.ReadFile(filepath.Join(r.ws, "inbox", files[0])); !bytes.Equal(got, data) {
		t.Fatal("the resumed file differs")
	}

	// Another hash with the id of a partial one starts at 0.
	other := testBytes(1000)
	other[0] ^= 1
	id2 := fileID(3)
	r.begin(t, "r_5", id2, "a", data)
	wantReady(t, readApp(t, r.c), "r_5", 0)
	sendData(t, r.c, id2, data[:500], 0, 500)
	r.begin(t, "r_6", id2, "a", other)
	wantReady(t, readApp(t, r.c), "r_6", 0)
}

// A partial declared with another size is not resumed either.
func TestUploadDoesNotResumeWithAnotherSize(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(1000)
	id := fileID(4)
	r.begin(t, "r_1", id, "a", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data[:400], 0, 400)
	// Same sha256, other size: not the same file.
	sendApp(t, r.c, bridge.TypeAttachBegin, "r_2", bridge.AttachBegin{ID: id, Name: "a", Size: 2000, SHA256: sumOf(data)})
	wantReady(t, readApp(t, r.c), "r_2", 0)
}

// A partial larger than the declared size starts at 0.
func TestUploadPartialLargerThanDeclaredStartsOver(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(100)
	id := fileID(5)
	partials := filepath.Join(r.ws, "inbox", ".partial")
	os.MkdirAll(partials, 0o700)
	os.WriteFile(filepath.Join(partials, id), testBytes(500), 0o600)
	meta, _ := json.Marshal(partialMeta{SHA256: sumOf(data), Size: 100})
	os.WriteFile(filepath.Join(partials, id+".json"), meta, 0o600)
	r.begin(t, "r_1", id, "a", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	if st, _ := os.Stat(filepath.Join(partials, id)); st == nil || st.Size() != 0 {
		t.Fatalf("partial %v", st)
	}
}

// A retry of a finished upload: the same hash gets ready at the size and the
// same attachment at attach.end; another hash is invalid.
func TestRetryOfAFinishedUpload(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(500)
	id := fileID(6)
	att := r.upload(t, id, "a.bin", data)

	// Same connection.
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", int64(len(data)))
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	if again := wantDone(t, readApp(t, r.c), "r_2"); again != att {
		t.Fatalf("%+v, want %+v", again, att)
	}
	// Frames of a finished upload are ignored.
	sendFrame(t, r.c, id, int64(len(data)), []byte("late"))

	// After a reconnection.
	r.c = r.connect(t)
	r.begin(t, "r_3", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_3", int64(len(data)))
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_4", bridge.AttachEnd{ID: id})
	if again := wantDone(t, readApp(t, r.c), "r_4"); again != att {
		t.Fatalf("%+v, want %+v", again, att)
	}
	if n := len(r.inboxFiles(t)); n != 1 {
		t.Fatalf("%d files in inbox", n)
	}

	// Another hash with that id.
	other := testBytes(500)
	other[3] ^= 1
	r.begin(t, "r_5", id, "a.bin", other)
	wantFileError(t, readApp(t, r.c), "r_5", bridge.CodeInvalid)
}

// Frames that do not belong are dropped: out of order, of an id never
// accepted, and those that reach the host after a new connection started.
func TestUploadFramesThatDoNotBelongAreDropped(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(600)
	id := fileID(7)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)

	sendFrame(t, r.c, id, 100, data[100:200]) // out of order: nothing written yet
	sendFrame(t, r.c, fileID(99), 0, data)    // an id nobody began
	sendFrame(t, r.c, id, 0, data[:200])
	sendFrame(t, r.c, id, 0, data[:200])      // again: not where the file stands
	sendFrame(t, r.c, id, 300, data[300:400]) // skipping 200..300
	sendFrame(t, r.c, id, 200, data[200:300])
	// A round trip on the old connection: its frames were all processed (the
	// reader takes messages in order) before the new connection comes.
	r.begin(t, "r_sync", fileID(50), "z", testBytes(5))
	wantReady(t, readApp(t, r.c), "r_sync", 0)
	// Another connection: the uploads of this one are forgotten, so a frame
	// of the old one that is still on its way is dropped.
	r.c = r.connect(t)
	r.l.onUploadFrame(mustFrameID(t, id), 300, data[300:400])
	r.begin(t, "r_2", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_2", 300) // 200 + 100: only the good ones were written
	sendData(t, r.c, id, data, 300, 300)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_3", bridge.AttachEnd{ID: id})
	wantDone(t, readApp(t, r.c), "r_3")
	if _, ok := r.l.chat.attachments.get(fileID(99)); ok {
		t.Fatal("an upload nobody began exists")
	}
}

func mustFrameID(t *testing.T, id string) bridge.FrameID {
	t.Helper()
	fid, err := bridge.ParseFrameID(id)
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

// More data than declared: the upload is dropped and attach.end says invalid
// (frames carry no rid); a new begin starts clean.
func TestUploadPastItsSizeIsInvalid(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(100)
	id := fileID(8)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendFrame(t, r.c, id, 0, data[:50])
	sendFrame(t, r.c, id, 50, testBytes(80)) // 130 > 100
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeInvalid)
	if p := r.partials(t); len(p) != 0 {
		t.Fatalf("left %v", p)
	}
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_3", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_3", bridge.CodeNotFound)
	if n := len(r.inboxFiles(t)); n != 0 {
		t.Fatalf("%d files", n)
	}
}

// What does not match the size or the hash is checksum, and starts over.
func TestUploadChecksum(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(300)
	bad := append([]byte{}, data...)
	bad[10] ^= 0xff
	id := fileID(9)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, bad, 0, 300)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeChecksum)
	if p := r.partials(t); len(p) != 0 || len(r.inboxFiles(t)) != 0 {
		t.Fatalf("left %v %v", p, r.inboxFiles(t))
	}

	// Too short.
	r.begin(t, "r_3", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_3", 0)
	sendData(t, r.c, id, data[:100], 0, 100)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_4", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_4", bridge.CodeChecksum)
	if _, ok := r.l.chat.attachments.get(id); ok {
		t.Fatal("registered")
	}
}

// attach.end without a begin.
func TestAttachEndWithoutBegin(t *testing.T) {
	r := newFilesRig(t, nil)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_1", bridge.AttachEnd{ID: fileID(1)})
	wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeNotFound)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: "nope"})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeInvalid)
}

func TestAttachBeginIsChecked(t *testing.T) {
	r := newFilesRig(t, nil)
	ok := bridge.AttachBegin{ID: fileID(1), Name: "a", MIME: "text/plain", Size: 10, SHA256: sumOf(nil)}
	for i, c := range []struct {
		name string
		mod  func(*bridge.AttachBegin)
		code string
	}{
		{"short id", func(a *bridge.AttachBegin) { a.ID = "abc" }, bridge.CodeInvalid},
		{"upper-case id", func(a *bridge.AttachBegin) { a.ID = strings.ToUpper(fileID(0xabcdef)) }, bridge.CodeInvalid},
		{"short sha", func(a *bridge.AttachBegin) { a.SHA256 = "abcd" }, bridge.CodeInvalid},
		{"non-hex sha", func(a *bridge.AttachBegin) { a.SHA256 = strings.Repeat("g", 64) }, bridge.CodeInvalid},
		{"negative size", func(a *bridge.AttachBegin) { a.Size = -1 }, bridge.CodeInvalid},
		{"long mime", func(a *bridge.AttachBegin) { a.MIME = strings.Repeat("a", 256) }, bridge.CodeInvalid},
		{"size over 200 MiB", func(a *bridge.AttachBegin) { a.Size = 200<<20 + 1 }, bridge.CodeTooLarge},
	} {
		a := ok
		c.mod(&a)
		rid := fmt.Sprintf("r_%d", i)
		sendApp(t, r.c, bridge.TypeAttachBegin, rid, a)
		wantFileError(t, readApp(t, r.c), rid, c.code)
	}
	// 255 characters of mime and 200 MiB exactly are fine.
	a := ok
	a.MIME = strings.Repeat("a", 255)
	a.Size = 200 << 20
	sendApp(t, r.c, bridge.TypeAttachBegin, "r_ok", a)
	wantReady(t, readApp(t, r.c), "r_ok", 0)
	sendApp(t, r.c, bridge.TypeAttachBegin, "r_bad", json.RawMessage(`"not an object"`))
	wantFileError(t, readApp(t, r.c), "r_bad", bridge.CodeInvalid)
	if len(r.l.chat.attachments.m) != 0 {
		t.Fatal("something registered")
	}
}

// At most 4 uploads at once on a connection; a finished one frees its place.
func TestTooManyUploadsAreBusy(t *testing.T) {
	r := newFilesRig(t, nil)
	datas := make([][]byte, 6)
	for i := range 4 {
		datas[i] = testBytes(10 + i)
		r.begin(t, fmt.Sprintf("r_%d", i), fileID(i+1), "a", datas[i])
		wantReady(t, readApp(t, r.c), fmt.Sprintf("r_%d", i), 0)
	}
	datas[4] = testBytes(20)
	r.begin(t, "r_4", fileID(5), "a", datas[4])
	wantFileError(t, readApp(t, r.c), "r_4", bridge.CodeBusy)

	// The same id again (a retry) is not a fifth one.
	r.begin(t, "r_re", fileID(1), "a", datas[0])
	wantReady(t, readApp(t, r.c), "r_re", 0)

	sendData(t, r.c, fileID(2), datas[1], 0, 100)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_end", bridge.AttachEnd{ID: fileID(2)})
	wantDone(t, readApp(t, r.c), "r_end")
	r.begin(t, "r_5", fileID(5), "a", datas[4])
	wantReady(t, readApp(t, r.c), "r_5", 0)

	// A new connection starts the count over.
	r.c = r.connect(t)
	r.begin(t, "r_6", fileID(6), "a", datas[4])
	wantReady(t, readApp(t, r.c), "r_6", 0)
}

// The quota counts inbox/ (files and partials) and the declared size of the
// uploads in progress, and its message says to free space.
func TestInboxQuota(t *testing.T) {
	setVar(t, &inboxQuota, 1000)
	r := newFilesRig(t, nil)
	r.upload(t, fileID(1), "a.bin", testBytes(600))

	data := testBytes(500)
	r.begin(t, "r_1", fileID(2), "b.bin", data)
	er := wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeTooLarge)
	if !strings.Contains(er.Text, "free space") {
		t.Fatalf("text %q", er.Text)
	}
	small := testBytes(400)
	r.begin(t, "r_2", fileID(3), "c.bin", small) // 600 + 400 fits
	wantReady(t, readApp(t, r.c), "r_2", 0)
	// 1000 is spent: what is declared counts before it is written.
	r.begin(t, "r_3", fileID(4), "d.bin", testBytes(1))
	wantFileError(t, readApp(t, r.c), "r_3", bridge.CodeTooLarge)
	// Free space: deleting a file lets it in.
	for _, f := range r.inboxFiles(t) {
		os.Remove(filepath.Join(r.ws, "inbox", f))
	}
	r.begin(t, "r_4", fileID(4), "d.bin", testBytes(1))
	wantReady(t, readApp(t, r.c), "r_4", 0)
}

// The partial files of another connection count for the quota too.
func TestInboxQuotaCountsPartialFiles(t *testing.T) {
	setVar(t, &inboxQuota, 1000)
	r := newFilesRig(t, nil)
	id := fileID(1)
	data := testBytes(900)
	r.begin(t, "r_1", id, "a", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data[:800], 0, 400)
	r.c = r.connect(t) // the 800 B stay in .partial
	r.begin(t, "r_2", fileID(2), "b", testBytes(300))
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeTooLarge)
	r.begin(t, "r_3", id, "a", data) // resuming it is not counted twice
	wantReady(t, readApp(t, r.c), "r_3", 800)
}

// Names are cleaned, and two uploads with the same name in the same second
// do not overwrite each other, nor a file that is already there.
func TestUploadNames(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	r := newFilesRig(t, nil)
	inbox := filepath.Join(r.ws, "inbox")
	os.MkdirAll(inbox, 0o700)
	os.WriteFile(filepath.Join(inbox, "20260102-030405-a.txt"), []byte("mine"), 0o600)

	var names []string
	for i, n := range []string{"a.txt", "a.txt", "a.txt", `..\x`, "../y", "CON", "con.txt", "a:b", "x. ", "", strings.Repeat("ñ", 300)} {
		data := []byte(fmt.Sprintf("content %d", i))
		att := r.upload(t, fileID(100+i), n, data)
		stored, _ := r.l.chat.attachments.get(att.ID)
		names = append(names, strings.TrimPrefix(stored.Path, "inbox/"))
		if got, err := os.ReadFile(filepath.Join(r.ws, filepath.FromSlash(stored.Path))); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("%s: %q %v", stored.Path, got, err)
		}
	}
	want := []string{
		"20260102-030405-a-2.txt", "20260102-030405-a-3.txt", "20260102-030405-a-4.txt",
		"20260102-030405-x", "20260102-030405-y", "20260102-030405-_CON", "20260102-030405-_con.txt",
		"20260102-030405-a_b", "20260102-030405-x-2", "20260102-030405-fichero",
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("name %d: %q, want %q", i, names[i], w)
		}
	}
	if long := names[10]; len(long) > len("20260102-030405-")+100+len("-2") || !strings.HasPrefix(long, "20260102-030405-ñ") {
		t.Errorf("long name %q (%d bytes)", long, len(long))
	}
	if got, _ := os.ReadFile(filepath.Join(inbox, "20260102-030405-a.txt")); string(got) != "mine" {
		t.Fatalf("a file that was there was overwritten: %q", got)
	}
}

// Two uploads that finish at once get different names.
func TestSimultaneousUploadsWithTheSameName(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	r := newFilesRig(t, nil)
	d1, d2 := testBytes(50), testBytes(60)
	r.begin(t, "r_1", fileID(1), "same.bin", d1)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	r.begin(t, "r_2", fileID(2), "same.bin", d2)
	wantReady(t, readApp(t, r.c), "r_2", 0)
	sendData(t, r.c, fileID(1), d1, 0, 100)
	sendData(t, r.c, fileID(2), d2, 0, 100)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_3", bridge.AttachEnd{ID: fileID(1)})
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_4", bridge.AttachEnd{ID: fileID(2)})
	wantDone(t, readApp(t, r.c), "r_3")
	wantDone(t, readApp(t, r.c), "r_4")
	files := r.inboxFiles(t)
	if len(files) != 2 {
		t.Fatalf("files %v", files)
	}
	for _, f := range files {
		got, _ := os.ReadFile(filepath.Join(r.ws, "inbox", f))
		if !bytes.Equal(got, d1) && !bytes.Equal(got, d2) {
			t.Fatalf("%s has other content", f)
		}
	}
}

// A .partial that is a symbolic link is not followed: the file it points to
// is neither read nor written, and the upload starts at 0.
func TestPartialSymlinkIsNotFollowed(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(200)
	id := fileID(1)
	partials := filepath.Join(r.ws, "inbox", ".partial")
	os.MkdirAll(partials, 0o700)
	target := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(target, []byte("do not touch"), 0o600)
	if err := os.Symlink(target, filepath.Join(partials, id)); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	meta, _ := json.Marshal(partialMeta{SHA256: sumOf(data), Size: 200})
	os.WriteFile(filepath.Join(partials, id+".json"), meta, 0o600)

	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data, 0, 100)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantDone(t, readApp(t, r.c), "r_2")
	if got, _ := os.ReadFile(target); string(got) != "do not touch" {
		t.Fatalf("the target was written: %q", got)
	}
}

// Uploads that did not advance for an hour are deleted when a new one begins.
func TestStaleUploadsAreDeleted(t *testing.T) {
	r := newFilesRig(t, nil)
	partials := filepath.Join(r.ws, "inbox", ".partial")
	os.MkdirAll(partials, 0o700)
	old := time.Now().Add(-2 * time.Hour)
	for _, n := range []string{fileID(1), fileID(1) + ".json"} {
		os.WriteFile(filepath.Join(partials, n), []byte("x"), 0o600)
		os.Chtimes(filepath.Join(partials, n), old, old)
	}
	os.WriteFile(filepath.Join(partials, fileID(2)), []byte("fresh"), 0o600)

	r.begin(t, "r_1", fileID(3), "a", testBytes(5))
	wantReady(t, readApp(t, r.c), "r_1", 0)
	got := r.partials(t)
	if len(got) != 3 || got[0] != fileID(2) {
		t.Fatalf("left %v, want the fresh one and the new one", got)
	}

	// One of this connection that stands still for an hour.
	later := time.Now().Add(90 * time.Minute)
	setVar(t, &filesNow, func() time.Time { return later })
	r.begin(t, "r_2", fileID(4), "a", testBytes(5))
	wantReady(t, readApp(t, r.c), "r_2", 0)
	for _, n := range r.partials(t) {
		if n == fileID(3) || n == fileID(3)+".json" || n == fileID(2) {
			t.Fatalf("%s was kept: %v", n, r.partials(t))
		}
	}
}

// An upload then a chat.send with it: the agent gets the path in inbox/.
func TestUploadedAttachmentGoesIntoAChatSend(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	cr := newChatRig(t)
	cr.l.useWorkspace(t.TempDir())
	fr := &filesRig{l: cr.l, c: cr.c, ws: cr.l.ws}
	cr.start(t, "claude")
	data := testBytes(80)
	att := fr.upload(t, fileID(1), "nota.txt", data)

	sendApp(t, cr.c, bridge.TypeChatSend, "r_s", bridge.ChatSend{ID: "m_1", Text: "mira", Attachments: []string{att.ID}})
	m := cr.read(t, bridge.TypeChatMessage)
	if cm := chatMessageOf(t, m); len(cm.Attachments) != 1 || cm.Attachments[0].ID != att.ID {
		t.Fatalf("%+v", cm)
	}
	if got := cr.pty(0).said(); !strings.Contains(got, "- inbox/20260102-030405-nota.txt") {
		t.Fatalf("typed %q", got)
	}
}

// ---- downloads ----

// putStored makes a file in the environment and registers it.
func (r *filesRig) putStored(t *testing.T, id, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(r.ws, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	r.l.chat.attachments.put(bridge.Attachment{ID: id, Name: filepath.Base(p), MIME: "application/octet-stream", Size: int64(len(data)), SHA256: sumOf(data)}, rel)
}

type got struct {
	frames []int64 // offsets
	data   []byte
	end    *bridge.Message
}

// readDownload reads frames until the file.end (or another message, which it
// returns as end).
func readDownload(t *testing.T, c *websocket.Conn, id string, start int64) got {
	t.Helper()
	var g got
	fid := mustFrameID(t, id)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		typ, data, err := c.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("download: %v after %d frames", err, len(g.frames))
		}
		if typ == websocket.MessageBinary {
			gid, off, d, err := bridge.DecodeFrame(data)
			if err != nil || gid != fid {
				t.Fatalf("frame %v %v", gid, err)
			}
			if want := int64(len(g.data)) + start; off != want {
				t.Fatalf("frame at %d, want %d", off, want)
			}
			g.frames = append(g.frames, off)
			g.data = append(g.data, d...)
			continue
		}
		var m bridge.Message
		json.Unmarshal(data, &m)
		g.end = &m
		return g
	}
}

func TestDownloadComplete(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(2*bridge.MaxFrameData + 1000)
	id := fileID(1)
	r.putStored(t, id, "outbox/informe.bin", data)
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: id})
	g := readDownload(t, r.c, id, 0)
	wantType(t, *g.end, "r_1", bridge.TypeFileEnd)
	var fe bridge.FileEnd
	json.Unmarshal(g.end.Payload, &fe)
	if fe.ID != id || fe.Size != int64(len(data)) || len(g.frames) != 3 || !bytes.Equal(g.data, data) {
		t.Fatalf("end %+v, %d frames, %d bytes", fe, len(g.frames), len(g.data))
	}
	// Nothing is left running.
	waitUntil(t, "the download to be forgotten", func() bool {
		r.l.files.dlMu.Lock()
		defer r.l.files.dlMu.Unlock()
		return len(r.l.files.downloads) == 0
	})
}

func TestDownloadFromAnOffset(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(bridge.MaxFrameData + 500)
	id := fileID(1)
	r.putStored(t, id, "inbox/x.bin", data)
	off := int64(bridge.MaxFrameData - 100)
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: id, Offset: off})
	g := readDownload(t, r.c, id, off)
	wantType(t, *g.end, "r_1", bridge.TypeFileEnd)
	if !bytes.Equal(g.data, data[off:]) || g.frames[0] != off {
		t.Fatalf("%d bytes from %v", len(g.data), g.frames)
	}
	// At the end: only file.end.
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: id, Offset: int64(len(data))})
	g = readDownload(t, r.c, id, int64(len(data)))
	wantType(t, *g.end, "r_2", bridge.TypeFileEnd)
	if len(g.frames) != 0 {
		t.Fatalf("%d frames", len(g.frames))
	}
	// An empty file.
	r.putStored(t, fileID(2), "inbox/empty", nil)
	sendApp(t, r.c, bridge.TypeFileGet, "r_3", bridge.FileGet{ID: fileID(2)})
	g = readDownload(t, r.c, fileID(2), 0)
	wantType(t, *g.end, "r_3", bridge.TypeFileEnd)
}

func TestDownloadErrors(t *testing.T) {
	r := newFilesRig(t, nil)
	r.putStored(t, fileID(1), "inbox/x.bin", testBytes(100))
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: fileID(1), Offset: 101})
	wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeInvalid)
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: fileID(1), Offset: -1})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeInvalid)
	sendApp(t, r.c, bridge.TypeFileGet, "r_3", bridge.FileGet{ID: fileID(77)})
	wantFileError(t, readApp(t, r.c), "r_3", bridge.CodeNotFound)
	sendApp(t, r.c, bridge.TypeFileGet, "r_4", bridge.FileGet{ID: "zzz"})
	wantFileError(t, readApp(t, r.c), "r_4", bridge.CodeInvalid)
	// The file is gone from the disk.
	os.Remove(filepath.Join(r.ws, "inbox", "x.bin"))
	sendApp(t, r.c, bridge.TypeFileGet, "r_5", bridge.FileGet{ID: fileID(1)})
	wantFileError(t, readApp(t, r.c), "r_5", bridge.CodeFailed)
	// A path outside the environment in the registry is never opened.
	r.l.chat.attachments.put(bridge.Attachment{ID: fileID(2), Size: 5}, "../outside")
	sendApp(t, r.c, bridge.TypeFileGet, "r_6", bridge.FileGet{ID: fileID(2)})
	wantFileError(t, readApp(t, r.c), "r_6", bridge.CodeFailed)
}

// bigStored makes a file of many frames that the test then does not read, so
// the download stays waiting for room in the queue.
func (r *filesRig) bigStored(t *testing.T, id string) {
	t.Helper()
	p := filepath.Join(r.ws, "outbox")
	os.MkdirAll(p, 0o700)
	f, err := os.Create(filepath.Join(p, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	chunk := testBytes(1 << 20)
	for range 64 {
		f.Write(chunk)
	}
	f.Close()
	r.l.chat.attachments.put(bridge.Attachment{ID: id, Name: "big.bin", Size: 64 << 20}, "outbox/big.bin")
}

func (r *filesRig) downloadsRunning() int {
	r.l.files.dlMu.Lock()
	defer r.l.files.dlMu.Unlock()
	return len(r.l.files.downloads)
}

// file.cancel stops the download: no file.end follows, and it is forgotten.
func TestDownloadCancel(t *testing.T) {
	r := newFilesRig(t, nil)
	id := fileID(1)
	r.bigStored(t, id)
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: id})
	waitUntil(t, "the download", func() bool { return r.downloadsRunning() == 1 })
	sendApp(t, r.c, bridge.TypeFileCancel, "", bridge.FileCancel{ID: id})
	waitUntil(t, "the download to stop", func() bool { return r.downloadsRunning() == 0 })
	frames := 0
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		typ, data, err := r.c.Read(ctx)
		cancel()
		if err != nil {
			break
		}
		if typ == websocket.MessageText {
			t.Fatalf("a message after the cancel: %.200s", data)
		}
		frames++
	}
	if frames >= 256 {
		t.Fatalf("%d frames: the whole file was sent", frames)
	}
	// (A read that times out closes the socket.) On a new connection a file
	// can be asked for again.
	r.c = r.connect(t)
	r.putStored(t, fileID(2), "inbox/small", testBytes(10))
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: fileID(2)})
	wantType(t, *readDownload(t, r.c, fileID(2), 0).end, "r_2", bridge.TypeFileEnd)
}

// A new file.get for an id replaces the download in progress.
func TestADownloadReplacesTheOneOfTheSameID(t *testing.T) {
	r := newFilesRig(t, nil)
	id := fileID(1)
	r.bigStored(t, id)
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: id})
	waitUntil(t, "the download", func() bool { return r.downloadsRunning() == 1 })
	last := int64(64<<20 - 1000)
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: id, Offset: last})
	// Read everything until the end: the first download sent frames from 0,
	// then the second sends the last 1000 bytes and ends, once.
	ends := 0
	var lastOff int64 = -1
	for ends == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		typ, data, err := r.c.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if typ == websocket.MessageBinary {
			_, lastOff, _, _ = bridge.DecodeFrame(data)
			continue
		}
		var m bridge.Message
		json.Unmarshal(data, &m)
		wantType(t, m, "r_2", bridge.TypeFileEnd)
		ends++
	}
	if lastOff != last {
		t.Fatalf("the last frame was at %d, want %d", lastOff, last)
	}
	noMessage(t, r.c)
	waitUntil(t, "no downloads", func() bool { return r.downloadsRunning() == 0 })
}

// A download is tied to its connection: it ends when the app goes, and when
// another connection replaces it.
func TestDownloadEndsWithTheConnection(t *testing.T) {
	r := newFilesRig(t, nil)
	id := fileID(1)
	r.bigStored(t, id)
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: id})
	waitUntil(t, "the download", func() bool { return r.downloadsRunning() == 1 })
	r.c.CloseNow()
	waitUntil(t, "the download to end with the app", func() bool { return r.downloadsRunning() == 0 })

	r.c = r.connect(t)
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: id})
	waitUntil(t, "the download", func() bool { return r.downloadsRunning() == 1 })
	r.c = r.connect(t)
	waitUntil(t, "the download to end with the replacement", func() bool { return r.downloadsRunning() == 0 })
}

// ---- captures ----

func shotOf(data []byte) func(context.Context, io.Writer, io.Writer) (int, error) {
	return func(_ context.Context, stdout, _ io.Writer) (int, error) {
		stdout.Write(data)
		return 0, nil
	}
}

func TestCapture(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	img := pngData(600, 300)
	r := newFilesRig(t, shotOf(img))
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_1", bridge.Empty{})
	m := readApp(t, r.c)
	wantType(t, m, "r_1", bridge.TypeCaptureReady)
	var cr bridge.CaptureReady
	if err := json.Unmarshal(m.Payload, &cr); err != nil {
		t.Fatal(err)
	}
	a := cr.Attachment
	if a.Name != "captura.png" || a.MIME != "image/png" || a.Size != int64(len(img)) || a.SHA256 != sumOf(img) || a.Thumb == "" || bridge.FrameID(mustFrameID(t, a.ID)).String() != a.ID {
		t.Fatalf("%+v", a)
	}
	got, err := os.ReadFile(filepath.Join(r.ws, "inbox", "20260102-030405-captura.png"))
	if err != nil || !bytes.Equal(got, img) {
		t.Fatalf("file: %v", err)
	}
	if st, ok := r.l.chat.attachments.get(a.ID); !ok || st.Path != "inbox/20260102-030405-captura.png" {
		t.Fatalf("registry %+v %v", st, ok)
	}
	if calls := r.phone.calls(); len(calls) != 1 || strings.Join(calls[0], " ") != "exec-out screencap -p" {
		t.Fatalf("adb calls %v", calls)
	}
	// Again in the same second: another name.
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_2", bridge.Empty{})
	wantType(t, readApp(t, r.c), "r_2", bridge.TypeCaptureReady)
	if files := r.inboxFiles(t); len(files) != 2 {
		t.Fatalf("%v", files)
	}
}

func TestCaptureFailures(t *testing.T) {
	var mode struct {
		sync.Mutex
		run func(context.Context, io.Writer, io.Writer) (int, error)
	}
	r := newFilesRig(t, func(ctx context.Context, so, se io.Writer) (int, error) {
		mode.Lock()
		f := mode.run
		mode.Unlock()
		return f(ctx, so, se)
	})
	set := func(f func(context.Context, io.Writer, io.Writer) (int, error)) {
		mode.Lock()
		mode.run = f
		mode.Unlock()
	}
	try := func(rid string) bridge.ErrorReply {
		sendApp(t, r.c, bridge.TypeCaptureReq, rid, bridge.Empty{})
		return wantFileError(t, readApp(t, r.c), rid, bridge.CodeFailed)
	}

	set(func(_ context.Context, _, se io.Writer) (int, error) {
		io.WriteString(se, "error: device \x1b[31moffline")
		return 1, nil
	})
	if er := try("r_1"); !strings.Contains(er.Text, "device") || strings.Contains(er.Text, "\x1b") {
		t.Fatalf("text %q", er.Text)
	}
	set(func(context.Context, io.Writer, io.Writer) (int, error) { return -1, errors.New("adb not found") })
	if er := try("r_2"); !strings.Contains(er.Text, "adb not found") {
		t.Fatalf("text %q", er.Text)
	}
	set(shotOf([]byte("this is not a png")))
	try("r_3")
	set(shotOf(append(append([]byte{}, pngSignature...), 1, 2, 3)))
	try("r_4") // a signature and nothing else
	setVar(t, &captureTimeout, 100*time.Millisecond)
	set(func(ctx context.Context, _, _ io.Writer) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if er := try("r_5"); !strings.Contains(er.Text, "10 seconds") {
		t.Fatalf("text %q", er.Text)
	}
	if n := len(r.inboxFiles(t)); n != 0 {
		t.Fatalf("%d files after failures", n)
	}
	// Access cut from the phone: no screenshot.
	r.l.onPauseMessage(bridge.Message{Type: bridge.TypePause})
	readApp(t, r.c) // the session.state of the pause
	set(shotOf(pngData(10, 10)))
	if er := try("r_6"); !strings.Contains(er.Text, "resume") {
		t.Fatalf("text %q", er.Text)
	}
	if n := len(r.phone.calls()); n != 5 {
		t.Fatalf("%d adb calls: a paused link took a screenshot", n)
	}
}

// One at a time: while a screenshot is being taken, another request is busy,
// and the reader is free meanwhile (it answers other requests).
func TestCaptureIsOneAtATime(t *testing.T) {
	release, entered := make(chan struct{}), make(chan struct{}, 2)
	img := pngData(50, 50)
	r := newFilesRig(t, func(ctx context.Context, so, _ io.Writer) (int, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return -1, ctx.Err()
		}
		so.Write(img)
		return 0, nil
	})
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_1", bridge.Empty{})
	<-entered
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_2", bridge.Empty{})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeBusy)
	// The reader is not stuck in adb.
	sendApp(t, r.c, bridge.TypeFileGet, "r_3", bridge.FileGet{ID: fileID(5)})
	wantFileError(t, readApp(t, r.c), "r_3", bridge.CodeNotFound)
	close(release)
	wantType(t, readApp(t, r.c), "r_1", bridge.TypeCaptureReady)
	// Free again.
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_4", bridge.Empty{})
	wantType(t, readApp(t, r.c), "r_4", bridge.TypeCaptureReady)
}

// A capture counts for the quota.
func TestCaptureRespectsTheQuota(t *testing.T) {
	setVar(t, &inboxQuota, 100)
	r := newFilesRig(t, shotOf(pngData(50, 50)))
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_1", bridge.Empty{})
	if er := wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeFailed); !strings.Contains(er.Text, "free space") {
		t.Fatalf("text %q", er.Text)
	}
}

// Without an environment (a link that serves none) nothing is kept.
func TestFilesWithoutAnEnvironment(t *testing.T) {
	r := newFilesRig(t, shotOf(pngData(5, 5)))
	r.l.useWorkspace("")
	r.begin(t, "r_1", fileID(1), "a", testBytes(5))
	wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeFailed)
	sendApp(t, r.c, bridge.TypeCaptureReq, "r_2", bridge.Empty{})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeFailed)
}

// A .partial that is not a regular file (a folder here; a link is the case
// above, on Unix) is replaced, never used.
func TestPartialThatIsAFolderIsReplaced(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(50)
	id := fileID(1)
	partials := filepath.Join(r.ws, "inbox", ".partial")
	os.MkdirAll(filepath.Join(partials, id), 0o700)
	meta, _ := json.Marshal(partialMeta{SHA256: sumOf(data), Size: 50})
	os.WriteFile(filepath.Join(partials, id+".json"), meta, 0o600)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data, 0, 50)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantDone(t, readApp(t, r.c), "r_2")
}

// ---- round 1 of the review ----

// Names with two dots in a row are fine: they upload, register and download
// (only a path element that is exactly ".." is refused).
func TestNamesWithDoubleDotsDownload(t *testing.T) {
	r := newFilesRig(t, nil)
	for i, n := range []string{"a..b.txt", "informe..v2.pdf", "...x"} {
		data := testBytes(300 + i)
		att := r.upload(t, fileID(10+i), n, data)
		sendApp(t, r.c, bridge.TypeFileGet, "r_g", bridge.FileGet{ID: att.ID})
		g := readDownload(t, r.c, att.ID, 0)
		wantType(t, *g.end, "r_g", bridge.TypeFileEnd)
		if !bytes.Equal(g.data, data) {
			t.Fatalf("%s: other content", n)
		}
	}
	// A real ".." element is refused, whatever the registry says.
	r.l.chat.attachments.put(bridge.Attachment{ID: fileID(1), Size: 5}, "inbox/../x")
	sendApp(t, r.c, bridge.TypeFileGet, "r_x", bridge.FileGet{ID: fileID(1)})
	wantFileError(t, readApp(t, r.c), "r_x", bridge.CodeFailed)
}

// What is registered must be a regular file: a folder, and a link (Unix) that
// points elsewhere, are not served.
func TestDownloadNeverFollowsLinks(t *testing.T) {
	r := newFilesRig(t, nil)
	os.MkdirAll(filepath.Join(r.ws, "outbox", "dir"), 0o700)
	r.l.chat.attachments.put(bridge.Attachment{ID: fileID(1), Size: 5}, "outbox/dir")
	sendApp(t, r.c, bridge.TypeFileGet, "r_1", bridge.FileGet{ID: fileID(1)})
	wantFileError(t, readApp(t, r.c), "r_1", bridge.CodeFailed)

	target := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(target, []byte("secret"), 0o600)
	if err := os.Symlink(target, filepath.Join(r.ws, "outbox", "link")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	r.l.chat.attachments.put(bridge.Attachment{ID: fileID(2), Size: 6}, "outbox/link")
	sendApp(t, r.c, bridge.TypeFileGet, "r_2", bridge.FileGet{ID: fileID(2)})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeFailed)
}

// The thumbnail is made without upMu: the frames of the other uploads, which
// take it, are not held while a large image is decoded.
func TestThumbnailIsMadeWithoutTheMutex(t *testing.T) {
	r := newFilesRig(t, nil)
	locked := make(chan bool, 4)
	setVar(t, &thumbnailOf, func(path string) string {
		if r.l.files.upMu.TryLock() {
			r.l.files.upMu.Unlock()
			locked <- false
		} else {
			locked <- true
		}
		return thumbnail(path)
	})
	att := r.upload(t, fileID(1), "a.png", pngData(40, 40))
	if att.Thumb == "" {
		t.Fatal("no thumbnail")
	}
	if held := <-locked; held {
		t.Fatal("the thumbnail was made with the uploads' mutex held")
	}
	// And the retry of that upload still gets the attachment, thumb included.
	r.begin(t, "r_2", fileID(1), "a.png", pngData(40, 40))
	wantReady(t, readApp(t, r.c), "r_2", int64(att.Size))
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_3", bridge.AttachEnd{ID: fileID(1)})
	if again := wantDone(t, readApp(t, r.c), "r_3"); again != att {
		t.Fatalf("%+v, want %+v", again, att)
	}
}

// A resumption does not take frames until its hash covers what was there, and
// the hash is right: a resumed upload verifies.
func TestResumedUploadHashIsRightWithoutTheMutex(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(5000)
	id := fileID(1)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data[:3000], 0, 1000)
	r.begin(t, "r_2", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_2", 3000)
	if u := r.l.files.uploads[id]; u == nil || !u.ready {
		t.Fatalf("upload %+v", u)
	}
	sendData(t, r.c, id, data, 3000, 1000)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_3", bridge.AttachEnd{ID: id})
	wantDone(t, readApp(t, r.c), "r_3")
}

// A failed write is not "more data than declared": attach.end says failed, in
// words that fit.
func TestUploadWriteFailureIsFailed(t *testing.T) {
	r := newFilesRig(t, nil)
	data := testBytes(100)
	id := fileID(1)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	r.l.files.upMu.Lock()
	r.l.files.uploads[id].f.Close() // the next write fails
	r.l.files.upMu.Unlock()
	sendFrame(t, r.c, id, 0, data)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	er := wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeFailed)
	if strings.Contains(er.Text, "more data") || !strings.Contains(er.Text, "disk") {
		t.Fatalf("text %q", er.Text)
	}
	if p := r.partials(t); len(p) != 0 {
		t.Fatalf("left %v", p)
	}
}
