# ADBuddy · T8 del inventario de validaciones · adb, modo remoto y release en la API 34 · Plan de implementación

> **Plan v2:** incorpora la revisión opus del plan (`.superpowers/sdd/2026-10-03-adbuddy-validaciones-t8/plan-review.md`: 0 críticos, 7 importantes, 7 menores) y las decisiones del controlador: I1 `sameAdbd` muestrea 5 s; I2 los tests que reinician adbd van detrás de `ADBUDDY_EMULATOR_ADBD=1` y se dice qué corre en la CI; I3 M5b-13 medido con honestidad (respuesta perdida de verdad, RED en el producto `RemoteCard.kt:69`, pendiente de ejecutar); I4 el reinicio de VB-41 entra en la ejecución prevista; I5 la Tarea 4 solo marca "hecha" lo que tenga GREEN y RED en el ledger, con filas completas en dos variantes; I6 la única RED de producto de M6a-2 es la del shrink; I7 fuera `TestTheHostRejectsTheSection5Attacks` (lo cubre `phoneapp/apksig_test.go`); menores 1-7.
>
> **Adenda v3 (ruling del supervisor sobre VB-41):** nueva **Tarea 5**, tras la 4, con un test que **afirma** (no solo registra) que el "off" del host cierra también un puerto de `persist.adb.tcp.port`; RED esperado hoy, y la forma del arreglo para un plan aparte. Las Tareas 1-4 y su código (ya implementado: 35a625d, 2731594, a075578, 5e30c1e) no cambian.
>
> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementador y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`). Las tres tareas de código tocan el modo remoto y la confianza de la app (🔒): las revisa `adbuddy-reviewer-opus`; la tarea de docs, `adbuddy-reviewer`. Nunca Haiku.

**Meta:** cerrar en el emulador propio API 34 la tarea T8 del inventario (`docs/superpowers/plans/2026-10-03-validaciones-manuales.md`): las filas **M5b-12, M5b-13, VB-37, VB-41** (modo remoto y puertos de adbd) y **M6a-2, M6a-3** (la app de release con R8 y lo que la spec M6a da por hecho de Android). Son tests de host en `host/internal/emutest/` (`remote_test.go`, `release_test.go`, `install_test.go`) que hacen de `serve` sobre el adbd real del emulador, y una tarea de docs que pasa a "Automatizado" en el inventario **solo lo que tenga evidencia** GREEN y RED en el ledger. **No se toca código de producción.** Si un test descubre un defecto, se registra como hallazgo para el controlador ("Hallazgos para el controlador"); no se arregla aquí.

**Arquitectura.** Los tests reutilizan el arnés de `host/internal/emutest/` y no duplican nada:

- `emulator_test.go`: `newEmu(t)` (salta sin `ADBUDDY_EMULATOR_SERIAL`, rechaza todo serial que no sea `emulator-*` y los puertos 5037/5038), `e.run`/`e.shell`/`e.state`/`e.waitState`, `installApp`, `uninstallAtCleanup`, `readAPK`, `appAPK`.
- `link_test.go` (T3): `openLink(t)` abre el canal real (bridge en 127.0.0.1, `adb reverse`, `phoneapp.Handshake`) y da `send`/`publishRID`/`waitFor`/`waitHellos`/`Hellos`/`uiRequest`, y para la pantalla `e.screen`/`e.waitNode`/`e.tap`/`l.openChat`/`poll`/`payload`/`notificationRecord`/`atCleanup`, además de `pidRe`. Lo único que cambia es un refactor de `openLink` en `openLink` + `linkTo` (Tarea 1, Step 1) para enlazar un APK que no sea el de debug; la firma de `openLink` y sus 12 llamadas no cambian.

Cada test habla con el adbd del emulador **directamente, como `serve`**, nunca por el shim. El controlador del modo remoto es el de producción (`remote.New(e.adb, store, clock)`) con un `Store` y un reloj en memoria de los tests: `remote.go` no se toca y se ejercita su comportamiento real. Los puertos se leen de **`service.adb.tcp.port`**, nunca del socket: el emulador escucha siempre en el 5555 del guest (su propio transporte, M2.5b S5), así que el socket no distingue el modo tcpip. Para saber si una orden reinició adbd (`adb tcpip` y `adb usb` lo hacen; el emulador sale de `device` ~2 s) se compara el **pid de adbd**; y para afirmar que **no** se reinició, se vigila el pid **5 s** (adb usb contesta antes de que adbd se reinicie).

El APK de release se construye con Gradle **offline** (R8 y shrink, como `release-check.yml`) y se firma con una **clave de usar y tirar** creada en `t.TempDir()` con el `keytool` del JDK (RSA 2048, un día), como el job `sign` de `release-check.yml`: **nunca la clave de release, nunca el keystore ni los secretos**. El host lee quién firmó con su propio parser (`phoneapp.Signers`/`CheckSigner`) y verifica la app instalada con `phoneapp.Verifier`, confiando en el certificado de esa clave temporal como si fuera el de release solo dentro del test.

**Tecnologías:** Go 1.27 (`host/`). Nada nuevo en `go.mod`. Android SDK build-tools (aapt2, zipalign, apksigner) y un JDK 17 (keytool, java) para lo que firma o compila APK; sin ellos esos tests **se saltan** con un mensaje que dice qué falta.

**Fuentes:** encargo `.superpowers/sdd/2026-10-03-adbuddy-validaciones-t8/plan-brief.md` · revisión `.superpowers/sdd/2026-10-03-adbuddy-validaciones-t8/plan-review.md` · inventario `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` ("Datos medidos", T8, filas M5b-12/13, VB-37, VB-41, M6a-2/3) · spec M6a `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` (§5, §6, §9, §11) · `docs/releasing.md` · `.github/workflows/release-check.yml` y `.github/workflows/emulator.yml` · plan de M5b (validaciones 12-13 y ruling 3) · diferidos B (validaciones 37 y 41) · `host/internal/remote/remote.go`, `host/internal/device/device.go`, `host/internal/emutest/cut_test.go`, `android/.../ui/RemoteCard.kt`.

**Verificación del plan (réplica acumulada).** La v2 se aplicó desde el texto de este plan sobre un `git archive` limpio de `origin/main` (2b8c07d; `git fetch` hecho, `main` no había avanzado) en el scratchpad, **tarea a tarea** (tras T1, tras T1+T2, tras T1+T2+T3): en cada etapa `GOOS=windows go build ./...`, `gofmt -l internal/emutest` vacío, `GOOS=windows go vet ./internal/emutest/...` y `go test -count=1 -timeout 5m ./internal/emutest/` en `ok` (los tests que piden emulador, `SKIP`). Con los APK construidos (Gradle offline, `:app:assembleDebug :app:assembleRelease`), `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey` pasa; la RED del shrink (`isMinifyEnabled = false`) se vio fallar en la réplica de la v1 (salida en "Réplica acumulada"). El APK de conflicto (aapt2 link, zipalign, apksigner con clave temporal) se construyó y firmó. **Nada se ejecutó con el emulador:** las ejecuciones (GREEN, RED, el reinicio de VB-41) quedan descritas en cada tarea, para cuando el controlador diga que hay emulador libre.

## Global Constraints

- Rama `test/validaciones-t8`, desde `origin/main` 2b8c07d. Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify` ni `--force`; sin `git stash` (un commit WIP si hace falta apartar algo). Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Sin IPs RFC 1918** ni de Tailscale reales: la única IP es `100.64.0.5` (de documentación; el controlador del modo remoto solo la necesita para no negarse con `ErrNoTailscale`, nunca se conecta a ella). Sin seriales reales: el `emulator-<puerto>` que da `scripts/emulator.sh`.
- **El móvil del usuario no se toca.** Todo corre contra el AVD `adbuddy-dev34` y su servidor adb propio, nunca el 5037/5038; la guarda de `emutest` (`checkEmuEnv`) lo impone.
- **NUNCA matar procesos por nombre de imagen.** Prohibido `taskkill /IM`, `Stop-Process -Name`, `pkill`/`killall` por nombre (`java`, `adb`, `emulator`, `qemu-system-*`, `node`, `python`…): apagarían el emulador de otra sesión o el adb del móvil del usuario. Solo se mata por **PID que hayas lanzado tú y comprobado** con CIM/`CommandLine` (PowerShell: `Get-CimInstance Win32_Process -Filter "ProcessId=<pid>" | Select-Object CommandLine`), o se apaga el emulador por su serial con `scripts/emulator.sh stop` (que hace `adb -s <serial> emu kill`). Ningún test de este plan mata procesos: solo dejan adbd como estaba (`adb usb`, `persist.adb.tcp.port` vacío).
- **Reglas de emulador (del supervisor).** UN solo emulador a la vez en el PC. AVD propio `adbuddy-dev34` (`system-images;android-34;google_apis;x86_64`); consola **≥ 5630**; servidor adb propio en el **5090** (nunca 5037/5038); `ADB_MDNS=0`. Arrancar solo cuando skimmer, puffin o tern avisen de que está libre y haya **≥ 12 GB de RAM** libres. **Apagar al terminar** con `scripts/emulator.sh stop` (mismas variables) y comprobar con `tasklist /FI "PID eq <pid>"` (el PID del emulador que lanzaste) que ya no está. **No hace falta arrancar el emulador para escribir ni replicar el plan.**
- **Variables de entorno de las ejecuciones con emulador** (bloque que repiten las tareas):
  ```bash
  export ADBUDDY_EMU_AVD=adbuddy-dev34 ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' \
         ADBUDDY_EMU_PORT=5630 ADBUDDY_EMU_ADB_PORT=5090 ADBUDDY_EMU_STABLE_SECS=120 JAVA_HOME="$JDK17"
  eval "$(scripts/emulator.sh start)"   # exporta ADBUDDY_EMULATOR_SERIAL=emulator-5630, ADBUDDY_EMULATOR_ADB y ANDROID_ADB_SERVER_PORT=5090
  ```
  `$JDK17` y `$GRADLE_BIN` son el JDK 17 y la carpeta `bin` de Gradle 9.8 de este PC (están en la sección de entorno del ledger y del CLAUDE.md del usuario); el plan no fija rutas personales.
