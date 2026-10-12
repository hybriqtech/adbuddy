# Pantalla oculta y overlay en la captura: plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que `adbuddy screenshot` no lleve dentro el marco ni la píldora del overlay, y que `adbuddy screen` explique al agente, con verdad y sin rodeos, por qué no puede leer una pantalla que el sistema o la app ocultan.

**Architecture:** (1) Nueva op `hideNotice {ms}` host→app: la app quita marco y píldora, espera 3 fotogramas y se restaura sola a los `ms`; el host la usa solo dentro de `screenshot` (`notice` → `hideNotice` → `screencap` → `notice`), y si no puede quitar el aviso lo dice (`noticeInImage`). (2) Campo aditivo `UiDump.unreadable` que la app calcula en `Dumper` y que `printScreen` convierte en una nota fija para el agente. (3) Una línea de log del servicio, sin contenido, para diagnosticar en el Pixel por qué el sistema no entrega el contenido.

**Tech Stack:** Go 1.27 (`host/`), Kotlin + kotlinx.serialization + Android (`android/`), fixtures JSON compartidos (`protocol/fixtures/`), emulador API 36 para `emutest`.

**Spec:** `docs/superpowers/specs/2026-10-12-p1-pantalla-oculta-design.md` (complementa `2026-09-29-adbuddy-v1-design.md` §5.7 y `2026-10-01-adbuddy-m5-ayudas-remoto-design.md` §3.1, §6, §12).

## Global Constraints

- Código y comentarios en inglés; docs, ledger y mensajes de commit en español como el resto del repo. Las cadenas para el agente (notas del CLI) son inglés fijo.
- Falla cerrado: **no** se toca el clasificador, la guarda de primer plano, la pausa ni los bloqueos existentes. Una pausa nunca se salta (ni `hideNotice` ni el temporizador de restauración la ignoran).
- `hideNotice` **no es un helper del agente** (no está en `helpers`, no tiene comando): solo `screenshot` la llama. Cualquier `tap`/`type` vuelve a mostrar el overlay (`acting`).
- Las lecturas en bruto (`screencap`, `screenrecord`, `uiautomator`, `dumpsys` por `adb shell`) siguen con `showNotice` y **sin** retirarlo.
- Protocolo: cambios **aditivos** (op `hideNotice`, campo `unreadable`); cada cambio va en **un solo commit** con Go, Kotlin y fixtures (protocolo §10) y con el visto bueno del líder (M5 §12). No se declara `isAccessibilityTool` ni se añade `canTakeScreenshot`.
- Tiempos: `MaxHideNoticeMs = 5000`; el host pide `2000`; el aviso lleva en pantalla `MIN_SHOWN_MS = 400` antes de retirarse; tras retirarlo el servicio espera 3 fotogramas (`FRAMES_AFTER_HIDE`) y al menos `MIN_GONE_MS = 50`; `hideFor` se rinde a los `HIDE_WAIT_MS = 2000`.
- Cada tarea compila y pasa sus tests por sí sola. Nada de `go test -race` en este PC. Pasa siempre `-timeout` a `go test`.
- Commits: identidad local hybriqtech y trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Sin `--force`. Antes de un push: `git grep` de seriales, IPs (`100.x`, `192.168.x`) y rutas personales en lo añadido.
- Nunca se prueba nada en el móvil del usuario: tests JVM, adb falso y emulador API 36 (`adbuddy-p1-36`); Android 17 no corre en emulador.
- Revisores: `adbuddy-reviewer-opus` (esto toca overlay y guarda); `adbuddy-rereviewer` para re-revisiones. Nunca Haiku.

## Comandos de verificación

- Host: `cd host && go vet ./... && go test -timeout 20m ./...`
- App: `cd android && gradle --offline --no-daemon :app:testDebugUnitTest lintDebug` (en el PC de desarrollo: `ANDROID_HOME=$LOCALAPPDATA/Android/Sdk` y `gradle.bat` de `$LOCALAPPDATA/Gradle/gradle-9.8.0/bin`; no hay `gradlew`).
- Emulador (T4): `cd host && go test -count=1 -timeout 8m -v -run <Test> ./internal/emutest`, con el emulador `adbuddy-p1-36` arrancado por `scripts/emulator.sh start` y el APK debug construido (`:app:assembleDebug`); variables en el ledger.

## Mapa de ficheros

| Fichero | Responsabilidad |
|---|---|
| `host/internal/bridge/ui_ops.go` | op `hideNotice`, `UIHideNoticeArgs`, `MaxHideNoticeMs`, `UIDump.Unreadable` |
| `android/.../bridge/ProtocolUiOps.kt` | espejo Kotlin de lo anterior |
| `protocol/fixtures/ui.json`, `host/ui.request@hidenotice.json`, `app/ui.response@hidenotice.json`, `app/ui.response@dumpunreadable.json` | contrato compartido |
| `android/.../a11y/Overlay.kt` | `hideFor(ms)` y restauración automática |
| `android/.../a11y/UiExecutor.kt` | op `hideNotice`, `Screen.hideNotice` |
| `android/.../a11y/AgentAccessibilityService.kt` | `hideNotice`, línea de log de diagnóstico |
| `android/.../a11y/Dumper.kt` | `unreadable`, `describe` |
| `host/internal/daemon/notice.go`, `ui.go` | `veilNotice`, `restoreNotice`, `screenshot` con velo, `ScreenshotResult.NoticeInImage` |
| `host/internal/cli/helpers.go`, `i18n/catalog/{en,es}.toml` | notas del CLI y ayuda |
| `host/internal/emutest/notice_test.go` | pruebas de emulador |

---

### Task 1: Protocolo, op `hideNotice`

**Files:**
- Modify: `host/internal/bridge/ui_ops.go`
- Modify: `host/internal/bridge/ui_ops_test.go` (`TestUILimits`)
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ProtocolUiOps.kt`
- Modify: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/UiOpsFixturesTest.kt` (`theLimitsAreUiJsons`)
- Modify: `protocol/fixtures/ui.json`
- Create: `protocol/fixtures/host/ui.request@hidenotice.json`, `protocol/fixtures/app/ui.response@hidenotice.json`

**Interfaces:**
- Produces (Go): `bridge.UIOpHideNotice = "hideNotice"`, `bridge.UIHideNoticeArgs{Ms int}` (`json:"ms"`), `bridge.MaxHideNoticeMs = 5000`; entrada en `UIOps` (args `UIHideNoticeArgs`, result `Empty`).
- Produces (Kotlin): `UiOp.HIDE_NOTICE`, `UiHideNoticeArgs(val ms: Int)`, `UiLimits.MAX_HIDE_NOTICE_MS = 5000`; entrada en `UiOp.ALL`.

- [ ] **Step 1: Tests primero (rojo).** En `ui_ops_test.go`, `TestUILimits`, añade la línea al mapa `got`:

```go
		"maxHideNoticeMs":     MaxHideNoticeMs,
```

En `UiOpsFixturesTest.kt`, `theLimitsAreUiJsons`, añade al `mapOf`:

```kotlin
                "maxHideNoticeMs" to UiLimits.MAX_HIDE_NOTICE_MS,
```

- [ ] **Step 2: Ver que no compila.** `cd host && go test -timeout 5m ./internal/bridge/` falla con `undefined: MaxHideNoticeMs`.

- [ ] **Step 3: Go.** En `ui_ops.go`:

```go
// UI operations.
const (
	UIOpDump          = "dump"
	UIOpClick         = "click"
	UIOpSetText       = "setText"
	UIOpForeground    = "foreground"
	UIOpNotifications = "notifications"
	UIOpNotice        = "notice"
	UIOpHideNotice    = "hideNotice"
)
```

En `UIOps` añade, tras `UIOpNotice`:

```go
	UIOpHideNotice:    {func() any { return new(UIHideNoticeArgs) }, func() any { return new(Empty) }},
```

En el bloque de límites añade (alineado con los demás):

```go
	MaxHideNoticeMs     = 5000      // milliseconds hideNotice may keep the app's notice off the screen
```

Tras `UISetTextArgs`:

```go
// UIHideNoticeArgs are hideNotice's args: how long, from 1 to MaxHideNoticeMs
// milliseconds, the app keeps its on-screen notice (frame and pill) off the
// screen so that a capture by adb does not carry it. The app puts it back by
// itself when the time is up, unless the agent was paused meanwhile.
type UIHideNoticeArgs struct {
	Ms int `json:"ms"`
}
```

