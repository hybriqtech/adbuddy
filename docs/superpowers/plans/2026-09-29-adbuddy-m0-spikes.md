# ADBuddy M0 · Spikes S1-S3 · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Objetivo:** despejar los tres riesgos técnicos de la spec antes de escribir código de producto:
- **S1:** modo remoto con `adb tcpip` y Tailscale, y el botón "Cortar acceso".
- **S2:** activar el servicio de accesibilidad por adb en una app instalada fuera de Play.
- **S3:** Claude Code dentro de un pseudoterminal ConPTY controlado desde Go en Windows.

**Arquitectura:** tres experimentos independientes con código desechable en `spikes/`. Cada uno deja un informe en `docs/spikes/` con preguntas, resultados, evidencias e impacto en la spec. Una tarea final propone al usuario los cambios en la spec y los aplica cuando da su visto bueno.

**Stack:**
- adb 37.0.1 y Tailscale 1.102.4;
- Android SDK (build-tools 34.0.0, platform android-30) con JDK 17, sin Gradle;
- Go (instalado con winget) con `github.com/UserExistsError/conpty` v0.1.4;
- Python 3.14 para utilidades;
- Claude Code 2.1.284.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`, sobre todo §3.3, §4.6, §5.1, §9 y §11.

## Restricciones globales

- **Todo lo de `spikes/` es desechable.** No se reutiliza en M1+: el código de producto se reescribe con TDD. Estos spikes no siguen TDD; cada tarea termina con un **resultado registrado** en `docs/spikes/`.
- **El móvil es el personal del usuario:** Pixel 9 Pro XL, Android 17 (SDK 37), adb Wi-Fi 2.0. **Al terminar cada tarea tiene que quedar exactamente como estaba:**
  - sin modo `tcpip`;
  - con los servicios de accesibilidad originales;
  - sin la app del spike;
  - sin `adb reverse` activos;
  - sin capturas guardadas.
- **Los pasos 👤 requieren al usuario con el móvil en la mano.** El ejecutor se detiene, explica qué hay que hacer y espera a que lo confirme.
- **Nada irreversible en el móvil.** La única acción disruptiva prevista es `adb_enabled=0` (Tarea 4), que el usuario revierte a mano y se avisa antes.
- **adb:** `/c/Users/Jose/AppData/Local/Microsoft/WinGet/Packages/Google.PlatformTools_Microsoft.Winget.Source_8wekyb3d8bbwe/platform-tools/adb.exe`
- **Android SDK:** `/c/Users/Jose/AppData/Local/Android/Sdk` (build-tools `34.0.0`, platform `android-30`).
- **JDK:** `/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1`
- **Claude Code:** `C:\Users\Jose\.local\bin\claude.exe`
- **Tailscale:** PC `rtx5080` = `<ip-tailnet-pc>`; móvil `pixel-9-pro-xl` = `<ip-tailnet>`.
- **Shell: Git Bash.**
  - `MSYS_NO_PATHCONV=1` para comandos adb con rutas del móvil (lo exporta `spikes/env.sh`).
  - `cygpath -w` para las rutas que se pasan a herramientas Java o Windows.
  - Para esperar se usa `pause <segundos>` (definida en `env.sh`), no `sleep`.
- **Git:**
  - rama `m0/spikes`;
  - identidad local de hybriqtech (ya configurada en el repo); nunca `git config --global`;
  - cada commit termina con el párrafo `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`;
  - push por `origin` (alias `github.com-videal`).

---

### Tarea 1: Preparación del entorno de spikes

**Ficheros:**
- Crear: `spikes/README.md`, `spikes/env.sh`
- Crear: `docs/spikes/s1-modo-remoto.md`, `docs/spikes/s2-accesibilidad.md`, `docs/spikes/s3-pty-windows.md`
- Modificar: `.gitignore` (añadir las salidas de los spikes)

**Interfaces:**
- Produce: `spikes/env.sh`, que exporta `ADB`, `ANDROID_SDK`, `JDK_HOME_UNIX`, `TS_IP`, `PKG_SPIKE` y `SERIAL`, y define las funciones `serial_wifi`, `probe <serial>`, `pause <segundos>` y `ts`. Todas las tareas siguientes empiezan con `source spikes/env.sh`.

- [ ] **Paso 1: Crear la rama**

```bash
cd /c/Users/Jose/Documents/projects/adbuddy
git checkout -b m0/spikes
```

- [ ] **Paso 2: Crear `spikes/README.md`**

```markdown
# Spikes de M0 (código desechable)

Experimentos para validar los riesgos S1-S3 de la spec
(`docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`, §11).

- Nada de esta carpeta se reutiliza en M1+: el código de producto se reescribe con TDD.
- Los resultados viven en `docs/spikes/`.
- Antes de cada spike: `source spikes/env.sh`.
```

- [ ] **Paso 3: Crear `spikes/env.sh`**

```bash
# Entorno común de los spikes de M0 (código desechable). Uso: source spikes/env.sh
export MSYS_NO_PATHCONV=1
export ADB="/c/Users/Jose/AppData/Local/Microsoft/WinGet/Packages/Google.PlatformTools_Microsoft.Winget.Source_8wekyb3d8bbwe/platform-tools/adb.exe"
export ANDROID_SDK="/c/Users/Jose/AppData/Local/Android/Sdk"
export JDK_HOME_UNIX="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"
export TS_IP="<ip-tailnet>"   # IP de Tailscale del Pixel (tailscale status)
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
```

- [ ] **Paso 4: Añadir las salidas de los spikes a `.gitignore`**

Añade al final de `.gitignore`:

```gitignore

# Spikes de M0 (salidas locales, pueden contener capturas del móvil)
spikes/out/
spikes/**/out/
spikes/s3-pty/work/.claude/
```

- [ ] **Paso 5: Crear `docs/spikes/s1-modo-remoto.md`**

```markdown
# Spike S1 · Modo remoto (`adb tcpip` + Tailscale) y "Cortar acceso"

> Spike desechable. Dispositivo: Pixel 9 Pro XL, Android 17. PC: Windows 11, adb 37.0.1, Tailscale 1.102.4.

## Preguntas