- **Gradle:** siempre `--offline`, **nunca `gradle --stop`**: `cd android && JAVA_HOME="$JDK17" "$GRADLE_BIN/gradle" --offline …`.
- **Cada tarea compila y pasa sus tests por sí sola** (comprobado en la réplica tras T1, T1+T2 y T1+T2+T3). La Tarea 2 usa `linkTo` (Tarea 1) y define `sdkTools`; la Tarea 3 usa `sdkTools`: van en orden.
- **Go:** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/...` y `go test -count=1 -timeout 5m ./internal/emutest/`. Las ejecuciones con emulador, con `-timeout 30m`. Este PC no corre `-race`; la CI sí.
- `docs/**` está en **CRLF** en el árbol de trabajo (`core.autocrlf=true`): las ediciones del inventario usan anclas de **una sola línea** (cada fila de tabla es una línea). `host/**` es LF (`.gitattributes`).
- No se tocan `doctor.go`, los catálogos i18n, `host/internal/classify/`, `scripts/emulator.sh`, `.github/` ni el emulador de otras sesiones. **Ningún texto nuevo de cara al usuario.** Las mutaciones de RED se revierten siempre (`git diff` vacío fuera de lo que la tarea añade).
- Pasar a los revisores límites de tiempo (`go test -timeout`, tope de 30 min por revisión).
- Windows con Git Bash: sin `sleep` en primer plano; lo largo con timeout amplio o en segundo plano.

## Qué corre en la CI y qué no

`emulator.yml` corre `go test -timeout 20m -count=1 -v ./internal/emutest` en la API 34 con el AVD conservado entre jobs y solo compila el APK de **debug** (`:app:assembleDebug :app:assembleDebugAndroidTest`). Con este plan:

| Test | ¿En la CI? | Por qué |
|---|---|---|
| `TestTheRemoteModeOpensAndClosesThePort`, `TestOffWithTheModeOffOnlyClosesAnOpenPort`, `TestPersistTCPPortAgainstUSBAndRestarts`, `TestTheCardSaysUnknownWhenTheAnswerIsLost` | **No** (`SKIP`) | Reinician adbd, usan `adb tcpip 7777` (sin medir: puede dejar el emulador sin transporte) o `su 0 setprop persist.adb.tcp.port` (si la limpieza falla, sobrevive en el AVD conservado). Solo con `ADBUDDY_EMULATOR_ADBD=1`, como `TestCutAccessAndReactivate` con `ADBUDDY_EMULATOR_CUT=1`. El reinicio de VB-41 pide además `ADBUDDY_EMULATOR_REBOOT=1`. |
| `TestOffClosesAPersistTCPPort` (Tarea 5) | **No** (`SKIP`) | Reinicia adbd y fija `persist.adb.tcp.port` con `su 0`: tras `ADBUDDY_EMULATOR_ADBD=1`. Es un RED esperado hasta el arreglo; no puede entrar en la CI en rojo. |
| `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`, `TestTheReleaseAppTalksOverTheLink` | **No** (`SKIP`) | La CI no compila el APK de release (`releaseHint`). `TestVersionLess` sí corre (es puro). |
| `TestTheInstalledAppReadsBackByteForByte` | **Sí** | Instala y desinstala el APK de debug; no deja nada. |
| `TestASecondProviderOnTheAuthorityIsRefused` | **Sí, si** el runner tiene build-tools y JDK (si no, `SKIP`) | Instala el debug, desactiva su provider (lo reactiva y desinstala en el cleanup) e intenta instalar un APK de aapt2 que Android rechaza. |

Ninguno añade clases a la suite instrumentada: son tests de host, así que `requiredClasses`, `allowedSkips` y `minInstrumentedTests` (`instrumented_test.go`) no cambian.

## Qué es automatizable de verdad

Lo que no es automatizable se queda en la checklist del Pixel ("Paso mínimo del usuario" del inventario); no se fuerza.

### M5b-12 · `tcpip 5555` por Tailscale con solo datos; `remote on`/`off`; caducidad
- **Automatizable (`TestTheRemoteModeOpensAndClosesThePort`):** el **controlador real** `remote.New(e.adb, store, clock)` contra el adbd del emulador. `Enable` guarda la caducidad y abre el puerto (`adb tcpip 5555`: `service.adb.tcp.port` a `5555`, pid nuevo). `Reconcile` con la caducidad por delante la conserva sin reiniciar adbd. `Disable` cierra (`adb usb`: `0` o vacío) y borra la caducidad. Con el reloj falso 12 h después: `Enabled && Closing`, **nunca "off"**, y `Expire` cierra. Una caducidad rancia con el puerto cerrado se borra sin `adb usb`; una pasada con el puerto abierto la cierra `Reconcile`.
- **No automatizable (Pixel):** Tailscale con **datos móviles** y que `serve` **reconecte** por `<ip-tailscale>:5555` con el Wi-Fi apagado. Red real y VPN.
- **Lo que el emulador no demuestra:** que el socket se abra o se cierre (escucha siempre en 5555), ni el cambio de transporte de un móvil (el del emulador no cambia y adbd vuelve en ~2 s). Solo se comprueba `service.adb.tcp.port`, que es lo que mira el host.

### M5b-13 · segundos hasta ver el estado tras `adb tcpip`; "desconocido" nunca como "apagado"
- **Ya cubierto en la JVM:** "nunca Off" como lógica (`RemoteUiTest`, `BridgeStateTest`).
- **Automatizable como integración (`TestTheCardSaysUnknownWhenTheAnswerIsLost`):** la tarjeta manda `remote.set`, el arnés ejecuta el `Enable` real (adbd se reinicia y el túnel cae) y **no contesta**, que es el caso de la respuesta perdida (ruling 3 de M5b). Al volver la app, durante 5 s, la tarjeta debe decir "Unknown", **nunca el "Off"** que conservaba (es lo que pone `mark = UnknownMark(...)`, `RemoteCard.kt:69`); luego el `remote.state` del host la pasa a "Until …". Registra los segundos hasta que vuelve la app.
- **RED de producto:** quitar `mark = UnknownMark(latest.remote)` en `RemoteCard.kt:69`. Ningún test de la JVM ejercita esa rama (`SettingsScreensTest` solo pinta la tarjeta), así que la RED **solo se puede ver en el emulador** y está **pendiente de ejecutar**; si al ejecutarla el test no se pone rojo, se anota en el ledger y la fila queda sin marcar.
- **Lo que no demuestra:** la cifra del Pixel. Los segundos del emulador son una **cota inferior** (adbd vuelve en ~2 s y el arnés reabre el túnel al momento con el mismo token; `serve` mira el túnel cada 15 s y entrega un token nuevo).

### VB-37 · "off" con el modo apagado no corta la conexión; un 5555/7777 abierto a mano se cierra
- **Automatizable (`TestOffWithTheModeOffOnlyClosesAnOpenPort`):** con el modo apagado y el puerto cerrado, `Disable` **no** reinicia adbd (mismo pid y puerto durante 5 s); con el 5555 o el 7777 abiertos a mano, los cierra con `adb usb` (pid nuevo, `service.adb.tcp.port` a `0`).
- **Lo que no demuestra:** que la conexión real de un móvil no se corte (el transporte del emulador no cambia). Se comprueba el **invariante del host** (no lanzar `adb usb` con el puerto ya cerrado), que es lo que protege al móvil. El caso 7777 no está medido en el emulador: va el último y, si el emulador no vuelve, el mensaje dice cómo recuperarlo (`restartHint`).

### VB-41 · `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd
- **Medible (`TestPersistTCPPortAgainstUSBAndRestarts`):** si el usuario `shell` puede fijar la propiedad (se registra y se vacía al momento), si fijarla con `su 0` reinicia adbd, qué deja `service.adb.tcp.port` tras `su 0 setprop ctl.restart adbd`, tras `adb tcpip`/`adb usb`, que con `service.adb.tcp.port=0` otro reinicio lo deja en 0 y `Disable` con el modo apagado no toca adbd, y, con `ADBUDDY_EMULATOR_REBOOT=1`, qué queda tras `adb reboot` y qué hace el host.
- **Lo que no demuestra:** qué puerto abre adbd de verdad (el emulador escucha siempre en 5555). El test **graba** propiedades y comportamiento del host, no afirma nada de la red.
- **Hipótesis que mide el reinicio:** el host (`remote.go:167,184,204,246`, `device.go:220`) solo lee `service.adb.tcp.port`. Si tras reiniciar con `persist` fijado `service.adb.tcp.port` queda vacío, el host tomaría por cerrado un puerto que adbd de AOSP abriría con el `persist`. Si se mide así, es un **hallazgo para adjudicar**, no un arreglo de T8.

### M6a-2 · la app de release (R8) funciona: la serialización con kotlinx
- **Automatizable:** `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey` (sin emulador: el APK de `:app:assembleRelease` firmado con clave temporal; el host lee su firma y R8 lo encogió: `RemoteUi` ya no está en el dex, y sí en el de debug) y `TestTheReleaseAppTalksOverTheLink` (instalado y verificado como release; `hello` y `device.info` de la app al host, un `chat.message` en pantalla, un `ui.request` contestado con su id y un `confirm.request` como notificación, por el arnés T3).
- **RED:** la del shrink (`isMinifyEnabled = false`), **la única de producto**: demostrada en la réplica de la v1. No se ha encontrado una regla de R8 realista que rompa kotlinx (las reglas de consumo de kotlinx y los serializadores que genera el compilador lo impiden), así que la parte de la serialización es una **guarda de regresión sin RED**; se dice así en la fila.
- **No automatizable:** la clave de release y los secretos (spec M6a §10, solo el usuario). Lo que se prueba es un APK de release funcionalmente idéntico firmado con otra clave.

### M6a-3 · lo que la spec M6a da por hecho de Android
- **Automatizable en API 34:** `TestTheInstalledAppReadsBackByteForByte` (`pm path` da una línea `package:/data/app/<…>/base.apk`; `stat -c '%s %i %Y'` tiene la forma que parsea `statKey`; `base64` devuelve el APK byte a byte, porque `Install` usa `--no-incremental`; `install -r` mueve el `stat`, la clave de la caché del `Verifier`) y `TestASecondProviderOnTheAuthorityIsRefused` (un APK de aapt2 con la autoridad `tech.hybriq.adbuddy.bridge`: `INSTALL_FAILED_CONFLICTING_PROVIDER`, también con el provider de ADBuddy desactivado, §6.3 Me-4).
- **Ya cubierto, no es avance de T8:** que el host rechaza las formas de ataque de §5 (hueco, ZIP64, comentario, firma v1) está en `phoneapp/apksig_test.go`.
- **No automatizable aquí:** instalar un **ataque real** y ver que Android vuelve a v1 necesita API 30 y `targetSdk` 29; las API 30 y 33 no tienen imagen instalada (decide el usuario) y la 37 está rota. Queda en la validación 3 de `docs/releasing.md`.

## Hallazgos para el controlador (no se arreglan aquí)

- **VB-41, hueco de seguridad (ruling del supervisor):** un adbd que sigue escuchando por TCP tras `remote off` o `EnsureNoTCPPort` es un puerto abierto que el usuario cree cerrado. La Tarea 5 lo afirma con `TestOffClosesAPersistTCPPort` (RED esperado); el arreglo va en una rama y un plan aparte ("Forma del arreglo", en la Tarea 5).
- **VB-41 (hipótesis, la mide el reinicio):** ver arriba. Si se confirma, el test escribe `FINDING (VB-41) …` y deja `hostReadsPersistPort = false`; con un arreglo del host, se pone a `true` y el paso del reinicio exige que `Disable` cierre el puerto.

## Mapa de ficheros

| Fichero | Tarea | Cambio |
|---|---|---|
| `host/internal/emutest/link_test.go` | 1 | `openLink` → `openLink` + `linkTo` |
| `host/internal/emutest/remote_test.go` | 1 | nuevo: `memStore`, `fakeClock`, ayudantes de adbd, `adbdTests`, cuatro tests |
| `host/internal/emutest/release_test.go` | 2 | nuevo: `sdkTools` y ayudantes, `installRelease`, tres tests |
| `host/internal/emutest/install_test.go` | 3 | nuevo: dos tests y ayudantes |
| `host/internal/emutest/persist_test.go` | 5 | nuevo: `TestOffClosesAPersistTCPPort` (RED esperado) y sus ayudantes |
| `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 4 | seis filas y la entrada T8, según la evidencia del ledger |
| `.superpowers/sdd/2026-10-03-adbuddy-validaciones-t8/progress.md` | 1-3 | evidencia GREEN/RED de cada ejecución (fuera de git) |

---

### Task 1: modo remoto y puertos de adbd (M5b-12, M5b-13, VB-37, VB-41) 🔒

**Files:** `host/internal/emutest/link_test.go`, `host/internal/emutest/remote_test.go` (nuevo).

- Produces: `func (e *emu) linkTo(t *testing.T, v phoneapp.Verified) *link`, `func adbdTests(t *testing.T)`, `TestTheRemoteModeOpensAndClosesThePort`, `TestOffWithTheModeOffOnlyClosesAnOpenPort`, `TestPersistTCPPortAgainstUSBAndRestarts`, `TestTheCardSaysUnknownWhenTheAnswerIsLost`.

- [ ] **Step 1: refactor de `openLink`.** Edit en `host/internal/emutest/link_test.go`:

**Reemplazar:**
```go
func openLink(t *testing.T) *link {
	t.Helper()
	e := newEmu(t)
	v := e.installApp(t)
	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
```
**por:**
```go
func openLink(t *testing.T) *link {
	t.Helper()
	e := newEmu(t)
	return e.linkTo(t, e.installApp(t))
}

// linkTo is openLink for the app v checked, already installed (and
// uninstalled at cleanup) by its caller: the debug app (installApp) or
// another build, such as the release one (installRelease).
func (e *emu) linkTo(t *testing.T, v phoneapp.Verified) *link {
	t.Helper()
	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
```
El resto del cuerpo de `openLink` queda como cuerpo de `linkTo` sin cambiar ninguna línea (usa `e` y `v`, ahora receptor y parámetro).

- [ ] **Step 2: `host/internal/emutest/remote_test.go`,** completo:

```go
package emutest

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// tsIP is a Tailscale address (100.64.0.0/10) for the remote controller. The
// emulator is never reached through it: Enable only refuses to run without one
// (M5b, ErrNoTailscale).
const tsIP = "100.64.0.5"

// memStore is the controller's config.toml, in memory.
type memStore struct {
	mu    sync.Mutex
	until time.Time
}

func (s *memStore) TailscaleIP() string { return tsIP }

func (s *memStore) Until() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.until
}

func (s *memStore) SetUntil(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.until = t
	return nil
}

// fakeClock is the controller's now: the 12 h pass without waiting for them.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// tryShell runs command on the emulator and returns its output, or "" when
// adb fails (the emulator away while adbd restarts).
func (e *emu) tryShell(command string) string {
	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	out, _, code, err := e.adb.Output(ctx, "shell", command)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
}

// adbdNow returns adbd's pid and service.adb.tcp.port, read by one shell
// command, so an empty port is never a failed read; ok is false when adb
// failed. A new pid is a restarted adbd: that is how these tests see whether
// `adb tcpip` or `adb usb` ran, both of which restart it (the emulator is away
// for about 2 s, which sampling `adb devices` every second can miss).
func (e *emu) adbdNow() (pid, port string, ok bool) {
	out := e.tryShell(`echo "$(pidof adbd)|$(getprop service.adb.tcp.port)"`)
	pid, port, ok = strings.Cut(out, "|")
	return pid, port, ok && pidRe.MatchString(pid)
}

// adbd returns adbd's pid and service.adb.tcp.port now, or fails.
func (e *emu) adbd(t *testing.T) (pid, port string) {
	t.Helper()
	pid, port, ok := e.adbdNow()
	if !ok {
		t.Fatalf("could not read adbd's pid and port (emulator %q)", e.state())
	}
	return pid, port
}

// restartHint is what to do when the emulator does not come back after a
// change of adbd's port: service.adb.tcp.port does not survive a cold boot.
const restartHint = "if the emulator stays away, stop it with scripts/emulator.sh stop and start it again (same ADBUDDY_EMU_* variables): service.adb.tcp.port does not survive a cold boot"

// restartedTo waits up to 60 s for adbd to run with a pid other than before
// and a service.adb.tcp.port in want, and returns the new pid.
func (e *emu) restartedTo(t *testing.T, before string, want ...string) string {
	t.Helper()
	var pid, port string
	if !poll(60*time.Second, func() bool {
		var ok bool
		pid, port, ok = e.adbdNow()
		return ok && pid != before && slices.Contains(want, port)
	}) {
		t.Fatalf("60 s on, adbd has pid %q (before %s) and service.adb.tcp.port %q, want a new pid and one of %q (emulator %q); %s",
			pid, before, port, want, e.state(), restartHint)
	}
	return pid
}

// sameFor is how long sameAdbd watches adbd: adb usb answers before adbd
// restarts (1-2 s later), so one read right after an order could still see
// the old pid.
const sameFor = 5 * time.Second

// sameAdbd checks, every half second for sameFor, that adbd keeps pid and
// port and answers: nothing restarted it.
func (e *emu) sameAdbd(t *testing.T, pid, port string) {
	t.Helper()
	deadline := time.Now().Add(sameFor)
	for {
		now, p, ok := e.adbdNow()
		if !ok || now != pid || p != port {
			t.Errorf("adbd has pid %q and service.adb.tcp.port %q (read ok %v), want the same %s and %q for %v: something restarted it", now, p, ok, pid, port, sameFor)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// adbdTests skips the tests that restart the emulator's adbd (adb tcpip, adb
// usb, ctl.restart, persist.adb.tcp.port) unless ADBUDDY_EMULATOR_ADBD=1, as
// TestCutAccessAndReactivate asks for ADBUDDY_EMULATOR_CUT: the CI keeps its
// AVD between jobs, and a port other than 5555 or a persist property left
// behind would outlive the job. They run on a development emulator only.
func adbdTests(t *testing.T) {
	t.Helper()
	if os.Getenv("ADBUDDY_EMULATOR_ADBD") != "1" {
		t.Skip("set ADBUDDY_EMULATOR_ADBD=1 to run it, on a development emulator only (it restarts adbd)")
	}
}

// portClosed says whether service.adb.tcp.port reads as no network port, as
// remote and device.EnsureNoTCPPort take it (empty or 0).
func portClosed(p string) bool { return p == "" || p == "0" }

// closedAtStartAndEnd requires adbd without a TCP port before the test and
// closes it at cleanup if the test left it open (adb usb).
func (e *emu) closedAtStartAndEnd(t *testing.T) {
	t.Helper()
	if _, p := e.adbd(t); !portClosed(p) {
		t.Fatalf("service.adb.tcp.port = %q before the test: a tcpip mode left over; close it with `adb usb`", p)
	}
	t.Cleanup(func() {
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		pid, p, ok := c.adbdNow()
		if !ok || portClosed(p) {
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "usb")
		if !poll(60*time.Second, func() bool {
			now, p, ok := c.adbdNow()
			return ok && now != pid && portClosed(p)
		}) {
			t.Errorf("service.adb.tcp.port still open after the cleanup's adb usb; %s", restartHint)
		}
	})
}

// Validation 12 of M5b, what the emulator can show of it: the remote
// controller of serve, against the emulator's real adbd. Enable saves the
// expiry and opens port 5555 (adb tcpip); Reconcile with the expiry ahead
// keeps it and runs nothing; Disable closes it (adb usb) and clears the
// expiry; with the 12 h passed the mode is Closing, never off, and Expire
// closes it; a stale expiry with the port closed is cleared without adb usb,
// and one that passed with the port open is closed by Reconcile. On the
// emulator adbd always listens on 5555 (its own transport, M2.5b S5): what
// counts is service.adb.tcp.port, never the socket. Tailscale with mobile
// data, and serve reconnecting through it, stay on the Pixel.
func TestTheRemoteModeOpensAndClosesThePort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	st, clk := &memStore{}, &fakeClock{t: time.Now().UTC()}
	ctl := remote.New(e.adb, st, clk.now)
	want := func(what string, got, want remote.State, err error) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("%s: state %+v, %v; want %+v", what, got, err, want)
		}
	}
	on := func() remote.State {
		return remote.State{Enabled: true, Address: tsIP + ":" + remote.Port, ExpiresAt: clk.now().Add(remote.Lifetime)}
	}

	pid, _ := e.adbd(t)
	s, err := ctl.Enable(e.ctx)
	want("Enable", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	if !st.Until().Equal(clk.now().Add(remote.Lifetime)) {
		t.Errorf("saved expiry %v, want now + %v", st.Until(), remote.Lifetime)
	}
	if s := e.state(); s != "device" {
		t.Errorf("the emulator is %q after adb tcpip: it keeps its serial (M2.5b S5)", s)
	}

	// serve starts again with the expiry ahead: kept, and adbd left alone.
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile with the expiry ahead", s, on(), err)
	e.sameAdbd(t, pid, remote.Port)

	s, err = ctl.Disable(e.ctx)
	want("Disable", s, remote.State{}, err)
	pid = e.restartedTo(t, pid, "0", "")
	if !st.Until().IsZero() {
		t.Errorf("the expiry %v is still saved after Disable", st.Until())
	}

	// The 12 h pass: Closing (never off) until Expire closes the port.
	s, err = ctl.Enable(e.ctx)
	want("Enable again", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	clk.add(remote.Lifetime + time.Second)
	if s := ctl.State(); !s.Enabled || !s.Closing {
		t.Errorf("past the expiry with the port open: %+v, want Enabled and Closing", s)
	}
	s, changed, err := ctl.Expire(e.ctx)
	if err != nil || !changed || s != (remote.State{}) {
		t.Fatalf("Expire: %+v, changed %v, %v", s, changed, err)
	}
	pid = e.restartedTo(t, pid, "0", "")

	// A stale expiry (the phone restarted and the port closed meanwhile):
	// Reconcile clears it without adb usb.
	_ = st.SetUntil(clk.now().Add(time.Hour))
	_, port := e.adbd(t)
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile with the port closed", s, remote.State{}, err)
	e.sameAdbd(t, pid, port)
	if !st.Until().IsZero() {
		t.Errorf("the stale expiry %v is still saved", st.Until())
	}

	// An expiry that passed while serve was stopped, with the port open:
	// Reconcile closes it now.
	s, err = ctl.Enable(e.ctx)
	want("Enable a third time", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	clk.add(remote.Lifetime + time.Second)
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile past the expiry", s, remote.State{}, err)
	e.restartedTo(t, pid, "0", "")
}

// Validation 37 of the v1 deferred B (VB-37): "off" with the remote mode off
// does not restart adbd when the port reads closed (on a phone that would cut
// its connection for nothing), and closes a port opened by hand, 5555 or any
// other (7777), with adb usb. adbd's pid tells a restart (adbdNow).
func TestOffWithTheModeOffOnlyClosesAnOpenPort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	ctl := remote.New(e.adb, &memStore{}, nil)
	t.Run("closed", func(t *testing.T) {
		pid, port := e.adbd(t)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
			t.Fatalf("Disable: %+v, %v", s, err)
		}
		e.sameAdbd(t, pid, port)
	})
	// 7777 last: the emulator's transport with a port other than 5555 has not
	// been measured (restartHint if it does not come back).
	for _, p := range []string{remote.Port, "7777"} {
		t.Run("open by hand on "+p, func(t *testing.T) {
			pid, _ := e.adbd(t)
			e.run(t, "tcpip", p)
			pid = e.restartedTo(t, pid, p)
			if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
				t.Fatalf("Disable: %+v, %v", s, err)
			}
			e.restartedTo(t, pid, "0")
		})
	}
}

// hostReadsPersistPort says whether remote and device.EnsureNoTCPPort read
// persist.adb.tcp.port. They do not (remote.go and device.go read only
// service.adb.tcp.port; pendientes, diferidos B, T2). Hypothesis the reboot
// step measures: after a reboot with persist set, service.adb.tcp.port reads
// empty while adbd listens on the persist port (AOSP adbd reads persist only
// when the service property is empty), and the host reads "closed". If it is
// measured so, it is a finding for the controller; with a fix, set this to
// true and the reboot step then wants Disable to close the port.
const hostReadsPersistPort = false

// rebootAndWait reboots the emulator and waits up to 3 min for it to boot
// (with -no-snapshot the AVD keeps its data: T1-9).
func (e *emu) rebootAndWait(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	_, _, _, _ = e.adb.Output(ctx, "reboot")
	e.waitState(func(s string) bool { return s != "device" }, 30*time.Second)
	if !poll(3*time.Minute, func() bool {
		return e.state() == "device" && e.tryShell("getprop sys.boot_completed") == "1"
	}) {
		t.Fatalf("the emulator did not boot 3 min after adb reboot (state %q)", e.state())
	}
}

// Validation 41 of the v1 deferred B (VB-41): persist.adb.tcp.port against
// adb usb and adbd's restarts, measured on the emulator's adbd. The emulator
// always listens on 5555, so which port adbd really opens is not visible here:
// the test records the properties and what the host does with them. The
// reboot step runs only with ADBUDDY_EMULATOR_REBOOT=1 (a minute or two more).
func TestPersistTCPPortAgainstUSBAndRestarts(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	persist := func() string { return e.shell(t, "su 0 getprop persist.adb.tcp.port") }
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q before the test: left over; clear it with `adb shell su 0 setprop persist.adb.tcp.port ''`", p)
	}
	// Cleanups run last first: this one before closedAtStartAndEnd's adb usb.
	e.atCleanup(t, "shell", "su 0 setprop persist.adb.tcp.port ''")

	// Can the shell user set it without su? Measured and logged, not
	// required (the classifier makes `setprop persist.adb…` 🔴 anyway). It is
	// emptied again at once, so the measurement below starts from nothing.
	out, _, code, _ := e.adb.Output(e.ctx, "shell", "setprop persist.adb.tcp.port 5555")
	t.Logf("setprop persist.adb.tcp.port 5555 as shell: exit %d %s; persist now %q", code, strings.TrimSpace(out), persist())
	e.shell(t, "su 0 setprop persist.adb.tcp.port ''")
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q after emptying it", p)
	}

	pid, port := e.adbd(t)
	e.shell(t, "su 0 setprop persist.adb.tcp.port 5555")
	restartedBySetprop := poll(10*time.Second, func() bool {
		now, _, ok := e.adbdNow()
		return ok && now != pid
	})
	t.Logf("setting persist.adb.tcp.port restarted adbd: %v", restartedBySetprop)
	pid, after := e.adbd(t)
	if after != port {
		t.Errorf("service.adb.tcp.port went from %q to %q when only persist.adb.tcp.port was set", port, after)
	}

	// adbd restarted by init, not by adb: the service property stays.
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, port)

	// adb tcpip and adb usb: usb sets the service property to 0.
	e.run(t, "tcpip", remote.Port)
	pid = e.restartedTo(t, pid, remote.Port)
	e.run(t, "usb")
	pid = e.restartedTo(t, pid, "0")
	t.Logf("persist.adb.tcp.port after adb usb: %q", persist())

	// With the service property at 0 a restart keeps it at 0 (AOSP adbd then
	// ignores persist), and "off" with the mode off leaves adbd alone.
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, "0")
	ctl := remote.New(e.adb, &memStore{}, nil)
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable: %+v, %v", s, err)
	}
	e.sameAdbd(t, pid, "0")

	if os.Getenv("ADBUDDY_EMULATOR_REBOOT") != "1" {
		t.Log("the reboot step needs ADBUDDY_EMULATOR_REBOOT=1")
		return
	}
	// After a reboot the service property is empty and persist is still set:
	// adbd takes the persist port, and the host reads "closed".
	if p := persist(); p != remote.Port {
		t.Logf("persist.adb.tcp.port = %q before the reboot (adb usb cleared it?): the reboot step has nothing to show", p)
		return
	}
	e.rebootAndWait(t)
	pid, port = e.adbd(t)
	t.Logf("after the reboot: service.adb.tcp.port %q, persist.adb.tcp.port %q", port, persist())
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable after the reboot: %+v, %v", s, err)
	}
	switch {
	case !portClosed(port):
		// The service property reflects persist: the host sees the port and closes it.
		e.restartedTo(t, pid, "0")
	case hostReadsPersistPort:
		e.restartedTo(t, pid, "0")
	default:
		e.sameAdbd(t, pid, port)
		t.Logf("FINDING (VB-41): after a reboot with persist.adb.tcp.port=%s, service.adb.tcp.port reads %q and the host takes the port as closed; Disable ran no adb usb", remote.Port, port)
	}
}

// cardText returns what the remote card says among nodes: one of its
// texts in English (the harness sets the app to English), or "".
func cardText(nodes []uiNode) string {
	for _, n := range nodes {
		switch {
		case n.Text == "Off", n.Text == "On", n.Text == "Working…", n.Text == "Not connected to the PC",
			strings.HasPrefix(n.Text, "Unknown:"), strings.HasPrefix(n.Text, "Closing…"), strings.HasPrefix(n.Text, "Until "):
			return n.Text
		}
	}
	return ""
}

// Validation 13 of M5b, on the emulator, with the answer lost (M5b ruling
// 3): the remote card's switch sends remote.set; the harness, as serve, runs
// the real Enable (adb tcpip 5555 restarts adbd and drops the tunnel) and does
// not answer, as when the answer is lost with the tunnel. The app gets no
// reply and marks the state unknown (RemoteCard.kt:69); once it is back, for
// 5 s, the card must say Unknown, never the "Off" it kept through the
// disconnection; then the host's remote.state (serve republishes it on
// attach, M5b-20) turns it to "Until …". It logs the seconds from adb tcpip
// until the app is back: the emulator's, a lower bound of the Pixel's (adbd is
// back in ~2 s here and the harness reopens the tunnel at once, keeping the
// token; serve looks every 15 s and hands over a new one, and a phone comes
// back over Wi-Fi). "Never Off" as logic is RemoteUiTest's and
// BridgeStateTest's on the JVM; this is the integration with a real drop.
func TestTheCardSaysUnknownWhenTheAnswerIsLost(t *testing.T) {
	adbdTests(t)
	l := openLink(t)
	e := l.e
	e.closedAtStartAndEnd(t)
	ctl := remote.New(e.adb, &memStore{}, nil)
	// As serve on attach (M5b-20): the mode as the host has it, off.
	l.send(t, bridge.TypeRemoteState, bridge.RemoteState{})
	l.openChat(t)
	e.waitNode(t, "the card saying Off", 15*time.Second, func(n uiNode) bool { return n.Text == "Off" })
	e.tap(t, e.waitNode(t, "the card's switch", 5*time.Second, func(n uiNode) bool { return n.Class == "android.widget.Switch" }))
	e.tap(t, e.waitNode(t, "Turn on", 10*time.Second, func(n uiNode) bool { return n.Text == "Turn on" }))
	req := l.waitFor(t, bridge.TypeRemoteSet, 10*time.Second)
	if p := payload[bridge.RemoteSet](t, req); !p.Enabled {
		t.Fatalf("remote.set %s, want enabled", req.Payload)
	}

	start := time.Now()
	hellos := len(l.Hellos())
	pid, _ := e.adbd(t)
	s, err := ctl.Enable(e.ctx)
	if err != nil || !s.Enabled {
		t.Fatalf("Enable: %+v, %v", s, err)
	}
	e.restartedTo(t, pid, remote.Port)
	// adbd's restart takes the reverse with it: reopened as serve's tunnel
	// check does (phone.go), without its new token.
	if open, err := phoneapp.TunnelOpen(e.ctx, e.adb, l.port); err != nil || !open {
		t.Logf("the reverse did not survive adb tcpip (open %v, %v): reopened", open, err)
		if err := phoneapp.Tunnel(e.ctx, e.adb, l.port); err != nil {
			t.Fatalf("adb reverse: %v", err)
		}
	}
	l.waitHellos(t, hellos+1, 90*time.Second)
	t.Logf("M5b-13 on the emulator: the app was back %.1f s after adb tcpip", time.Since(start).Seconds())

	// Back, and no state from the host yet: unknown, never the old off.
	var seen []string
	note := func() string {
		text := cardText(e.screen(t))
		if text != "" && (len(seen) == 0 || seen[len(seen)-1] != text) {
			seen = append(seen, text)
		}
		return text
	}
	poll(5*time.Second, func() bool { note(); return false })
	if slices.Contains(seen, "Off") {
		t.Errorf("after the lost answer the card said Off: %q", seen)
	}
	if len(seen) == 0 || !strings.HasPrefix(seen[len(seen)-1], "Unknown:") {
		t.Errorf("after the lost answer the card said %q, want Unknown", seen)
	}

	// The host speaks: the card says Until …
	l.send(t, bridge.TypeRemoteState, bridge.RemoteState{Enabled: true, Address: s.Address, ExpiresAt: s.ExpiresAt})
	if !poll(20*time.Second, func() bool { return strings.HasPrefix(note(), "Until ") }) {
		t.Errorf("the card did not say Until … 20 s after remote.state; it said %q", seen)
	}
}
```

- [ ] **Step 3: compila y los tests sin emulador pasan.**
```bash
cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/...
go test -count=1 -timeout 5m ./internal/emutest/
```
Esperado: `ok`; los cuatro tests nuevos `SKIP` (sin emulador, o sin `ADBUDDY_EMULATOR_ADBD=1`).

- [ ] **Step 4: GREEN y RED en el emulador** (solo cuando el controlador diga que hay emulador libre). Con el bloque de variables de "Global Constraints" y los APK de debug (`cd android && JAVA_HOME="$JDK17" "$GRADLE_BIN/gradle" --offline :app:assembleDebug :app:assembleDebugAndroidTest`):
  - **GREEN** (una ejecución, con el reinicio de VB-41 incluido; guardar la salida entera de `-v` en el ledger):
    ```bash
    cd host && ADBUDDY_EMULATOR_ADBD=1 ADBUDDY_EMULATOR_REBOOT=1 go test -count=1 -timeout 30m -v \
      -run 'TestTheRemoteModeOpensAndClosesThePort|TestOffWithTheModeOffOnlyClosesAnOpenPort|TestPersistTCPPortAgainstUSBAndRestarts|TestTheCardSaysUnknownWhenTheAnswerIsLost' ./internal/emutest/
    ```
    Del ledger salen: los `t.Logf` de VB-41 (si `shell` pudo fijar `persist`, si fijarlo reinició adbd, `persist` tras `adb usb`, las propiedades tras el reinicio y, si aparece, el `FINDING (VB-41)`) y los segundos de M5b-13.
  - **RED por mutación** (una cada vez, cada una con su salida en el ledger, y revertida con `git checkout -- <fichero>`):
    1. `host/internal/remote/remote.go`, en `Disable`, borrar el bloque `if c.st.Until().IsZero() { … }` (que siempre llame a `c.disable`): `TestOffWithTheModeOffOnlyClosesAnOpenPort/closed` falla con `adbd has pid … want the same …: something restarted it` (VB-37).
    2. `host/internal/remote/remote.go`, en `Reconcile`, cambiar `case port != Port:` por `case false:`: con la caducidad por delante y el puerto cerrado, `Reconcile` devuelve el estado guardado y `TestTheRemoteModeOpensAndClosesThePort` falla en `Reconcile with the port closed: state {Enabled:true Closing:false Address:100.64.0.5:5555 …}, <nil>; want {Enabled:false …}`.
    3. `android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/RemoteCard.kt:69`, cambiar `reply == null -> mark = UnknownMark(latest.remote)` por `reply == null -> {}`, reconstruir el APK de debug y correr `TestTheCardSaysUnknownWhenTheAnswerIsLost`: debe fallar con `after the lost answer the card said Off` (M5b-13). **Si no falla**, anotarlo en el ledger tal cual: la fila M5b-13 se queda en "test escrito, sin RED de producto" (Tarea 4, variante B).
  - Apagar el emulador al terminar con `scripts/emulator.sh stop` (mismas variables) y comprobar por PID que no queda.

- [ ] **Step 5: commit.** `test(emutest): modo remoto y puertos de adbd en la API 34 (M5b-12/13, VB-37, VB-41)`, con el trailer exacto.

---

### Task 2: la app de release (R8) en la API 34 (M6a-2) 🔒

**Files:** `host/internal/emutest/release_test.go` (nuevo). Usa `linkTo` (Tarea 1) y el arnés.

- Produces: `sdkTools` (`findTools`, `run`, `newKey`, `sign`), `exe`, `sdkRoot`, `versionLess`, `newest`, `exists`, `dexHas`, `releaseSigned`, `installRelease`, `TestVersionLess`, `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`, `TestTheReleaseAppTalksOverTheLink`.

- [ ] **Step 1: `host/internal/emutest/release_test.go`,** completo:

```go
package emutest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// releaseAPK is the unsigned APK of `gradle :app:assembleRelease` (R8 and
// resource shrinking), as release-check.yml builds it, relative to this
// package.
var releaseAPK = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "release", "app-release-unsigned.apk")

// releaseHint says how to build it, with release-check's test version.
const releaseHint = "build it first: cd android && gradle --offline :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101"

// releaseVersion is the versionName releaseHint builds.
const releaseVersion = "0.0.1-rc.1"

// shrunkAway is a class of the app that R8 renames or inlines in the release
// build (no keep rule names it) and that the debug build has: its descriptor
// in a dex tells an unshrunk APK.
const shrunkAway = "Ltech/hybriq/adbuddy/ui/RemoteUi;"

// sdkTools are the SDK's and the JDK's tools the tests build and sign APKs
// with, never the release key (spec M6a §10: only its owner touches it).
type sdkTools struct {
	aapt2, zipalign, apksigner string // build-tools
	androidJar                 string // a platform's android.jar, for aapt2
	keytool, javaHome          string // the JDK
}

// exe adds Windows' extension to a tool's name.
func exe(name, ext string) string {
	if runtime.GOOS == "windows" {
		return name + ext
	}
	return name
}

// sdkRoot is the Android SDK, found as scripts/emulator.sh finds it, or "".
func sdkRoot() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk"),
		filepath.Join(home, "Android", "Sdk"), filepath.Join(home, "Library", "Android", "sdk")} {
		if d == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(d, "platform-tools")); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// versionLess compares two dotted versions ("36.0.0" < "37.0.0"; "android-37.0" < "android-38").
func versionLess(a, b string) bool {
	num := func(s string) []int {
		var n []int
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
			v, _ := strconv.Atoi(f)
			n = append(n, v)
		}
		return n
	}
	return slices.Compare(num(a), num(b)) < 0
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"36.0.0", "37.0.0", true}, {"37.0.0", "36.0.0", false}, {"9.0.0", "10.0.0", true},
		{"android-34", "android-37.0", true}, {"android-37.0", "android-37.0", false},
	} {
		if got := versionLess(c.a, c.b); got != c.less {
			t.Errorf("versionLess(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// newest returns the entry of dir, newest version first, that ok accepts, or "".
func newest(dir string, ok func(string) bool) string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.SortFunc(names, func(a, b string) int {
		if versionLess(b, a) {
			return -1
		}
		if versionLess(a, b) {
			return 1
		}
		return 0
	})
	for _, n := range names {
		if p := filepath.Join(dir, n); ok(p) {
			return p
		}
	}
	return ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// findTools returns the tools, or skips the test saying what is missing.
func findTools(t *testing.T) sdkTools {
	t.Helper()
	sdk := sdkRoot()
	if sdk == "" {
		t.Skip("no Android SDK: set ANDROID_HOME")
	}
	var tl sdkTools
	bt := newest(filepath.Join(sdk, "build-tools"), func(d string) bool {
		return exists(filepath.Join(d, exe("aapt2", ".exe"))) && exists(filepath.Join(d, exe("zipalign", ".exe"))) &&
			exists(filepath.Join(d, exe("apksigner", ".bat")))
	})
	if bt == "" {
		t.Skip("no build-tools with aapt2, zipalign and apksigner in " + sdk + ": sdkmanager \"build-tools;37.0.0\"")
	}
	tl.aapt2, tl.zipalign, tl.apksigner = filepath.Join(bt, exe("aapt2", ".exe")), filepath.Join(bt, exe("zipalign", ".exe")), filepath.Join(bt, exe("apksigner", ".bat"))
	platform := newest(filepath.Join(sdk, "platforms"), func(d string) bool { return exists(filepath.Join(d, "android.jar")) })
	if platform == "" {
		t.Skip("no platform with android.jar in " + sdk + ": sdkmanager \"platforms;android-37.0\"")
	}
	tl.androidJar = filepath.Join(platform, "android.jar")
	if jh := os.Getenv("JAVA_HOME"); jh != "" && exists(filepath.Join(jh, "bin", exe("keytool", ".exe"))) {
		tl.javaHome = jh
	} else if kt, err := exec.LookPath("keytool"); err == nil {
		tl.javaHome = filepath.Dir(filepath.Dir(kt))
	} else {
		t.Skip("no JDK: set JAVA_HOME to a JDK 17 (keytool, and java for apksigner)")
	}
	tl.keytool = filepath.Join(tl.javaHome, "bin", exe("keytool", ".exe"))
	return tl
}

// run runs one of the tools with JAVA_HOME and its java first in PATH (and
// env on top), and returns its output; it fails the test on an error.
func (tl sdkTools) run(t *testing.T, env []string, tool string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = append(os.Environ(), "JAVA_HOME="+tl.javaHome,
		"PATH="+filepath.Join(tl.javaHome, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", filepath.Base(tool), args, err, out)
	}
	return string(out)
}

// newKey creates a throwaway signing key (RSA 2048, one day) in dir, as
// release-check does, and returns the keystore and its password. Nothing of
// it outlives the test (t.TempDir).
func (tl sdkTools) newKey(t *testing.T, dir, cn string) (ks, pass string) {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	pass = "throwaway-" + hex.EncodeToString(b)
	ks = filepath.Join(dir, cn+".p12")
	tl.run(t, []string{"KS_PASS=" + pass}, tl.keytool, "-genkeypair", "-keystore", ks, "-storetype", "PKCS12",
		"-storepass:env", "KS_PASS", "-alias", "adbuddy", "-keyalg", "RSA", "-keysize", "2048", "-validity", "1",
		"-dname", "CN="+cn)
	return ks, pass
}

// sign aligns in and signs it into out with the key, with apksigner's
// scheme flags (--v1-signing-enabled …).
func (tl sdkTools) sign(t *testing.T, in, out, ks, pass string, schemes ...string) {
	t.Helper()
	aligned := out + ".aligned"
	tl.run(t, nil, tl.zipalign, "-f", "4", in, aligned)
	args := append([]string{"sign", "--ks", ks, "--ks-type", "PKCS12", "--ks-pass", "env:KS_PASS", "--ks-key-alias", "adbuddy"}, schemes...)
	tl.run(t, []string{"KS_PASS=" + pass}, tl.apksigner, append(args, "--out", out, aligned)...)
}

// dexHas reports whether any classes*.dex of the APK holds s.
func dexHas(t *testing.T, apk []byte, s string) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(apk), int64(len(apk)))
	if err != nil {
		t.Fatalf("read the APK as a zip: %v", err)
	}
	dexes := 0
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "classes") || !strings.HasSuffix(f.Name, ".dex") {
			continue
		}
		dexes++
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	if dexes == 0 {
		t.Fatal("the APK has no classes*.dex")
	}
	return false
}

// releaseSigned signs the release APK with a throwaway key made for this
// test (never the release key) and returns the signed APK and the SHA-256
// of its certificate, which the test then trusts as the release one. It
// skips without the APK or the tools, and fails if the APK is not shrunk.
func releaseSigned(t *testing.T) (apk, cert string) {
	t.Helper()
	if !exists(releaseAPK) {
		t.Skip("no release APK at " + releaseAPK + "; " + releaseHint)
	}
	tl := findTools(t)
	dir := t.TempDir()
	ks, pass := tl.newKey(t, dir, "emutest-release")
	apk = filepath.Join(dir, "adbuddy.apk")
	tl.sign(t, releaseAPK, apk, ks, pass, "--v1-signing-enabled", "false", "--v2-signing-enabled", "true", "--v3-signing-enabled", "true")
	data, err := os.ReadFile(apk)
	if err != nil {
		t.Fatal(err)
	}
	certs, err := phoneapp.Signers(data)
	if err != nil || len(certs) == 0 {
		t.Fatalf("the host does not read the signed release APK: %v", err)
	}
	if err := phoneapp.CheckSigner(data, certs[0]); err != nil {
		t.Fatalf("CheckSigner with its own certificate: %v", err)
	}
	if dexHas(t, data, shrunkAway) {
		t.Fatalf("%s is in the release APK's dex: it was not shrunk by R8 (isMinifyEnabled in android/app/build.gradle.kts?)", shrunkAway)
	}
	return apk, certs[0]
}

// Validation 2 of M6a, its part on the PC: the release APK (R8) signed as
// release-check signs it, with a throwaway key; every block's signer is that
// key (the host's own parser, spec M6a §5) and R8 did shrink it. The debug
// APK, when built, is the control: the class shrunkAway is in its dex.
func TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey(t *testing.T) {
	releaseSigned(t)
	if data, err := os.ReadFile(appAPK); err == nil {
		if !dexHas(t, data, shrunkAway) {
			t.Errorf("%s is not in the debug APK either: the check would not tell a shrunk APK", shrunkAway)
		}
	} else {
		t.Logf("no debug APK for the control (%v)", err)
	}
}

// installRelease installs the signed release APK on a clean slate and
// returns the proof the host gives it, trusting cert as the release
// certificate (not a development APK: Dev is false).
func (e *emu) installRelease(t *testing.T, apk, cert string) phoneapp.Verified {
	t.Helper()
	_, _, _, _ = e.adb.Output(e.ctx, "uninstall", phoneapp.Package)
	if err := phoneapp.Install(e.ctx, e.adb, apk); err != nil {
		t.Fatalf("install the release APK: %v", err)
	}
	e.uninstallAtCleanup(t, phoneapp.Package)
	v, err := phoneapp.NewVerifier(func() phoneapp.Trust { return phoneapp.Trust{Release: cert} }, phoneapp.MinAppVersionCode).Verify(e.ctx, e.adb)
	if err != nil {
		t.Fatalf("verify the installed release app: %v", err)
	}
	if v.Dev() || v.Cert() != cert {
		t.Fatalf("verified as dev %v with certificate %s, want the release certificate %s", v.Dev(), v.Cert(), cert)
	}
	return v
}

// Validation 2 of M6a: the release app (R8) works on API 34. Installed as
// setup installs a release, checked by the host against its certificate, it
// says hello and sends device.info (kotlinx.serialization, app to host); it
// shows a chat.message (host to app), answers a ui.request with its id
// (both ways, with a rid) and shows a confirm.request as its notification.
func TestTheReleaseAppTalksOverTheLink(t *testing.T) {
	e := newEmu(t)
	apk, cert := releaseSigned(t)
	l := e.linkTo(t, e.installRelease(t, apk, cert))
	if h := l.Hellos()[0]; h.AppVersion != releaseVersion || h.Protocol < bridge.MinAppProtocol {
		t.Errorf("hello: appVersion %q, protocol %d; want %q", h.AppVersion, h.Protocol, releaseVersion)
	}
	if l.Info.ProtectedApps == nil {
		t.Error("device.info without protectedApps")
	}

	// Before the question: its dialog would cover the chat.
	l.openChat(t)
	const said = "Hello from the release check"
	l.send(t, bridge.TypeChatMessage, bridge.ChatMessage{ID: "m_release", Role: "assistant", Text: said, Attachments: []bridge.Attachment{}})
	e.waitNode(t, "the host's chat message", 15*time.Second, func(n uiNode) bool { return strings.Contains(n.Text, said) })

	r := l.uiRequest(t, "u_release", bridge.UIOpForeground, bridge.Empty{})
	if !r.OK && (r.Error == nil || r.Error.Code == bridge.CodeInvalid) {
		t.Errorf("ui.response to foreground: %+v", r)
	}

	l.send(t, bridge.TypeConfirmReq, bridge.ConfirmRequest{
		ID: "j_release", Level: "red", Title: "Restart the phone", Detail: "may restart",
		RawCommand: "adb shell reboot", ExpiresAt: time.Now().Add(2 * time.Minute).UTC(),
	})
	var rec string
	if !poll(20*time.Second, func() bool {
		rec = notificationRecord(e.shell(t, "dumpsys notification --noredact"), phoneapp.Package, "confirm")
		return strings.Contains(rec, "Restart the phone")
	}) {
		t.Errorf("no question on the confirm channel 20 s after the confirm.request:\n%s", rec)
	}
}
```

- [ ] **Step 2: APK de release y su prueba sin emulador.**
```bash
cd android && JAVA_HOME="$JDK17" "$GRADLE_BIN/gradle" --offline :app:assembleDebug :app:assembleRelease \
  -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101
cd ../host && go test -count=1 -timeout 5m -v -run 'TestVersionLess|TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey' ./internal/emutest/
```
Esperado: los dos `PASS`.
  - **RED del shrink (la única RED de producto de M6a-2):** en `android/app/build.gradle.kts` poner `isMinifyEnabled = false` e `isShrinkResources = false`, reconstruir el release y correr el test: `FAIL` con `Ltech/hybriq/adbuddy/ui/RemoteUi; is in the release APK's dex: it was not shrunk by R8`. Revertir (`git checkout -- android/app/build.gradle.kts`), reconstruir, `PASS`. Salidas al ledger.

- [ ] **Step 3: compila y tests sin emulador.** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/... && go test -count=1 -timeout 5m ./internal/emutest/`.

- [ ] **Step 4: GREEN con emulador** (con el bloque de variables y el APK de release construido): `cd host && go test -count=1 -timeout 30m -v -run TestTheReleaseAppTalksOverTheLink ./internal/emutest/`, salida al ledger. No hay RED de producto para la serialización bajo R8 (ver "Qué es automatizable"); el ledger lo dice. Apagar el emulador al terminar (por serial, comprobado por PID).

- [ ] **Step 5: commit.** `test(emutest): la app de release con R8 en la API 34 (M6a-2)`.

---

### Task 3: pm path, stat, base64, install -r y la autoridad duplicada (M6a-3) 🔒

**Files:** `host/internal/emutest/install_test.go` (nuevo). Usa `sdkTools` (Tarea 2).

- Produces: `TestTheInstalledAppReadsBackByteForByte`, `TestASecondProviderOnTheAuthorityIsRefused`, `buildConflictAPK` y los ayudantes `base64`, `nonEmptyLines`, `basePath`, `firstPath`, `threeInts`.

- [ ] **Step 1: `host/internal/emutest/install_test.go`,** completo:

```go
package emutest

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Validation 3 of M6a, what API 34 can show: the Android behaviour the host's
// trust model relies on (spec M6a §6.2). pm path gives one base.apk under
// /data/app; stat -c '%s %i %Y' has the shape the host parses; base64 reads
// the APK back byte for byte (--no-incremental: adb does not keep reading the
// file after the check); an update in place (install -r) changes base.apk's
// stat (the cache key). The host's refusal of the §5 attack shapes is
// phoneapp/apksig_test.go's; installing a real attacker (API 30, targetSdk 29)
// and the two older images (API 30 and 33, not installed) stay for the manual
// check before the first release (docs/releasing.md).
func TestTheInstalledAppReadsBackByteForByte(t *testing.T) {
	e := newEmu(t)
	abs, want := readAPK(t, appAPK)
	e.installApp(t)

	// pm path: exactly one package:/data/app/<...>/base.apk.
	out := e.shell(t, shq.Join("pm", "path", phoneapp.Package))
	lines := nonEmptyLines(out)
	if len(lines) != 1 {
		t.Fatalf("pm path gave %d lines: %q", len(lines), out)
	}
	path, ok := strings.CutPrefix(lines[0], "package:")
	if !ok || !basePath(path) {
		t.Fatalf("pm path = %q, want one package:/data/app/<...>/base.apk", lines[0])
	}

	// stat -c '%s %i %Y': size, inode, mtime, as statKey parses it.
	stat := e.shell(t, shq.Join("stat", "-c", "%s %i %Y", path))
	size, inode, mtime, okStat := threeInts(stat)
	if !okStat {
		t.Fatalf("stat = %q, want three integers (size inode mtime)", stat)
	}
	if size != int64(len(want)) {
		t.Errorf("stat size %d, local APK %d bytes", size, len(want))
	}

	// base64: the APK read back is byte for byte the file installed.
	got := e.base64(t, path, size)
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("the APK read back (%d bytes) is not the one installed (%d bytes) from %s", len(got), len(want), abs)
	}

	// install -r keeps the data and changes base.apk's stat (the cache key).
	if out := e.run(t, "install", "-r", "--no-incremental", abs); !strings.Contains(out, "Success") {
		t.Fatalf("install -r: %s", out)
	}
	// mtime is whole seconds: wait for the inode or the mtime to move.
	var stat2 string
	if !poll(5*time.Second, func() bool {
		path2 := firstPath(e.shell(t, shq.Join("pm", "path", phoneapp.Package)))
		stat2 = e.shell(t, shq.Join("stat", "-c", "%s %i %Y", path2))
		_, i2, m2, ok := threeInts(stat2)
		return ok && (i2 != inode || m2 != mtime)
	}) {
		t.Errorf("stat %q unchanged after install -r (was inode %d mtime %d): the cache key would not move", stat2, inode, mtime)
	}
}

// Validation 3 of M6a, the provider's authority: Android installs no second
// app that claims tech.hybriq.adbuddy.bridge (INSTALL_FAILED_CONFLICTING_PROVIDER),
// even with ADBuddy's own provider disabled, so content://…bridge stays the
// verified app's (spec M6a §6.3, Me-4).
func TestASecondProviderOnTheAuthorityIsRefused(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	conflict := e.buildConflictAPK(t)
	e.uninstallAtCleanup(t, conflictPackage)

	refused := func(what string) {
		out, errOut, _, _ := e.adb.Output(e.ctx, "install", "-r", "--no-incremental", conflict)
		msg := strings.ReplaceAll(out+errOut, "\r", "")
		if !strings.Contains(msg, "INSTALL_FAILED_CONFLICTING_PROVIDER") {
			t.Errorf("installing a second provider on the authority %s: %q", what, strings.TrimSpace(msg))
		}
	}
	refused("with ADBuddy's provider enabled")

	// With ADBuddy's provider disabled the authority is still taken.
	e.atCleanup(t, "shell", shq.Join("pm", "enable", providerComponent))
	e.shell(t, shq.Join("pm", "disable", providerComponent))
	refused("with ADBuddy's provider disabled")
}

// base64 reads the n bytes of path on the emulator and decodes them.
func (e *emu) base64(t *testing.T, path string, n int64) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, pullOnEmu)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", shq.Join("base64", path))
	if err != nil || code != 0 {
		t.Fatalf("base64 %s: exit %d, %v: %s", path, code, err, strings.TrimSpace(errOut))
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out), ""))
	if err != nil {
		t.Fatalf("decode base64 of %s: %v", path, err)
	}
	if int64(len(data)) != n {
		t.Fatalf("base64 of %s gave %d bytes, stat said %d", path, len(data), n)
	}
	return data
}

// pullOnEmu bounds reading the APK over the emulator's adb (local, fast).
const pullOnEmu = 2 * time.Minute

const (
	conflictPackage   = "tech.hybriq.adbuddy.conflict"
	providerComponent = phoneapp.Package + "/.bridge.TokenProvider"
)

// conflictManifest declares a provider on ADBuddy's authority, in another
// package. hasCode="false": no dex, so aapt2 alone builds it.
const conflictManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="` + conflictPackage + `">
  <application android:hasCode="false">
    <provider android:name="android.content.ContentProvider" android:authorities="` + phoneapp.Authority + `" android:exported="false"/>
  </application>
</manifest>
`

// buildConflictAPK builds and signs (throwaway key, never the release one) a
// minimal APK that claims ADBuddy's provider authority, or skips without the
// SDK tools.
func (e *emu) buildConflictAPK(t *testing.T) string {
	t.Helper()
	tl := findTools(t)
	dir := t.TempDir()
	manifest := filepath.Join(dir, "AndroidManifest.xml")
	if err := os.WriteFile(manifest, []byte(conflictManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	unsigned := filepath.Join(dir, "conflict-unsigned.apk")
	tl.run(t, nil, tl.aapt2, "link", "-o", unsigned, "--manifest", manifest, "-I", tl.androidJar, "--min-sdk-version", "30")
	signed := filepath.Join(dir, "conflict.apk")
	ks, pass := tl.newKey(t, dir, "emutest-conflict")
	tl.sign(t, unsigned, signed, ks, pass, "--v1-signing-enabled", "false", "--v2-signing-enabled", "true", "--v3-signing-enabled", "true")
	return signed
}

// nonEmptyLines splits out into its non-empty, trimmed lines.
func nonEmptyLines(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// basePath reports whether p is a /data/app/<...>/base.apk path.
func basePath(p string) bool {
	return strings.HasPrefix(p, "/data/app/") && strings.HasSuffix(p, "/base.apk") && !strings.Contains(p, "..")
}

// firstPath returns the first package: path of pm path output.
func firstPath(out string) string {
	for _, l := range nonEmptyLines(out) {
		if p, ok := strings.CutPrefix(l, "package:"); ok {
			return p
		}
	}
	return ""
}

// threeInts parses "a b c" into three int64 (stat -c '%s %i %Y').
func threeInts(s string) (a, b, c int64, ok bool) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if a, err = strconv.ParseInt(f[0], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if b, err = strconv.ParseInt(f[1], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if c, err = strconv.ParseInt(f[2], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	return a, b, c, true
}
```

- [ ] **Step 2: compila y tests sin emulador.** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/... && go test -count=1 -timeout 5m ./internal/emutest/` (los dos tests nuevos, `SKIP` sin emulador).

- [ ] **Step 3: GREEN y RED en el emulador.** `cd host && go test -count=1 -timeout 30m -v -run 'TestTheInstalledAppReadsBackByteForByte|TestASecondProviderOnTheAuthorityIsRefused' ./internal/emutest/`, salida al ledger. RED (una cada vez, revertida):
  1. En `TestTheInstalledAppReadsBackByteForByte`, justo después de `abs, want := readAPK(t, appAPK)`, añadir `want = want[:len(want)-1]`: falla `stat size … local APK … bytes` y la comparación byte a byte. Prueba que compara el contenido y el tamaño.
  2. En `conflictManifest`, cambiar `phoneapp.Authority` por `"tech.hybriq.other.bridge"`: Android instala el APK y `TestASecondProviderOnTheAuthorityIsRefused` falla en los dos casos (sin `INSTALL_FAILED_CONFLICTING_PROVIDER`). Prueba que choca la autoridad y nada más. (El cleanup desinstala ese paquete.)
  - Apagar el emulador al terminar (por serial, comprobado por PID).

- [ ] **Step 4: commit.** `test(emutest): pm path, stat, base64, install -r y la autoridad duplicada en la API 34 (M6a-3)`.

---

### Task 4: el inventario, según la evidencia

**Files:** `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (CRLF). Cada fila es una sola línea: el "Reemplazar" es la línea entera de hoy.

**Regla (decisión del controlador):** una fila pasa a "hecha" (variante **A**) solo si el ledger tiene, para cada test que cita, la salida GREEN **y** la RED indicada en su tarea. Si falta algo, la fila usa la variante **B** ("test escrito, pendiente de ejecutar en emulador"). `<fecha>` es la fecha real de la ejecución que está en el ledger (`AAAA-MM-DD`); `<resultado del reinicio>` es lo que registró el GREEN de VB-41. No se mezclan variantes dentro de una fila.

- [ ] **Step 1: fila M5b-12.**

**Reemplazar:**
```
| M5b-12 | `tcpip 5555` por Tailscale con solo datos; `remote on` y `off`; caducidad | Parcial, lógica hecha (adb falso) | T8: `remote.New(adb.Runner{Serial: emu})`, `Enable` y `Disable`, leyendo `service.adb.tcp.port` (medido en S5). | Tailscale con datos móviles. Paso: fuera de casa o con el Wi-Fi apagado, `remote on`, y `serve` reconecta. | medio | T8 + Pixel |
```
**por (A):**
```
| M5b-12 | `tcpip 5555` por Tailscale con solo datos; `remote on` y `off`; caducidad | Parcial; lo del emulador, hecho (T8) | T8: `TestTheRemoteModeOpensAndClosesThePort` (con `ADBUDDY_EMULATOR_ADBD=1`; la CI no lo corre): el controlador real `remote.New` contra el adbd del emulador (`Enable`, `Reconcile`, `Disable` y `Expire` con reloj falso), leyendo `service.adb.tcp.port` y el pid de adbd, nunca el socket (el emulador escucha siempre en 5555). GREEN y RED en el ledger de T8 (<fecha>). | Tailscale con datos móviles y que `serve` reconecte por `<ip-tailscale>:5555`. Paso: fuera de casa o con el Wi-Fi apagado, `remote on`, y `serve` reconecta. | medio | T8 (hecha) + Pixel |
```
**o por (B):**
```
| M5b-12 | `tcpip 5555` por Tailscale con solo datos; `remote on` y `off`; caducidad | Parcial, lógica hecha (adb falso); test de emulador escrito (T8), pendiente de ejecutar | T8: test escrito, pendiente de ejecutar en emulador: `TestTheRemoteModeOpensAndClosesThePort` (con `ADBUDDY_EMULATOR_ADBD=1`), el controlador real `remote.New` contra el adbd del emulador, leyendo `service.adb.tcp.port` (medido en S5). | Tailscale con datos móviles. Paso: fuera de casa o con el Wi-Fi apagado, `remote on`, y `serve` reconecta. | medio | T8 (pendiente de ejecutar) + Pixel |
```

- [ ] **Step 2: fila M5b-13.**

**Reemplazar:**
```
| M5b-13 | Segundos que tarda en verse el estado tras `adb tcpip`; "desconocido" nunca se muestra como "apagado" | Parcial, lógica hecha (`BridgeStateTest`) | T8: medido con el arnés. | La cifra del Pixel, informativa. Sin paso. | bajo | T8 |
```
**por (A):**
```
| M5b-13 | Segundos que tarda en verse el estado tras `adb tcpip`; "desconocido" nunca se muestra como "apagado" | Parcial; la lógica en la JVM (`BridgeStateTest`, `RemoteUiTest`) y la integración en el emulador, hecha (T8) | T8: `TestTheCardSaysUnknownWhenTheAnswerIsLost` (con `ADBUDDY_EMULATOR_ADBD=1`): `remote.set` desde la tarjeta, el `Enable` real y la respuesta perdida; al volver la app la tarjeta dice "Unknown" 5 s, nunca "Off", y luego "Until …". RED de producto: sin `mark = UnknownMark(...)` (`RemoteCard.kt:69`) dice "Off". Segundos hasta que vuelve la app en el emulador: <segundos> (cota inferior: el arnés reabre el túnel al momento; `serve` mira cada 15 s). GREEN y RED en el ledger de T8 (<fecha>). | La cifra del Pixel, informativa. Sin paso. | bajo | T8 (hecha) |
```
**o por (B):**
```
| M5b-13 | Segundos que tarda en verse el estado tras `adb tcpip`; "desconocido" nunca se muestra como "apagado" | Parcial, lógica hecha (`BridgeStateTest`, `RemoteUiTest`); test de emulador escrito (T8), pendiente de ejecutar | T8: test escrito, pendiente de ejecutar en emulador: `TestTheCardSaysUnknownWhenTheAnswerIsLost` (con `ADBUDDY_EMULATOR_ADBD=1`), la respuesta perdida tras el `Enable` real; su RED de producto (`RemoteCard.kt:69`) solo se ve en el emulador. | La cifra del Pixel, informativa. Sin paso. | bajo | T8 (pendiente de ejecutar) |
```

- [ ] **Step 3: fila M6a-2.**

**Reemplazar:**
```
| M6a-2 | La app de release (R8) funciona: la serialización con kotlinx | Sí | T8: el APK de `release-check` firmado con una clave de prueba solo para el emulador (nunca la de release), instalado en la API 34; el arnés (T3) comprueba el saludo y los mensajes. | — | alto | T8 |
```
**por (A):**
```
| M6a-2 | La app de release (R8) funciona: la serialización con kotlinx | Sí, hecha (T8) | T8: `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey` (sin emulador: R8 encogió el APK y el host lee su firma; RED: `isMinifyEnabled = false`) y `TestTheReleaseAppTalksOverTheLink` (APK de `:app:assembleRelease` firmado con una clave temporal en `t.TempDir()`, nunca la de release, instalado y verificado como release: `hello`, `device.info`, `chat.message`, `ui.request` y `confirm.request` por el arnés T3). La serialización bajo R8 no tiene RED de producto: es una guarda de regresión. La CI no los corre (no compila el release). GREEN y RED en el ledger de T8 (<fecha>). | — | alto | T8 (hecha) |
```
**o por (B):**
```
| M6a-2 | La app de release (R8) funciona: la serialización con kotlinx | Sí; la parte del PC hecha (T8), la del emulador escrita y pendiente de ejecutar | T8: `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey` (R8 encogió el APK y el host lee su firma; RED `isMinifyEnabled = false` vista en la réplica del plan); `TestTheReleaseAppTalksOverTheLink`, test escrito, pendiente de ejecutar en emulador (el saludo y los mensajes del APK de release por el arnés T3). | — | alto | T8 (parte del emulador pendiente de ejecutar) |
```

- [ ] **Step 4: fila M6a-3.**

**Reemplazar:**
```
| M6a-3 | Lo que la spec da por hecho de Android: APK de ataque, salidas de `pm path`/`stat`/`base64`, `install -r`, `base.apk` idéntico, autoridad duplicada | Parcial | T8: todo en la API 34. | Las API 30 y 33 piden imágenes no instaladas (decide el usuario), y la última es la 37 rota. | alto | T8 |
```
**por (A):**
```
| M6a-3 | Lo que la spec da por hecho de Android: APK de ataque, salidas de `pm path`/`stat`/`base64`, `install -r`, `base.apk` idéntico, autoridad duplicada | Parcial; lo de la API 34, hecho (T8) | T8: `TestTheInstalledAppReadsBackByteForByte` (`pm path`, `stat -c '%s %i %Y'`, `base64` byte a byte con `--no-incremental`, `install -r` mueve el `stat`) y `TestASecondProviderOnTheAuthorityIsRefused` (un APK de aapt2 con la autoridad del provider: `INSTALL_FAILED_CONFLICTING_PROVIDER`, también con el de ADBuddy desactivado); los dos corren en la CI (el segundo si hay build-tools y JDK). Que el host rechaza las formas de ataque de §5 ya lo cubre `phoneapp/apksig_test.go`. GREEN y RED en el ledger de T8 (<fecha>). | Instalar un ataque real (vuelta a v1 con `targetSdk` 29) y las API 30 y 33, sin imagen instalada (decide el usuario); la 37 está rota. Paso: la validación 3 de `docs/releasing.md`, antes de la primera release pública. | alto | T8 (API 34) + validación 3 |
```
**o por (B):**
```
| M6a-3 | Lo que la spec da por hecho de Android: APK de ataque, salidas de `pm path`/`stat`/`base64`, `install -r`, `base.apk` idéntico, autoridad duplicada | Parcial; tests de la API 34 escritos (T8), pendientes de ejecutar | T8: tests escritos, pendientes de ejecutar en emulador: `TestTheInstalledAppReadsBackByteForByte` y `TestASecondProviderOnTheAuthorityIsRefused`. Que el host rechaza las formas de ataque de §5 ya lo cubre `phoneapp/apksig_test.go`. | Instalar un ataque real (API 30, `targetSdk` 29) y las API 30 y 33, sin imagen instalada (decide el usuario); la 37 está rota. Paso: la validación 3 de `docs/releasing.md`. | alto | T8 (pendiente de ejecutar) + validación 3 |
```

- [ ] **Step 5: fila VB-37.**

**Reemplazar:**
```
| VB-37 | "Apagar" con el modo remoto apagado no corta la conexión; un 5555 o un 7777 abiertos a mano se cierran | Sí | T8: `remote` contra el emulador; el registro de estados (`device` o `gone`) muestra si adbd se reinició, porque `adb usb` lo reinicia en ~1 s (S5). | — | medio | T8 |
```
**por (A):**
```
| VB-37 | "Apagar" con el modo remoto apagado no corta la conexión; un 5555 o un 7777 abiertos a mano se cierran | Sí, hecha (T8) | T8: `TestOffWithTheModeOffOnlyClosesAnOpenPort` (con `ADBUDDY_EMULATOR_ADBD=1`; la CI no lo corre): con el puerto cerrado `Disable` no reinicia adbd (mismo pid durante 5 s); con el 5555 o el 7777 abiertos a mano los cierra con `adb usb` (pid nuevo, `service.adb.tcp.port` a 0). GREEN y RED en el ledger de T8 (<fecha>). | — | medio | T8 (hecha) |
```
**o por (B):**
```
| VB-37 | "Apagar" con el modo remoto apagado no corta la conexión; un 5555 o un 7777 abiertos a mano se cierran | Sí; test escrito (T8), pendiente de ejecutar | T8: test escrito, pendiente de ejecutar en emulador: `TestOffWithTheModeOffOnlyClosesAnOpenPort` (con `ADBUDDY_EMULATOR_ADBD=1`), el pid de adbd dice si `Disable` lo reinició. | — | medio | T8 (pendiente de ejecutar) |
```

- [ ] **Step 6: fila VB-41.**

**Reemplazar:**
```
| VB-41 | `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd | Sí | T8: `su 0 setprop persist.adb.tcp.port 5555` (`su 0` medido), `adb usb`, reinicio de adbd y `service.adb.tcp.port`. | — | medio | T8 |
```
**por (A):**
```
| VB-41 | `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd | Sí, medida (T8) | T8: `TestPersistTCPPortAgainstUSBAndRestarts` (con `ADBUDDY_EMULATOR_ADBD=1` y, para el reinicio, `ADBUDDY_EMULATOR_REBOOT=1`): `su 0 setprop persist.adb.tcp.port 5555`, `ctl.restart adbd`, `adb tcpip` y `adb usb`, y `adb reboot`; graba las propiedades (el emulador escucha siempre en 5555: qué puerto abre adbd no se ve). Resultado del reinicio, en el ledger de T8 (<fecha>): <resultado del reinicio>. | Si el reinicio dejó `service.adb.tcp.port` vacío con `persist` fijado: hallazgo pendiente de adjudicar (el host no lee `persist.adb.tcp.port`). | medio | T8 (medida); hallazgo pendiente de adjudicar si se confirmó |
```
**o por (B):**
```
| VB-41 | `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd | Sí; test escrito (T8), pendiente de ejecutar | T8: test escrito, pendiente de ejecutar en emulador: `TestPersistTCPPortAgainstUSBAndRestarts` (con `ADBUDDY_EMULATOR_ADBD=1` y `ADBUDDY_EMULATOR_REBOOT=1`); mide la hipótesis de que el host toma por cerrado un puerto de `persist` tras un reinicio. | — | medio | T8 (pendiente de ejecutar) |
```

- [ ] **Step 7: la entrada T8 del "Plan de automatización propuesto".**

**Reemplazar:**
```
- RED por mutación: un `Disable` que lanza siempre `adb usb` hace que la VB-37 vea `gone`. GREEN: con el código de hoy.
```
**por (si las seis filas quedaron en A):**
```
- **Hecha el <fecha>** (rama `test/validaciones-t8`, plan `docs/superpowers/plans/2026-10-04-adbuddy-validaciones-t8.md`): `remote_test.go`, `release_test.go` e `install_test.go`, con GREEN y RED de cada test en el ledger. Los que reinician adbd van con `ADBUDDY_EMULATOR_ADBD=1` y la CI no los corre. VB-41: <resultado del reinicio>.
```
**o por (si alguna quedó en B):**
```
- Plan `docs/superpowers/plans/2026-10-04-adbuddy-validaciones-t8.md` (rama `test/validaciones-t8`): tests escritos; ejecutados con GREEN y RED: <filas en A>; pendientes de ejecutar en emulador: <filas en B>. Los que reinician adbd van con `ADBUDDY_EMULATOR_ADBD=1` y la CI no los corre.
```

- [ ] **Step 8: comprobar y commit.** `git diff --stat` (solo el inventario), CRLF conservado (`git diff` sin cambios de fin de línea en otras líneas). Commit `docs(validaciones): filas de la T8 según la evidencia del ledger`.

---

### Task 5: "off" cierra también un puerto de `persist.adb.tcp.port` (VB-41, seguridad) 🔒 — RED esperado

**Por qué (ruling del supervisor):** un adbd que sigue escuchando por TCP después de `remote off` o del invariante de `serve` (`EnsureNoTCPPort`) es justo el puerto abierto que el usuario cree cerrado y que ADBuddy debe impedir. La Tarea 1 solo **registra** el hueco (`TestPersistTCPPortAgainstUSBAndRestarts`, `FINDING (VB-41)`); esta tarea añade un test que lo **afirma**. No cambia nada de lo ya implementado (35a625d, 2731594, a075578, 5e30c1e): es un fichero nuevo que reutiliza los ayudantes de `remote_test.go`.

**Files:** `host/internal/emutest/persist_test.go` (nuevo).

- Produces: `TestOffClosesAPersistTCPPort` (subtests `Disable after adb tcpip` y `EnsureNoTCPPort with service.adb.tcp.port empty`), `persistPort`, `withPersist`, `closedAfterOff`.
- Reutiliza de `remote_test.go`: `adbdTests`, `closedAtStartAndEnd`, `adbd`, `adbdNow`, `restartedTo`, `sameAdbd`, `portClosed`, `memStore`, `rebootAndWait`, `tryShell`, `restartHint`; de `link_test.go`, `poll`. El controlador real `remote.New` y `device.EnsureNoTCPPort`, como en la Tarea 1.

**Qué afirma.** Con `persist.adb.tcp.port=5555` (puesto con `su 0`: el usuario `shell` puede no poder), tras el "off" del host:
1. `getprop persist.adb.tcp.port` vacío;
2. `service.adb.tcp.port` cerrado: **vacío o `0`**. Son los valores medidos (inventario, "Datos medidos": `adb usb` lo deja en `0`; al arrancar, vacío) y el criterio del host (`portClosed`, `device.TCPPort`). `-1` no está medido: el host lo trataría como abierto (lado seguro) y aquí cuenta como no cerrado;
3. adbd **reiniciado**: un pid nuevo y estable (`sameAdbd` 5 s), porque un adbd que no se reinició puede seguir escuchando en el puerto de `persist` que tomó al arrancar.

Dos caminos del "off":
- **`Disable after adb tcpip`:** `persist` y el puerto abierto a mano (`adb tcpip 5555`); `ctl.Disable` con el modo apagado (`memStore` sin caducidad).
- **`EnsureNoTCPPort with service.adb.tcp.port empty`:** `persist` y `service.adb.tcp.port` vacío, el estado de después de un reinicio (adbd de AOSP lee `persist` solo con la propiedad de servicio vacía). Con `ADBUDDY_EMULATOR_REBOOT=1` se llega con `adb reboot`; sin él, con `su 0 setprop service.adb.tcp.port ''` y `su 0 setprop ctl.restart adbd` (no medido: si la propiedad no queda vacía, el subtest hace `Skip` y pide `ADBUDDY_EMULATOR_REBOOT=1`). Luego `device.EnsureNoTCPPort(ctx, e.adb, "")`, el invariante de `serve` sin modo remoto.

**Qué se espera hoy, leyendo el código: ROJO.**
- `host/internal/remote/remote.go` (`Disable`, `disable`, `Reconcile`, `Enable`) y `host/internal/device/device.go` (`TCPPort`, `EnsureNoTCPPort`) solo leen `service.adb.tcp.port`; **nadie lee ni limpia `persist.adb.tcp.port`**.
- Subtest 1: `Disable` ve `5555` y lanza `adb usb` (pid nuevo, `service.adb.tcp.port` a `0`), pero `adb usb` de adbd solo escribe la propiedad de servicio: `persist` sigue en `5555` → **falla la afirmación 1** (`persist.adb.tcp.port = "5555" after off`). (Lo que deja `adb usb` en `persist` también lo registra la Tarea 1, `persist.adb.tcp.port after adb usb`.)
- Subtest 2: `TCPPort` lee `""` y devuelve "sin puerto"; `EnsureNoTCPPort` no lanza `adb usb` → **fallan la 1 y la 3** (`persist` sigue en `5555` y `adbd still has pid … after off`), mientras adbd, según AOSP, escucha en el `5555` de `persist` (en el emulador no se ve: escucha siempre en 5555).

**El test es un RED esperado y es el test de aceptación del arreglo.** Si sale rojo con emulador, **no se arregla en esta rama**: el arreglo va en una rama y un plan cortos aparte (ver "Forma del arreglo"), que pondrán este test en verde (y `hostReadsPersistPort = true` en la Tarea 1). Si saliera verde, se anota en el ledger y la hipótesis de VB-41 queda descartada. Mientras el arreglo no exista, este test no entra en la CI (va tras `ADBUDDY_EMULATOR_ADBD=1`) y la Tarea 4 no lo cuenta como "hecho".

- [ ] **Step 1: `host/internal/emutest/persist_test.go`,** completo:

```go
package emutest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// persistPort is the port the test leaves in persist.adb.tcp.port: 5555, the
// emulator's own transport port (M2.5b S5), so a persist the cleanup failed to
// clear never moves the emulator's adbd to an unmeasured port.
const persistPort = remote.Port

// withPersist sets persist.adb.tcp.port (su 0: the shell user may be denied)
// and makes sure the cleanup empties it again and restarts adbd, so the
// emulator ends without a persist port nor an adbd that took one. Cleanups run
// last first: this one runs before closedAtStartAndEnd's adb usb.
func (e *emu) withPersist(t *testing.T) func() string {
	t.Helper()
	persist := func() string { return e.shell(t, "su 0 getprop persist.adb.tcp.port") }
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q before the test: left over; clear it with `adb shell su 0 setprop persist.adb.tcp.port ''`", p)
	}
	t.Cleanup(func() {
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		if c.state() != "device" {
			t.Errorf("the emulator is %q at the cleanup: persist.adb.tcp.port may be left at %s; %s", c.state(), persistPort, restartHint)
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "shell", "su 0 setprop persist.adb.tcp.port ''")
		pid, _, _ := c.adbdNow()
		_, _, _, _ = c.adb.Output(ctx, "shell", "su 0 setprop ctl.restart adbd")
		if !poll(60*time.Second, func() bool {
			now, _, ok := c.adbdNow()
			return ok && now != pid && c.tryShell("su 0 getprop persist.adb.tcp.port") == ""
		}) {
			t.Errorf("persist.adb.tcp.port not empty, or adbd not back, after the cleanup; %s", restartHint)
		}
	})
	e.shell(t, "su 0 setprop persist.adb.tcp.port "+persistPort)
	if p := persist(); p != persistPort {
		t.Fatalf("persist.adb.tcp.port = %q after su 0 setprop, want %s", p, persistPort)
	}
	return persist
}

// closedAfterOff is what this test asks of the host's "off" with a persist
// port set: persist.adb.tcp.port emptied, service.adb.tcp.port closed (empty
// or 0: the values measured after adb usb and at boot, and the host's own
// criterion, portClosed; -1 is not measured and the host would take it as
// open, the safe side) and adbd restarted (a new pid that stays: it cannot
// still be listening on the persist port it took when it started).
func (e *emu) closedAfterOff(t *testing.T, persist func() string, before string) {
	t.Helper()
	if p := persist(); p != "" {
		t.Errorf("persist.adb.tcp.port = %q after off, want it emptied: adbd takes it again at its next start with service.adb.tcp.port empty (a reboot)", p)
	}
	pid, port, ok := e.adbdNow()
	if !ok {
		t.Fatalf("could not read adbd after off (emulator %q)", e.state())
	}
	if !portClosed(port) {
		t.Errorf("service.adb.tcp.port = %q after off, want empty or 0", port)
	}
	if pid == before {
		t.Errorf("adbd still has pid %s after off: it was not restarted, and may still listen on the persist port it took when it started", pid)
		return
	}
	e.sameAdbd(t, pid, port)
}

// Security of VB-41 (supervisor's ruling): "off" must leave adbd without a
// TCP port, persist.adb.tcp.port included. The host's two "off" paths, the
// real remote controller's Disable (mode off) and device.EnsureNoTCPPort (the
// invariant at serve's start), run against the emulator's adbd with
// persist.adb.tcp.port=5555. RED EXPECTED TODAY: remote.go and device.go read
// only service.adb.tcp.port and never touch persist, so persist stays 5555
// (and with service.adb.tcp.port empty they do not even restart adbd). A red
// run is the finding: the fix goes in its own branch and plan; this test is
// its acceptance test.
func TestOffClosesAPersistTCPPort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)

	// persist set and the port opened by hand (adb tcpip): Disable with the
	// mode off sees 5555 and runs adb usb, which leaves persist as it was.
	t.Run("Disable after adb tcpip", func(t *testing.T) {
		persist := e.withPersist(t)
		pid, _ := e.adbd(t)
		e.run(t, "tcpip", remote.Port)
		pid = e.restartedTo(t, pid, remote.Port)
		ctl := remote.New(e.adb, &memStore{}, nil)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
			t.Fatalf("Disable: %+v, %v", s, err)
		}
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
	})

	// persist set and service.adb.tcp.port empty, as after a reboot: adbd took
	// the persist port at its start (AOSP adbd reads persist only when the
	// service property is empty), and the host reads "closed".
	t.Run("EnsureNoTCPPort with service.adb.tcp.port empty", func(t *testing.T) {
		persist := e.withPersist(t)
		if os.Getenv("ADBUDDY_EMULATOR_REBOOT") == "1" {
			e.rebootAndWait(t)
		} else {
			// Without a reboot: the service property emptied and adbd
			// restarted by init. Not measured yet: if the property does not
			// read empty, this case needs ADBUDDY_EMULATOR_REBOOT=1.
			pid, _ := e.adbd(t)
			_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop service.adb.tcp.port ''")
			_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
			e.restartedTo(t, pid, "", "0")
		}
		pid, port := e.adbd(t)
		if port != "" {
			t.Skipf("service.adb.tcp.port = %q, not empty: this case needs ADBUDDY_EMULATOR_REBOOT=1", port)
		}
		if p := persist(); p != persistPort {
			t.Fatalf("persist.adb.tcp.port = %q before off, want %s", p, persistPort)
		}
		restarted, err := device.EnsureNoTCPPort(e.ctx, e.adb, "")
		if err != nil {
			t.Fatalf("EnsureNoTCPPort: %v", err)
		}
		t.Logf("EnsureNoTCPPort ran adb usb: %v", restarted)
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
	})
}
```

- [ ] **Step 2: compila y tests sin emulador.** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/... && go test -count=1 -timeout 5m ./internal/emutest/` → `ok`; `TestOffClosesAPersistTCPPort` hace `SKIP` sin `ADBUDDY_EMULATOR_ADBD=1`.

- [ ] **Step 3: la ejecución (RED esperado), con emulador libre.** Con el bloque de variables de "Global Constraints":
```bash
cd host && ADBUDDY_EMULATOR_ADBD=1 ADBUDDY_EMULATOR_REBOOT=1 go test -count=1 -timeout 30m -v -run TestOffClosesAPersistTCPPort ./internal/emutest/
```
Salida entera al ledger. Lo esperado: `FAIL` en los dos subtests con los mensajes de arriba. Después, comprobar a mano que el emulador quedó como estaba: `adb -P 5090 -s emulator-5630 shell su 0 getprop persist.adb.tcp.port` vacío y `getprop service.adb.tcp.port` vacío o `0` (los cleanups lo hacen; si no, `su 0 setprop persist.adb.tcp.port ''`, `su 0 setprop ctl.restart adbd` y, si hace falta, `adb usb`). Apagar el emulador por su serial (`scripts/emulator.sh stop`) y comprobar por PID; nunca matar por nombre.
  - **Control de que el test mira lo que dice** (en lugar de una RED, que hoy es el estado normal): con la línea `e.shell(t, "su 0 setprop persist.adb.tcp.port "+persistPort)` de `withPersist` comentada (y su comprobación), el subtest 1 debe pasar (persist vacío, `adb usb` real): prueba que el rojo de hoy viene de `persist` y no del resto. Revertir.

- [ ] **Step 4: commit.** `test(emutest): off debe cerrar también persist.adb.tcp.port (VB-41, RED esperado)`, con el trailer exacto.

#### Forma del arreglo (para el siguiente plan; sin código)

- **Dónde:** un solo criterio de "puerto TCP abierto" en `device` (p. ej. leer `service.adb.tcp.port` y `persist.adb.tcp.port` en una orden), usado por `device.TCPPort`/`EnsureNoTCPPort` y por `remote` (`Disable`, `disable`, `Reconcile`, la rama de fallo de `Enable`), en lugar de los cinco `getprop service.adb.tcp.port` sueltos de hoy. Abierto = servicio con puerto, **o** `persist` con puerto.
- **Leer `persist` sin root:** no está medido. Si SELinux deniega la lectura al usuario `shell`, `getprop` devuelve vacío con código 0 (es lo que pasa con `service.adb.tls.port`, T1-12; `service.adb.tcp.port` sí se lee) y el host no podría distinguir "vacío" de "denegado". Lo primero del plan siguiente es medirlo en el emulador sin `su` (y en el Pixel en la pasada de solo lectura).
- **Limpiar `persist` sin root:** `setprop persist.adb.tcp.port ""` como `shell` puede estar denegado en un móvil de producción (la Tarea 1 registra si en la imagen userdebug lo está). Sin root no hay otra forma de borrar una propiedad `persist`.
- **Lo que sí se puede sin root:** `adb usb` escribe `service.adb.tcp.port=0` y reinicia adbd; con la propiedad de servicio no vacía, adbd de AOSP **no** lee `persist`, así que el puerto queda cerrado **hasta el siguiente arranque** (al arrancar la propiedad de servicio vuelve vacía y `persist` manda otra vez). De ahí: (a) el invariante de `serve` tiene que lanzar `adb usb` también cuando `service.adb.tcp.port` está vacío y `persist` tiene puerto (el caso del subtest 2); (b) si no puede vaciar `persist`, decirlo al usuario en `serve` y en `doctor` (alguien con root fijó un puerto que se reabre en cada arranque) en lugar de darlo por cerrado.
- **El test de aceptación:** este. Si limpiar `persist` resulta imposible sin root, la afirmación 1 no se puede cumplir con ningún arreglo del host: el plan siguiente decide con el usuario si se relaja a "cerrado hasta el arranque, con aviso" (afirmaciones 2 y 3, más el aviso) y lo dice en el test.
- **Fuera del arreglo:** el clasificador ya hace 🔴 `setprop persist.adb…` para el agente; esto es lo que hace el host con sus propias órdenes, que no pasan por el shim.

---

## Réplica acumulada (resultado)

Sobre `git archive origin/main` (2b8c07d, tras `git fetch`; `main` no había avanzado), aplicando el texto de este plan:

- **Tras T1** (`link_test.go` + `remote_test.go`), **tras T1+T2** (+ `release_test.go`) y **tras T1+T2+T3** (+ `install_test.go`), cada una en su propia copia: `GOOS=windows go build ./...` sin salida, `gofmt -l internal/emutest` vacío, `GOOS=windows go vet ./internal/emutest/...` sin salida y `go test -count=1 -timeout 5m ./internal/emutest/` → `ok` en las tres etapas (sin emulador: los tests que lo piden, `SKIP`; los de `ADBUDDY_EMULATOR_ADBD`, también).
- **APK:** `gradle --offline -q :app:assembleDebug :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101` → exit 0. Con ellos, `go test -v -run 'TestVersionLess|TestTheReleaseAPKIsShrunk' ./internal/emutest/` → `--- PASS: TestVersionLess`, `--- PASS: TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey (1.82s)` (el de debug como control: `RemoteUi` sí está en su dex).
- **RED del shrink** (réplica de la v1, mismo test): con `isMinifyEnabled = false` e `isShrinkResources = false`, `--- FAIL: TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey … Ltech/hybriq/adbuddy/ui/RemoteUi; is in the release APK's dex: it was not shrunk by R8`; revertido y reconstruido, `ok`. `build.gradle.kts` quedó idéntico a `origin/main`.
- **APK de conflicto** (la cadena de `buildConflictAPK`, a mano): `aapt2 link` del manifiesto con la autoridad `tech.hybriq.adbuddy.bridge` → APK de 799 bytes (`AndroidManifest.xml`, `resources.arsc`); `zipalign` y `apksigner` con una clave temporal de `keytool` → firmado; `aapt2 dump badging` → `package: name='tech.hybriq.adbuddy.conflict'`.
- **Sin emulador** no se ejecutó: ningún GREEN ni RED de adbd, de la tarjeta (incluida la RED de producto de `RemoteCard.kt:69`), del release instalado ni de la instalación. La RED de `RemoteCard.kt:69` no tiene camino en la JVM (ningún test ejercita la rama `reply == null`): queda pendiente de ejecutar en el emulador.

**Tarea 5 (adenda v3):** sobre `git archive` de la rama en 5e30c1e (las Tareas 1-3 ya implementadas; `origin/main` sigue en 2b8c07d), con `persist_test.go` añadido desde el texto de este plan: `GOOS=windows go build ./...` sin salida, `gofmt -l internal/emutest` vacío, `GOOS=windows go vet ./internal/emutest/...` sin salida, `go test -count=1 -timeout 5m ./internal/emutest/` → `ok`, y `-v -run TestOffClosesAPersistTCPPort` → `--- SKIP` (sin `ADBUDDY_EMULATOR_ADBD=1`). Sin emulador: el RED esperado no se ha ejecutado.

## Validaciones manuales pendientes (lo que T8 deja en el Pixel)

- **M5b-12, M5b-15 y M5b-16:** Tailscale con datos móviles, el cambio de red en sesión y la reconexión por `<ip-tailscale>:5555`.
- **M5b-13:** la cifra de segundos del Pixel (informativa).
- **M6a-3:** los APK de ataque de §5 **instalados** y la vuelta a v1, en las API 30 y 33 (imágenes no instaladas) y en la última cuando haya una que funcione: la validación 3 de `docs/releasing.md`, antes de la primera release pública.
- **M6a-4:** la clave, el environment, los secretos y la primera etiqueta: solo el usuario.
- **Todas las filas que la Tarea 4 deje en la variante B:** su ejecución en el emulador, cuando haya uno libre.
