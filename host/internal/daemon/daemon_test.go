package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// fakeADB records calls. Shell answers come from shell; Run exit codes from exits.
type fakeADB struct {
	mu    sync.Mutex
	shell map[string]string
	exits map[string]int
	out   map[string]string
	runs  []string
}

func (f *fakeADB) Shell(_ context.Context, cmd string) (string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, "precheck: "+cmd)
	if out, ok := f.shell[cmd]; ok {
		return out, 0, nil
	}
	return "", 1, nil
}

func (f *fakeADB) Pull(context.Context, string, string) error { return errors.New("no pull") }

func (f *fakeADB) Run(_ context.Context, _ string, args []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.Join(args, " ")
	f.runs = append(f.runs, key)
	if stdin != nil {
		in, _ := io.ReadAll(stdin)
		stdout.Write(in)
	}
	io.WriteString(stdout, f.out[key])
	return f.exits[key], nil
}

func (f *fakeADB) ran() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.runs...)
}

type fakeConfirmer struct {
	decision confirm.Decision
	asked    []confirm.Request
}

func (c *fakeConfirmer) Confirm(_ context.Context, r confirm.Request) confirm.Decision {
	c.asked = append(c.asked, r)
	return c.decision
}

type stream struct {
	stdin       io.Reader
	out, errOut bytes.Buffer
	data        [][]byte
}

func (s *stream) Stdin() io.Reader {
	if s.stdin == nil {
		return strings.NewReader("")
	}
	return s.stdin
}
func (s *stream) Stdout() io.Writer { return &s.out }
func (s *stream) Stderr() io.Writer { return &s.errOut }
func (s *stream) Data(v any) error {
	b, err := json.Marshal(v)
	s.data = append(s.data, b)
	return err
}

// settingsInFront is dumpsys with Settings, not a protected app, in front.
const settingsInFront = "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n"

type env struct {
	ws      string // the agent's environment
	d       *Daemon
	adb     *fakeADB
	conf    *fakeConfirmer
	store   *journal.Store
	entries []journal.Entry
}

func newEnv(t *testing.T) *env {
	t.Helper()
	c, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	e := &env{
		ws: ws,
		adb: &fakeADB{shell: map[string]string{
			// What the guard reads before input and screencap (M5 spec §4.3).
			"dumpsys activity activities": settingsInFront,
		}, exits: map[string]int{}, out: map[string]string{}},
		conf:  &fakeConfirmer{decision: confirm.Allow},
		store: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("test key")),
	}
	e.d = New(Options{Classifier: c, ADB: e.adb, Journal: e.store, Confirmer: e.conf, Serial: "dev1", Session: "s_1", Lang: "en",
		Workspace: ws, OnEntry: func(j journal.Entry) { e.entries = append(e.entries, j) }})
	return e
}

func (e *env) exec(t *testing.T, argv ...string) (*stream, ipc.Exit) {
	t.Helper()
	return e.execIn(t, t.TempDir(), argv...)
}

// execIn runs argv as the agent would from the working directory cwd.
func (e *env) execIn(t *testing.T, cwd string, argv ...string) (*stream, ipc.Exit) {
	t.Helper()
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: cwd, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return s, exit
}

func (e *env) last(t *testing.T) journal.Entry {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil || len(all) == 0 {
		t.Fatalf("journal: %v %d", err, len(all))
	}
	return all[len(all)-1]
}

func TestGreenRunsAndIsJournaled(t *testing.T) {
	e := newEnv(t)
	e.adb.out["shell getprop ro.product.model"] = "Pixel\n"
	s, exit := e.exec(t, "shell", "getprop", "ro.product.model")
	if exit.Code != 0 || s.out.String() != "Pixel\n" {
		t.Fatalf("exit %+v out %q err %q", exit, s.out.String(), s.errOut.String())
	}
	j := e.last(t)
	if j.Status != journal.StatusOK || j.Level != "green" || j.Source != journal.SourceAgent || j.Session != "s_1" || *j.ExitCode != 0 || j.Inverse != nil {
		t.Fatalf("entry %+v", j)
	}
	if len(e.conf.asked) != 0 || len(e.entries) != 1 {
		t.Fatalf("asked %d, live %d", len(e.conf.asked), len(e.entries))
	}
}

