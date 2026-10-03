#!/usr/bin/env bash
# usage: measure.sh NAME WIDTH HEIGHT DPI RAM CORES GPU [TESTRUN]
# One emulator, android-34, own AVD (adbuddy-puffin-NAME), console 5620, adb 5080.
# Prints one TSV line: name, boot_s, ws_mb_stable, priv_mb_stable, ws_mb_peak_load, priv_mb_peak_load, ws_mb_end, test_result
set -u
NAME=$1 W=$2 H=$3 DPI=$4 RAM=$5 CORES=$6 GPU=$7 TESTRUN=${8:-TestInstrumentedSuite}
REPO=$(git rev-parse --show-toplevel)
SDK=${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}
export ANDROID_HOME=$SDK ANDROID_SDK_ROOT=$SDK
export JAVA_HOME=${JAVA_HOME:?set JAVA_HOME to a JDK 17}
export PATH="$JAVA_HOME/bin:$PATH"
OUT=${MEASURE_OUT:-$REPO/../.measure}; mkdir -p "$OUT"
export ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD="adbuddy-puffin-$NAME" \
  ADBUDDY_EMU_PORT=5620 ADBUDDY_EMU_ADB_PORT=5080 ADBUDDY_EMU_STABLE_SECS=${STABLE:-120} ADB_MDNS=0 ADB_MDNS_AUTO_CONNECT=0 ADBUDDY_EMU_GPU=$GPU
cd "$REPO"
# own AVD, with the variant's hardware
AVDDIR="$HOME/.android/avd/$ADBUDDY_EMU_AVD.avd"
if [ ! -d "$AVDDIR" ]; then
  echo no | "$SDK/cmdline-tools/latest/bin/avdmanager.bat" create avd -n "$ADBUDDY_EMU_AVD" -k "$ADBUDDY_EMU_IMAGE" -d pixel_6 >/dev/null 2>&1
fi
CFG="$AVDDIR/config.ini"
setk() { grep -q "^$1" "$CFG" && sed -i "s|^$1 *=.*|$1 = $2|" "$CFG" || echo "$1 = $2" >> "$CFG"; }
setk hw.lcd.width "$W"; setk hw.lcd.height "$H"; setk hw.lcd.density "$DPI"
setk hw.ramSize "$RAM"; setk hw.cpu.ncore "$CORES"
mem() { # ws private (MB) of MY qemu (the one on console port 5620)
  powershell -NoProfile -Command "\$c=Get-CimInstance Win32_Process -Filter \"Name like 'qemu%'\" | Where-Object { \$_.CommandLine -match '-port 5620 ' } | Select-Object -First 1; if(\$c){\$p=Get-Process -Id \$c.ProcessId; '{0} {1}' -f [int](\$p.WorkingSet64/1MB),[int](\$p.PrivateMemorySize64/1MB)}else{'0 0'}" | tr -d '
'
}
t0=$(date +%s)
envout=$(scripts/emulator.sh start 2>"$OUT/$NAME.start.err"); rc=$?
eval "$envout"
if [ $rc -ne 0 ]; then
  printf '%s\tSTART_FAILED\t%s\n' "$NAME" "$(tail -1 "$OUT/$NAME.start.err")"
  scripts/emulator.sh stop; exit 1
fi
boot=$(( $(date +%s) - t0 )); eff=$(grep -E "^hw.ramSize" "$AVDDIR/hardware-qemu.ini" | tr -d " ")
read -r ws0 pr0 <<<"$(mem)"
# sample during the load
rm -f "$OUT/$NAME.samples"
( while :; do echo "$(( $(date +%s) - t0 )) $(mem)" >> "$OUT/$NAME.samples"; sleep "${SAMPLE_SECS:-10}"; done ) & SAMP=$!
(cd host && go test -count=1 -v -timeout 25m -run "$TESTRUN" ${GOSKIP:+-skip "$GOSKIP"} ./internal/emutest > "$OUT/$NAME.test.log" 2>&1)
res="pass=$(grep -c -- '--- PASS' "$OUT/$NAME.test.log") fail=$(grep -c -- '--- FAIL' "$OUT/$NAME.test.log") skip=$(grep -c -- '--- SKIP' "$OUT/$NAME.test.log") $(tail -1 "$OUT/$NAME.test.log")"
kill $SAMP 2>/dev/null
read -r wsE prE <<<"$(mem)"
wsP=$(awk '{if($2>m)m=$2}END{print m+0}' "$OUT/$NAME.samples"); prP=$(awk '{if($3>m)m=$3}END{print m+0}' "$OUT/$NAME.samples")
scripts/emulator.sh stop
sleep 5
printf '%s\t%ss\t%s\t%s\t%s\t%s\t%s\t%s\n' "$NAME" "$boot" "$ws0" "$pr0" "$wsP" "$prP" "$wsE" "$res"
