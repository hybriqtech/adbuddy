# CI on self-hosted runners Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let every CI job of the repo run on self-hosted runners on the maintainer's PC (so no GitHub-hosted minutes are spent) and switch back to GitHub-hosted runners by changing repository variables, with no workflow edit.

**Architecture:** Each job's `runs-on` is read from a repository variable (`vars.CI_RUNNER_*`) holding a JSON string (`"ubuntu-latest"`) or array (`["self-hosted","windows","adbuddy-local"]`), via `fromJSON(vars.X || '"default"')`. With no variable set, every workflow behaves exactly as today. The host test matrix keeps its cell names so the check names do not change. Jobs that run on a self-hosted runner refuse pull requests from forks. `release.yml` is not touched (signing secrets, tags only).

**Tech Stack:** GitHub Actions (`actions/runner` 2.337.0 self-hosted, Windows now, Linux in WSL2 later), actionlint, bash (Git for Windows) on the Windows runner.

**Spec:** this is operational work with no spec; the brief is `PROMPT-puffin.md` (outside the repo, in the workspace root) and the findings in `INVESTIGACION-publicar-y-ci.md` and `COORDINACION.md` (same place). Summary: GitHub Actions does not start any job (billing), the repo stays private, CI is not removed because it caught Linux-only failures (`go test -race`, flock, Unix paths).

## Global Constraints

- Default (no variables set) = today's behaviour: `ubuntu-latest`, `windows-latest`, `macos-latest`.
- Variable values are JSON: a string `"ubuntu-latest"` or an array `["self-hosted","windows","adbuddy-local"]`. The special value `"none"` skips a job (used to drop the macOS cell, which no local runner can cover).
- Variables: `CI_RUNNER_LINUX` (host ubuntu cell, release-check `scripts` and `goreleaser`), `CI_RUNNER_WINDOWS` (host windows cell), `CI_RUNNER_MACOS` (host macOS cell), `CI_RUNNER_ANDROID` (android.yml), `CI_RUNNER_EMULATOR` (emulator.yml), `CI_RUNNER_SIGN` (release-check `sign`: a Linux runner with an Android SDK).
- Labels of the own runners: `self-hosted,<windows|linux>,adbuddy-local`.
- Every job that can land on a self-hosted runner has `if:` refusing fork pull requests: `github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository`.
- Triggers stay `pull_request` / `push` (never `pull_request_target`); no signing secrets in these jobs; `permissions: contents: read` stays.
- `yes | sdkmanager ...` steps must NOT run under `bash -eo pipefail` (SIGPIPE from `yes`): on Windows use the custom shell `bash -e {0}`, never plain `shell: bash`.
- Action pins (`uses: ...@<sha> # vN`) are not changed.
- Docs in `docs/` are English, no personal paths (no `C:\Users\<name>`), no private IPs: `TestDocs` checks this.
- Commits: identity local `hybriqtech`; the message ends with exactly the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. No `--force`. Do not push (the controller pushes).
- Validation tool for every workflow task: `cd <repo root> && go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/<file>.yml` must print nothing and exit 0 (run it from `host/` as cwd for the Go module cache, passing the absolute path).

---

### Task 1: host.yml reads its runners from variables

**Files:**
- Modify: `.github/workflows/host.yml`

**Interfaces:**
- Produces: the pattern every other task copies (matrix `include` with `name` and `runner`, `runs-on: ${{ fromJSON(matrix.runner) }}`, the `if:` guard).

- [ ] **Step 1: Replace the `test` job header**

Replace from `jobs:` up to and including `runs-on: ${{ matrix.os }}` with:

```yaml
jobs:
  test:
    name: go test (${{ matrix.name }})
    # "none" drops a cell; fork pull requests never reach a self-hosted runner.
    if: ${{ matrix.runner != '"none"' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - name: ubuntu-latest
            runner: ${{ vars.CI_RUNNER_LINUX || '"ubuntu-latest"' }}
          - name: windows-latest
            runner: ${{ vars.CI_RUNNER_WINDOWS || '"windows-latest"' }}
          - name: macos-latest
            runner: ${{ vars.CI_RUNNER_MACOS || '"macos-latest"' }}
    runs-on: ${{ fromJSON(matrix.runner) }}
```

