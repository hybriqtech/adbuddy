package precheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// fakeDevice answers Shell calls from a table keyed by the exact command.
type fakeDevice struct {
	answers map[string]string // command -> stdout (exit 0)
	fail    map[string]bool   // command -> exit 1
	calls   []string
	pulled  map[string]string // remote -> content written to local
}

func (f *fakeDevice) Shell(_ context.Context, cmd string) (string, int, error) {
	f.calls = append(f.calls, cmd)
	if f.fail[cmd] {
		return "", 1, nil
	}
	out, ok := f.answers[cmd]
	if !ok {
		return "", 0, errors.New("unexpected command: " + cmd)
	}
	return out, 0, nil
}

func (f *fakeDevice) Pull(_ context.Context, remote, local string) error {
	content, ok := f.pulled[remote]
	if !ok {
		return errors.New("no such file")
	}
	return os.WriteFile(local, []byte(content), 0o600)
}

func kindCmd(p string) string {
	q := p
	if strings.ContainsAny(p, " $'\\") {
		q = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return "if [ -d " + q + " ]; then echo dir; elif [ -e " + q + " ] || [ -L " + q + " ]; then echo file; else echo none; fi"
}

func op(kind string, kv ...string) *classify.Op {
	a := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		a[kv[i]] = kv[i+1]
	}
	return &classify.Op{Kind: kind, Args: a}
}

func joined(argv []string) string { return strings.Join(argv, " ") }

func TestSettingsPutRestoresPreviousValueQuoted(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{
		"settings get global k": "a b; $(reboot)\n",
	}}
	p := Prepare(context.Background(), dev, op("settings-put", "ns", "global", "key", "k", "value", "1"), classify.Yellow, Env{})
	if p.Level != classify.Yellow {
		t.Fatalf("level %v: %s", p.Level, p.Reason)
	}
	if got := joined(p.Inverse); got != "shell settings put global k 'a b; $(reboot)'" {
		t.Fatalf("inverse %q", got)
	}
}

func TestSettingsPutMissingValueDeletes(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{"settings --user 0 get secure k": "null\n"}}
	p := Prepare(context.Background(), dev, op("settings-put", "ns", "secure", "key", "k", "value", "1", "user", "0"), classify.Yellow, Env{})
	if got := joined(p.Inverse); got != "shell settings --user 0 delete secure k" {
		t.Fatalf("inverse %q", got)
	}
}

func TestPhoneErrorRaisesToRed(t *testing.T) {
	dev := &fakeDevice{fail: map[string]bool{"settings get global k": true}}
	p := Prepare(context.Background(), dev, op("settings-put", "ns", "global", "key", "k", "value", "1"), classify.Yellow, Env{})
	if p.Level != classify.Red || p.Inverse != nil || !strings.Contains(p.Reason, "could not check") {
		t.Fatalf("got %+v", p)
	}
}

func TestPmDisable(t *testing.T) {
	const listCmd = "pm list packages -d --user 0 com.facebook.katana"
	dev := &fakeDevice{answers: map[string]string{listCmd: ""}}
	p := Prepare(context.Background(), dev, op("pm-disable", "pkg", "com.facebook.katana", "user", "0"), classify.Yellow, Env{})
	if got := joined(p.Inverse); got != "shell pm enable --user 0 com.facebook.katana" {
		t.Fatalf("inverse %q", got)
	}
	dev.answers[listCmd] = "package:com.facebook.katana\n"
	p = Prepare(context.Background(), dev, op("pm-disable", "pkg", "com.facebook.katana", "user", "0"), classify.Yellow, Env{})
	if p.Level != classify.Yellow || p.Inverse != nil {
		t.Fatalf("already disabled: %+v", p)
	}
}

