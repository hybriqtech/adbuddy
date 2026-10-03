## Restricciones globales

- **Rama `feat/m2.5b-app-emulador`**, local, sobre `main` 2f3d650. Si `main` avanza antes de la Tarea 1, se rebasa sobre `origin/main` (la rama no se ha subido). Sin push salvo como copia (tras `git grep` de seriales e IPs) y nunca `--force`; a `main` integra el coordinador.
- **Ledger:** `.superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md` (se añade con `git add -f`). Cada decisión, como `Ruling: qué — por qué — qué cuesta si me equivoco`.
- **El móvil del usuario no se toca.** Ningún paso ejecuta nada en un móvil real. Todo lo que habla con un dispositivo lo hace con el número de serie del emulador (`emulator-5600` en desarrollo) y en el servidor adb del emulador (`ANDROID_ADB_SERVER_PORT`, nunca 5037). Gradle se niega a lanzar un test conectado si `ANDROID_SERIAL` no es exactamente un `emulator-<puerto>` (Tarea 2) y `emutest` se niega con otro número de serie o con el servidor 5037.
- **Un emulador a la vez en el PC** (cada uno ocupa ~5 GB de RAM y el runner de la CI corre en este PC). Dos comprobaciones con nombre:
  - **CI-LIBRE:** ningún job del workflow `emulator` (oráculo) en marcha o en cola. Tiene que imprimir `0`:

    ```bash
    GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech" gh run list -R hybriqtech/adbuddy --workflow emulator.yml --limit 20 \
      --json status --jq '[.[] | select(.status != "completed")] | length'
    ```

  - **SIN-EMULADOR:** ningún proceso de emulador en el PC. Tiene que imprimir nada (PowerShell):

    ```powershell
    Get-Process qemu-system-x86_64-headless,emulator -ErrorAction SilentlyContinue | Select-Object Id, ProcessName
    ```

  Antes de arrancar el emulador de desarrollo: CI-LIBRE y SIN-EMULADOR. Antes de cada paso largo con el emulador encendido (Gradle `connected…`, `go test ./internal/emutest`, R3 del spike): CI-LIBRE otra vez. Si no se cumplen, espera (con `Monitor` y un bucle `until`, nunca `sleep` en primer plano) y vuelve a mirar; si un job de la CI empieza con el de desarrollo encendido, páralo y espera a que acabe. Tras cada `stop`, SIN-EMULADOR hasta ~30 s:

  ```powershell
  $end = (Get-Date).AddSeconds(30)
  while ((Get-Process qemu-system-x86_64-headless,emulator -ErrorAction SilentlyContinue) -and (Get-Date) -lt $end) { Start-Sleep -Seconds 1 }
  Get-CimInstance Win32_Process | Where-Object { $_.Name -match '^(qemu-system|emulator)' } | Select-Object ProcessId, Name, CommandLine
  ```

  Si queda algo, actúa **solo** sobre los procesos cuya línea de órdenes contiene `-port 5600` (`Stop-Process -Id <ProcessId>`); cualquier otro (p. ej. `-port 5590`, de la CI) no se toca: detente y avisa.
