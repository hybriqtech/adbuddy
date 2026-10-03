package emutest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAppWorkflowPathsCoverDependencies keeps the path filter of
// .github/workflows/emulator-app.yml in step with what this package depends
// on: a change in a package emutest imports (directly or not) must run the
// app tests. The filter had silently missed several packages. It fails, never
// skips: a renamed workflow or a broken go must not let it pass.
func TestAppWorkflowPathsCoverDependencies(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash("../../../.github/workflows/emulator-app.yml"))
	if err != nil {
		t.Fatalf("the app workflow: %v", err)
	}
	var wf struct {
		On struct {
			PullRequest struct {
				Paths []string `yaml:"paths"`
			} `yaml:"pull_request"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	if len(wf.On.PullRequest.Paths) == 0 {
		t.Fatal("no on.pull_request.paths parsed from emulator-app.yml")
	}
	have := map[string]bool{}
	for _, p := range wf.On.PullRequest.Paths {
		have[p] = true
	}
	// The union over every GOOS: some dependencies exist behind build tags
	// (procinfo is Linux-only), and the test must not depend on where it runs.
	const prefix = "github.com/hybriqtech/adbuddy/host/"
	missing := map[string]bool{}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		cmd := exec.Command("go", "list", "-test", "-deps", "-f", "{{.ImportPath}}", ".")
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("go list (GOOS=%s): %v: %s", goos, err, errb.String())
		}
		for _, imp := range strings.Fields(out.String()) {
			rest, ok := strings.CutPrefix(strings.TrimSuffix(imp, ".test"), prefix)
			if !ok {
				continue
			}
			if path := "host/" + rest + "/**"; !have[path] {
				missing[path] = true
			}
		}
	}
	var list []string
	for m := range missing {
		list = append(list, m)
	}
	sort.Strings(list)
	if len(list) > 0 {
		t.Errorf("emulator-app.yml paths miss packages emutest depends on: add %v", list)
	}
}