- [ ] **Step 4: Kotlin.** En `ProtocolUiOps.kt`: `const val HIDE_NOTICE = "hideNotice"` en `UiOp` (tras `NOTICE`), la entrada `HIDE_NOTICE to (UiHideNoticeArgs.serializer() to Empty.serializer()),` en `ALL`, `const val MAX_HIDE_NOTICE_MS = 5000` en `UiLimits` (tras `MAX_PACKAGE`) y, tras `UiSetTextArgs`:

```kotlin
/** hideNotice's args: how long, 1 to MAX_HIDE_NOTICE_MS milliseconds, the notice stays off the screen for a capture by adb; it comes back by itself. */
@Serializable
data class UiHideNoticeArgs(val ms: Int)
```

- [ ] **Step 5: Fixtures.** `protocol/fixtures/ui.json`: añade `"hideNotice"` al final de `ops` (tras `"notice"`, con coma), `"ui.response@hidenotice": "hideNotice"` en `responses` (tras la de `notice`) y `"maxHideNoticeMs": 5000` al final de `limits` (tras `maxPackage`). Crea `protocol/fixtures/host/ui.request@hidenotice.json`:

```json
{
  "v": 1,
  "type": "ui.request",
  "ts": "2026-10-01T10:00:00Z",
  "payload": {
    "id": "u_8",
    "op": "hideNotice",
    "args": {
      "ms": 2000
    },
    "expiresAt": "2026-10-01T10:00:10Z"
  }
}
```

y `protocol/fixtures/app/ui.response@hidenotice.json`:

```json
{
  "v": 1,
  "type": "ui.response",
  "ts": "2026-10-01T10:00:00Z",
  "payload": {
    "id": "u_8",
    "ok": true,
    "result": {}
  }
}
```

- [ ] **Step 6: Verde.** `cd host && go vet ./internal/bridge/ && go test -timeout 5m ./internal/bridge/` y `cd android && gradle --offline --no-daemon :app:testDebugUnitTest --tests '*UiOpsFixturesTest*'`. Los dos pasan. Nota: `UiExecutor` aún no maneja la op (responde `unsupported`); T2 lo hace.

- [ ] **Step 7: Mutación.** Cambia `MaxHideNoticeMs` a `5001`: `TestUILimits` en rojo; revierte. Cambia `"ms": 2000` del fixture a `"ms": "2000"`: `TestUIOps/ui.request@hidenotice` en rojo; revierte. Apúntalas en el ledger.

- [ ] **Step 8: Commit** (Go, Kotlin y fixtures juntos):

```bash
git add host/internal/bridge protocol/fixtures android/app/src
git commit -m "feat(protocolo): op hideNotice para retirar el aviso durante una captura

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: App, `hideNotice` (Screen, UiExecutor, Overlay, servicio)

Paralelizable con T3 (árboles disjuntos: `android/` y `host/`).

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/UiExecutor.kt`
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/Overlay.kt`
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/AgentAccessibilityService.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/a11y/UiExecutorTest.kt`

**Interfaces:**
- Consumes (T1): `UiOp.HIDE_NOTICE`, `UiHideNoticeArgs(ms: Int)`, `UiLimits.MAX_HIDE_NOTICE_MS`.
- Produces: `Screen.hideNotice(ms: Long): Boolean`; `Overlay(service, isPaused: () -> Boolean, onPause: () -> Unit)` con `fun hideFor(ms: Long): Boolean`.

- [ ] **Step 1: Tests primero.** En `UiExecutorTest.kt`: añade `import tech.hybriq.adbuddy.bridge.UiLimits`; al constructor de `FakeScreen` añade `var hideWorks: Boolean = true,` tras `actingWorks`; y en el cuerpo, tras `acting`:

```kotlin
        override fun hideNotice(ms: Long): Boolean {
            did += "hideNotice $ms"
            return hideWorks
        }
```

Tras `noticeOpRefusedWhilePaused` añade:

```kotlin
    @Test fun hideNoticeTakesTheOverlayOffForTheCapture() {
        val s = FakeScreen()
        screen = s
        val r = x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))
        assertTrue(r.ok)
        assertEquals(listOf("hideNotice 2000"), s.did)
        assertFalse(s.windowsRead)
    }

    @Test fun hideNoticeFailsWhenTheOverlayCannotBeTakenOff() {
        val s = FakeScreen(hideWorks = false)
        screen = s
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
    }

    @Test fun hideNoticeIsRefusedWhilePaused() {
        paused = true
        val s = FakeScreen()
        screen = s
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
        assertTrue(s.did.isEmpty())
    }

    @Test fun hideNoticeNeedsATimeWithinTheLimit() {
        val s = FakeScreen()
        screen = s
        for (ms in listOf(0, -1, UiLimits.MAX_HIDE_NOTICE_MS + 1)) {
            assertEquals("ms=$ms", ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to ms)))))
        }
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000, "extra" to 1)))))
        assertTrue(s.did.isEmpty())
        assertTrue(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 1))).ok)
        assertTrue(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to UiLimits.MAX_HIDE_NOTICE_MS))).ok)
    }

    @Test fun hideNoticeWithoutTheServiceIsUnavailable() {
        screen = null
        assertEquals(ErrorCode.UNAVAILABLE, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
    }
```

- [ ] **Step 2: Rojo.** `gradle ... :app:testDebugUnitTest --tests '*UiExecutorTest*'` no compila (`hideNotice` no existe en `Screen`).

- [ ] **Step 3: `UiExecutor.kt`.** Importa `tech.hybriq.adbuddy.bridge.UiHideNoticeArgs`. En `interface Screen`, tras `acting`:

```kotlin
    /**
     * The host is about to capture the screen by adb: take the overlay off the
     * screen for ms at most and wait until it is gone from what is drawn. It
     * comes back by itself (never while paused), or with the next acting().
     * false when it could not be taken off.
     */
    fun hideNotice(ms: Long): Boolean
```

En `run`, tras la rama `UiOp.NOTICE`:

```kotlin
                UiOp.HIDE_NOTICE -> hideNotice(r, s, args(r, UiHideNoticeArgs.serializer()) ?: return invalid(r))
```

Y tras la función `notice`:

```kotlin
    /** The host is about to capture the screen by adb: take the notice off the screen for the capture. It reads nothing, so no heads-up check; the pause does count. */
    private fun hideNotice(r: UiRequest, s: Screen, a: UiHideNoticeArgs): UiResponse = when {
        a.ms < 1 || a.ms > UiLimits.MAX_HIDE_NOTICE_MS -> invalid(r)
        paused() -> pausedNow(r)
        s.hideNotice(a.ms.toLong()) -> ok(r, Empty, Empty.serializer())
        else -> fail(r, ErrorCode.FAILED, "ADBuddy could not take its notice off the screen for the capture.")
    }
```

- [ ] **Step 4: `Overlay.kt`.** Imports nuevos: `android.view.Choreographer`, `java.util.concurrent.CompletableFuture`. Cambia el constructor y el comentario de clase:

```kotlin
 * the pill takes touches; it hides 4 s after the agent's last action. Its
 * windows are ADBuddy's, so the helpers never show nor touch them. For a
 * capture by adb the host asks it to go off the screen for a moment
 * (hideFor); it comes back by itself, never while the agent is paused.
 */
class Overlay(
    private val service: AccessibilityService,
    private val isPaused: () -> Boolean,
    private val onPause: () -> Unit,
) {
    private val main = Handler(Looper.getMainLooper())
    private val wm = service.getSystemService(WindowManager::class.java)
    @Volatile private var frame: FrameView? = null
    @Volatile private var pill: TextView? = null
    private val hider = Runnable { remove() }

    /** Puts the overlay back after hideFor's time, unless the agent was paused meanwhile. */
    private val restorer = Runnable { if (!isPaused()) showNow(null) }
```

En `showNow`, antes de `main.removeCallbacks(hider)` añade `main.removeCallbacks(restorer)`:

```kotlin
        mark?.let { frame?.mark(it) }
        main.removeCallbacks(restorer)
        main.removeCallbacks(hider)
        main.postDelayed(hider, HIDE_AFTER_MS)
        return true
```

Sustituye `remove()` por:

