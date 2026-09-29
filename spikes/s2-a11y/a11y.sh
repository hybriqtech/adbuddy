#!/usr/bin/env bash
# Spike S2 (código desechable): guarda, activa, inspecciona y restaura los servicios de accesibilidad del móvil.
set -euo pipefail
source "$(dirname "$0")/../env.sh"
[ -n "$SERIAL" ] || { echo "ERROR: SERIAL vacío (no hay depuración inalámbrica)" >&2; exit 1; }
SNAP="$(dirname "$0")/out/a11y-snapshot.env"
SVC="$PKG_SPIKE/.ProbeService"

sget() { "$ADB" -s "$SERIAL" shell settings get secure "$1" | tr -d '\r'; }
sset() { # sset <clave> <valor|null>: "null" o vacío borran la clave
  if [ "$2" = "null" ] || [ -z "$2" ]; then "$ADB" -s "$SERIAL" shell settings delete secure "$1" >/dev/null
  else "$ADB" -s "$SERIAL" shell settings put secure "$1" "$2"; fi
}

case "${1:-}" in
  snapshot)
    if [ -f "$SNAP" ]; then echo "Ya existe $SNAP; no se sobrescribe:"; cat "$SNAP"; exit 0; fi
    svcs="$(sget enabled_accessibility_services)"
    en="$(sget accessibility_enabled)"
    [ -n "$svcs" ] || { echo "ERROR: lectura vacía de enabled_accessibility_services" >&2; exit 1; }
    case "$en" in null|0|1) ;; *) echo "ERROR: accessibility_enabled inesperado: '$en'" >&2; exit 1 ;; esac
    mkdir -p "$(dirname "$SNAP")"
    printf 'ORIG_SVCS=%q\nORIG_ENABLED=%q\n' "$svcs" "$en" > "$SNAP"
    cat "$SNAP" ;;
  enable)
    [ -f "$SNAP" ] || { echo "ERROR: no hay snapshot en $SNAP" >&2; exit 1; }
    . "$SNAP"
    if [ "$ORIG_SVCS" = "null" ] || [ -z "$ORIG_SVCS" ]; then NEW="$SVC"; else NEW="$ORIG_SVCS:$SVC"; fi
    "$ADB" -s "$SERIAL" logcat -c
    sset enabled_accessibility_services "$NEW"
    sset accessibility_enabled 1
    echo "enabled_accessibility_services=$(sget enabled_accessibility_services)" ;;
  status)
    echo "enabled_accessibility_services=$(sget enabled_accessibility_services)"
    "$ADB" -s "$SERIAL" shell dumpsys accessibility | tr -d '\r' | grep -iE "ProbeService|Bound services|Enabled services" || true
    echo "--- logcat A11yProbe"
    "$ADB" -s "$SERIAL" logcat -d -s A11yProbe:I | tr -d '\r' | tail -20 ;;
  restore)
    [ -f "$SNAP" ] || { echo "ERROR: no hay snapshot en $SNAP" >&2; exit 1; }
    . "$SNAP"
    sset enabled_accessibility_services "$ORIG_SVCS"
    sset accessibility_enabled "$ORIG_ENABLED"
    NOW="$(sget enabled_accessibility_services)"
    NOW_EN="$(sget accessibility_enabled)"
    echo "restaurado: enabled_accessibility_services=$NOW accessibility_enabled=$NOW_EN"
    if [ "$NOW" = "$ORIG_SVCS" ] && [ "$NOW_EN" = "$ORIG_ENABLED" ]; then echo "OK: coincide con el snapshot"; else echo "ATENCIÓN: no coincide con el snapshot"; exit 1; fi ;;
  *) echo "uso: a11y.sh snapshot|enable|status|restore"; exit 2 ;;
esac
