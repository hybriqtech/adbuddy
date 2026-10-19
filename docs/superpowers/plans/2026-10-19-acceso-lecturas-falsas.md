# Acceso: lecturas falsas de `adb_enabled`. Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que cortar y reactivar funcionen en Android 17, donde la app lee `adb_enabled=0` aunque esté a 1: no guardar ni restaurar ese 0 falso, confirmar el corte y la reactivación con lo que sí se puede leer, y que el registro `ADBuddyAccess` diga cuándo una lectura se contradice.

**Architecture:** la regla vive en `control/AccessSwitch.kt` (pura, tests JVM). Una función `adbZeroIsDoubtful(adb, wifi, channelUp)` decide cuándo un 0 no se cree, y la usan el guardado de `cut()` y la relectura de `write()`. El canal vivo llega desde los dos botones de cortar a través de `CutAccess.cut(context, channelUp)`. `AccessDiag` añade la marca de contradicción a su línea.

**Tech Stack:** Kotlin, Robolectric (JVM), Gradle 9.8 sin wrapper (como la CI).

**Spec:** `docs/superpowers/specs/2026-10-19-acceso-lecturas-falsas.md` (adenda, R16-R21), subordinada a `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md` (R1-R5 y §8 siguen en vigor).

## Global Constraints

- Ningún test de §8 de la spec del 2026-10-18 se borra ni se debilita: `cuttingTurnsBothOff`, `aWriteThatDidNotStickIsNotDone`, `withoutThePermissionNothingIsCut`, `restoringWithNothingSavedTurnsOnlyAdbOn` y los de UI. El único test existente que cambia es `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere` (tarea 1, ruling R16), que no está en §8. Si otro test existente falla, la tarea se para y lo adjudica el controlador.
- `cut()` nunca escribe un 1. El canal (`channelUp`) no entra nunca en `isCut()`, `readAsOn()` ni `forgetIfRestored()` (R20).
- Un 1 leído se cree siempre; solo un 0 se pone en duda, y solo con contradicción (R17). Nada se decide por `Build.VERSION.SDK_INT`.
- El registro solo lleva números y palabras fijas (sin nombre del PC ni texto del host). Los literales de log de `CutAccess.kt` que ya están en `android/app/src/test/resources/i18n-exceptions.txt` (`cut.after ok=`, `restore.after ok=`) no cambian de texto: `NoHardcodedTextTest.everyExceptionStillMatchesSomething` falla si dejan de existir.
- Cada test crítico lleva su **mutación**: el implementador la aplica a mano, ve fallar el test indicado, la deshace y lo dice en su informe.
- Nunca se ejecuta nada en el móvil del usuario. Ni tareas `connected*` de Gradle, ni `adb` contra el Pixel.
- Git: identidad local hybriqtech del repo, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, sin push.

## Comandos (los reales: `.github/workflows/android.yml` y el plan del 2026-10-18)

Desde `C:\Users\Jose\orca\workspaces\adbuddy\ux\android`, en Git Bash. No hay wrapper ni `local.properties` en el worktree (está ignorado), así que el SDK va por variable:

```bash
export ANDROID_HOME='C:/Users/Jose/AppData/Local/Android/Sdk'
G=/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle
```

- Tests concretos: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest'` (la primera ejecución en frío tarda unos 5 min; después, unos 20 s).
- Resultados: `app/build/test-results/testDebugUnitTest/TEST-<clase>.xml` (`failures=`, y `<failure` con el test). Con `-q`, Gradle no lista los tests que fallan: mirar el XML.
- Todo lo de la CI (tarea 4): `$G --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (en segundo plano, timeout 20 min).

## Revisores

| Tareas | Revisor | Por qué |
|---|---|---|
| 1, 2 | `adbuddy-reviewer-opus` | cortar y reactivar el acceso (seguridad) |
| 3, 4 | `adbuddy-reviewer` | diagnóstico y cierre |

Tiempo para los revisores: solo `--tests` concretos, máximo 15 min de ejecución por revisión.

## Mapa de ficheros

Rutas de Kotlin relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/`, y las de tests a `android/app/src/test/kotlin/tech/hybriq/adbuddy/`.

