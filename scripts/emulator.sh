#!/usr/bin/env bash
# Starts and stops the Android emulator the tests use: a real mksh for the
# classifier's differential tests and a phone for the app's tests. It has its
# own adb server (port 5039 by default), so it never touches the one a real
# phone is on. The AVD, the emulator port and the adb server port can be
# overridden with ADBUDDY_EMU_AVD, ADBUDDY_EMU_PORT and ADBUDDY_EMU_ADB_PORT
# (the CI job does, to coexist with an emulator started by hand). The emulator's
# GPU mode can be set with ADBUDDY_EMU_GPU (swiftshader_indirect by default).
# The system image can be set with ADBUDDY_EMU_IMAGE (the android-37.0
# google_apis x86_64 one by default). The AVD is created with that image the
# first time only: to change the image, use another AVD name (ADBUDDY_EMU_AVD).
# After boot, start can watch for ADBUDDY_EMU_STABLE_SECS seconds (0 = off, the
# default) that system_server and surfaceflinger do not restart.
# ADBUDDY_EMU_RAM is the guest's RAM in plain MB (1024-8192, empty = the AVD's
# own), passed to the emulator as -memory. "1536M" or "2G" in the AVD's config
# are not honoured the same way: the emulator may raise them.
# The mem command prints one line with the emulator's working set and private
# memory in MB (or says it is not running or that it could not be measured);
# it is a diagnostic and always exits 0.
#
# Exit codes:
#   1    the SDK or its packages are missing
#   2    incorrect usage, or ADBUDDY_EMU_ADB_PORT or ADBUDDY_EMU_PORT invalid (empty,
#        not a number, outside 1024-65535 for the adb port, or 5037/5038: the adb
#        servers of the user and of ADBuddy); for start, also an invalid
#        ADBUDDY_EMU_RAM
#   3    the emulator's adb server sees another device (or none: it fails closed),
#        or the emulator on the port runs another AVD than ADBUDDY_EMU_AVD
#
# stop kills the emulator and waits (up to 60 s) until it is gone before it
# stops the adb server; it always exits 0.
#   4    the emulator is not stable: system_server or surfaceflinger restarted
#   124  the emulator did not boot in 10 minutes
set -euo pipefail

# The adb server of the tests must never auto-connect a real phone: a phone
# with Wireless debugging on is found over mDNS by any adb server, and every
# adb without -s (or Gradle without ANDROID_SERIAL) would then reach it.
export ADB_MDNS=0
export ADB_MDNS_AUTO_CONNECT=0

AVD="${ADBUDDY_EMU_AVD:-adbuddy-api37}"
IMAGE="${ADBUDDY_EMU_IMAGE:-system-images;android-37.0;google_apis;x86_64}"
PORT="${ADBUDDY_EMU_PORT:-5580}"
# mem puts $PORT in a PowerShell command: it must be a plain number.
if [[ ! "$PORT" =~ ^[0-9]{1,5}$ ]]; then
  echo "ADBUDDY_EMU_PORT='$PORT' is not valid: it must be a port number" >&2
  exit 2