| # | Pregunta | Resultado | Evidencia (sección del registro) |
|---|---|---|---|
| Q1 | ¿`adb tcpip 5555` funciona estando conectado por depuración inalámbrica en Android 17? ¿Se pierde la conexión inalámbrica al hacerlo? | | |
| Q2 | ¿Se puede conectar a `<ip-tailnet>:5555` por Tailscale con el móvil en Wi-Fi? ¿Latencia? | | |
| Q3 | ¿Y con solo datos móviles? ¿Latencia y tiempo de una captura? | | |
| Q4 | ¿El modo `tcpip` sobrevive al cambio Wi-Fi → datos → Wi-Fi? ¿Vuelve sola la depuración inalámbrica (Wi-Fi 2.0)? | | |
| Q5 | ¿`adb_wifi_enabled=0` corta el modo `tcpip`? ¿Y `adb_enabled=0`? | | |
| Q6 | ¿Reactivar la depuración a mano recupera el modo `tcpip`? ¿`adb usb` vuelve al modo normal y recupera la depuración inalámbrica? | | |

## Conclusión e impacto en la spec (§3.3 modo remoto, §4.6 Cortar acceso)

## Registro
```

- [ ] **Paso 6: Crear `docs/spikes/s2-accesibilidad.md`**

```markdown
# Spike S2 · Activar la accesibilidad por adb en una app instalada fuera de Play

> Spike desechable. APK mínimo `tech.hybriq.adbuddy.spike.a11y` con un servicio de accesibilidad que solo escribe en logcat.

## Preguntas

| # | Pregunta | Resultado | Evidencia (sección del registro) |
|---|---|---|---|
| Q1 | Instalado con `adb install`, ¿se activa con `settings put secure enabled_accessibility_services`? | | |
| Q2 | ¿Cuánto tarda leer el árbol de una ventana (ms y nodos) frente a `uiautomator dump`? | | |
| Q3 | Instalado desde el navegador (flujo real del QR 1), ¿qué instalador figura y qué devuelve `appops get … ACCESS_RESTRICTED_SETTINGS`? | | |
| Q4 | En ese caso, ¿se activa por adb pese al ajuste restringido? | | |
| Q5 | Si no, ¿lo desbloquea `appops set … ACCESS_RESTRICTED_SETTINGS allow`? | | |
| Q6 | ¿Aparece algún aviso de Play Protect o de verificación de desarrollador al instalar desde el navegador? | | |

## Conclusión e impacto en la spec (§3.1 onboarding, §9 casos límite, §12 publicación)

## Registro
```

- [ ] **Paso 7: Crear `docs/spikes/s3-pty-windows.md`**

```markdown
# Spike S3 · Claude Code dentro de ConPTY desde Go (Windows)

> Spike desechable. Windows 11, Go (winget), `github.com/UserExistsError/conpty` v0.1.4, Claude Code 2.1.284.

## Resultados

(tabla generada por `go run .`)

## Observaciones

- ¿Diálogo de confianza en la carpeta nueva?
- ¿Aparecen colores (SGR) en la salida?
- ¿Influye la configuración global del usuario (plugins, hooks) en la sesión?
- ¿Algún problema de codificación (ñ, tildes) en la entrada o en la salida?

## Conclusión e impacto en la spec (§5.1 sesión, §5.4 entrada por bracketed paste)
```

- [ ] **Paso 8: Verificar el entorno**

```bash
source spikes/env.sh
echo "SERIAL=$SERIAL"
probe "$SERIAL"
tailscale status | grep -E "pixel-9-pro-xl|rtx5080"
"$ADB" version | head -2
```

Esperado:
- `SERIAL=adb-…._adb-tls-connect._tcp`
- `OK adb-… <n>ms`
- las dos máquinas en `tailscale status`
- `Version 37.0.1-…`

Si `SERIAL` está vacío: 👤 pide al usuario que active la depuración inalámbrica en el móvil y repite.

- [ ] **Paso 9: Commit**

```bash
git add spikes/README.md spikes/env.sh .gitignore docs/spikes/
git commit -m "spike(m0): entorno común y plantillas de resultados" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 2: S1a · `adb tcpip` por Tailscale con el móvil en Wi-Fi

**Ficheros:**
- Modificar: `docs/spikes/s1-modo-remoto.md` (añadir al registro)

**Interfaces:**
- Consume: `spikes/env.sh` (Tarea 1).
- Produce: el móvil en modo `tcpip 5555`, conectado como `<ip-tailnet>:5555` (lo usa la Tarea 3).

- [ ] **Paso 1: Registrar la línea base**

