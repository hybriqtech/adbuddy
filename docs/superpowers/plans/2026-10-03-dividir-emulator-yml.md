# Split `emulator.yml` into the oracle and the app tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** the oracle (about 16 minutes of the emulator job) stops running on pull requests that do not change its inputs, the app tests' path list covers everything `emutest` depends on, and a weekly run of the oracle on `main` catches a change of the system image.

**Basis:** `docs/superpowers/plans/2026-10-03-optimizar-runners-local.md` (measured study, commit d36d66a of `origin/ci/runners-optimizacion`; PR1 there is "P2a and P2b"; the file may not be in this branch). Today one workflow, `.github/workflows/emulator.yml` (job `mksh`), runs the oracle on android-37 and then, after a restart, the app tests on android-34 in two groups. It triggers on 55 of the last 80 commits; the oracle depends on far fewer paths than it is triggered by, and the app-test paths miss `daemon`, `device`, `ipc`, `pairing`, `pathguard`, `precheck`, `remote`, `screenshot` and `uidump`.

**Architecture:** two workflows. `emulator-oracle.yml` (job `mksh`, same name as today: "oracle against the emulator's mksh") keeps the android-37 emulator, the oracle recording, the `git diff` check and the skills' read-only examples; it also runs weekly on `main`. `emulator-app.yml` (job `app`, name "the app on the emulator") builds the app, starts the android-34 emulator and runs the guard group, the restart and the rest. A Go test keeps the app workflow's `paths` in step with `go list -test -deps ./internal/emutest`. `emulator.yml` is deleted.