```kotlin
    private fun remove() {
        main.removeCallbacks(hider)
        main.removeCallbacks(restorer)
        removeViews()
    }

    private fun removeViews() {
        frame?.let { runCatching { wm.removeView(it) } }
        pill?.let { runCatching { wm.removeView(it) } }
        frame = null
        pill = null
    }

    /**
     * Takes the frame and the pill off the screen for a capture by adb (from
     * any thread but the main one) and waits until the screen has been drawn
     * without them. They have been on the screen for MIN_SHOWN_MS at least
     * first, so that the person can see the notice before the capture. They
     * come back by themselves ms after they went, unless the agent is paused
     * then; show() and remove() cancel that. true when they are off the
     * screen or were not on it; false when they could not be taken off (they
     * are put back at once).
     */
    fun hideFor(ms: Long): Boolean {
        val done = CompletableFuture<Boolean>()
        val abandoned = AtomicBoolean(false)
        main.post {
            val dwell = (MIN_SHOWN_MS - (SystemClock.uptimeMillis() - shownAt)).coerceAtLeast(0)
            main.postDelayed({ if (!abandoned.get()) takeOff(ms, done) }, dwell)
        }
        return try {
            done.get(HIDE_WAIT_MS, TimeUnit.MILLISECONDS)
        } catch (e: Exception) {
            Log.e(TAG, "overlay did not hide in time", e)
            abandoned.set(true)
            main.post(restorer)
            false
        }
    }

    /** hideFor's work, on the main thread. */
    private fun takeOff(ms: Long, done: CompletableFuture<Boolean>) {
        try {
            if (frame == null) { // nothing on the screen: nothing to hide
                done.complete(true)
                return
            }
            main.removeCallbacks(hider)
            main.removeCallbacks(restorer)
            removeViews()
            main.postDelayed(restorer, ms)
            val removedAt = SystemClock.uptimeMillis()
            afterFrames(FRAMES_AFTER_HIDE) {
                // A fast display draws three frames in 25 ms: wait a minimum too.
                val rest = MIN_GONE_MS - (SystemClock.uptimeMillis() - removedAt)
                if (rest > 0) main.postDelayed({ done.complete(true) }, rest) else done.complete(true)
            }
        } catch (e: RuntimeException) {
            Log.e(TAG, "overlay did not hide", e)
            restorer.run()
            done.complete(false)
        }
    }

    /** Calls done on the main thread once n frames have been drawn. */
    private fun afterFrames(n: Int, done: () -> Unit) {
        val choreographer = Choreographer.getInstance()
        var left = n
        choreographer.postFrameCallback(object : Choreographer.FrameCallback {
            override fun doFrame(frameTimeNanos: Long) {
                if (--left <= 0) done() else choreographer.postFrameCallback(this)
            }
        })
    }
```

Añade también el campo `@Volatile private var shownAt = 0L` junto a `frame` y `pill`, y en `add()`, justo después de `frame = f; pill = p`, `shownAt = SystemClock.uptimeMillis()`. Imports: `java.util.concurrent.atomic.AtomicBoolean` (`SystemClock` ya está). En `companion object` añade:

```kotlin
        /** The overlay is on the screen this long at least before it is taken off for a capture: the person sees the notice first (ruling R10). */
        const val MIN_SHOWN_MS = 400L

        /** At least this long off the screen before hideFor answers, whatever the display's rate. */
        const val MIN_GONE_MS = 50L

        /** hideFor gives up after this long (the dwell, three frames and the minimum fit well inside). */
        const val HIDE_WAIT_MS = 2_000L
        const val FRAMES_AFTER_HIDE = 3
```

El `setOnClickListener` de la píldora sigue llamando `onPause()` y `remove()`.

- [ ] **Step 5: `AgentAccessibilityService.kt`.** Sustituye la creación del overlay:

```kotlin
        overlay = Overlay(this, isPaused = { (application as AdbuddyApp).bridge.state.value.sessionState.paused }) {
            (application as AdbuddyApp).bridge.pause(true)
        }
```

y tras `acting`:

```kotlin
    override fun hideNotice(ms: Long): Boolean = overlay?.hideFor(ms) ?: true // no overlay object: nothing on the screen
```

- [ ] **Step 6: Verde.** `gradle --offline --no-daemon :app:testDebugUnitTest lintDebug`. Todo pasa (los tests JVM no pueden ejercitar `Overlay`, que usa `Handler`: lo cubre T4 en emulador).

- [ ] **Step 7: Mutaciones** (apuntar en el ledger): quita la rama `paused() -> pausedNow(r)` de `hideNotice` → `hideNoticeIsRefusedWhilePaused` en rojo; cambia `a.ms < 1` a `a.ms < 0` → `hideNoticeNeedsATimeWithinTheLimit` en rojo; revierte.

- [ ] **Step 8: Commit.**

```bash
git add android/app/src
git commit -m "feat(app): hideNotice quita el overlay durante una captura y vuelve solo

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Host, `screenshot` con el aviso retirado y `noticeInImage`

Paralelizable con T2.

**Files:**
- Modify: `host/internal/daemon/notice.go` (`veilNotice`, `restoreNotice`, `screenshotVeilMs`)
- Modify: `host/internal/daemon/ui.go` (`ScreenshotResult`, `screenshot`)
- Modify: `host/internal/daemon/ui_test.go` (`withApp`: `bridge.UIOpHideNotice: bridge.Empty{}`)
- Modify: `host/internal/daemon/notice_test.go` (`TestScreenshotAsksTheAppForTheNoticeFirst`: ahora hay dos `notice`)
- Create: `host/internal/daemon/veil_test.go`
- Modify: `host/internal/cli/helpers.go` (`newScreenshotCmd`, `noticeInImage`)
- Modify: `host/internal/cli/helpers_test.go`
- Modify: `host/internal/i18n/catalog/en.toml`, `es.toml` (`help.screenshot.long`)

**Interfaces:**
- Consumes (T1): `bridge.UIOpHideNotice`, `bridge.UIHideNoticeArgs`.
- Produces: `daemon.ScreenshotResult.NoticeInImage bool` (`json:"noticeInImage,omitempty"`).

- [ ] **Step 1: Tests primero.** En `ui_test.go`, `withApp`, añade a `results` tras `UIOpNotice`:

```go
		bridge.UIOpHideNotice: bridge.Empty{}, // like the real app
```

En `notice_test.go`, `TestScreenshotAsksTheAppForTheNoticeFirst`: ahora `screenshot` pide el aviso dos veces (antes de la captura y justo después), así que la comprobación pasa a:

```go
	// The first notice comes before screencap, the second (the notice put back) after it.
	if len(order) != 2 || strings.Contains(order[0], "screencap") || !strings.Contains(order[1], "screencap") {
		t.Fatalf("the notice must come before screencap and again after it: %q", order)
	}
```

Crea `host/internal/daemon/veil_test.go`:

```go
package daemon

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// The notice is taken off the screen while adbuddy screenshot captures it
// (plan 2026-10-12-p1-pantalla-oculta, T3).

type appCall struct {
	op  string
	adb int // adb commands that had run when the op reached the app
}

// noticeOps records, in order, the notice and hideNotice ops the app gets.
func noticeOps(e *env, a *fakeApp) *[]appCall {
	var order []appCall
	a.onCall = func(op string) {
		if op == bridge.UIOpNotice || op == bridge.UIOpHideNotice {
			order = append(order, appCall{op, len(ranOnPhone(e))})
		}
	}
	return &order
}

func resultOf(t *testing.T, s *stream) ScreenshotResult {
	t.Helper()
	if len(s.data) != 1 {
		t.Fatalf("data %q", s.data)
	}
	var r ScreenshotResult
	if err := json.Unmarshal(s.data[0], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScreenshotTakesTheNoticeOffForTheCapture(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	order := noticeOps(e, a)
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	// notice and hideNotice before screencap, the notice again right after it.
	want := []appCall{{bridge.UIOpNotice, 0}, {bridge.UIOpHideNotice, 0}, {bridge.UIOpNotice, 1}}
	if len(*order) != len(want) {
		t.Fatalf("ops %+v, want %+v", *order, want)
	}
	for i := range want {
		if (*order)[i] != want[i] {
			t.Fatalf("ops %+v, want %+v", *order, want)
		}
	}
	var hide string
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpHideNotice+" ") {
			hide = c
		}
	}
	if hide != `hideNotice {"ms":2000}` {
		t.Fatalf("hideNotice call %q", hide)
	}
	if r := resultOf(t, s); r.NoticeInImage {
		t.Fatalf("result %+v: the notice was taken off, nothing to tell", r)
	}
}

