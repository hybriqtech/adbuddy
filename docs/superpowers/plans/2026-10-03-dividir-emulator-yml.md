# Split `emulator.yml` into the oracle and the app tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** the oracle (about 16 minutes of the emulator job) stops running on pull requests that do not change its inputs, the app tests' path list covers everything `emutest` depends on, and a weekly run of the oracle on `main` catches a change of the system image.

**Basis:** `docs/superpowers/plans/2026-10-03-optimizar-runners-local.md` (measured study; PR1 there is "P2a and P2b"). Today one workflow, `.github/workflows/emulator.yml` (job `mksh`), runs the oracle on android-37 and then, after a restart, the app tests on android-34 in two groups. It triggers on 55 of the last 80 commits; the oracle depends on far fewer paths than it is triggered by, and the app-test paths miss `daemon`, `device`, `ipc`, `pairing`, `pathguard`, `precheck`, `remote`, `screenshot` and `uidump`.

**Architecture:** two workflows. `emulator-oracle.yml` (job `mksh`, same name as today: "oracle against the emulator's mksh") keeps the android-37 emulator, the oracle recording, the `git diff` check and the skills' read-only examples; it also runs weekly on `main`. `emulator-app.yml` (job `app`, name "the app on the emulator") builds the app, starts the android-34 emulator and runs the guard group, the restart and the rest. A Go test keeps the app workflow's `paths` in step with `go list -test -deps ./internal/emutest`. `emulator.yml` is deleted.

**What does NOT change (security, say it in the PR):**
- The oracle still re-records the real mksh's answers and fails if the recording differs from the repository, and runs the skills' read-only examples, whenever its inputs change: `host/internal/classify/testdata/**`, `host/internal/classify/differential_test.go`, `host/internal/classify/corpus_test.go`, `host/internal/workspace/skills/**`, `host/internal/cli/skills_*_test.go`, `scripts/emulator.sh`, its own workflow.
- What changes: it no longer runs when only the evaluator's code changed with the corpus intact, because `TestWhatMkshRunsIsSpecial` and `TestAdmittedSinksStayBelowRed` check the evaluator against the recorded `oracle.jsonl` offline in `host.yml`; the weekly run on `main` catches a new mksh in the system image, which a classify pull request would reveal today. The app tests run on MORE paths than today (the missing dependencies).
- `-race`, gofmt, golden screens, guard tests and the signing check are untouched.

**Global constraints**
- Prose and comments in English. Copy steps from the current `emulator.yml` verbatim unless this plan says otherwise (action pins by SHA with their version comment, `persist-credentials: false`, the fork guard `if`, `shell: bash -e {0}` default, the AVD point-to step, KVM step, the `.bat` fallback for sdkmanager, diagnostics steps, `if: always()` stop step).
- No emulator may be started by an implementer (RAM; other sessions have turns). Verify with `actionlint` if installed (it is not on this machine: use `python -c "import yaml; yaml.safe_load(open(F))"` and read), `cd host && go test ./internal/... -run 'TestDocs|TestAppWorkflow'` and `go vet ./internal/emutest`.
- Never kill anything by image name; never touch adb 5037/5038 or the phone.
- Commit identity local hybriqtech, trailer exactly `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- A required check that a path filter skips stays pending: the PR text and the docs say that neither workflow may be made a required check until that is solved.

---

### Task 1: the app workflow and its path guard

**Files:**
- Create: `.github/workflows/emulator-app.yml`
- Create: `host/internal/emutest/workflow_paths_test.go`

- [ ] **Step 1: `emulator-app.yml`.** `name: emulator-app`. The same header comment idea (runners from variables). Triggers: `pull_request` with `paths` as ONE line of JSON-style flow array (the test parses it), and `workflow_dispatch`:

```yaml
    paths: ["host/internal/emutest/**", "host/internal/adb/**", "host/internal/bridge/**", "host/internal/classify/**", "host/internal/confirm/**", "host/internal/daemon/**", "host/internal/device/**", "host/internal/i18n/**", "host/internal/ipc/**", "host/internal/journal/**", "host/internal/pairing/**", "host/internal/pathguard/**", "host/internal/phoneapp/**", "host/internal/precheck/**", "host/internal/remote/**", "host/internal/safeio/**", "host/internal/screenshot/**", "host/internal/shq/**", "host/internal/uidump/**", "host/go.mod", "host/go.sum", "android/**", "protocol/**", "scripts/emulator.sh", ".github/workflows/emulator-app.yml"]
