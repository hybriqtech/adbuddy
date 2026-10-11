# Plan: leer la pantalla también exige el aviso del overlay (2026-10-11)

> Compañero `seguridad`. Solo investigación y plan: nada de esto está implementado. Spec: v1 §5.7 («Superposición de accesibilidad: marco sutil, aviso "Agente usando la pantalla · Pausar" y marcas donde toca») y M5 §6 («Cuándo: al empezar cualquier `ui.request` de acción (`click` y `setText`), y también con `dump`… Con la sesión en pausa no se muestra nunca»).

## 1. Diagnóstico

El hallazgo es **real, y más ancho de lo que decía**: el aviso es obligatorio (falla cerrado) al actuar y opcional al leer.

| Camino | ¿Muestra el aviso? | ¿Lo exige? | Cita |
|---|---|---|---|
| `tap`/`type` por la app | sí | sí: `acting()==false` → `FAILED` «won't act» | `UiExecutor.kt:213`, `229`, `268`; `noNotice` en `:184` |
| `screen` (dump) por la app | sí, mejor esfuerzo | **no**: el booleano de `s.acting(null)` se descarta | `UiExecutor.kt:194` |
| `screen` por adb (`uiautomator`; solo si la app no está o no tiene accesibilidad) | no | no | `host/internal/daemon/ui.go:411-421` |
| `adbuddy screenshot` (siempre `adb exec-out screencap`) | **nunca** | no | `host/internal/daemon/ui.go:531-534` |
| `adb shell screencap/screenrecord/uiautomator` crudo | **nunca** | no | `host/internal/daemon/guard.go:35,180-191`; `daemon.go:592` |
| `dumpsys activity top` y demás `dumpsys` que vuelcan la vista | nunca | no | `guard.go:180-191` (`screenAction`, `:189`) |
| `foreground`, `notifications` | nunca | no | `UiExecutor.kt:273-279` (foreground), `:152-164` (notifications) |

Qué ve el agente sin que la persona vea el aviso: el árbol de la pantalla (textos, descripciones, hints; `Dumper`) y, con `screenshot`/`screencap`, los píxeles (más que el dump). Las apps protegidas y ADBuddy quedan fuera en todos los caminos (guarda de primer plano, `ui.go:133-142`) y la pausa los bloquea todos (`ui.go:116`, `:144`, `daemon.go:430`): esto no abre un 🔴 ni salta la guarda; falta el **aviso a la persona**.

Choque con la spec: M5 §6 dice «también con `dump`» pero no dice si falla cerrado, y el test actual fija lo contrario (`UiExecutorTest.kt:366`: con `actingWorks=false`, `DUMP` y `FOREGROUND` siguen `ok`). `screenshot` y los `screencap` crudos no se nombran en §6, aunque v1 §5.7 («Cuando el agente usa la pantalla») los cubre por intención. No hay ruling previo que justifique la asimetría.

Sin app o con la accesibilidad apagada no hay overlay posible: es diseño (la persona quitó el servicio; el respaldo por adb existe para eso).

## 2. Opciones

- **A. `dump` por la app exige el aviso** (T1). Coste mínimo. `dump` falla (`FAILED`, sin respaldo por adb) solo si no se puede dibujar el overlay, la misma condición que ya impide tocar y escribir.
- **B. Aviso previo a las lecturas por adb** (T2): nueva op `notice` host→app, llamada antes de `screenshot` y de `screencap/screenrecord/uiautomator` crudos. Sin app, sin accesibilidad, app antigua o sin respuesta a tiempo, la lectura sigue (no hay a quién avisar). Si la app responde `FAILED` (no pudo dibujar), se bloquea. Coste: una op en el protocolo común (Go + Kotlin + fixtures) y un viaje a la app por lectura.
- **C. Documentarlo como diseño.** Barato, pero la persona no sabe cuándo el agente ve sus píxeles. No recomendada.

**Recomendación: A + B.** El agente no queda capado en el uso normal: con la accesibilidad activa el overlay se dibuja siempre, y el fallo solo aparece cuando tampoco podría tocar. Lo mínimo si el líder lo prefiere: solo T1; pero entonces `adbuddy screenshot` y los `screencap` siguen dando los píxeles sin aviso (T2 pasa a pendiente).