Leave `defaults` and `steps` as they are (they use `runner.os`, which works on any runner).

- [ ] **Step 2: Add a comment at the top of the file, under `name: host`**

```yaml
# Runners come from repository variables (docs/ci-self-hosted.md); with none
# set, GitHub-hosted runners are used.
```

- [ ] **Step 3: Audit (read-only) whether the host tests touch user-level state**

Run: `git grep -nE "UserConfigDir|UserHomeDir|os\.Getenv\(\"(APPDATA|LOCALAPPDATA|USERPROFILE|HOME)\"\)|127\.0\.0\.1:(5037|5038|5039|5555)" -- host ':!*_test.go' ':!docs'` and the same on `*_test.go`. For every test hit, say whether the test isolates it (`t.Setenv`, `t.TempDir`, port 0). Do not fix; write the list in your report. A self-hosted runner runs as the maintainer, so a test that writes the real home or binds a fixed port could disturb the maintainer's `adbuddy serve` or adb server.

- [ ] **Step 4: Validate**

Run the actionlint command. Expected: no output, exit 0.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/host.yml
git commit -m "ci(host): runners from repository variables"   # plus the trailer line
```

---

### Task 2: android.yml reads its runner from a variable and runs on Windows

**Files:**
- Modify: `.github/workflows/android.yml`

**Interfaces:**
- Consumes: the `if:` guard and the `fromJSON(vars.X || '"default"')` pattern from Task 1.

- [ ] **Step 1: Edit the `unit` job**

```yaml
jobs:
  unit:
    if: ${{ vars.CI_RUNNER_ANDROID != '"none"' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}
    runs-on: ${{ fromJSON(vars.CI_RUNNER_ANDROID || '"ubuntu-latest"') }}
    defaults:
      run:
        shell: bash -e {0}
        working-directory: android
```

`shell: bash -e {0}` is what GitHub already uses on Linux when no shell is given, so Linux is unchanged, and on Windows it selects Git Bash (the steps are bash: `yes | ...`, `$ANDROID_HOME`). The Windows runner has `ANDROID_HOME` and `JAVA_HOME` set in its own `.env` file (not in the workflow).

- [ ] **Step 2: Add the same two-line comment as Task 1 under `name: android`.**

- [ ] **Step 3: Validate** with actionlint (no output, exit 0).

- [ ] **Step 4: Commit** `ci(android): runner from a repository variable`.

---

### Task 3: emulator.yml reads its runner from a variable and tolerates Windows

**Files:**
- Modify: `.github/workflows/emulator.yml`

- [ ] **Step 1: Job header and shell**

```yaml
jobs:
  mksh:
    name: oracle against the emulator's mksh
    if: ${{ vars.CI_RUNNER_EMULATOR != '"none"' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}
    runs-on: ${{ fromJSON(vars.CI_RUNNER_EMULATOR || '"ubuntu-latest"') }}
    timeout-minutes: 90
    defaults:
      run:
        shell: bash -e {0}
```

- [ ] **Step 2: Make the Linux-only parts conditional**

Remove the job-level `env: ANDROID_AVD_HOME: /home/runner/...` (and keep its explanatory comment) and add as the first step after checkout/setup:

```yaml
      - name: Point the emulator at the AVD (Linux runners)
        if: runner.os == 'Linux'
        run: echo "ANDROID_AVD_HOME=$HOME/.config/.android/avd" >> "$GITHUB_ENV"
```

Add `if: runner.os == 'Linux'` to the `Enable KVM` step. In `Emulator diagnostics`, change `if: failure()` to `if: failure() && runner.os == 'Linux'`. The `Start the emulator` step keeps `shell: bash` (with pipefail, on purpose; the step-level shell overrides the default).

- [ ] **Step 3: Check scripts/emulator.sh on Windows**: it already handles `Windows_NT` (`.exe`, `.bat`). Do not edit it (owned by another session); report anything that would break under Git Bash.

- [ ] **Step 4: Validate** with actionlint. **Step 5: Commit** `ci(emulator): runner from a repository variable, Linux-only steps guarded`.

---

### Task 4: release-check.yml reads its runners from variables

**Files:**
- Modify: `.github/workflows/release-check.yml`

- [ ] **Step 1: Edit the three jobs**

`scripts` and `goreleaser`: `runs-on: ${{ fromJSON(vars.CI_RUNNER_LINUX || '"ubuntu-latest"') }}`; `sign`: `runs-on: ${{ fromJSON(vars.CI_RUNNER_SIGN || '"ubuntu-latest"') }}`. Each job gets
`if: ${{ vars.<ITS VARIABLE> != '"none"' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}`.
Add a comment above `jobs:` saying `sign` needs a Linux runner with `ANDROID_HOME` (apksigner and zipalign are Linux scripts here), `scripts` needs `shellcheck`.

- [ ] **Step 2: Validate** with actionlint. **Step 3: Commit** `ci(release-check): runners from repository variables`.

`release.yml` is not edited. Add nothing to it.

---

### Task 5: Documentation and cleanup

**Files:**
- Create: `docs/ci-self-hosted.md`
- Modify: `docs/index.md` or wherever other contribution docs are linked (look at how `docs/releasing.md` is linked and do the same)
- Delete: `.github/workflows/ci-runner-smoke.yml`

- [ ] **Step 1: Write `docs/ci-self-hosted.md` (English)** with these sections, concrete commands, no personal paths (use `~/actions-runner` and `%USERPROFILE%\actions-runner`), no IPs:
  1. Why (billing/outage, privacy, speed) and what it does not replace (release.yml stays GitHub-hosted).
  2. The variables table (the six above, default, what each job group needs from the runner).
  3. Setting up a Windows runner: download from the official `actions/runner` releases and verify the SHA-256 listed in the release notes; get a registration token with `gh api -X POST repos/<owner>/<repo>/actions/runners/registration-token --jq .token` (short-lived; never write it to a file, a log or a commit); `config.cmd --url ... --token ... --labels self-hosted,windows,adbuddy-local`; run `run.cmd` as a foreground process (not a service); runner `.env` file with `ANDROID_HOME` and `JAVA_HOME`; runner `.path` file so Git for Windows' `bash` comes before `C:\Windows\System32\bash.exe` (installed with WSL).
  4. Setting up a Linux runner (WSL2 Ubuntu): `sudo apt install -y build-essential git curl unzip zip shellcheck`, same download/config with `linux-x64` and labels `self-hosted,linux,adbuddy-local`; `gcc` is needed by `go test -race`; optional Android SDK + `ANDROID_HOME` for `CI_RUNNER_SIGN`.
  5. Switching: `gh variable set CI_RUNNER_WINDOWS --body '["self-hosted","windows","adbuddy-local"]'`; back to GitHub-hosted: `gh variable delete CI_RUNNER_WINDOWS` (all six); skipping a job: `--body '"none"'`.
  6. Security: a runner executes the code of the branches with the user that runs it; private repo and branches of the repo only; fork pull requests are refused by the workflows (and leave "Run workflows from fork pull requests" off in the repo settings); no `pull_request_target`; no secrets in these jobs; one job at a time per runner (register a second runner directory to parallelise).
  7. What is not covered locally: macOS (`CI_RUNNER_MACOS="none"`); anything else the final table of the controller's report says.

- [ ] **Step 2: Link it** from the same place as `docs/releasing.md`.

- [ ] **Step 3: Delete the smoke workflow** (`git rm .github/workflows/ci-runner-smoke.yml`).

- [ ] **Step 4: Run the docs guard**: `cd host && go test ./... -run TestDocs -count=1` (find the package with `git grep -ln "func TestDocs"`). Expected: PASS.

- [ ] **Step 5: Commit** `docs: running CI on self-hosted runners`.
