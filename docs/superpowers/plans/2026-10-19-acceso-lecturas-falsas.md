# Acceso: lecturas falsas de `adb_enabled`. Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que cortar y reactivar funcionen en Android 17, donde la app lee `adb_enabled=0` aunque esté a 1. Para ello: no guardar ni restaurar ese 0 falso; confirmar el corte y la reactivación con lo que sí se puede leer (lo que devuelve `putInt` y `adb_wifi_enabled`); comprobar después del corte que el canal cae y que nadie responde en 127.0.0.1:5555; y que el registro `ADBuddyAccess` diga cuándo una lectura se contradice.

**Architecture:** la regla vive en `control/AccessSwitch.kt` (pura, tests JVM). `adbZeroIsDoubtful(adb, wifi, channelUp)` decide cuándo no se cree un 0. Si `cut()` guardó un 1 por encima de un 0 en duda, lo marca en el registro (`adb_enabled_doubted`), y `restore()` solo acepta releer 0 tras escribir 1 con esa marca. La comprobación posterior al corte es `control/CutConfirm.kt` (pura, con tiempo y sondeo inyectados), y la cablean los dos botones de cortar a través de `CutAccess.confirm`.

**Tech Stack:** Kotlin, coroutines, Robolectric (JVM), Gradle 9.8 sin wrapper (como la CI).

**Spec:** `docs/superpowers/specs/2026-10-19-acceso-lecturas-falsas.md` (adenda, R16-R23), subordinada a `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md` (R1-R5 y §8 siguen en vigor).

## Global Constraints

- Ningún test de §8 de la spec del 2026-10-18 se borra ni se debilita: `cuttingTurnsBothOff`, `aWriteThatDidNotStickIsNotDone`, `withoutThePermissionNothingIsCut`, `restoringWithNothingSavedTurnsOnlyAdbOn` y los de UI. Dos tests existentes cambian, los dos fuera de §8 y con su ruling: `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere` (tarea 1, R16) y `ChatRootCutWiringTest.theShieldCutsBothSettingsAndSaysSo` (tarea 4, R22: espera al aviso, que ahora llega tras la comprobación). Si falla cualquier otro test existente, la tarea se para y lo adjudica el controlador.
- `cut()` nunca escribe un 1. El canal (`channelUp`) no entra nunca en `isCut()`, `readAsOn()`, `forgetIfRestored()` ni `restore()` (R20). En `CutConfirm` solo sirve como prueba en contra.
- Un 1 leído se cree siempre. Un 0 solo se pone en duda con contradicción (R17). Nada se decide por `Build.VERSION.SDK_INT`.
- El registro solo lleva números y palabras fijas (sin nombre del PC ni texto del host). Los literales de log de `CutAccess.kt` que figuran en `android/app/src/test/resources/i18n-exceptions.txt` (`cut.after ok=`, `restore.after ok=`) no cambian de texto.
- Todo texto visible nuevo va en `res/values/strings.xml` y en `res/values-es/strings.xml` (`StringsParityTest`, `NoHardcodedTextTest`).
- Cada test crítico lleva su **mutación**: el implementador la aplica a mano, ve fallar el test indicado, la deshace y lo dice en su informe.
- Nunca se ejecuta nada en el móvil del usuario: ni tareas `connected*` de Gradle ni `adb` contra el Pixel.
- Git: identidad local hybriqtech del repo, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, sin push. **La rama no se integra en `main` hasta que pase V2** (adenda §5).

## Comandos (los reales: `.github/workflows/android.yml` y el plan del 2026-10-18)

Desde `C:\Users\Jose\orca\workspaces\adbuddy\ux\android`, en Git Bash. No hay wrapper ni `local.properties` en el worktree (está ignorado), así que el SDK va por variable:

```bash
export ANDROID_HOME='C:/Users/Jose/AppData/Local/Android/Sdk'
G=/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle
```

- Tests concretos: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest'` (la primera ejecución en frío tarda unos 5 min; después, unos 20-45 s).
- Resultados: `app/build/test-results/testDebugUnitTest/TEST-<clase>.xml` (`failures=` y `<failure` con el nombre del test). Con `-q`, Gradle no lista los tests que fallan: hay que mirar el XML.
- Todo lo de la CI (tarea 5): `$G --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (en segundo plano, timeout 20 min).

## Revisores

| Tareas | Revisor | Por qué |
|---|---|---|
| 1, 2, 4 | `adbuddy-reviewer-opus` | cortar y reactivar el acceso, y comprobar el corte (seguridad) |
| 3, 5 | `adbuddy-reviewer` | diagnóstico y cierre |

Tiempo para los revisores: solo `--tests` concretos, máximo 15 min de ejecución por revisión.

## Mapa de ficheros

Rutas de Kotlin relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/`; tests en `android/app/src/test/kotlin/tech/hybriq/adbuddy/`.