| Fichero | Tarea | Qué |
|---|---|---|
| `control/AccessSwitch.kt` | 1, 2 | `adbZeroIsDoubtful`; guardado y relectura con el 0 en duda; parámetro `channelUp` |
| `control/AccessSwitchTest.kt` | 1, 2 | `FakeSettings.readsZero` (Android 17) y tests nuevos |
| `control/CutAccess.kt` | 2, 3 | `channelUp(state)`, `cut(context, channelUp)`; el canal en el registro |
| `ui/ChatRoot.kt`, `bridge/BridgeService.kt` | 2 | los dos botones de cortar pasan el canal |
| `control/CutAccessTest.kt` | 2 | `channelUp` y cableado |
| `control/AccessDiag.kt`, `control/AccessDiagTest.kt` | 3 | `channel=` y `doubtful=adb_enabled(…)` |
| `android/README.md`, `docs/superpowers/plans/pendientes-m3-m5.md`, `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 4 | cierre |

---

### Task 1: Un 0 de `adb_enabled` que la inalámbrica contradice no se cree — crítica, revisor opus

R16, R17, R18 y R19 sin el canal: el canal llega en la tarea 2. Un 0 de `adb_enabled` con `adb_wifi_enabled=1` se guarda como 1. Al restaurar, escribir 1 y releer ese 0 en duda cuenta como hecho. Un 1 releído tras escribir 0 sigue siendo un fallo.

**Files:**
- Modify: `control/AccessSwitch.kt`
- Test: `control/AccessSwitchTest.kt`

**Interfaces:**
- Produces: `AccessSwitch.adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false): Boolean` (companion, pura).

- [ ] **Step 1: tests que fallan.** En `AccessSwitchTest.kt`:

(a) En `FakeSettings`, añadir el modo Android 17 (lee 0 sea cual sea el valor). Queda así:

```kotlin
        var unreadable = emptySet<String>() // reading throws, as Android 12+ may do for a hidden setting
        var readsZero = emptySet<String>() // reads 0 whatever the value, as Android 17 does for adb_enabled (addendum 2026-10-19)
        override fun get(name: String): Int? {
            if (name in unreadable) throw SecurityException("cannot read $name")
            if (name in readsZero) return 0
            return values[name]
        }
```

(b) Sustituir **entero** `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere` por este test (ruling R16):

```kotlin
    // Addendum 2026-10-19 R16: a 0 for adb_enabled with wireless debugging at 1 is not believed. Before, this test saved
    // adb_enabled=0 and restored it as 0; on Android 17 that 0 is false and restoring it would turn USB debugging off.
    @Test fun cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals("a 0 that wireless debugging contradicts is saved as 1", 1, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertTrue(sw.isCut())
        assertTrue(sw.restore())
        assertEquals(1, s.values[adb])
        assertEquals("wireless debugging comes back", 1, s.values[wifi])
        assertFalse(sw.isCut())
    }
```

(c) Añadir al final de la clase:

```kotlin
    // --- Addendum 2026-10-19: Android 17 reads adb_enabled=0 for apps; adb_wifi_enabled reads right. ---

    @Test fun android17_cuttingSavesUsbAsOnAndCuts() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals("the false 0 is not saved", 1, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertEquals(0, s.values[adb])
        assertEquals(0, s.values[wifi])
        assertTrue(sw.isCut())
    }

    @Test fun android17_restoringBringsBothBackAndIsDone() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertTrue("adb_enabled reads 0 after writing 1, but wireless reads 1: the 0 is the doubtful one", sw.restore())
        assertEquals(1, s.values[adb])
        assertEquals(1, s.values[wifi])
        assertTrue("forgotten once restored", saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun aWirelessThatDoesNotComeBackFailsTheRestore() {
        for (android17 in listOf(false, true)) {
            val s = FakeSettings(adb to 1, wifi to 1).apply { if (android17) readsZero = setOf(adb) }
            val saved = FakeSaved()
            val sw = AccessSwitch(s, saved)
            assertTrue(sw.cut())
            s.ignoreWrites = setOf(wifi)
            assertFalse("android17=$android17", sw.restore())
            assertEquals("the record stays", 1, saved.values[adb])
            assertEquals("the record stays", 1, saved.values[wifi])
            assertEquals("still cut only while nothing reads 1 (USB came back where it reads right)", android17, sw.isCut())
        }
    }

    @Test fun aZeroWithNothingAgainstItIsBelieved() {
        val s = FakeSettings(adb to 0, wifi to 0)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals(0, saved.values[adb])
        assertEquals(0, saved.values[wifi])
        assertTrue("R4: a record of both off is not guessed", sw.needsManualRestore())
    }

    @Test fun withoutAContradictionAZeroAfterRestoringIsBelieved() {
        // Known cost (addendum R17 b): USB only on Android 17. Writing 1 and reading 0 with wireless at 0 is not done.
        val s = FakeSettings(adb to 0, wifi to 0).apply { readsZero = setOf(adb) }
        val saved = FakeSaved().apply { values[adb] = 1; values[wifi] = 0 }
        val sw = AccessSwitch(s, saved)
        assertFalse(sw.restore())
        assertEquals("the record stays", 1, saved.values[adb])
    }

    @Test fun onlyAContradictedZeroIsDoubted() {
        assertTrue(AccessSwitch.adbZeroIsDoubtful(0, 1))
        assertTrue(AccessSwitch.adbZeroIsDoubtful(0, 0, channelUp = true))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(0, 0))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(0, null))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(1, 1, channelUp = true))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(null, 1, channelUp = true))
    }
