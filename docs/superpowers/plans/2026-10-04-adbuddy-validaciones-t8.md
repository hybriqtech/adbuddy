# ADBuddy · T8 del inventario de validaciones · adb, modo remoto y release en la API 34 · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementador y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`). Las tres tareas de código tocan el modo remoto y la confianza de la app (🔒): las revisa `adbuddy-reviewer-opus`; la tarea de docs, `adbuddy-reviewer`. Nunca Haiku. **Antes de la Tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido** (CLAUDE.md), y el plan se corrige con sus hallazgos.

**Meta:** cerrar en el emulador propio API 34 la tarea T8 del inventario (`docs/superpowers/plans/2026-10-03-validaciones-manuales.md`): las filas **M5b-12, M5b-13, VB-37, VB-41** (modo remoto y puertos de adbd) y **M6a-2, M6a-3** (la app de release con R8 y lo que la spec M6a da por hecho de Android). Son tres tests de host en `host/internal/emutest/` (`remote_test.go`, `release_test.go`, `install_test.go`) que juegan el papel de `serve` sobre el adbd real del emulador, más una tarea de docs que pasa a "Automatizado" en el inventario solo lo que de verdad lo queda. **No se toca código de producción.** Si un test descubriera un defecto, se registra como hallazgo para el controlador (ver "Hallazgos para el controlador"); no se arregla aquí.

**Arquitectura.** Los tests reutilizan el arnés que ya existe en `host/internal/emutest/` y no duplican nada:

- `emulator_test.go`: `newEmu(t)` (salta sin `ADBUDDY_EMULATOR_SERIAL`, rechaza todo serial que no sea `emulator-*` y los puertos 5037/5038), `e.run`/`e.shell`/`e.state`/`e.waitState`, `installApp`, `uninstallAtCleanup`, `readAPK`, los constantes `appAPK`/`testsAPK`/`buildHint`.
- `link_test.go` (T3): `openLink(t)` abre el canal real (bridge en 127.0.0.1, `adb reverse`, `phoneapp.Handshake`) y devuelve un `*link` con `send`/`reply`/`publishRID`/`waitFor`/`take`/`drain`/`Hellos`/`uiRequest`, y para la pantalla `e.screen`/`e.waitNode`/`e.tap`/`l.openChat`/`poll`/`payload`/`notificationRecord`/`atCleanup`. La **única** firma que cambia es un refactor de una línea de `openLink` para poder enlazar un APK que no sea el de debug (`linkTo`), descrito en la Tarea 2.

Cada test habla con el adbd del emulador **directamente, como `serve`**, nunca por el shim. El controlador del modo remoto es el de producción (`remote.New(e.adb, store, clock)`) con un `Store` y un reloj en memoria propios de los tests: así `remote.go` no se toca y se ejercita su comportamiento real (`Enable`, `Disable`, `Reconcile`, `Expire`, `State`). Los puertos se leen de **`service.adb.tcp.port`**, nunca del socket: en el emulador adbd escucha siempre en el 5555 del guest (es su propio transporte, M2.5b S2/S5), así que el socket no distingue "modo tcpip encendido" de "apagado"; lo que cambia es la propiedad. Para saber si una orden reinició adbd (lo que `adb tcpip` y `adb usb` hacen, con el emulador fuera de `device` ~2 s) se compara el **pid de adbd** antes y después, más fiable que muestrear `adb devices` cada segundo.

El APK de release se construye con Gradle **offline** (R8 y shrink de recursos, como `release-check.yml`) y se firma con una **clave de usar y tirar** creada en `t.TempDir()` con `keytool` del JDK (RSA 2048, un día), exactamente como hace el job `sign` de `release-check.yml`: **nunca la clave de release, nunca el keystore ni los secretos**. El host lee quién firmó con su propio parser (`phoneapp.Signers`/`CheckSigner`) y verifica la app instalada con `phoneapp.Verifier`, confiando en el certificado de esa clave temporal como si fuera el de release solo dentro del test.

**Tecnologías:** Go 1.27 (`host/`). Nada nuevo en `go.mod`. Android SDK build-tools (aapt2, zipalign, apksigner) y un JDK 17 (keytool, java) para las dos tareas que firman o compilan APK; los tests que los necesitan **se saltan limpiamente** si no están.

**Fuentes:** encargo `.superpowers/sdd/2026-10-03-adbuddy-validaciones-t8/plan-brief.md` · inventario `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` ("Datos medidos", T8 línea ~404, filas M5b-12/13, VB-37, VB-41, M6a-2/3, "Hallazgos de la T1") · spec M6a `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` (§5 parser, §6 confianza, §9 release-check, §11 ruling de la validación 3) · `docs/releasing.md` · `.github/workflows/release-check.yml` · plan de M5b `docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md` (validaciones 12-13) · diferidos B `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-b.md` (VB-37 = 37, VB-41 = 41) · `host/internal/remote/remote.go` y `host/internal/cli/cut_test.go` (patrón de reinicio de adbd en el emulador).