| Fichero | Tarea | Qué |
|---|---|---|
| `control/AccessSwitch.kt` | 1, 2 | `put(): Boolean`, `adbZeroIsDoubtful`, marca `ADB_DOUBTED`, registro antiguo 0/1; parámetro `channelUp` |
| `control/AccessSwitchTest.kt` | 1, 2 | `FakeSettings.readsZero` (Android 17), `refuses` (`put` = false) y tests nuevos |
| `control/CutAccess.kt` | 1, 2, 3, 4 | `put` devuelve lo de `putInt`; `channelUp(state)`, `cut(context, channelUp)`; canal en `cut.before`; `confirm`, `probe` y `waitMs` |
| `ui/ChatRoot.kt`, `bridge/BridgeService.kt` | 2, 4 | los dos botones de cortar pasan el canal y comprueban el corte |
| `control/CutAccessTest.kt` | 2, 4 | `channelUp` y cableado |
| `control/AccessDiag.kt`, `control/AccessDiagTest.kt` | 3 | `channel=`, `saved.adb_enabled_doubted=` y `doubtful=adb_enabled(…)` |
| `control/CutConfirm.kt` (nuevo), `control/CutConfirmTest.kt` (nuevo) | 4 | comprobación tras el corte |
| `res/values/strings.xml`, `res/values-es/strings.xml` | 4 | `cut_still_connected` |
| `ui/ChatRootCutWiringTest.kt` | 4 | sondeo falso, espera del aviso, aviso "sigue conectado" |
| `android/README.md`, `docs/superpowers/plans/pendientes-m3-m5.md`, `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 5 | cierre |

---

### Task 1: Un 0 de `adb_enabled` que la inalámbrica contradice no se cree; `put` que falla es un fallo — crítica, revisor opus

Cubre R16, R17, R18, R19 y R23, todavía sin el canal (llega en la tarea 2):

- Un 0 de `adb_enabled` con `adb_wifi_enabled=1` se guarda como 1, y el registro lleva `adb_enabled_doubted=1`.
- Al restaurar, releer 0 tras escribir 1 solo vale con esa marca.
- `put()` devuelve `Boolean`; un `false` es una escritura fallida.
- Un registro 0/1, que solo pudo dejar la build anterior, se restaura con `adb_enabled=1`.

**Files:**
- Modify: `control/AccessSwitch.kt` (entero), `control/CutAccess.kt` (solo el `put` del `Settings` anónimo)
- Test: `control/AccessSwitchTest.kt`

**Interfaces:**
- Produces: `AccessSwitch.Settings.put(name, value): Boolean`, `AccessSwitch.ADB_DOUBTED = "adb_enabled_doubted"`, `AccessSwitch.adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false): Boolean`.

- [ ] **Step 1: tests que fallan.** En `AccessSwitchTest.kt`:

(a) `FakeSettings` queda así (añade `readsZero` y `refuses`, y `put` devuelve `Boolean`):

```kotlin
    private class FakeSettings(vararg values: Pair<String, Int>) : AccessSwitch.Settings {
        val values = mutableMapOf(*values)
        var allowed = true
        var ignoreWrites = emptySet<String>() // written without error, but the value does not change
        var unreadable = emptySet<String>() // reading throws, as Android 12+ may do for a hidden setting
        var readsZero = emptySet<String>() // reads 0 whatever the value, as Android 17 does for adb_enabled (addendum 2026-10-19)
        var refuses = emptySet<String>() // put() says false and nothing changes (Settings.Global.putInt failing)
        override fun get(name: String): Int? {
            if (name in unreadable) throw SecurityException("cannot read $name")
            if (name in readsZero) return 0
            return values[name]
        }
        override fun put(name: String, value: Int): Boolean {
            if (!allowed) throw SecurityException("no WRITE_SECURE_SETTINGS")
            if (name in refuses) return false
            if (name !in ignoreWrites) values[name] = value
            return true
        }
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
        // Known cost (addendum R19): USB only on Android 17. Writing 1 and reading 0 with wireless at 0 is not done.
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

    // --- Addendum 2026-10-19 R18: put() saying false is a failed write, even when the reading cannot see it. ---

    @Test fun android17_aWriteAndroidRefusesIsNotDone() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb); refuses = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertFalse("adb_enabled reads 0 anyway: only put() can tell", sw.cut())
        assertTrue(saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun android17_aRefusedRestoreKeepsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.refuses = setOf(adb)
        assertFalse(sw.restore())
        assertEquals(1, saved.values[adb])
    }

    // --- Addendum 2026-10-19 R23: a record of only wireless debugging comes from the build before the addendum. ---

    @Test fun anOldRecordOfOnlyWirelessRestoresUsbToo() {
        for (android17 in listOf(false, true)) {
            val s = FakeSettings(adb to 0, wifi to 0).apply { if (android17) readsZero = setOf(adb) }
            val saved = FakeSaved().apply { values[adb] = 0; values[wifi] = 1 }
            assertTrue("android17=$android17", AccessSwitch(s, saved).restore())
            assertEquals("android17=$android17", 1, s.values[adb])
            assertEquals(1, s.values[wifi])
            assertTrue(saved.values.isEmpty())
        }
    }
```

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest'`
Expected: FAIL de compilación (el fake devuelve `Boolean` y la interfaz no; `adbZeroIsDoubtful` no existe). Para ver el fallo de comportamiento, poner un andamio que compile, que el paso 3 sustituye: en la interfaz, `fun put(name: String, value: Int): Boolean`; la línea de `CutAccess` del paso 3b; y en el `companion`, `const val ADB_DOUBTED = "adb_enabled_doubted"` y `fun adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false) = false`. `write` sigue ignorando lo que devuelve `put`. Con eso fallan `cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth`, `android17_cuttingSavesUsbAsOnAndCuts`, `android17_restoringBringsBothBackAndIsDone`, `onlyAContradictedZeroIsDoubted`, `android17_aWriteAndroidRefusesIsNotDone`, `android17_aRefusedRestoreKeepsTheRecord` y `anOldRecordOfOnlyWirelessRestoresUsbToo`. `aWirelessThatDoesNotComeBackFailsTheRestore`, `aZeroWithNothingAgainstItIsBelieved` y `withoutAContradictionAZeroAfterRestoringIsBelieved` ya pasan: impiden dudar de más, y los prueban M4, M7 y M8. Guardar las dos salidas en el informe.

- [ ] **Step 3: implementación mínima.**

(a) `control/AccessSwitch.kt` entero. Su KDoc de `cut()` **sustituye** al actual; no se añade un segundo:

```kotlin
package tech.hybriq.adbuddy.control

/**
 * Cut access and its undo (spec v1 §4.6; spec 2026-10-18 R1-R4), over any settings store, so it is unit-tested.
 * Cutting turns off wireless debugging and then adb debugging (S1: both together close adbd and tcpip mode), after
 * saving what they were; restoring puts those values back as they were, never wireless debugging on blindly.
 *
 * "Cut" is OUR cut: the saved values are the record of a cut ADBuddy made and read back. Without it nothing is cut,
 * whatever is read (on the user's Pixel, Android 17, the app read adb_enabled=0 while adb worked: Task 1's diagnosis).
 * A readable setting at 1 means someone gave access back by hand: the record goes. An unreadable one proves nothing.
 *
 * False readings (addendum 2026-10-19, R16-R19): Android 17 gives apps adb_enabled=0 while `adb shell` says 1, and
 * reads adb_wifi_enabled right. A 1 read is always believed (the false readings are 0); a 0 is doubted only when the
 * rest contradicts it (adbZeroIsDoubtful). Then cut() saves adb_enabled=1 and marks the record (ADB_DOUBTED), and
 * restore() takes a 0 read after writing 1 as done only with that mark. A put() that says false is a failed write.
 * So on Android 17 cut() is confirmed by put() and by wireless debugging reading 0, not by adb_enabled: the screen
 * checks the cut afterwards (CutConfirm, R22) and V2 of the addendum measures it from the shell.
 */
class AccessSwitch(private val settings: Settings, private val saved: Saved) {
    /** Settings.Global: get is null for a setting never written and THROWS SecurityException when it cannot be read; put throws without WRITE_SECURE_SETTINGS and returns false when the write failed (Settings.Global.putInt). */
    interface Settings {
        fun get(name: String): Int?
        fun put(name: String, value: Int): Boolean
    }

    /** Where the values before the cut are kept (the app's private preferences); null removes. */
    interface Saved {
        fun get(name: String): Int?
        fun put(name: String, value: Int?)
    }

    /** The value, with a never-written setting as [missing]; null when it cannot be read. */
    private fun read(name: String, missing: Int): Int? = try {
        settings.get(name) ?: missing
    } catch (e: SecurityException) {
        null
    }

    private fun hasRecord(): Boolean = saved.get(ADB_ENABLED) != null && saved.get(ADB_WIFI_ENABLED) != null

    /** A readable setting says access is on (USB or wireless debugging at 1). adb_enabled never written counts as on (getInt(…, 1) of old). */
    private fun readAsOn(): Boolean = read(ADB_ENABLED, missing = 1) == 1 || read(ADB_WIFI_ENABLED, missing = 0) == 1

    fun isCut(): Boolean = hasRecord() && !readAsOn()

    /** true once wireless debugging reads back 0 and adb_enabled does not read back 1 (R18); false without the permission, if a change did not stick or cannot be read back. */
    fun cut(): Boolean {
        forgetIfRestored() // a record that a readable setting contradicts is stale: do not keep it
        // Cutting again keeps the values of the first cut.
        val savedNow = !hasRecord()
        if (savedNow) {
            val adb = read(ADB_ENABLED, missing = 1)
            val wifi = read(ADB_WIFI_ENABLED, missing = 0)
            // A 0 that wireless debugging contradicts is Android 17's false reading, not the setting (R16);
            // the record says so, for restore() to read back past it (R19).
            val doubted = adb != null && adbZeroIsDoubtful(adb, wifi)
            saved.put(ADB_ENABLED, if (adb == null || doubted) 1 else adb)
            saved.put(ADB_WIFI_ENABLED, wifi ?: 0)
            saved.put(ADB_DOUBTED, if (doubted) 1 else null)
        }
        val ok = write(ADB_WIFI_ENABLED to 0, ADB_ENABLED to 0)
        if (!ok && savedNow) forget() // a cut that did not stick must not leave a record that says "cut"
        return ok
    }

    /**
     * Puts back what cut() saved, as it was (adb_enabled=0 included). Without a record: access already on is left as
     * it is (wireless debugging is never turned off), otherwise only adb debugging comes back and wireless debugging is
     * not touched (its value is unknown or already off). A record of both off is never guessed: nothing is written, the
     * record stays and needsManualRestore() says so (the screen sends the person to Developer options). A tcpip port
     * that was open comes back too until the host runs `adb usb` (spec §4.6).
     */
    fun restore(): Boolean {
        val savedAdb = saved.get(ADB_ENABLED)
        val savedWifi = saved.get(ADB_WIFI_ENABLED)
        if (savedAdb == null || savedWifi == null) {
            if (readAsOn()) return true
            return write(ADB_ENABLED to 1)
        }
        if (savedAdb == 0 && savedWifi == 0) return false
        // Only wireless debugging saved: a record of a build before the addendum, which kept Android 17's false 0 (R23).
        val legacy = savedAdb == 0 && savedWifi == 1
        val adb = if (legacy) 1 else savedAdb
        val doubted = legacy || saved.get(ADB_DOUBTED) == 1
        if (!write(ADB_ENABLED to adb, ADB_WIFI_ENABLED to savedWifi, doubted = doubted)) return false
        forget()
        return true
    }

    /** Our record says both were off before the cut (or they were read so): restore() will not guess what to turn on. */
    fun needsManualRestore(): Boolean = saved.get(ADB_ENABLED) == 0 && saved.get(ADB_WIFI_ENABLED) == 0

    /**
     * Forgets the record once a readable setting says access is on again without restore() (the person turned it on
     * in Settings): a later cut must not bring back what there was before the first one. Called on every change of
     * the settings (CutAccess.observe). An unreadable setting keeps the record.
     */
    fun forgetIfRestored() {
        if (hasRecord() && readAsOn()) forget()
    }

    private fun forget() {
        saved.put(ADB_ENABLED, null)
        saved.put(ADB_WIFI_ENABLED, null)
        saved.put(ADB_DOUBTED, null)
    }

    /** Writes in order and reads back; false as soon as Android refuses one (an exception, or put() saying false: R18). */
    private fun write(vararg changes: Pair<String, Int>, doubted: Boolean = false): Boolean = try {
        changes.all { (name, value) -> settings.put(name, value) } &&
            changes.all { (name, value) -> readsBackAs(name, value, doubted) }
    } catch (e: SecurityException) {
        false
    }

    /**
     * Whether [name] reads back as [value] after writing it (never written = 0; unreadable never passes, R2).
     * adb_enabled=1 also passes with a 0 when [doubted]: the record says the 1 was saved over a doubtful 0 (R19), so
     * Android 17's false 0 cannot say the write did not stick. Without that mark a 0 is believed.
     * adb_enabled=0 still fails with a 1: a 1 read is never false (R18).
     */
    private fun readsBackAs(name: String, value: Int, doubted: Boolean): Boolean {
        val now = read(name, missing = 0) ?: return false
        if (now == value) return true
        return doubted && name == ADB_ENABLED && value == 1 && now == 0
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden

        /** In the record only (never a setting): 1 when the adb_enabled saved as 1 had been read as a doubtful 0 (R19). */
        const val ADB_DOUBTED = "adb_enabled_doubted"

        /**
         * A 0 read for adb_enabled that the rest contradicts (addendum 2026-10-19, R16-R17): wireless debugging reads
         * 1, or a channel with the PC is alive (it goes through adbd). Detected by the contradiction, never by the
         * Android version. A 1 is never doubted, and a 0 with nothing against it is believed.
         */
        fun adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false): Boolean = adb == 0 && (wifi == 1 || channelUp)
    }
}
```

(b) En `control/CutAccess.kt`, el `put` del `AccessSwitch.Settings` anónimo pasa a:

```kotlin
                override fun put(name: String, value: Int): Boolean = Settings.Global.putInt(resolver, name, value)
```

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest'`
Expected: PASS (`AccessSwitchTest` 30 tests, `CutAccessTest` 1, `ChatRootCutWiringTest` 4).

Mutaciones (una a una: aplicar, ver el fallo en el XML y deshacer). Las marcadas con ✓ se comprobaron en la réplica.
- M1 ✓. En `write`, ignorar lo que devuelve `put` (`changes.map { … settings.put(…) }.let { true } &&`) → fallan `android17_aWriteAndroidRefusesIsNotDone` y `android17_aRefusedRestoreKeepsTheRecord`.
- M2 ✓. En `readsBackAs`, `return false` en vez de la línea con `doubted` → fallan `android17_restoringBringsBothBackAndIsDone` y `anOldRecordOfOnlyWirelessRestoresUsbToo`.
- M3 ✓. En `forget()`, no borrar `ADB_DOUBTED` → falla `android17_restoringBringsBothBackAndIsDone` ("forgotten once restored").
- M4 ✓. En `restore()`, `val doubted = true` → falla `withoutAContradictionAZeroAfterRestoringIsBelieved`.
- M5 ✓. En `restore()`, `val adb = savedAdb` (sin R23) → falla `anOldRecordOfOnlyWirelessRestoresUsbToo`.
- M6 ✓. En `cut()`, guardar `adb ?: 1` sin la duda → fallan `cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth`, `android17_cuttingSavesUsbAsOnAndCuts` y `android17_restoringBringsBothBackAndIsDone`.
- M7 ✓. `adbZeroIsDoubtful = adb == 0` → fallan `aZeroWithNothingAgainstItIsBelieved` y `onlyAContradictedZeroIsDoubted`.
- M8 ✓. En `write`, releer solo `adb_enabled` (`changes.filter { it.first == ADB_ENABLED }.all { … }`) → falla `aWirelessThatDoesNotComeBackFailsTheRestore`.

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/test/kotlin/tech/hybriq/adbuddy/control/AccessSwitchTest.kt
git commit -m "fix(android): un 0 de adb_enabled que la inalámbrica contradice no se guarda ni impide reactivar

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Un canal vivo con el PC también contradice el 0 — crítica, revisor opus

