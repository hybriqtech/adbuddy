# CI on self-hosted runners Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let every CI job of the repo run on self-hosted runners on the maintainer's PC (so no GitHub-hosted minutes are spent) and switch back to GitHub-hosted runners by changing repository variables, with no workflow edit.

**Architecture:** Each job's `runs-on` is read from a repository variable (`vars.CI_RUNNER_*`) holding a JSON string (`"ubuntu-latest"`) or array (`["self-hosted","windows","adbuddy-local"]`), via `fromJSON(vars.X || '"default"')`. With no variable set, every workflow behaves exactly as today. The host test matrix comes whole from one variable (`CI_HOST_MATRIX`, a JSON array of `{name, runner}` cells) and keeps its cell names so the check names do not change; dropping a cell is leaving it out of the array (`matrix` is not allowed in a job-level `if`). Jobs that run on a self-hosted runner refuse pull requests from forks. `release.yml` is not touched (signing secrets, tags only).

**Tech Stack:** GitHub Actions (`actions/runner` 2.337.0 self-hosted, Windows now, Linux in WSL2 later), actionlint, bash (Git for Windows) on the Windows runner.

**Spec:** this is operational work with no spec; the brief is `PROMPT-puffin.md` (outside the repo, in the workspace root) and the findings in `INVESTIGACION-publicar-y-ci.md` and `COORDINACION.md` (same place). Summary: GitHub Actions does not start any job (billing), the repo stays private, CI is not removed because it caught Linux-only failures (`go test -race`, flock, Unix paths).

## Global Constraints

- Default (no variables set) = today's behaviour: `ubuntu-latest`, `windows-latest`, `macos-latest`.
- Variable values are JSON: a string `"ubuntu-latest"` or an array `["self-hosted","windows","adbuddy-local"]`. The special value `"none"` skips a single-job workflow's job (job-level `if` on `vars`).
- Variables: `CI_HOST_MATRIX` (host.yml: JSON array of cells `{"name":"ubuntu-latest","runner":"ubuntu-latest"}`; default the three GitHub-hosted cells; the macOS cell is dropped locally by leaving it out), `CI_RUNNER_ANDROID` (android.yml), `CI_RUNNER_EMULATOR` (emulator.yml), `CI_RUNNER_RELEASE` (release-check `scripts` and `goreleaser`: Linux with `shellcheck`), `CI_RUNNER_SIGN` (release-check `sign`: Linux with an Android SDK; `apksigner` and `zipalign` are Linux scripts here).
- Never point a Linux cell at the Windows runner: the `runner.os == 'Linux'` steps (gofmt, `-race`) would be skipped and the cell would be green without them.
- Labels of the own runners: `self-hosted,<windows|linux>,adbuddy-local`.
- Every job that can land on a self-hosted runner has `if:` refusing fork pull requests: `github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository`.
- Triggers stay `pull_request` / `push` (never `pull_request_target`); no signing secrets in these jobs; `permissions: contents: read` stays.
- `yes | sdkmanager ...` steps must NOT run under `bash -eo pipefail` (SIGPIPE from `yes`): on Windows use the custom shell `bash -e {0}`, never plain `shell: bash`.
- `actions/checkout` gets `persist-credentials: false` in host, android and emulator (the token must not stay in `.git/config` of a persistent workspace). Caches of setup-go / setup-gradle only on GitHub-hosted runners: `cache: ${{ runner.environment == 'github-hosted' }}` (setup-go) and `cache-disabled: ${{ runner.environment != 'github-hosted' }}` (setup-gradle), so a self-hosted runner never restores tarballs over the user's real `GOMODCACHE`/`~/.gradle`.
- Every job gets `timeout-minutes` (host 30, android 45; emulator keeps 90; release-check 30): one hung job must not hold the only runner.
- On Windows, `sdkmanager` is `sdkmanager.bat`: `sm="$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager"; [ -e "$sm" ] || sm="$sm.bat"; yes | "$sm" ...`. Same on Linux (the first branch).
- Comments that say "Linux only" in android.yml and release-check.yml are updated.
- Action pins (`uses: ...@<sha> # vN`) are not changed.
- Docs in `docs/` are English, no personal paths (no `C:\Users\<name>`), no private IPs: `TestDocs` checks this.
- Commits: identity local `hybriqtech`; the message ends with exactly the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. No `--force`. Do not push (the controller pushes).
- Validation tool for every workflow task: `cd <repo root> && go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/<file>.yml` must print nothing and exit 0 (run it from `host/` as cwd for the Go module cache, passing the absolute path).

---

### Task 1: host.yml reads its matrix from a variable

**Files:**
- Modify: `.github/workflows/host.yml`

**Interfaces:**
- Produces: the `if:` guard and the `fromJSON(vars.X || '"default"')` pattern every other task copies.

- [ ] **Step 1: Replace the `test` job header**