**What does NOT change (security, say it in the PR):**
- The oracle still re-records the real mksh's answers and fails if the recording differs from the repository, and runs the skills' read-only examples, whenever its inputs change: `host/internal/classify/testdata/**`, `host/internal/classify/differential_test.go`, `host/internal/classify/corpus_test.go`, `host/internal/workspace/skills/**`, `host/internal/workspace/skills.go` (the embed), `host/internal/cli/skills_*_test.go`, `scripts/emulator.sh`, its own workflow.
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
	cmd := exec.Command("go", "list", "-test", "-deps", "-f", "{{.ImportPath}}", ".")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list: %v: %s", err, errb.String())
	}
	const prefix = "github.com/hybriqtech/adbuddy/host/"
	missing := map[string]bool{}
	for _, imp := range strings.Fields(out.String()) {
		rest, ok := strings.CutPrefix(strings.TrimSuffix(imp, ".test"), prefix)
		if !ok || strings.HasPrefix(rest, "[") {
			continue
		}
		if path := "host/" + rest + "/**"; !have[path] {
			missing[path] = true
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
```

(The `[...]` tokens of `go list -test` are test variants and are skipped; `emutest.test` is trimmed. The implementer makes it compile and pass on Windows and Linux, and updates `host/internal/emutest/doc.go`, which says every test of the package skips without an emulator: this one does not.) The test must FAIL when a dependency is missing from `paths`: demonstrate it once by removing one path locally, running it, and restoring it (state the output in the report).

- [ ] **Step 2b: `host.yml`** gets `".github/workflows/emulator-app.yml"` in both its `paths` lists, so a pull request that only edits the app workflow's path list still runs the guard test on `host.yml` at once instead of 30 minutes later inside the app job.
- [ ] **Step 3: verify** `gofmt -l host` (empty), `python -c "import yaml; yaml.safe_load(open('.github/workflows/emulator-app.yml'))"`, `cd host && go vet ./internal/emutest && go test -count=1 -run TestAppWorkflowPathsCoverDependencies ./internal/emutest`.

- [ ] **Step 4: Commit** `ci(emulator): the app tests in their own workflow, with complete paths and a guard test`.

---

### Task 2: the oracle workflow, and delete `emulator.yml`

**Files:**
- Create: `.github/workflows/emulator-oracle.yml`
- Delete: `.github/workflows/emulator.yml`

- [ ] **Step 1: `emulator-oracle.yml`.** `name: emulator-oracle`. Triggers: `pull_request` with `paths: ["host/internal/classify/testdata/**", "host/internal/classify/differential_test.go", "host/internal/classify/corpus_test.go", "host/internal/workspace/skills/**", "host/internal/workspace/skills.go", "host/internal/cli/skills_*_test.go", "scripts/emulator.sh", ".github/workflows/emulator-oracle.yml"]`, `workflow_dispatch`, and `schedule: [{cron: "17 4 * * 1"}]` (weekly, Monday 04:17 UTC; a schedule runs on the default branch only, and a comment says why: the system image's mksh can change without any pull request). Comment: why the oracle depends on these paths only (the offline tests check the evaluator against the recorded oracle), and that a required check skipped by a path filter stays pending. `concurrency: {group: emulator-oracle-${{ github.ref }}, cancel-in-progress: true}`. Job `mksh`, `name: oracle against the emulator's mksh`, same `if`, `runs-on`, `timeout-minutes: 60` and the recording step's `go test -timeout 45m` (so Go's own timeout fires before the job's), job env as today's (`adbuddy-ci-api37`, 5590, 5049, mDNS off). Steps from today's job up to and including "The skills' read-only examples run on the emulator" (checkout, setup-go, setup-java, setup-gradle is NOT needed here: drop it and the app build step), "Point the emulator at the AVD", "Enable KVM", "Install the emulator and the system image" (android-37), "Clear a leftover emulator of this job", "Start the emulator", "Record the oracle", "The recorded oracle is the one in the repository", the skills step, then a memory report (`scripts/emulator.sh mem || true`, `if: always()`), the Linux and Windows diagnostics steps, and the `if: always()` stop. Remove everything about android-34, the build and the restart. In the guard of "The recorded oracle is the one in the repository" nothing changes.

- [ ] **Step 2: delete `.github/workflows/emulator.yml`.**

- [ ] **Step 3: verify** the YAML of both loads; every step of the old file is where this table says, and nowhere else:

| Step of the old `emulator.yml` | Goes to |
| --- | --- |
| checkout, setup-go, setup-java (`sdkmanager` and `avdmanager` are Java) | both |
| setup-gradle, "Build the app and its instrumented tests" | app only |
| "Point the emulator at the AVD", "Enable KVM" | both |
| "Install the emulator and the system image" (android-37) | oracle |
| "Install the android-34 system image" | app, merged with installing the `emulator` package |
| "Clear a leftover emulator", "Start the emulator" | both (in the app, id `start34`) |
| "Record the oracle", "The recorded oracle is the one in the repository", the skills step, "Emulator memory after the oracle" | oracle |
| "Stop the android-37 emulator", "Start the android-34 emulator" | disappear: the app workflow has one start and one final stop |
| guard group, memory, "Restart" (`restart34`), rest group, memory | app |
| "Emulator diagnostics" (Linux and Windows), "Stop the emulator" with `if: always()` | both |

Update the Windows diagnostics comment in each file: "the log is the last emulator started (android-37 or android-34)" is no longer true in either. Add to both workflows a comment: with a single runner carrying the emulator label the two workflows run one after the other; with two runners on the same machine they would run together, and neither the RAM nor the log file `adbuddy-emulator.log` (shared, in the temp folder) would hold: see `docs/ci-self-hosted.md`. Add to the oracle workflow a comment on the weekly schedule: it runs on the self-hosted Windows runner at 04:17 UTC on Mondays; if the PC is off the job waits in the queue (at most 24 hours, then it is cancelled), when it runs it competes for RAM with development emulators, and a failure is reported to whoever last edited the cron line, so the docs name who reads the run list each week (the supervisor).
- `git grep -n "emulator.yml"` lists what Task 3 must fix.

- [ ] **Step 4: Commit** `ci(emulator): the oracle in its own workflow, weekly on main; delete emulator.yml`.

---

### Task 3: docs

**Files:**
- Modify: `docs/ci-self-hosted.md` (the `CI_RUNNER_ANDROID` duration; the variables table row of `CI_RUNNER_EMULATOR`; the triggers paragraph, with the weekly run; "the job restarts the emulator after the oracle"; every "emulator job" mention; the sentence that says the emulator job stops the android-37 emulator and starts the android-34 one on the same port, false now; the `emulator.yml` mentions in the security sections; the AVD and ports line: add `adbuddy-ci-api34`, 5592 and 5051), `.github/workflows/android.yml` (its step name says "emulator.yml runs them"), `docs/superpowers/plans/pendientes-m3-m5.md` (mark "split emulator.yml in two" as done), `host/internal/emutest/doc.go`; `android/README.md` and `host/README.md` only if `git grep` finds `emulator.yml` there.
- Do NOT rewrite the historical plans and specs under `docs/superpowers/` nor `.superpowers/`.

- [ ] **Step 1:** replace every mention of `emulator.yml` by the right workflow (`emulator-oracle.yml` for the oracle and skills; `emulator-app.yml` for the app's tests); in the variables table both use `CI_RUNNER_EMULATOR`; the section about the emulator job says there are two workflows with their own paths, the ports (5590/5049 and 5592/5051), the weekly oracle on `main`, that the app workflow's paths are checked by `TestAppWorkflowPathsCoverDependencies`, and that neither workflow may be a required check while path filters can skip it. Update the durations (oracle about 16-26 minutes, app about 30-40).
- [ ] **Step 2:** `cd host && go test ./internal/... -run TestDocs` passes (no personal paths, no angle-bracket placeholders in prose, no private IPs).
- [ ] **Step 3: Commit** `docs: the emulator workflows are two`.

---

## Validaciones manuales pendientes (CLAUDE.md)

The controller does them with one emulator at a time, once the PR's own CI runs:
1. Pull requests by what they touch: only `host/internal/classify/testdata/**` runs both workflows (it is under `classify/**`); only `host/internal/classify/*.go` or `host/internal/daemon/**` runs the app workflow only; only `host/internal/workspace/skills/**` runs the oracle only.
2. The weekly schedule fires on `main` (check the run list after the first Monday) and the dispatch works from the Actions tab.
3. The first run of each workflow on the Windows runner: green, with the memory lines.
