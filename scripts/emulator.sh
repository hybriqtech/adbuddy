#!/usr/bin/env bash
# Starts and stops the Android emulator the tests use: a real mksh for the
# classifier's differential tests and a phone for the app's tests. It has its
# own adb server (port 5039), so it never touches the one a real phone is on.
set -euo pipefail

AVD=adbuddy-api37
IMAGE="system-images;android-37.0;google_apis;x86_64"
PORT=5580
SERIAL="emulator-$PORT"
export ANDROID_ADB_SERVER_PORT=5039

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
[[ "${OS:-}" == Windows_NT ]] && { EXE=.exe; BAT=.bat; }
ADB="$SDK/platform-tools/adb$EXE"

print_env() {
  echo "export ANDROID_ADB_SERVER_PORT=$ANDROID_ADB_SERVER_PORT"
  echo "export ADBUDDY_MKSH_SERIAL=$SERIAL"
  echo "export ADBUDDY_MKSH_ADB='$ADB'"
  echo "export ADBUDDY_EMULATOR_SERIAL=$SERIAL"
  echo "export ADBUDDY_EMULATOR_ADB='$ADB'"
}

start() {
  if [[ ! -x "$SDK/emulator/emulator$EXE" || ! -d "$SDK/system-images/android-37.0/google_apis/x86_64" ]]; then
    echo "missing SDK packages; install them with:" >&2
    echo "  sdkmanager emulator '$IMAGE'" >&2
    exit 1
  fi
  if ! "$SDK/emulator/emulator$EXE" -list-avds | tr -d '\r' | grep -qx "$AVD"; then
    echo no | "$SDK/cmdline-tools/latest/bin/avdmanager$BAT" create avd -n "$AVD" -k "$IMAGE" -d pixel_6 >&2
  fi
  if [[ "$("$ADB" -s "$SERIAL" get-state 2>/dev/null | tr -d '\r')" != device ]]; then
    # -partition-size: the AVD avdmanager makes on a CI runner has a small data
    # partition, and the oracle pushes some 8000 case files to /data/local/tmp.
    # Not on Windows: its emulator caps it at 2047 MB and exits on 4096.
    args=(-avd "$AVD" -port "$PORT" -no-window -no-audio -no-boot-anim -no-snapshot)
    [[ -z "$EXE" ]] && args+=(-partition-size 4096)
    args+=(-gpu swiftshader_indirect)
    nohup "$SDK/emulator/emulator$EXE" "${args[@]}" >"${TMPDIR:-/tmp}/adbuddy-emulator.log" 2>&1 &
  fi
  if ! MSYS_NO_PATHCONV=1 timeout 600 "$ADB" -s "$SERIAL" wait-for-device shell \
    'while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 1; done' </dev/null; then
    echo "the emulator did not finish booting in 10 minutes; its log: ${TMPDIR:-/tmp}/adbuddy-emulator.log" >&2
    exit 124
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