**Verificación del plan (réplica acumulada).** Los tres ficheros de test de este plan se escribieron y se aplicaron paso a paso sobre un `git archive` de `origin/main` 2b8c07d en el scratchpad, y en esa copia: `GOOS=windows go build ./...`, `gofmt -l internal/emutest` (vacío), `GOOS=windows go vet ./internal/emutest/...` y `go test ./internal/emutest/` (todos los tests que no necesitan emulador en verde; los que lo necesitan `SKIP` limpiamente). Para el APK de release se compiló con Gradle offline (`:app:assembleRelease`, 1 min 20 s), y la RED del shrink se vio de verdad: con `isMinifyEnabled=false` el test falla (la clase `RemoteUi` sigue en el dex) y con `isMinifyEnabled=true` pasa. El APK de conflicto (aapt2 link + zipalign + apksigner con clave temporal) se construyó y firmó en la réplica; el host lo lee con `Signers`. **Las ejecuciones con el emulador encendido no se hicieron:** quedan descritas en cada tarea (comandos, puertos, RED por mutación) para cuando el controlador diga que hay emulador libre. Las salidas citadas son las de esa réplica.

## Global Constraints

- Rama `test/validaciones-t8`, desde `origin/main` 2b8c07d. Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify` ni `--force`; sin `git stash` (usa un commit WIP). Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Sin IPs RFC 1918** ni Tailscale reales en código, tests ni docs: la única IP es `100.64.0.5` (Tailscale de documentación; el modo remoto solo la necesita para no negarse por `ErrNoTailscale`, nunca se conecta a ella). Sin seriales reales: los tests usan el `emulator-<port>` que da `scripts/emulator.sh`.
- **El móvil del usuario no se toca.** Todo corre contra el AVD `adbuddy-dev34` y su servidor adb propio, nunca el 5037/5038. La guarda de `emutest` (`checkEmuEnv`) ya lo impone.
- **NUNCA matar procesos por nombre de imagen.** Prohibido `taskkill /IM`, `Stop-Process -Name`, `pkill`/`killall` por nombre (`java`, `adb`, `emulator`, `qemu`, `node`, `python`…): apagarían el emulador de otra sesión o el adb del móvil del usuario. Solo se mata por **PID que hayas lanzado tú y comprobado** con CIM/`CommandLine` (PowerShell: `Get-CimInstance Win32_Process -Filter "ProcessId=<pid>"` y mirar `CommandLine`), o por el mecanismo del propio emulador por serial (`scripts/emulator.sh stop`, que hace `adb -s <serial> emu kill`). Ningún test ni script de este plan apaga el emulador por nombre: el apagado es el de `scripts/emulator.sh stop` con las mismas `ADBUDDY_EMU_*`, y los tests solo dejan el adbd como estaba (cierre del puerto con `adb usb`, nunca matando nada).
- **Reglas de emulador (del supervisor).** UN solo emulador a la vez en el PC. AVD propio `adbuddy-dev34` (imagen `system-images;android-34;google_apis;x86_64`); consola **≥ 5630**; servidor adb propio en el **5090** (`ANDROID_ADB_SERVER_PORT=5090`, `-P 5090`), nunca 5037/5038; `ADB_MDNS=0`. Arrancar solo cuando skimmer/puffin/tern avisen de que está libre y haya **≥ 12 GB de RAM** libres. **Apagar al terminar** con `scripts/emulator.sh stop` (mismas variables) y verificar con `tasklist`/`Get-Process` que no queda su `qemu` (por su PID/ventana, no matando por nombre). **No hay que arrancar el emulador para escribir ni replicar el plan:** la réplica se limita a compilar y a los tests que no lo necesitan.
- **Cada tarea compila y pasa sus tests por sí sola.** Los tres ficheros son independientes entre sí (comparten solo el arnés ya existente y, release/install, los ayudantes de `sdkTools` que define `release_test.go`: por eso `release_test.go` va antes que `install_test.go`).
- **Go:** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/...`; los tests sin emulador con `go test -count=1 -timeout 5m ./internal/emutest/`. Este PC no corre `-race`: la concurrencia se razona y la corre la CI.
- `docs/**` está en **CRLF** en el árbol de trabajo (`core.autocrlf=true`, sin regla en `.gitattributes`): las ediciones del inventario usan anclas de **una sola línea**. `host/**` es LF (`.gitattributes`); los tres `*_test.go` se escriben con LF.
- No se tocan `doctor.go`, los catálogos i18n (`host/internal/i18n/catalog/*`), `host/internal/classify/`, `scripts/emulator.sh` ni el emulador de otras sesiones. **Ningún texto nuevo de cara al usuario.**
- Pasar a los revisores límites de tiempo (`go test -timeout`, tope de 30 min por revisión).
- Windows con Git Bash: sin `sleep` en primer plano; comandos largos con timeout amplio o en segundo plano. Gradle: `JAVA_HOME=/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1`, gradle en `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin`, **siempre `--offline`**, **nunca `gradle --stop`**.

## Qué es automatizable de verdad (parte del plan)

Para cada fila, qué parte se automatiza en el emulador API 34, con qué órdenes/API, y qué NO y por qué. Lo que no es automatizable se queda en la checklist del Pixel (ver "Paso mínimo del usuario" del inventario); no se fuerza.