```

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest'`
Expected: FAIL de compilación (`adbZeroIsDoubtful` no existe). Para ver el fallo de comportamiento, comentar temporalmente `onlyAContradictedZeroIsDoubted`: fallan `cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth`, `android17_cuttingSavesUsbAsOnAndCuts` y `android17_restoringBringsBothBackAndIsDone`. Descomentarlo después. Guardar las dos salidas en el informe.

- [ ] **Step 3: implementación mínima** en `control/AccessSwitch.kt`:

(a) Al final del KDoc de la clase (tras "An unreadable one proves nothing."), añadir:

```kotlin
 *
 * False readings (addendum 2026-10-19, R16-R19): Android 17 gives apps adb_enabled=0 while `adb shell` says 1, and
 * reads adb_wifi_enabled right. A 1 read is always believed (the false readings are 0); a 0 is doubted only when the
 * rest contradicts it (adbZeroIsDoubtful). Then cut() saves adb_enabled=1, and restore() takes a doubtful 0 after
 * writing 1 as done. So on Android 17 cut() is confirmed by wireless debugging reading 0 and adb_enabled not reading
 * 1: that adb_enabled=0 really applied is not proven (V2 of the addendum measures it from the shell).
```

(b) En `cut()`, el KDoc y el bloque de guardado quedan así (el resto de `cut()` no cambia):

```kotlin
    /** true once wireless debugging reads back 0 and adb_enabled does not read back 1 (R18); false without the permission, if a change did not stick or cannot be read back. */
    fun cut(): Boolean {
        forgetIfRestored() // a record that a readable setting contradicts is stale: do not keep it
        // Cutting again keeps the values of the first cut.
        val savedNow = !hasRecord()
        if (savedNow) {
            val adb = read(ADB_ENABLED, missing = 1)
            val wifi = read(ADB_WIFI_ENABLED, missing = 0)
            // A 0 that wireless debugging contradicts is Android 17's false reading, not the setting (R16).
            saved.put(ADB_ENABLED, if (adb == null || adbZeroIsDoubtful(adb, wifi)) 1 else adb)
            saved.put(ADB_WIFI_ENABLED, wifi ?: 0)
        }
```

(c) Sustituir `write` y el `companion object` por:

```kotlin
    private fun write(vararg changes: Pair<String, Int>): Boolean = try {
        for ((name, value) in changes) settings.put(name, value)
        changes.all { (name, value) -> readsBackAs(name, value) }
    } catch (e: SecurityException) {
        false
    }

    /**
     * Whether [name] reads back as [value] after writing it (never written = 0; unreadable never passes, R2).
     * adb_enabled=1 also passes with a doubtful 0 (R19): Android 17's false 0 cannot say the write did not stick.
     * adb_enabled=0 still fails with a 1: a 1 read is never false (R18).
     */
    private fun readsBackAs(name: String, value: Int): Boolean {
        val now = read(name, missing = 0) ?: return false
        if (now == value) return true
        return name == ADB_ENABLED && value == 1 && adbZeroIsDoubtful(now, read(ADB_WIFI_ENABLED, missing = 0))
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden

        /**
         * A 0 read for adb_enabled that the rest contradicts (addendum 2026-10-19, R16-R17): wireless debugging reads
         * 1, or a channel with the PC is alive (it goes through adbd). Detected by the contradiction, never by the
         * Android version. A 1 is never doubted, and a 0 with nothing against it is believed.
         */
        fun adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false): Boolean = adb == 0 && (wifi == 1 || channelUp)
    }
```

`restore()`, `isCut()`, `readAsOn()`, `forgetIfRestored()` y `needsManualRestore()` no cambian: `restore()` ya pasa por `write()`.

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest'`
Expected: PASS (`AccessSwitchTest` 27 tests, `CutAccessTest` 1, `ChatRootCutWiringTest` 4).

