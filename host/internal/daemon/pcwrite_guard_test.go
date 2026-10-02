package daemon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// V09: the daemon's check of adb pull and adbuddy send share one guard
// (pathguard). The daemon keeps no copy of it that could drift: no function of
// the package resolves links, compares roots or looks for reparse points on
// its own, and pcwrite.go imports pathguard.
func TestPCWriteUsesPathguard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	copies := map[string]bool{"resolveLinks": true, "within": true, "reparseBelow": true}
	imports := false
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && copies[fn.Name.Name] {
				t.Errorf("%s declares %s: use pathguard instead", name, fn.Name.Name)
			}
		}
		if name != "pcwrite.go" {
			continue
		}
		for _, im := range f.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == "github.com/hybriqtech/adbuddy/host/internal/pathguard" {
				imports = true
			}
		}
	}
	if !imports {
		t.Error("pcwrite.go does not use pathguard")
	}
}

// V09: a junction inside the environment (Windows: filepath.EvalSymlinks does
// not follow it) is refused and named, as a folder, as a file under it, as a
// path under it that does not exist yet and as the working directory.
func TestPullThroughAJunctionSaysSo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows matter")
	}
	e := newEnv(t)
	outside := t.TempDir()
	j := filepath.Join(e.ws, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", j, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	const why = "through a junction or another link ADBuddy does not follow"
	for _, target := range []string{"j", "j/x.txt", "j/new/y.txt", "J/x.txt"} {
		if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", target); !strings.Contains(d, why) {
			t.Errorf("%s: detail %q", target, d)
		}
	}
	if d := deniedPC(t, e, j, "pull", "/sdcard/x"); !strings.Contains(d, why) {
		t.Errorf("working directory in the junction: %q", d)
	}
}