R16 y R20: los dos botones de cortar (el escudo de la pantalla y "Cortar" de la notificación) pasan si el canal está vivo. El canal solo influye en lo que se guarda (y en la marca); nunca deshace un corte ni entra en `restore()`.

**Files:**
- Modify: `control/AccessSwitch.kt`, `control/CutAccess.kt`, `ui/ChatRoot.kt:136`, `bridge/BridgeService.kt:161`
- Test: `control/AccessSwitchTest.kt`, `control/CutAccessTest.kt`

**Interfaces:**
- Consumes: `AccessSwitch.adbZeroIsDoubtful`, `ADB_DOUBTED` (tarea 1).
- Produces: `AccessSwitch(settings, saved, channelUp: Boolean = false)`, `CutAccess.channelUp(state: BridgeState): Boolean`, `CutAccess.cut(context: Context, channelUp: Boolean = false): Boolean`. Sin el argumento funciona como antes, así que `CutAccessOnEmulatorTest` compila sin cambios.

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

    @Test fun aDoubtRecordedAtTheCutLetsTheRestorePass() {
        // R19: restoring is only possible once cut (the channel is gone by then), so the doubt travels in the record.
        val s = FakeSettings(adb to 1, wifi to 0).apply { readsZero = setOf(adb) } // USB only, Android 17
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals(1, saved.values[AccessSwitch.ADB_DOUBTED])
        assertTrue(AccessSwitch(s, saved).restore())
        assertEquals(1, s.values[adb])
        assertEquals(0, s.values[wifi])
        assertTrue("the doubt goes with the record", saved.values.isEmpty())
    }

    @Test fun aZeroNobodyDoubtedLeavesNoMark() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals("a 1 read is not a doubt", null, saved.values[AccessSwitch.ADB_DOUBTED])
        s.ignoreWrites = setOf(adb) // and a USB write that does not stick is still seen when the reading is right
        assertFalse(AccessSwitch(s, saved).restore())
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

(b) `CutAccessTest.kt` entero en esta tarea (el test existente no cambia; `bothCutButtonsPassTheChannel` se amplía en la tarea 4):

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

    // The shield of the screen and Cut in the notification (spec 2026-10-18 R5, §8.7) tell cut() whether the channel is
    // alive (R20).
    @Test fun bothCutButtonsPassTheChannel() {
        val up = Regex("""val up = CutAccess\.channelUp\(app\.bridge\.state\.value\)""")
        val cut = Regex("""CutAccess\.cut\(\s*(context|this)\s*,\s*up\s*\)""")
        val withoutChannel = Regex("""CutAccess\.cut\(\s*(context|this)\s*\)""")
        for (path in listOf("ui/ChatRoot.kt", "bridge/BridgeService.kt")) {
            val src = File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText()
            assertTrue("$path reads the channel", up.containsMatchIn(src))
            assertTrue("$path passes it to cut()", cut.containsMatchIn(src))
            assertFalse("$path cuts without the channel", withoutChannel.containsMatchIn(src))
        }
    }
}
```

`bothCutButtonsPassTheChannel` mira el código fuente, como `TouchGuardSourceTest`: el estado `CONNECTED` del bridge no se puede provocar en Robolectric sin un servidor.

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest'`
Expected: FAIL de compilación (no existen el parámetro `channelUp` de `AccessSwitch` ni `CutAccess.channelUp`).

- [ ] **Step 3: implementación mínima.**

(a) `control/AccessSwitch.kt`:
- Cabecera: `class AccessSwitch(private val settings: Settings, private val saved: Saved, private val channelUp: Boolean = false) {`
- En `cut()`, el comentario y la duda pasan a:

```kotlin
            // A 0 that wireless debugging or a live channel contradicts is Android 17's false reading, not the setting (R16);
            // the record says so, for restore() to read back past it (R19).
            val doubted = adb != null && adbZeroIsDoubtful(adb, wifi, channelUp)
```

- Nada más usa `channelUp`: ni `readAsOn()`, ni `isCut()`, ni `forgetIfRestored()`, ni `restore()`, ni `readsBackAs` (R20). Reactivar solo es posible con el estado CUT, y entonces el canal ya no está: la duda viaja en el registro.

(b) `control/CutAccess.kt`:
- Import: `import tech.hybriq.adbuddy.bridge.BridgeState`.
- `private fun switch(context: Context, channelUp: Boolean = false): AccessSwitch`, y `channelUp,` como tercer argumento de `AccessSwitch(…)`, tras el objeto `Saved`.
- Sustituir `cut` y su KDoc de una línea por este bloque. El KDoc nuevo **sustituye** al actual (no se añade un segundo), y `channelUp(state)` es nueva:

```kotlin
    /**
     * A live channel with the PC (addendum 2026-10-19, R20): the host welcomed the app through adb, paused included.
     * INCOMPATIBLE is left out on purpose: the app stopped reconnecting there and the socket may be gone.
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

(c) `ui/ChatRoot.kt:136`:

```kotlin
            cut = {
                val up = CutAccess.channelUp(app.bridge.state.value)
                report(ok = CutAccess.cut(context, up), restoring = false)
            },
```

(d) `bridge/BridgeService.kt:161` (`app` ya existe en `onStartCommand`):

```kotlin
            ACTION_CUT -> {
                val up = CutAccess.channelUp(app.bridge.state.value)
                if (!CutAccess.cut(this, up)) toast(R.string.notif_cut_failed)
            }
```

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest' --tests '*LinkStatusTest' --tests '*ConversationViewTest' --tests '*NoHardcodedTextTest'`; luego `$G :app:assembleDebug :app:assembleDebugAndroidTest` (compila `CutAccessOnEmulatorTest`, que sigue llamando a `CutAccess.cut(context)`).
Expected: PASS (`AccessSwitchTest` 34, `CutAccessTest` 4, `ChatRootCutWiringTest` 4) y BUILD SUCCESSFUL.