fi
SERIAL="emulator-$PORT"
# The adb server of the script must not be the user's (5037) or ADBuddy's
# (5038), and must be a plain port number: anything else exits with 2.
ADB_PORT="${ADBUDDY_EMU_ADB_PORT-5039}"
if [[ ! "$ADB_PORT" =~ ^[0-9]{1,5}$ ]] || ((10#$ADB_PORT < 1024 || 10#$ADB_PORT > 65535)) ||
  ((10#$ADB_PORT == 5037 || 10#$ADB_PORT == 5038)); then
  echo "ADBUDDY_EMU_ADB_PORT='$ADB_PORT' is not valid: it must be a port in 1024-65535 other than 5037 (the user's adb server) and 5038 (ADBuddy's)" >&2
  exit 2
fi
# adb gives these two priority over the port and they would point at another
# server (the user's): the script must only talk to its own.
unset ADB_SERVER_SOCKET ANDROID_ADB_SERVER_ADDRESS
export ANDROID_ADB_SERVER_PORT="$((10#$ADB_PORT))"

usage() {
  echo "usage: $0 start|stop|env|mem" >&2
  exit 2
}

sdk() {
  local d
  for d in "${ANDROID_HOME:-}" "${ANDROID_SDK_ROOT:-}" "${LOCALAPPDATA:-}/Android/Sdk" "$HOME/Android/Sdk" "$HOME/Library/Android/sdk"; do
    [[ -n "$d" && -d "$d/platform-tools" ]] && { echo "${d//\\//}"; return; }
  done
  echo "Android SDK not found: set ANDROID_HOME" >&2
  exit 1
}

SDK=$(sdk)
EXE=""
BAT=""
# The runner may have no OS variable (env -i), so uname decides too.
windows=0
[[ "${OS:-}" == Windows_NT ]] && windows=1
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) windows=1 ;; esac
if [[ "$windows" == 1 ]]; then EXE=.exe; BAT=.bat; fi
ADB="$SDK/platform-tools/adb$EXE"

print_env() {
  echo "export ADB_MDNS=0"
  echo "export ADB_MDNS_AUTO_CONNECT=0"
  echo "export ANDROID_ADB_SERVER_PORT=$ANDROID_ADB_SERVER_PORT"
  echo "export ADBUDDY_MKSH_SERIAL=$SERIAL"
  echo "export ADBUDDY_MKSH_ADB='$ADB'"
  echo "export ADBUDDY_EMULATOR_SERIAL=$SERIAL"
  echo "export ADBUDDY_EMULATOR_ADB='$ADB'"
}

# The pids of system_server and surfaceflinger, as "pid pid" (empty if adb or
# pidof fails). boot_completed can be 1 on an image whose system restarts every
# few seconds, so this is what tells a stable system from a looping one.
stable_sample() {
  MSYS_NO_PATHCONV=1 "$ADB" -s "$SERIAL" shell pidof system_server surfaceflinger </dev/null 2>/dev/null | tr -d '\r' || true
}

# Exits 4 if, for ADBUDDY_EMU_STABLE_SECS, either pid changes or pidof does
# not return both of them.
wait_stable() {
  local total="${ADBUDDY_EMU_STABLE_SECS:-0}" elapsed=0 first now
  ((total > 0)) || return 0
  first=$(stable_sample)
  while [[ "$(wc -w <<<"$first")" -eq 2 ]]; do
    ((elapsed < total)) || return 0
    sleep 5
    elapsed=$((elapsed + 5))
    now=$(stable_sample)
    [[ "$now" == "$first" ]] || break
  done
  "$ADB" -s "$SERIAL" emu kill >/dev/null 2>&1 || true
  "$ADB" kill-server >/dev/null 2>&1 || true
  echo "the emulator restarts system_server/surfaceflinger: it is not stable (image $IMAGE, gpu ${ADBUDDY_EMU_GPU:-swiftshader_indirect}); try ADBUDDY_EMU_GPU or ADBUDDY_EMU_IMAGE, or set ADBUDDY_EMU_STABLE_SECS=0 to skip this check" >&2
  exit 4
}

start() {
  # Validated here, not at script level: stop and mem must exit 0 whatever the
  # environment says.
  RAM="${ADBUDDY_EMU_RAM:-}"
  if [[ -n "$RAM" ]] && { [[ ! "$RAM" =~ ^[0-9]{3,4}$ ]] || ((10#$RAM < 1024 || 10#$RAM > 8192)); }; then
    echo "ADBUDDY_EMU_RAM='$RAM' is not valid: plain MB between 1024 and 8192" >&2
    exit 2
  fi
  # An old server of our port may have been started with mDNS on; one without
  # it is left alone (it may be serving other work).
  if ! "$ADB" mdns check 2>/dev/null | tr -d '\r' | grep -q 'discovery disabled'; then
    "$ADB" kill-server >/dev/null 2>&1 || true
  fi
  if [[ ! -x "$SDK/emulator/emulator$EXE" || ! -d "$SDK/${IMAGE//;//}" ]]; then
    echo "missing SDK packages; install them with:" >&2
    echo "  sdkmanager emulator '$IMAGE'" >&2
    exit 1
  fi
  if ! "$SDK/emulator/emulator$EXE" -list-avds | tr -d '\r' | grep -qx "$AVD"; then
    echo no | "$SDK/cmdline-tools/latest/bin/avdmanager$BAT" create avd -n "$AVD" -k "$IMAGE" -d pixel_6 >&2
  fi
  # Launch it only if it does not show up in 5 s (the server may have just restarted).
  if ! timeout 5 "$ADB" -s "$SERIAL" wait-for-device >/dev/null 2>&1 </dev/null; then
    # -partition-size: the AVD avdmanager makes on a CI runner has a small data
    # partition, and the oracle pushes some 8000 case files to /data/local/tmp.
    # 2047 MB on Windows: its emulator caps it there and exits on 4096.
    args=(-avd "$AVD" -port "$PORT" -no-window -no-audio -no-boot-anim -no-snapshot)
    if [[ "$windows" == 1 ]]; then args+=(-partition-size 2047); else args+=(-partition-size 4096); fi
    [[ -z "$RAM" ]] || args+=(-memory "$((10#$RAM))")
    args+=(-gpu "${ADBUDDY_EMU_GPU:-swiftshader_indirect}")
    nohup "$SDK/emulator/emulator$EXE" "${args[@]}" >"${TMPDIR:-/tmp}/adbuddy-emulator.log" 2>&1 &
  fi
  if ! MSYS_NO_PATHCONV=1 timeout 600 "$ADB" -s "$SERIAL" wait-for-device shell \
    'while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 1; done' </dev/null; then
    echo "the emulator did not finish booting in 10 minutes; its log: ${TMPDIR:-/tmp}/adbuddy-emulator.log" >&2
    exit 124
  fi
  # The emulator on $SERIAL must run the AVD asked for: one of another AVD still
  # on the port (a stop that did not wait) passes wait-for-device as well. The
  # console answers with the name and then OK.
  local running
  running=$(MSYS_NO_PATHCONV=1 timeout 20 "$ADB" -s "$SERIAL" emu avd name </dev/null 2>/dev/null | tr -d '\r') || true
  running=${running%%$'\n'*}
  if [[ "$running" != "$AVD" ]]; then
    stop
    echo "$SERIAL runs the AVD '${running:-?}', not '$AVD' (an emulator of a previous run still on port $PORT?): refusing to continue" >&2
    exit 3
  fi
  wait_stable
  # Safety net: this adb server must see the emulator and nothing else.
  local line devices other="" seen=""
  devices=$("$ADB" devices | tr -d '\r') || other="(adb devices failed)"
  while IFS= read -r line; do
    [[ -z "$line" || "$line" == "List of devices"* || "$line" == "*"* ]] && continue
    line=${line%%[[:space:]]*}
    if [[ "$line" == "$SERIAL" ]]; then seen=1; else other="${line:-?}"; fi
  done <<<"$devices"
  if [[ -n "$other" || -z "$seen" ]]; then
    "$ADB" -s "$SERIAL" emu kill >/dev/null 2>&1 || true
    "$ADB" kill-server >/dev/null 2>&1 || true
    echo "the emulator's adb server must see $SERIAL and nothing else, but it sees: ${other:-no $SERIAL} (a phone over USB, or an adb connect?): refusing to continue" >&2
    exit 3
  fi
  print_env
}

# True while this adb server lists $SERIAL, in any state (offline too). The
# output is read whole before grep: with pipefail, an early grep -q exit could
# turn a match into a SIGPIPE failure.
serial_listed() {
  local devices
  devices=$("$ADB" devices </dev/null 2>/dev/null | tr -d '\r') || true
  grep -q "^$SERIAL[[:space:]]" <<<"$devices"
}

# True while something listens on the emulator's console port: its process is
# still alive, even when adb no longer lists it (adbd cut, or the server just
# restarted). bash makes the connection itself, so it works in Git Bash too.
console_open() {
  timeout 5 bash -c "exec 3<>/dev/tcp/127.0.0.1/$PORT" </dev/null 2>/dev/null
}

# Kills the emulator and waits, for up to 60 s, until adb no longer lists it
# and its console port is closed; only then the adb server is killed. Without
# the wait a start right after (the CI stops one AVD and starts another on the
# same port) could take the dying emulator for the new one. It exits 0 even
# if the emulator outlives the wait: stop must be safe to run any time.
stop() {
  local waited=0
  while true; do
    "$ADB" -s "$SERIAL" emu kill >/dev/null 2>&1 </dev/null || true
    serial_listed || console_open || break
    if ((waited >= 60)); then
      echo "warning: $SERIAL (console port $PORT) is still alive 60 s after emu kill; stopping its adb server anyway" >&2
      break
    fi
    sleep 2
    waited=$((waited + 2))
  done
  "$ADB" kill-server >/dev/null 2>&1 || true
}

# Host memory of this script's emulator (the qemu process started with -port $PORT).
# On Linux the second number repeats RSS (ps has no private figure), and ps finds
# nothing in Git Bash (hence the PowerShell branch). Three outcomes: not running,
# unknown (the system could not be asked), measured. It never exits non-zero.
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

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  mem) mem ;;
  env) print_env ;;
  *) usage ;;
esac