Replace from `jobs:` up to and including `runs-on: ${{ matrix.os }}` with:

```yaml
jobs:
  test:
    name: go test (${{ matrix.name }})
    # Fork pull requests never reach a self-hosted runner.
    if: ${{ github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository }}
    timeout-minutes: 30
    strategy:
      fail-fast: false
      matrix:
        include: ${{ fromJSON(vars.CI_HOST_MATRIX || '[{"name":"ubuntu-latest","runner":"ubuntu-latest"},{"name":"windows-latest","runner":"windows-latest"},{"name":"macos-latest","runner":"macos-latest"}]') }}
    runs-on: ${{ matrix.runner }}
```

`matrix.runner` is a string or an array of labels; `runs-on` accepts both. Leave `defaults` and `steps` as they are (they use `runner.os`), except: `actions/checkout` gets `with: {persist-credentials: false}` and setup-go gets `cache: ${{ runner.environment == 'github-hosted' }}` (keep `cache-dependency-path`).

- [ ] **Step 2: Add a comment under `name: host`**

```yaml
# Runners come from repository variables (docs/ci-self-hosted.md); with none
# set, GitHub-hosted runners are used.
```

- [ ] **Step 3: Audit (read-only) whether the host tests touch user-level state or environment switches**

Run `git grep -nE "UserConfigDir|UserHomeDir|Getenv\(\"(APPDATA|LOCALAPPDATA|USERPROFILE|HOME)\"\)|127\.0\.0\.1:(5037|5038|5039|5555)" -- host` and `git grep -nE "(Getenv|LookupEnv)\(\"(ADBUDDY_|ANDROID_)" -- 'host/**_test.go'`. For every test hit say whether the test isolates it (`t.Setenv`, `t.TempDir`, port 0) and which environment variable switches a test from fake to real (adb, Claude credentials, the phone). Do not fix; list them in your report. A self-hosted runner runs as the maintainer: a test that writes the real home, binds a fixed port or reaches the real adb/phone/credentials could disturb the maintainer's setup.

- [ ] **Step 4: Validate**: actionlint (no output, exit 0).

- [ ] **Step 5: Commit** `ci(host): test matrix from a repository variable` (plus the trailer line).

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
    timeout-minutes: 45
    defaults:
      run:
        shell: bash -e {0}
        working-directory: android
```

`shell: bash -e {0}` is what GitHub already uses on Linux when no shell is given, so Linux is unchanged, and on Windows it selects Git Bash (the steps are bash: `yes | ...`, `$ANDROID_HOME`). The Windows runner has `ANDROID_HOME` and `JAVA_HOME` set in its own `.env` file (not in the workflow).

- [ ] **Step 2: Add the same two-line comment as Task 1 under `name: android`; fix the "Linux only" comment; checkout gets `persist-credentials: false`; setup-gradle gets `cache-disabled: ${{ runner.environment != 'github-hosted' }}`; the `sdkmanager` step uses the `.bat` fallback from the Global Constraints.**

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

Remove the job-level `env: ANDROID_AVD_HOME: /home/runner/...` (keep its explanatory comment) and add as the first step after checkout/setup:

```yaml
      - name: Point the emulator at the AVD (Linux runners)
        if: runner.os == 'Linux'
        run: '[ -z "$XDG_CONFIG_HOME" ] || echo "ANDROID_AVD_HOME=$XDG_CONFIG_HOME/.android/avd" >> "$GITHUB_ENV"'