Mutaciones:
- M9. En `cut()`, `adbZeroIsDoubtful(adb, wifi)` sin `channelUp` → fallan `aLiveChannelSavesUsbAsOnEvenIfItReadsOff`, `aDoubtRecordedAtTheCutLetsTheRestorePass` y `CutAccessTest.aLiveChannelSavesUsbAsOn`.
- M10 ✓. `readAsOn()` termina en `… == 1 || channelUp` → falla `aLiveChannelNeverUndoesACut`.
- M11. Guardar siempre la marca (`saved.put(ADB_DOUBTED, 1)`) → falla `aZeroNobodyDoubtedLeavesNoMark`.
- M12. `channelUp(state)` también verdadero con `INCOMPATIBLE` → falla `theChannelIsUpOnlyWhenTheHostIsConnected`.
- M13. En `BridgeService`, volver a `CutAccess.cut(this)` → falla `bothCutButtonsPassTheChannel`.

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/ChatRoot.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeService.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/control
git commit -m "fix(android): un canal vivo con el PC prueba adb_enabled=1 al cortar, sin deshacer nunca un corte

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: El registro `ADBuddyAccess` dice el canal, la marca y cuándo el 0 se contradice — revisor adbuddy-reviewer

R21. Cambios en la línea:
- `channel=up|down` solo en `cut.before`, que es el único evento que lo sabe.
- `saved.adb_enabled_doubted=` junto a los otros valores guardados.
- `doubtful=adb_enabled(wifi|channel|wifi+channel)` cuando el 0 leído está en duda.

**Files:**
- Modify: `control/AccessDiag.kt`, `control/CutAccess.kt`
- Test: `control/AccessDiagTest.kt`

**Interfaces:**
- Consumes: `AccessSwitch.adbZeroIsDoubtful`, `ADB_DOUBTED` (tarea 1), `CutAccess.cut(context, channelUp)` (tarea 2).
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

- [ ] **Step 3: implementación mínima** en `control/AccessDiag.kt`. `line` y `log` quedan así, y `doubt` es nueva (`Reading`, `read` y `TAG` no cambian):

```kotlin
    /**
     * [channelUp]: whether a channel with the PC was alive (null when not known: only cut.before says it). The line ends
     * with `doubtful=adb_enabled(<why>)` when the adb_enabled read is a 0 the rest contradicts (addendum R21).
     */
    fun line(event: String, sdk: Int, readings: List<Reading>, saved: Map<String, Int?>, channelUp: Boolean? = null): String = buildString {
        append(event).append(" sdk=").append(sdk)
        for (r in readings) {
            append(' ').append(r.name).append('=')
            append(
                when {
                    r.error != null -> "error:" + r.error
                    r.raw == null -> "null"
                    else -> "raw:" + r.raw.replace(Regex("[\r\n]+"), " ").take(16)
                },
            )
        }
        for ((name, value) in saved) append(" saved.").append(name).append('=').append(value?.toString() ?: "-")
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

```kotlin
    /** Logs one line; never throws (a diagnosis must not break the screen or the service). */
    fun log(context: Context, event: String, channelUp: Boolean? = null) {
        try {
            val resolver = context.contentResolver
            val prefs = context.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE)
            val names = listOf(AccessSwitch.ADB_ENABLED, AccessSwitch.ADB_WIFI_ENABLED)
            val saved = (names + AccessSwitch.ADB_DOUBTED).associateWith { if (prefs.contains(it)) prefs.getInt(it, 0) else null }
            Log.i(TAG, line(event, Build.VERSION.SDK_INT, names.map { read(resolver, it) }, saved, channelUp))
        } catch (e: RuntimeException) {
            Log.w(TAG, "diagnosis failed: " + e.javaClass.simpleName)
        }
    }
```

En `control/CutAccess.kt`, `cut` pasa el canal **solo** a la primera línea: `AccessDiag.log(context, "cut.before", channelUp)`. La de `cut.after ok=$it` sigue sin canal y con el mismo literal. `observe` y `restore` también siguen sin canal: no llevan `channel=`, pero sí `doubtful=adb_enabled(wifi)` cuando toca.

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*AccessDiagTest' --tests '*CutAccessTest' --tests '*NoHardcodedTextTest'`
Expected: PASS (`AccessDiagTest` 6, `CutAccessTest` 4, `NoHardcodedTextTest` 3). En la réplica, `NoHardcodedTextTest` no marcó ningún literal nuevo. Si lo marcara, añadir su línea a `i18n-exceptions.txt` ("línea de log para adb logcat, no se muestra") y decirlo en el informe.

Mutaciones:
- M14. Quitar la línea `doubt(…)?.let { … }` → fallan `theLineSaysWhenWirelessContradictsAZero` y `theLineSaysTheChannelWhenItIsKnown`.
- M15. `doubt(readings, false)` en `line` (ignorar el canal) → falla `theLineSaysTheChannelWhenItIsKnown`.
- M16. En `value`, `?.toIntOrNull() ?: 0` (un ajuste nunca escrito cuenta como 0) → falla `nothingContradictedNothingSaid` (caso `r(null, "1")`).

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/test/kotlin/tech/hybriq/adbuddy/control
git commit -m "feat(android): el registro ADBuddyAccess dice el canal y cuándo adb_enabled lee un 0 contradicho

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Comprobar el corte: el canal debe caer y nadie debe responder en 5555 — crítica, revisor opus

R22. Tras un `cut()` que dio `true`, se espera hasta unos 5 s a que caiga el canal que estaba vivo al tocar y a que nadie acepte conexiones en 127.0.0.1:5555 (adbd en modo tcpip, el puerto del modo remoto). Si las dos cosas pasan, sale `cut_done`. Si no, sale `cut_still_connected` con el botón a Opciones de desarrollador; en la notificación, como `toast`. El canal solo sirve aquí como prueba en contra (R20).

**Files:**
- Create: `control/CutConfirm.kt`, `control/CutConfirmTest.kt`
- Modify: `control/CutAccess.kt`, `ui/ChatRoot.kt`, `bridge/BridgeService.kt`, `res/values/strings.xml`, `res/values-es/strings.xml`
- Test: `control/CutAccessTest.kt` (`bothCutButtonsPassTheChannel` → `bothCutButtonsPassTheChannelAndCheckTheCut`), `ui/ChatRootCutWiringTest.kt`

**Interfaces:**
- Consumes: `CutAccess.channelUp`, `CutAccess.cut(context, channelUp)` (tarea 2), `AccessDiag.log` (tarea 3).
- Produces: `enum class CutCheck { DONE, STILL_CONNECTED }`, `CutConfirm.check(wasUp, channelUp: () -> Boolean, listening: () -> Boolean, waitMs, stepMs): CutCheck` (suspend), `CutConfirm.adbdListening(): Boolean`, `CutConfirm.WAIT_MS = 5_000L`, `CutAccess.confirm(context, wasUp, channelUp: () -> Boolean): CutCheck` (suspend, en `Dispatchers.IO`), `CutAccess.probe` y `CutAccess.waitMs` (`internal`, `@VisibleForTesting`), `R.string.cut_still_connected`.

- [ ] **Step 1: tests que fallan.**