```bash
source spikes/env.sh
LOG=docs/spikes/s1-modo-remoto.md
{
  echo; echo "### Línea base ($(ts))"; echo '```'
  "$ADB" devices -l | tr -d '\r'
  for k in adb_enabled adb_wifi_enabled; do echo "$k=$("$ADB" -s "$SERIAL" shell settings get global $k | tr -d '\r')"; done
  echo "service.adb.tcp.port=$("$ADB" -s "$SERIAL" shell getprop service.adb.tcp.port | tr -d '\r')"
  probe "$SERIAL"
  echo '```'
} | tee -a "$LOG"
```

Esperado: `adb_enabled=1`, `adb_wifi_enabled=1`, `service.adb.tcp.port=` vacío y `OK`.

- [ ] **Paso 2: Activar el modo tcpip y ver qué pasa con la conexión inalámbrica**

```bash
"$ADB" -s "$SERIAL" tcpip 5555
pause 6
{
  echo; echo "### Tras adb tcpip 5555 ($(ts))"; echo '```'
  "$ADB" devices -l | tr -d '\r'
  echo "serial inalámbrico ahora: $(serial_wifi)"
  echo '```'
} | tee -a "$LOG"
```

Esperado: `restarting in TCP mode port: 5555`. Anota si el dispositivo `_adb-tls-connect` desaparece o se reconecta (responde a Q1).

- [ ] **Paso 3: Conectar por Tailscale**

```bash
"$ADB" connect "$TS_IP:5555"
{
  echo; echo "### Conexión por Tailscale con Wi-Fi ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  echo "service.adb.tcp.port=$("$ADB" -s "$TS_IP:5555" shell getprop service.adb.tcp.port | tr -d '\r')"
  tailscale ping --c 3 "$TS_IP" 2>&1 | tail -3
  echo '```'
} | tee -a "$LOG"
```

Esperado: `connected to <ip-tailnet>:5555`, `OK … <n>ms` y `service.adb.tcp.port=5555`. Si el móvil muestra "¿Permitir depuración?", 👤 el usuario acepta, y se anota en el registro.

- [ ] **Paso 4: Medir una captura por Tailscale con Wi-Fi y borrarla**

```bash
mkdir -p spikes/out
t0=$(date +%s%N); "$ADB" -s "$TS_IP:5555" exec-out screencap -p > spikes/out/s1-wifi.png; t1=$(date +%s%N)
echo "- captura Wi-Fi+Tailscale: $(( (t1 - t0) / 1000000 )) ms, $(stat -c %s spikes/out/s1-wifi.png) bytes" | tee -a "$LOG"
rm -f spikes/out/s1-wifi.png
```

- [ ] **Paso 5: Rellenar Q1 y Q2 en la tabla de `docs/spikes/s1-modo-remoto.md`**

Escribe el resultado ("Sí/No" con el dato clave) y la sección del registro que lo demuestra, a partir de las salidas de los pasos 2-4.

---

### Tarea 3: S1b · Solo datos móviles y cambio de red (👤)

**Ficheros:**
- Modificar: `docs/spikes/s1-modo-remoto.md`

**Interfaces:**
- Consume: el móvil en modo `tcpip 5555` (Tarea 2).

- [ ] **Paso 1: 👤 Pedir al usuario que desactive el Wi-Fi del móvil**

Mensaje al usuario: *"Desactiva el Wi-Fi del móvil (deja los datos móviles y Tailscale activos) y avísame."* Espera su confirmación.

- [ ] **Paso 2: Probar la conexión con solo datos móviles**

```bash
source spikes/env.sh
LOG=docs/spikes/s1-modo-remoto.md
pause 15
"$ADB" disconnect "$TS_IP:5555" >/dev/null 2>&1
"$ADB" connect "$TS_IP:5555"
{
  echo; echo "### Solo datos móviles ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  echo "adb_wifi_enabled=$("$ADB" -s "$TS_IP:5555" shell settings get global adb_wifi_enabled | tr -d '\r')"
  echo "service.adb.tcp.port=$("$ADB" -s "$TS_IP:5555" shell getprop service.adb.tcp.port | tr -d '\r')"
  tailscale ping --c 3 "$TS_IP" 2>&1 | tail -3
  echo '```'
} | tee -a "$LOG"
```

Anota si hay `OK` (Q3). Si `probe` da `FAIL` pero `tailscale ping` responde, adbd no escucha en datos móviles o perdió el modo `tcpip` al apagarse el Wi-Fi. Anótalo: es un resultado, no un bloqueo, así que sigue.

- [ ] **Paso 3: Medir una captura con datos móviles (solo si el paso 2 dio OK) y borrarla**

```bash
t0=$(date +%s%N); "$ADB" -s "$TS_IP:5555" exec-out screencap -p > spikes/out/s1-datos.png; t1=$(date +%s%N)
echo "- captura datos móviles+Tailscale: $(( (t1 - t0) / 1000000 )) ms, $(stat -c %s spikes/out/s1-datos.png) bytes" | tee -a "$LOG"
rm -f spikes/out/s1-datos.png
```

- [ ] **Paso 4: 👤 Pedir al usuario que vuelva a activar el Wi-Fi**

Mensaje: *"Vuelve a activar el Wi-Fi y avísame."*

- [ ] **Paso 5: Comprobar qué sobrevive al cambio de red**

```bash
pause 20
"$ADB" connect "$TS_IP:5555"
{
  echo; echo "### De vuelta en Wi-Fi ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  S=$(serial_wifi); echo "serial inalámbrico: ${S:-<ninguno>}"
  [ -n "$S" ] && probe "$S"
  echo '```'
} | tee -a "$LOG"
```

- [ ] **Paso 6: Rellenar Q3 y Q4 en la tabla**

---

### Tarea 4: S1c · "Cortar acceso" y vuelta al modo normal (👤)

**Ficheros:**
- Modificar: `docs/spikes/s1-modo-remoto.md`

**Interfaces:**
- Consume: `spikes/env.sh`. Si el modo `tcpip` se perdió en la Tarea 3, el paso 1 lo reactiva.

- [ ] **Paso 1: Asegurar que el modo tcpip está activo**

```bash
source spikes/env.sh
LOG=docs/spikes/s1-modo-remoto.md
if ! probe "$TS_IP:5555" | grep -q '^OK'; then
  "$ADB" -s "$(serial_wifi)" tcpip 5555; pause 6; "$ADB" connect "$TS_IP:5555"
fi
probe "$TS_IP:5555"
```

Esperado: `OK <ip-tailnet>:5555 …`.

- [ ] **Paso 2: Nivel 1, `adb_wifi_enabled=0`**

```bash
"$ADB" -s "$TS_IP:5555" shell settings put global adb_wifi_enabled 0
pause 5
{
  echo; echo "### Cortar acceso, nivel 1: adb_wifi_enabled=0 ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  echo "serial inalámbrico: $(serial_wifi)"
  echo '```'
} | tee -a "$LOG"
```

Hipótesis: `tcpip` sigue vivo (`OK`) y el serial inalámbrico desaparece.

- [ ] **Paso 3: 👤 Avisar antes del nivel 2**

Mensaje: *"Voy a desactivar la depuración USB (`adb_enabled=0`). Se cortará toda la conexión adb y después tendrás que activar a mano 'Depuración USB' y 'Depuración inalámbrica' en Opciones de desarrollador. ¿Sigo?"* Espera su confirmación.

- [ ] **Paso 4: Nivel 2, `adb_enabled=0`**

```bash
"$ADB" -s "$TS_IP:5555" shell settings put global adb_enabled 0
pause 5
"$ADB" disconnect "$TS_IP:5555" >/dev/null 2>&1
"$ADB" connect "$TS_IP:5555"
{
  echo; echo "### Cortar acceso, nivel 2: adb_enabled=0 ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  echo '```'
} | tee -a "$LOG"
```

Esperado: `FAIL` (no se puede conectar).

- [ ] **Paso 5: 👤 El usuario reactiva la depuración a mano**

Mensaje: *"Activa 'Depuración USB' y 'Depuración inalámbrica' en Opciones de desarrollador y avísame."*

- [ ] **Paso 6: ¿Vuelve el modo tcpip al reactivar?**

```bash
pause 10
"$ADB" connect "$TS_IP:5555"
{
  echo; echo "### Tras reactivar a mano ($(ts))"; echo '```'
  probe "$TS_IP:5555"
  S=$(serial_wifi); echo "serial inalámbrico: ${S:-<ninguno>}"
  [ -n "$S" ] && echo "service.adb.tcp.port=$("$ADB" -s "$S" shell getprop service.adb.tcp.port | tr -d '\r')"
  echo '```'
} | tee -a "$LOG"
```

Si `probe` da `OK`, reactivar vuelve a exponer el puerto 5555. La spec tendría entonces que resetear el puerto al "Reactivar".

- [ ] **Paso 7: Volver al modo normal y comprobar el estado final**

```bash
if probe "$TS_IP:5555" | grep -q '^OK'; then "$ADB" -s "$TS_IP:5555" usb; pause 8; fi
"$ADB" disconnect "$TS_IP:5555" >/dev/null 2>&1
export SERIAL="$(serial_wifi)"
{
  echo; echo "### Estado final ($(ts))"; echo '```'
  "$ADB" devices -l | tr -d '\r'
  echo "service.adb.tcp.port=$("$ADB" -s "$SERIAL" shell getprop service.adb.tcp.port | tr -d '\r')"
  for k in adb_enabled adb_wifi_enabled; do echo "$k=$("$ADB" -s "$SERIAL" shell settings get global $k | tr -d '\r')"; done
  "$ADB" connect "$TS_IP:5555" 2>&1 | tr -d '\r'
  echo '```'
} | tee -a "$LOG"
"$ADB" disconnect "$TS_IP:5555" >/dev/null 2>&1
```

Esperado:
- `service.adb.tcp.port` vacío o `0`;
- `adb_enabled=1` y `adb_wifi_enabled=1`;
- el `adb connect … :5555` final **falla**.

Si `SERIAL` queda vacío, 👤 el usuario apaga y enciende la depuración inalámbrica. Si el puerto 5555 sigue aceptando conexiones, repite `adb -s <ip-tailnet>:5555 usb`. Como último recurso, 👤 pide permiso para reiniciar el móvil (`adb reboot`), que siempre limpia el modo `tcpip`.

- [ ] **Paso 8: Rellenar Q5 y Q6, y escribir la conclusión de S1**

En "Conclusión e impacto en la spec" responde:
1. Si el modo remoto con solo datos móviles es viable tal como describe §3.3, o si se queda en "cualquier Wi-Fi + Tailscale".
2. Qué debe hacer exactamente "Cortar acceso" (§4.6) para cerrar también el modo `tcpip`, y si "Reactivar" tiene que resetear el puerto.
3. Las latencias medidas (red local, Tailscale con Wi-Fi y Tailscale con datos).

- [ ] **Paso 9: Commit**

```bash
git add docs/spikes/s1-modo-remoto.md
git commit -m "spike(s1): modo remoto por tcpip + Tailscale y cortar acceso" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 5: S2 · APK mínimo con servicio de accesibilidad, compilado sin Gradle