func TestYellowJournalsInverseBeforeRunning(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global window_animation_scale"] = "1.0\n"
	_, exit := e.exec(t, "shell", "settings", "put", "global", "window_animation_scale", "0.5")
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	runs := e.adb.ran()
	if len(runs) != 2 || runs[0] != "precheck: settings get global window_animation_scale" || runs[1] != "shell settings put global window_animation_scale 0.5" {
		t.Fatalf("runs %q", runs)
	}
	j := e.last(t)
	if j.Level != "yellow" || j.Status != journal.StatusOK || strings.Join(j.Inverse.Argv, " ") != "shell settings put global window_animation_scale 1.0" {
		t.Fatalf("entry %+v", j)
	}
	// The "running" record precedes the final one in the file.
	data, _ := os.ReadFile(filepath.Join(e.store.Dir(), time.Now().UTC().Format("2006-01")+".jsonl"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"status":"running"`) || !strings.Contains(lines[0], `"inverse":{"argv"`) {
		t.Fatalf("lines %q", lines)
	}
}

func TestRedDeniedExits126WithoutRunning(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	s, exit := e.exec(t, "shell", "rm", "-rf", "/sdcard/DCIM")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "Denied by user: Delete files: -rf /sdcard/DCIM") {
		t.Fatalf("exit %+v stderr %q", exit, s.errOut.String())
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("ran %q", e.adb.ran())
	}
	r := e.conf.asked[0]
	if r.Title != "Delete files: -rf /sdcard/DCIM" || r.RawCommand != "adb shell rm -rf /sdcard/DCIM" || time.Until(r.ExpiresAt) > 2*time.Minute {
		t.Fatalf("request %+v", r)
	}
	if j := e.last(t); j.Status != journal.StatusDenied || *j.ExitCode != ExitDenied {
		t.Fatalf("entry %+v", j)
	}
}

func TestRedTimeout(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Timeout
	s, exit := e.exec(t, "reboot")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "(confirmation expired)") {
		t.Fatalf("exit %+v stderr %q", exit, s.errOut.String())
	}
	if j := e.last(t); j.Status != journal.StatusTimeout {
		t.Fatalf("entry %+v", j)
	}
	if !e.conf.asked[0].Special {
		t.Fatal("reboot must be flagged special")
	}
}

func TestRedAllowedRuns(t *testing.T) {
	e := newEnv(t)
	e.adb.exits["shell rm /sdcard/x"] = 1
	_, exit := e.exec(t, "shell", "rm", "/sdcard/x")
	if exit.Code != 1 {
		t.Fatalf("exit %+v", exit)
	}
	if j := e.last(t); j.Status != journal.StatusError || j.Level != "red" || *j.ExitCode != 1 {
		t.Fatalf("entry %+v", j)
	}
}

func TestPrecheckEscalatesToRed(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	e.adb.shell["pm list packages -s com.user"] = ""
	_, exit := e.exec(t, "shell", "pm", "uninstall", "--user", "0", "com.user")
	if exit.Code != ExitDenied || len(e.conf.asked) != 1 || !strings.Contains(e.conf.asked[0].Detail, "not a system app") {
		t.Fatalf("exit %+v asked %+v", exit, e.conf.asked)
	}
}

func TestJournalUnwritableBlocksChangesButNotReads(t *testing.T) {
	e := newEnv(t)
	// Make the journal directory a file.
	if err := os.MkdirAll(filepath.Dir(e.store.Dir()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.store.Dir(), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	e.d.o.OnWarning = func(w string) { warnings = append(warnings, w) }
	e.adb.shell["settings get global k"] = "1\n"
	for _, argv := range [][]string{{"shell", "settings", "put", "global", "k", "2"}, {"shell", "rm", "/sdcard/x"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "Blocked by ADBuddy: the journal cannot be written") {
			t.Fatalf("%q: exit %+v stderr %q", argv, exit, s.errOut.String())
		}
	}
	if len(e.adb.ran()) != 0 || len(e.conf.asked) != 0 {
		t.Fatalf("ran %q asked %d", e.adb.ran(), len(e.conf.asked))
	}
	_, exit := e.exec(t, "shell", "getprop")
	if exit.Code != 0 || len(e.adb.ran()) != 1 || len(warnings) == 0 {
		t.Fatalf("green: exit %+v runs %q warnings %q", exit, e.adb.ran(), warnings)
	}
}

func TestForbiddenTargetsAreBlocked(t *testing.T) {
	e := newEnv(t)
	for _, argv := range [][]string{{"-s", "other", "shell", "ls"}, {"-P", "5037", "devices"}, {"-d", "shell", "ls"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "Blocked by ADBuddy") {
			t.Fatalf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "ls"}, Env: map[string]string{"ANDROID_SERIAL": "other"}}, s)
	if exit.Code != ExitDenied {
		t.Fatalf("ANDROID_SERIAL: %+v", exit)
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("ran %q", e.adb.ran())
	}
	if _, exit := e.exec(t, "-s", "dev1", "shell", "ls"); exit.Code != 0 {
		t.Fatalf("own serial: %+v", exit)
	}
}

func TestStdinIsForwarded(t *testing.T) {
	e := newEnv(t)
	s := &stream{stdin: strings.NewReader("data")}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "cat"}}, s)
	if exit.Code != 0 || s.out.String() != "data" {
		t.Fatalf("%+v %q", exit, s.out.String())
	}
}

func TestInstallInverseAfterRun(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["pm list packages"] = "package:com.a\n"
	var once sync.Once
	e.d.o.ADB = &installADB{fakeADB: e.adb, after: func() { once.Do(func() { e.adb.shell["pm list packages"] = "package:com.a\npackage:com.new\n" }) }}
	_, exit := e.exec(t, "install", "app.apk")
	if exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	if j := e.last(t); j.Inverse == nil || strings.Join(j.Inverse.Argv, " ") != "uninstall com.new" {
		t.Fatalf("entry %+v", j)
	}
}

// installADB simulates the package list changing when install runs.
type installADB struct {
	*fakeADB
	after func()
}

func (a *installADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	code, err := a.fakeADB.Run(ctx, dir, args, stdin, stdout, stderr)
	a.after()
	return code, err
}

func TestPing(t *testing.T) {
	e := newEnv(t)
	s := &stream{}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindPing}, s); exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("%+v", exit)
	}
	var p ipc.Ping
	json.Unmarshal(s.data[0], &p)
	if p.Serial != "dev1" || p.Session != "s_1" {
		t.Fatalf("%+v", p)
	}
}

func TestConfirmationDetailAndRawCommandAreEscaped(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	e.exec(t, "shell", "rm -rf /sdcard/DCIM; echo x > '/sdcard/a\x1b[3A\x1b[2K   Read the battery level\x1b[B'")
	r := e.conf.asked[0]
	for _, field := range []string{r.Title, r.RawCommand} {
		if strings.ContainsRune(field, 0x1b) {
			t.Fatalf("unescaped: %q", field)
		}
	}
	for _, c := range r.Detail {
		if c < 0x20 && c != '\n' {
			t.Fatalf("detail keeps control characters: %q", r.Detail)
		}
	}
	e.exec(t, "exec-out", "sh", "-c", "rm -rf /sdcard/DCIM")
	if got := e.conf.asked[1].RawCommand; got != "adb exec-out sh -c 'rm -rf /sdcard/DCIM'" {
		t.Fatalf("raw command %q", got)
	}
}

func TestAllowedButAbandonedDoesNotRun(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &stream{}
	exit := e.d.Handle(ctx, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "rm", "/sdcard/x"}}, s)
	if exit.Code != ExitDenied || len(e.adb.ran()) != 0 {
		t.Fatalf("%+v ran %q", exit, e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusDenied || j.Reason != "abandoned by the agent before it ran" {
		t.Fatalf("%+v", j)
	}
}

// T5 re-review, M2: when serve stops (Ctrl+C), the journal does not blame
// the agent.
func TestAllowedButServeStoppedDoesNotRun(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ipc.ErrClosed)
	exit := e.d.Handle(ctx, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "rm", "/sdcard/x"}}, &stream{})
	if exit.Code != ExitDenied || len(e.adb.ran()) != 0 {
		t.Fatalf("%+v ran %q", exit, e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusDenied || j.Reason != "abandoned: ADBuddy stopped before it ran" {
		t.Fatalf("%+v", j)
	}
}

// slowADB counts how many Run calls overlap.
type slowADB struct {
	*fakeADB
	mu        sync.Mutex
	now, peak int
}

func (a *slowADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	a.mu.Lock()
	a.now++
	if a.now > a.peak {
		a.peak = a.now
	}
	a.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	a.mu.Lock()
	a.now--
	a.mu.Unlock()
	return a.fakeADB.Run(ctx, dir, args, stdin, stdout, stderr)
}

func TestChangesAreSerialised(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.adb.shell["settings get global b"] = "1\n"
	slow := &slowADB{fakeADB: e.adb}
	e.d.o.ADB = slow
	var wg sync.WaitGroup
	for _, k := range []string{"a", "b"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "settings", "put", "global", k, "2"}}, &stream{})
		}(k)
	}
	wg.Wait()
	if slow.peak != 1 {
		t.Fatalf("%d changes ran at once", slow.peak)
	}
}

// failingADB cannot start adb, or is killed by a signal.
type failingADB struct {
	*fakeADB
	code int
	err  error
}

func (a *failingADB) Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
	return a.code, a.err
}

func TestAdbFailures(t *testing.T) {
	e := newEnv(t)
	e.d.o.ADB = &failingADB{fakeADB: e.adb, code: -1, err: errors.New("exec: adb not found")}
	s, exit := e.exec(t, "shell", "getprop")
	if exit.Code != 1 || !strings.Contains(s.errOut.String(), "could not run adb") || e.last(t).Status != journal.StatusError {
		t.Fatalf("start failure: %+v %q", exit, s.errOut.String())
	}
	e.d.o.ADB = &failingADB{fakeADB: e.adb, code: -1}
	if _, exit := e.exec(t, "shell", "getprop"); exit.Code != 1 {
		t.Fatalf("signal: %+v", exit)
	}
}

// Nobody could be asked (serve --confirm deny, or no terminal): the journal
// and the shim say so instead of "denied by user".
func TestRedDeniedWhenNobodyCanConfirm(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Nobody
	s, exit := e.exec(t, "reboot")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "Denied: nobody could confirm: Restart the phone") || strings.Contains(s.errOut.String(), "by user") {
		t.Fatalf("exit %+v stderr %q", exit, s.errOut.String())
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("ran %q", e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusDenied || j.Reason != "denied: nobody to confirm (serve --confirm deny or no terminal)" || *j.ExitCode != ExitDenied {
		t.Fatalf("entry %+v", j)
	}
}

// The agent keeps the ANDROID_SERIAL it started with: after the phone changes
// address, the serials used earlier in the session are still its phone.
func TestSerialsUsedBeforeAreAccepted(t *testing.T) {
	e := newEnv(t)
	e.d.o.Serial = "old"
	e.d.o.Serials = func() []string { return []string{"old", "new"} }
	for _, serial := range []string{"old", "new"} {
		s := &stream{}
		exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "ls"}, Env: map[string]string{"ANDROID_SERIAL": serial}}, s)
		if exit.Code != 0 {
			t.Fatalf("%s: %+v %q", serial, exit, s.errOut.String())
		}
	}
	if _, exit := e.exec(t, "-s", "new", "shell", "ls"); exit.Code != 0 {
		t.Fatalf("-s new: %+v", exit)
	}
	ran := len(e.adb.ran())
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "ls"}, Env: map[string]string{"ANDROID_SERIAL": "other"}}, s)
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "only the active phone can be used") {
		t.Fatalf("other: %+v %q", exit, s.errOut.String())
	}
	if _, exit := e.exec(t, "-s", "other", "shell", "ls"); exit.Code != ExitDenied {
		t.Fatalf("-s other: %+v", exit)
	}
	if len(e.adb.ran()) != ran {
		t.Fatalf("a blocked command reached adb: %q", e.adb.ran())
	}
}

func TestPingReportsCurrentSerial(t *testing.T) {
	e := newEnv(t)
	e.d.o.Serial = "old"
	e.d.o.CurrentSerial = func() string { return "new" }
	s := &stream{}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindPing}, s); exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("%+v", exit)
	}
	var p ipc.Ping
	json.Unmarshal(s.data[0], &p)
	if p.Serial != "new" {
		t.Fatalf("%+v", p)
	}
}