Mutaciones (aplicar una a una, ver el fallo en el XML y deshacer):
- M1. En `cut()`, guardar `adb ?: 1` (sin la duda) → fallan `cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth`, `android17_cuttingSavesUsbAsOnAndCuts`, `android17_restoringBringsBothBackAndIsDone` y `aWirelessThatDoesNotComeBackFailsTheRestore`.
- M2. En `readsBackAs`, `return false` en vez de la línea con `adbZeroIsDoubtful` → falla `android17_restoringBringsBothBackAndIsDone`.
- M3. En `write`, comprobar solo `adb_enabled` (`changes.filter { it.first == ADB_ENABLED }.all { … }`) → fallan `aWirelessThatDoesNotComeBackFailsTheRestore`, `aFailedCutLeavesNoRecord` y `aReadBackThatCannotBeReadIsNotDone`.
- M4. `adbZeroIsDoubtful = adb == 0` (todo 0 en duda) → fallan `aZeroWithNothingAgainstItIsBelieved`, `withoutAContradictionAZeroAfterRestoringIsBelieved` y `onlyAContradictedZeroIsDoubted`.
- M5. En `readsBackAs`, aceptar cualquier valor de `adb_enabled` (`if (name == ADB_ENABLED) return true` al principio) → falla `aWriteThatDidNotStickIsNotDone` (§8.8).

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control/AccessSwitch.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/control/AccessSwitchTest.kt
git commit -m "fix(android): un 0 de adb_enabled que la inalámbrica contradice no se guarda ni impide reactivar

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Un canal vivo con el PC también contradice el 0 — crítica, revisor opus

R16 y R20: los dos botones de cortar (el escudo de la pantalla y "Cortar" de la notificación) pasan si el canal está vivo. El canal solo cambia lo que se guarda y la relectura; nunca deshace un corte.

**Files:**
- Modify: `control/AccessSwitch.kt`, `control/CutAccess.kt`, `ui/ChatRoot.kt:136`, `bridge/BridgeService.kt:161`
- Test: `control/AccessSwitchTest.kt`, `control/CutAccessTest.kt`

**Interfaces:**
- Consumes: `AccessSwitch.adbZeroIsDoubtful` (tarea 1).
- Produces: `AccessSwitch(settings, saved, channelUp: Boolean = false)`, `CutAccess.channelUp(state: BridgeState): Boolean`, `CutAccess.cut(context: Context, channelUp: Boolean = false): Boolean` (sin el argumento, como antes: `CutAccessOnEmulatorTest` compila sin cambios).

- [ ] **Step 1: tests que fallan.**

(a) Al final de `AccessSwitchTest`:

```kotlin
    // --- Addendum 2026-10-19 R16/R20: a live channel with the PC goes through adbd. ---

    @Test fun aLiveChannelSavesUsbAsOnEvenIfItReadsOff() {
        val s = FakeSettings(adb to 1, wifi to 0).apply { readsZero = setOf(adb) } // USB only, Android 17, channel alive
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals(1, saved.values[adb])
        assertEquals("wireless debugging is saved as read", 0, saved.values[wifi])
    }

    @Test fun aLiveChannelLetsADoubtfulZeroPassTheRestore() {
        val s = FakeSettings(adb to 0, wifi to 0).apply { readsZero = setOf(adb) }
        val saved = FakeSaved().apply { values[adb] = 1; values[wifi] = 0 }
        assertTrue(AccessSwitch(s, saved, channelUp = true).restore())
        assertEquals(1, s.values[adb])
        assertTrue(saved.values.isEmpty())
    }

    @Test fun aLiveChannelNeverUndoesACut() {
        // In the second the tunnel takes to fall after a cut the channel still says CONNECTED: the record stays.
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved, channelUp = true)
        assertTrue(sw.cut())
        sw.forgetIfRestored()
        assertTrue(sw.isCut())
        assertEquals(1, saved.values[wifi])
    }
```

(b) `CutAccessTest.kt` entero (conserva el test existente tal cual):

