package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// Final review, I1: the working directory of adb pull and adb bugreport is
// the agent's, so the daemon checks where they write against the root of the
// agent's environment.

// deniedPC runs argv from cwd with the user saying no and returns the reason
// shown in the confirmation ("" if nobody was asked).
func deniedPC(t *testing.T, e *env, cwd string, argv ...string) string {
	t.Helper()
	e.conf.decision = confirm.Deny
	e.conf.asked = nil
	before := len(e.adb.ran())
	_, exit := e.execIn(t, cwd, argv...)
	if len(e.conf.asked) == 0 {
		if exit.Code != 0 || len(e.adb.ran()) != before+1 {
			t.Fatalf("%q from %s: exit %+v, runs %q", argv, cwd, exit, e.adb.ran())
		}
		return ""
	}
	if exit.Code != ExitDenied || len(e.adb.ran()) != before {
		t.Fatalf("%q from %s: denied but exit %+v, runs %q", argv, cwd, exit, e.adb.ran())
	}
	if j := e.last(t); j.Level != "red" {
		t.Fatalf("%q: journaled as %s", argv, j.Level)
	}
	return e.conf.asked[0].Detail
}

func TestPullFromOutsideTheEnvironmentIsRed(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	for _, argv := range [][]string{
		{"pull", "/sdcard/Download/x", "run.txt"},
		{"pull", "/sdcard/Download/authorized_keys"},
		{"bugreport"},
		{"bugreport", "report.zip"},
		{"wait-for-device", "pull", "/sdcard/a.jpg"},
	} {
		d := deniedPC(t, e, outside, argv...)
		if !strings.Contains(d, "writes outside the agent environment: "+outside) {
			t.Errorf("%q: detail %q", argv, d)
		}
	}
}

func TestPullInsideTheEnvironmentRunsAtOnce(t *testing.T) {
	e := newEnv(t)
	photos := filepath.Join(e.ws, "photos")
	if err := os.MkdirAll(photos, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cwd  string
		argv []string
	}{
		{e.ws, []string{"pull", "/sdcard/DCIM/a.jpg"}},
		{e.ws, []string{"pull", "/sdcard/DCIM/a.jpg", "photos"}},
		{e.ws, []string{"bugreport", "photos/bug.zip"}},
		{photos, []string{"pull", "/sdcard/notes.txt"}},
		{photos, []string{"bugreport"}},
	} {
		if d := deniedPC(t, e, c.cwd, c.argv...); d != "" {
			t.Errorf("%q from %s: asked (%q)", c.argv, c.cwd, d)
		}
	}
}

func TestPullOverTheEnvironmentsOwnFilesIsRed(t *testing.T) {
	e := newEnv(t)
	for _, dir := range []string{".claude", "journal", filepath.Join(".adbuddy", "bin")} {
		if err := os.MkdirAll(filepath.Join(e.ws, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		cwd  string
		argv []string
	}{
		{filepath.Join(e.ws, ".claude"), []string{"pull", "/sdcard/settings.local.txt"}},
		{filepath.Join(e.ws, ".adbuddy", "bin"), []string{"pull", "/sdcard/x", "tool"}},
		{filepath.Join(e.ws, "journal"), []string{"bugreport"}},
		{filepath.Join(e.ws, "journal"), []string{"pull", "/sdcard/2026-09.txt"}},
	} {
		if d := deniedPC(t, e, c.cwd, c.argv...); !strings.Contains(d, "own files") {
			t.Errorf("%q from %s: detail %q", c.argv, c.cwd, d)
		}
	}
}

// linkDir makes link point to the folder target: a symlink, or a junction on
// Windows without the privilege to create symlinks.
func linkDir(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err != nil && runtime.GOOS == "windows" {
		out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if jerr == nil {
			return
		}
		err = fmt.Errorf("%v; mklink /J: %v %s", err, jerr, out)
	}
	if err != nil {
		t.Skip("cannot create links here:", err)
	}
}

func TestPullThroughALinkOutOfTheEnvironmentIsRed(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	linkDir(t, outside, filepath.Join(e.ws, "out"))
	if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", "out"); !strings.Contains(d, "writes outside the agent environment") {
		t.Errorf("link to a folder: %q", d)
	}
	if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", "out/new/y"); !strings.Contains(d, "writes outside the agent environment") {
		t.Errorf("new path under the link: %q", d)
	}
	if d := deniedPC(t, e, filepath.Join(e.ws, "out"), "pull", "/sdcard/x"); !strings.Contains(d, "writes outside the agent environment") {
		t.Errorf("cwd through the link: %q", d)
	}
}

func TestPullOverADanglingLinkIsRed(t *testing.T) {
	e := newEnv(t)
	// adb would create the link's target, wherever it points.
	if err := os.Symlink(filepath.Join(t.TempDir(), "new"), filepath.Join(e.ws, "dangling.txt")); err != nil {
		t.Skip("cannot create symlinks here:", err)
	}
	if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", "dangling.txt"); d == "" {
		t.Error("dangling link: not asked")
	}
}

func TestPullWithoutAKnownEnvironmentIsRed(t *testing.T) {
	e := newEnv(t)
	e.d.o.Workspace = ""
	if d := deniedPC(t, e, e.ws, "pull", "/sdcard/a.jpg"); !strings.Contains(d, "environment is unknown") {
		t.Errorf("detail %q", d)
	}
}

func TestPullComparesPathsWithoutCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive paths are a Windows matter")
	}
	e := newEnv(t)
	if d := deniedPC(t, e, strings.ToUpper(e.ws), "pull", "/sdcard/a.jpg"); d != "" {
		t.Errorf("same folder in capitals: asked (%q)", d)
	}
}

func TestUserRuleDoesNotLetPullWriteOutside(t *testing.T) {
	e := newEnv(t)
	c, err := classify.New([]byte("rules:\n  - {id: u.pull, scope: adb, cmd: pull, level: green}\n  - {id: u.bug, scope: adb, cmd: bugreport, level: green}\n"))
	if err != nil {
		t.Fatal(err)
	}
	e.d.o.Classifier = c
	if d := deniedPC(t, e, t.TempDir(), "pull", "/sdcard/x"); !strings.Contains(d, "writes outside the agent environment") {
		t.Errorf("cwd outside: %q", d)
	}
	if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", "../x"); d == "" {
		t.Error("target outside: not asked")
	}
	if d := deniedPC(t, e, e.ws, "bugreport", "adb.bat"); d == "" {
		t.Error("program at the top: not asked")
	}
}