## 3. Tareas

Cada tarea compila y pasa sus tests sola; T2 no depende de T1.

### T1 — `dump` falla cerrado sin aviso (app, JVM)

Ficheros: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/UiExecutor.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/a11y/UiExecutorTest.kt`, `host/internal/daemon/ui_test.go`.

Tests primero. En `FakeScreen` añade `var windowsRead = false` al cuerpo y cambia `override fun windows() = windows` por `override fun windows() = windows.also { windowsRead = true }`. En `withoutTheNoticeClickAndSetTextDoNotAct` quita la línea `assertTrue(x.run(req(UiOp.DUMP)).ok)` y añade:

```kotlin
    @Test fun withoutTheNoticeDumpDoesNotRead() {
        val s = FakeScreen(actingWorks = false)
        screen = s
        val a = x.run(req(UiOp.DUMP))
        assertEquals(ErrorCode.FAILED, error(a))
        assertEquals("ADBuddy can't show the on-screen notice, so it won't read the screen. Ask the user to do this part.", a.error!!.text)
        // The tree is not read at all: the check comes before s.windows().
        assertFalse(s.windowsRead)
        // Foreground only reads the name of the app, not what it shows: it still answers.
        assertTrue(x.run(req(UiOp.FOREGROUND)).ok)
    }

    @Test fun aProtectedAppIsRefusedBeforeTheNotice() {
        screen = FakeScreen(actingWorks = false, fg = "com.example.bank" to "com.example.bank.Main")
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
    }

    @Test fun pausedDumpIsRefused() {
        // Paused: AgentAccessibilityService.acting() is true without drawing, so the dump must not rely on it
        // (the pause can arrive after the host's doors looked).
        paused = true
        val s = FakeScreen()
        screen = s
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
        assertFalse(s.windowsRead)
    }
```

(si `FakeScreen` nombra de otra forma el parámetro del primer plano, usar el suyo; ver su constructor.)

Test de host (`ui_test.go`, junto a los de dump): `TestDumpThatTheAppCannotAnnounceDoesNotFallBackToADB` (de caracterización: ya pasa hoy porque `CodeFailed` no activa el respaldo; fija que siga así): `a.errs[bridge.UIOpDump] = &UIError{Code: bridge.CodeFailed, Text: "can't show the notice"}`; esperado `UI_FAILED` y ninguna llamada `uiautomator dump` en `e.adb` (`fallsBack` solo salta con `unavailable`, `ui.go:328-332`).

Código (`UiExecutor.kt`, en `dump`; sustituye la línea 194 `s.acting(null)`):

```kotlin
        if (!locked && guarded(pkg)) return protectedApp(r, pkg)
        if (paused()) return pausedNow(r)
        if (!s.acting(null)) return noNoticeToRead(r)
```

Cambia también `pausedNoTapsNorTyping` (`UiExecutorTest.kt:222`): `assertTrue(x.run(req(UiOp.DUMP)).ok)` pasa a `assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))`. El host convierte `protected` en `UI_PROTECTED` (salida 126) sin respaldo por adb (`appFailure`, `ui.go:345`). `foreground` y `notifications` siguen sin comprobar la pausa (no leen contenido; `readingNotificationsDoesNotNeedADishonestHeadsUpOrAPause`).

y junto a `noNotice`:

```kotlin
    private fun noNoticeToRead(r: UiRequest) = fail(r, ErrorCode.FAILED, "ADBuddy can't show the on-screen notice, so it won't read the screen. Ask the user to do this part.")