(a) `control/CutConfirmTest.kt` (nuevo):

```kotlin
package tech.hybriq.adbuddy.control

import java.net.InetAddress
import java.net.ServerSocket
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Addendum 2026-10-19 R22: after a cut, the channel that was alive must fall and nothing may answer on 5555. */
class CutConfirmTest {
    private fun check(wasUp: Boolean, channelUp: () -> Boolean, listening: () -> Boolean) =
        runBlocking { CutConfirm.check(wasUp, channelUp, listening, waitMs = 100, stepMs = 10) }

    private fun answers(vararg values: Boolean): () -> Boolean {
        var i = 0
        return { values[minOf(i++, values.size - 1)] }
    }

    @Test fun aChannelThatFallsIsDone() {
        assertEquals(CutCheck.DONE, check(true, answers(true, true, false), { false }))
    }

    @Test fun aChannelThatStaysIsStillConnected() {
        assertEquals(CutCheck.STILL_CONNECTED, check(true, { true }, { false }))
    }

    @Test fun aPortThatStillAnswersIsStillConnected() {
        assertEquals(CutCheck.STILL_CONNECTED, check(false, { false }, { true }))
    }

    @Test fun aPortThatClosesIsDone() {
        assertEquals(CutCheck.DONE, check(false, { false }, answers(true, true, false)))
    }

    @Test fun aChannelThatWasNotUpIsNotWaitedFor() {
        // R20: the channel only speaks against a cut it was alive for.
        assertEquals(CutCheck.DONE, check(false, { true }, { false }))
    }

    @Test fun thePortProbeSeesAListener() {
        val server = try {
            ServerSocket(5555, 1, InetAddress.getByName("127.0.0.1"))
        } catch (e: java.io.IOException) {
            null // something already listens there (an emulator on this PC): that is a listener too
        }
        try {
            assertEquals(true, CutConfirm.adbdListening())
        } finally {
            server?.close()
        }
        assumeTrue("5555 still taken by someone else", server != null)
        assertFalse(CutConfirm.adbdListening())
    }
}
```

(b) En `CutAccessTest.kt`, sustituir `bothCutButtonsPassTheChannel` por:

```kotlin
    // The shield of the screen and Cut in the notification (spec 2026-10-18 R5, §8.7) tell cut() whether the channel is
    // alive (R20) and check the cut afterwards (R22).
    @Test fun bothCutButtonsPassTheChannelAndCheckTheCut() {
        val up = Regex("""val up = CutAccess\.channelUp\(app\.bridge\.state\.value\)""")
        val cut = Regex("""CutAccess\.cut\(\s*(context|this)\s*,\s*up\s*\)""")
        val confirm = Regex("""CutAccess\.confirm\(\s*(context|this@BridgeService)\s*,\s*up\s*\)\s*\{\s*CutAccess\.channelUp\(app\.bridge\.state\.value\)\s*\}""")
        val withoutChannel = Regex("""CutAccess\.cut\(\s*(context|this)\s*\)""")
        for (path in listOf("ui/ChatRoot.kt", "bridge/BridgeService.kt")) {
            val src = File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText()
            assertTrue("$path reads the channel", up.containsMatchIn(src))
            assertTrue("$path passes it to cut()", cut.containsMatchIn(src))
            assertTrue("$path checks the cut", confirm.containsMatchIn(src))
            assertTrue("$path says when the PC is still connected", src.contains("CutCheck.STILL_CONNECTED"))
            assertFalse("$path cuts without the channel", withoutChannel.containsMatchIn(src))
        }
    }
```

(c) En `ui/ChatRootCutWiringTest.kt`:
- Imports nuevos: `androidx.compose.ui.test.onAllNodesWithText`, `org.junit.After`, `org.junit.Before`, `tech.hybriq.adbuddy.control.CutConfirm`.
- Tras la línea `@get:Rule val rules …`:

```kotlin
    // The PC running the tests may have an emulator on 127.0.0.1:5555: the check after a cut (R22) must not see it.
    @Before fun noAdbdHere() {
        CutAccess.probe = { false }
    }

    @After fun realProbe() {
        CutAccess.probe = CutConfirm::adbdListening
        CutAccess.waitMs = CutConfirm.WAIT_MS
    }

    private fun waitForText(id: Int) = compose.waitUntil(5_000) { compose.onAllNodesWithText(text(id)).fetchSemanticsNodes().isNotEmpty() }
```

- En `theShieldCutsBothSettingsAndSaysSo` (ruling R22: el aviso llega tras la comprobación, fuera del hilo principal), antes de su última línea: `waitForText(R.string.cut_done) // after the check of R22, off the main thread`. Las aserciones del test no cambian.
- Test nuevo:

```kotlin
    @Test fun aCutThatLeavesAdbAnsweringSaysSoWithTheWayToDeveloperOptions() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        CutAccess.probe = { true } // adbd still answers on 5555: USB debugging did not go off
        CutAccess.waitMs = 50
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        waitForText(R.string.cut_still_connected)
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.action_developer_options)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.cut_done)).assertDoesNotExist()
    }
```

- [ ] **Step 2: comprobar que falla**

Run: `$G :app:testDebugUnitTest --tests '*CutConfirmTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest'`
Expected: FAIL de compilación (`CutConfirm`, `CutCheck`, `CutAccess.probe` y `R.string.cut_still_connected` no existen).

- [ ] **Step 3: implementación mínima.**

(a) `control/CutConfirm.kt`:

```kotlin
package tech.hybriq.adbuddy.control

import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket
import kotlinx.coroutines.delay

/** What is seen after a cut whose settings were written (addendum 2026-10-19, R22). */
enum class CutCheck { DONE, STILL_CONNECTED }

/**
 * The check after a cut (R22): on Android 17 the reading cannot prove adb_enabled=0 applied, so the cut is also
 * judged by what it must stop. A channel that was alive at the tap must fall, and nothing may answer on
 * 127.0.0.1:5555 (adbd in tcpip mode, the remote mode's port), both within [WAIT_MS]. The channel only speaks against
 * the cut here: it never undoes one (R20).
 */
object CutConfirm {
    const val WAIT_MS = 5_000L
    const val STEP_MS = 250L

    /** [wasUp]: the channel was alive at the tap; [channelUp] and [listening] are asked again on each step. Blocking: run off the main thread. */
    suspend fun check(wasUp: Boolean, channelUp: () -> Boolean, listening: () -> Boolean, waitMs: Long = WAIT_MS, stepMs: Long = STEP_MS): CutCheck {
        var waited = 0L
        while (true) {
            val connected = wasUp && channelUp()
            if (!connected && !listening()) return CutCheck.DONE
            if (waited >= waitMs) return CutCheck.STILL_CONNECTED
            delay(stepMs)
            waited += stepMs
        }
    }

    /** Whether something accepts connections on 127.0.0.1:5555, as CutAccessOnEmulatorTest.listening(). Never on the main thread. */
    fun adbdListening(): Boolean = try {
        Socket().use { it.connect(InetSocketAddress("127.0.0.1", 5555), 500) }
        true
    } catch (e: IOException) {
        false
    }
}
```