```kotlin
package tech.hybriq.adbuddy.control

import android.content.Context
import android.provider.Settings
import java.io.File
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.SessionState

@RunWith(RobolectricTestRunner::class)
class CutAccessTest {
    private val app get() = RuntimeEnvironment.getApplication()

    @Test fun onlyWirelessOnAndNoRecordIsNotCut_untilACutOfOurs() {
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_ENABLED, 0)
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_WIFI_ENABLED, 1)
        assertFalse(CutAccess.isCut(app))
        assertFalse("the first emission too", runBlocking { CutAccess.observe(app).first() })
        assertTrue(CutAccess.cut(app))
        assertTrue(CutAccess.isCut(app))
        assertTrue(runBlocking { CutAccess.observe(app).first() })
    }

    // Addendum 2026-10-19 R20: only a host that welcomed the app (paused included) is a live channel.
    @Test fun theChannelIsUpOnlyWhenTheHostIsConnected() {
        assertTrue(CutAccess.channelUp(BridgeState(connection = BridgeState.Connection.CONNECTED)))
        assertTrue(
            "paused is still a live channel",
            CutAccess.channelUp(BridgeState(connection = BridgeState.Connection.CONNECTED, sessionState = SessionState(paused = true))),
        )
        for (c in listOf(BridgeState.Connection.WAITING, BridgeState.Connection.CONNECTING, BridgeState.Connection.INCOMPATIBLE)) {
            assertFalse(c.name, CutAccess.channelUp(BridgeState(connection = c)))
        }
    }

    @Test fun aLiveChannelSavesUsbAsOn() {
        val prefs = app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE)
        prefs.edit().clear().commit()
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_ENABLED, 0)
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_WIFI_ENABLED, 0)
        assertTrue(CutAccess.cut(app, channelUp = true))
        assertEquals(1, prefs.getInt(AccessSwitch.ADB_ENABLED, -1))
        assertEquals(0, prefs.getInt(AccessSwitch.ADB_WIFI_ENABLED, -1))
    }

    // The shield of the screen and Cut in the notification (spec 2026-10-18 R5, §8.7) tell cut() whether the channel is alive.
    @Test fun bothCutButtonsPassTheChannel() {
        val withChannel = Regex("""CutAccess\.cut\(\s*(context|this)\s*,\s*CutAccess\.channelUp\(app\.bridge\.state\.value\)\s*\)""")
        val withoutChannel = Regex("""CutAccess\.cut\(\s*(context|this)\s*\)""")
        for (path in listOf("ui/ChatRoot.kt", "bridge/BridgeService.kt")) {
            val src = File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText()
            assertTrue(path, withChannel.containsMatchIn(src))
            assertFalse("$path cuts without the channel", withoutChannel.containsMatchIn(src))
        }
    }
}
```

`bothCutButtonsPassTheChannel` mira el código fuente, como `TouchGuardSourceTest`: el estado `CONNECTED` del bridge no se puede provocar en Robolectric sin un servidor.

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest'`
Expected: FAIL de compilación (no existen ni el parámetro `channelUp` de `AccessSwitch` ni `CutAccess.channelUp`).

- [ ] **Step 3: implementación mínima.**

(a) `control/AccessSwitch.kt`:
- Cabecera: `class AccessSwitch(private val settings: Settings, private val saved: Saved, private val channelUp: Boolean = false) {`
- En `cut()`: el comentario pasa a `// A 0 that wireless debugging or a live channel contradicts is Android 17's false reading, not the setting (R16).` y la llamada a `adbZeroIsDoubtful(adb, wifi, channelUp)`.
- En `readsBackAs`: `adbZeroIsDoubtful(now, read(ADB_WIFI_ENABLED, missing = 0), channelUp)`.
- `readAsOn()`, `isCut()` y `forgetIfRestored()` **no** usan `channelUp` (R20).

(b) `control/CutAccess.kt`:
- Import: `import tech.hybriq.adbuddy.bridge.BridgeState`.
- `private fun switch(context: Context, channelUp: Boolean = false): AccessSwitch`, y `channelUp,` como tercer argumento de `AccessSwitch(…)`, tras el objeto `Saved`.
- Sustituir `cut` por:

```kotlin
    /**
     * A live channel with the PC (addendum 2026-10-19, R20): the host welcomed the app through adb, paused included.
     * INCOMPATIBLE does not count: the app stopped there and the socket may be gone.
     */
    fun channelUp(state: BridgeState): Boolean = state.connection == BridgeState.Connection.CONNECTED

    /**
     * Returns false when the permission is missing or the change did not stick. [channelUp] is channelUp() of the
     * bridge at the tap: a live channel proves adb_enabled=1 when it reads 0 (R16), and nothing else (R20).
     */
    fun cut(context: Context, channelUp: Boolean = false): Boolean {
        AccessDiag.log(context, "cut.before")
        return switch(context, channelUp).cut().also { AccessDiag.log(context, "cut.after ok=$it") }
    }
```

`restore`, `isCut`, `needsManualRestore` y `observe` no cambian: reactivar solo es posible con el estado CUT, que el canal vivo descarta (R3, R5).

(c) `ui/ChatRoot.kt:136`: `cut = { report(ok = CutAccess.cut(context, CutAccess.channelUp(app.bridge.state.value)), restoring = false) },`

