package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

func sendReq(path, caption string) ipc.Request {
	return ipc.Request{Kind: ipc.KindSend, Argv: []string{path, caption}, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}
}

// bareLink is a chat whose app never connects: messages are only kept.
func bareLink(t *testing.T) (*phoneLink, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app := &App{Stderr: &syncBuffer{}, Stdout: io.Discard, Getenv: func(string) string { return "" }}
	l, err := startPhoneLink(ctx, app, linkEnv(t), &appPhone{}, "s_1", "claude", &recording{})
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	l.useWorkspace(ws)
	return l, ws
}

func writeIn(t *testing.T, dir, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func wantSendError(t *testing.T, e ipc.Exit, code, contains string) {
	t.Helper()
	if e.Code == 0 || e.ErrorCode != code || !strings.Contains(e.Error, contains) {
		t.Fatalf("exit %+v, want error %s containing %q", e, code, contains)
	}
}

func outboxNames(t *testing.T, ws string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(ws, "outbox"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// A file of the environment is copied to outbox/, registered, kept in the
// history and published to the app without a rid; the app downloads it with
// file.get and it comes back in the snapshot.
func TestSendReachesTheApp(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	setVar(t, &filesNow, func() time.Time { return now })
	r := newFilesRig(t, nil)
	data := testBytes(3 * 1000)
	p := writeIn(t, r.ws, "work/informe.txt", data)

	msg, e := r.l.sendFile(r.ws, sendReq(p, "aqui va"))
	if e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	if !strings.Contains(msg, "informe.txt") || strings.Contains(msg, "not connected") {
		t.Fatalf("message %q", msg)
	}
	if got := outboxNames(t, r.ws); len(got) != 1 || got[0] != "20260102-030405-informe.txt" {
		t.Fatalf("outbox %v", got)
	}
	m := readApp(t, r.c)
	wantType(t, m, "", bridge.TypeChatMessage)
	cm := chatMessageOf(t, m)
	if cm.Role != "assistant" || cm.Text != "aqui va" || len(cm.Attachments) != 1 {
		t.Fatalf("%+v", cm)
	}
	a := cm.Attachments[0]
	sum := sha256.Sum256(data)
	if a.Name != "informe.txt" || a.Size != int64(len(data)) || a.SHA256 != hex.EncodeToString(sum[:]) || a.Thumb != "" {
		t.Fatalf("%+v", a)
	}

	sendApp(t, r.c, bridge.TypeFileGet, "r_g", bridge.FileGet{ID: a.ID})
	g := readDownload(t, r.c, a.ID, 0)
	if !bytes.Equal(g.data, data) {
		t.Fatalf("downloaded %d bytes", len(g.data))
	}
	s := r.l.snapshot()
	if len(s.Chat) != 1 || s.Chat[0].Attachments[0].ID != a.ID || s.Chat[0].Text != "aqui va" {
		t.Fatalf("snapshot %+v", s.Chat)
	}
}

func TestSendImageHasAThumbnail(t *testing.T) {
	r := newFilesRig(t, nil)
	p := writeIn(t, r.ws, "grafica.png", pngData(64, 48))
	if _, e := r.l.sendFile(r.ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	a := chatMessageOf(t, readApp(t, r.c)).Attachments[0]
	if a.Thumb == "" || a.MIME != "image/png" {
		t.Fatalf("%+v", a)
	}
	// The snapshot drops it.
	if s := r.l.snapshot(); s.Chat[0].Attachments[0].Thumb != "" {
		t.Fatal("thumb in the snapshot")
	}
}

// Without an app the file is copied and registered all the same, and the
// answer says it will arrive later.
func TestSendWithoutAnApp(t *testing.T) {
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("hola"))
	msg, e := l.sendFile(ws, sendReq(p, "x"))
	if e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	if !strings.Contains(msg, "not connected") {
		t.Fatalf("message %q", msg)
	}
	s := l.snapshot()
	if len(s.Chat) != 1 || len(s.Chat[0].Attachments) != 1 {
		t.Fatalf("snapshot %+v", s.Chat)
	}
	if _, ok := l.chat.attachments.get(s.Chat[0].Attachments[0].ID); !ok {
		t.Fatal("not registered")
	}
}

func TestSendTwiceTheSameNameInTheSameSecond(t *testing.T) {
	setVar(t, &filesNow, func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local) })
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("1"))
	for range 2 {
		if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
			t.Fatalf("%+v", e)
		}
	}
	got := outboxNames(t, ws)
	if len(got) != 2 || (got[0] != "20260102-030405-a-2.txt" && got[1] != "20260102-030405-a-2.txt") {
		t.Fatalf("outbox %v", got)
	}
}