### M5b-12 · `tcpip 5555` por Tailscale con solo datos; `remote on`/`off`; caducidad
- **Automatizable (T8, `remote_test.go`, `TestTheRemoteModeOpensAndClosesThePort`):** el **controlador real** `remote.New(e.adb, store, clock)` contra el adbd del emulador. `Enable` guarda la caducidad y abre el puerto (`adb tcpip 5555`); `service.adb.tcp.port` pasa a `5555` y el pid de adbd cambia. `Reconcile` con la caducidad por delante la conserva y no reinicia adbd. `Disable` cierra (`adb usb`), deja `service.adb.tcp.port` en `0`/vacío y borra la caducidad. Con la caducidad pasada (reloj falso +12 h) el estado es `Enabled && Closing`, **nunca "off"**, y `Expire` cierra. Una caducidad rancia con el puerto cerrado se borra **sin** `adb usb`; una caducidad pasada con el puerto abierto la cierra `Reconcile`. Todo leyendo `service.adb.tcp.port` (medido en M2.5b S5), nunca el socket.
- **NO automatizable (queda en el Pixel):** Tailscale con **datos móviles** y que `serve` **reconecte** por `<ip-tailscale>:5555` con el Wi-Fi apagado. Necesita red real y la VPN; el emulador no tiene Tailscale ni datos móviles. La IP de Tailscale del test (`100.64.0.5`) es de documentación.
- **Falso positivo evitado:** en el emulador adbd escucha siempre en 5555 (su transporte), así que **no** se afirma que el modo tcpip "abra el socket"; solo se comprueba la propiedad `service.adb.tcp.port`, que es lo que mira el host. adbd del emulador vuelve en ~2 s y el transporte no cambia: distinto de un móvil real, donde `adb tcpip` cambia el transporte. El test no presenta esto como validación del móvil.

### M5b-13 · segundos hasta ver el estado tras `adb tcpip`; "desconocido" nunca como "apagado"
- **Automatizable (T8, `remote_test.go`, `TestTheCardIsNeverOffWhileTheModeTurnsOn`):** por el canal real (arnés T3) la app muestra la tarjeta del modo remoto; al tocar el interruptor manda `remote.set`; el arnés hace de `serve` (ejecuta `Enable` real, que reinicia adbd y tira el túnel) y contesta con `remote.state` por `publishRID` (ruling 3 de M5b: la respuesta puede perderse). Hasta que vuelve el estado la tarjeta dice "Working…", "Not connected" o "Unknown", **nunca "Off"** (se comprueba leyendo los nodos con `uiautomator`), y se registra con `t.Logf` cuántos segundos tarda en pasar a "Until …". adbd vuelve en ~2 s y el arnés reabre el `adb reverse` al momento conservando el token.
- **NO automatizable / falso positivo:** la **cifra real de segundos del Pixel** es informativa y distinta: en `serve` el vigilante del túnel mira cada 15 s y entrega un **token nuevo**, y el móvil vuelve por Wi-Fi/datos. Los segundos del emulador son una **cota inferior**, no la del móvil; el test lo dice en su `t.Logf` y no afirma ninguna cifra de producto.

### VB-37 · "off" con el modo apagado no corta la conexión; un 5555/7777 abierto a mano se cierra
- **Automatizable (T8, `remote_test.go`, `TestOffWithTheModeOffOnlyClosesAnOpenPort`):** con el modo apagado y el puerto cerrado, `Disable` **no** reinicia adbd (el pid no cambia): en un móvil eso evita cortar su conexión para nada. Con el 5555 abierto a mano (`adb tcpip 5555`) o con otro puerto (`adb tcpip 7777`) y el modo apagado, `Disable` lo cierra con `adb usb` (pid nuevo, `service.adb.tcp.port` a `0`). El pid de adbd es lo que distingue "no tocó adbd" de "lo reinició".
- **Falso positivo evitado:** no se mira el socket (siempre 5555 en el emulador); se mira pid + `service.adb.tcp.port`. "Que no se corte la conexión real del móvil" no se puede ver en el emulador (su transporte no cambia): se comprueba el **invariante del host** (no lanzar `adb usb` con el puerto ya cerrado), que es lo que de verdad protege al móvil.

### VB-41 · `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd
- **Automatizable (T8, `remote_test.go`, `TestPersistTCPPortAgainstUSBAndRestarts`):** con `su 0 setprop persist.adb.tcp.port 5555` (su funciona en la imagen userdebug, M2.5b): se registra si fijarlo reinicia adbd, qué deja `service.adb.tcp.port` tras `su 0 setprop ctl.restart adbd`, tras `adb tcpip`/`adb usb`, y que con `service.adb.tcp.port=0` un reinicio de adbd lo mantiene en 0 y `Disable` con el modo apagado no toca adbd. El paso de **reinicio del emulador** (`adb reboot`, con `-no-snapshot` conserva datos, T1-9) solo con `ADBUDDY_EMULATOR_REBOOT=1`.
- **NO automatizable / falso positivo:** el emulador siempre escucha en 5555, así que **qué puerto abre adbd de verdad** no se ve; el test **graba** las propiedades y qué hace el host con ellas, no afirma el comportamiento de red. Y el host (`remote`, `device.EnsureNoTCPPort`) **no lee** `persist.adb.tcp.port` (solo `service.adb.tcp.port`): si tras un reinicio con `persist` fijado `service.adb.tcp.port` queda vacío, el host toma el puerto como "cerrado". Eso es un posible **hallazgo** (ver "Hallazgos para el controlador"), no un arreglo de esta tarea. En un móvil sin root no hay `su`, así que fijar `persist` es cosa del emulador; además `setprop persist.adb…` es 🔴 en el clasificador.

