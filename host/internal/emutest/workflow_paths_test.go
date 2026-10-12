package emutest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// hostedLabel is a GitHub-hosted runner label (ubuntu-latest, windows-2022,
// macos-14...).
var hostedLabel = regexp.MustCompile(`^(ubuntu|windows|macos)-[a-z0-9.]+(-[a-z0-9.]+)*$`)

// runnerOfMatrix is a runner the matrix of host.yml names: "runner":"x".
var runnerOfMatrix = regexp.MustCompile(`"runner":"([^"]+)"`)

// TestWorkflowsUseHostedRunnersOnly keeps the one rule of docs/ci-self-hosted.md
// that a workflow file could break: the repository is public, so no job may
// run on a self-hosted runner (a pull request from a fork would run its code on
// a maintainer's machine). Every job's runs-on is a GitHub-hosted label, or
// ${{ matrix.runner }} with only hosted labels in the matrix, and nothing in a
// workflow mentions self-hosted or the CI_* variables that used to pick a
// runner. It fails, never skips.
func TestWorkflowsUseHostedRunnersOnly(t *testing.T) {
	dir := filepath.FromSlash("../../../.github/workflows")
	var files []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		m, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatalf("no workflows found in %s", dir)
	}
	for _, file := range files {
		name := filepath.Base(file)
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var matrixRunners []string
		var walk func(n *yaml.Node)
		walk = func(n *yaml.Node) {
			if n.Kind == yaml.ScalarNode {
				for _, bad := range []string{"self-hosted", "vars.CI_"} {
					if strings.Contains(n.Value, bad) {
						t.Errorf("%s:%d: mentions %q: CI runs on GitHub-hosted runners only", name, n.Line, bad)
					}
				}
				for _, m := range runnerOfMatrix.FindAllStringSubmatch(n.Value, -1) {
					matrixRunners = append(matrixRunners, m[1])
				}
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(&doc)

		var wf struct {
			Jobs map[string]struct {
				RunsOn yaml.Node `yaml:"runs-on"`
			} `yaml:"jobs"`
		}
		if err := doc.Decode(&wf); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(wf.Jobs) == 0 {
			t.Errorf("%s: no jobs parsed", name)
		}
		for job, j := range wf.Jobs {
			switch v := strings.TrimSpace(j.RunsOn.Value); {
			case j.RunsOn.Kind != yaml.ScalarNode:
				t.Errorf("%s: job %s: runs-on must be one GitHub-hosted label, not a list or a group", name, job)
			case v == "${{ matrix.runner }}":
				if len(matrixRunners) == 0 {
					t.Errorf("%s: job %s: runs-on is matrix.runner but no \"runner\" entry was found", name, job)
				}
				for _, r := range matrixRunners {
					if !hostedLabel.MatchString(r) {
						t.Errorf("%s: job %s: matrix runner %q is not a GitHub-hosted label", name, job, r)
					}
				}
			case !hostedLabel.MatchString(v):
				t.Errorf("%s: job %s: runs-on %q is not a GitHub-hosted label", name, job, v)
			}
		}
	}
}
