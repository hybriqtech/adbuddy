# Optimising the CI and the local runners: measured study and PR order

Scope: study only. Nothing here changes a runner, a variable or a workflow; each proposal is a later, small PR, or a decision for the user. Nothing that protects security today is removed without saying so (see "What stays").

## What was measured

Source: the jobs of the last 80 workflow runs (79 with jobs; one `startup_failure` has none), 2026-10-02 19:33 UTC to 2026-10-03 11:38 UTC (about 16 hours), from `gh api .../actions/runs/ID/jobs` (job `created_at`, `started_at`, `completed_at`, runner name, steps). The window includes the days of the emulator-memory trouble, so durations are on the high side. One Windows host job queued 72.8 minutes and was cancelled without ever getting a runner: it is left out of runner-minutes and not counted as a wait.

| Job (runner) | n | wait p50 / p90 / max (min) | duration median / max (min) | runner-minutes |
| --- | --- | --- | --- | --- |
| emulator workflow, job oracle (Windows) | 17 (14 ok, 2 failed, 1 cancelled) | 4.1 / 30.6 / 30.9 (16 not cancelled) | 26.0 / 35.8 (ok) | 413 (16 jobs); 453 with the cancelled one |
| go test windows (Windows) | 38 | 0.1 / 47.6 / 76.2 | 2.0 / 4.3 | 68 |
| go test ubuntu (Linux, WSL) | 39 | 0.0 / 111 / 137.5 | 3.0 / 3.5 | 78 |
| android unit (Linux, WSL) | 17 | 2.5 / 125 / 126 | 1.6 / 13.8 | 50 |
| sign, goreleaser, scripts (Linux) | 5 each | about 8 / - / 117 | under 2 | 9 |

Total: 658 runner-minutes. The emulator workflow is 69% of them (453 of 658) and the Windows runner runs one job at a time.

By event (runner-minutes): emulator on pull requests 453; host on pull requests 156 and on pushes to main 62; android 30 and 20; release-check 5 and 4. Pushes to main are 86 minutes (13%).

### Where the time goes (steps of the last four successes of each main job)

Only steps of 8 s or more are listed: every other step, including each SDK, JDK and Gradle installation step, is under 8 s at the median, so repeated installation is not a cost. Caches are already persistent: each runner's `.env` points `GRADLE_USER_HOME`, `GOMODCACHE` and `GOCACHE` into its own folder, so `cache: false` and `cache-disabled` in the workflows only switch off the actions' remote cache and there is nothing left to enable.

| Job | Step | median / max (s) |
| --- | --- | --- |
| emulator workflow | record the oracle (8000 cases through adb) | 858 / 1035 |
| | start the android-37 emulator | 43 / 96 |
| | start the android-34 emulator (includes the 120 s stability wait) | 152 / 157 |
| | restart the android-34 emulator | 156 / 156 |
| | app tests, rest group / guard group | 440 / 761, 103 |
| | build the app and its instrumented tests | 46 / 52 |
| host windows | `go test ./...` | 68 / 125 |
| host ubuntu | `go test ./...`, `go test -race` | 44 / 84, 58 / 106 |
| android unit | Gradle (tests, assemble, lint) | 352 / 647 in these four runs; the job's median is 1.6 min and its maximum 13.8 min: the variance is large and not explained (cold Gradle state after a runner restart, or the 6 WSL CPUs shared with Windows jobs) |

### Why jobs wait

For every job that waited more than 5 minutes, the wait was split into the time its runner was busy with another job and the time it was idle:

| Runner | Jobs | Total wait (min) | Runner busy | Runner idle (offline or not yet picked) |
| --- | --- | --- | --- | --- |
| Windows | 22 | 627 | 582 (93%) | 45 |
| Linux (WSL) | 25 | 1788 | 211 (12%) | 1577 (88%) |

- **Windows waits are a queue**: 33% of Windows host jobs wait more than 10 minutes (p90 47.6 min), and all 15 that waited more than 5 minutes had an emulator job running on the single Windows runner meanwhile.
- **Linux waits are an offline runner**, not a queue (jobs take about 3 minutes): the Linux runner has no process of its own after a reboot or when the shell that held it ends. 15% of Ubuntu jobs and 24% of android jobs waited more than 10 minutes.
- Superseded runs (a later run of the same workflow, branch and event exists): 182 runner-minutes, but 86 are emulator runs (that workflow already has `concurrency` with `cancel-in-progress`, since 2026-10-02 22:25 UTC) and 73 are pushes. Among host, android and release-check pull requests only 22 minutes were superseded, and none could have been cut: each of those runs had finished before the next push.

### Facts about the machine