- **El emulador solo se arranca con `scripts/emulator.sh`:** nunca `emulator.exe` a mano, ni con otro `-gpu` u otras opciones que las del script.
- **Receta del emulador de desarrollo** (desde la raíz del repo; el estado de la shell no persiste entre órdenes, así que cada orden carga el fichero de variables):
  - Arrancar (hasta 10 min; `timeout` amplio o en segundo plano):

    ```bash
    export ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060
    scripts/emulator.sh start > /tmp/adbuddy-m25b-emu.env && cat /tmp/adbuddy-m25b-emu.env
    ```

    La primera vez crea el AVD `adbuddy-dev` (imagen de Android 37, ya instalada). Debe imprimir `ADBUDDY_EMULATOR_SERIAL=emulator-5600` y `ANDROID_ADB_SERVER_PORT=5060`.
  - Preludio de cada orden posterior (en adelante, **`EMU;`**):

    ```bash
    export ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060; . /tmp/adbuddy-m25b-emu.env; export ANDROID_SERIAL="$ADBUDDY_EMULATOR_SERIAL"; ADB="$ADBUDDY_EMULATOR_ADB"; S="$ADBUDDY_EMULATOR_SERIAL"
    ```

  - Parar al terminar cada tarea: `EMU; scripts/emulator.sh stop`.
  - **Recuperación del AVD de desarrollo** (solo si el arranque no llega a `device` porque quedó `adb_enabled=0` en sus datos): con el emulador parado, borra los datos de usuario **de `adbuddy-dev` y de ningún otro AVD**, y arranca de nuevo (el emulador los recrea desde la imagen):

    ```bash
    ls "$HOME/.android/avd/adbuddy-dev.avd/"
    rm -f "$HOME/.android/avd/adbuddy-dev.avd/userdata-qemu.img" "$HOME/.android/avd/adbuddy-dev.avd/userdata-qemu.img.qcow2"
    ```

    Si en ese directorio no están esos ficheros, no borres otros: detente y explícalo.
  - **ADB-ON tras una recuperación.** Cada vez que el emulador vuelve a `device` después de un corte que no deshizo la app (recuperación, S6/S7 del spike, la mutación A5), antes de seguir:

    ```bash
    EMU; "$ADB" -s "$S" shell 'settings put global adb_enabled 1; settings get global adb_enabled'
    ```

    Tiene que imprimir `1`. Un `adb_enabled=0` que se queda en los datos del AVD falsea las medidas siguientes (y en la CI dejaría su AVD sin adb en el siguiente arranque).
  - Nunca toques el AVD `adbuddy-ci-api37`, el puerto 5590 ni el servidor 5049 (son de la CI), ni `adbuddy-api37`/5580/5039 (valores por defecto) ni `adbuddy-api34`.
