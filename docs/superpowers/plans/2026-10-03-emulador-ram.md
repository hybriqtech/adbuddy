# Emulator RAM in CI: restart between test groups, plain-MB guest RAM, memory report — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** the CI `emulator` job (and any long local run) must not let the emulator's qemu process grow past ~6 GB of RAM on the runner PC, which has 31 GB shared with other work.

**Measured basis** (`.superpowers/sdd/2026-10-03-ram-emuladores/mediciones.md`, android-34, 1080x2400, 2560 MB guest, 4 cores, swiftshader_indirect, one emulator alone on the PC): qemu's working set is 3.6 GB after boot and only grows, never returning memory: 7.3 GB after the full `emutest` (10 min), 5.5 GB without the `TestTheGuard*` tests. A CI job that ran the oracle on android-37 and then the whole `emutest` reached 14-15 GB. Screen size, core count and `-gpu guest` change it by ±200 MB. The AVD's `hw.ramSize` is not what the emulator honours when written as `1536M`: the effective value (`hardware-qemu.ini`) stayed 2560.

**Architecture:** three small changes. `scripts/emulator.sh` gets `ADBUDDY_EMU_RAM` (guest RAM in plain MB, passed as `-memory`) and a `mem` command that prints the emulator's host memory. `emulator.yml` runs `emutest` in two invocations (the guard tests, then the rest) with the emulator restarted between them, and prints the memory after each. `docs/ci-self-hosted.md` says so.