- No runner starts by itself: no scheduled task, no service, no Run-key entry. The only Startup entries are unrelated shortcuts. The runners' launch scripts are `start-runner-windows.cmd` and `start-runner-linux.cmd` in the runners folder of the workspace, outside the repository.
- Hardware: AMD Ryzen 7 7800X3D (8 cores, 16 threads), 31 GB of RAM, 239 GB free on C:. WSL 3.0.1 is capped at 8 GB, `processors=6`, `swap=2GB`; its idle memory is about 0.5 GB.
- No KVM in WSL today: `/dev/kvm` does not exist. The CPU is AMD and `svm` is visible inside WSL (nested virtualization is exposed), so what is missing is the `kvm_amd` module or device, not the hardware.
- One qemu of the emulator peaks at about 8 GB private memory and grows through a run (see the emulator-memory study); two emulators at once do not fit with the rest of the machine's load.

## What the workflows run, and when

| Workflow | Trigger | Jobs | Notes |
| --- | --- | --- | --- |
| host | pull request and push to main; paths host, protocol, README, docs | `go test` on ubuntu (`-race`, gofmt) and windows, in parallel on different runners | docs-only changes under `docs/superpowers` trigger it |
| android | same, paths android, protocol | one job on the Linux runner | |
| release-check | pull request and push; release paths | scripts, goreleaser, sign | tiny |
| emulator | pull request and dispatch; paths classify, skills, emutest, phoneapp, adb, bridge, shq, safeio, journal, confirm, i18n, go.mod, go.sum, android, emulator.sh | the oracle (android-37) then the app tests (android-34, two emulator starts) in one job | has `concurrency` |

Findings:
- **The emulator job is one 55-65 minute block** on the only Windows runner: the oracle (about 26 minutes) and the app tests (about 30 minutes with two starts).
- **The oracle runs on far more paths than it depends on.** It records the answers of the phone's mksh for the corpus (`differential.jsonl` plus the cases `generatedCases()` builds in `differential_test.go`) into `oracle.jsonl`, then the job fails if the recording differs from the repository. The offline unit tests already fail if the evaluator and `oracle.jsonl` disagree on any case, or if a case has no recorded answer. So re-recording is only informative when the corpus, the generator or `oracle.jsonl` changed (a hand-edited `oracle.jsonl` is exactly what the job catches) or when the emulator setup changed. A change to the evaluator alone is checked by the unit tests against the recorded oracle.
- **The app tests' path list is incomplete.** `go list -deps` of `emutest` shows `classify`, `daemon`, `ipc`, `pairing`, `pathguard`, `precheck`, `screenshot` and `uidump` as dependencies; none of `daemon`, `ipc`, `pairing`, `pathguard`, `precheck`, `screenshot` or `uidump` is in the workflow's paths, so a change there does not run the app tests today. This is a hole in the check, to be closed in the split below.
- **Matrix**: `-race` and gofmt run only on the ubuntu cell and the Windows cell tests Windows semantics; they run in parallel on different runners, so the matrix costs no queue by itself.
- **Pushes to main** repeat what the pull request passed: `main` advances by fast-forward (49 of the last 60 first-parent commits are plain commits; the 11 merge commits are branch tips that were merged into the branch before it was fast-forwarded). They cost 86 minutes, mostly the cheap host jobs.
- **Automatic retries of cancelled jobs**: none was needed in the window (the cancelled jobs were cancelled on purpose). A retry loop would hide flaky tests and could loop; do it by hand with `gh run rerun --failed`.

## Proposals

Impact: runner-minutes saved or waiting removed. "Estimate" means computed from the measurements above, not measured after the change. Cost: effort. Risk: what can go wrong.