**Ficheros:**
- Crear: `spikes/s2-a11y/AndroidManifest.xml`
- Crear: `spikes/s2-a11y/res/values/strings.xml`
- Crear: `spikes/s2-a11y/res/xml/a11y_probe.xml`
- Crear: `spikes/s2-a11y/src/tech/hybriq/adbuddy/spike/a11y/ProbeService.java`
- Crear: `spikes/s2-a11y/build.sh`, `spikes/s2-a11y/a11y.sh`, `spikes/s2-a11y/serve.py`

**Interfaces:**
- Produce:
  - `spikes/s2-a11y/build/s2-a11y.apk` (paquete `tech.hybriq.adbuddy.spike.a11y`, servicio `.ProbeService`, etiqueta de logcat `A11yProbe`);
  - `a11y.sh snapshot|enable|status|restore`;
  - `serve.py`: servidor HTTP en `127.0.0.1:8765` que se apaga solo a los 10 minutos.

- [ ] **Paso 1: `AndroidManifest.xml`**

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="tech.hybriq.adbuddy.spike.a11y"
    android:versionCode="1"
    android:versionName="0.0.1-spike">

    <uses-sdk android:minSdkVersion="30" android:targetSdkVersion="34" />

    <application android:label="@string/app_name" android:allowBackup="false">
        <service
            android:name=".ProbeService"
            android:exported="true"
            android:label="@string/service_label"
            android:permission="android.permission.BIND_ACCESSIBILITY_SERVICE">
            <intent-filter>
                <action android:name="android.accessibilityservice.AccessibilityService" />
            </intent-filter>
            <meta-data
                android:name="android.accessibilityservice"
                android:resource="@xml/a11y_probe" />
        </service>
    </application>
</manifest>
```

- [ ] **Paso 2: Recursos**

`res/values/strings.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">ADBuddy S2 probe</string>
    <string name="service_label">ADBuddy S2 probe</string>
    <string name="service_description">Spike desechable de ADBuddy: comprueba si este servicio se puede activar por adb. Solo escribe en logcat.</string>
</resources>
```

`res/xml/a11y_probe.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<accessibility-service xmlns:android="http://schemas.android.com/apk/res/android"
    android:description="@string/service_description"
    android:accessibilityEventTypes="typeWindowStateChanged"
    android:accessibilityFeedbackType="feedbackGeneric"
    android:notificationTimeout="100"
    android:canRetrieveWindowContent="true" />
```

- [ ] **Paso 3: `ProbeService.java`**

```java
package tech.hybriq.adbuddy.spike.a11y;

import android.accessibilityservice.AccessibilityService;
import android.util.Log;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;

/** Spike desechable: registra en logcat cuándo se conecta y cuánto cuesta leer el árbol de cada ventana. */
public class ProbeService extends AccessibilityService {
    private static final String TAG = "A11yProbe";

    @Override
    protected void onServiceConnected() {
        Log.i(TAG, "CONNECTED");
    }

    @Override
    public void onAccessibilityEvent(AccessibilityEvent event) {
        if (event.getEventType() != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) return;
        long t0 = System.nanoTime();
        AccessibilityNodeInfo root = getRootInActiveWindow();
        int nodes = root == null ? -1 : countNodes(root);
        long ms = (System.nanoTime() - t0) / 1_000_000;
        Log.i(TAG, "WINDOW pkg=" + event.getPackageName() + " nodes=" + nodes + " ms=" + ms);
    }

    private static int countNodes(AccessibilityNodeInfo node) {
        int total = 1;
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i);
            if (child != null) total += countNodes(child);
        }
        return total;
    }

    @Override
    public void onInterrupt() {
        Log.i(TAG, "INTERRUPT");
    }
}
```

- [ ] **Paso 4: `build.sh` (aapt2 + javac + d8 + zipalign + apksigner)**

```bash
#!/usr/bin/env bash
# Compila el APK del spike S2 sin Gradle (código desechable).
set -euo pipefail
cd "$(dirname "$0")"
SDK="/c/Users/Jose/AppData/Local/Android/Sdk"
BT="$SDK/build-tools/34.0.0"
JDK="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"
JAR_W="$(cygpath -w "$SDK/platforms/android-30/android.jar")"
export JAVA_HOME="$(cygpath -w "$JDK")"
export PATH="$JDK/bin:$PATH"

rm -rf build && mkdir -p build/classes build/dex
"$BT/aapt2.exe" compile --dir res -o build/res.zip
"$BT/aapt2.exe" link -o build/base.apk -I "$JAR_W" --manifest AndroidManifest.xml build/res.zip
javac -source 8 -target 8 -bootclasspath "$JAR_W" -d build/classes \
  src/tech/hybriq/adbuddy/spike/a11y/ProbeService.java