**Ordering (review I1):** `TestTheGuard*` (guard_test.go, overlay_test.go) exists only in `origin/test/guard-samples` (PR #46), not yet in `main`. The implementer works on a branch from `origin/main`; the controller rebases it onto `main` once #46 is in and validates it on top of `test/guard-samples`. The guard group therefore FAILS when it finds no guard test (Task 2 step 1), so a stale split never goes green silently. With the guard group out, the "rest" group is what was measured as `leakNoGuard`: peak 5.0 GB, 5.5 GB at the end (under the 6 GB target by little: say so in the docs).

**Not in this plan:** which value `ADBUDDY_EMU_RAM` takes in CI (the controller measures `-memory 1536` and confirms the guest still boots and the suite passes; until then the workflow does not set it), the android-37 oracle's own memory (not measured), A5-6/A5-7 (a separate plan).

**Global constraints**
- Comments and prose in English, like the rest of `scripts/` and `.github/`; docs obey `TestDocs` (no personal paths, no `/home/` literals, no angle-bracket placeholders in prose, no private IPs).
- Windows runner: the runner starts jobs with a minimal PATH (Git for Windows' `bin` first); do not assume `powershell` is on PATH: use `"$SYSTEMROOT/System32/WindowsPowerShell/v1.0/powershell.exe"` when `SYSTEMROOT` (or `SystemRoot`) is set, else `powershell.exe`.
- Never kill anything by image name (`taskkill /IM`, `pkill`, `Stop-Process -Name`): only through `emulator.sh stop` (adb `emu kill`) or a PID checked against its command line.
- Never start an emulator while another one is running on the PC; the controller validates by hand. Implementers verify with `bash -n`, `shellcheck` if installed, `actionlint` if installed, and by reading, NOT by starting an emulator.
- Commit trailer exactly `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; identity local hybriqtech.

---

### Task 1: `scripts/emulator.sh`: `ADBUDDY_EMU_RAM` and `mem`

**Files:**
- Modify: `scripts/emulator.sh`

- [ ] **Step 1: Header.** Document, next to the other `ADBUDDY_EMU_*` knobs: `ADBUDDY_EMU_RAM` = the guest's RAM in plain MB (1024-8192, empty = the AVD's own), passed to the emulator as `-memory` ("1536M" or "2G" in the AVD's config are not honoured the same way: the emulator may raise them). Document the `mem` command: prints one line with the emulator's working set and private memory in MB, or says it is not running; always exits 0 (it is a diagnostic). Update `usage()` to `start|stop|env|mem`.

- [ ] **Step 2: Validation at the top of `start()` (not at script level: `stop` and `mem` must keep exiting 0 whatever the environment says)**, and `$PORT` validated like `ADB_PORT` at script level (`^[0-9]{1,5}$`, else exit 2) because `mem` puts it in a PowerShell command:

```bash
RAM="${ADBUDDY_EMU_RAM:-}"
if [[ -n "$RAM" ]] && { [[ ! "$RAM" =~ ^[0-9]{3,4}$ ]] || ((10#$RAM < 1024 || 10#$RAM > 8192)); }; then
  echo "ADBUDDY_EMU_RAM='$RAM' is not valid: plain MB between 1024 and 8192" >&2
  exit 2
fi
```

Add this case to the exit-code-2 line of the comment ("for `start`").

- [ ] **Step 3: `-memory` in the launch.** After `args=( ... )` is built and before `-gpu`, `[[ -z "$RAM" ]] || args+=(-memory "$((10#$RAM))")`. Nothing else about the launch changes.

- [ ] **Step 4: `mem` command.**

```bash
# Host memory of this script's emulator (the qemu process started with -port $PORT).
mem() {
  local line
  if [[ "$windows" == 1 ]]; then
    local pwsh="${SYSTEMROOT:-${SystemRoot:-}}"
    if [[ -n "$pwsh" ]]; then pwsh="$(cygpath -u "$pwsh" 2>/dev/null || echo "$pwsh")/System32/WindowsPowerShell/v1.0/powershell.exe"; else pwsh=powershell.exe; fi
    line=$(MSYS_NO_PATHCONV=1 "$pwsh" -NoProfile -NonInteractive -Command "\$c = Get-CimInstance Win32_Process -Filter \"Name like 'qemu%'\" | Where-Object { \$_.CommandLine -match '-port $PORT ' } | Select-Object -First 1; if (\$c) { \$p = Get-Process -Id \$c.ProcessId; '{0} {1}' -f [int](\$p.WorkingSet64/1MB), [int](\$p.PrivateMemorySize64/1MB) } else { 'none' }" 2>/dev/null | tr -d '\r') || true
  else
    line=$(ps -eo rss=,args= 2>/dev/null | awk -v p="-port $PORT " '$2 ~ /qemu-system/ && index($0, p) { printf "%d %d\n", $1/1024, $1/1024; exit }') || true
  fi
  if [[ -z "$line" ]]; then
    echo "emulator $SERIAL host memory: unknown (could not ask the system)"
  elif [[ "$line" == none ]]; then
    echo "emulator $SERIAL is not running"
  else
    echo "emulator $SERIAL host memory: ${line% *} MB working set, ${line#* } MB private"
  fi
}
```

and `mem) mem ;;` in the `case`. (On Linux the second number repeats RSS and `ps` finds nothing in Git Bash: say so in the comment. Three outcomes: not running, unknown, measured.) A `mem` call must not fail the job: callers use `|| true` and the function itself never exits non-zero.

- [ ] **Step 5: Verify** `bash -n scripts/emulator.sh`; `shellcheck scripts/emulator.sh` if installed; `ADBUDDY_EMU_RAM=abc scripts/emulator.sh start` and `ADBUDDY_EMU_RAM=99999 scripts/emulator.sh start` exit 2 before touching adb or the SDK (use the 5999/5099 ports below), and `ADBUDDY_EMU_RAM=abc scripts/emulator.sh stop` exits 0; `scripts/emulator.sh mem` with no emulator on the port prints "is not running" and exits 0 (use `ADBUDDY_EMU_PORT=5999 ADBUDDY_EMU_ADB_PORT=5099` so it cannot match another emulator).

- [ ] **Step 6: Commit** `feat(emulator): ADBUDDY_EMU_RAM in plain MB and a mem command`.

---

### Task 2: `emulator.yml`: two `emutest` groups with a restart between them, memory report

**Files:**
- Modify: `.github/workflows/emulator.yml`

- [ ] **Step 1: Replace the step "The host against the app on the emulator"** (job `mksh`, after "Start the android-34 emulator") by these steps, keeping the existing comments about the variables and `ADBUDDY_EMULATOR_CUT`:

```yaml
      # qemu's memory only grows while the emulator runs (about 4 GB per 10 min of
      # these tests, never returned), so the screen-driven guard tests run first
      # on a fresh emulator and the rest on another fresh one.
      - name: The host against the app on the emulator (guard tests)
        working-directory: host
        env:
          ADBUDDY_EMULATOR_CUT: ""
        run: |
          test -n "$ADBUDDY_EMULATOR_SERIAL" && test -n "$ADBUDDY_EMULATOR_ADB"
          # A split that finds nothing is stale (a renamed test): fail, do not pass silently.
          go test -list '^TestTheGuard' ./internal/emutest | grep -q '^TestTheGuard' || { echo "no guard tests: the split of emutest is stale" >&2; exit 1; }
          go test -timeout 20m -count=1 -v -run '^TestTheGuard' ./internal/emutest
      - name: Emulator memory after the guard tests
        if: always()
        run: scripts/emulator.sh mem || true
      - name: Restart the android-34 emulator
        id: restart34
        shell: bash # with pipefail, like the first start
        env: <copy, literally, the `env:` block of "Start the android-34 emulator" (image, AVD, ports, STABLE_SECS, CUT) and say in a comment above both that they must stay identical>
        run: |
          scripts/emulator.sh stop
          scripts/emulator.sh start | sed "s/^export //; s/'//g" >> "$GITHUB_ENV"
      - name: The host against the app on the emulator (the rest)
        working-directory: host
        env:
          ADBUDDY_EMULATOR_CUT: ""
        run: test -n "$ADBUDDY_EMULATOR_SERIAL" && test -n "$ADBUDDY_EMULATOR_ADB" && go test -timeout 20m -count=1 -v -skip '^TestTheGuard' ./internal/emutest
      - name: Emulator memory after the rest
        if: always()
        run: scripts/emulator.sh mem || true
```

The two env blocks (start and restart) must be identical: say so in a comment, or use a YAML anchor only if the repo already uses them (it does not: copy). Give "Start the android-34 emulator" the `id: start34`. If the guard group fails, the rest still runs (half the signal is better than none): the restart and the rest step get `if: (success() || failure()) && steps.start34.outcome == 'success'` (not `cancelled()`).

- [ ] **Step 2: The `mem` steps need the job's emulator variables.** The job `env` already has `ADBUDDY_EMU_PORT=5590` and `ADBUDDY_EMU_ADB_PORT=5049`; the android-34 start step overrides AVD and image only in its own env, which `mem` does not need (it matches the port).

- [ ] **Step 3: One memory line after the oracle too**, before "Stop the android-37 emulator": step "Emulator memory after the oracle" (`if: always()`, `scripts/emulator.sh mem || true`).

- [ ] **Step 4: Do not set `ADBUDDY_EMU_RAM`** in the workflow (the controller decides it after measuring). Add a comment above the android-34 start env: `# ADBUDDY_EMU_RAM (plain MB) lowers the guest's RAM; see docs/ci-self-hosted.md.`

- [ ] **Step 5: Validate** `actionlint` if installed (no output); check by reading that `-run '^TestTheGuard'` and `-skip '^TestTheGuard'` use the same pattern and that `go.mod`'s Go version supports `-skip` (Go 1.20+: `git grep -n "^go " host/go.mod`).

- [ ] **Step 6: Commit** `ci(emulator): run emutest in two groups with a fresh emulator for each; print the emulator's memory`.

---

### Task 3: `docs/ci-self-hosted.md`

**Files:**
- Modify: `docs/ci-self-hosted.md`

- [ ] **Step 1:** in the section on the emulator job, one short paragraph: qemu's memory only grows while an emulator runs (about 3.6 GB after boot, 7 GB after the full app suite); the job therefore restarts the android-34 emulator between the guard tests and the rest, and prints the memory after each stage (`emulator.sh mem`); `ADBUDDY_EMU_RAM` (plain MB) lowers the guest RAM; do not write `1536M` or `2G` in an AVD's config (the emulator may raise them). Fix the duration sentence about the app's tests (it says "about 15 min (estimated, not measured)"): measured, the tests take about 10 minutes, and the restart adds 4.5-5.5 minutes (boot ~160 s, stability wait 120 s, stop up to 60 s); the job stays under its 90 minutes (about 55-65). Say the "rest" group peaks at ~5 GB, close to the 6 GB target.
- [ ] **Step 1b:** add a row `ADBUDDY_EMU_RAM` to the table of emulator environment variables; extend the exit-code-2 line ("for `start`: invalid `ADBUDDY_EMU_RAM`"); mention the `mem` command; and add one line next to `ADBUDDY_EMU_STABLE_SECS` in `host/README.md`.
- [ ] **Step 1c:** update the comments of `emulator.yml` that no longer fit: the one about the rewrite of `$GITHUB_ENV` (it also happens at the restart) and "Without the variables the tests would skip and the step would pass" (it applies to both test steps).
- [ ] **Step 2:** `go test ./internal/... -run TestDocs` from `host/` passes (no personal paths, no angle-bracket placeholders in prose).
- [ ] **Step 3: Commit** `docs: emulator memory in the self-hosted CI`.

---

## Validaciones manuales pendientes (CLAUDE.md)

Solo se pueden comprobar con el runner de verdad; las hace el controlador, con un emulador a la vez:
1. `-memory 1536` arranca el android-34 y `hardware-qemu.ini` lo refleja (`hw.ramSize`), y el emutest completo pasa; solo entonces se fija `ADBUDDY_EMU_RAM` en el workflow.
2. `scripts/emulator.sh mem` encuentra PowerShell en el runner Windows lanzado con `env -i` (SYSTEMROOT presente) y da una cifra, no "unknown".
3. La primera ejecución del job en CI con los dos grupos: tres líneas de memoria (tras el oráculo, tras el guard, tras el resto), y qemu por debajo de ~6 GB.
4. `mem` en Linux (runner de WSL o hospedado): da una cifra.