| # | Proposal | Before (measured) | After (estimate) | Cost | Risk |
| --- | --- | --- | --- | --- | --- |
| P5 | Start both runners at user logon (Startup-folder shortcuts to the existing launch scripts) | Linux: 1577 min of idle-runner wait; 15-24% of Linux jobs wait more than 10 min | those waits go away after a reboot | two shortcuts | changes what runs at login: the user's call; starts only after the user logs in (a service that runs without a login needs an administrator and a dedicated account; automatic logon is the other way) |
| P2a | Split `emulator.yml` into the oracle and the app tests, each with its own `paths` and its own concurrency group (`emulator-oracle-REF`, `emulator-app-REF`) | the oracle half is about 16 minutes (recording 14.3, start, build) and runs on 55 of the last 80 commits (first-parent commits stand in for pull requests) | the oracle runs on about 13 of those 80 (corpus, generator, `oracle.jsonl`, emulator script, workflow); the other 42 skip about 16 minutes each | one workflow file; the app workflow's paths come from `go list -deps` of `emutest` | a wrong `paths` list skips a needed check. Both keep `scripts/emulator.sh` and their own file in their paths. A required check that a path filter skips stays pending and blocks the merge, so make neither required until that is solved. In a single Windows runner the split does not shorten the two halves when both run: it lets a host job slip between them and lets a PR skip the oracle |
| P2b | App-test paths complete (add `classify`, `daemon`, `ipc`, `pairing`, `pathguard`, `precheck`, `screenshot`, `uidump`) and a guard that keeps them in step (a script or test that compares the list with `go list -deps`) | those packages are untested on the emulator today | the hole is closed; more PRs run the app tests, so measure the mix before counting | small | more emulator runs: the cost of the check that was missing |
| P3 | A second Windows runner for `host` only | 33% of Windows host jobs wait more than 10 minutes (p90 47.6) | those waits go to about 0 | about 30 minutes, a few GB of disk (its own Go and Gradle caches); same account as the first | Before registering it, give the first runner an extra label `adbuddy-emulator` and set `CI_RUNNER_EMULATOR` to `["self-hosted","windows","adbuddy-emulator"]`; the second gets only `self-hosted,windows,adbuddy-local`. With today's labels the emulator job could land on it and two emulators with the same fixed ports (5590 and 5049) would collide. Same exposure as the first (A5-1). CPU contention with an emulator job might make timing-sensitive host tests flaky: run `go test ./...` once while an emulator job is running before relying on it. Two emulators at once do not fit in RAM |
| P4 | `host.yml` ignores `docs/superpowers/**` (`"!docs/superpowers/**"` after `"docs/**"` in `paths`) | 10 of the last 80 first-parent commits touch only `docs/superpowers`; each triggers 2 host jobs (about 5 runner-minutes) | those runs disappear | one line per trigger | `TestDocs` does not read `docs/superpowers` (the folder is in `_config.yml`'s `exclude` and the walk skips it: checked). A PR that changes an excluded file together with any included one still runs. Same required-check caveat as P2a |
| P6 | Android Gradle: try a daemon or the configuration cache on the self-hosted runners | variance 1.6 to 13.8 min, unexplained | not estimated | an experiment behind a variable | the configuration cache can break Roborazzi or the instrumented build |
| P7 | `autoMemoryReclaim=gradual` in `.wslconfig` (section `experimental`) | idle 0.5 GB; the Linux runner keeps page cache after a Gradle build | frees order of 1 GB (not measured) | one line plus `wsl --shutdown` | `wsl --shutdown` kills a running Linux job: only when idle, and the user agrees |
| P8 | The emulator job on the Linux runner | needs `/dev/kvm` | would run beside the Windows host jobs without a second Windows runner | one experiment (the `kvm_amd` module in the WSL kernel), a memory cap above the emulator's 8 GB peak, GPU | medium; parked until P2, P3 and P5 are in |
| P1 | `concurrency` for host, android, release-check on pull requests only | about 0 runner-minutes in this sample (every superseded pull-request run had already finished) | hygiene for bursts of pushes | one block per workflow | never on pushes to main: a pending push run is cancelled when a third one arrives, even with `cancel-in-progress: false`. A cancelled check shows as cancelled: re-run after the new push if it ever becomes required |
| P9 | Drop the `push` to main trigger of android and release-check (host stays) | 24 runner-minutes of the 86 | those 24 | two lines | `main` is the fast-forward of a tested PR but the PR was tested merged into the main of that moment, so main is not byte-for-byte what ran. Leave it unless the queue stays long: the saving is small |

Already settled by the measurements: persistent caches (nothing to enable), repeated SDK, JDK and Gradle installs (each under 8 s), moving the Windows host cell to Linux (it exists to test Windows), sampling a subset of the oracle's cases in pull requests (a partial recording gives false assurance: skip the oracle entirely when its inputs did not change instead), and automatic retries.

## What stays

No check that protects security is removed or weakened:
- **The oracle** (real mksh answers, re-recorded and compared) still runs whenever its inputs change: the corpus, the generator, `oracle.jsonl`, the emulator script or the workflow. What changes is that it no longer re-records on an edit of the evaluator alone, which the unit tests check against the recorded oracle. If that trade is not wanted, add a weekly scheduled run of the full oracle on `main` instead.
- **`-race`, gofmt, the golden screens, the app and guard tests, the signing check** all keep running with the same commands.

## Order of the pull requests (one at a time)

1. **P2a and P2b**: split the emulator workflow, with complete paths. The biggest effect, and it closes a hole.
2. **P4**: the `docs/superpowers` path filter.
3. **P3**: the second Windows runner, with the label separation first and one measurement of host tests under an emulator job; the user decides on the dedicated account.
4. **P5**: logon start (the user decides).
5. **P1** (hygiene), **P9** (only if queues stay long), then **P6** and **P7** as experiments each behind its own measurement, and **P8** last.

## For the user

- **A dedicated Windows account for the runners** (A5-1): still the only real barrier for adb's key; a second runner doubles the exposure unless this is done.
- **Start at logon (P5)**: do you want both runners to start when you log in? Without a login nothing runs, with or without a Startup shortcut; unattended survival of a reboot needs automatic logon or a service run by an administrator under a dedicated account (the default service accounts are not acceptable for these jobs).
- **A second Windows runner (P3)**: go or no-go; reversible by removing the runner. It needs the label change on the first runner and one variable.
- **`wsl --shutdown` (P7)**: only when no Linux job is running.