### M6a-2 · la app de release (R8) funciona: la serialización con kotlinx
- **Automatizable (T8, `release_test.go`):** el APK de `:app:assembleRelease` (R8 + shrink, versión de prueba `0.0.1-rc.1`/`101` como `release-check.yml`) firmado con **clave temporal** en `t.TempDir()`; el host lo lee con `Signers`/`CheckSigner` (parser §5) y comprueba que R8 **sí** encogió (la clase `RemoteUi`, que ninguna keep rule nombra, ya no está en el dex). Instalado en la API 34 y verificado con `phoneapp.Verifier` confiando en el certificado de esa clave como release: por el canal real (arnés T3) la app dice `hello` y manda `device.info` (kotlinx app→host), muestra un `chat.message` (host→app), contesta un `ui.request` con su id (los dos sentidos con rid) y publica un `confirm.request` como notificación. Esto ejercita la serialización bajo R8, que es el riesgo real.
- **NO automatizable:** la **clave de release** y los secretos nunca se tocan (spec M6a §10, solo el usuario). El APK de release **público** (firmado con la clave real) se prueba en la primera release; aquí es un APK de release **funcionalmente idéntico** firmado con clave de prueba, que es lo que mide "R8 no rompió la app".

### M6a-3 · lo que la spec M6a da por hecho de Android
- **Automatizable (T8, `install_test.go`):** en la API 34, `pm path` (una línea `package:/data/app/<…>/base.apk`), `stat -c '%s %i %Y'` (forma que parsea `phoneapp.statKey`), `base64` (toybox) que devuelve el APK **byte a byte** idéntico al fichero (porque `Install` usa `--no-incremental`), y que un `install -r` cambia el `stat` de `base.apk` (la clave de la caché del `Verifier`). La **autoridad duplicada**: se construye con aapt2 un APK mínimo que declara `tech.hybriq.adbuddy.bridge`, se firma con clave temporal, y Android lo rechaza con `INSTALL_FAILED_CONFLICTING_PROVIDER` **también con el provider de ADBuddy desactivado** (`pm disable`), que es lo que garantiza §6.3. Los **APK de ataque de §5** (hueco antes del EOCD, localizador ZIP64, comentario, firma v1) se construyen con `apktest` y se comprueba que el **host los rechaza** (`Signers` da error) — esto no necesita emulador y complementa `apksig_test.go`.
- **NO automatizable (queda para la validación 3 manual, spec §11):** instalar un **ataque real** y ver que **Android** vuelve a v1 e instala al atacante mientras el host lee otro bloque necesita **API 30 y `targetSdk` 29** (imagen no instalada), y la 37 está rota (M2.5b). Las API 30/33 las decide el usuario; la última sirve-para-nada hasta que haya imagen que funcione. El test lo dice y deja esa parte fuera.

**Resumen de categorías tras T8:** M5b-12 Parcial→con parte **Automatizada** (lógica real contra el emulador); M5b-13 **Automatizada** la lógica y la cota, la cifra del Pixel queda; VB-37 **Automatizada**; VB-41 **Automatizada** lo observable (graba lo demás, con un posible hallazgo); M6a-2 **Automatizada**; M6a-3 **Automatizada** en API 34, el ataque real y API 30/33 quedan para el Pixel/validación 3.

## Hallazgos para el controlador (no se arreglan aquí)

- **VB-41 (posible, a confirmar en el emulador):** `remote` y `device.EnsureNoTCPPort` solo leen `service.adb.tcp.port`, no `persist.adb.tcp.port`. Si tras un reinicio con `persist.adb.tcp.port=5555` la imagen deja `service.adb.tcp.port` vacío (adbd de AOSP toma el `persist` solo cuando la propiedad de servicio está vacía), el host tomaría el puerto como "cerrado" y `Disable` no lanzaría `adb usb`. Ya estaba anotado como validación 41 de los diferidos B (misma suposición que `Reconcile`/`Enable` desde M5b). El test lo registra con un `t.Logf("FINDING (VB-41)…")` y una constante `hostReadsPersistPort = false`: si el controlador decide que el host debe mirar `persist`, se pone a `true` y el paso del reinicio exige que `Disable` cierre el puerto. **No se arregla en T8.**

## Mapa de ficheros