func TestSendOutsideTheEnvironment(t *testing.T) {
	l, ws := bareLink(t)
	p := writeIn(t, t.TempDir(), "secreto.txt", []byte("x"))
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "OUTSIDE_WORKSPACE", "outside")
	if len(outboxNames(t, ws)) != 0 || len(l.snapshot().Chat) != 0 {
		t.Fatal("something was sent")
	}
	// ".." that leaves it.
	_, e = l.sendFile(ws, sendReq(filepath.Join(ws, "..", filepath.Base(filepath.Dir(p)), "secreto.txt"), ""))
	wantSendError(t, e, "OUTSIDE_WORKSPACE", "outside")
}

func TestSendSymlinkThatLeaves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows: the junction test covers it")
	}
	l, ws := bareLink(t)
	target := writeIn(t, t.TempDir(), "secreto.txt", []byte("x"))
	link := filepath.Join(ws, "enlace.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, e := l.sendFile(ws, sendReq(link, ""))
	wantSendError(t, e, "OUTSIDE_WORKSPACE", "outside")
	// A folder that is a link out.
	dirLink := filepath.Join(ws, "carpeta")
	if err := os.Symlink(filepath.Dir(target), dirLink); err != nil {
		t.Fatal(err)
	}
	_, e = l.sendFile(ws, sendReq(filepath.Join(dirLink, "secreto.txt"), ""))
	wantSendError(t, e, "OUTSIDE_WORKSPACE", "outside")
}

func TestSendJunctionThatLeaves(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows")
	}
	l, ws := bareLink(t)
	outside := t.TempDir()
	writeIn(t, outside, "secreto.txt", []byte("x"))
	junction := filepath.Join(ws, "salida")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	_, e := l.sendFile(ws, sendReq(filepath.Join(junction, "secreto.txt"), ""))
	wantSendError(t, e, "OUTSIDE_WORKSPACE", "outside")
	if len(outboxNames(t, ws)) != 0 {
		t.Fatal("something was copied")
	}
}

// Windows compares the environment's path without case.
func TestSendWithAnotherCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case is only ignored on Windows")
	}
	l, ws := bareLink(t)
	p := writeIn(t, ws, "Sub/Nota.txt", []byte("x"))
	if _, e := l.sendFile(ws, sendReq(strings.ToUpper(p), "")); e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	if len(l.snapshot().Chat) != 1 {
		t.Fatal("not sent")
	}
}

func TestSendTooLarge(t *testing.T) {
	l, ws := bareLink(t)
	p := filepath.Join(ws, "grande.bin")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSendSize + 1); err != nil {
		t.Skipf("cannot make a big file: %v", err)
	}
	f.Close()
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "TOO_LARGE", "too large")
	if len(outboxNames(t, ws)) != 0 {
		t.Fatal("something was copied")
	}
}

// The copy stops at the limit even if the file grew after it was measured.
func TestSendCopyStopsAtTheLimit(t *testing.T) {
	setVar(t, &maxSendSize, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 11))
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "TOO_LARGE", "too large")
	setVar(t, &statSize, func(os.FileInfo) int64 { return 0 }) // measured small, it is not
	_, e = l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "TOO_LARGE", "too large")
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("left %v", got)
	}
}

func TestSendNotAFile(t *testing.T) {
	l, ws := bareLink(t)
	dir := filepath.Join(ws, "carpeta")
	os.MkdirAll(dir, 0o700)
	_, e := l.sendFile(ws, sendReq(dir, ""))
	wantSendError(t, e, "NOT_A_FILE", "not a regular file")
	_, e = l.sendFile(ws, sendReq(ws, ""))
	wantSendError(t, e, "NOT_A_FILE", "not a regular file")
}