```

**Prueba no vacua.** Mutación 1: volver a `s.acting(null)` sin mirar el resultado → `withoutTheNoticeDumpDoesNotRead` en rojo (`ok` en vez de `FAILED`). Mutación 2: mover `noNoticeToRead` antes de la comprobación de app protegida → `aProtectedAppIsRefusedBeforeTheNotice` en rojo (`FAILED` en vez de `PROTECTED`). Mutación 3: quitar `if (paused())` → `pausedDumpIsRefused` en rojo. Ejecutar: `cd android && gradle --no-daemon :app:testDebugUnitTest --tests '*UiExecutorTest*'` y `cd host && go test -count=1 -timeout 3m ./internal/daemon/ -run 'Dump'` (tope 10 min cada uno).

### T2 — aviso previo a las lecturas por adb (protocolo + host + app)

**Protocolo** (común; los tests de fixtures lo exigen en los dos lados):
- `host/internal/bridge/ui_ops.go`: `UIOpNotice = "notice"` y en `UIOps`: `UIOpNotice: {func() any { return new(Empty) }, func() any { return new(Empty) }},`.
- `ProtocolUiOps.kt`: `const val NOTICE = "notice"` y `NOTICE to (Empty.serializer() to Empty.serializer())` en `ALL`.
- `protocol/fixtures/ui.json`: `"notice"` en `ops` y una respuesta `"ui.response@notice": "notice"` con su fichero de ejemplo (`result: {}`), a la manera de las de `foreground` (los tests `TestUIOps` y `UiOpsFixturesTest.kt:79` comparan los conjuntos) y `protocol/fixtures/host/ui.request@notice.json` (`args: {}`) y `protocol/fixtures/app/ui.response@notice.json` (`result: {}`); el nombre `notice` coincide con el tipo de mensaje de primer nivel `notice` pero son espacios distintos (op de `ui.request` frente a `type`): se queda, ruling en el ledger. Proceso: visto bueno de adbuddy-e5 para cambiar el protocolo (M5 §12) y Go + Kotlin + fixtures en un solo commit (protocolo §10).

**App** (`UiExecutor.run`, en el `when`):

```kotlin
                UiOp.NOTICE -> if (args(r, Empty.serializer()) == null) invalid(r) else notice(r, s)
```
```kotlin
    /** The host is about to read the screen by adb (screencap, uiautomator): show the notice first. It reads nothing, so no heads-up check; the pause does count. */
    private fun notice(r: UiRequest, s: Screen): UiResponse = when {
        paused() -> pausedNow(r) // acting() is true without drawing while paused: the host must not read then
        s.acting(null) -> ok(r, Empty, Empty.serializer())
        else -> noNoticeToRead(r)
    }