func TestScreenshotWithoutTheVeilSaysSo(t *testing.T) {
	for _, c := range []struct {
		name     string
		err      error
		inImage  bool // the notice may be in the image
		restored bool // the notice was asked again after the capture
	}{
		{"an older app", &UIError{Code: bridge.CodeUnsupported, Text: "old"}, true, false},
		{"the app failed", &UIError{Code: bridge.CodeFailed, Text: "no"}, true, true},
		{"the app hung", ErrUITimeout, true, true},
		{"accessibility off", &UIError{Code: bridge.CodeUnavailable, Text: "off"}, false, false},
		{"no app", ErrNoApp, false, false},
	} {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpHideNotice] = c.err
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
		if exit.Code != 0 || !screenRead(e) {
			t.Errorf("%s: exit %+v, the capture must go on", c.name, exit)
			continue
		}
		if r := resultOf(t, s); r.NoticeInImage != c.inImage {
			t.Errorf("%s: noticeInImage %v, want %v", c.name, r.NoticeInImage, c.inImage)
		}
		want := 1
		if c.restored {
			want = 2
		}
		if n := noticeCalls(a); n != want {
			t.Errorf("%s: the notice was asked %d times, want %d", c.name, n, want)
		}
	}
}

func TestScreenshotWithNoAppHasNoNoticeToTellOf(t *testing.T) {
	e := newEnv(t)
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if r := resultOf(t, s); r.NoticeInImage {
		t.Fatalf("result %+v", r)
	}
}

// A pause stops the capture, as it stops any read, wherever it is found: the
// app says so, or the host knows once the notice is off.
func TestScreenshotPausedWhileTheNoticeComesOffIsBlocked(t *testing.T) {
	for _, how := range []string{"the app says so", "the host knows"} {
		e := newEnv(t)
		a := e.withApp()
		paused := false
		e.d.o.Paused = func() bool { return paused }
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		if how == "the app says so" {
			a.errs[bridge.UIOpHideNotice] = &UIError{Code: bridge.CodeProtected, Text: "The agent is paused from the phone."}
		} else {
			a.onCall = func(op string) {
				if op == bridge.UIOpHideNotice {
					paused = true
				}
			}
		}
		s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
		if exit.Code != ExitDenied {
			t.Errorf("%s: exit %+v", how, exit)
		}
		if screenRead(e) || s.out.Len() != 0 {
			t.Errorf("%s: adb ran %q, %d bytes out", how, ranOnPhone(e), s.out.Len())
		}
		if n := noticeCalls(a); n != 1 {
			t.Errorf("%s: the notice was asked %d times: a paused session gets no notice back", how, n)
		}
	}
}

// A slow veil leaves the guard old: a protected app that came to the front
// meanwhile stops the capture, and the notice is put back.
func TestScreenshotLooksAtTheGuardAgainAfterASlowVeil(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	a.onCall = func(op string) {
		if op == bridge.UIOpHideNotice {
			time.Sleep(baselineSlack + 100*time.Millisecond)
			a.mu.Lock()
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank"}
			a.mu.Unlock()
		}
	}
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "is a protected app") {
		t.Fatalf("exit %+v", exit)
	}
	if screenRead(e) || s.out.Len() != 0 {
		t.Fatalf("adb ran %q, %d bytes out", ranOnPhone(e), s.out.Len())
	}
	if n := noticeCalls(a); n != 2 {
		t.Fatalf("the notice was asked %d times, want 2 (it is put back)", n)
	}
}

// hideNotice is for screenshot only: the agent cannot ask for the notice to
// go, and then act.
func TestTheAgentCannotAskToHideTheNotice(t *testing.T) {
	if _, ok := helpers[bridge.UIOpHideNotice]; ok {
		t.Fatal("hideNotice is a helper")
	}
	e := newEnv(t)
	a := e.withApp()
	_, exit := e.helper(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 2000})
	if exit.ErrorCode != "UI_INVALID" {
		t.Fatalf("exit %+v", exit)
	}
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpHideNotice) {
			t.Fatalf("the app got %q", c)
		}
	}
}

// The agent's own raw reads keep the notice on the screen (and in the image):
// only adbuddy screenshot takes it off.
func TestRawReadsDoNotHideTheNotice(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	order := noticeOps(e, a)
	if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 0 {
		t.Fatalf("exit %+v %q", exit, s.errOut.String())
	}
	if len(*order) != 1 || (*order)[0].op != bridge.UIOpNotice {
		t.Fatalf("ops %+v, want only the notice", *order)
	}
}
```

- [ ] **Step 2: Rojo.** `cd host && go test -timeout 5m ./internal/daemon/ -run 'Veil|TakesTheNoticeOff|WithoutTheVeil|NoNoticeToTell|PausedWhileTheNotice|CannotAskToHide|RawReadsDoNot'` no compila (`NoticeInImage`).

- [ ] **Step 3: `ui.go`.** En `ScreenshotResult`:

```go
type ScreenshotResult struct {
	Format string `json:"format"`
	Bytes  int    `json:"bytes"`
	// NoticeInImage: ADBuddy's on-screen notice could not be taken off the
	// screen for the capture and may be in the image.
	NoticeInImage bool `json:"noticeInImage,omitempty"`
}
```

Sustituye `screenshot`:

```go
// screenshot takes the capture by adb, scales it and sends it on stdout. The
// app's on-screen notice was shown before (showNotice); it is taken off the
// screen for the capture itself, so the frame and the pill do not cover the
// image, and put back right after.
func (d *Daemon) screenshot(ctx context.Context, a ScreenshotArgs, s Stream) (any, string, *uiFailure) {
	veilStart := time.Now()
	restore, inImage, f := d.veilNotice(ctx)
	if f != nil {
		return nil, "", f
	}
	if d.o.Paused() { // paused while the notice came off: nothing is read, and a paused agent gets no notice back
		return nil, "", fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+PausedMessage)
	}
	// The guard said yes before the notice. If taking the notice off was slow
	// (the app dwells so that the person can see it, or it hung), look at the
	// foreground again, as an act does after a slow baseline.
	if time.Since(veilStart) > baselineSlack {
		if why := d.guardForeground(ctx, nil); why != "" {
			restore()
			return nil, "", fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+why)
		}
		if d.o.Paused() {
			return nil, "", fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+PausedMessage)
		}
	}
	var buf, errb bytes.Buffer
	code, err := d.o.ADB.Run(ctx, "", []string{"exec-out", "screencap", "-p"}, nil, &limited{w: &buf, n: maxCapture}, &errb)
	restore()
	if err != nil || code != 0 {
		return nil, "", fail(1, "UI_FAILED", "screencap failed: "+classify.Printable(strings.TrimSpace(errb.String())))
	}
	img, err := screenshot.Scale(buf.Bytes(), a.Scale, a.Format)
	if err != nil {
		return nil, "", fail(1, "UI_FAILED", err.Error())
	}
	if _, err := s.Stdout().Write(img); err != nil {
		return nil, "", fail(1, "UI_FAILED", err.Error())
	}
	return ScreenshotResult{Format: a.Format, Bytes: len(img), NoticeInImage: inImage}, "adb", nil
}
```

- [ ] **Step 4: `notice.go`.** Añade imports `encoding/json`, `time` y:

```go
// screenshotVeilMs is how long the app keeps its notice off the screen for a
// screenshot at most (it puts it back by itself when the time is up). The
// capture takes 0.2 to 0.9 s on the phones measured.
const screenshotVeilMs = 2000

// restoreWait bounds the call that puts the notice back after a capture.
const restoreWait = 3 * time.Second

// veilNotice asks the app to take its on-screen notice off the screen for the
// screencap that follows, so the frame and the pill do not end up in the
// image. It returns what to call once the capture is done (it puts the
// notice back at once; the app would anyway when screenshotVeilMs is up) and
// whether the notice may be in the image all the same. A pause the app
// reports is a failure: a paused session reads nothing. Not being able to
// take the notice off never refuses the capture: the person saw it go up
// before (showNotice), and the agent is told the image may carry it.
func (d *Daemon) veilNotice(ctx context.Context) (restore func(), inImage bool, f *uiFailure) {
	raw, _ := json.Marshal(bridge.UIHideNoticeArgs{Ms: screenshotVeilMs})
	_, err := d.callApp(ctx, bridge.UIOpHideNotice, raw)
	var ue *UIError
	switch {
	case err == nil:
		return d.restoreNotice, false, nil
	case errors.Is(err, ErrNoApp), errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable:
		return func() {}, false, nil // no app: no notice on the screen
	case errors.As(err, &ue) && ue.Code == bridge.CodeUnsupported:
		return func() {}, true, nil // an older app cannot take it off
	case errors.As(err, &ue) && ue.Code == bridge.CodeProtected: // the only reason hideNotice gives it is a pause
		return nil, false, fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+PausedMessage)
	}
	// Failed, expired, no answer in time: it may or may not be off the screen. Put it back.
	return d.restoreNotice, true, nil
}