"$BT/d8.bat" --release --min-api 30 --lib "$JAR_W" --output build/dex \
  $(find build/classes -name '*.class')

# Añade classes.dex sin tocar resources.arsc (debe seguir sin comprimir para targetSdk >= 30).
cp build/base.apk build/unsigned.apk
python - <<'EOF'
import zipfile
with zipfile.ZipFile("build/unsigned.apk", "a", compression=zipfile.ZIP_DEFLATED) as z:
    z.write("build/dex/classes.dex", "classes.dex")
EOF

"$BT/zipalign.exe" -f -p 4 build/unsigned.apk build/aligned.apk
[ -f spike.jks ] || keytool -genkeypair -keystore spike.jks -storepass spike123 -keypass spike123 \
  -alias spike -keyalg RSA -keysize 2048 -validity 365 -dname "CN=ADBuddy spike"
"$BT/apksigner.bat" sign --ks spike.jks --ks-pass pass:spike123 --out build/s2-a11y.apk build/aligned.apk
"$BT/apksigner.bat" verify build/s2-a11y.apk
echo "APK OK: spikes/s2-a11y/build/s2-a11y.apk"
```

- [ ] **Paso 5: `a11y.sh` (snapshot, activar, estado y restaurar)**

```bash
#!/usr/bin/env bash
# Spike S2 (código desechable): guarda, activa, inspecciona y restaura los servicios de accesibilidad del móvil.
set -euo pipefail
source "$(dirname "$0")/../env.sh"
SNAP="$(dirname "$0")/build/a11y-snapshot.env"
SVC="$PKG_SPIKE/.ProbeService"

sget() { "$ADB" -s "$SERIAL" shell settings get secure "$1" | tr -d '\r'; }
sset() { # sset <clave> <valor|null>: "null" o vacío borran la clave
  if [ "$2" = "null" ] || [ -z "$2" ]; then "$ADB" -s "$SERIAL" shell settings delete secure "$1" >/dev/null
  else "$ADB" -s "$SERIAL" shell settings put secure "$1" "$2"; fi
}

case "${1:-}" in
  snapshot)
    if [ -f "$SNAP" ]; then echo "Ya existe $SNAP; no se sobrescribe:"; cat "$SNAP"; exit 0; fi
    mkdir -p "$(dirname "$SNAP")"
    printf 'ORIG_SVCS=%s\nORIG_ENABLED=%s\n' "$(sget enabled_accessibility_services)" "$(sget accessibility_enabled)" > "$SNAP"
    cat "$SNAP" ;;
  enable)
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
    . "$SNAP"
    sset enabled_accessibility_services "$ORIG_SVCS"
    sset accessibility_enabled "$ORIG_ENABLED"
    NOW="$(sget enabled_accessibility_services)"
    echo "restaurado: enabled_accessibility_services=$NOW accessibility_enabled=$(sget accessibility_enabled)"
    if [ "$NOW" = "$ORIG_SVCS" ]; then echo "OK: coincide con el snapshot"; else echo "ATENCIÓN: no coincide con el snapshot"; exit 1; fi ;;
  *) echo "uso: a11y.sh snapshot|enable|status|restore"; exit 2 ;;
esac
```

- [ ] **Paso 6: `serve.py` (sirve el APK al móvil por `adb reverse` y se apaga solo)**

```python
"""Spike S2 (código desechable): sirve build/ en 127.0.0.1:8765 durante 10 minutos como máximo."""
import functools
import http.server
import pathlib
import threading