```

(The implementer regenerates this list from `cd host && go list -test -deps ./internal/emutest | grep adbuddy/host/internal | sed ...` and compares: add anything missing, keep `android/**`, `protocol/**` (the instrumented tests' fixtures), `host/go.mod`, `host/go.sum`.) `permissions: contents: read`. `concurrency: {group: emulator-app-${{ github.ref }}, cancel-in-progress: true}` with the same comment as today.
Job `app`: `name: the app on the emulator`; the same `if` (fork guard on `vars.CI_RUNNER_EMULATOR`), `runs-on` from `CI_RUNNER_EMULATOR`, `timeout-minutes: 75`. Job env: `ADBUDDY_EMU_AVD: adbuddy-ci-api34`, `ADBUDDY_EMU_IMAGE: system-images;android-34;google_apis;x86_64`, `ADBUDDY_EMU_PORT: "5592"`, `ADBUDDY_EMU_ADB_PORT: "5051"`, `ADBUDDY_EMU_STABLE_SECS: "120"`, `ADB_MDNS: "0"`, `ADB_MDNS_AUTO_CONNECT: "0"`, `ADBUDDY_EMULATOR_CUT: ""` (the ports differ from the oracle's 5590 and 5049 so the two workflows never share a console or adb port; one emulator still fits in RAM at a time, so they must not run at the same time on two runners: say it in a comment). Steps: checkout, setup-go, setup-java, setup-gradle (same pins and options), "Point the emulator at the AVD", "Enable KVM", "Install the emulator and the android-34 system image" (sdkmanager `emulator` and `system-images;android-34;google_apis;x86_64`, with the `.bat` fallback), "Build the app and its instrumented tests", "Clear a leftover emulator of this job" (`scripts/emulator.sh stop`), "Start the emulator" (id `start34`, `shell: bash`, the `start | sed` into `$GITHUB_ENV`), then EXACTLY the steps of today's job after "Start the android-34 emulator": guard group, memory report, restart (id `restart34`, same condition), rest group (condition on `restart34`), memory report, diagnostics (Linux and Windows), stop with `if: always()`. Because the env is now the job's, the restart and start steps need no repeated `env:` block.

- [ ] **Step 2: the guard test** `host/internal/emutest/workflow_paths_test.go` (package `emutest`; it must not need an emulator and must run on every OS in `host.yml`):

```go
package emutest

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"os"

	"gopkg.in/yaml.v3"
)

// TestAppWorkflowPathsCoverDependencies keeps the path filter of
// .github/workflows/emulator-app.yml in step with what the package depends
// on: a change in a package emutest imports (directly or not) must run the
// app tests. (The filter had silently missed several packages.)
func TestAppWorkflowPathsCoverDependencies(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash("../../../.github/workflows/emulator-app.yml"))
	if err != nil {
		t.Skipf("the workflow is not in this checkout: %v", err)
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
	have := map[string]bool{}
	for _, p := range wf.On.PullRequest.Paths {
		have[p] = true
	}
	cmd := exec.Command("go", "list", "-test", "-deps", "-f", "{{.ImportPath}}", ".")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Skipf("go list failed (%v): %s", err, errb.String())
	}
	const prefix = "github.com/hybriqtech/adbuddy/host/"
	var missing []string
	for _, imp := range strings.Fields(out.String()) {
		imp = strings.TrimSuffix(imp, ".test")
		rest, ok := strings.CutPrefix(imp, prefix)
		if !ok || !strings.HasPrefix(rest, "internal/") {
			continue
		}
		pkg := strings.SplitN(rest, " ", 2)[0]
		if i := strings.Index(pkg, " ["); i >= 0 {
			pkg = pkg[:i]
		}
		if !have["host/"+pkg+"/**"] {
			missing = append(missing, "host/"+pkg+"/**")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("emulator-app.yml paths miss packages emutest depends on: add %v", missing)
	}
}
```

(The implementer fixes the details so it compiles and passes on Windows and Linux: `go list` runs in the package directory, so import paths for the package itself and its test variant must be deduplicated; use a set; skip the test's own package `internal/emutest` only if the path is already present. Remove the unused import if any.) The test must FAIL when a dependency is missing from `paths`: demonstrate it once by removing one path locally, running it, and restoring it (state the output in the report).

- [ ] **Step 3: verify** `python -c "import yaml; yaml.safe_load(open('.github/workflows/emulator-app.yml'))"`, `cd host && go vet ./internal/emutest && go test -count=1 -run TestAppWorkflowPathsCoverDependencies ./internal/emutest`.

- [ ] **Step 4: Commit** `ci(emulator): the app tests in their own workflow, with complete paths and a guard test`.

---

### Task 2: the oracle workflow, and delete `emulator.yml`

**Files:**
- Create: `.github/workflows/emulator-oracle.yml`
- Delete: `.github/workflows/emulator.yml`

- [ ] **Step 1: `emulator-oracle.yml`.** `name: emulator-oracle`. Triggers: `pull_request` with `paths: ["host/internal/classify/testdata/**", "host/internal/classify/differential_test.go", "host/internal/classify/corpus_test.go", "host/internal/workspace/skills/**", "host/internal/cli/skills_*_test.go", "scripts/emulator.sh", ".github/workflows/emulator-oracle.yml"]`, `workflow_dispatch`, and `schedule: [{cron: "17 4 * * 1"}]` (weekly, Monday 04:17 UTC; a schedule runs on the default branch only, and a comment says why: the system image's mksh can change without any pull request). Comment: why the oracle depends on these paths only (the offline tests check the evaluator against the recorded oracle), and that a required check skipped by a path filter stays pending. `concurrency: {group: emulator-oracle-${{ github.ref }}, cancel-in-progress: true}`. Job `mksh`, `name: oracle against the emulator's mksh`, same `if`, `runs-on`, `timeout-minutes: 60`, job env as today's (`adbuddy-ci-api37`, 5590, 5049, mDNS off). Steps from today's job up to and including "The skills' read-only examples run on the emulator" (checkout, setup-go, setup-java, setup-gradle is NOT needed here: drop it and the app build step), "Point the emulator at the AVD", "Enable KVM", "Install the emulator and the system image" (android-37), "Clear a leftover emulator of this job", "Start the emulator", "Record the oracle", "The recorded oracle is the one in the repository", the skills step, then a memory report (`scripts/emulator.sh mem || true`, `if: always()`), the Linux and Windows diagnostics steps, and the `if: always()` stop. Remove everything about android-34, the build and the restart. In the guard of "The recorded oracle is the one in the repository" nothing changes.

- [ ] **Step 2: delete `.github/workflows/emulator.yml`.**

- [ ] **Step 3: verify** YAML loads; each of the two workflows contains every step of the old file exactly once between them (list the step names of the old file and say where each went); `git grep -n "emulator.yml"` shows the remaining references (Task 3 fixes them).

- [ ] **Step 4: Commit** `ci(emulator): the oracle in its own workflow, weekly on main; delete emulator.yml`.

---

### Task 3: docs

**Files:**
- Modify: `docs/ci-self-hosted.md`, `android/README.md`, `.github/workflows/android.yml` (its step name says "emulator.yml runs them"), `host/README.md` if it names `emulator.yml`

- [ ] **Step 1:** replace every mention of `emulator.yml` by the right workflow (`emulator-oracle.yml` for the oracle and skills; `emulator-app.yml` for the app's tests); in the variables table both use `CI_RUNNER_EMULATOR`; the section about the emulator job says there are two workflows with their own paths, the ports (5590/5049 and 5592/5051), the weekly oracle on `main`, that the app workflow's paths are checked by `TestAppWorkflowPathsCoverDependencies`, and that neither workflow may be a required check while path filters can skip it. Update the durations (oracle about 16-26 minutes, app about 30-40).
- [ ] **Step 2:** `cd host && go test ./internal/... -run TestDocs` passes (no personal paths, no angle-bracket placeholders in prose, no private IPs).
- [ ] **Step 3: Commit** `docs: the emulator workflows are two`.

---

## Validaciones manuales pendientes (CLAUDE.md)

The controller does them with one emulator at a time, once the PR's own CI runs:
1. A pull request that touches only `host/internal/classify/testdata/**` runs `emulator-oracle` and not `emulator-app` (except through the app paths, which include `classify/**`: it runs both); one that touches only `host/internal/daemon/**` runs `emulator-app` only.
2. The weekly schedule fires on `main` (check the run list after the first Monday) and the dispatch works from the Actions tab.
3. The first run of each workflow on the Windows runner: green, with the memory lines.