(b) `control/CutAccess.kt`: imports `androidx.annotation.VisibleForTesting`, `kotlinx.coroutines.Dispatchers` y `kotlinx.coroutines.withContext`, y, tras `cut`:

```kotlin
    /** The port check of confirm(); tests on a PC with an emulator on 5555 replace it. */
    @VisibleForTesting internal var probe: () -> Boolean = CutConfirm::adbdListening

    @VisibleForTesting internal var waitMs: Long = CutConfirm.WAIT_MS

    /**
     * After a cut() that said true (R22): DONE once the channel that was alive at the tap ([wasUp]) has fallen and
     * nothing answers on 127.0.0.1:5555; STILL_CONNECTED after waitMs otherwise (the screen says to turn USB debugging
     * off by hand). [channelUp] reads the bridge now. Runs on the IO dispatcher.
     */
    suspend fun confirm(context: Context, wasUp: Boolean, channelUp: () -> Boolean): CutCheck = withContext(Dispatchers.IO) {
        CutConfirm.check(wasUp, channelUp, probe, waitMs).also { AccessDiag.log(context, "cut.check=$it") }
    }
```

(c) Textos (antes de `cut_done` en cada fichero):

```xml
    <string name="cut_still_connected">The PC is still connected: turn off USB debugging in Developer options.</string>
```

```xml
    <string name="cut_still_connected">El PC sigue conectado: apaga la depuración USB en Opciones de desarrollador.</string>
```

(d) `ui/ChatRoot.kt`:
- Import `tech.hybriq.adbuddy.control.CutCheck`.
- `val cutStillConnected = stringResource(R.string.cut_still_connected)` junto a `restoreManual`.
- `report` queda así:

```kotlin
    fun report(ok: Boolean, restoring: Boolean, stillConnected: Boolean = false) {
        val message = when {
            stillConnected -> cutStillConnected
            !ok && restoring && CutAccess.needsManualRestore(context) -> restoreManual
            !ok && restoring -> restoreFailed
            !ok -> cutFailed
            restoring -> cutRestored
            else -> cutDone
        }
        scope.launch {
            // Without the permission (or if the change did not stick), the person does it by hand.
            val r = snackbar.notify(message, actionLabel = if (ok && !stillConnected) null else developerOptions)
            if (r == SnackbarResult.ActionPerformed) openDeveloperOptions(context)
        }
    }
```

- y `cut` en `ChatActions`:

```kotlin
            cut = {
                val up = CutAccess.channelUp(app.bridge.state.value)
                if (!CutAccess.cut(context, up)) {
                    report(ok = false, restoring = false)
                } else {
                    scope.launch {
                        // R22: the channel and port 5555 say whether the cut really closed adb.
                        val check = CutAccess.confirm(context, up) { CutAccess.channelUp(app.bridge.state.value) }
                        report(ok = true, restoring = false, stillConnected = check == CutCheck.STILL_CONNECTED)
                    }
                }
            },
```

(e) `bridge/BridgeService.kt`: import `tech.hybriq.adbuddy.control.CutCheck` y:

```kotlin
            ACTION_CUT -> {
                val up = CutAccess.channelUp(app.bridge.state.value)
                if (!CutAccess.cut(this, up)) {
                    toast(R.string.notif_cut_failed)
                } else {
                    lifecycleScope.launch {
                        // R22: the channel and port 5555 say whether the cut really closed adb.
                        val check = CutAccess.confirm(this@BridgeService, up) { CutAccess.channelUp(app.bridge.state.value) }
                        if (check == CutCheck.STILL_CONNECTED) toast(R.string.cut_still_connected)
                    }
                }
            }
```

- [ ] **Step 4: comprobar que pasa**

Run: `$G :app:testDebugUnitTest --tests '*CutConfirmTest' --tests '*CutAccessTest' --tests '*ChatRootCutWiringTest' --tests '*AccessSwitchTest' --tests '*AccessDiagTest' --tests '*NoHardcodedTextTest' --tests '*StringsParityTest'`; luego `$G :app:assembleDebug :app:lintDebug`.
Expected: PASS (`CutConfirmTest` 6, `CutAccessTest` 4, `ChatRootCutWiringTest` 5, `AccessSwitchTest` 34, `AccessDiagTest` 6, `NoHardcodedTextTest` 3, `StringsParityTest` 4) y BUILD SUCCESSFUL.

Mutaciones:
- M17 ✓. En `check`, `val connected = false` (no esperar al canal) → falla `aChannelThatStaysIsStillConnected`.
- M18 ✓. En `check`, `if (!connected) return CutCheck.DONE` (sin sondeo) → falla `aPortThatStillAnswersIsStillConnected`.
- M19. En `check`, sondear una sola vez (`return if (listening()) STILL_CONNECTED else DONE` tras el canal) → falla `aPortThatClosesIsDone`.
- M20. En `check`, `val connected = channelUp()` (sin `wasUp`) → falla `aChannelThatWasNotUpIsNotWaitedFor`.
- M21 ✓. En `ChatRoot`, `stillConnected = false` → falla `aCutThatLeavesAdbAnsweringSaysSoWithTheWayToDeveloperOptions`.
- M22. En `BridgeService`, quitar el `lifecycleScope.launch { … }` → falla `bothCutButtonsPassTheChannelAndCheckTheCut`.
- M23. `adbdListening()` siempre `false` → falla `thePortProbeSeesAListener`.

- [ ] **Step 5: commit**

```bash
git add android/app/src/main android/app/src/test
git commit -m "fix(android): tras cortar, el canal debe caer y nadie debe responder en 5555; si no, se avisa

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Cierre: batería de la CI, README e inventario — revisor adbuddy-reviewer

**Files:**
- Modify: `android/README.md` (párrafo "Cut access state"), `docs/superpowers/plans/pendientes-m3-m5.md` (sección UX1), `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (filas UX1-1 y UX1-2)