ROOT = pathlib.Path(__file__).parent / "build"
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(ROOT))
server = http.server.ThreadingHTTPServer(("127.0.0.1", 8765), handler)
threading.Timer(600, server.shutdown).start()
print("sirviendo", ROOT, "en http://127.0.0.1:8765 (10 min)")
server.serve_forever()
```

- [ ] **Paso 7: Compilar y comprobar el APK**

```bash
chmod +x spikes/s2-a11y/build.sh spikes/s2-a11y/a11y.sh
spikes/s2-a11y/build.sh
"/c/Users/Jose/AppData/Local/Android/Sdk/build-tools/34.0.0/aapt2.exe" dump badging spikes/s2-a11y/build/s2-a11y.apk | head -3
```

Esperado: `Verified using v2 scheme (APK Signature Scheme v2): true` o `APK OK`, y `package: name='tech.hybriq.adbuddy.spike.a11y' versionCode='1' … sdkVersion:'30' targetSdkVersion:'34'`.

Si `aapt2 link` rechaza `build/res.zip`, descomprime los `.flat` (`python -m zipfile -e build/res.zip build/flat`) y pásalos como argumentos: `build/flat/*.flat`.

- [ ] **Paso 8: Commit**

```bash
git add spikes/s2-a11y/
git commit -m "spike(s2): APK mínimo con servicio de accesibilidad compilado sin Gradle" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 6: S2 caso A · Instalado por `adb install` y activado por adb

**Ficheros:**
- Modificar: `docs/spikes/s2-accesibilidad.md`

**Interfaces:**
- Consume: `build/s2-a11y.apk`, `a11y.sh` (Tarea 5).

- [ ] **Paso 1: Snapshot de la accesibilidad actual**

```bash
source spikes/env.sh
LOG=docs/spikes/s2-accesibilidad.md
{ echo; echo "### Snapshot inicial ($(ts))"; echo '```'; spikes/s2-a11y/a11y.sh snapshot; echo '```'; } | tee -a "$LOG"
```

- [ ] **Paso 2: Instalar por adb y ver el origen de la instalación**

```bash
"$ADB" -s "$SERIAL" install -r spikes/s2-a11y/build/s2-a11y.apk
{
  echo; echo "### Caso A: instalado con adb install ($(ts))"; echo '```'
  "$ADB" -s "$SERIAL" shell dumpsys package "$PKG_SPIKE" | tr -d '\r' | grep -iE "installerPackageName|initiatingPackageName|installInitiatingPackageName|packageSource" || true
  echo "ACCESS_RESTRICTED_SETTINGS: $("$ADB" -s "$SERIAL" shell appops get "$PKG_SPIKE" ACCESS_RESTRICTED_SETTINGS | tr -d '\r')"
  echo '```'
} | tee -a "$LOG"
```

Esperado: `Success`.

- [ ] **Paso 3: Activar por adb y comprobar que el servicio se conecta**

```bash
spikes/s2-a11y/a11y.sh enable
pause 3
"$ADB" -s "$SERIAL" shell am start -a android.settings.SETTINGS >/dev/null
pause 3
"$ADB" -s "$SERIAL" shell input keyevent KEYCODE_HOME
pause 2
{ echo; echo "### Caso A: tras activar por adb ($(ts))"; echo '```'; spikes/s2-a11y/a11y.sh status; echo '```'; } | tee -a "$LOG"
```

Esperado si funciona (Q1 = sí):
- `A11yProbe: CONNECTED`;
- `WINDOW pkg=com.android.settings nodes=<n> ms=<t>`;
- el servicio aparece en "Bound services".

- [ ] **Paso 4: Comparar con `uiautomator dump` en la misma pantalla (Q2)**

```bash
"$ADB" -s "$SERIAL" shell am start -a android.settings.SETTINGS >/dev/null
pause 3
t0=$(date +%s%N); "$ADB" -s "$SERIAL" shell uiautomator dump /data/local/tmp/s2.xml >/dev/null; t1=$(date +%s%N)
echo "- uiautomator dump en Ajustes: $(( (t1 - t0) / 1000000 )) ms" | tee -a "$LOG"
"$ADB" -s "$SERIAL" shell rm -f /data/local/tmp/s2.xml
"$ADB" -s "$SERIAL" shell input keyevent KEYCODE_HOME
"$ADB" -s "$SERIAL" logcat -d -s A11yProbe:I | tr -d '\r' | grep WINDOW | tail -3 | tee -a "$LOG"
```

- [ ] **Paso 5: Restaurar y desinstalar**

```bash
{ echo; echo "### Caso A: restauración ($(ts))"; echo '```'; spikes/s2-a11y/a11y.sh restore; "$ADB" -s "$SERIAL" uninstall "$PKG_SPIKE"; echo '```'; } | tee -a "$LOG"
```

Esperado: `OK: coincide con el snapshot` y `Success`.

- [ ] **Paso 6: Rellenar Q1 y Q2 en la tabla**

---

### Tarea 7: S2 caso B · Instalado desde el navegador, el flujo real (👤)

**Ficheros:**
- Modificar: `docs/spikes/s2-accesibilidad.md`

**Interfaces:**
- Consume: `build/s2-a11y.apk`, `a11y.sh` y `serve.py` (Tarea 5), y el snapshot de la Tarea 6.

- [ ] **Paso 1: Servir el APK y abrirlo en el navegador del móvil**

```bash
source spikes/env.sh
LOG=docs/spikes/s2-accesibilidad.md
python spikes/s2-a11y/serve.py &
pause 2
"$ADB" -s "$SERIAL" reverse tcp:8765 tcp:8765
"$ADB" -s "$SERIAL" shell am start -a android.intent.action.VIEW -d "http://127.0.0.1:8765/s2-a11y.apk" >/dev/null
```

- [ ] **Paso 2: 👤 El usuario instala el APK desde el navegador**

Mensaje: *"En el móvil se ha abierto la descarga de 'ADBuddy S2 probe'. Descárgala, ábrela e instálala: permite al navegador instalar apps desconocidas si te lo pide. Apunta cualquier aviso que salga (Play Protect, verificación de desarrollador, espera de 24 h…) y avísame."* Anota en el registro lo que cuente (Q6).

- [ ] **Paso 3: Ver el instalador y el estado del ajuste restringido (Q3)**

```bash
{
  echo; echo "### Caso B: instalado desde el navegador ($(ts))"; echo '```'
  "$ADB" -s "$SERIAL" shell dumpsys package "$PKG_SPIKE" | tr -d '\r' | grep -iE "installerPackageName|initiatingPackageName|installInitiatingPackageName|packageSource" || true
  echo "ACCESS_RESTRICTED_SETTINGS: $("$ADB" -s "$SERIAL" shell appops get "$PKG_SPIKE" ACCESS_RESTRICTED_SETTINGS | tr -d '\r')"
  echo '```'
} | tee -a "$LOG"
```

- [ ] **Paso 4: 👤 (Opcional) Ver el bloqueo en la interfaz**

Mensaje: *"Ve a Ajustes → Accesibilidad → 'ADBuddy S2 probe' e intenta activarlo. ¿Sale el aviso de 'Ajuste restringido'? No lo actives: solo dime qué aparece."*

- [ ] **Paso 5: Activar por adb pese al ajuste restringido (Q4)**

```bash
spikes/s2-a11y/a11y.sh enable
pause 3
"$ADB" -s "$SERIAL" shell am start -a android.settings.SETTINGS >/dev/null
pause 3
"$ADB" -s "$SERIAL" shell input keyevent KEYCODE_HOME
pause 2
{ echo; echo "### Caso B: activado por adb ($(ts))"; echo '```'; spikes/s2-a11y/a11y.sh status; echo '```'; } | tee -a "$LOG"
```

- [ ] **Paso 6: Si no se conectó, probar a desbloquear con appops (Q5)**

Ejecuta este paso solo si en el paso 5 no aparece `CONNECTED`:

```bash
"$ADB" -s "$SERIAL" shell appops set "$PKG_SPIKE" ACCESS_RESTRICTED_SETTINGS allow
spikes/s2-a11y/a11y.sh restore
spikes/s2-a11y/a11y.sh enable
pause 3
"$ADB" -s "$SERIAL" shell am start -a android.settings.SETTINGS >/dev/null
pause 3
"$ADB" -s "$SERIAL" shell input keyevent KEYCODE_HOME
pause 2
{ echo; echo "### Caso B: tras appops ACCESS_RESTRICTED_SETTINGS allow ($(ts))"; echo '```'; spikes/s2-a11y/a11y.sh status; echo '```'; } | tee -a "$LOG"
```

- [ ] **Paso 7: Restaurar, desinstalar y limpiar**

```bash
{
  echo; echo "### Caso B: restauración ($(ts))"; echo '```'
  spikes/s2-a11y/a11y.sh restore
  "$ADB" -s "$SERIAL" uninstall "$PKG_SPIKE"
  "$ADB" -s "$SERIAL" reverse --remove tcp:8765
  echo "reverse activos: [$("$ADB" -s "$SERIAL" reverse --list | tr -d '\r')]"
  echo '```'
} | tee -a "$LOG"
rm -f spikes/s2-a11y/build/a11y-snapshot.env
```

Esperado: `OK: coincide con el snapshot`, `Success` y `reverse activos: []`. El servidor de `serve.py` se apaga solo a los 10 minutos. El usuario puede borrar el APK descargado de su carpeta Descargas; díselo.

- [ ] **Paso 8: Rellenar Q3-Q6 y escribir la conclusión de S2**

En "Conclusión e impacto en la spec" responde:
1. Si el onboarding de §3.1 puede activar la accesibilidad por adb, y con qué instalación.
2. Si hace falta el paso manual de §9 o basta con `appops`.
3. Si la verificación de desarrolladores de Google cambia el orden del onboarding. Contexto: desde el 30 de septiembre de 2026 se aplica en BR, ID, SG y TH, y en 2027 en todo el mundo; las instalaciones por adb están exentas.

- [ ] **Paso 9: Commit**

```bash
git add docs/spikes/s2-accesibilidad.md
git commit -m "spike(s2): activar accesibilidad por adb según el origen de instalación" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 8: S3 · Claude Code dentro de ConPTY desde Go

**Ficheros:**
- Crear: `spikes/s3-pty/main.go`, `spikes/s3-pty/go.mod`, `spikes/s3-pty/go.sum`
- Crear: `spikes/s3-pty/work/CLAUDE.md`
- Modificar: `docs/spikes/s3-pty-windows.md`

**Interfaces:**
- Produce: `go run . -dir work -out out`, que imprime una tabla markdown `| Check | Resultado | Detalle |` con los checks C1-C8, y guarda `out/raw.log` y `out/plain.log`.

- [ ] **Paso 1: Instalar Go**

```bash
winget install --id GoLang.Go -e --accept-source-agreements --accept-package-agreements
"/c/Program Files/Go/bin/go.exe" version
```

Esperado: `go version go1.2x.x windows/amd64`. En el resto de la tarea, `GO="/c/Program Files/Go/bin/go.exe"`, porque el PATH de la sesión no se refresca.

- [ ] **Paso 2: Carpeta de trabajo para Claude**

`spikes/s3-pty/work/CLAUDE.md`:

```markdown
Esta carpeta es un spike desechable de ADBuddy. Responde exactamente lo que se pide, en una sola línea, sin usar herramientas.
```

- [ ] **Paso 3: `main.go`**

```go
// Spike S3 de ADBuddy (código desechable): comprueba si Claude Code funciona dentro
// de un pseudoterminal ConPTY controlado desde Go en Windows.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/UserExistsError/conpty"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// ansiRe quita las secuencias CSI, OSC y de juego de caracteres para buscar texto plano.
var ansiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78]`)

type output struct {
	mu   sync.Mutex
	raw  strings.Builder
	last time.Time
}

func (o *output) Write(p []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.raw.Write(p)
	o.last = time.Now()
}

func (o *output) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.raw.Len()
}

func (o *output) RawFrom(offset int) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.raw.String()
	if offset > len(s) {
		offset = len(s)
	}
	return s[offset:]
}

func (o *output) PlainFrom(offset int) string {
	return ansiRe.ReplaceAllString(o.RawFrom(offset), "")
}

func (o *output) Idle() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.last.IsZero() {
		return 0
	}
	return time.Since(o.last)
}

func waitUntil(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return cond()
}

type spike struct {
	cpty *conpty.ConPty
	out  *output
}

func (s *spike) send(text string) {
	if _, err := s.cpty.Write([]byte(text)); err != nil {
		fmt.Fprintf(os.Stderr, "error al escribir: %v\n", err)
	}
}

// submit pega el texto con bracketed paste y pulsa Enter, igual que hará el host de ADBuddy.
func (s *spike) submit(text string) {
	s.send(pasteStart + text + pasteEnd)
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
}

// settle espera a que la salida lleve 3 s quieta (60 s como máximo).
func (s *spike) settle() {
	waitUntil(60*time.Second, func() bool { return s.out.Idle() > 3*time.Second })
}

// ask envía un prompt y comprueba que la respuesta contiene want (que no aparece en el prompt).
func (s *spike) ask(name, prompt, want string) {
	offset := s.out.Len()
	s.submit(prompt)
	ok := waitUntil(180*time.Second, func() bool { return strings.Contains(s.out.PlainFrom(offset), want) })
	report(name, ok, fmt.Sprintf("esperado %q", want))
	s.settle()
}

func report(name string, ok bool, detail string) {
	status := "FAIL"
	if ok {
		status = "PASS"
	}
	fmt.Printf("| %s | %s | %s |\n", name, status, detail)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func main() {
	claude := flag.String("claude", `C:\Users\Jose\.local\bin\claude.exe`, "ruta de claude.exe")
	workDir := flag.String("dir", "work", "carpeta de trabajo de la sesión")
	outDir := flag.String("out", "out", "carpeta para los logs")
	flag.Parse()

	absWork, err := filepath.Abs(*workDir)
	must(err)
	must(os.MkdirAll(*outDir, 0o755))
	if !conpty.IsConPtyAvailable() {
		fmt.Println("ConPTY no está disponible en este Windows")
		os.Exit(1)
	}

	cpty, err := conpty.Start(*claude, conpty.ConPtyDimensions(120, 40), conpty.ConPtyWorkDir(absWork))
	must(err)
	defer cpty.Close()

	out := &output{}
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := cpty.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	s := &spike{cpty: cpty, out: out}

	fmt.Println("| Check | Resultado | Detalle |")
	fmt.Println("|---|---|---|")

	// C1: arranque y posible diálogo de confianza de la carpeta.
	started := waitUntil(45*time.Second, func() bool { return out.Len() > 0 && out.Idle() > 2*time.Second })
	trust := strings.Contains(strings.ToLower(out.PlainFrom(0)), "trust")
	if trust {
		s.send("\r")
		s.settle()
	}
	report("C1 arranque", started, fmt.Sprintf("%d bytes; diálogo de confianza: %v", out.Len(), trust))

	// C2: ¿activa Claude Code el modo bracketed paste?
	report("C2 bracketed paste activo", strings.Contains(out.RawFrom(0), "\x1b[?2004h"), "busca ESC[?2004h")

	// C3: una línea con Unicode en la entrada y en la salida.
	s.ask("C3 una línea + Unicode",
		"Responde únicamente con la palabra ñandú en mayúsculas seguida del resultado de 6*7, sin espacios ni nada más.",
		"ÑANDÚ42")

	// C4: varias líneas pegadas de una vez deben llegar como un solo mensaje.
	s.ask("C4 multilínea",
		"Cuenta las líneas de este mensaje.\nEsta es la segunda línea.\nY esta es la última. Responde solo con el número, escrito con letras mayúsculas.",
		"TRES")

	// C5: redimensionar y seguir funcionando.
	must(cpty.Resize(80, 30))
	s.settle()
	s.ask("C5 tras redimensionar a 80x30",
		"Responde únicamente con el color del cielo despejado, en español y en mayúsculas.",
		"AZUL")

	// C6: Esc interrumpe una respuesta larga.
	offset := out.Len()
	s.submit("Escribe los números del 1 al 2000, uno por línea, sin nada más.")
	time.Sleep(6 * time.Second)
	s.send("\x1b")
	interrupted := waitUntil(20*time.Second, func() bool {
		return strings.Contains(strings.ToLower(out.PlainFrom(offset)), "interrupted")
	})
	report("C6 Esc interrumpe", interrupted, "busca 'Interrupted' tras enviar Esc")
	s.settle()

	// C7: colores en la salida (SGR de 256 colores o truecolor).
	raw := out.RawFrom(0)
	report("C7 colores", strings.Contains(raw, "\x1b[38;5;") || strings.Contains(raw, "\x1b[38;2;"), "busca ESC[38;5; o ESC[38;2;")

	// C8: /exit termina el proceso limpiamente.
	s.send("/exit")
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, werr := cpty.Wait(ctx)
	report("C8 /exit termina", werr == nil, fmt.Sprintf("código %d, error %v", code, werr))

	must(os.WriteFile(filepath.Join(*outDir, "raw.log"), []byte(out.RawFrom(0)), 0o644))
	must(os.WriteFile(filepath.Join(*outDir, "plain.log"), []byte(out.PlainFrom(0)), 0o644))
}
```

- [ ] **Paso 4: Módulo y dependencias**

```bash
cd /c/Users/Jose/Documents/projects/adbuddy/spikes/s3-pty
GO="/c/Program Files/Go/bin/go.exe"
"$GO" mod init github.com/hybriqtech/adbuddy/spikes/s3-pty
"$GO" get github.com/UserExistsError/conpty@v0.1.4
"$GO" mod tidy
"$GO" vet ./...
```

Esperado: `go vet` sin errores. Si alguna firma no coincide, consúltala con `"$GO" doc github.com/UserExistsError/conpty`. Las verificadas para v0.1.4 son: `Start(string, ...ConPtyOption) (*ConPty, error)`, `ConPtyDimensions(w, h int)`, `ConPtyWorkDir(string)`, `Resize(w, h int) error`, `Wait(ctx) (uint32, error)`, `Read`, `Write`, `Close` e `IsConPtyAvailable() bool`.

- [ ] **Paso 5: Ejecutar y registrar la tabla**

```bash
"$GO" run . -dir work -out out | tee -a ../../docs/spikes/s3-pty-windows.md
```

Esperado: una tabla con C1-C8, que se añade al informe. Tarda unos minutos, porque son cuatro respuestas de Claude Code. El coste de uso es mínimo.

- [ ] **Paso 6: Revisar los logs y completar las observaciones**

```bash
grep -c . out/plain.log
grep -n -i -E "trust|interrupted|ñandú|error" out/plain.log | head -20
```

Completa "Observaciones" en `docs/spikes/s3-pty-windows.md`:
- si salió el diálogo de confianza (dato para §5.1: el host tendrá que preconfiar el entorno);
- si los hooks o plugins globales del usuario aparecieron en la sesión (busca en `plain.log` textos de SessionStart o de skills);
- problemas de codificación.

**No subas** `out/`, que está en `.gitignore` y puede contener texto de la sesión.

- [ ] **Paso 7: Conclusión de S3**

Si C1, C3, C4, C5, C6 y C8 dan PASS, ConPTY desde Go es viable para §5.1. Si falla C4 (multilínea), la entrada del chat tendrá que enviar las líneas con otro mecanismo. Anota cuál funcionaría viendo `raw.log`: si no aparece `ESC[?2004h`, prueba en una ejecución manual a enviar las líneas separadas con `\x1b\r` (Alt+Enter). Si fallan C1 o C8, evalúa la alternativa de §11: un proceso auxiliar con `node-pty` (Node 24 ya está instalado).

- [ ] **Paso 8: Commit**

```bash
cd /c/Users/Jose/Documents/projects/adbuddy
git add spikes/s3-pty/main.go spikes/s3-pty/go.mod spikes/s3-pty/go.sum spikes/s3-pty/work/CLAUDE.md docs/spikes/s3-pty-windows.md
git commit -m "spike(s3): Claude Code dentro de ConPTY desde Go en Windows" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 9: Consolidación · Proponer los cambios en la spec y aplicarlos con el visto bueno del usuario

**Ficheros:**
- Modificar: `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§11 y las secciones que afecten los resultados)

**Interfaces:**
- Consume: los tres informes de `docs/spikes/`.

- [ ] **Paso 1: Comprobar que el móvil está como al principio**

```bash
source spikes/env.sh
"$ADB" devices -l | tr -d '\r'
echo "tcp.port=$("$ADB" -s "$SERIAL" shell getprop service.adb.tcp.port | tr -d '\r')"
echo "a11y=$("$ADB" -s "$SERIAL" shell settings get secure enabled_accessibility_services | tr -d '\r')"
"$ADB" -s "$SERIAL" shell pm list packages "$PKG_SPIKE" | tr -d '\r'
echo "reverse=[$("$ADB" -s "$SERIAL" reverse --list | tr -d '\r')]"
```

Esperado:
- solo el dispositivo `_adb-tls-connect`;
- `tcp.port` vacío o `0`;
- `a11y` igual que el snapshot original de la Tarea 6 (anotado en el registro de S2);
- ningún paquete del spike;
- `reverse=[]`.

- [ ] **Paso 2: Redactar la propuesta de cambios**

Para cada spike, a partir de su "Conclusión e impacto en la spec", prepara una lista de cambios concretos: qué sección, qué texto sale y qué texto entra. Incluye siempre esta propuesta, sea cual sea el resultado de S2: **§3.1**, que la instalación de la app por adb desde el host pase de alternativa a vía principal, porque está exenta de la verificación de desarrolladores de Google; y **§12**, decidir si hybriqtech se registra como desarrollador verificado antes de la release pública.

- [ ] **Paso 3: 👤 Presentar la propuesta al usuario y esperar su decisión**

Muestra un resumen de los resultados de S1, S2 y S3 con los cambios propuestos. **No edites la spec hasta que el usuario lo apruebe.**

- [ ] **Paso 4: Aplicar los cambios aprobados**

En §11 añade la columna "Resultado (2026-09-29)" con el veredicto de cada spike y el enlace a su informe (`docs/spikes/…`). Aplica en las demás secciones solo los cambios aprobados en el paso 3.

- [ ] **Paso 5: Commit y push**

```bash
git add docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md
git commit -m "docs(spec): resultados de los spikes de M0" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push -u origin m0/spikes
```

- [ ] **Paso 6: 👤 Preguntar al usuario si se integra en `main`**

Con su visto bueno:

```bash
git checkout main
git merge --no-ff m0/spikes -m "Merge M0: spikes S1-S3" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push origin main
```