// restoreNotice puts the app's notice back after a capture. Best effort: the
// app does it by itself when the time it was given is up, and not for a
// paused session.
func (d *Daemon) restoreNotice() {
	ctx, cancel := context.WithTimeout(context.Background(), restoreWait)
	defer cancel()
	_, _ = d.callApp(ctx, bridge.UIOpNotice, nil)
}
```

- [ ] **Step 5: Verde en el daemon.** `cd host && go vet ./internal/daemon/ && go test -timeout 10m ./internal/daemon/`. Pasa, incluidos los demás tests de `notice_test.go` (el primer `notice` sigue antes que `screencap`).

- [ ] **Step 6: CLI, tests primero.** En `helpers_test.go` añade:

```go
// The notice that could not be taken off the screen is in the image: the agent is told.
func TestScreenshotSaysWhenTheNoticeIsInTheImage(t *testing.T) {
	img := []byte("\x89PNG fake bytes")
	helperDaemon(t, func(req ipc.Request, c *ipc.Conn) ipc.Exit {
		_, _ = c.Stdout().Write(img)
		_ = c.Data(daemon.ScreenshotResult{Format: "png", Bytes: len(img), NoticeInImage: true})
		return ipc.Exit{}
	})
	t.Chdir(t.TempDir())
	out, errOut, code := runHelper(t, "screenshot")
	if code != 0 || strings.Contains(out, "notice") || !strings.Contains(errOut, "on-screen notice") || !strings.Contains(errOut, "ignore it") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	out, _, code = runHelper(t, "screenshot", "-o", "s.png", "--json")
	if code != 0 || !strings.Contains(out, `"noticeInImage":true`) {
		t.Fatalf("json: %d %q", code, out)
	}
}