func TestPmUninstallUser(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{
		"pm list packages -s com.sys":  "package:com.sys\n",
		"pm list packages -s com.user": "",
	}}
	p := Prepare(context.Background(), dev, op("pm-uninstall-user", "pkg", "com.sys", "user", "0"), classify.Yellow, Env{})
	if p.Level != classify.Yellow || joined(p.Inverse) != "shell cmd package install-existing --user 0 com.sys" {
		t.Fatalf("system app: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("pm-uninstall-user", "pkg", "com.user", "user", "0"), classify.Yellow, Env{})
	if p.Level != classify.Red || p.Inverse != nil {
		t.Fatalf("user app: %+v", p)
	}
}

func TestAppopsSet(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{
		"appops get com.x RUN_IN_BACKGROUND": "Uid mode: RUN_IN_BACKGROUND: allow\nRUN_IN_BACKGROUND: ignore; time=+1d\n",
		"appops get com.y RUN_IN_BACKGROUND": "No operations.\n",
		"appops get com.z RUN_IN_BACKGROUND": "garbage\n",
	}}
	p := Prepare(context.Background(), dev, op("appops-set", "pkg", "com.x", "op", "RUN_IN_BACKGROUND", "mode", "deny"), classify.Yellow, Env{})
	if joined(p.Inverse) != "shell appops set com.x RUN_IN_BACKGROUND ignore" {
		t.Fatalf("x: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("appops-set", "pkg", "com.y", "op", "RUN_IN_BACKGROUND", "mode", "deny"), classify.Yellow, Env{})
	if joined(p.Inverse) != "shell appops set com.y RUN_IN_BACKGROUND default" {
		t.Fatalf("y: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("appops-set", "pkg", "com.z", "op", "RUN_IN_BACKGROUND", "mode", "deny"), classify.Yellow, Env{})
	if p.Level != classify.Red {
		t.Fatalf("z: %+v", p)
	}
}

func TestDeviceidle(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{"dumpsys deviceidle whitelist": "system-excidle,com.a,1000\nuser,com.b,10123\n"}}
	p := Prepare(context.Background(), dev, op("deviceidle", "sign", "+", "pkg", "com.c"), classify.Yellow, Env{})
	if joined(p.Inverse) != "shell dumpsys deviceidle whitelist -com.c" {
		t.Fatalf("add: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("deviceidle", "sign", "+", "pkg", "com.b"), classify.Yellow, Env{})
	if p.Inverse != nil {
		t.Fatalf("already there: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("deviceidle", "sign", "-", "pkg", "com.b"), classify.Yellow, Env{})
	if joined(p.Inverse) != "shell dumpsys deviceidle whitelist +com.b" {
		t.Fatalf("remove: %+v", p)
	}
}

func TestDeviceidleOnlyUserList(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{"dumpsys deviceidle whitelist": "system,com.s,1000\n"}}
	p := Prepare(context.Background(), dev, op("deviceidle", "sign", "+", "pkg", "com.s"), classify.Yellow, Env{})
	if joined(p.Inverse) != "shell dumpsys deviceidle whitelist -com.s" {
		t.Fatalf("add: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("deviceidle", "sign", "-", "pkg", "com.s"), classify.Yellow, Env{})
	if p.Inverse != nil || p.Level != classify.Yellow {
		t.Fatalf("remove: %+v", p)
	}
	dev.answers["dumpsys deviceidle whitelist"] = "garbage\n"
	p = Prepare(context.Background(), dev, op("deviceidle", "sign", "+", "pkg", "com.s"), classify.Yellow, Env{})
	if p.Level != classify.Red {
		t.Fatalf("garbage: %+v", p)
	}
}

func TestMv(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{
		kindCmd("/sdcard/new name"): "none\n",
		kindCmd("/sdcard/taken"):    "file\n",
	}}
	p := Prepare(context.Background(), dev, op("mv", "src", "/sdcard/a", "dst", "/sdcard/new name"), classify.Yellow, Env{})
	if p.Level != classify.Yellow || joined(p.Inverse) != "shell mv '/sdcard/new name' /sdcard/a" {
		t.Fatalf("free: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("mv", "src", "/sdcard/a", "dst", "/sdcard/taken"), classify.Yellow, Env{})
	if p.Level != classify.Red || p.Inverse != nil {
		t.Fatalf("taken: %+v", p)
	}
}

func TestMkdirAndNewFile(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{
		kindCmd("/sdcard/New"):   "none\n",
		kindCmd("/sdcard/Old"):   "dir\n",
		kindCmd("/sdcard/s.png"): "file\n",
		kindCmd("/sdcard/t.png"): "none\n",
	}}
	if p := Prepare(context.Background(), dev, op("mkdir", "path", "/sdcard/New"), classify.Yellow, Env{}); joined(p.Inverse) != "shell rmdir /sdcard/New" {
		t.Fatalf("mkdir new: %+v", p)
	}
	if p := Prepare(context.Background(), dev, op("mkdir", "path", "/sdcard/Old"), classify.Yellow, Env{}); p.Inverse != nil || p.Level != classify.Yellow {
		t.Fatalf("mkdir existing: %+v", p)
	}
	if p := Prepare(context.Background(), dev, op("new-file", "path", "/sdcard/s.png"), classify.Yellow, Env{}); p.Level != classify.Red {
		t.Fatalf("overwrite: %+v", p)
	}
	if p := Prepare(context.Background(), dev, op("new-file", "path", "/sdcard/t.png"), classify.Yellow, Env{}); joined(p.Inverse) != "shell rm -f /sdcard/t.png" {
		t.Fatalf("new file: %+v", p)
	}
}

func TestInstallInverseComputedAfter(t *testing.T) {
	dev := &fakeDevice{answers: map[string]string{"pm list packages": "package:com.a\npackage:com.b\n"}}
	p := Prepare(context.Background(), dev, op("adb-install", "apk", "x.apk"), classify.Yellow, Env{})
	if p.After == nil {
		t.Fatal("no After")
	}
	dev.answers["pm list packages"] = "package:com.a\npackage:com.b\npackage:com.new\n"
	inv, err := p.After(context.Background())
	if err != nil || joined(inv) != "uninstall com.new" {
		t.Fatalf("after: %q %v", inv, err)
	}
	dev.answers["pm list packages"] = "package:com.a\npackage:com.b\n"
	if inv, _ := p.After(context.Background()); inv != nil {
		t.Fatalf("update must have no inverse: %q", inv)
	}
}

func TestPush(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "notes.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cwd, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(t.TempDir(), "backups", "j_1")
	dev := &fakeDevice{
		answers: map[string]string{
			kindCmd("/sdcard/Download"):           "dir\n",
			kindCmd("/sdcard/Download/notes.txt"): "file\n",
			kindCmd("/sdcard/fresh.txt"):          "none\n",
			kindCmd("/sdcard/missing/"):           "none\n",
		},
		pulled: map[string]string{"/sdcard/Download/notes.txt": "old"},
	}
	env := Env{Cwd: cwd, BackupDir: backups}

	p := Prepare(context.Background(), dev, op("adb-push", "local", "notes.txt", "remote", "/sdcard/Download"), classify.Yellow, env)
	wantBackup := filepath.Join(backups, "original")
	if p.Level != classify.Yellow || joined(p.Inverse) != "push "+wantBackup+" /sdcard/Download/notes.txt" {
		t.Fatalf("overwrite: %+v", p)
	}
	if b, _ := os.ReadFile(wantBackup); string(b) != "old" {
		t.Fatalf("backup content %q", b)
	}

	p = Prepare(context.Background(), dev, op("adb-push", "local", "notes.txt", "remote", "/sdcard/fresh.txt"), classify.Yellow, env)
	if joined(p.Inverse) != "shell rm -f /sdcard/fresh.txt" {
		t.Fatalf("new: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("adb-push", "local", "folder", "remote", "/sdcard/fresh.txt"), classify.Yellow, env)
	if p.Level != classify.Red {
		t.Fatalf("folder: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("adb-push", "local", "notes.txt", "remote", "/sdcard/missing/"), classify.Yellow, env)
	if p.Level != classify.Red {
		t.Fatalf("missing dir: %+v", p)
	}
	p = Prepare(context.Background(), dev, op("adb-push", "local", "nope.txt", "remote", "/sdcard/fresh.txt"), classify.Yellow, env)
	if p.Level != classify.Red {
		t.Fatalf("missing local: %+v", p)
	}

	evil := `/data/local/tmp/..\..\2026-09.jsonl`
	dev.answers[kindCmd(evil)] = "file\n"
	dev.pulled[evil] = "journal"
	p = Prepare(context.Background(), dev, op("adb-push", "local", "notes.txt", "remote", evil), classify.Yellow, env)
	if p.Level != classify.Yellow || len(p.Backups) != 1 || p.Backups[0] != wantBackup ||
		joined(p.Inverse) != "push "+wantBackup+" "+evil {
		t.Fatalf("hostile name: %+v", p)
	}
}

func TestNoOp(t *testing.T) {
	p := Prepare(context.Background(), &fakeDevice{}, nil, classify.Green, Env{})
	if p.Level != classify.Green || p.Inverse != nil {
		t.Fatalf("%+v", p)
	}
}