- [ ] **Step 1:** `$G --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (en segundo plano, timeout 20 min) → BUILD SUCCESSFUL. Las capturas no cambian, porque no se toca la UI visible salvo un texto de snackbar: `git status --short android/app/src/test/screenshots` → limpio.
- [ ] **Step 2:** en `android/README.md`, tras el párrafo **Cut access state**, añadir:

```markdown
**False readings (Android 17).** An app reads `adb_enabled=0` there while adb works; `adb_wifi_enabled` reads right.
ADBuddy believes a 1 always and a 0 only when nothing contradicts it: with wireless debugging at 1 or a live channel
with the PC, it saves USB debugging as on before a cut (marked `adb_enabled_doubted` in the record) and restores it so.
A cut is confirmed by `putInt` and by wireless debugging reading 0, and then checked: the channel must fall and nothing
may answer on 127.0.0.1:5555 within 5 s, or the app says the PC is still connected. The log line ends with
`doubtful=adb_enabled(…)` when a 0 was not believed.
```

- [ ] **Step 3:** en `2026-10-03-validaciones-manuales.md`:
  - UX1-1 pasa a **hecha**: el 2026-10-19 la app leyó `raw:0` en `adb_enabled` mientras el shell decía `1`, y `adb_wifi_enabled` coincidía (adenda `2026-10-19-acceso-lecturas-falsas.md`).
  - UX1-2 pasa a decir "V2 rehecha de la adenda (§5), condición para integrar en main", y su prerrequisito pasa de "UX1-1" a "build de `ux/acceso-a17` instalada (permiso)".
  - En la lista de pasos del usuario (línea "Estado del acceso y aspecto visual"), cambiar la build a `ux/acceso-a17` y quitar la lectura de UX1-1.
- [ ] **Step 4:** en `pendientes-m3-m5.md`, sección UX1, añadir esta fila: `UX-f | En Android 17 la lectura no confirma que adb_enabled=0/1 se aplique | Adenda 2026-10-19 R18-R19 y R22 (comprobación por canal y 5555, que no ve una USB encendida sin modo tcpip). Lo mide V2 (UX1-2), condición para integrar.`
- [ ] **Step 5: commit** `docs: cierre de la adenda de lecturas falsas de adb_enabled` (con el trailer).

---

## Validaciones manuales pendientes (solo lo que exige el móvil real; siempre con permiso explícito del usuario)

**V2 es condición para integrar la rama en `main`.**

- **V2 (rehecha; spec §5).** Con la build de este plan instalada (instalar es un cambio en el móvil: pedir permiso):
  1. Antes: `adb shell settings get global adb_enabled` y `adb shell settings get global adb_wifi_enabled` (esperado `1` y `1`), y `adb logcat -c`. Apuntar si se usó el modo remoto (`adb tcpip`) y la IP del móvil.
  2. Cortar desde el escudo de la app con el chat conectado. Esperado: el PC pierde la conexión (`adb devices` sin el móvil u `offline`; `adbuddy serve` lo dice), la app dice "Acceso cortado" y, en unos segundos, "Hecho. El PC ya no puede tocar tu móvil." (no "El PC sigue conectado…").
  3. En el móvil, Opciones de desarrollador: el interruptor "Depuración por USB" y "Depuración inalámbrica" deben estar apagados. Si la USB sigue encendida, R18 se queda corto: apuntarlo y abrir un plan.
  4. Si hubo modo remoto: `adb connect <ip>:5555` desde el PC debe fallar. Con cable USB, si lo hay: `adb -d devices` no debe listar el móvil.
  5. Reactivar desde la app (escudo tachado, diálogo y confirmar): "El PC vuelve a tener acceso" y el PC reconecta (el puerto inalámbrico puede cambiar).
  6. `adb shell settings get global adb_enabled` / `adb_wifi_enabled` → `1` / `1`. Y `adb logcat -d -s ADBuddyAccess`, en este orden:
     - `cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 saved.adb_enabled=- saved.adb_wifi_enabled=- saved.adb_enabled_doubted=- channel=up doubtful=adb_enabled(wifi+channel)`
     - `cut.after ok=true sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:0 saved.adb_enabled=1 saved.adb_wifi_enabled=1 saved.adb_enabled_doubted=1`
     - `cut.check=DONE sdk=37 …`
     - `restore.before …`
     - `restore.after ok=true sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 saved.adb_enabled=- saved.adb_wifi_enabled=- saved.adb_enabled_doubted=- doubtful=adb_enabled(wifi)`
  7. Si reactivar falla, el usuario enciende la depuración inalámbrica (y la USB, si la tenía) en Opciones de desarrollador. Apuntar la línea `restore.after`.
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+).

## Autorrevisión

- Cobertura de la adenda:

| Regla | Tareas | Tests o mutaciones |
|---|---|---|
| R16 | 1 (inalámbrica) y 2 (canal) | |
| R17 | 1 | `onlyAContradictedZeroIsDoubted`, `aZeroWithNothingAgainstItIsBelieved`, `withoutAContradictionAZeroAfterRestoringIsBelieved` |
| R18 | 1 | `put` = false, M1; `aWriteThatDidNotStickIsNotDone` sin tocar |
| R19 | 1 y 2 | marca `ADB_DOUBTED`, `aDoubtRecordedAtTheCutLetsTheRestorePass`, `aZeroNobodyDoubtedLeavesNoMark` |
| R20 | 2 | `aLiveChannelNeverUndoesACut`, `theChannelIsUpOnlyWhenTheHostIsConnected` |
| R21 | 3 | |
| R22 | 4 | |
| R23 | 1 | `anOldRecordOfOnlyWirelessRestoresUsbToo` |
| V2-V3 | 5 y sección de arriba | |

- Garantías R1-R5: `isCut`, `readAsOn`, `forgetIfRestored`, `restore` sin registro y `needsManualRestore` no cambian. Cortar sigue siendo inmediato y sin confirmación (la comprobación solo cambia el aviso). Reactivar sigue pasando por el diálogo.
- Cada tarea compila sola: 1 (`AccessSwitch` y una línea de `CutAccess`); 2 (usa 1; `CutAccess.cut(context)` sigue valiendo por el valor por defecto); 3 (usa 1 y 2); 4 (usa 2 y 3); 5 (docs).
- Réplica: el estado final de las tareas 1-4 se aplicó entero sobre `ux/acceso-a17` antes de escribir este plan, y después se deshizo. Resultados en verde: `AccessSwitchTest` 34/34, `CutAccessTest` 4/4, `CutConfirmTest` 6/6, `AccessDiagTest` 6/6, `ChatRootCutWiringTest` 5/5, `NoHardcodedTextTest` 3/3, `StringsParityTest` 4/4, `LinkStatusTest` 9/9 y `ConversationViewTest` 21/21. Las mutaciones marcadas con ✓ se comprobaron: fallan los tests indicados. Quedan sin réplica las ejecuciones intermedias de cada tarea por separado, `lintDebug` y `assembleDebugAndroidTest`.
- Nombres: `AccessSwitch.adbZeroIsDoubtful`, `AccessSwitch.ADB_DOUBTED`, `AccessSwitch(…, channelUp)`, `CutAccess.channelUp(state)`, `CutAccess.cut(context, channelUp)`, `CutAccess.confirm`, `CutAccess.probe`, `CutAccess.waitMs`, `CutConfirm.check`, `CutConfirm.adbdListening`, `CutCheck`, `AccessDiag.line(…, channelUp)`, `AccessDiag.doubt`, `AccessDiag.log(…, channelUp)`, `FakeSettings.readsZero`, `FakeSettings.refuses`, `R.string.cut_still_connected`.