| Fichero | Tarea | Cambio |
|---|---|---|
| `host/internal/emutest/link_test.go` | 1 | refactor de `openLink` en `openLink`+`linkTo` (una función nueva; firma de `openLink` intacta) |
| `host/internal/emutest/remote_test.go` | 1 | nuevo: `memStore`, `fakeClock`, ayudantes de adbd, `TestTheRemoteModeOpensAndClosesThePort`, `TestOffWithTheModeOffOnlyClosesAnOpenPort`, `TestPersistTCPPortAgainstUSBAndRestarts`, `TestTheCardIsNeverOffWhileTheModeTurnsOn` |
| `host/internal/emutest/release_test.go` | 2 | nuevo: `sdkTools` y ayudantes, `TestVersionLess`, `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`, `TestTheReleaseAppTalksOverTheLink` |
| `host/internal/emutest/install_test.go` | 3 | nuevo: `TestTheInstalledAppReadsBackByteForByte`, `TestASecondProviderOnTheAuthorityIsRefused`, `TestTheHostRejectsTheSection5Attacks` y ayudantes |
| `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 4 | filas M5b-12/13, VB-37, VB-41, M6a-2/3 a "Automatizado"; T8 marcada hecha |

**Sobre `instrumented_test.go` (`requiredClasses`/`allowedSkips`):** los tres tests nuevos son **de host** (Go), no instrumentados de la app (`am instrument`), así que **no** añaden clases a la suite instrumentada ni tocan `requiredClasses`/`allowedSkips`/`minInstrumentedTests`. Se saltan solos (`newEmu`, `findTools`, `releaseSigned`) sin emulador o sin APK. No hay que tocar `instrumented_test.go`.

---

### Task 1: modo remoto y puertos de adbd (M5b-12, M5b-13, VB-37, VB-41) 🔒

**Files:** `host/internal/emutest/link_test.go` (refactor de una función), `host/internal/emutest/remote_test.go` (nuevo).

- Produces: `TestTheRemoteModeOpensAndClosesThePort`, `TestOffWithTheModeOffOnlyClosesAnOpenPort`, `TestPersistTCPPortAgainstUSBAndRestarts`, `TestTheCardIsNeverOffWhileTheModeTurnsOn`, y `func (e *emu) linkTo(t *testing.T, v phoneapp.Verified) *link`.
- Reutiliza: `remote.New`/`State`/`Port`/`Lifetime` (sin tocar `remote.go`), el arnés de `link_test.go` y `emulator_test.go`.

- [ ] **Step 1: refactor de `openLink` para poder enlazar otro APK.** La Tarea 2 instala el APK de release y necesita el canal con esa app ya instalada. Se parte `openLink` en dos sin cambiar su firma ni su comportamiento.

Edit en `host/internal/emutest/link_test.go`:

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
(El resto de `openLink` queda como cuerpo de `linkTo`: usa `e` y `v`, que ahora son el receptor y el parámetro. No se cambia ninguna otra línea.)

- [ ] **Step 2: `remote_test.go`.** Escribir el fichero completo:

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

// sameAdbd checks that adbd still has pid and port: nothing restarted it.
func (e *emu) sameAdbd(t *testing.T, pid, port string) {
	t.Helper()
	now, p, ok := e.adbdNow()
	if !ok || now != pid || p != port {
		t.Errorf("adbd has pid %q and service.adb.tcp.port %q (read ok %v), want the same %s and %q: something restarted it", now, p, ok, pid, port)
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
// persist.adb.tcp.port. They do not (pendientes, diferidos B, T2): after a
// reboot with it set, service.adb.tcp.port is empty, adbd listens on the
// persist port (AOSP adbd: persist only counts when the service property is
// empty) and the host reads "closed". Finding of T8 for the controller; set it
// to true with the fix, and the reboot step then wants Disable to close it.
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
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	persist := func() string { return e.shell(t, "su 0 getprop persist.adb.tcp.port") }
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q before the test: left over; clear it with `adb shell su 0 setprop persist.adb.tcp.port ''`", p)
	}
	// Cleanups run last first: this one before closedAtStartAndEnd's adb usb.
	e.atCleanup(t, "shell", "su 0 setprop persist.adb.tcp.port ''")

	// Can the shell user set it without root? Recorded, not required: a phone
	// without root has only the shell user (and the classifier makes
	// `setprop persist.adb…` 🔴).
	out, _, code, _ := e.adb.Output(e.ctx, "shell", "setprop persist.adb.tcp.port 5555")
	t.Logf("setprop persist.adb.tcp.port 5555 as shell: exit %d %s; persist now %q", code, strings.TrimSpace(out), persist())

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

// Validation 13 of M5b, on the emulator: the remote card's switch sends
// remote.set; the harness, as serve, runs the real Enable (adb tcpip 5555,
// which restarts adbd and drops the tunnel) and answers with remote.state
// (M5b ruling 3: the answer may be lost). Until the state comes back the card
// says Working…, Not connected or Unknown, never Off; the seconds until it
// says "Until …" are logged. They are the emulator's (adbd back in ~2 s, the
// harness reopens the tunnel at once and keeps the token), a lower bound of
// the Pixel's: serve looks at the tunnel every 15 s and hands over a new
// token, and a phone comes back over Wi-Fi.
func TestTheCardIsNeverOffWhileTheModeTurnsOn(t *testing.T) {
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
	pid, _ := e.adbd(t)
	s, err := ctl.Enable(e.ctx)
	if err != nil || !s.Enabled {
		t.Fatalf("Enable: %+v, %v", s, err)
	}
	// serve's answer (phone_remote.go, run): the tunnel may drop before it
	// reaches the app; the server keeps it for the app's return.
	l.publishRID(t, req, bridge.TypeRemoteState, bridge.RemoteState{Enabled: true, Address: s.Address, ExpiresAt: s.ExpiresAt})
	e.restartedTo(t, pid, remote.Port)
	// adbd's restart takes the reverse with it: reopened as serve's tunnel
	// check does (phone.go), without its new token.
	if open, err := phoneapp.TunnelOpen(e.ctx, e.adb, l.port); err != nil || !open {
		t.Logf("the reverse did not survive adb tcpip (open %v, %v): reopened", open, err)
		if err := phoneapp.Tunnel(e.ctx, e.adb, l.port); err != nil {
			t.Fatalf("adb reverse: %v", err)
		}
	}
	var seen []string
	if !poll(90*time.Second, func() bool {
		text := cardText(e.screen(t))
		if text != "" && (len(seen) == 0 || seen[len(seen)-1] != text) {
			seen = append(seen, text)
		}
		return strings.HasPrefix(text, "Until ")
	}) {
		t.Fatalf("the card did not say Until … 90 s after adb tcpip; it said %q", seen)
	}
	t.Logf("M5b-13 on the emulator: %.1f s from adb tcpip to the card's %q; the card said %q", time.Since(start).Seconds(), seen[len(seen)-1], seen)
	if slices.Contains(seen, "Off") {
		t.Errorf("the card said Off while the mode turned on: %q", seen)
	}
}
```