- **Gradle** (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon` (en los pasos, `gradle`). `android/local.properties` no se versiona; si falta: `printf 'sdk.dir=C\\:/Users/Jose/AppData/Local/Android/Sdk\n' > android/local.properties`.
- **Go:** `go` y `gofmt` del PATH (`/c/Program Files/Go/bin`). Siempre `-count=1` y `-timeout`. Antes de cada commit de `host/`: `gofmt -l .` vacío y `go vet ./...` con `GOOS=windows`, `linux` y `darwin`. Sin `-race` en Windows.
- **Windows y Git Bash:** sin `sleep` en primer plano en el PC (las esperas cortas van dentro del emulador, `adb shell 'sleep N; …'`, o en un script en segundo plano). Una orden que espera dentro del emulador necesita un `timeout` de la herramienta mayor que la espera (p. ej. 300000 ms para `sleep 120`), y Gradle con un dispositivo, unos 900000 ms. Ficheros con LF.
- **Móvil real fuera de alcance (hallazgo del spike, 2026-10-02):** el servidor adb propio del emulador autoconectó por mDNS el móvil real del usuario. Regla de TODOS los preludios, pasos y guardas de Gradle: `ANDROID_SERIAL=emulator-<n>` obligatorio (y `-s emulator-<n>` en cada orden de adb), `ADB_MDNS=0` y `ADB_MDNS_AUTO_CONNECT=0` en el servidor propio, y comprobar antes de cada paso que `adb devices` del servidor propio no lista ningún dispositivo que no sea `emulator-<n>`; si aparece uno, parar y escalar. `scripts/emulator.sh` lo hará él mismo (PR de puffin); el plan no lo da por hecho en los pasos.
- **GPU del emulador (hallazgo del spike):** con la imagen android-37.0 y `swiftshader_indirect`, `surfaceflinger` aborta en bucle (S1). Tras el PR de puffin se reintenta con `ADBUDDY_EMU_GPU=swangle_indirect`; si sigue inestable, escalar al controlador con el log (cambiar de imagen o de GPU en la CI no es de este plan).
- **No cambies la app ni el protocolo** (`android/app/src/main`, `protocol/`), salvo las mutaciones temporales que un paso pide y revierte en el mismo paso. Si un test revela un fallo de la app, detente y repórtalo: el arreglo es otra tarea.
- **No instales paquetes del SDK** ni imágenes de sistema ni nada que pida administrador. Si falta algo, `BLOQUEADO`.
- **Commits:** identidad local del repo (hybriqtech), nunca `git config --global`. Cada mensaje termina con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, exactamente así. `git add` con rutas explícitas, nunca `-A`.
- **Privacidad:** ni seriales ni IPs reales en el repo; `emulator-5600`, `emulator-5590` y `emulator-5580` son los únicos de este plan.
- **Revisores:** no arrancan el emulador; leen la evidencia del informe (órdenes y salidas). Límite de 30 min por revisión y `go test -timeout` en lo que ejecuten. Sin subagentes dentro de una tarea.

### Tarea 3: `host/internal/emutest` — validaciones 1 (lado del shell), 2 y 3

**Implementador:** `adbuddy-implementer` (Sonnet). **Revisor:** `adbuddy-reviewer-opus` (token, handshake y la negativa a tocar un móvil real).

**Ficheros:**
- Crear: `host/internal/emutest/doc.go`, `host/internal/emutest/emulator_test.go`, `host/internal/emutest/app_test.go`
- Modificar: `host/README.md` (sección nueva tras "The skills' read-only examples on the emulator")

**Interfaces:**
- Consume (todas existen en `main`): `adb.Runner{Path, ServerPort, Serial}`, `(adb.Runner).Output(ctx, args...) (stdout, stderr string, code int, err error)`, `adb.ParseDevices(out) []adb.Device`; `phoneapp.Install(ctx, a, path) error`, `phoneapp.NewVerifier(load func() phoneapp.Trust, minCode int) *phoneapp.Verifier`, `(*Verifier).Verify(ctx, a) (phoneapp.Verified, error)`, `phoneapp.Trust{DevAPK: string}`, `phoneapp.MinAppVersionCode`, `phoneapp.Call(ctx, a, method, extras...) (map[string]string, error)`, `phoneapp.Handshake(ctx, a, v, token, port, hostName) error`, `phoneapp.AllowBackground(ctx, a, v) error`, `phoneapp.Package`, `phoneapp.Authority`, `phoneapp.ConsentPending`; `bridge.NewToken() (string, error)`.
- Produce para la Tarea 4: `type emu struct{ ctx context.Context; adb, server adb.Runner }`, `newEmu(t) *emu`, `(e *emu) run(t, args...) string`, `(e *emu) shell(t, command) string`, `(e *emu) state() string` (`"device"`, `"offline"`… o `"gone"`), `(e *emu) waitState(ok func(string) bool, d time.Duration) (string, bool)`, `(e *emu) installApp(t) phoneapp.Verified` y `(e *emu) installTests(t)` (los dos desinstalan al acabar el test, vía `(e *emu) uninstallAtCleanup(t, pkg)`, si el emulador sigue en `device`).

- [ ] **Paso 1: el test sin emulador (RED).** Crea `host/internal/emutest/doc.go`:

  ```go
  // Package emutest holds the host's tests against the Android emulator (M2.5
  // spec §7): they reach the app over adb, as the shell user, the way the host
  // reaches a phone. They skip without ADBUDDY_EMULATOR_SERIAL (scripts/emulator.sh
  // start prints it) and refuse any serial that is not an emulator: they never
  // run on a real phone.
  package emutest
  ```

  y `host/internal/emutest/app_test.go` solo con el test de la lectura de `dumpsys` (el resto llega en el paso 3):

  ```go
  package emutest

  import (
  	"strings"
  	"testing"
  )

  // TestInForeground runs without an emulator: the reading of dumpsys.
  func TestInForeground(t *testing.T) {
  	dump := `ACTIVITY MANAGER SERVICES (dumpsys activity services)
    User 0 active services:
    * ServiceRecord{5c1e2a1 u0 tech.hybriq.adbuddy/.bridge.BridgeService c:tech.hybriq.adbuddy}
      intent={cmp=tech.hybriq.adbuddy/.bridge.BridgeService}
      isForeground=true foregroundId=1 types=0x40000000 foregroundNoti=Notification(channel=conversation)
    * ServiceRecord{77aa001 u0 tech.hybriq.adbuddy/.listener.AgentNotificationListener c:tech.hybriq.adbuddy}
      isForeground=false foregroundId=0
  `
  	if !inForeground(dump, ".bridge.BridgeService") {
  		t.Error("BridgeService in the foreground not seen (Android 13+ record, with c:)")
  	}
  	if old := strings.ReplaceAll(dump, " c:tech.hybriq.adbuddy}", "}"); !inForeground(old, ".bridge.BridgeService") {
  		t.Error("BridgeService in the foreground not seen (record without c:)")
  	}
  	if inForeground(dump, ".bridge.Bridge") {
  		t.Error("a prefix of the service's name read as the service")
  	}
  	if inForeground(dump, ".listener.AgentNotificationListener") {
  		t.Error("a background service read as in the foreground")
  	}
  	if inForeground("  (nothing)\n", ".bridge.BridgeService") {
  		t.Error("no services, yet one in the foreground")
  	}
  	bg := strings.Replace(dump, "isForeground=true", "isForeground=false", 1)
  	if inForeground(bg, ".bridge.BridgeService") {
  		t.Error("isForeground=false read as the foreground")
  	}
  }
  ```

  (El literal empieza por `ACTIVITY` en la columna 0 y sus líneas siguientes van sangradas con espacios, como las imprime `dumpsys`; `gofmt` no toca el interior de un literal. Desde Android 13 la primera línea de cada registro acaba en ` c:<paquete>}`; antes, en `}`: el test cubre las dos.)

- [ ] **Paso 2:** `cd host && go test -count=1 -timeout 120s ./internal/emutest` → falla al compilar con `undefined: inForeground`.

- [ ] **Paso 3: el acceso al emulador y las validaciones.** Crea `host/internal/emutest/emulator_test.go`:

  ```go
  package emutest

  import (
  	"context"
  	"crypto/sha256"
  	"encoding/hex"
  	"os"
  	"path/filepath"
  	"strconv"
  	"strings"
  	"testing"
  	"time"

  	"github.com/hybriqtech/adbuddy/host/internal/adb"
  	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
  )

  // The debug APKs of `gradle :app:assembleDebug :app:assembleDebugAndroidTest`
  // (run from android/), relative to this package.
  var (
  	appAPK   = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "debug", "app-debug.apk")
  	testsAPK = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "androidTest", "debug", "app-debug-androidTest.apk")
  )

  // buildHint says how to get the APKs.
  const buildHint = "build them first: cd android && gradle :app:assembleDebug :app:assembleDebugAndroidTest"

  // emu is the emulator scripts/emulator.sh started, behind its own adb server.
  type emu struct {
  	ctx    context.Context
  	adb    adb.Runner // with Serial: commands for the emulator
  	server adb.Runner // without Serial: `adb devices`
  }

  // newEmu returns the emulator, or skips the test without one. It refuses a
  // serial that is not an emulator and a missing adb server port: nothing here
  // runs on a real phone or on the adb server a phone is on.
  func newEmu(t *testing.T) *emu {
  	t.Helper()
  	serial, path := os.Getenv("ADBUDDY_EMULATOR_SERIAL"), os.Getenv("ADBUDDY_EMULATOR_ADB")
  	if serial == "" || path == "" {
  		t.Skip("set ADBUDDY_EMULATOR_SERIAL and ADBUDDY_EMULATOR_ADB (scripts/emulator.sh start prints them)")
  	}
  	if !strings.HasPrefix(serial, "emulator-") {
  		t.Fatalf("ADBUDDY_EMULATOR_SERIAL=%q is not an emulator: these tests never run on a real phone", serial)
  	}
  	port, err := strconv.Atoi(os.Getenv("ANDROID_ADB_SERVER_PORT"))
  	if err != nil || port <= 0 || port == 5037 {
  		t.Fatalf("ANDROID_ADB_SERVER_PORT=%q: the emulator has an adb server of its own, never adb's default 5037 where a phone may be (scripts/emulator.sh start prints its port)", os.Getenv("ANDROID_ADB_SERVER_PORT"))
  	}
  	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
  	t.Cleanup(cancel)
  	e := &emu{
  		ctx:    ctx,
  		adb:    adb.Runner{Path: path, ServerPort: port, Serial: serial},
  		server: adb.Runner{Path: path, ServerPort: port},
  	}
  	if s := e.state(); s != "device" {
  		t.Fatalf("the emulator %s is %q, not online", serial, s)
  	}
  	return e
  }

  // run runs adb against the emulator and returns its output without CRs; it
  // fails the test if adb fails or exits with an error.
  func (e *emu) run(t *testing.T, args ...string) string {
  	t.Helper()
  	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Minute)
  	defer cancel()
  	out, errOut, code, err := e.adb.Output(ctx, args...)
  	if err != nil || code != 0 {
  		t.Fatalf("adb %q: exit %d, %v: %s %s", args, code, err, strings.TrimSpace(errOut), strings.TrimSpace(out))
  	}
  	return strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
  }

  // shell runs command on the emulator as the shell user.
  func (e *emu) shell(t *testing.T, command string) string {
  	t.Helper()
  	return e.run(t, "shell", command)
  }

  // state is the emulator's state in `adb devices` ("device", "offline"…), or
  // "gone" when its adb server does not list it.
  func (e *emu) state() string {
  	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
  	defer cancel()
  	out, _, code, err := e.server.Output(ctx, "devices")
  	if err != nil || code != 0 {
  		return "gone"
  	}
  	for _, d := range adb.ParseDevices(out) {
  		if d.Serial == e.adb.Serial {
  			return d.State
  		}
  	}
  	return "gone"
  }

  // waitState polls the emulator's state every second until ok accepts it or
  // d passes; it returns the last state seen and whether ok accepted it.
  func (e *emu) waitState(ok func(string) bool, d time.Duration) (string, bool) {
  	deadline := time.Now().Add(d)
  	for {
  		s := e.state()
  		if ok(s) {
  			return s, true
  		}
  		if time.Now().After(deadline) {
  			return s, false
  		}
  		select {
  		case <-e.ctx.Done():
  			return s, false
  		case <-time.After(time.Second):
  		}
  	}
  }

  // readAPK returns the absolute path and the bytes of one of the debug APKs,
  // or fails the test.
  func readAPK(t *testing.T, path string) (string, []byte) {
  	t.Helper()
  	abs, err := filepath.Abs(path)
  	if err != nil {
  		t.Fatal(err)
  	}
  	data, err := os.ReadFile(abs)
  	if err != nil {
  		t.Fatalf("%v; %s", err, buildHint)
  	}
  	return abs, data
  }

  // installApp installs the debug app on a clean slate (the uninstall takes
  // its data, the saved link included) and returns the proof the host's
  // functions ask for, with that exact APK trusted as the development build,
  // as `adbuddy setup --apk` does. The app is never opened: it stays in the
  // background.
  func (e *emu) installApp(t *testing.T) phoneapp.Verified {
  	t.Helper()
  	abs, data := readAPK(t, appAPK)
  	// Not installed is fine: the answer does not matter.
  	_, _, _, _ = e.adb.Output(e.ctx, "uninstall", phoneapp.Package)
  	if err := phoneapp.Install(e.ctx, e.adb, abs); err != nil {
  		t.Fatalf("install %s: %v", abs, err)
  	}
  	e.uninstallAtCleanup(t, phoneapp.Package)
  	sum := sha256.Sum256(data)
  	trust := phoneapp.Trust{DevAPK: hex.EncodeToString(sum[:])}
  	v, err := phoneapp.NewVerifier(func() phoneapp.Trust { return trust }, phoneapp.MinAppVersionCode).Verify(e.ctx, e.adb)
  	if err != nil {
  		t.Fatalf("verify the installed app: %v", err)
  	}
  	return v
  }

  // installTests installs the app's instrumented tests (tech.hybriq.adbuddy.test).
  func (e *emu) installTests(t *testing.T) {
  	t.Helper()
  	abs, _ := readAPK(t, testsAPK)
  	if out := e.run(t, "install", "-r", "--no-incremental", abs); !strings.Contains(out, "Success") {
  		t.Fatalf("install %s: %s", abs, out)
  	}
  	e.uninstallAtCleanup(t, phoneapp.Package+".test")
  }

  // uninstallAtCleanup leaves the emulator as the test found it: the CI keeps
  // its AVD between jobs. Only while the emulator is online (a test that left
  // its adb cut has nothing to clean through it).
  func (e *emu) uninstallAtCleanup(t *testing.T, pkg string) {
  	t.Cleanup(func() {
  		if e.state() != "device" {
  			return
  		}
  		ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
  		defer cancel()
  		_, _, _, _ = e.adb.Output(ctx, "uninstall", pkg)
  	})
  }
  ```

  y sustituye todo `host/internal/emutest/app_test.go` por:

  ```go
  package emutest

  import (
  	"regexp"
  	"strconv"
  	"strings"
  	"testing"
  	"time"

  	"github.com/hybriqtech/adbuddy/host/internal/bridge"
  	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
  )

  // A port nothing listens on: the app keeps trying to connect to it, which is
  // all these tests need.
  const handshakePort = 47123

  var nonceRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

  // Validation 1 of M2, the shell's side: the provider answers the shell user,
  // as adb reaches it. The other side (it refuses everyone else) is
  // TokenProviderTest, instrumented.
  func TestTheShellUserIsAnswered(t *testing.T) {
  	e := newEmu(t)
  	e.installApp(t)
  	res, err := phoneapp.Call(e.ctx, e.adb, "ping")
  	if err != nil {
  		t.Fatalf("ping: %v", err)
  	}
  	if res["ok"] != "true" || res["port"] != "0" {
  		t.Errorf("ping of a fresh install = %v, want ok=true and port=0", res)
  	}
  	res, err = phoneapp.Call(e.ctx, e.adb, "getSetupConsent")
  	if err != nil {
  		t.Fatalf("getSetupConsent: %v", err)
  	}
  	if res["consent"] != string(phoneapp.ConsentPending) {
  		t.Errorf("consent of a fresh install = %q, want pending", res["consent"])
  	}
  }

  // Validation 2 of M2: `content write` with the handshake on stdin reaches the
  // provider's openFile. phoneapp.Handshake only returns nil once ping gives
  // back that handshake's nonce and port; the token never comes back out.
  func TestTheHandshakeOnStdinReachesTheApp(t *testing.T) {
  	e := newEmu(t)
  	v := e.installApp(t)
  	token, err := bridge.NewToken()
  	if err != nil {
  		t.Fatal(err)
  	}
  	if err := phoneapp.Handshake(e.ctx, e.adb, v, token, handshakePort, "emutest"); err != nil {
  		t.Fatalf("handshake: %v", err)
  	}
  	res, err := phoneapp.Call(e.ctx, e.adb, "ping")
  	if err != nil {
  		t.Fatalf("ping: %v", err)
  	}
  	if res["port"] != strconv.Itoa(handshakePort) || !nonceRe.MatchString(res["nonce"]) {
  		t.Errorf("ping after the handshake = %v, want port %d and a nonce", res, handshakePort)
  	}
  	for k, val := range res {
  		if strings.Contains(val, token) {
  			t.Errorf("ping returned the token in %s", k)
  		}
  	}
  	out, _, _, _ := e.adb.Output(e.ctx, "shell", "content read --uri content://"+phoneapp.Authority+"/handshake")
  	if strings.Contains(out, token) {
  		t.Error("the handshake could be read back")
  	}
  }

  // Validation 3 of M2: after the handshake the provider starts BridgeService
  // in the foreground, with the app in the background (never opened) and out
  // of battery optimisation, as `adbuddy setup` leaves it (AllowBackground).
  func TestTheServiceIsInTheForegroundAfterTheHandshake(t *testing.T) {
  	e := newEmu(t)
  	v := e.installApp(t)
  	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
  		t.Fatalf("battery optimisation: %v", err)
  	}
  	if acts := e.shell(t, "dumpsys activity activities"); strings.Contains(acts, phoneapp.Package+"/") {
  		t.Fatal("the app has an activity: it is not in the background")
  	}
  	token, err := bridge.NewToken()
  	if err != nil {
  		t.Fatal(err)
  	}
  	if err := phoneapp.Handshake(e.ctx, e.adb, v, token, handshakePort, "emutest"); err != nil {
  		t.Fatalf("handshake: %v", err)
  	}
  	var last string
  	deadline := time.Now().Add(20 * time.Second)
  	for time.Now().Before(deadline) {
  		last = e.shell(t, "dumpsys activity services "+phoneapp.Package)
  		if inForeground(last, ".bridge.BridgeService") {
  			return
  		}
  		time.Sleep(time.Second)
  	}
  	t.Fatalf("BridgeService is not in the foreground 20 s after the handshake:\n%s", last)
  }

  // inForeground reads `dumpsys activity services <package>`: the record of
  // the service whose short name is svc says isForeground=true. The record's
  // first line ends in "/<svc>}" or, from Android 13, "/<svc> c:<package>}".
  func inForeground(dump, svc string) bool {
  	name := regexp.MustCompile("/" + regexp.QuoteMeta(svc) + "[ }]")
  	recs := strings.Split(dump, "* ServiceRecord{")
  	for _, rec := range recs[1:] {
  		head, _, _ := strings.Cut(rec, "\n")
  		if name.MatchString(head) && strings.Contains(rec, "isForeground=true") {
  			return true
  		}
  	}
  	return false
  }

  // TestInForeground runs without an emulator: the reading of dumpsys.
  func TestInForeground(t *testing.T) {
  	dump := `ACTIVITY MANAGER SERVICES (dumpsys activity services)
    User 0 active services:
    * ServiceRecord{5c1e2a1 u0 tech.hybriq.adbuddy/.bridge.BridgeService c:tech.hybriq.adbuddy}
      intent={cmp=tech.hybriq.adbuddy/.bridge.BridgeService}
      isForeground=true foregroundId=1 types=0x40000000 foregroundNoti=Notification(channel=conversation)
    * ServiceRecord{77aa001 u0 tech.hybriq.adbuddy/.listener.AgentNotificationListener c:tech.hybriq.adbuddy}
      isForeground=false foregroundId=0
  `
  	if !inForeground(dump, ".bridge.BridgeService") {
  		t.Error("BridgeService in the foreground not seen (Android 13+ record, with c:)")
  	}
  	if old := strings.ReplaceAll(dump, " c:tech.hybriq.adbuddy}", "}"); !inForeground(old, ".bridge.BridgeService") {
  		t.Error("BridgeService in the foreground not seen (record without c:)")
  	}
  	if inForeground(dump, ".bridge.Bridge") {
  		t.Error("a prefix of the service's name read as the service")
  	}
  	if inForeground(dump, ".listener.AgentNotificationListener") {
  		t.Error("a background service read as in the foreground")
  	}
  	if inForeground("  (nothing)\n", ".bridge.BridgeService") {
  		t.Error("no services, yet one in the foreground")
  	}
  	bg := strings.Replace(dump, "isForeground=true", "isForeground=false", 1)
  	if inForeground(bg, ".bridge.BridgeService") {
  		t.Error("isForeground=false read as the foreground")
  	}
  }
  ```

  (`time.Sleep` dentro de un test de Go no es el `sleep` de la shell: está permitido.)

- [ ] **Paso 4: sin emulador (GREEN y las negativas).** Desde `host/`:

  ```bash
  gofmt -l ./internal/emutest
  for os in windows linux darwin; do GOOS=$os go vet ./internal/emutest || echo "FAIL $os"; done
  env -u ADBUDDY_EMULATOR_SERIAL -u ADBUDDY_EMULATOR_ADB go test -count=1 -timeout 120s -v ./internal/emutest
  ADBUDDY_EMULATOR_SERIAL=192.0.2.10:5555 ADBUDDY_EMULATOR_ADB=adb go test -count=1 -timeout 60s -run TestTheShellUserIsAnswered ./internal/emutest
  ADBUDDY_EMULATOR_SERIAL=emulator-5600 ADBUDDY_EMULATOR_ADB=adb ANDROID_ADB_SERVER_PORT= go test -count=1 -timeout 60s -run TestTheShellUserIsAnswered ./internal/emutest
  ADBUDDY_EMULATOR_SERIAL=emulator-5600 ADBUDDY_EMULATOR_ADB=adb ANDROID_ADB_SERVER_PORT=5037 go test -count=1 -timeout 60s -run TestTheShellUserIsAnswered ./internal/emutest
  ```

  Esperado: `gofmt` y `vet` sin salida; la tercera, `PASS` con `TestInForeground` verde y los otros tres `SKIP`; la cuarta, `FAIL` con `"192.0.2.10:5555" is not an emulator`; la quinta, `FAIL` con `ANDROID_ADB_SERVER_PORT=""`; la sexta, `FAIL` con `ANDROID_ADB_SERVER_PORT="5037"` y `never adb's default 5037`. Ninguna de las tres últimas llega a ejecutar adb (falla antes).