func TestScreenshotWithoutNoticeInTheImageSaysNothing(t *testing.T) {
	img := []byte("\x89PNG fake bytes")
	helperDaemon(t, func(req ipc.Request, c *ipc.Conn) ipc.Exit {
		_, _ = c.Stdout().Write(img)
		_ = c.Data(daemon.ScreenshotResult{Format: "png", Bytes: len(img)})
		return ipc.Exit{}
	})
	t.Chdir(t.TempDir())
	out, errOut, code := runHelper(t, "screenshot")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	out, _, code = runHelper(t, "screenshot", "-o", "s.png", "--json")
	if code != 0 || strings.Contains(out, "noticeInImage") {
		t.Fatalf("json: %d %q", code, out)
	}
}
```

Rojo: `go test ./internal/cli -run NoticeInTheImage` falla (no imprime nada).

- [ ] **Step 7: CLI.** En `helpers.go`, junto a `stillChanging`:

```go
// noticeInImage is what the agent reads on stderr when ADBuddy's on-screen
// notice could not be taken off the screen for a screenshot. Fixed English,
// for the agent.
const noticeInImage = "(ADBuddy's on-screen notice, a thin frame round the screen and a pill near the top, may be in this image because it could not be taken off the screen for the capture: ignore it)"
```

En `newScreenshotCmd`, sustituye el final de `RunE` (desde `if app.JSON {`):

```go
			if r.NoticeInImage && !app.JSON {
				fmt.Fprintln(app.Stderr, noticeInImage)
			}
			if app.JSON {
				data := map[string]any{"path": out, "format": r.Format, "bytes": img.Len()}
				if r.NoticeInImage {
					data["noticeInImage"] = true
				}
				return WriteData(app.Stdout, data)
			}
			fmt.Fprintln(app.Stdout, out)
			return nil
```

(Con `--json` la nota no va por stderr: el campo lo dice.) En los catálogos, añade al final de `help.screenshot.long`: EN `"While it captures, ADBuddy takes its on-screen notice (the frame and the pill) off the screen for a moment so it does not cover the image."`; ES `"Mientras captura, ADBuddy retira un instante su aviso en pantalla (el marco y la píldora) para que no tape la imagen."`, como frase nueva en una línea propia dentro del bloque.

- [ ] **Step 8: Verde.** `cd host && go vet ./... && go test -timeout 20m ./...`.

- [ ] **Step 9: Mutaciones** (ledger): borra `restore()` en `screenshot` → `TestScreenshotTakesTheNoticeOffForTheCapture` en rojo; pon `inImage` a `false` siempre en `veilNotice` → `TestScreenshotWithoutTheVeilSaysSo` en rojo; quita la comprobación `d.o.Paused()` tras el velo → el caso «the host knows» en rojo; quita `hideNotice` de `veilNotice` (devuelve `func(){}, false, nil` sin llamar) → el primero en rojo; quita el bloque `time.Since(veilStart) > baselineSlack` → `TestScreenshotLooksAtTheGuardAgainAfterASlowVeil` en rojo. Revierte todas.

- [ ] **Step 10: Commit.**

```bash
git add host
git commit -m "feat(host): screenshot retira el aviso durante la captura y avisa al agente si no puede

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Prueba de emulador, la captura sale limpia

Necesita T2 y T3 integrados, el APK debug construido y el emulador `adbuddy-p1-36` (lock `emu-lock.sh acquire seguridad`).

**Files:**
- Modify: `host/internal/emutest/notice_test.go`

**Interfaces:**
- Consumes: `daemon.ScreenshotResult`, `bridge.UIOpHideNotice`, `bridge.UIHideNoticeArgs`; helpers existentes `a11yLink`, `newAppGuard`, `appGuard.read`, `emu.waitOverlay`, `emu.overlayGone`, `emu.rawScreencap`, `edgeDiffers`, `link.uiCall`, `link.send`, `poll`.

- [ ] **Step 1: Registro de ops en `appGuard`.** Añade `sync` a los imports. En el tipo `appGuard` añade `mu sync.Mutex` y `ops []string`, y en `newAppGuard` cambia `UI: l.appUI(t)` por una envoltura que apunta las ops del aviso:

```go
	ui := l.appUI(t)
	...
		UI: func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
			if op == bridge.UIOpNotice || op == bridge.UIOpHideNotice {
				g.mu.Lock()
				g.ops = append(g.ops, op)
				g.mu.Unlock()
			}
			return ui(ctx, op, args)
		},
```

(`g` se declara antes de `daemon.New`, como hoy; mueve `g.d = daemon.New(...)` tal cual.) Añade:

```go
// noticeOps returns the notice and hideNotice ops the app got, in order, and forgets them.
func (g *appGuard) noticeOps() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	ops := g.ops
	g.ops = nil
	return ops
}
```

- [ ] **Step 2: Fracción de píxeles distintos** (de los spikes; úsala solo en este fichero):

```go
// diffFrac is the fraction of the pixels of region r (left, top, right,
// bottom) that differ between a and b.
func diffFrac(a, b image.Image, r [4]int) float64 {
	n, total := 0, 0
	for y := r[1]; y < r[3]; y++ {
		for x := r[0]; x < r[2]; x++ {
			total++
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb {
				n++
			}
		}
	}
	return float64(n) / float64(total)
}
```

- [ ] **Step 3: Tests nuevos.**

```go
// P1.2: the capture of adbuddy screenshot has neither the frame nor the pill,
// and both are back on the screen once it is done.
func TestTheScreenshotDoesNotCarryTheNotice(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	g := newAppGuard(t, l)

	// A first read: it waits out the notification that always arrives right
	// after the service is turned on (it would sit over the pill's region),
	// and it shows the overlay, so that the pill's place is known.
	if exit, _, _ := g.read(t, bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 {
		t.Fatalf("adbuddy screen: %+v", exit)
	}
	pill := e.waitOverlay(t)
	e.overlayGone(t, "before the plain capture")
	plain := e.rawScreencap(t)

	// The control of this test: a raw capture with the overlay up has the
	// pill. Without it, a clean screenshot would prove nothing.
	l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
	e.waitOverlay(t)
	if d := diffFrac(plain, e.rawScreencap(t), pill); d < 0.5 {
		t.Fatalf("a raw capture with the overlay up differs from the plain one in only %.3f of the pill's region: the control does not see the pill", d)
	}
	e.overlayGone(t, "after the control")
	g.noticeOps() // forget the ops so far

	start := time.Now()
	exit, data, img := g.read(t, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 1, Format: "png"})
	took := time.Since(start)
	back := e.overlayWindows(t) // right after the command, before the 4 s that the notice stays
	if exit.Code != 0 {
		t.Fatalf("adbuddy screenshot: %+v", exit)
	}
	var res daemon.ScreenshotResult
	if err := json.Unmarshal([]byte(data), &res); err != nil || res.NoticeInImage {
		t.Fatalf("result %s (%v): the notice must have been taken off", data, err)
	}
	// The retries of g.read for a notification that just arrived come before any notice op.
	if got, want := g.noticeOps(), []string{bridge.UIOpNotice, bridge.UIOpHideNotice, bridge.UIOpNotice}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops %v, want %v: the notice, then it comes off for the capture, then it is put back", got, want)
	}
	shot := decodePNG(t, img)
	if d := diffFrac(plain, shot, pill); d > 0.02 {
		t.Errorf("the pill is in the screenshot: %.3f of its region differs from the screen without the overlay", d)
	}
	if n := edgeDiffers(plain, shot); n != 0 {
		t.Errorf("the frame is in the screenshot: %d of 6 edge points differ", n)
	}
	if len(back) != 2 {
		t.Errorf("the frame and the pill are not back after the screenshot: %v", back)
	}
	t.Logf("adbuddy screenshot took %v with the notice taken off", took)
}

// The notice that was taken off comes back by itself, stays at least a moment
// before it goes, and does not come back for a paused session.
func TestTheHiddenNoticeComesBackByItself(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	e.overlayGone(t, "start")

	hide := func(ms int) time.Time {
		t.Helper()
		l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
		e.waitOverlay(t)
		if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: ms}); !r.OK {
			t.Fatalf("hideNotice: %+v", r.Error)
		}
		if w := e.overlayWindows(t); len(w) != 0 {
			t.Fatalf("hideNotice answered but the overlay is still up: %v", w)
		}
		return time.Now()
	}

	hide(1500)
	if !poll(5*time.Second, func() bool { return len(e.overlayWindows(t)) == 2 }) {
		t.Fatal("the overlay did not come back by itself 5 s after hideNotice 1500")
	}
	e.overlayGone(t, "after it came back")

	// The person sees the notice before it goes: it is up for 400 ms at least.
	l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
	t0 := time.Now()
	if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 1500}); !r.OK {
		t.Fatalf("hideNotice: %+v", r.Error)
	} else if dt := time.Since(t0); dt < 300*time.Millisecond {
		t.Errorf("hideNotice answered %v after the notice: the overlay was not up for the minimum", dt)
	}
	e.overlayGone(t, "after the dwell test")

	// Out of range: refused, and the overlay is untouched.
	for _, ms := range []int{0, bridge.MaxHideNoticeMs + 1} {
		if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: ms}); r.OK || r.Error == nil || r.Error.Code != bridge.CodeInvalid {
			t.Errorf("hideNotice ms=%d: %+v, want invalid", ms, r)
		}
	}

	// Paused while it was off: it does not come back. The longest time, so
	// the app learns of the pause well before the timer.
	hiddenAt := hide(bridge.MaxHideNoticeMs)
	l.send(t, bridge.TypeSessionState, bridge.SessionState{Agent: "emutest", State: "working", Paused: true})
	if !poll(3*time.Second, func() bool {
		r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 1500}) // the overlay is off: it only answers, it does not reschedule
		return !r.OK && r.Error != nil && r.Error.Code == bridge.CodeProtected
	}) {
		t.Fatal("the app does not take itself as paused 3 s after session.state paused")
	}
	time.Sleep(time.Until(hiddenAt.Add(bridge.MaxHideNoticeMs*time.Millisecond + time.Second)))
	if w := e.overlayWindows(t); len(w) != 0 {
		t.Errorf("the overlay came back for a paused session: %v", w)
	}
	// Nor does the heads-up for acting: while paused, the app refuses the op
	// before it shows anything (ruling of the leader, 2026-10-12).
	if r := l.uiCall(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "any"}); r.OK {
		t.Errorf("a click was accepted while paused: %+v", r)
	}
	if w := e.overlayWindows(t); len(w) != 0 {
		t.Errorf("a refused click in a paused session put the overlay up: %v", w)
	}
}
```

Si algún helper no coincide con el nombre real (`l.send`, `e.waitOverlay`), usa los que ya usa `TestTheNoticeWhenTheHostReadsTheScreen` en el mismo fichero. Añade los imports que falten.

- [ ] **Step 4: Invertir la aserción del test existente.** En `TestTheNoticeWhenTheHostReadsTheScreen`, sustituye el bloque desde el comentario «adbuddy screenshot: the notice goes up before screencap…» hasta el cierre del `if n := edgeDiffers…` por:

```go
	// adbuddy screenshot: the notice goes up, comes off the screen for the
	// capture (so the frame is not in the image: the other test checks it) and
	// is put back; the overlay may be gone again by now on a slow emulator.
	exit, data, img := g.read(t, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 1, Format: "png"})
	if exit.Code != 0 {
		t.Fatalf("adbuddy screenshot: %+v", exit)
	}
	t.Logf("screenshot %s: overlay windows after the command: %v", data, e.overlayWindows(t))
	if n := edgeDiffers(plain, decodePNG(t, img)); n != 0 {
		t.Errorf("the frame is in the screenshot: %d of 6 edge points differ", n)
	}
```

- [ ] **Step 5: Ejecutar en el emulador.** Con el lock, el emulador arrancado y el APK construido (`:app:assembleDebug`), `go test -count=1 -timeout 8m -v -run 'TestTheScreenshotDoesNotCarryTheNotice|TestTheHiddenNoticeComesBackByItself|TestTheNoticeWhenTheHostReadsTheScreen' ./internal/emutest`. Los tres en verde; guarda la salida y las latencias en el ledger. Corre también `TestADumpOfTheLockedScreen` y los tests de overlay existentes (`-run 'Overlay|Notice|Locked'`).

- [ ] **Step 6: Mutaciones** (ledger; una a una, reconstruyendo el APK si toca Kotlin): (a) en `screenshot` sustituye `d.veilNotice(ctx)` por `func() {}, false, nil` → `TestTheScreenshotDoesNotCarryTheNotice` en rojo (píldora y marco en la captura); (b) quita `main.postDelayed(restorer, ms)` en `Overlay.takeOff` → `TestTheHiddenNoticeComesBackByItself` en rojo; (c) quita `if (!isPaused())` del `restorer` → el caso de pausa en rojo; (d) pon `FRAMES_AFTER_HIDE = 0` → `TestTheScreenshotDoesNotCarryTheNotice` en rojo, o inestable: si no se pone rojo, apunta que 0 fotogramas no basta o no se distingue y no se promete lo contrario. (e) pon `MIN_SHOWN_MS = 0` → la comprobación del mínimo en `TestTheHiddenNoticeComesBackByItself` en rojo. Revierte todo y vuelve a pasar los tests.

- [ ] **Step 7: Commit.**

```bash
git add host/internal/emutest
git commit -m "test(emulador): la captura de screenshot sale sin marco ni píldora y el aviso vuelve solo

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Protocolo, `UiDump.unreadable` y `Dumper`

**Files:**
- Modify: `host/internal/bridge/ui_ops.go` (`UIDump`)
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ProtocolUiOps.kt` (`UiDump`)
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/Dumper.kt`
- Modify: `protocol/fixtures/ui.json`
- Create: `protocol/fixtures/app/ui.response@dumpunreadable.json`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/a11y/DumperTest.kt`

**Interfaces:**
- Produces: `bridge.UIDump.Unreadable bool` (`json:"unreadable,omitempty"`); Kotlin `UiDump.unreadable: Boolean? = null` (`true` o ausente: nunca se emite `false`).

- [ ] **Step 1: Tests primero.** En `DumperTest.kt` añade (imports: `assertNull` ya está; `UiWindow` y `Box` son del mismo paquete):

```kotlin
    private fun dumpOf(windows: List<UiWindow>) =
        Dumper.dump(windows, SCREEN, 0, "", "", false, false, { false })

    @Test fun anAppWindowWithoutContentMakesTheScreenUnreadable() {
        val d = dumpOf(listOf(UiWindow("system", 2, Box(0, 0, 1344, 199), null), UiWindow("app", 1, SCREEN, null)))
        assertEquals(true, d.unreadable)
        assertTrue(d.elements.isEmpty())
    }

    @Test fun aDialogWithoutContentMakesItUnreadableToo() {
        assertEquals(true, dumpOf(windows + UiWindow("dialog", 5, Box(100, 100, 900, 900), null)).unreadable)
    }

    @Test fun noWindowsAtAllIsUnreadable() {
        assertEquals(true, dumpOf(emptyList()).unreadable)
    }

    @Test fun aReadableScreenIsNotMarked() {
        assertNull(dump().unreadable)
        assertNull(dump(all = true).unreadable)
    }

    @Test fun aSystemWindowWithoutContentAloneIsNotUnreadable() {
        assertNull(dumpOf(windows + UiWindow("system", 9, Box(0, 0, 1344, 199), null)).unreadable)
    }
```

(`windows` es `ScreenFixtures.windows`, ya importado.) El 3.er test usa una lista vacía: la pantalla bloqueada ya vuelve antes (`locked`).

- [ ] **Step 2: Rojo.** No compila (`unreadable`).

- [ ] **Step 3: Kotlin.** En `ProtocolUiOps.kt`, `UiDump`, añade tras `elements`:

```kotlin
    /** true when an app window gave no content (Android or the app hides it from this service) or there is no window at all; absent otherwise. */
    val unreadable: Boolean? = null,
```

con el KDoc de la clase ya existente. En `Dumper.dump`, antes de `val d = UiDump(...)` del camino no bloqueado:

```kotlin
        // A window without content was skipped above: say so, so the agent knows
        // the empty screen is not an empty app.
        val unreadable = windows.isEmpty() || windows.any { (it.type == "app" || it.type == "dialog") && it.root == null }
```

y `UiDump(locked = false, ..., elements = elements, unreadable = true.takeIf { unreadable })`. Actualiza el KDoc de la clase `Dumper` con «and says when an app window gave no content (unreadable)».

- [ ] **Step 4: Go.** En `UIDump` añade tras `Elements`:

```go
	// Unreadable: an app window gave the app no content (Android or the app
	// hides it from accessibility services) or there is no window at all.
	Unreadable bool `json:"unreadable,omitempty"`
```

y ajusta el comentario del tipo («… Unreadable says the screen could not be read.»).

- [ ] **Step 5: Fixtures.** En `ui.json`, `responses`, añade `"ui.response@dumpunreadable": "dump"`. Crea `protocol/fixtures/app/ui.response@dumpunreadable.json`:

```json
{
  "v": 1,
  "type": "ui.response",
  "ts": "2026-10-01T10:00:00Z",
  "payload": {
    "id": "u_9",
    "ok": true,
    "result": {
      "locked": false,
      "package": "",
      "activity": "",
      "width": 1344,
      "height": 2992,
      "rotation": 0,
      "truncated": false,
      "elements": [],
      "unreadable": true
    }
  }
}
```

- [ ] **Step 6: Verde.** `cd host && go vet ./internal/bridge/ && go test -timeout 5m ./internal/bridge/ ./internal/daemon/ ./internal/cli/` y `gradle --offline --no-daemon :app:testDebugUnitTest lintDebug`. `TestUIOpsHaveEveryField` exige un fixture con `unreadable`: lo da el nuevo.

- [ ] **Step 7: Mutaciones** (ledger): en `Dumper`, quita `|| windows.any { ... }` → `anAppWindowWithoutContentMakesTheScreenUnreadable` en rojo; cambia `true.takeIf { unreadable }` por `unreadable` (emite `false`) → los round-trips de fixtures (`UiOpsFixturesTest`) y `aReadableScreenIsNotMarked` en rojo; revierte.

- [ ] **Step 8: Commit** (Go, Kotlin y fixtures juntos):

```bash
git add host/internal/bridge protocol/fixtures android/app/src
git commit -m "feat(protocolo): UiDump.unreadable avisa de que una ventana no entrega contenido

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Host, nota de pantalla ilegible en `adbuddy screen`

**Files:**
- Modify: `host/internal/cli/helpers.go` (`printScreen`, `unreadableScreen`)
- Modify: `host/internal/cli/helpers_test.go`
- Modify: `host/internal/i18n/catalog/en.toml`, `es.toml` (`help.screen.long`)

**Interfaces:**
- Consumes (T5): `bridge.UIDump.Unreadable`.

- [ ] **Step 1: Tests primero.** En `helpers_test.go`:

```go
// An app that hides its content from accessibility services: the agent is told
// what is known, what it can do, and nothing about getting round it.
func TestScreenSaysWhenItCannotReadIt(t *testing.T) {
	helperDaemon(t, func(req ipc.Request, c *ipc.Conn) ipc.Exit {
		_ = c.Data(bridge.UIDump{Width: 1344, Height: 2992, Unreadable: true, Elements: []bridge.UIElement{}})
		return ipc.Exit{}
	})
	out, _, code := runHelper(t, "screen")
	lines := strings.Split(out, "\n")
	if code != 0 || lines[0] != "unknown app · 1344×2992" || !strings.Contains(lines[1], "cannot read some or all of this screen") {
		t.Fatalf("exit %d %q", code, out)
	}
	for _, want := range []string{"hides its content from accessibility services", "adbuddy screenshot", "ask the user to do this part"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, never := range []string{"uiautomator", "enable", "disable", "bypass"} {
		if strings.Contains(strings.ToLower(out), never) {
			t.Errorf("the note mentions %q:\n%s", never, out)
		}
	}
	out, _, code = runHelper(t, "screen", "--json")
	if code != 0 || !strings.Contains(out, `"unreadable":true`) {
		t.Fatalf("json: %d %q", code, out)
	}
}

func TestAReadableScreenHasNoUnreadableNote(t *testing.T) {
	helperDaemon(t, func(req ipc.Request, c *ipc.Conn) ipc.Exit {
		_ = c.Data(bridge.UIDump{Package: "com.android.settings", Width: 1, Height: 2, Elements: []bridge.UIElement{}})
		return ipc.Exit{}
	})
	out, _, code := runHelper(t, "screen")
	if code != 0 || strings.Contains(out, "cannot read") {
		t.Fatalf("exit %d %q", code, out)
	}
	if out, _, _ = runHelper(t, "screen", "--json"); strings.Contains(out, "unreadable") {
		t.Fatalf("json: %q", out)
	}
}
```

- [ ] **Step 2: Rojo.** `go test ./internal/cli -run 'CannotReadIt|NoUnreadableNote'`: el primero falla.

- [ ] **Step 3: Código.** En `helpers.go`:

```go
// unreadableScreen is what the agent reads when the app could not read part or
// all of the screen: Android or the app hides its content from accessibility
// services (or the screen is changing). It says what is known and what the
// agent can do, never how to get round it. Fixed English, for the agent.
const unreadableScreen = "(ADBuddy cannot read some or all of this screen: Android or the app hides its content from accessibility services, or the screen is still changing, so try once more. `adbuddy screenshot` may still show it, unless the app blocks screenshots. `adbuddy tap` refuses to touch what ADBuddy cannot read: ask the user to do this part.)"
```

En `printScreen`, justo tras `fmt.Fprintf(w, "%s · %d×%d\n", head, d.Width, d.Height)`:

```go
	if d.Unreadable {
		fmt.Fprintln(w, unreadableScreen)
	}
```

Catálogos: añade al final de `help.screen.long` EN: `"If the app or Android hides the screen from ADBuddy, it says so; adbuddy screenshot may still show it."`; ES: `"Si la app o Android ocultan la pantalla a ADBuddy, lo dice; adbuddy screenshot puede aún enseñarla."`, como frase nueva en el bloque.

- [ ] **Step 4: Verde.** `cd host && go vet ./... && go test -timeout 20m ./internal/cli/ ./internal/i18n/ ./internal/daemon/` (los de `textlint`/catálogo incluidos).

- [ ] **Step 5: Mutación** (ledger): quita `if d.Unreadable { ... }` → `TestScreenSaysWhenItCannotReadIt` en rojo; revierte.

- [ ] **Step 6: Commit.**

```bash
git add host
git commit -m "feat(host): adbuddy screen explica al agente que la pantalla esta oculta a ADBuddy

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Diagnóstico para la causa en el Pixel

Paralelizable con T6 (`android/` frente a `host/`), pero tras T5 (mismo `Dumper.kt`).

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/Dumper.kt` (`describe`)
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/AgentAccessibilityService.kt` (`windows()`)
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/a11y/DumperTest.kt`

**Interfaces:**
- Produces: `Dumper.describe(windows: List<UiWindow>): String`.

- [ ] **Step 1: Test primero.** En `DumperTest.kt`:

```kotlin
    @Test fun describeSaysTypesAndNothingElse() {
        val secret = UiNode(pkg = "com.example.bank", cls = "android.widget.TextView", text = "1234 5678", bounds = SCREEN)
        val s = Dumper.describe(listOf(UiWindow("system", 0, Box(0, 0, 1344, 199), null), UiWindow("app", 1, SCREEN, secret)))
        assertEquals("2 windows: app@1:content, system@0:no content", s)
        assertFalse(s.contains("bank") || s.contains("1234"))
        assertEquals("0 windows: ", Dumper.describe(emptyList()))
    }
```



- [ ] **Step 2: Rojo.** No compila (`describe`).

- [ ] **Step 3: `Dumper.describe`:**

```kotlin
    /**
     * One line about the windows for the log: type, layer and whether the
     * window gave content. Never titles, packages or text.
     */
    fun describe(windows: List<UiWindow>): String =
        windows.sortedByDescending { it.layer }
            .joinToString(prefix = "${windows.size} windows: ", separator = ", ") { "${it.type}@${it.layer}:${if (it.root == null) "no content" else "content"}" }
```

- [ ] **Step 4: Servicio.** En `AgentAccessibilityService`, añade `@Volatile private var lastWindowsLog = ""`; renombra el `return windows.map { w -> ... }` de `windows()` a `val list = windows.map { w -> ... }` y termina la función con:

```kotlin
        // For a phone where the system does not hand over the content (Android 17 on a Pixel, plan 2026-10-12-p1-pantalla-oculta): once per change, no content in the line.
        if (list.isEmpty() || list.any { it.root == null }) {
            Dumper.describe(list).takeIf { it != lastWindowsLog }?.let {
                lastWindowsLog = it
                Log.i(TAG, it)
            }
        } else {
            lastWindowsLog = "" // readable again: the next unreadable state is logged again
        }
        return list
```

- [ ] **Step 5: Verde.** `gradle --offline --no-daemon :app:testDebugUnitTest lintDebug`.

- [ ] **Step 6: Mutación** (ledger): `describe` con `it.pkg` en la cadena → el test (`contains("bank")`) en rojo; revierte.

- [ ] **Step 7: Commit.**

```bash
git add android/app/src
git commit -m "feat(app): linea de log sin contenido con las ventanas que no entregan contenido

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Documentos

**Files:**
- Modify: `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`
- Modify: `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`
- Modify: `docs/superpowers/specs/2026-10-01-adbuddy-protocolo-m3-m5-design.md` (lista «Operaciones de partida»: añadir `hideNotice`, y el campo `unreadable` de `dump`)
- Create: `docs/superpowers/fragmentos/fix-p1-pantalla-oculta.md`
- Modify: este plan (sección «Validaciones manuales pendientes» al final: marcar estados)
- Modify: `docs/safety.md`, `docs/usage.md` (visto bueno del líder: `noticeInImage` documentado para el agente)

- [ ] **Step 0: `docs/safety.md` y `docs/usage.md`.** En `docs/safety.md`, al final del párrafo que empieza «While the agent reads the screen», añade (inglés): «`adbuddy screenshot` takes the frame and the pill off the screen for a moment while it captures, so they do not cover the image, and puts them back right after; the notice is up for at least 400 ms before that. If it cannot take them off, the picture still comes out and says so: `noticeInImage` in `--json`, or a note on stderr.» En `docs/usage.md`, en la viñeta de `adbuddy screenshot` («saves a picture, scaled to half by default»), añade: «If ADBuddy's on-screen notice could not be taken off for the capture, `--json` has `"noticeInImage": true` and a note is printed on stderr: ignore the thin frame and the pill near the top.» Y, en `docs/safety.md` junto al párrafo de `adbuddy screen` (línea que empieza «The agent can read and act on the screen»), una frase: «When ADBuddy cannot read a screen (Android or the app hides its content from accessibility services), `adbuddy screen` says so (`unreadable` in `--json`); `adbuddy screenshot` may still show it. The cause is not known in every case, so the note does not name one.»

- [ ] **Step 1: M5.** En §6, «Cuándo» (línea que empieza «**Cuándo:**»), añade al final: «En `adbuddy screenshot` el aviso lleva en pantalla 400 ms como mínimo antes de retirarse y se retira unos 0,3 s durante la propia captura (`hideNotice`, con restauración automática a los 2 s como mucho y nunca en pausa) para que el marco y la píldora no tapen la imagen; vuelve justo después. Las lecturas en bruto del agente no lo retiran.» En §3.6 (`screenshot`, «La hace el host…») añade: «Entre el aviso y `screencap` pide `hideNotice`; si no puede retirarlo, la captura sale con el aviso dentro y `ScreenshotResult.noticeInImage` lo dice.» Tras la descripción de `notice` (línea «Pide a la app que muestre la superposición…») añade un párrafo para `hideNotice`: `args {ms: 1..5000}`, `result {}`; la app quita las vistas, espera 3 fotogramas y se restaura a los `ms` salvo pausa; `PROTECTED` en pausa; `FAILED` si no puede. En §3.1 (`dump`) añade `unreadable` al resultado. En §12 añade dos ítems tras el 5: «6. La op `hideNotice` de `ui.request` (p1-pantalla-oculta): `args {ms}`, `result` vacío; `MaxHideNoticeMs` 5000. … Aditivo; un commit con Go, Kotlin y fixtures.» y «7. El campo `unreadable` de `UiDump` (p1-pantalla-oculta): `bool` con `omitempty` en Go, `Boolean?` sin emitir `false` en Kotlin; fixture `app/ui.response@dumpunreadable.json`. Aditivo.»

- [ ] **Step 2: v1 §5.7.** Añade al final de la sección: «El aviso se retira un instante mientras `adbuddy screenshot` captura, para no ensuciar la imagen del agente, y vuelve de inmediato (spec `2026-10-12-p1-pantalla-oculta-design.md`).»

- [ ] **Step 3: Fragmento** `docs/superpowers/fragmentos/fix-p1-pantalla-oculta.md` con: todos los rulings del ledger (R1-R13) (formato `qué — por qué — qué cuesta`), las filas para `pendientes-m3-m5.md` (lo que queda: causa de la pantalla oculta en Android 17; decisión FLAG_SECURE; permiso de notificaciones del Pixel) y la tabla de «Validaciones manuales pendientes» de abajo, sin editar los documentos comunes.

- [ ] **Step 4: Validaciones manuales pendientes.** Copia la tabla de abajo al fragmento y deja los estados en este plan.

- [ ] **Step 5: Comprobar y commit.** `git grep -nE '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|C:[/\\]Users' -- <ficheros añadidos>` sin resultados nuevos, y `git diff main... | grep '^+' | grep -E '[A-Z0-9]{12,}'` sin ningún número de serie; `git add docs`; commit `docs: ...` con el trailer.

## Validaciones manuales pendientes

| Validación | Qué comprobar | Estado |
|---|---|---|
| Causa de la pantalla oculta en Opciones para desarrolladores (Pixel, Android 17) | Con el APK de esta rama: abrir Opciones para desarrolladores, `adbuddy screen` (debe imprimir la nota de pantalla ilegible) y `adb logcat -d -s ADBuddyA11y` (línea `N windows: …@…:no content`). Distingue «el sistema no entrega la ventana» de «la entrega sin contenido». | PENDIENTE: necesita el móvil real |
| El velo en Android 17 | `adbuddy screenshot` en el Pixel con el aviso visible: la captura sin marco ni píldora y el aviso de vuelta tras el comando; latencia razonable. Medido solo en API 36. | PENDIENTE: Android 17 no corre en emulador |
| FLAG_SECURE negro | Una app con `FLAG_SECURE` en primer plano (no protegida): `adbuddy screenshot` sale negra; `adbuddy screen` la lee con normalidad (FLAG_SECURE no oculta el contenido a accesibilidad), sin nota de pantalla ilegible. | PENDIENTE |
| Velo en API 36 (emulador) | `TestTheScreenshotDoesNotCarryTheNotice`, `TestTheHiddenNoticeComesBackByItself` | A rellenar con la ejecución de T4 |