(d) `bridge/BridgeService.kt:161`: `ACTION_CUT -> if (!CutAccess.cut(this, CutAccess.channelUp(app.bridge.state.value))) toast(R.string.notif_cut_failed)` (`app` ya existe en `onStartCommand`).

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest' --tests '*LinkStatusTest' --tests '*ConversationViewTest' --tests '*NoHardcodedTextTest'` y luego `$G :app:assembleDebug :app:assembleDebugAndroidTest` (compila `CutAccessOnEmulatorTest`, que sigue llamando a `CutAccess.cut(context)`).
Expected: PASS (`AccessSwitchTest` 30, `CutAccessTest` 4, `ChatRootCutWiringTest` 4) y BUILD SUCCESSFUL.

Mutaciones:
- M6. En `cut()`, `adbZeroIsDoubtful(adb, wifi)` sin `channelUp` → falla `aLiveChannelSavesUsbAsOnEvenIfItReadsOff` (y `CutAccessTest.aLiveChannelSavesUsbAsOn`).
- M7. `readAsOn()` termina en `… == 1 || channelUp` → falla `aLiveChannelNeverUndoesACut`.
- M8. En `readsBackAs`, sin `channelUp` → falla `aLiveChannelLetsADoubtfulZeroPassTheRestore`.
- M9. `channelUp(state)` también verdadero con `INCOMPATIBLE` → falla `theChannelIsUpOnlyWhenTheHostIsConnected`.
- M10. En `BridgeService`, volver a `CutAccess.cut(this)` → falla `bothCutButtonsPassTheChannel`.

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/ChatRoot.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeService.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/control
git commit -m "fix(android): un canal vivo con el PC prueba adb_enabled=1 al cortar, sin deshacer nunca un corte

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: El registro `ADBuddyAccess` dice el canal y cuándo el 0 se contradice — revisor adbuddy-reviewer

R21. La línea añade `channel=up|down` cuando se sabe (al cortar) y `doubtful=adb_enabled(wifi|channel|wifi+channel)`.

**Files:**
- Modify: `control/AccessDiag.kt`, `control/CutAccess.kt`
- Test: `control/AccessDiagTest.kt`

**Interfaces:**
- Consumes: `AccessSwitch.adbZeroIsDoubtful` (tarea 1), `CutAccess.cut(context, channelUp)` (tarea 2).
- Produces: `AccessDiag.line(event, sdk, readings, saved, channelUp: Boolean? = null)`, `AccessDiag.doubt(readings: List<Reading>, channelUp: Boolean): String?`, `AccessDiag.log(context, event, channelUp: Boolean? = null)`.

- [ ] **Step 1: tests que fallan.** Al final de `AccessDiagTest` (los tres tests existentes no cambian):

```kotlin
    // Addendum 2026-10-19 R21: the line says when the adb_enabled read is a 0 the rest contradicts.
    private fun r(adb: String?, wifi: String?) = listOf(AccessDiag.Reading("adb_enabled", adb, null), AccessDiag.Reading("adb_wifi_enabled", wifi, null))

    @Test fun theLineSaysWhenWirelessContradictsAZero() {
        // What the user's Pixel (Android 17) logged on 2026-10-19 while `adb shell` said 1 and 1.
        assertEquals(
            "observe sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 saved.adb_enabled=- saved.adb_wifi_enabled=- doubtful=adb_enabled(wifi)",
            AccessDiag.line("observe", 37, r("0", "1"), mapOf("adb_enabled" to null, "adb_wifi_enabled" to null)),
        )
    }

    @Test fun theLineSaysTheChannelWhenItIsKnown() {
        assertEquals(
            "cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:0 channel=up doubtful=adb_enabled(channel)",
            AccessDiag.line("cut.before", 37, r("0", "0"), emptyMap(), channelUp = true),
        )
        assertEquals(
            "cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 channel=up doubtful=adb_enabled(wifi+channel)",
            AccessDiag.line("cut.before", 37, r("0", "1"), emptyMap(), channelUp = true),
        )
    }

    @Test fun nothingContradictedNothingSaid() {
        assertEquals("cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:0 channel=down", AccessDiag.line("cut.before", 37, r("0", "0"), emptyMap(), channelUp = false))
        assertEquals("observe sdk=37 adb_enabled=raw:1 adb_wifi_enabled=raw:1", AccessDiag.line("observe", 37, r("1", "1"), emptyMap()))
        assertEquals("observe sdk=37 adb_enabled=null adb_wifi_enabled=raw:1", AccessDiag.line("observe", 37, r(null, "1"), emptyMap()))
    }
```

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*AccessDiagTest'`
Expected: FAIL de compilación (`line` no tiene `channelUp`). Si se quita el argumento de los dos tests que lo usan, falla `theLineSaysWhenWirelessContradictsAZero` (falta `doubtful=`).

- [ ] **Step 3: implementación mínima** en `control/AccessDiag.kt`:

```kotlin
    /**
     * [channelUp]: whether a channel with the PC was alive (null when not known: only cut() knows it). The line ends
     * with `doubtful=adb_enabled(<why>)` when the adb_enabled read is a 0 the rest contradicts (addendum R21).
     */
    fun line(event: String, sdk: Int, readings: List<Reading>, saved: Map<String, Int?>, channelUp: Boolean? = null): String = buildString {
        // … el cuerpo actual sin cambios (event, sdk, lecturas, saved.*), y al final:
        if (channelUp != null) append(" channel=").append(if (channelUp) "up" else "down")
        doubt(readings, channelUp == true)?.let { append(" doubtful=adb_enabled(").append(it).append(')') }
    }

    /** Why a 0 read for adb_enabled is not believed (AccessSwitch.adbZeroIsDoubtful): "wifi", "channel", "wifi+channel"; null when it is. */
    fun doubt(readings: List<Reading>, channelUp: Boolean): String? {
        fun value(name: String) = readings.firstOrNull { it.name == name && it.error == null }?.raw?.trim()?.toIntOrNull()
        val adb = value(AccessSwitch.ADB_ENABLED)
        val wifi = value(AccessSwitch.ADB_WIFI_ENABLED)
        if (!AccessSwitch.adbZeroIsDoubtful(adb, wifi, channelUp)) return null
        return listOfNotNull("wifi".takeIf { wifi == 1 }, "channel".takeIf { channelUp }).joinToString("+")
    }
```

`log` pasa a `fun log(context: Context, event: String, channelUp: Boolean? = null)` y llama a `line(event, Build.VERSION.SDK_INT, names.map { read(resolver, it) }, saved, channelUp)`.

En `control/CutAccess.kt`, `cut` pasa el canal a las dos líneas sin cambiar los literales: `AccessDiag.log(context, "cut.before", channelUp)` y `AccessDiag.log(context, "cut.after ok=$it", channelUp)`. `observe` y `restore` siguen sin canal (`null`): su línea no lleva `channel=`, pero sí `doubtful=adb_enabled(wifi)` cuando toca.

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessDiagTest' --tests '*CutAccessTest' --tests '*NoHardcodedTextTest'`
Expected: PASS (`AccessDiagTest` 6, `CutAccessTest` 4, `NoHardcodedTextTest` 3). Si `NoHardcodedTextTest` marcara un literal nuevo de `AccessDiag.kt`, añadir su línea a `i18n-exceptions.txt` ("línea de log para adb logcat, no se muestra") y decirlo en el informe. En la réplica del plan no lo marcó.

Mutaciones:
- M11. Quitar la línea `doubt(…)?.let { … }` → fallan `theLineSaysWhenWirelessContradictsAZero` y `theLineSaysTheChannelWhenItIsKnown`.
- M12. `doubt(readings, false)` en `line` (ignorar el canal) → falla `theLineSaysTheChannelWhenItIsKnown`.
- M13. En `value`, `?.toIntOrNull() ?: 0` (un ajuste nunca escrito cuenta como 0) → falla `nothingContradictedNothingSaid` (caso `r(null, "1")`).

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/test/kotlin/tech/hybriq/adbuddy/control
git commit -m "feat(android): el registro ADBuddyAccess dice el canal y cuándo adb_enabled lee un 0 contradicho

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Cierre: batería de la CI, README e inventario — revisor adbuddy-reviewer

**Files:**
- Modify: `android/README.md` (párrafo "Cut access state"), `docs/superpowers/plans/pendientes-m3-m5.md` (sección UX1), `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (filas UX1-1 y UX1-2)

- [ ] **Step 1:** `$G --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (en segundo plano, timeout 20 min) → BUILD SUCCESSFUL. Las capturas no cambian (no se toca UI): `git status --short android/app/src/test/screenshots` → limpio.
- [ ] **Step 2:** en `android/README.md`, tras el párrafo **Cut access state**, añadir:

```markdown
**False readings (Android 17).** An app reads `adb_enabled=0` there while adb works; `adb_wifi_enabled` reads right.
ADBuddy believes a 1 always and a 0 only when nothing contradicts it: with wireless debugging at 1 or a live channel
with the PC, it saves USB debugging as on before a cut and restores it so. A cut is confirmed by wireless debugging
reading 0 (and USB debugging not reading 1); the log line ends with `doubtful=adb_enabled(…)` when a 0 was not believed.
```