- [ ] **Step 3: compila y los tests sin emulador pasan.**
```
cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/...
go test -count=1 -timeout 5m ./internal/emutest/   # sin ADBUDDY_EMULATOR_SERIAL: todo SKIP o pasa
```

- [ ] **Step 4: RED → GREEN en el emulador** (cuando el controlador diga que hay emulador libre). Arrancar con `scripts/emulator.sh start` (AVD `adbuddy-dev34`, `ADBUDDY_EMU_PORT=5630`, `ADBUDDY_EMU_ADB_PORT=5090`, `ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64'`, `JAVA_HOME` al JDK 17) y exportar lo que imprime (`ADBUDDY_EMULATOR_SERIAL`, `ADBUDDY_EMULATOR_ADB`, `ANDROID_ADB_SERVER_PORT=5090`). Construir los APK de debug una vez (`cd android && gradle --offline :app:assembleDebug :app:assembleDebugAndroidTest`). Luego:
  - **GREEN:** `go test -count=1 -timeout 20m -v -run 'TestTheRemoteModeOpensAndClosesThePort|TestOffWithTheModeOffOnlyClosesAnOpenPort|TestPersistTCPPortAgainstUSBAndRestarts|TestTheCardIsNeverOffWhileTheModeTurnsOn' ./internal/emutest/`.
  - **RED por mutación (una cada vez, revertir después):**
    - en `remote.go`, en `Disable`, quitar la guarda `if c.st.Until().IsZero() { if port … portClosed(port) { return State{}, nil } }` (que siempre llame a `disable`): `TestOffWithTheModeOffOnlyClosesAnOpenPort/closed` ve un pid nuevo y falla (adbd reiniciado sin motivo). Es la VB-37.
    - en `remote.go`, en `Reconcile`, cambiar `case port != Port:` por `case false:` (nunca borra la caducidad rancia): `TestTheRemoteModeOpensAndClosesThePort` falla en "Reconcile with the port closed" (la caducidad sigue guardada).
    - en el test, cambiar `l.publishRID(...)` de `TestTheCardIsNeverOffWhileTheModeTurnsOn` por un `remote.state` con `Enabled:false`: la tarjeta pasaría a "Off" mientras enciende y el test falla ("the card said Off while the mode turned on"). (Mutación del propio test, para demostrar que vigila el invariante "nunca Off".)
  - Al terminar: apagar con `scripts/emulator.sh stop` (mismas variables) y verificar con `tasklist`/`Get-Process` que no queda su `qemu` (por PID/ventana del emulador, **nunca** matando por nombre).

- [ ] **Step 5: commit.** `test(emutest): modo remoto y puertos de adbd en la API 34 (M5b-12/13, VB-37, VB-41)` con el trailer exacto.

---

### Task 2: la app de release (R8) en la API 34 (M6a-2) 🔒

**Files:** `host/internal/emutest/release_test.go` (nuevo). Depende de la Tarea 1 (usa `linkTo`, `memStore` no; usa `cardText` no; usa `poll`, `payload`, `notificationRecord`, `openChat`, `e.waitNode` del arnés). Puede ir en paralelo con la Tarea 3 solo si una de las dos define `sdkTools`: **`release_test.go` define `sdkTools` y sus ayudantes**; `install_test.go` los usa. Por eso la Tarea 2 va antes que la 3.

- Produces: `sdkTools` (+ `findTools`, `newKey`, `sign`, `run`, `exe`, `sdkRoot`, `versionLess`, `newest`, `exists`, `dexHas`, `releaseSigned`), `TestVersionLess`, `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`, `TestTheReleaseAppTalksOverTheLink`, `installRelease`.

- [ ] **Step 1: `release_test.go`.** Escribir el fichero completo:

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

- [ ] **Step 2: construir el APK de release y comprobar RED/GREEN del shrink (no necesita emulador).**
```
cd android && JAVA_HOME=/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1 \
  /c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --offline \
  :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101
cd ../host && go test -count=1 -timeout 5m -run 'TestVersionLess|TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey' ./internal/emutest/
```
  - **RED del shrink (demostrada en la réplica):** con `isMinifyEnabled = false` e `isShrinkResources = false` en `android/app/build.gradle.kts`, reconstruir el APK de release y correr `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`: **falla** (`Ltech/hybriq/adbuddy/ui/RemoteUi; is in the release APK's dex: it was not shrunk by R8`). **GREEN:** revertir a `true`, reconstruir, pasa. **Revertir siempre `build.gradle.kts`** (no se toca código de producción).