- [ ] **Paso 5: contra el emulador.** CI-LIBRE y SIN-EMULADOR, arranca el de desarrollo y compila los APK (CI-LIBRE otra vez antes del `go test`):

  ```bash
  cd android && gradle -q :app:assembleDebug :app:assembleDebugAndroidTest && cd ..
  EMU; cd host && go test -count=1 -timeout 15m -v ./internal/emutest
  ```

  Esperado: `PASS` con los cuatro tests verdes (ninguno `SKIP`). Copia al informe el `-v` completo.

  **Si falla `TestTheServiceIsInTheForegroundAfterTheHandshake`** (el servicio no pasa a primer plano): no lo cambies. Es la validación 3 fallando en Android 17 (la alternativa de M2 es que el host lance `am start-foreground-service` tras el handshake, que toca el host y queda fuera de este plan). Detente con `BLOQUEADO`, el `dumpsys` del fallo y `EMU; "$ADB" -s "$S" logcat -d | grep -i -E 'ForegroundService|BridgeService|ADBuddy' | tail -40`. El controlador decide. **Lo mismo si falla `TestTheHandshakeOnStdinReachesTheApp`** (`content write` no llega a `openFile`).

- [ ] **Paso 6: mutación de la validación 2** (que el test ve si el handshake no llega). En `TestTheHandshakeOnStdinReachesTheApp` sustituye temporalmente

  ```go
  	if err := phoneapp.Handshake(e.ctx, e.adb, v, token, handshakePort, "emutest"); err != nil {
  		t.Fatalf("handshake: %v", err)
  	}
  ```

  por `_ = v // mutation: no handshake` y corre (CI-LIBRE) `EMU; cd host && go test -count=1 -timeout 10m -run TestTheHandshakeOnStdinReachesTheApp ./internal/emutest`. Esperado: `FAIL` con `ping after the handshake = map[… port:0 …], want port 47123 and a nonce`. Revierte (`git diff` sin cambios en el test respecto al paso 3). Para el emulador (con la espera de SIN-EMULADOR).

