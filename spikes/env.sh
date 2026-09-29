# Entorno común de los spikes de M0 (código desechable). Uso: source spikes/env.sh
export MSYS_NO_PATHCONV=1
export ADB="/c/Users/Jose/AppData/Local/Microsoft/WinGet/Packages/Google.PlatformTools_Microsoft.Winget.Source_8wekyb3d8bbwe/platform-tools/adb.exe"
export ANDROID_SDK="/c/Users/Jose/AppData/Local/Android/Sdk"
export JDK_HOME_UNIX="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"
export TS_IP="100.84.27.94"   # IP de Tailscale del Pixel (tailscale status)
export PKG_SPIKE="tech.hybriq.adbuddy.spike.a11y"

# Serial mDNS de la depuración inalámbrica (vacío si no está conectado).
serial_wifi() { "$ADB" devices | tr -d '\r' | awk '/_adb-tls-connect/ {print $1; exit}'; }

# probe <serial>: imprime "OK <serial> <ms>ms" o "FAIL <serial>: <error>".
probe() {
  local s="$1" t0 t1 out
  t0=$(date +%s%N)
  out=$("$ADB" -s "$s" shell echo ok 2>&1 | tr -d '\r')
  t1=$(date +%s%N)
  if [ "$out" = "ok" ]; then echo "OK $s $(( (t1 - t0) / 1000000 ))ms"; else echo "FAIL $s: $out"; fi
}

pause() { python -c "import sys, time; time.sleep(float(sys.argv[1]))" "$1"; }
ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }

export SERIAL="$(serial_wifi)"
[ -n "$SERIAL" ] || echo "AVISO: no hay dispositivo por depuración inalámbrica (adb devices)" >&2