- [ ] **Step 3: compila y tests sin emulador.** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/... && go test -count=1 -timeout 5m ./internal/emutest/`.

- [ ] **Step 4: GREEN con emulador.** Con el emulador arrancado (como en la Tarea 1) y el APK de release construido: `go test -count=1 -timeout 20m -v -run 'TestTheReleaseAppTalksOverTheLink' ./internal/emutest/`. **RED por mutación en el emulador:** firmar con `--v2-signing-enabled false --v3-signing-enabled false --v1-signing-enabled true` (solo v1) en `releaseSigned`: `phoneapp.Signers` da error y `releaseSigned` falla (el host no lee bloque de firma), lo que confirma que el test exige la firma por esquema. Apagar el emulador al terminar (`scripts/emulator.sh stop`, verificar con `tasklist`, nunca por nombre).

- [ ] **Step 5: commit.** `test(emutest): la app de release con R8 en la API 34 (M6a-2)`.

---

### Task 3: pm path/stat/base64, install -r, autoridad duplicada y ataques §5 (M6a-3) 🔒

**Files:** `host/internal/emutest/install_test.go` (nuevo). Depende de la Tarea 2 (usa `sdkTools`/`findTools`/`newKey`/`sign`/`exe`/`exists`).

- Produces: `TestTheInstalledAppReadsBackByteForByte`, `TestASecondProviderOnTheAuthorityIsRefused`, `TestTheHostRejectsTheSection5Attacks`, `buildConflictAPK` y ayudantes (`base64`, `nonEmptyLines`, `basePath`, `firstPath`, `threeInts`).

- [ ] **Step 1: `install_test.go`.** Escribir el fichero completo:

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
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Validation 3 of M6a, what API 34 can show: the Android behaviour the host's
// trust model relies on (spec M6a §6.2). pm path gives one base.apk under
// /data/app; stat -c '%s %i %Y' has the shape the host parses; base64 reads
// the APK back byte for byte (--no-incremental: adb does not keep reading the
// file after the check); an update in place (install -r) changes base.apk's
// stat (the cache key). The attack APKs of §5 and the two older images (API
// 30 and 33, not installed) stay for the manual check before the first
// release (docs/releasing.md).
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

// Validation 3 of M6a, the host's side of the §5 attacks: the parser refuses
// the shapes an attacker would use to make Android read a block it did not
// read (a v1 signature with a gap before the EOCD, a ZIP64 locator, a
// comment). Installing a crafted attacker and seeing Android fall back to v1
// needs API 30 and targetSdk 29 (not an installed image): the manual check
// before the first release (docs/releasing.md). Here the host rejects each,
// so none would ever be trusted.
func TestTheHostRejectsTheSection5Attacks(t *testing.T) {
	v2 := apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: apktest.Cert("attacker")}}}
	cases := map[string][]byte{
		"a gap before the EOCD":    apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Gap: 32}),
		"a ZIP64 locator":          apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Zip64Locator: true}),
		"a comment in the EOCD":    apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Comment: "x"}),
		"a v1 signature (CERT.SF)": apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "META-INF/CERT.SF"}}),
	}
	for name, apk := range cases {
		if _, err := phoneapp.Signers(apk); err == nil {
			t.Errorf("the host accepted an APK with %s", name)
		}
	}
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

- [ ] **Step 2: compila y tests sin emulador.** `cd host && GOOS=windows go build ./... && gofmt -l internal/emutest && GOOS=windows go vet ./internal/emutest/... && go test -count=1 -timeout 5m -run 'TestTheHostRejectsTheSection5Attacks' ./internal/emutest/` (este test no necesita emulador y pasa en la réplica).

- [ ] **Step 3: GREEN con emulador.** `go test -count=1 -timeout 20m -v -run 'TestTheInstalledAppReadsBackByteForByte|TestASecondProviderOnTheAuthorityIsRefused' ./internal/emutest/`.
  - **RED por mutación (en el emulador):**
    - en el test, cambiar `sha256.Sum256(got) != sha256.Sum256(want)` por `len(got) != len(want)+1`: pasaría en falso; mejor demostración: truncar `want` (`want = want[:len(want)-1]`) y ver que falla — prueba que compara byte a byte. (Mutación del test.)
    - para la autoridad: construir el APK de conflicto con **otra** autoridad (cambiar `phoneapp.Authority` por `"tech.hybriq.other.bridge"` en `conflictManifest`): Android lo instala y `TestASecondProviderOnTheAuthorityIsRefused` falla (no hay `INSTALL_FAILED_CONFLICTING_PROVIDER`), lo que prueba que es la autoridad lo que choca.
  - **RED del rechazo §5 (sin emulador):** quitar una de las comprobaciones de `apksig.go` (p. ej. la del comentario del EOCD, condición 1) haría que `TestTheHostRejectsTheSection5Attacks/a comment in the EOCD` dejara de ver el error. No se deja aplicada: es del parser, que no se toca.
  - Apagar el emulador al terminar (`scripts/emulator.sh stop`, verificación por PID/ventana, nunca por nombre).

- [ ] **Step 4: commit.** `test(emutest): adb, pm path/stat/base64, autoridad duplicada y ataques §5 en la API 34 (M6a-3)`.

---

### Task 4: pasar las filas del inventario a "Automatizado"

**Files:** `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (CRLF, anclas de una sola línea).

- [ ] **Step 1: editar las seis filas y la T8.** Ediciones (cada "Reemplazar" es una línea única del fichero):

