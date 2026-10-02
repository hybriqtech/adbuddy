#!/usr/bin/env bash
# Starts and stops the Android emulator the tests use: a real mksh for the
# classifier's differential tests and a phone for the app's tests. It has its
# own adb server (port 5039 by default), so it never touches the one a real
# phone is on. The AVD, the emulator port and the adb server port can be
# overridden with ADBUDDY_EMU_AVD, ADBUDDY_EMU_PORT and ADBUDDY_EMU_ADB_PORT
# (the CI job does, to coexist with an emulator started by hand). The emulator's
# GPU mode can be set with ADBUDDY_EMU_GPU (swiftshader_indirect by default).
#
# Exit codes:
#   1    the SDK or its packages are missing
#   2    incorrect usage
#   3    the emulator's adb server sees another device (or none: it fails closed)
#   124  the emulator did not boot in 10 minutes
set -euo pipefail

# The adb server of the tests must never auto-connect a real phone: a phone
# with Wireless debugging on is found over mDNS by any adb server, and every
# adb without -s (or Gradle without ANDROID_SERIAL) would then reach it.
export ADB_MDNS=0
export ADB_MDNS_AUTO_CONNECT=0

AVD="${ADBUDDY_EMU_AVD:-adbuddy-api37}"
IMAGE="system-images;android-37.0;google_apis;x86_64"
PORT="${ADBUDDY_EMU_PORT:-5580}"
SERIAL="emulator-$PORT"
export ANDROID_ADB_SERVER_PORT="${ADBUDDY_EMU_ADB_PORT:-5039}"

usage() {
  echo "usage: $0 start|stop|env" >&2
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

start() {
  # An old server of our port may have been started with mDNS on; one without
  # it is left alone (it may be serving other work).
  if ! "$ADB" mdns check 2>/dev/null | tr -d '\r' | grep -q 'discovery disabled'; then
    "$ADB" kill-server >/dev/null 2>&1 || true
  fi
  if [[ ! -x "$SDK/emulator/emulator$EXE" || ! -d "$SDK/system-images/android-37.0/google_apis/x86_64" ]]; then
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
    args+=(-gpu "${ADBUDDY_EMU_GPU:-swiftshader_indirect}")
    nohup "$SDK/emulator/emulator$EXE" "${args[@]}" >"${TMPDIR:-/tmp}/adbuddy-emulator.log" 2>&1 &
  fi
  if ! MSYS_NO_PATHCONV=1 timeout 600 "$ADB" -s "$SERIAL" wait-for-device shell \
    'while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 1; done' </dev/null; then
    echo "the emulator did not finish booting in 10 minutes; its log: ${TMPDIR:-/tmp}/adbuddy-emulator.log" >&2
    exit 124
  fi
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

stop() {
  "$ADB" -s "$SERIAL" emu kill >/dev/null 2>&1 || true
  "$ADB" kill-server >/dev/null 2>&1 || true
}

case "${1:-}" in
  start) start ;;
  stop) stop ;;
  env) print_env ;;
  *) usage ;;
esac