```

(On GitHub-hosted `XDG_CONFIG_HOME` is `/home/runner/.config`: same value as today. In WSL with it unset, avdmanager's default is already right.) The `sdkmanager` step uses the `.bat` fallback; checkout gets `persist-credentials: false`; setup-go `cache: ${{ runner.environment == 'github-hosted' }}`.

Add `if: runner.os == 'Linux'` to the `Enable KVM` step. In `Emulator diagnostics`, change `if: failure()` to `if: failure() && runner.os == 'Linux'`. The `Start the emulator` step keeps `shell: bash` (with pipefail, on purpose; the step-level shell overrides the default).

- [ ] **Step 3: Check scripts/emulator.sh on Windows**: it already handles `Windows_NT` (`.exe`, `.bat`). Do not edit it (owned by another session); report anything that would break under Git Bash.

- [ ] **Step 4: Validate** with actionlint. **Step 5: Commit** `ci(emulator): runner from a repository variable, Linux-only steps guarded`.

---

### Task 4: release-check.yml reads its runners from variables

**Files:**
- Modify: `.github/workflows/release-check.yml`

- [ ] **Step 1: Edit the three jobs**

`scripts` and `goreleaser`: `runs-on: ${{ fromJSON(vars.CI_RUNNER_RELEASE || '"ubuntu-latest"') }}`; `sign`: `runs-on: ${{ fromJSON(vars.CI_RUNNER_SIGN || '"ubuntu-latest"') }}`. Each job gets `timeout-minutes: 30` and
`if: ${{ vars.<ITS VARIABLE> != '"none"' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}`.
Fix the "Linux only" comment. Add a comment above `jobs:` saying `sign` needs a Linux runner with `ANDROID_HOME` (apksigner and zipalign are Linux scripts here), `scripts` needs `shellcheck`.

- [ ] **Step 2: Validate** with actionlint. **Step 3: Commit** `ci(release-check): runners from repository variables`.

`release.yml` is not edited. Add nothing to it.

---

### Task 5: Documentation and cleanup

**Files:**
- Create: `docs/ci-self-hosted.md`
- Modify: `docs/_config.yml` (add `ci-self-hosted.md` to `exclude`: it is not part of the published user guide) and `host/README.md` (link it next to the `docs/releasing.md` mention)
- Delete: `.github/workflows/ci-runner-smoke.yml`

- [ ] **Step 1: Write `docs/ci-self-hosted.md` (English)** with these sections, concrete commands, no personal paths (use `~/actions-runner` and `%USERPROFILE%\actions-runner`), no IPs:
  1. Why (billing/outage, privacy, speed) and what it does not replace (release.yml stays GitHub-hosted).
  2. The variables table (the five above, default, what each job group needs from the runner), plus the "only a Windows runner for now" example: `CI_HOST_MATRIX='[{"name":"ubuntu-latest","runner":"ubuntu-latest"},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'`, `CI_RUNNER_ANDROID=["self-hosted","windows","adbuddy-local"]`, `CI_RUNNER_RELEASE`/`CI_RUNNER_SIGN` unset or `"none"`; and the warning not to point a Linux cell at a Windows runner. Write `OWNER/REPO` inside code spans (the docs guard rejects angle-bracket placeholders in prose).
  3. Setting up a Windows runner: download from the official `actions/runner` releases and verify the SHA-256 listed in the release notes; get a registration token with `gh api -X POST repos/<owner>/<repo>/actions/runners/registration-token --jq .token` (short-lived; never write it to a file, a log or a commit); `config.cmd --url ... --token ... --labels self-hosted,windows,adbuddy-local`; run `run.cmd` as a foreground process (not a service); runner `.env` file with `ANDROID_HOME` (forward slashes), `JAVA_HOME`, and `GRADLE_USER_HOME`/`GOMODCACHE`/`GOCACHE` pointing to folders of the runner; runner `.path` file: it REPLACES the PATH, so generate it from the current PATH with Git for Windows' `bin` before `C:\Windows\System32` (WSL installs a `bash.exe` there that would shadow Git Bash).
  4. Setting up a Linux runner (WSL2 Ubuntu): `sudo apt install -y build-essential git curl unzip zip shellcheck`, same download/config with `linux-x64` and labels `self-hosted,linux,adbuddy-local`; `gcc` is needed by `go test -race`; optional Android SDK + `ANDROID_HOME` for `CI_RUNNER_SIGN`.
  5. Switching: `gh variable set CI_RUNNER_ANDROID --body '["self-hosted","windows","adbuddy-local"]'`; back to GitHub-hosted: `gh variable delete CI_HOST_MATRIX` (all five); skipping a job: `--body '"none"'`.
  6. Security: a runner executes the code of the branches with the user that runs it, i.e. it can reach that user's adb key (authorised on the phone), adb server, SSH keys, `gh` token and agent credentials. Therefore: recommended, run the runner under a dedicated local Windows account with none of those; minimum, start `run.cmd` from a clean console without `ADBUDDY_*`, `ANDROID_SERIAL` or `ANDROID_ADB_SERVER_PORT`. The real barriers are: private repo, branches of the repo only, "Run workflows from fork pull requests" off in the repo settings (the workflows' fork guard is only an extra: a PR runs the workflow files of its own merge commit). Take the runners down before making the repo public. No `pull_request_target`; no secrets in these jobs. One job at a time per runner (register a second runner directory to parallelise). The emulator job uses and stops the AVD `adbuddy-api37` and the adb server on port 5039 of the runner's machine: do not use them by hand meanwhile. A Linux runner needs passwordless `sudo` or an already accessible `/dev/kvm` for the emulator job.
  7. What is not covered locally: macOS (the macOS cell is left out of `CI_HOST_MATRIX`); anything else the final table of the controller's report says.

- [ ] **Step 2: Exclude and link it** as in Files.

- [ ] **Step 3: Delete the smoke workflow** (`git rm .github/workflows/ci-runner-smoke.yml`).

- [ ] **Step 4: Run the docs guard**: `cd host && go test ./... -run TestDocs -count=1` (find the package with `git grep -ln "func TestDocs"`). Expected: PASS.

- [ ] **Step 5: Commit** `docs: running CI on self-hosted runners`.