func TestSendDoesNotExist(t *testing.T) {
	l, ws := bareLink(t)
	_, e := l.sendFile(ws, sendReq(filepath.Join(ws, "nada.txt"), ""))
	wantSendError(t, e, "NOT_FOUND", "does not exist")
	_, e = l.sendFile(ws, sendReq(filepath.Join(ws, "no", "hay", "nada.txt"), ""))
	wantSendError(t, e, "NOT_FOUND", "does not exist")
}

func TestSendArgv(t *testing.T) {
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("x"))
	for _, argv := range [][]string{nil, {p}, {p, "", "extra"}} {
		_, e := l.sendFile(ws, ipc.Request{Kind: ipc.KindSend, Argv: argv})
		if e.Code != 2 || e.ErrorCode != "USAGE" {
			t.Errorf("%v: %+v", argv, e)
		}
	}
	// A relative path is not trusted: the client sends an absolute one.
	_, e := l.sendFile(ws, sendReq("a.txt", ""))
	wantSendError(t, e, "USAGE", "absolute")
	// The caption: 64 KiB are fine, one byte more is not.
	if _, e := l.sendFile(ws, sendReq(p, strings.Repeat("a", maxSendCaption))); e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	_, e = l.sendFile(ws, sendReq(p, strings.Repeat("a", maxSendCaption+1)))
	wantSendError(t, e, "TOO_LARGE", "caption")
}

func TestSendWithoutAnEnvironment(t *testing.T) {
	l := &phoneLink{}
	_, e := l.sendFile("", sendReq(filepath.Join(t.TempDir(), "a"), ""))
	if e.Code == 0 {
		t.Fatal("sent")
	}
}

// outbox cannot be a link: the copy would be written wherever it points.
func TestSendOutboxThatIsALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	l, ws := bareLink(t)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(ws, "outbox")); err != nil {
		t.Fatal(err)
	}
	p := writeIn(t, ws, "a.txt", []byte("x"))
	_, e := l.sendFile(ws, sendReq(p, ""))
	if e.Code == 0 {
		t.Fatal("sent")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote outside: %v", entries)
	}
}

func TestSendNamesAreSafe(t *testing.T) {
	l, ws := bareLink(t)
	name := strings.Repeat("ñ", 80) + ".txt"
	p := writeIn(t, ws, name, []byte("x"))
	if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	a := l.snapshot().Chat[0].Attachments[0]
	if len(a.Name) > maxNameBytes || !strings.HasSuffix(a.Name, ".txt") {
		t.Fatalf("name %q", a.Name)
	}
}

// handleSend answers the agent in its terminal: stdout for the result.
func TestHandleSendThroughIPC(t *testing.T) {
	dir, err := os.MkdirTemp("", "snd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("x"))
	secret, err := ipc.EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(dir, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		return l.handleSend(ctx, ws, req, c)
	})
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	defer srv.Close()
	var out, errb bytes.Buffer
	exit, err := ipc.Call(context.Background(), dir, sendReq(p, "hi"), nil, &out, &errb, nil)
	if err != nil || exit.Code != 0 || !strings.Contains(out.String(), "a.txt") {
		t.Fatalf("%v %+v out %q err %q", err, exit, out.String(), errb.String())
	}
	exit, err = ipc.Call(context.Background(), dir, sendReq(filepath.Join(ws, "nope"), ""), nil, &out, &errb, nil)
	if err != nil || exit.Code == 0 || !strings.Contains(errb.String(), "does not exist") {
		t.Fatalf("%v %+v err %q", err, exit, errb.String())
	}
}

func TestSendCommandChecksTheCaptionBeforeCalling(t *testing.T) {
	app := &App{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Getenv: func(string) string { return "agent" }}
	cmd := newSendCmd(app)
	cmd.SetArgs([]string{"a.txt", "--caption", strings.Repeat("a", maxSendCaption+1)})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "caption") {
		t.Fatalf("%v", err)
	}
}