En la fila **M5b-12** (tabla M5b), sustituir el estado `| Parcial, lógica hecha (adb falso) |` y la celda "Cómo" para que diga la parte automatizada:
- Estado → `Parcial; lógica contra el emulador (T8)`.
- Cómo → `T8 (hecha): \`TestTheRemoteModeOpensAndClosesThePort\` corre el controlador real \`remote.New(adb.Runner{Serial: emu})\` contra el adbd del emulador (\`Enable\`/\`Disable\`/\`Reconcile\`/\`Expire\`) leyendo \`service.adb.tcp.port\` (nunca el socket: el emulador escucha siempre en 5555).`
- Por qué no → dejar lo de Tailscale con datos y la reconexión por \`<ip>:5555\` en el Pixel.

En la fila **M5b-13**: estado → `Automatizada la lógica y la cota (T8)`; Cómo → `T8 (hecha): \`TestTheCardIsNeverOffWhileTheModeTurnsOn\` por el canal real; la tarjeta nunca dice "Off" mientras enciende y registra los segundos (cota inferior; la cifra del Pixel queda).`

En la fila **VB-37** (tabla v1 Diferidos): estado `Sí` → `Sí, hecha (T8)`; Cómo → `T8 (hecha): \`TestOffWithTheModeOffOnlyClosesAnOpenPort\`: \`Disable\` con el modo apagado no reinicia adbd (pid igual) y cierra un 5555/7777 abierto a mano con \`adb usb\`.`

En la fila **VB-41**: estado `Sí` → `Sí, hecha (T8)`; Cómo → `T8 (hecha): \`TestPersistTCPPortAgainstUSBAndRestarts\` con \`su 0 setprop persist.adb.tcp.port\`, \`adb usb\`, \`ctl.restart adbd\` y (con \`ADBUDDY_EMULATOR_REBOOT=1\`) \`adb reboot\`; graba lo que el emulador no distingue y señala el posible hallazgo del host que no lee \`persist\`.`; Por qué no → añadir que el host no lee `persist.adb.tcp.port` (posible hallazgo, ver el plan de T8).

En la fila **M6a-2** (tabla M6a): estado `Sí` → `Sí, hecha (T8)`; Cómo → `T8 (hecha): \`TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey\` (R8 encoge; firma por esquema con clave temporal) y \`TestTheReleaseAppTalksOverTheLink\` (instalada y verificada, habla por el canal real; kotlinx bajo R8).`

En la fila **M6a-3**: estado `Parcial` se mantiene; Cómo → `T8 (hecha en API 34): \`TestTheInstalledAppReadsBackByteForByte\` (\`pm path\`, \`stat\`, \`base64\` byte a byte, \`install -r\` mueve el stat), \`TestASecondProviderOnTheAuthorityIsRefused\` (autoridad duplicada, con el provider propio desactivado) y \`TestTheHostRejectsTheSection5Attacks\` (el host rechaza los ataques §5).`; Por qué no → mantener API 30/33 y el ataque real instalado en el Pixel (validación 3).

En el **"Plan de automatización propuesto"**, en la entrada **T8**, añadir al final `**Hecha el 2026-10-04** (rama test/validaciones-t8): host-only tests en host/internal/emutest/ (remote_test.go, release_test.go, install_test.go); RED/GREEN por mutación en el informe; un posible hallazgo VB-41 (el host no lee persist.adb.tcp.port) para el controlador.`

- [ ] **Step 2: commit.** `docs(validaciones): T8 (modo remoto, release e instalación) automatizada en la API 34`.

---

## Réplica acumulada (resultado)

Sobre `git archive origin/main` (2b8c07d) en el scratchpad, aplicando las Tareas 1-3:

- `GOOS=windows go build ./...` → sin salida (OK).
- `gofmt -l internal/emutest` → vacío.
- `GOOS=windows go vet ./internal/emutest/...` → OK.
- `go test -count=1 -timeout 5m ./internal/emutest/` (sin emulador) → `ok`, con los tests que necesitan emulador en `SKIP` y los de host puros en verde (`TestVersionLess`, `TestTheHostRejectsTheSection5Attacks`, `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey`, más los ya existentes `TestUINodes`, `TestSearchField`, `TestCheckEmuEnv`, `TestParseInstrument`, …).
- APK de release: `gradle --offline :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101` → `BUILD SUCCESSFUL in 1m 20s`, `app-release-unsigned.apk` (3,2 MB).
- **RED del shrink demostrada:** con `isMinifyEnabled=false`/`isShrinkResources=false`, `TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey` → `FAIL` (`Ltech/hybriq/adbuddy/ui/RemoteUi; is in the release APK's dex`). Con `true`/`true` (revertido) → `ok`. `build.gradle.kts` restaurado idéntico a `origin/main`.
- APK de conflicto: `aapt2 link` (manifiesto con la autoridad `tech.hybriq.adbuddy.bridge`) + `zipalign` + `apksigner` con clave temporal → firmado; `aapt2 dump badging` confirma `package: name='tech.hybriq.adbuddy.conflict'` y el provider; el host lo lee con `Signers`.

Las partes que necesitan el emulador encendido (GREEN y las RED por mutación de adbd/instalación) **no se ejecutaron**: quedan descritas en los Steps 4/3 de cada tarea.

## Validaciones manuales pendientes (lo que T8 deja en el Pixel)

- **M5b-12/15/16:** Tailscale con datos móviles, el cambio de red en sesión y la reconexión por `<ip-tailscale>:5555`. Red real.
- **M5b-13:** la cifra de segundos del Pixel (informativa).
- **M6a-3:** los APK de ataque §5 **instalados** y el fallback a v1 de Android, y las API 30/33 (imágenes no instaladas) y la última (37 rota): la validación 3 de `docs/releasing.md`, antes de la primera release pública.
- **M6a (release real):** la clave, el environment, los secretos y la primera etiqueta (M6a-4): solo el usuario.