- [ ] **Step 3:** en `2026-10-03-validaciones-manuales.md`: UX1-1 pasa a **hecha** (2026-10-19: `raw:0` frente a `1` del shell en `adb_enabled`, `adb_wifi_enabled` coincide; adenda `2026-10-19-acceso-lecturas-falsas.md`). UX1-2 pasa a decir "V2 rehecha de la adenda (§5): cortar y reactivar con los valores reales desde el shell y, si hay cable, `adb -d` tras el corte", con prerrequisito "build de `ux/acceso-a17` instalada (permiso)" en vez de "UX1-1". En la lista de pasos del usuario (línea "Estado del acceso y aspecto visual"), cambiar la build a `ux/acceso-a17` y quitar la lectura de UX1-1.
- [ ] **Step 4:** en `pendientes-m3-m5.md`, sección UX1, añadir una fila: `UX-f | En Android 17 no se confirma que adb_enabled=0/1 se aplique (lectura falsa) | Adenda 2026-10-19 R18-R19; lo mide V2 (UX1-2). Si no se aplica, plan aparte: confirmar el corte con el canal.`
- [ ] **Step 5: commit** `docs: cierre de la adenda de lecturas falsas de adb_enabled` (con el trailer).

---

## Validaciones manuales pendientes (solo lo que exige el móvil real; siempre con permiso explícito del usuario)

- **V2 (rehecha; spec §5).** Con la build de este plan instalada (instalar es un cambio en el móvil: pedir permiso):
  1. Antes: `adb shell settings get global adb_enabled` y `adb shell settings get global adb_wifi_enabled` (esperado `1` y `1`), y `adb logcat -c`.
  2. Cortar desde el escudo de la app con el chat conectado. El PC pierde la conexión (`adb devices` sin el móvil u `offline`; `adbuddy serve` lo dice) y la app dice "Acceso cortado".
  3. Con un cable USB, si lo hay: `adb -d shell settings get global adb_enabled`. Si responde, la USB no se cortó (R18 se queda corto: apuntarlo y abrir un plan). Si no hay dispositivo, el corte se aplicó.
  4. Reactivar desde la app (escudo tachado, diálogo y confirmar): "El PC vuelve a tener acceso" y el PC reconecta (el puerto inalámbrico puede cambiar).
  5. `adb shell settings get global adb_enabled` / `adb_wifi_enabled` → `1` / `1`; `adb logcat -d -s ADBuddyAccess`: `cut.before … channel=up doubtful=adb_enabled(wifi+channel)`, `cut.after ok=true … saved.adb_enabled=1 saved.adb_wifi_enabled=1`, `restore.after ok=true`.
  6. Si reactivar falla, el usuario enciende la depuración inalámbrica (y la USB si la tenía) en Opciones de desarrollador. Apuntar la línea `restore.after`.
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+).

## Autorrevisión

- Cobertura de la adenda: R16 → tareas 1 (inalámbrica) y 2 (canal); R17 → 1 (`onlyAContradictedZeroIsDoubted`, `aZeroWithNothingAgainstItIsBelieved`, `withoutAContradictionAZeroAfterRestoringIsBelieved`); R18 → 1 (`aWriteThatDidNotStickIsNotDone` sin tocar, M5); R19 → 1 y 2; R20 → 2 (`aLiveChannelNeverUndoesACut`, `theChannelIsUpOnlyWhenTheHostIsConnected`, `bothCutButtonsPassTheChannel`); R21 → 3; V2-V3 → sección de arriba y tarea 4.
- Garantías R1-R5: ningún test existente cambia salvo el de R16; `isCut`, `readAsOn`, `forgetIfRestored`, `restore` sin registro y `needsManualRestore` no se tocan; la UI no cambia.
- Cada tarea compila sola: 1 (solo `AccessSwitch`); 2 (usa 1; `CutAccess.cut(context)` sigue valiendo por el valor por defecto); 3 (usa 1 y 2); 4 (docs).
- Réplica: el código de las tareas 1-3 se aplicó entero sobre `ux/acceso-a17` (9f3e353a) antes de escribir el plan. Resultado: `AccessSwitchTest` 30/30, `CutAccessTest` 4/4, `AccessDiagTest` 6/6, `ChatRootCutWiringTest` 4/4, `NoHardcodedTextTest` 3/3, `LinkStatusTest` 9/9 y `ConversationViewTest` 21/21. M1-M4 y M7 se comprobaron (fallan los tests indicados). Después se deshizo la réplica.
- Nombres: `AccessSwitch.adbZeroIsDoubtful`, `AccessSwitch(…, channelUp)`, `CutAccess.channelUp(state)`, `CutAccess.cut(context, channelUp)`, `AccessDiag.line(…, channelUp)`, `AccessDiag.doubt`, `AccessDiag.log(…, channelUp)`, `FakeSettings.readsZero`.