- [ ] **Paso 7: README.** En `host/README.md`, justo antes de `### Claude Code with a real agent`, añade:

  ````text
  ### The app on the emulator

  `internal/emutest` reaches the app over adb, as the host reaches a phone: the provider answers the shell user, the handshake on stdin reaches it, and `BridgeService` goes to the foreground after it (validations 1, 2 and 3 of the M2 plan). It installs the debug APKs, so build them first; with the emulator's variables loaded (`scripts/emulator.sh start`):

  ```sh
  (cd android && gradle :app:assembleDebug :app:assembleDebugAndroidTest)
  cd host && go test -timeout 15m -count=1 ./internal/emutest
  ```

  Without `ADBUDDY_EMULATOR_SERIAL` the tests skip; they refuse a serial that is not `emulator-*` and an `ANDROID_ADB_SERVER_PORT` that is unset or adb's default 5037. They uninstall what they installed. The `emulator` workflow runs them after the app's instrumented tests.

  ````

  (La línea en blanco final separa la sección de la siguiente.)

- [ ] **Paso 8: suite del host** (en segundo plano si hace falta): `cd host && gofmt -l . && go vet ./... && go test -count=1 -timeout 580s ./...` → todo verde (`emutest` salta sin variables).

- [ ] **Paso 9: commit.**

  ```bash
  git add host/internal/emutest/doc.go host/internal/emutest/emulator_test.go host/internal/emutest/app_test.go host/README.md
  git commit -m "test(host): emutest, el host contra la app en el emulador (validaciones 1, 2 y 3 de M2)

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

---