```
La rama `UiOp.NOTICE` va antes del `else ->` del `when` (`UiExecutor.kt:140`).

**Host** (`ui.go`):

Primero se extrae de `uiautomator()` (`ui.go:449-455`) la comprobación que ya hace, `func (d *Daemon) serviceOn(ctx context.Context) (bool, error)` (`ADB.Shell("dumpsys accessibility")` + `uidump.ServiceOn`; error si el shell falla o sale con código distinto de 0), y `uiautomator()` pasa a usarla (mismo comportamiento: error → `UI_UNAVAILABLE`).

```go
// showNotice asks the app to put up its on-screen notice before the host reads
// the screen by adb (screenshot, raw screencap/screenrecord/uiautomator,
// dumpsys that dumps the view). Allowlist: only these let the read go on.
// Anything else (failed, protected because of a pause, invalid, expired, a
// channel error) refuses, as tap and type do.
func (d *Daemon) showNotice(ctx context.Context) string {
	_, err := d.callApp(ctx, bridge.UIOpNotice, nil)
	var ue *UIError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &ue) && ue.Code == bridge.CodeUnsupported:
		return "" // an older app that answers: it cannot show the notice, but it is not stuck
	case errors.Is(err, ErrNoApp), errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable:
		// No app, or accessibility off: nobody to tell... unless the
		// accessibility service is still bound (the app is stuck, not absent),
		// or it cannot be checked (fail closed, as uiautomator() does).
		on, serr := d.serviceOn(ctx)
		switch {
		case serr != nil:
			return "could not check ADBuddy's accessibility service, so ADBuddy cannot tell whether it can show the on-screen notice; ask the user to do this part"
		case on:
			return "the ADBuddy app does not answer but its accessibility service is on, so it cannot show the on-screen notice; ask the user to open the ADBuddy app"
		}
		return ""
	case errors.Is(err, ErrUITimeout): // 10 s: the app hung; the overlay answers in 2 s at most
		return "the ADBuddy app did not answer in time, so ADBuddy cannot show the on-screen notice; ask the user to open the ADBuddy app"
	}
	return "ADBuddy can't show the on-screen notice, so it won't read the screen; ask the user to do this part"
}
```

Nota de efecto: un `unsupported` (app antigua con accesibilidad activa) deja leer; el coste de `serviceOn` es un `adb shell` extra solo cuando no hay app o está apagada, y los tests de lecturas sin app preparan `e.adb.shell["dumpsys accessibility"]` (el `fakeADB.Shell` devuelve exit 1 por defecto, `daemon_test.go:38`) y cuentan una entrada más en `e.adb.ran()`: revisar `grep -ln screencap host/internal/daemon/*_test.go` (daemon, dumpsys_guard, guard_api34, guard_home, guard_notice, shade, ui).

Llamadas (siempre ANTES de la última comprobación de pausa, para que una pausa que llegue mientras tanto bloquee):
- `ui()`, justo antes de `if d.o.Paused()` de `ui.go:144` («paused while the guard looked»): `if req.UIOp == OpScreenshot { if why := d.showNotice(ctx); why != "" { return d.blockUI(e, why) } }`. `blockUI` pone el prefijo `Blocked by ADBuddy:` (M5 §8). Si `ctx` se canceló durante el aviso, se repite la rama de abandono de la guarda (`ui.go:134-139`: `abandoned`, no `blocked`), mejor en un helper común.
- `daemon.go`, justo después de `d.guard` (`:592`) y antes de la comprobación de pausa de `:595`: `if readsScreen(res) { if why := d.showNotice(ctx); why != "" { return d.refuse(ctx, e, s, why) } }`, con `readsScreen(res classify.Result) bool` que recorre `res.Findings` con `screenAction` (`reads`), sin cambiar la firma de `guard`.

Tests (host, adb y app falsos; `fakeApp` registra `calls`): `TestScreenshotAsksTheAppForTheNoticeFirst` (`UIOpNotice` en `a.calls` antes de `exec-out screencap -p`); `TestRawReadsAskForTheNotice` (tabla de argv: `exec-out screencap -p`, `shell screencap -p /sdcard/s.png`, `shell uiautomator dump /sdcard/u.xml`, `shell dumpsys activity top`, y `shell screenrecord /sdcard/r.mp4`, que es 🔴: el test contesta la confirmación y comprueba que el aviso sale después del Allow); `TestNoNoticePossibleStillReads` (`ErrNoApp`, `unavailable`, `unsupported`, con el servicio no vinculado: la captura sale; `unsupported` sale siempre); `TestStuckAppWithServiceBoundBlocks` (también `dumpsys accessibility` que falla → bloquea; `ErrNoApp` o tiempo agotado con el servicio vinculado → `UI_BLOCKED`, ningún `screencap`); `TestAppCannotDrawNoticeBlocksTheScreenshot` (`CodeFailed` → `UI_BLOCKED` con prefijo `Blocked by ADBuddy:`, ningún `screencap`); `TestAppPausedAnswerBlocks` (`CodeProtected` → bloquea); `TestPauseDuringTheNoticeBlocksTheScreenshot` (el `fakeApp` gana un campo `onCall func(op string)`, llamado fuera de `a.mu`, que activa la pausa al recibir `notice`: ningún `screencap`); `TestNoticeComesAfterTheGuard` (app protegida delante: `UI_BLOCKED` por la guarda y ninguna op `notice`). App: `noticeOpShowsTheOverlay` (`did` contiene `acting null`), `noticeOpFailsClosed` (`actingWorks=false` → `FAILED`) y `noticeOpRefusedWhilePaused` (`paused=true` → `PROTECTED`, `did` sin `acting`).

**Prueba no vacua.** Quitar la llamada de `ui()` → `TestScreenshotAsksTheAppForTheNoticeFirst` en rojo; quitar la de `daemon.go` → `TestRawReadsAskForTheNotice` en rojo; `showNotice` que siempre devuelve `""` → `TestAppCannotDrawNoticeBlocksTheScreenshot` en rojo; mover la llamada después de la comprobación de pausa → `TestPauseDuringTheNoticeBlocksTheScreenshot` en rojo; `notice` de la app que devuelve `ok` sin llamar a `acting` → `noticeOpShowsTheOverlay` en rojo; quitar `paused() ->` de `notice` → `noticeOpRefusedWhilePaused` en rojo. Ejecutar con `cd host && go test -count=1 -timeout 3m ./internal/daemon/ ./internal/bridge/` y el gradle de T1 (más `*UiOpsFixturesTest*`).

**Compatibilidad.** Una app anterior contesta `unsupported` (`UiExecutor.kt:140`): el host sigue leyendo si el servicio no está vinculado. El orden de merge entre host y app es indiferente. `screenrecord` solo muestra el aviso al empezar (el overlay se oculta a los 4 s, `HIDE_AFTER_MS`): se documenta; es 🔴 y la persona lo confirma.


### T3 — Docs y validación

M5 §6 «Cuándo»: «…y con `dump` (si no se puede mostrar, `dump` falla como `click` y `setText`) y antes de `screenshot` y de los `screencap`/`screenrecord`/`uiautomator` y `dumpsys` que vuelque la vista (crudos) (si la app no puede mostrarlo, se bloquean; sin app o sin accesibilidad no hay aviso posible)». Una línea en la sección de usuario «Cuando el agente usa la pantalla». Validación en emulador (cuando haya turno; no en esta tarea): `emutest` lanza `adbuddy screenshot` y comprueba con `dumpsys window` la ventana `TYPE_ACCESSIBILITY_OVERLAY` de ADBuddy; la mutación de T2 la pone en rojo. Apuntar en «Validaciones manuales pendientes».

## Validaciones manuales pendientes

Necesitan emulador o móvil. Hechas en emulador (AVD adbuddy-dev36, API 36) el 2026-10-11 con `host/internal/emutest/notice_test.go`:

- (a) HECHA: `TestTheNoticeWhenTheHostReadsTheScreen`. Tras `adbuddy screenshot` están las dos ventanas `TYPE_ACCESSIBILITY_OVERLAY` de ADBuddy (marco y píldora) en `dumpsys window windows`, y el marco sale en la propia captura (los 6 puntos de los bordes difieren de un `screencap` sin overlay). `adbuddy screen` (dump de la app) también lo muestra. En pausa (solo la app, y luego también el host) ambas lecturas se bloquean y el overlay no aparece. Mutaciones en rojo: quitar `showNotice` del screenshot en `ui.go` y una op `notice` de la app que contesta ok sin `acting` («the overlay is not on screen»). Nota: justo tras activar la accesibilidad llega siempre una notificación y el guarda rechaza la primera lectura («A notification that just arrived…»); el test reintenta a los 10 s, como indica el propio mensaje.
- (a, texto original) Overlay visible durante `adbuddy screenshot` en emulador: `emutest` lanza `adbuddy screenshot` con la accesibilidad activa y comprueba con `dumpsys window` que existe la ventana `TYPE_ACCESSIBILITY_OVERLAY` de ADBuddy; la mutación de T2 la pone en rojo.
- (b) HECHA: `TestADumpOfTheLockedScreen` (PIN de prueba, pantalla de bloqueo): el overlay se dibuja sobre el keyguard y el dump se lee (`locked=true`, 0 elementos, marco y píldora en pantalla); no devuelve FAILED. El test acepta ambas salidas y falla si se lee sin overlay.
- (b, texto original) `dump` con la pantalla bloqueada: o el overlay se dibuja sobre el keyguard, o `dump` devuelve `FAILED` («won't read», porque la app no puede mostrar el aviso). Ambas son aceptables; lo que no puede pasar es leer sin aviso.

## 4. Rulings

Ver `.superpowers/sdd/2026-10-11-screen-overlay-frame/progress.md`.
