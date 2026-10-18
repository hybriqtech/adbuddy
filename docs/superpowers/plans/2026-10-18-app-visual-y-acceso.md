# App Android: aspecto visual y estado del acceso. Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que "Acceso cortado" solo salga cuando ADBuddy ha cortado de verdad y no hay canal, que la pantalla principal se entienda de un vistazo (pastilla de estado, una tarjeta de aviso, Cortar siempre visible) y que la app tenga identidad propia (paleta completa, icono, monoespaciada), corrigiendo los bugs de la auditoría.

**Architecture:** primero la lógica de acceso (pura, en `control/` y `ui/LinkStatus.kt`, con tests JVM), con un registro de diagnóstico para el caso abierto del Pixel; luego el tema (tokens en `ui/theme/`), el icono (recursos vectoriales), y por último las pantallas, que consumen tokens y componentes nuevos (`StatusPill`, `ActionCard`, `HomeNotice`). Cada tarea de UI regraba y revisa sus capturas Roborazzi.

**Tech Stack:** Kotlin, Jetpack Compose Material 3 (BOM del catálogo), Robolectric + Roborazzi (tests JVM y capturas), Gradle 9.8 sin wrapper (como la CI).

**Spec:** `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md` (leerla entera antes de cada tarea; §8 son las protecciones que no se pueden romper).

## Global Constraints

- `minSdk = 30`, `compileSdk = targetSdk = 37` (`android/app/build.gradle.kts`): nada de APIs de 31+ sin comprobación de `Build.VERSION.SDK_INT`.
- Sin dependencias nuevas ni ficheros binarios de fuentes (R7): monoespaciada = `FontFamily.Monospace`.
- Todo texto visible en `res/values/strings.xml` (en) **y** `res/values-es/strings.xml` (es), con los mismos marcadores; `NoHardcodedTextTest` y `StringsParityTest` en verde.
- Ningún test de la lista de §8 de la spec se borra ni se debilita. Si hace falta tocar uno, la tarea se para y lo adjudica el controlador.
- Filtro de toques tapados (spec §8.5): `ObscuredTouchGuard()` solo dentro de diálogos y menús (ventanas propias); en una pantalla, `Modifier.refuseObscuredTouches()` solo en el botón que concede algo (p. ej. el de `ActionCard`). Nunca `ObscuredTouchGuard()` en un composable de pantalla entera (Ajustes, chat vacío): marcaría toda la ventana de la actividad. El escudo de Cortar va **sin** filtro: un corte provocado por un toque tapado es inofensivo; reactivar va por un diálogo con su guarda.
- Cada test crítico lleva su **mutación**: el cambio de código que debe hacerlo fallar. El implementador la aplica a mano, ve el fallo y la deshace, y lo dice en su informe.
- Colores solo desde `MaterialTheme.colorScheme` o `AdbuddyTheme.colors` (tarea 5); el único sitio con valores ARGB es `ui/theme/Palette.kt` (y `a11y/OverlayColors.kt`, tarea 4, que no tiene Compose).
- Nunca se ejecuta nada en el móvil del usuario. Los tests usan JVM/Robolectric; lo instrumentado, el emulador (`android/README.md`), nunca tareas `connected*` de Gradle.
- Git: identidad local hybriqtech del repo, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, sin push.

## Comandos (los reales: `android/README.md` y `.github/workflows/android.yml`)

Todos desde `android/` del worktree (`C:\Users\Jose\orca\workspaces\adbuddy\ux-visual\android`), con `gradle` del PATH (no hay wrapper; la CI usa Gradle 9.8.0):

- Un test: `gradle :app:testDebugUnitTest --tests '*AccessSwitchTest'`
- Todo lo de la CI: `gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (timeout amplio: 20 min; en segundo plano si hace falta).
- Regrabar capturas (solo en Windows, R14): `gradle :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.screens.*' -Padbuddy.screens=record`; luego verificar sin la propiedad (en Windows el modo por defecto es `verify` y falla si algo cambia): `gradle :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.screens.*'`.
- Evidencia visual de cada tarea de UI: lista de PNG cambiados (`git status --short app/src/test/screenshots`), y el implementador **mira** al menos los `_light`, `_dark`, `_font_200` y `_font_200_es` de las pantallas que toca (herramienta Read sobre el PNG) y lo dice en su informe.

## Revisores

| Tareas | Revisor | Por qué |
|---|---|---|
| 1, 2, 3, 4, 7, 8, 10, 11 | `adbuddy-reviewer-opus` | acceso/cortar, notificación, superposición, tarjeta de aviso con preguntas 🔴, cabecera con cortar/reactivar, tarjeta de pregunta y diálogo 🔴 |
| 5, 6, 9, 12, 13, 14 | `adbuddy-reviewer` | tema, icono, estado vacío y burbujas, ajustes, actividad y cierre |

Tiempo para los revisores: `gradle ... --tests` concretos, máximo 25 min de ejecución por revisión.

## Mapa de ficheros

| Fichero | Tarea | Qué |
|---|---|---|
| `control/AccessDiag.kt` (nuevo) | 1 | línea de diagnóstico pura + `Log.i("ADBuddyAccess", …)` |
| `control/CutAccess.kt` | 1, 2 | registra lecturas; delega `isCut` en `AccessSwitch`; deja pasar `SecurityException` |
| `control/AccessSwitch.kt` | 2 | regla nueva: registro propio + lecturas legibles |
| `ui/LinkStatus.kt` | 3 | el canal vivo gana a `cut` |
| `notify/ConversationView.kt`, `bridge/BridgeService.kt`, `ui/ChatRoot.kt` | 2, 3 | llamadas nuevas a `CutAccess.isCut(context)` |
| `a11y/OverlayColors.kt` (nuevo), `a11y/Overlay.kt` | 4 | colores fijos de la superposición |
| `ui/theme/Palette.kt`, `Theme.kt`, `AdbuddyColors.kt` (nuevo), `Type.kt` (nuevo), `Shapes.kt` (nuevo) | 5 | tokens completos |
| `res/drawable/*`, `res/mipmap-anydpi/*`, `res/values/colors.xml`, `AndroidManifest.xml`, notifiers | 6 | icono |
| `ui/HomeNotice.kt`, `ui/ActionCard.kt`, `ui/StatusPill.kt` (nuevos) | 7 | componentes de la pantalla principal |
| `ui/ChatParts.kt` (`ChatHeader`), `ui/ChatScreen.kt`, `ui/ChatRoot.kt`, `ui/RestoreAccessDialog.kt` (nuevo) | 8 | cabecera, tarjeta única, modo remoto fuera |
| `ui/ChatEmptyState.kt`, `ui/ChatParts.kt` (`Bubble`) | 9 | estado vacío y burbujas |
| `ui/PermissionCard.kt` | 10 | nombre del agente, monoespaciada |
| `ui/Screens.kt` (`ConfirmDialog`) | 11 | comando en bloque monoespaciado |
| `ui/SettingsScreen.kt`, `ui/theme/ThemePrefs.kt` (nuevo), `MainActivity.kt`, `ChatBubbleActivity.kt`, `ui/RemoteCard.kt` | 12 | ajustes |
| `ui/ActivityScreen.kt`, `ui/ChatParts.kt` (`DraftChips`) | 13 | diario y chips |
| `docs/superpowers/plans/pendientes-m3-m5.md`, `android/README.md` | 14 | cierre |

Rutas de Kotlin relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` y de tests a `android/app/src/test/kotlin/tech/hybriq/adbuddy/`.

---

### Task 1: Diagnóstico del acceso (registro `ADBuddyAccess`) — crítica, revisor opus

El Pixel del usuario (Android 17, app 0.2.0) tiene `adb_enabled=1` y `adb_wifi_enabled=1` según `adb shell` y aun así la app dice "cortado". Hoy `CutAccess.isCut` hace `Settings.Global.getInt(resolver, ADB_ENABLED, 1) == 0`, así que **la app lee 0** (o algo que no es 1). La causa está abierta: esta tarea solo hace que la app diga qué lee y cómo, sin cambiar ningún comportamiento.

**Files:**
- Create: `control/AccessDiag.kt`
- Modify: `control/CutAccess.kt`, `android/app/src/test/resources/i18n-exceptions.txt`
- Test: `control/AccessDiagTest.kt`

**Interfaces:**
- Produces: `AccessDiag.Reading(name: String, raw: String?, error: String?)`, `AccessDiag.line(event: String, sdk: Int, readings: List<Reading>, saved: Map<String, Int?>): String`, `AccessDiag.read(resolver: ContentResolver, name: String): Reading`, `AccessDiag.log(context: Context, event: String)`, `const val TAG = "ADBuddyAccess"`.

- [ ] **Step 1: test que falla**

```kotlin
package tech.hybriq.adbuddy.control

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class AccessDiagTest {
    @Test fun theLineSaysWhatWasReadAndHow() {
        val line = AccessDiag.line(
            "observe", 37,
            listOf(AccessDiag.Reading("adb_enabled", "0", null), AccessDiag.Reading("adb_wifi_enabled", null, "SecurityException")),
            mapOf("adb_enabled" to null, "adb_wifi_enabled" to 1),
        )
        assertEquals(
            "observe sdk=37 adb_enabled=raw:0 adb_wifi_enabled=error:SecurityException saved.adb_enabled=- saved.adb_wifi_enabled=1",
            line,
        )
    }

    @Test fun aNeverWrittenSettingIsNull_notZero() {
        val line = AccessDiag.line("cut", 30, listOf(AccessDiag.Reading("adb_wifi_enabled", null, null)), emptyMap())
        assertEquals("cut sdk=30 adb_wifi_enabled=null", line)
    }

    @Test fun theLineCarriesNoTextFromOutside() {
        // A raw value is cut to 16 chars and stripped of line breaks: the log is not a place for anything else.
        val line = AccessDiag.line("x", 37, listOf(AccessDiag.Reading("adb_enabled", "1\n" + "9".repeat(40), null)), emptyMap())
        assertFalse(line.contains("\n"))
        assertEquals("x sdk=37 adb_enabled=raw:1 99999999999999", line)
    }
}
```

- [ ] **Step 2: comprobar que falla**

Run: `gradle :app:testDebugUnitTest --tests '*AccessDiagTest'`
Expected: FAIL de compilación (`Unresolved reference: AccessDiag`).

- [ ] **Step 3: implementación**

`control/AccessDiag.kt`:

```kotlin
package tech.hybriq.adbuddy.control

import android.content.ContentResolver
import android.content.Context
import android.os.Build
import android.provider.Settings
import android.util.Log

/**
 * What the app reads of the two debugging settings, and how (spec 2026-10-18 §3, diagnóstico abierto). On the user's
 * Pixel (Android 17) `adb shell` says adb_enabled=1 and the app showed "cut": `adb logcat -s ADBuddyAccess` says what
 * the app really gets. Only numbers and exception class names are logged: no PC name, no command, nothing from the host.
 */
object AccessDiag {
    const val TAG = "ADBuddyAccess"

    /** raw: Settings.Global.getString as is (null = never written); error: the exception's class name when it threw. */
    data class Reading(val name: String, val raw: String?, val error: String?)

    fun line(event: String, sdk: Int, readings: List<Reading>, saved: Map<String, Int?>): String = buildString {
        append(event).append(" sdk=").append(sdk)
        for (r in readings) {
            append(' ').append(r.name).append('=')
            append(
                when {
                    r.error != null -> "error:" + r.error
                    r.raw == null -> "null"
                    else -> "raw:" + r.raw.replace(Regex("[\\r\\n]+"), " ").take(16)
                },
            )
        }
        for ((name, value) in saved) append(" saved.").append(name).append('=').append(value?.toString() ?: "-")
    }

    fun read(resolver: ContentResolver, name: String): Reading = try {
        Reading(name, Settings.Global.getString(resolver, name), null)
    } catch (e: SecurityException) {
        Reading(name, null, e.javaClass.simpleName)
    }

    /** Logs one line; never throws (a diagnosis must not break the screen or the service). */
    fun log(context: Context, event: String) {
        try {
            val resolver = context.contentResolver
            val prefs = context.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE)
            val names = listOf(AccessSwitch.ADB_ENABLED, AccessSwitch.ADB_WIFI_ENABLED)
            val saved = names.associateWith { if (prefs.contains(it)) prefs.getInt(it, 0) else null }
            Log.i(TAG, line(event, Build.VERSION.SDK_INT, names.map { read(resolver, it) }, saved))
        } catch (e: RuntimeException) {
            Log.w(TAG, "diagnosis failed: " + e.javaClass.simpleName)
        }
    }
}
```

En `control/CutAccess.kt`: añadir `const val PREFS = "cut_access"` (una sola constante; `switch()` pasa a usar `context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)`); dentro de `observe`, en `current()`, primera línea `AccessDiag.log(context, "observe")`; en `cut(context)` y `restore(context)`, registrar antes y después:

```kotlin
    fun cut(context: Context): Boolean {
        AccessDiag.log(context, "cut.before")
        return switch(context).cut().also { AccessDiag.log(context, "cut.after ok=$it") }
    }

    fun restore(context: Context): Boolean {
        AccessDiag.log(context, "restore.before")
        return switch(context).restore().also { AccessDiag.log(context, "restore.after ok=$it") }
    }
```

Los eventos y textos de log (`"cut.before"`, `"cut.after ok=$it"`, `"diagnosis failed: "`, `" sdk="`…) no son texto para una persona: ejecutar `NoHardcodedTextTest` y añadir a `android/app/src/test/resources/i18n-exceptions.txt`, con su formato `fichero (desde tech/hybriq/adbuddy/) | literal | motivo`, **exactamente** los literales que marque, con el motivo "línea de log para adb logcat, no se muestra" (o `control/AccessDiag.kt | * | líneas de log para adb logcat, no se muestran` si marca varios de ese fichero). No añadir líneas que no marque: el test falla si una línea no coincide con nada.

- [ ] **Step 4: comprobar que pasa**

Run: `gradle :app:testDebugUnitTest --tests '*AccessDiagTest' --tests '*NoHardcodedTextTest' --tests '*AccessSwitchTest'`
Expected: PASS.

- [ ] **Step 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/control android/app/src/test/kotlin/tech/hybriq/adbuddy/control android/app/src/test/resources/i18n-exceptions.txt
git commit -m "feat(android): registro ADBuddyAccess con lo que la app lee de los ajustes de depuración

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

La lectura en el Pixel queda en "Validaciones manuales pendientes" (V1). El plan **no espera** a ese resultado: la tarea 2 está diseñada para ser correcta tanto si la lectura es fiable como si Android 17 devuelve 0 o un valor falso a la app.

---

### Task 2: "Cortado" = lo cortó ADBuddy y las lecturas no lo desmienten — crítica, revisor opus

Regla (spec R1-R4 tal como quedan tras el dato del Pixel):

- **`isCut()` es verdadero solo si hay registro de un corte de ADBuddy** (valores guardados, que solo quedan si el corte se confirmó leyendo) **y las lecturas no lo desmienten.** Desmentir = una lectura **legible** dice que alguno de los dos está a 1 (entonces se olvida el registro: alguien volvió a dar acceso a mano). Una lectura ilegible no desmiente nada y tampoco crea un "cortado".
- Sin registro **nunca** se dice "cortado", lea lo que lea (cubre el Pixel: si Android 17 da 0 a la app, ya no sale "cortado"). Un corte hecho fuera de ADBuddy se ve como "Buscando tu PC…", cuya tarjeta dice que encienda la depuración inalámbrica.
- `cut()` guarda los dos valores siempre que no haya registro, escribe los dos a 0 y relee; si no se confirma, **borra lo que guardó en esta llamada** (no deja un registro falso).
- `restore()` escribe lo guardado tal cual (también `adb_enabled=0`). Sin registro: si una lectura legible ya dice que hay acceso, **no escribe nada** (nunca apaga la inalámbrica que estaba encendida); si no, escribe solo `adb_enabled=1` y no toca la inalámbrica (su valor no se conoce o ya es 0). Con un registro de "los dos a 0" (lecturas falsas al cortar, o un corte con todo apagado) **no adivina**: no escribe, conserva el registro y `needsManualRestore()` es verdadero; la pantalla manda a Opciones de desarrollador.
- Límite conocido (spec, diagnóstico abierto): si Android 17 da lecturas falsas a la app, esta tarea evita el falso "cortado", pero cortar y reactivar no se pueden confirmar de verdad. No se afirma que la tarea sea correcta en ese caso: lo decide V1.
- La relectura tras escribir no da por bueno un valor ilegible.

**Files:**
- Modify: `control/AccessSwitch.kt` (entero), `control/CutAccess.kt`, `bridge/BridgeService.kt:73`, `ui/ChatRoot.kt:67-68,121`, `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt:36,51` (`CutAccess.isCut(resolver)` → `CutAccess.isCut(context)`; `context` ya existe en ese test)
- Test: `control/AccessSwitchTest.kt`, `control/CutAccessTest.kt` (nuevo, Robolectric)

**Interfaces:**
- Consumes: `AccessDiag.log` (tarea 1).
- Produces: `AccessSwitch.Settings.get(name)` **lanza `SecurityException`** cuando no se puede leer (antes devolvía null); `AccessSwitch.isCut(): Boolean`; `AccessSwitch.restore(): Boolean`; `AccessSwitch.needsManualRestore(): Boolean`; `CutAccess.PREFS`; `CutAccess.isCut(context: Context): Boolean` (sustituye a `isCut(resolver)`); `CutAccess.needsManualRestore(context: Context): Boolean`; `CutAccess.observe(context): Flow<Boolean>` emite `switch.isCut()`.

- [ ] **Step 1: tests que fallan.** En `AccessSwitchTest.kt`, añadir a `FakeSettings` un conjunto de ajustes ilegibles y los tests nuevos; cambiar los existentes que asumen la regla vieja como se indica:

```kotlin
    private class FakeSettings(vararg values: Pair<String, Int>) : AccessSwitch.Settings {
        val values = mutableMapOf(*values)
        var allowed = true
        var ignoreWrites = emptySet<String>() // written without error, but the value does not change
        var unreadable = emptySet<String>() // reading throws, as Android 12+ may do for a hidden setting
        override fun get(name: String): Int? {
            if (name in unreadable) throw SecurityException("cannot read $name")
            return values[name]
        }
        override fun put(name: String, value: Int) {
            if (!allowed) throw SecurityException("no WRITE_SECURE_SETTINGS")
            if (name !in ignoreWrites) values[name] = value
        }
    }
```

Tests nuevos:

```kotlin
    @Test fun onlyWirelessDebuggingOnIsNotCut() {
        val sw = AccessSwitch(FakeSettings(adb to 0, wifi to 1), FakeSaved())
        assertFalse(sw.isCut())
    }

    @Test fun withoutACutOfOursNothingIsCut_whateverIsRead() {
        // The Pixel case (Android 17): the app may read 0 for both while adb works. Without our own record it is not cut.
        assertFalse(AccessSwitch(FakeSettings(adb to 0, wifi to 0), FakeSaved()).isCut())
        assertFalse(AccessSwitch(FakeSettings(), FakeSaved()).isCut())
    }

    @Test fun unreadableWithoutARecordIsNotCut() {
        val s = FakeSettings(adb to 0, wifi to 0).apply { unreadable = setOf(wifi) }
        assertFalse(AccessSwitch(s, FakeSaved()).isCut())
    }

    @Test fun cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals(0, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertTrue(sw.isCut())
        assertTrue(sw.restore())
        assertEquals("USB debugging was off: it stays off", 0, s.values[adb])
        assertEquals("wireless debugging comes back", 1, s.values[wifi])
        assertFalse(sw.isCut())
    }

    @Test fun aFailedCutLeavesNoRecord() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { ignoreWrites = setOf(wifi) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertFalse(sw.cut())
        assertTrue("a cut that did not stick must not look like one", saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun aReadBackThatCannotBeReadIsNotDone() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        s.unreadable = setOf(wifi)
        assertFalse(sw.cut())
        assertTrue(saved.values.isEmpty())
    }

    @Test fun unreadableAfterOurCutStaysCut_andKeepsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.unreadable = setOf(wifi)
        sw.forgetIfRestored()
        assertTrue(sw.isCut())
        assertEquals(1, saved.values[wifi])
    }

    @Test fun aRecordOfBothOffIsNeverGuessed() {
        val s = FakeSettings(adb to 0, wifi to 0)
        val saved = FakeSaved().apply { values[adb] = 0; values[wifi] = 0 }
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.needsManualRestore())
        assertFalse(sw.restore())
        assertEquals("nothing written", 0, s.values[adb])
        assertEquals("nothing written", 0, s.values[wifi])
        assertEquals("the record stays", 0, saved.values[wifi])
    }

    @Test fun restoringTwiceWithOnlyWirelessNeverTurnsItOff() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val sw = AccessSwitch(s, FakeSaved())
        assertTrue(sw.restore())
        assertTrue(sw.restore())
        assertEquals("wireless debugging stays on", 1, s.values[wifi])
        assertEquals("nothing turned on that was off", 0, s.values[adb])
    }

    @Test fun turningOnlyWirelessBackOnByHandForgetsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.values[wifi] = 1 // the person turns wireless debugging on in Settings (USB stays off)
        sw.forgetIfRestored()
        assertTrue(saved.values.isEmpty())
        assertFalse(sw.isCut())
    }
```

Mutaciones (cada una debe hacer fallar el test indicado; aplicarla, ver el fallo, deshacerla):
- quitar `hasRecord() &&` de `isCut()` → `withoutACutOfOursNothingIsCut_whateverIsRead`;
- quitar `forget()` del corte fallido → `aFailedCutLeavesNoRecord`;
- volver a `saved.get(ADB_ENABLED)?.takeIf { it != 0 } ?: 1` en `restore()` → `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere`;
- quitar `if (readAsOn()) return true` → `restoringTwiceWithOnlyWirelessNeverTurnsItOff`;
- dejar `CutAccess.isCut` con `getInt(ADB_ENABLED, 1) == 0` → `CutAccessTest`.

`control/CutAccessTest.kt` (Robolectric, sobre el `Settings.Global` y las preferencias reales de la app):

```kotlin
package tech.hybriq.adbuddy.control

import android.provider.Settings
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment

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
}
```

Cambios en tests existentes (la regla vieja ya no vale; ninguno de §8 de la spec pierde aserciones):
- `restoringWithNothingSavedTurnsOnlyAdbOn`, `cuttingTurnsBothOff`, `withoutThePermissionNothingIsCut`, `aWriteThatDidNotStickIsNotDone`, `restoringPutsBackWhatWasThere`, `restoringWithWirelessDebuggingOnTurnsItBackOn`, `aSettingNeverWrittenCountsAsOff`, `cuttingTwiceKeepsTheFirstValues`, `aFailedRestoreKeepsTheSavedValues`: **sin cambios** (deben seguir pasando).
- `reactivatingByHandForgetsTheSavedValues`: sin cambios (el primer `forgetIfRestored` con los dos a 0 conserva; con `adb=1` olvida).

- [ ] **Step 2: comprobar que fallan**

Run: `gradle :app:testDebugUnitTest --tests '*AccessSwitchTest'`
Expected: compilación rota por `needsManualRestore` (aún no existe). Para ver los fallos por el motivo esperado, comentar temporalmente `aRecordOfBothOffIsNeverGuessed` y ejecutar: deben fallar `onlyWirelessDebuggingOnIsNotCut`, `withoutACutOfOursNothingIsCut_whateverIsRead`, `unreadableWithoutARecordIsNotCut`, `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere`, `aFailedCutLeavesNoRecord`, `aReadBackThatCannotBeReadIsNotDone` (con `SecurityException` sin capturar), `restoringTwiceWithOnlyWirelessNeverTurnsItOff` y `turningOnlyWirelessBackOnByHandForgetsTheRecord`; `unreadableAfterOurCutStaysCut_andKeepsTheRecord` ya pasa con el código viejo (blinda que no se rompa). `CutAccessTest` falla en su primera aserción. Copiar la salida al informe y descomentar.

- [ ] **Step 3: implementación.** `control/AccessSwitch.kt` entero:

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
 */
class AccessSwitch(private val settings: Settings, private val saved: Saved) {
    /** Settings.Global: get is null for a setting never written and THROWS SecurityException when it cannot be read; put throws without WRITE_SECURE_SETTINGS. */
    interface Settings {
        fun get(name: String): Int?
        fun put(name: String, value: Int)
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

    /** true once both are read back as off; false without the permission, if a change did not stick or cannot be read back. */
    fun cut(): Boolean {
        // Cutting again keeps the values of the first cut.
        val savedNow = !hasRecord()
        if (savedNow) {
            saved.put(ADB_ENABLED, read(ADB_ENABLED, missing = 1) ?: 1)
            saved.put(ADB_WIFI_ENABLED, read(ADB_WIFI_ENABLED, missing = 0) ?: 0)
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
        if (!write(ADB_ENABLED to savedAdb, ADB_WIFI_ENABLED to savedWifi)) return false
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
    }

    private fun write(vararg changes: Pair<String, Int>): Boolean = try {
        for ((name, value) in changes) settings.put(name, value)
        changes.all { (name, value) -> (settings.get(name) ?: 0) == value }
    } catch (e: SecurityException) {
        false
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden
    }
}
```

Nota para el revisor: en `write`, `adb_enabled` nunca escrito se relee como `0`; tras `put` siempre existe, así que no cambia nada respecto a hoy.

En `control/CutAccess.kt`:
- En el `AccessSwitch.Settings` anónimo, `get` deja de capturar la excepción: `override fun get(name: String): Int? = Settings.Global.getString(resolver, name)?.toIntOrNull()` (comentario: "throws SecurityException when Android refuses the read: AccessSwitch treats it as unknown").
- Sustituir `isCut(resolver: ContentResolver)` por:

```kotlin
    /** Our cut, not contradicted by a readable setting (AccessSwitch.isCut). */
    fun isCut(context: Context): Boolean = switch(context).isCut()

    /** restore() will not guess (a record of both off): the person turns debugging on in Developer options. */
    fun needsManualRestore(context: Context): Boolean = switch(context).needsManualRestore()
```

- En `observe`, `current()` pasa a `AccessDiag.log(context, "observe"); switch.forgetIfRestored(); return switch.isCut()`. Quitar el import de `ContentResolver` si queda sin uso.
- `bridge/BridgeService.kt:73`: `CutAccess.isCut(contentResolver)` → `CutAccess.isCut(this)`.
- `ui/ChatRoot.kt:68` y `:121`: `CutAccess.isCut(context.contentResolver)` → `CutAccess.isCut(context)`.

En `android/README.md`, en la sección de cortar acceso (o al final de "How it works" si no la hay), añadir el párrafo:

```markdown
**Cut access state.** The app says "access cut" only after a cut it made itself and read back (its saved values in the
`cut_access` preferences), and while no readable setting says USB or wireless debugging is on again. It never infers a
cut from what it reads alone: on Android 17 an app may read `adb_enabled=0` while adb works. `adb logcat -s ADBuddyAccess`
shows what the app reads and how.
```

- [ ] **Step 4: comprobar que pasa**

Run: `gradle :app:testDebugUnitTest --tests '*AccessSwitchTest' --tests '*CutAccessTest' --tests '*AccessDiagTest' --tests '*ConversationViewTest' --tests '*LinkStatusTest'` y luego `gradle :app:assembleDebug :app:assembleDebugAndroidTest` (compila `CutAccessOnEmulatorTest` ya adaptado). Aplicar y deshacer las cinco mutaciones de arriba.
Expected: PASS y BUILD SUCCESSFUL.

- [ ] **Step 5: commit**

```bash
git add android/app/src android/README.md
git commit -m "fix(android): cortado solo tras un corte propio que ninguna lectura desmiente; reactivar restaura tal cual

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Un canal vivo gana a "cortado"; la notificación fija dice el estado del agente — crítica, revisor opus

**Files:**
- Modify: `ui/LinkStatus.kt:33-42`, `notify/ConversationView.kt` (sin cambios de lógica salvo los que arrastra `LinkStatus`), `res/values*/strings.xml` (`status_cut`)
- Test: `ui/LinkStatusTest.kt`, `notify/ConversationViewTest.kt`, `screens/ChatScreensTest.kt` (y sus capturas `chat_cut_*`)

**Interfaces:**
- Produces: `LinkStatus.of(state, cut, configured)` con orden INCOMPATIBLE(APP) > INCOMPATIBLE > PAUSED > CONNECTED > CUT > CONNECTING > NOT_SET_UP > WAITING.

- [ ] **Step 1: tests que fallan.** En `LinkStatusTest.kt` sustituir `aCutAccessSaysSoWhateverTheConnection` (blindaba el bug) por:

```kotlin
    @Test fun aLiveChannelBeatsCut_itProvesAdbIsUp() {
        assertEquals(LinkStatus.CONNECTED, LinkStatus.of(s(BridgeState.Connection.CONNECTED), cut = true))
        assertEquals(LinkStatus.PAUSED, LinkStatus.of(s(BridgeState.Connection.CONNECTED, p = true), cut = true))
        assertEquals(LinkStatus.UPDATE_APP, LinkStatus.of(s(BridgeState.Connection.INCOMPATIBLE, outdated = BridgeState.Outdated.APP), cut = true))
        assertEquals(LinkStatus.UPDATE_HOST, LinkStatus.of(s(BridgeState.Connection.INCOMPATIBLE, outdated = BridgeState.Outdated.HOST), cut = true))
    }

    @Test fun withoutAChannelACutSaysSo() {
        for (c in listOf(BridgeState.Connection.WAITING, BridgeState.Connection.CONNECTING)) {
            assertEquals(c.name, LinkStatus.CUT, LinkStatus.of(s(c, p = true), cut = true))
        }
        assertEquals(LinkStatus.CUT, LinkStatus.of(BridgeState(), cut = true, configured = false))
    }
```

(Comprobar con `grep -n "enum class Connection" -A3 bridge/BridgeState.kt` que no hay más valores; si los hay, añadirlos a la lista sin canal.)

En `theStatusNotificationChangesOnlyWithWhatItShows`, la última aserción usa un estado CONNECTED, donde `cut` ya no cambia nada. Sustituirla por:

```kotlin
        assertEquals("a live channel: cut changes nothing shown", StatusView.of(a, cut = false), StatusView.of(a, cut = true))
        val lost = s(BridgeState.Connection.WAITING)
        assertNotEquals(StatusView.of(lost, cut = false), StatusView.of(lost, cut = true))
```

En `ConversationViewTest.kt`:
- `withoutAConnectionTheLinkStatusSpeaks`: la segunda aserción pasa a `assertEquals(Headline(R.string.status_cut), ConversationView.of(state(conn = BridgeState.Connection.WAITING), cut = true).headline())`.
- `pausedOrDisconnectedIsNotAsking_andResumeStaysOffered`: la última línea pasa a `assertTrue(!ConversationView.of(withPerm(state(conn = BridgeState.Connection.WAITING)), cut = true).asking)`.
- Test nuevo (bug 4):

```kotlin
    @Test fun aLiveChannelNeverHeadlinesCut_theAgentStateDoes() {
        assertEquals(Headline(R.string.notif_working_on, "Open Settings"), ConversationView.of(state(), cut = true).headline())
        assertEquals(Headline(R.string.notif_waiting), ConversationView.of(state("waiting"), cut = true).headline())
        val v = ConversationView.of(withPerm(state()), cut = true)
        assertTrue("a question is still announced (generic)", v.asking)
        assertEquals(listOf(NotifAction.CUT), v.actions())
    }
```

En `ChatScreensTest.kt`, `cutSaysSoInTheBanner` pasa a usar `BridgeState(connection = BridgeState.Connection.WAITING, hostName = "my-pc")` y comprueba `status_short_cut` en la cabecera en vez de `session_idle` (leer el cuerpo del test y sustituir solo esas dos cosas); `aCutBannerDoesNotSurviveTheCutEndingWhileConnecting` pasa a `bannerLeavesWhenConnecting(BridgeState(connection = BridgeState.Connection.WAITING, hostName = "my-pc"), true, false, text(R.string.status_cut))`.

- [ ] **Step 2: comprobar que fallan**

Run: `gradle :app:testDebugUnitTest --tests '*LinkStatusTest' --tests '*ConversationViewTest'`
Expected: FAIL en `aLiveChannelBeatsCut_itProvesAdbIsUp`, `theStatusNotificationChangesOnlyWithWhatItShows` y `aLiveChannelNeverHeadlinesCut_theAgentStateDoes`.

- [ ] **Step 3: implementación.** En `LinkStatus.of`:

```kotlin
        /**
         * A live channel (connected, paused or of the wrong version) goes through `adb reverse`: it proves adb is not
         * cut, so it beats cut (spec 2026-10-18 R3). Without one, cut says so over anything else. Paused only while
         * connected: the pause of a lost link is not news. configured: the host has handed this phone a link
         * (BridgePrefs.link); without it and without a channel the phone was never set up.
         */
        fun of(state: BridgeState, cut: Boolean, configured: Boolean = true): LinkStatus = when {
            state.connection == BridgeState.Connection.INCOMPATIBLE && state.outdated == BridgeState.Outdated.APP -> UPDATE_APP
            state.connection == BridgeState.Connection.INCOMPATIBLE -> UPDATE_HOST
            state.connection == BridgeState.Connection.CONNECTED && state.sessionState.paused -> PAUSED
            state.connection == BridgeState.Connection.CONNECTED -> CONNECTED
            cut -> CUT
            state.connection == BridgeState.Connection.CONNECTING -> CONNECTING
            !configured -> NOT_SET_UP
            else -> WAITING
        }
```

`status_cut`: en `values/strings.xml` "Access cut: the PC cannot touch your phone"; en `values-es/strings.xml` "Acceso cortado: el PC no puede tocar tu móvil".

- [ ] **Step 4: comprobar que pasa y regrabar `chat_cut_*`**

Run: `gradle :app:testDebugUnitTest --tests '*LinkStatusTest' --tests '*ConversationViewTest' --tests '*AskNoticeTest'` (mutación: poner `cut -> CUT` primero en `LinkStatus.of` → falla `aLiveChannelBeatsCut_itProvesAdbIsUp`); luego `gradle :app:testDebugUnitTest --tests '*ChatScreensTest' -Padbuddy.screens=record` y sin la propiedad.
Expected: PASS. Mirar `chat_cut_light.png`: la cabecera ya no dice "Ready" con el banner de corte.

- [ ] **Step 5: commit**

```bash
git add android/app/src
git commit -m "fix(android): un canal vivo gana a cortado y la notificación fija muestra al agente

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Superposición del agente con colores fijos (API 30, contraste oscuro) — revisor opus

**Files:**
- Create: `a11y/OverlayColors.kt`
- Modify: `a11y/Overlay.kt:162-181`, `res/values*/strings.xml` (`overlay_pill`)
- Test: `a11y/OverlayColorsTest.kt`

**Interfaces:**
- Produces: `OverlayColors.of(dark: Boolean): OverlayColors.Pair` con `background: Int`, `text: Int`.

- [ ] **Step 1: test que falla**

```kotlin
package tech.hybriq.adbuddy.a11y

import java.io.File
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.ui.theme.Palette

class OverlayColorsTest {
    @Test fun thePillReadsInLightAndDark() {
        for (dark in listOf(false, true)) {
            val c = OverlayColors.of(dark)
            assertTrue("dark=$dark", Palette.contrast(c.text, c.background) >= 4.5)
        }
    }

    @Test fun noColorOfApi31OnAMinSdk30App() {
        val src = File(System.getProperty("adbuddy.kotlinDir"), "tech/hybriq/adbuddy/a11y/Overlay.kt").readText()
        assertFalse("system_accent1_* is API 31+ (minSdk 30)", src.contains("system_accent"))
    }
}
```

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*OverlayColorsTest'` → FAIL (compilación).

- [ ] **Step 3: implementación.** `a11y/OverlayColors.kt`:

```kotlin
package tech.hybriq.adbuddy.a11y

/**
 * The agent's frame and pill (classic views, no Compose theme): ADBuddy's primary, fixed. Not system_accent1_*:
 * that is API 31 and the app runs on 30 (spec 2026-10-18 §6, bug 5); and dark text on the light accent of the dark
 * theme (bug 6: white on a light tone did not read). Same values as Palette's primary/onPrimary.
 */
object OverlayColors {
    data class Pair(val background: Int, val text: Int)

    fun of(dark: Boolean): Pair =
        if (dark) Pair(0xFF7FD8C6.toInt(), 0xFF00382F.toInt()) else Pair(0xFF00695C.toInt(), 0xFFFFFFFF.toInt())
}
```

En `Overlay.kt`: `accent()` devuelve `OverlayColors.of(dark)` (renombrar a `colors()`); en `add()`, `val colors = colors()`, `FrameView(colors.background, …)`, `setTextColor(colors.text)`, `setColor(colors.background)`. Quitar el import de `Color` si queda sin uso.

`overlay_pill`: en "Agent using the screen · ⏸ Pause"; es "Agente usando la pantalla · ⏸ Pausar".

- [ ] **Step 4:** `gradle :app:testDebugUnitTest --tests '*OverlayColorsTest' --tests '*OverlayTest' --tests '*StringsParityTest'` → PASS. Si hay emulador API 30 disponible con cupo (memoria "Emuladores: cupos por RAM"), opcional: `GuardOnEmulatorTest` en API 30; si no, queda en la ledger como validación de emulador, no del móvil.

- [ ] **Step 5: commit** `fix(android): la superposición del agente usa colores propios legibles y válidos en Android 11` (con el trailer).

---

### Task 5: Paleta completa, tema y tipografía; Material You apagado por defecto — revisor adbuddy-reviewer

**Files:**
- Modify: `ui/theme/Palette.kt` (mapas enteros), `ui/theme/Theme.kt` (entero)
- Create: `ui/theme/AdbuddyColors.kt`, `ui/theme/Type.kt`, `ui/theme/Shapes.kt`
- Test: `ui/theme/PaletteContrastTest.kt` (entero), capturas de todas las pantallas

**Interfaces:**
- Produces: `Palette.light/dark` con los 48 roles de M3 1.4 (los 36 de la spec §4.1 y los 12 `*Fixed`) y `Palette.extraLight/extraDark` (success*, warning*); `AdbuddyColors` (data class con `success, onSuccess, successContainer, onSuccessContainer, warning, onWarning, warningContainer, onWarningContainer: Color`); `AdbuddyTheme.colors: AdbuddyColors` (`@Composable get`); `AdbuddyType.code: TextStyle`, `AdbuddyType.codeSmall: TextStyle` (`@Composable get`); `AdbuddyTheme(dark, dynamic = false, content)`; `BubbleShapes.mine/agent`.

- [ ] **Step 1: test que falla.** `PaletteContrastTest.kt` entero:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PaletteContrastTest {
    /** Every colour role of Material 3's ColorScheme: a missing one falls back to the lilac baseline (bug 7). */
    private val roles = setOf(
        "primary", "onPrimary", "primaryContainer", "onPrimaryContainer", "inversePrimary",
        "secondary", "onSecondary", "secondaryContainer", "onSecondaryContainer",
        "tertiary", "onTertiary", "tertiaryContainer", "onTertiaryContainer",
        "background", "onBackground", "surface", "onSurface", "surfaceVariant", "onSurfaceVariant", "surfaceTint",
        "inverseSurface", "inverseOnSurface", "error", "onError", "errorContainer", "onErrorContainer",
        "outline", "outlineVariant", "scrim", "surfaceBright", "surfaceDim",
        "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest", "surfaceContainerLow", "surfaceContainerLowest",
        "primaryFixed", "primaryFixedDim", "onPrimaryFixed", "onPrimaryFixedVariant",
        "secondaryFixed", "secondaryFixedDim", "onSecondaryFixed", "onSecondaryFixedVariant",
        "tertiaryFixed", "tertiaryFixedDim", "onTertiaryFixed", "onTertiaryFixedVariant",
    )

    private val surfaces = listOf("surface", "background", "surfaceBright", "surfaceDim", "surfaceContainerLowest", "surfaceContainerLow",
        "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest")

    // The pairs that carry text or controls (WCAG AA: 4.5 for text).
    private val pairs = listOf(
        "onPrimary" to "primary", "onPrimaryContainer" to "primaryContainer",
        "onSecondary" to "secondary", "onSecondaryContainer" to "secondaryContainer",
        "onTertiary" to "tertiary", "onTertiaryContainer" to "tertiaryContainer", "tertiary" to "tertiaryContainer",
        "onSurfaceVariant" to "surfaceVariant", "inverseOnSurface" to "inverseSurface", "inversePrimary" to "inverseSurface",
        "onError" to "error", "onErrorContainer" to "errorContainer",
        "primary" to "surface", "error" to "surface", "tertiary" to "surface", "primary" to "surfaceContainerHigh",
    ) + surfaces.flatMap { listOf("onSurface" to it, "onSurfaceVariant" to it) } +
        listOf("Primary", "Secondary", "Tertiary").flatMap { r ->
            listOf("${r.lowercase()}Fixed", "${r.lowercase()}FixedDim").flatMap { bg -> listOf("on${r}Fixed" to bg, "on${r}FixedVariant" to bg) }
        }

    private val extraPairs = listOf(
        "onSuccess" to "success", "onSuccessContainer" to "successContainer",
        "onWarning" to "warning", "onWarningContainer" to "warningContainer",
    )

    private fun check(name: String, scheme: Map<String, Int>, extra: Map<String, Int>) {
        for ((fg, bg) in pairs) {
            val c = Palette.contrast(scheme.getValue(fg), scheme.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
        for ((fg, bg) in extraPairs) {
            val c = Palette.contrast(extra.getValue(fg), extra.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
        for (s in listOf("success", "warning")) {
            val c = Palette.contrast(extra.getValue(s), scheme.getValue("surface"))
            assertTrue("$name: $s on surface is $c", c >= 4.5)
        }
    }

    @Test fun lightPaletteIsReadable() = check("light", Palette.light, Palette.extraLight)
    @Test fun darkPaletteIsReadable() = check("dark", Palette.dark, Palette.extraDark)

    @Test fun noRoleIsLeftToTheBaseline() {
        assertEquals(roles, Palette.light.keys)
        assertEquals(roles, Palette.dark.keys)
    }

    @Test fun contrastOfBlackOnWhiteIs21() {
        assertTrue(Math.abs(Palette.contrast(0xFF000000.toInt(), 0xFFFFFFFF.toInt()) - 21.0) < 0.01)
    }
}
```

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*PaletteContrastTest'` → FAIL (`extraLight` no existe).

- [ ] **Step 3: implementación.** En `Palette.kt`, sustituir los dos mapas (valores verificados con el cálculo WCAG: todos los pares ≥ 4,5) y añadir los extra:

```kotlin
    private fun c(v: Long) = v.toInt()

    /** The *Fixed roles of Material 3 (1.4): the same in light and dark (all pairs checked: >= 5.4:1). */
    private val fixed: Map<String, Int> = mapOf(
        "primaryFixed" to c(0xFFB8F0E4), "primaryFixedDim" to c(0xFF7FD8C6), "onPrimaryFixed" to c(0xFF00201B), "onPrimaryFixedVariant" to c(0xFF004F45),
        "secondaryFixed" to c(0xFFCDE8E1), "secondaryFixedDim" to c(0xFFB1CCC6), "onSecondaryFixed" to c(0xFF06201C), "onSecondaryFixedVariant" to c(0xFF334B47),
        "tertiaryFixed" to c(0xFFD6E3FF), "tertiaryFixedDim" to c(0xFFA8C8FF), "onTertiaryFixed" to c(0xFF001B3E), "onTertiaryFixedVariant" to c(0xFF26477A),
    )

    val light: Map<String, Int> = fixed + mapOf(
        "primary" to c(0xFF00695C), "onPrimary" to c(0xFFFFFFFF), "primaryContainer" to c(0xFFB8F0E4), "onPrimaryContainer" to c(0xFF00201B),
        "inversePrimary" to c(0xFF7FD8C6),
        "secondary" to c(0xFF4A635E), "onSecondary" to c(0xFFFFFFFF), "secondaryContainer" to c(0xFFCDE8E1), "onSecondaryContainer" to c(0xFF06201C),
        "tertiary" to c(0xFF3F5F90), "onTertiary" to c(0xFFFFFFFF), "tertiaryContainer" to c(0xFFD6E3FF), "onTertiaryContainer" to c(0xFF001B3E),
        "background" to c(0xFFF6FAF8), "onBackground" to c(0xFF171D1B), "surface" to c(0xFFF6FAF8), "onSurface" to c(0xFF171D1B),
        "surfaceVariant" to c(0xFFDBE5E1), "onSurfaceVariant" to c(0xFF3F4946), "surfaceTint" to c(0xFF00695C),
        "inverseSurface" to c(0xFF2B3230), "inverseOnSurface" to c(0xFFEDF2EF),
        "error" to c(0xFFBA1A1A), "onError" to c(0xFFFFFFFF), "errorContainer" to c(0xFFFFDAD6), "onErrorContainer" to c(0xFF410002),
        "outline" to c(0xFF6F7976), "outlineVariant" to c(0xFFBFC9C5), "scrim" to c(0xFF000000),
        "surfaceBright" to c(0xFFF6FAF8), "surfaceDim" to c(0xFFD6DBD9),
        "surfaceContainerLowest" to c(0xFFFFFFFF), "surfaceContainerLow" to c(0xFFF0F5F2), "surfaceContainer" to c(0xFFEAEFEC),
        "surfaceContainerHigh" to c(0xFFE4E9E7), "surfaceContainerHighest" to c(0xFFDEE4E1),
    )

    val dark: Map<String, Int> = fixed + mapOf(
        "primary" to c(0xFF7FD8C6), "onPrimary" to c(0xFF00382F), "primaryContainer" to c(0xFF004F45), "onPrimaryContainer" to c(0xFFB8F0E4),
        "inversePrimary" to c(0xFF00695C),
        "secondary" to c(0xFFB1CCC6), "onSecondary" to c(0xFF1C3531), "secondaryContainer" to c(0xFF334B47), "onSecondaryContainer" to c(0xFFCDE8E1),
        "tertiary" to c(0xFFA8C8FF), "onTertiary" to c(0xFF06305F), "tertiaryContainer" to c(0xFF26477A), "onTertiaryContainer" to c(0xFFD6E3FF),
        "background" to c(0xFF0F1513), "onBackground" to c(0xFFDEE4E1), "surface" to c(0xFF0F1513), "onSurface" to c(0xFFDEE4E1),
        "surfaceVariant" to c(0xFF3F4946), "onSurfaceVariant" to c(0xFFBFC9C5), "surfaceTint" to c(0xFF7FD8C6),
        "inverseSurface" to c(0xFFDEE4E1), "inverseOnSurface" to c(0xFF2B3230),
        "error" to c(0xFFFFB4AB), "onError" to c(0xFF690005), "errorContainer" to c(0xFF93000A), "onErrorContainer" to c(0xFFFFDAD6),
        "outline" to c(0xFF899390), "outlineVariant" to c(0xFF3F4946), "scrim" to c(0xFF000000),
        "surfaceBright" to c(0xFF353B39), "surfaceDim" to c(0xFF0F1513),
        "surfaceContainerLowest" to c(0xFF0A0F0E), "surfaceContainerLow" to c(0xFF171D1B), "surfaceContainer" to c(0xFF1B211F),
        "surfaceContainerHigh" to c(0xFF252B2A), "surfaceContainerHighest" to c(0xFF303634),
    )

    /** ADBuddy's own semantic colours (the status pill, the journal's levels): never from the wallpaper (spec R6). */
    val extraLight: Map<String, Int> = mapOf(
        "success" to c(0xFF2E6B30), "onSuccess" to c(0xFFFFFFFF), "successContainer" to c(0xFFC8E6C9), "onSuccessContainer" to c(0xFF0A2E0C),
        "warning" to c(0xFF7A4A00), "onWarning" to c(0xFFFFFFFF), "warningContainer" to c(0xFFFFDDB3), "onWarningContainer" to c(0xFF291800),
    )
    val extraDark: Map<String, Int> = mapOf(
        "success" to c(0xFF8BD88F), "onSuccess" to c(0xFF00390A), "successContainer" to c(0xFF1F5323), "onSuccessContainer" to c(0xFFC8E6C9),
        "warning" to c(0xFFFFB951), "onWarning" to c(0xFF452B00), "warningContainer" to c(0xFF633F00), "onWarningContainer" to c(0xFFFFDDB3),
    )
```

`ui/theme/AdbuddyColors.kt`:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/** Success and warning, which Material 3 lacks: the status pill and the journal's levels (spec §4.1). */
@Immutable
data class AdbuddyColors(
    val success: Color, val onSuccess: Color, val successContainer: Color, val onSuccessContainer: Color,
    val warning: Color, val onWarning: Color, val warningContainer: Color, val onWarningContainer: Color,
) {
    companion object {
        fun of(m: Map<String, Int>) = AdbuddyColors(
            Color(m.getValue("success")), Color(m.getValue("onSuccess")), Color(m.getValue("successContainer")), Color(m.getValue("onSuccessContainer")),
            Color(m.getValue("warning")), Color(m.getValue("onWarning")), Color(m.getValue("warningContainer")), Color(m.getValue("onWarningContainer")),
        )
        val Light = of(Palette.extraLight)
        val Dark = of(Palette.extraDark)
    }
}

val LocalAdbuddyColors = staticCompositionLocalOf { AdbuddyColors.Light }
```

`ui/theme/Type.kt`:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDirection

/** The system font, with titles a little heavier (spec §4.2). */
internal val AdbuddyTypography: Typography = Typography().let { t ->
    t.copy(
        titleLarge = t.titleLarge.copy(fontWeight = FontWeight.SemiBold),
        titleMedium = t.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        labelLarge = t.labelLarge.copy(fontWeight = FontWeight.Medium),
    )
}

/** What is going to run (commands, IP:port, keys): monospaced and always left to right (spec R7). */
object AdbuddyType {
    val code: TextStyle
        @Composable @ReadOnlyComposable get() = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace, textDirection = TextDirection.Ltr)
    val codeSmall: TextStyle
        @Composable @ReadOnlyComposable get() = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, textDirection = TextDirection.Ltr)
}
```

`ui/theme/Shapes.kt`:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Shapes
import androidx.compose.ui.unit.dp

internal val AdbuddyShapes = Shapes(
    extraSmall = RoundedCornerShape(4.dp), small = RoundedCornerShape(8.dp), medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(24.dp), extraLarge = RoundedCornerShape(28.dp),
)

/** 18/18/18/4: the small corner points at who speaks (spec §4.2). */
object BubbleShapes {
    val mine = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp, bottomEnd = 4.dp, bottomStart = 18.dp)
    val agent = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp, bottomEnd = 18.dp, bottomStart = 4.dp)
}
```

`ui/theme/Theme.kt`: construir los esquemas con **todos** los roles y el tema nuevo:

```kotlin
private fun Map<String, Int>.c(name: String) = Color(getValue(name))

private fun scheme(p: Map<String, Int>, dark: Boolean): ColorScheme {
    val base = if (dark) darkColorScheme() else lightColorScheme()
    return base.copy(
        primary = p.c("primary"), onPrimary = p.c("onPrimary"), primaryContainer = p.c("primaryContainer"), onPrimaryContainer = p.c("onPrimaryContainer"),
        inversePrimary = p.c("inversePrimary"),
        secondary = p.c("secondary"), onSecondary = p.c("onSecondary"), secondaryContainer = p.c("secondaryContainer"), onSecondaryContainer = p.c("onSecondaryContainer"),
        tertiary = p.c("tertiary"), onTertiary = p.c("onTertiary"), tertiaryContainer = p.c("tertiaryContainer"), onTertiaryContainer = p.c("onTertiaryContainer"),
        background = p.c("background"), onBackground = p.c("onBackground"), surface = p.c("surface"), onSurface = p.c("onSurface"),
        surfaceVariant = p.c("surfaceVariant"), onSurfaceVariant = p.c("onSurfaceVariant"), surfaceTint = p.c("surfaceTint"),
        inverseSurface = p.c("inverseSurface"), inverseOnSurface = p.c("inverseOnSurface"),
        error = p.c("error"), onError = p.c("onError"), errorContainer = p.c("errorContainer"), onErrorContainer = p.c("onErrorContainer"),
        outline = p.c("outline"), outlineVariant = p.c("outlineVariant"), scrim = p.c("scrim"),
        surfaceBright = p.c("surfaceBright"), surfaceDim = p.c("surfaceDim"),
        surfaceContainer = p.c("surfaceContainer"), surfaceContainerHigh = p.c("surfaceContainerHigh"), surfaceContainerHighest = p.c("surfaceContainerHighest"),
        surfaceContainerLow = p.c("surfaceContainerLow"), surfaceContainerLowest = p.c("surfaceContainerLowest"),
        primaryFixed = p.c("primaryFixed"), primaryFixedDim = p.c("primaryFixedDim"), onPrimaryFixed = p.c("onPrimaryFixed"), onPrimaryFixedVariant = p.c("onPrimaryFixedVariant"),
        secondaryFixed = p.c("secondaryFixed"), secondaryFixedDim = p.c("secondaryFixedDim"), onSecondaryFixed = p.c("onSecondaryFixed"), onSecondaryFixedVariant = p.c("onSecondaryFixedVariant"),
        tertiaryFixed = p.c("tertiaryFixed"), tertiaryFixedDim = p.c("tertiaryFixedDim"), onTertiaryFixed = p.c("onTertiaryFixed"), onTertiaryFixedVariant = p.c("onTertiaryFixedVariant"),
    )
}

private val lightScheme = scheme(Palette.light, dark = false)
private val darkScheme = scheme(Palette.dark, dark = true)

/**
 * Material 3 with ADBuddy's palette, light and dark. Material You (the wallpaper's colours, Android 12+) only when
 * asked (spec R6: Settings, "Use wallpaper colors"); the semantic colours (AdbuddyTheme.colors) never follow it.
 */
@Composable
fun AdbuddyTheme(dark: Boolean = isSystemInDarkTheme(), dynamic: Boolean = false, content: @Composable () -> Unit) {
    val context = LocalContext.current
    val scheme = when {
        dynamic && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> darkScheme
        else -> lightScheme
    }
    CompositionLocalProvider(LocalAdbuddyColors provides if (dark) AdbuddyColors.Dark else AdbuddyColors.Light) {
        MaterialTheme(colorScheme = scheme, typography = AdbuddyTypography, shapes = AdbuddyShapes, content = content)
    }
}

object AdbuddyTheme {
    val colors: AdbuddyColors
        @Composable @ReadOnlyComposable get() = LocalAdbuddyColors.current
}
```

(Imports nuevos: `androidx.compose.runtime.CompositionLocalProvider`, `androidx.compose.runtime.ReadOnlyComposable`. Si `ColorScheme.copy` con `surfaceBright`… no compila con la versión de Material 3 del BOM, usar el constructor `lightColorScheme(...)`/`darkColorScheme(...)` con los mismos parámetros con nombre. La anotación `AdbuddyPreviews` no cambia.)

`res/values/themes.xml` y `res/values-night/themes.xml`: comprobar que el fondo de la ventana de arranque usa `#F6FAF8` / `#0F1513`; si tiene otros colores fijos, cambiarlos a estos.

- [ ] **Step 4: tests y capturas**

Run: `gradle :app:testDebugUnitTest --tests '*PaletteContrastTest'` → PASS. Luego regrabar **todas** las capturas (`--tests 'tech.hybriq.adbuddy.screens.*' -Padbuddy.screens=record`), verificar sin la propiedad, y después la batería completa de la CI.
Evidencia: mirar `activity_entries_light.png` y `settings_confirmed_dark.png` (ya no hay lila ni morado), `consent_dark.png`, `chat_connected_messages_font_200_es.png`.

- [ ] **Step 5: commit** `feat(android): paleta propia completa, tipografía y formas; Material You solo si se pide` (con el trailer; incluye los PNG regrabados).

---

### Task 6: Icono propio adaptativo y de notificación — revisor adbuddy-reviewer

**Files:**
- Create: `android/app/src/main/res/drawable/ic_launcher_foreground.xml`, `drawable/ic_launcher_monochrome.xml`, `drawable/ic_stat_adbuddy.xml`, `drawable/ic_stat_shield.xml`, `drawable/ic_shield.xml`, `drawable/ic_shield_off.xml` (los dos del botón de la cabecera, tarea 8), `mipmap-anydpi/ic_launcher.xml`, `mipmap-anydpi/ic_launcher_round.xml`, `values/colors.xml`
- Modify: `AndroidManifest.xml:30` (y añadir `android:roundIcon`), `notify/ConversationNotifier.kt:52-130`, `confirm/ConfirmNotifier.kt:109`, cualquier otro `setSmallIcon(android.R.drawable.…)` (`grep -rn "android.R.drawable" app/src/main/kotlin`)
- Test: `notify/AppIconsTest.kt`

- [ ] **Step 1: test que falla**

```kotlin
package tech.hybriq.adbuddy.notify

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import tech.hybriq.adbuddy.R

@RunWith(RobolectricTestRunner::class)
class AppIconsTest {
    @Test fun theAppHasItsOwnIcon() {
        val app = RuntimeEnvironment.getApplication()
        assertEquals(R.mipmap.ic_launcher, app.applicationInfo.icon)
    }

    @Test fun noNotificationUsesAFrameworkIcon() {
        val dir = File(System.getProperty("adbuddy.kotlinDir"))
        val offenders = dir.walkTopDown().filter { it.isFile && it.extension == "kt" && it.readText().contains("android.R.drawable.") }.map { it.name }.toList()
        assertTrue("framework drawables in: $offenders", offenders.isEmpty())
    }

    @Test fun theNotificationIconsExist() {
        val res = RuntimeEnvironment.getApplication().resources
        for (id in listOf(R.drawable.ic_stat_adbuddy, R.drawable.ic_stat_shield, R.drawable.ic_shield, R.drawable.ic_shield_off)) assertTrue(res.getDrawable(id, null) != null)
    }
}
```

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*AppIconsTest'` → FAIL (compilación: `R.mipmap.ic_launcher`).

- [ ] **Step 3: recursos.** `values/colors.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <!-- The launcher icon's background: Palette.light primary (spec §4.3). -->
    <color name="ic_launcher_background">#00695C</color>
</resources>
```

`drawable/ic_launcher_foreground.xml` (108 dp; todo dentro de la zona segura 21-87):

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">
    <!-- A chat bubble with a terminal prompt: talk to the agent that runs things. -->
    <path android:fillColor="#FFFFFF"
        android:pathData="M38,32 H70 A8,8 0 0 1 78,40 V62 A8,8 0 0 1 70,70 H48 L36,80 V70 H38 A8,8 0 0 1 30,62 V40 A8,8 0 0 1 38,32 Z" />
    <path android:fillColor="#00695C" android:pathData="M41,45.5 L43.5,43 L51.5,51 L43.5,59 L41,56.5 L46.5,51 Z" />
    <path android:fillColor="#00695C" android:pathData="M54,56 H66 V59.5 H54 Z" />
</vector>
```

`drawable/ic_launcher_monochrome.xml` (la burbuja con el `>_` hueco):

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="108dp" android:height="108dp" android:viewportWidth="108" android:viewportHeight="108">
    <path android:fillColor="#FFFFFFFF" android:fillType="evenOdd"
        android:pathData="M38,32 H70 A8,8 0 0 1 78,40 V62 A8,8 0 0 1 70,70 H48 L36,80 V70 H38 A8,8 0 0 1 30,62 V40 A8,8 0 0 1 38,32 Z M41,45.5 L43.5,43 L51.5,51 L43.5,59 L41,56.5 L46.5,51 Z M54,56 H66 V59.5 H54 Z" />
</vector>
```

`mipmap-anydpi/ic_launcher.xml` e `ic_launcher_round.xml` (iguales):

```xml
<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@color/ic_launcher_background" />
    <foreground android:drawable="@drawable/ic_launcher_foreground" />
    <monochrome android:drawable="@drawable/ic_launcher_monochrome" />
</adaptive-icon>
```

(minSdk 30 ≥ 26: no hacen falta PNG por densidad. `mipmap-anydpi` sin sufijo de versión vale con AGP 9; si `lintDebug` lo pide, renombrar a `mipmap-anydpi-v26`.)

`drawable/ic_stat_adbuddy.xml` (24 dp; la misma burbuja recortada con el viewport):

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="24dp" android:height="24dp" android:viewportWidth="52" android:viewportHeight="52">
    <group android:translateX="-28" android:translateY="-30">
        <path android:fillColor="#FFFFFFFF" android:fillType="evenOdd"
            android:pathData="M38,32 H70 A8,8 0 0 1 78,40 V62 A8,8 0 0 1 70,70 H48 L36,80 V70 H38 A8,8 0 0 1 30,62 V40 A8,8 0 0 1 38,32 Z M41,45.5 L43.5,43 L51.5,51 L43.5,59 L41,56.5 L46.5,51 Z M54,56 H66 V59.5 H54 Z" />
    </group>
</vector>
```

`drawable/ic_stat_shield.xml` (escudo con "!" hueco, para las 🔴):

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="24dp" android:height="24dp" android:viewportWidth="24" android:viewportHeight="24">
    <path android:fillColor="#FFFFFFFF" android:fillType="evenOdd"
        android:pathData="M12,2 L4,5 V11 C4,16 7.4,20.7 12,22 C16.6,20.7 20,16 20,11 V5 Z M11,7 H13 V14 H11 Z M11,16 H13 V18 H11 Z" />
</vector>
```

`drawable/ic_shield.xml` (escudo relleno: "Cortar acceso del PC"; Compose lo tiñe) y `drawable/ic_shield_off.xml` (escudo en contorno tachado: "Volver a dar acceso"):

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="24dp" android:height="24dp" android:viewportWidth="24" android:viewportHeight="24">
    <path android:fillColor="#FFFFFFFF" android:pathData="M12,2 L4,5 V11 C4,16 7.4,20.7 12,22 C16.6,20.7 20,16 20,11 V5 Z" />
</vector>
```

```xml
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="24dp" android:height="24dp" android:viewportWidth="24" android:viewportHeight="24">
    <path android:fillColor="#00000000" android:strokeColor="#FFFFFFFF" android:strokeWidth="2" android:strokeLineJoin="round"
        android:pathData="M12,3 L5,5.7 V11 C5,15.4 8,19.6 12,20.9 C16,19.6 19,15.4 19,11 V5.7 Z" />
    <path android:fillColor="#00000000" android:strokeColor="#FFFFFFFF" android:strokeWidth="2" android:strokeLineCap="round"
        android:pathData="M3,3 L21,21" />
</vector>
```

Manifiesto: `android:icon="@mipmap/ic_launcher"` y `android:roundIcon="@mipmap/ic_launcher_round"`. Notifiers: todo `android.R.drawable.stat_notify_sync_noanim` → `R.drawable.ic_stat_adbuddy`; `android.R.drawable.ic_dialog_alert` → `R.drawable.ic_stat_shield`; la burbuja (`IconCompat.createWithResource(ctx, android.R.drawable.sym_action_chat)`) → `IconCompat.createWithResource(ctx, R.mipmap.ic_launcher)`. Y en cada `NotificationCompat.Builder` de la conversación y de las 🔴, `.setColor(ContextCompat.getColor(ctx, R.color.ic_launcher_background))` (tinte del icono en la sombra).

- [ ] **Step 4:** `gradle :app:testDebugUnitTest --tests '*AppIconsTest' --tests '*ConfirmNotificationTest' --tests '*BubbleFlowTest'` → PASS; `gradle :app:assembleDebug :app:lintDebug` → BUILD SUCCESSFUL. Evidencia visual: abrir los cuatro XML con la vista previa no es posible sin Android Studio; el implementador renderiza el primer plano con un test Robolectric temporal **no commiteado** o, más simple, confía en el revisor: dejar en el informe que el icono se valida en el emulador (`adb -s <emulador> shell cmd package resolve-activity` no hace falta; basta una captura del lanzador del emulador si hay cupo) y como V3 en el Pixel.

- [ ] **Step 5: commit** `feat(android): icono propio adaptativo con capa monocroma e iconos de notificación` (con el trailer).

---
### Task 7: Componentes de la pantalla principal: `HomeNotice`, `ActionCard`, `StatusPill` — revisor opus

La tarjeta de las preguntas 🔴 cerradas sustituye a `PendingQuestionsBanner`: su botón solo **reabre** el diálogo (con su armado y su scroll), nunca responde. Esta tarea crea los componentes y sus tests; la siguiente los conecta.

**Files:**
- Create: `ui/HomeNotice.kt`, `ui/ActionCard.kt`, `ui/StatusPill.kt`
- Modify: `res/values*/strings.xml` (`card_*`, `status_pill_remote`, `status_waiting`, `status_short_waiting`, `status_not_set_up`, `action_cut`, `action_restore`)
- Test: `ui/HomeNoticeTest.kt`, `screens/ActionCardScreensTest.kt`

**Interfaces:**
- Consumes: `AdbuddyTheme.colors` (tarea 5), `LinkStatus` (tarea 3).
- Produces: `enum class HomeNotice(val hasAction: Boolean) { QUESTIONS, CUT, UPDATE_APP, UPDATE_HOST, NOT_SET_UP, NOTIFICATIONS, PAUSED, WAITING }` (WAITING con botón "Cómo conectar") con `HomeNotice.of(banner: LinkStatus?, pendingQuestions: Int, notificationsBlocked: Boolean): HomeNotice?`; `@Composable fun ActionCard(notice: HomeNotice, pendingQuestions: Int, onAction: () -> Unit, modifier: Modifier = Modifier)`; `@Composable fun StatusPill(status: LinkStatus, host: String, remote: Boolean, modifier: Modifier = Modifier)`.

- [ ] **Step 1: test que falla** (`ui/HomeNoticeTest.kt`):

```kotlin
package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class HomeNoticeTest {
    @Test fun questionsFirst_theyExpire() {
        assertEquals(HomeNotice.QUESTIONS, HomeNotice.of(LinkStatus.CUT, pendingQuestions = 2, notificationsBlocked = true))
    }

    @Test fun thenTheLinkProblems_inOrder() {
        assertEquals(HomeNotice.CUT, HomeNotice.of(LinkStatus.CUT, 0, true))
        assertEquals(HomeNotice.UPDATE_APP, HomeNotice.of(LinkStatus.UPDATE_APP, 0, true))
        assertEquals(HomeNotice.UPDATE_HOST, HomeNotice.of(LinkStatus.UPDATE_HOST, 0, true))
        assertEquals(HomeNotice.NOT_SET_UP, HomeNotice.of(LinkStatus.NOT_SET_UP, 0, true))
    }

    @Test fun blockedNotificationsBeatPauseAndWaiting() {
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.PAUSED, 0, true))
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.WAITING, 0, true))
        assertEquals(HomeNotice.PAUSED, HomeNotice.of(LinkStatus.PAUSED, 0, false))
        assertEquals(HomeNotice.WAITING, HomeNotice.of(LinkStatus.WAITING, 0, false))
    }

    @Test fun nothingToDoNoCard() {
        assertNull(HomeNotice.of(null, 0, false))
        assertNull(HomeNotice.of(LinkStatus.CONNECTED, 0, false))
        assertNull(HomeNotice.of(LinkStatus.CONNECTING, 0, false))
    }

    @Test fun onlyTheCardsWithSomethingToTapHaveAButton() {
        assertEquals(
            setOf(HomeNotice.QUESTIONS, HomeNotice.CUT, HomeNotice.NOTIFICATIONS, HomeNotice.PAUSED, HomeNotice.WAITING),
            HomeNotice.entries.filter { it.hasAction }.toSet(),
        )
    }
}
```

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*HomeNoticeTest'` → FAIL (compilación).

- [ ] **Step 3: implementación.** `ui/HomeNotice.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

/**
 * The one card under the header (spec 2026-10-18 R8), instead of stacked strips. Red questions first (they expire),
 * then what stops everything (cut, versions, not set up), then blocked notifications, pause and waiting.
 * banner: the LinkStatus that deserves one (ChatScreen keeps the last one through CONNECTING), or null.
 */
enum class HomeNotice(val hasAction: Boolean) {
    QUESTIONS(true), CUT(true), UPDATE_APP(false), UPDATE_HOST(false), NOT_SET_UP(false), NOTIFICATIONS(true), PAUSED(true), WAITING(true);

    companion object {
        fun of(banner: LinkStatus?, pendingQuestions: Int, notificationsBlocked: Boolean): HomeNotice? = when {
            pendingQuestions > 0 -> QUESTIONS
            banner == LinkStatus.CUT -> CUT
            banner == LinkStatus.UPDATE_APP -> UPDATE_APP
            banner == LinkStatus.UPDATE_HOST -> UPDATE_HOST
            banner == LinkStatus.NOT_SET_UP -> NOT_SET_UP
            notificationsBlocked -> NOTIFICATIONS
            banner == LinkStatus.PAUSED -> PAUSED
            banner == LinkStatus.WAITING -> WAITING
            else -> null
        }
    }
}
```

`ui/ActionCard.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The card of HomeNotice: icon, title, one line and at most one button. QUESTIONS' button only reopens the closed
 * red questions (their dialog arms and makes you read again): nothing here answers anything (spec §8.4).
 */
@Composable
fun ActionCard(notice: HomeNotice, pendingQuestions: Int, onAction: () -> Unit, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    val extra = AdbuddyTheme.colors
    val neutral = notice == HomeNotice.NOT_SET_UP || notice == HomeNotice.WAITING
    val (container, content) = when (notice) {
        HomeNotice.QUESTIONS, HomeNotice.CUT, HomeNotice.UPDATE_APP, HomeNotice.UPDATE_HOST -> scheme.errorContainer to scheme.onErrorContainer
        HomeNotice.NOTIFICATIONS, HomeNotice.PAUSED -> extra.warningContainer to extra.onWarningContainer
        HomeNotice.NOT_SET_UP, HomeNotice.WAITING -> scheme.surfaceContainerHigh to scheme.onSurface
    }
    // Own shield vectors (task 6) for the security notices; only Info/Notifications come from material-icons-core.
    val icon: Painter = when (notice) {
        HomeNotice.CUT -> painterResource(R.drawable.ic_shield_off)
        HomeNotice.NOTIFICATIONS -> rememberVectorPainter(Icons.Default.Notifications)
        HomeNotice.NOT_SET_UP, HomeNotice.WAITING -> rememberVectorPainter(Icons.Default.Info)
        else -> painterResource(R.drawable.ic_shield)
    }
    val title = when (notice) {
        HomeNotice.QUESTIONS -> pluralStringResource(R.plurals.confirm_waiting, pendingQuestions, pendingQuestions)
        HomeNotice.CUT -> stringResource(R.string.card_cut_title)
        HomeNotice.UPDATE_APP -> stringResource(R.string.card_update_app_title)
        HomeNotice.UPDATE_HOST -> stringResource(R.string.card_update_host_title)
        HomeNotice.NOT_SET_UP -> stringResource(R.string.card_not_set_up_title)
        HomeNotice.NOTIFICATIONS -> stringResource(R.string.card_notifications_title)
        HomeNotice.PAUSED -> stringResource(R.string.card_paused_title)
        HomeNotice.WAITING -> stringResource(R.string.card_waiting_title)
    }
    val body: String? = when (notice) {
        HomeNotice.QUESTIONS -> null
        HomeNotice.CUT -> stringResource(R.string.card_cut_body)
        HomeNotice.UPDATE_APP -> stringResource(R.string.status_update_app)
        HomeNotice.UPDATE_HOST -> stringResource(R.string.status_update_host)
        HomeNotice.NOT_SET_UP -> stringResource(R.string.status_not_set_up)
        HomeNotice.NOTIFICATIONS -> stringResource(R.string.notifications_blocked)
        HomeNotice.PAUSED -> stringResource(R.string.status_paused)
        HomeNotice.WAITING -> stringResource(R.string.status_waiting)
    }
    val action: Int? = when (notice) {
        HomeNotice.QUESTIONS -> R.string.confirm_waiting_show
        HomeNotice.CUT -> R.string.action_restore
        HomeNotice.NOTIFICATIONS -> R.string.notifications_open_settings
        HomeNotice.PAUSED -> R.string.action_resume
        HomeNotice.WAITING -> R.string.card_waiting_action
        else -> null
    }
    Surface(color = container, contentColor = content, shape = MaterialTheme.shapes.medium, modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
            Icon(icon, contentDescription = null, tint = if (neutral) scheme.primary else LocalContentColor.current)
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                // Only the texts are the live region: the button stays an actionable node of its own.
                Column(Modifier.semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite }) {
                    Text(title, style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
                    if (body != null) Text(body, style = MaterialTheme.typography.bodyMedium)
                }
                if (action != null) {
                    // Only the button refuses covered touches (spec §8.5): no window-wide guard on a screen.
                    FilledTonalButton(onClick = onAction, modifier = Modifier.padding(top = 4.dp).refuseObscuredTouches()) { Text(stringResource(action)) }
                }
            }
        }
    }
}
```

`ui/StatusPill.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The link in one pill under the agent's name (spec §5): colour and dot by state, the PC's name when connected, and
 * " · Remote" while remote mode is on or unknown (RemoteUi.treatAsRemote: never a false "off").
 */
@Composable
fun StatusPill(status: LinkStatus, host: String, remote: Boolean, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    val extra = AdbuddyTheme.colors
    val (container, content, dot) = when (status) {
        LinkStatus.CONNECTED -> Triple(extra.successContainer, extra.onSuccessContainer, extra.success)
        LinkStatus.PAUSED -> Triple(extra.warningContainer, extra.onWarningContainer, extra.warning)
        LinkStatus.CUT, LinkStatus.UPDATE_APP, LinkStatus.UPDATE_HOST -> Triple(scheme.errorContainer, scheme.onErrorContainer, scheme.error)
        LinkStatus.CONNECTING, LinkStatus.WAITING, LinkStatus.NOT_SET_UP -> Triple(scheme.surfaceContainerHighest, scheme.onSurfaceVariant, scheme.outline)
    }
    val base = if (status == LinkStatus.CONNECTED && host.isNotEmpty()) stringResource(R.string.status_connected, host) else stringResource(status.shortText)
    val text = if (remote && (status == LinkStatus.CONNECTED || status == LinkStatus.PAUSED)) stringResource(R.string.status_pill_remote, base) else base
    Surface(color = container, contentColor = content, shape = CircleShape, modifier = modifier) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(dot, CircleShape))
            Text(text, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}
```

Cadenas (en / es), en los dos ficheros:
- `status_waiting`: "Looking for your PC… On the PC, run adbuddy serve with wireless debugging on." / "Buscando tu PC… En el PC, ejecuta adbuddy serve con la depuración inalámbrica encendida."
- `status_short_waiting`: "Looking for your PC…" / "Buscando tu PC…"
- `status_not_set_up`: "Not paired with your PC yet. On the PC: adbuddy setup" / "Falta emparejar con tu PC. En el PC: adbuddy setup"
- `status_pill_remote`: "%1$s · Remote" / "%1$s · Remoto"
- Títulos de tarjeta que **no repiten** la pastilla (si coinciden, `onNodeWithText` encuentra dos nodos y además no informa): `card_cut_title` "The PC cannot touch your phone" / "El PC no puede tocar tu móvil"; `card_cut_body` "You cut its access. Give it back when you want the agent to work again." / "Le cortaste el acceso. Devuélveselo cuando quieras que el agente vuelva a trabajar."; `card_update_app_title` "Update the app" / "Actualiza la app"; `card_update_host_title` "Update ADBuddy on the PC" / "Actualiza ADBuddy en el PC"; `card_not_set_up_title` "Not paired yet" / "Falta emparejar"; `card_notifications_title` "Notifications are off" / "Las notificaciones están apagadas"; `card_paused_title` "The agent is paused" / "El agente está en pausa"; `card_waiting_title` "Your PC is not answering" / "Tu PC no responde"; `card_waiting_action` "How to connect" / "Cómo conectar"; `setup_guide_url` (`translatable="false"`, solo en `values`) "https://hybriqtech.github.io/adbuddy/setup/".
- `action_restore`: "Give access back" / "Volver a dar acceso"; `action_cut`: "Cut PC access" / "Cortar acceso del PC" (los usa también el botón de la notificación: mismo texto, a propósito).

`UpdateAppTextTest` no se toca (`status_update_app` no cambia). Si algún test existente compara el texto viejo de `status_waiting` o `status_not_set_up` con un literal, se actualiza a la clave (no al literal).

`screens/ActionCardScreensTest.kt`: parametrizado por `ScreenMode` como `PendingQuestionsBannerScreensTest` (copiar su cabecera: `@RunWith(ParameterizedRobolectricTestRunner::class)`, `@GraphicsMode(GraphicsMode.Mode.NATIVE)`, `@Config(qualifiers = PHONE)`, `RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)`, `companion object` con `screenModes()`). Un test por `HomeNotice` que pinta `AdbuddyTheme { Column { StatusPill(LinkStatus.CONNECTED, "my-pc", remote = false); ActionCard(n, 2, {}) } }` (pastilla CONNECTED a propósito: su texto no coincide con ningún título de tarjeta), comprueba que el título se ve y que solo las de `hasAction` tienen botón (`compose.onAllNodes(hasClickAction()).assertCountEquals(if (n.hasAction) 1 else 0)`), y graba `shot("action_card_${n.name.lowercase()}", mode)`. Otro test: `StatusPill(LinkStatus.CONNECTED, "my-pc", remote = true)` muestra `app.getString(R.string.status_pill_remote, app.getString(R.string.status_connected, "my-pc"))`.

- [ ] **Step 4:** `gradle :app:testDebugUnitTest --tests '*HomeNoticeTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest' --tests '*TouchGuardSourceTest' --tests '*TouchGuardBehaviorTest'` (si `TouchGuardSourceTest` exige la guarda en `ActionCard`, ajustar su lista para aceptar `refuseObscuredTouches()` en el botón, sin quitar ninguna otra exigencia: decisión de la revisión del plan, spec §8.5); luego `--tests '*ActionCardScreensTest' -Padbuddy.screens=record` y sin la propiedad. Mirar `action_card_cut_dark.png` y `action_card_update_app_font_200_es.png` (texto entero).

- [ ] **Step 5: commit** `feat(android): tarjeta de aviso única y pastilla de estado` (con el trailer).

---

### Task 8: Cabecera con Cortar siempre visible, tarjeta única y modo remoto fuera del chat — crítica, revisor opus

**Files:**
- Create: `ui/RestoreAccessDialog.kt`
- Modify: `ui/ChatParts.kt` (`ChatHeader` entero y su vista previa `ChatHeaderPreview`, línea ~450, que pasa `onCut = {}, onRestore = {}` en vez de `onToggleCut = {}`), `ui/ChatScreen.kt` (`ChatActions`, cuerpo), `ui/ChatRoot.kt` (acciones, sin `RemoteCard`), `res/values*/strings.xml`, `confirm/ConfirmReopenTest.kt:11,46` (protegido §8.4: cambia solo el composable `PendingQuestionsBanner` → `ActionCard(HomeNotice.QUESTIONS, …)`, ninguna aserción), `ui/ChatScrollTest.kt:54` (quita `remoteCard = {}`)
- Delete: `ui/LinkBanner.kt`, `ui/PendingQuestionsBanner.kt`, `ui/NotificationsBanner.kt`, `screens/PendingQuestionsBannerScreensTest.kt`, `screens/NotificationsBannerScreensTest.kt` y sus PNG (sus aserciones pasan a `ChatScreensTest` y `QuestionsCardTest`)
- Test: `screens/ChatScreensTest.kt`, `ui/ChatPendingQuestionsTest.kt`, `ui/PendingQuestionsBannerTest.kt` → `ui/QuestionsCardTest.kt`

**Interfaces:**
- Consumes: `HomeNotice`, `ActionCard`, `StatusPill` (tarea 7); `CutAccess.cut/restore/needsManualRestore` (tarea 2); `R.drawable.ic_shield`, `R.drawable.ic_shield_off` (tarea 6).
- Produces: `ChatActions` sin `toggleCut`, con `cut: () -> Unit` y `restore: () -> Unit` en su lugar (16 lambdas) y `fun copyWith(cut: () -> Unit = this.cut, restore: () -> Unit = this.restore): ChatActions`; `ChatScreen(...)` **sin** el parámetro `remoteCard`; `ChatHeader(state, cut, terminalShown, onToggleTerminal, onPause, onCut, onRestore, onActivity, onSettings, onNewConversation, configured)`; `RestoreAccessDialog(onRestore, onDismiss)`.

**Por qué dos acciones y no un conmutador (crítico):** el escudo pinta su estado con `LinkStatus` (un canal vivo gana a `cut`) y `toggleCut` decidía leyendo `CutAccess.isCut` al tocar. Con un registro de corte y el canal aún vivo (el segundo que tarda en caer el túnel, o lecturas ilegibles), el escudo diría "Cortar" y el toque **restauraría**. Pulsar el escudo sin cortar **siempre** corta (`AccessSwitch.cut()` con registro conserva los primeros valores y vuelve a escribir 0). Además el menú ⋮ lleva **siempre** "Cortar acceso del PC", que solo corta (idempotente), aunque la cabecera muestre "Volver a dar acceso". Mutación: volver a un solo conmutador (`onClick = onToggleCut` con la decisión por `CutAccess.isCut`) → falla `theShieldAlwaysCutsWhenItSaysCut_evenWithACutRecordAndALiveChannel`.

- [ ] **Step 1: tests que fallan** (en `ChatScreensTest.kt`; `show()` y `bannerLeavesWhenConnecting()` dejan de pasar `remoteCard = {}`):

```kotlin
    @Test fun theShieldAlwaysCutsWhenItSaysCut_evenWithACutRecordAndALiveChannel() {
        var cuts = 0
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(cut = { cuts++ }, restore = { restores++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        assertEquals(1, cuts)
        assertEquals(0, restores)
    }

    @Test fun givingAccessBackAsksFirst() {
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(restore = { restores++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        assertEquals("the dialog asks first", 0, restores)
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.restore_confirm_confirm)).performClick()
        assertEquals(1, restores)
    }

    @Test fun theMenuAlwaysOffersCut_evenWhileCut() {
        var cuts = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(cut = { cuts++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.action_cut)).performClick()
        assertEquals(1, cuts)
    }

    @Test fun theRestoreDialogClosesWhenTheCutEnds() {
        var cut by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = cut, terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None,
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertIsDisplayed()
        cut = false // access came back some other way (Settings, the notification)
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertDoesNotExist()
    }

    @Test fun oneCardOnly_questionsBeatCutAndBlockedNotifications() {
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None,
                    notificationsBlocked = true, pendingQuestions = 2,
                )
            }
        }
        compose.onNodeWithText(app.resources.getQuantityString(R.plurals.confirm_waiting, 2, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.card_cut_title)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.card_notifications_title)).assertDoesNotExist()
        compose.shot("chat_questions_card", mode)
    }
```

(El botón de confirmar del diálogo usa una clave propia, `restore_confirm_confirm`, para que no choque en el árbol con el botón de la tarjeta, `action_restore`. Imports: `performClick`.)

`PendingQuestionsBannerTest` se renombra a `QuestionsCardTest` y `ChatPendingQuestionsTest` se adapta: cambia solo el composable montado (`PendingQuestionsBanner(count, onShow)` → `ActionCard(HomeNotice.QUESTIONS, count, onShow)`), **no** lo que comprueban (el número se ve; "Ver" llama a `onShow` y no responde nada).

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*ChatScreensTest'` → FAIL (compilación).

- [ ] **Step 3: implementación.**

`ui/RestoreAccessDialog.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import tech.hybriq.adbuddy.R

/** Giving the PC its access back asks first and says what comes back (spec R5). Cutting never asks. */
@Composable
fun RestoreAccessDialog(onRestore: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.restore_confirm_title)) },
        text = {
            ObscuredTouchGuard()
            Text(stringResource(R.string.restore_confirm_body))
        },
        confirmButton = { TextButton(onClick = onRestore) { Text(stringResource(R.string.restore_confirm_confirm)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}
```

`ChatHeader` (sustituye al actual; mantiene `TOP_BAR_TAG` y `AdbuddyTopBarSpec.height`, que vigilan `AdbuddyTopBarTest` y `AdbuddyTopBarSpecTest`):

```kotlin
/** Header: the agent (and what it does, only on a live, unpaused link), the link's pill; the shield (always), terminal, menu. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatHeader(
    state: BridgeState,
    cut: Boolean,
    terminalShown: Boolean,
    onToggleTerminal: () -> Unit,
    onPause: (Boolean) -> Unit,
    onCut: () -> Unit,
    onRestore: () -> Unit,
    onActivity: () -> Unit,
    onSettings: () -> Unit,
    onNewConversation: () -> Unit,
    configured: Boolean = true,
) {
    var menu by remember { mutableStateOf(false) }
    val status = LinkStatus.of(state, cut, configured)
    val agent = state.sessionState.agent.ifEmpty { stringResource(R.string.app_name) }
    // The agent's state only on a live, unpaused link: never "Ready" beside a pause or a cut (bug 9).
    val agentState = when (SessionLabel.of(state).takeIf { status == LinkStatus.CONNECTED }) {
        SessionLabel.WORKING -> R.string.session_working
        SessionLabel.WAITING -> R.string.session_waiting
        SessionLabel.IDLE -> R.string.session_idle
        SessionLabel.STOPPED -> R.string.session_stopped
        else -> null
    }
    val title = if (agentState == null) agent else stringResource(R.string.title_agent_state, agent, stringResource(agentState))
    val twoLines = with(LocalDensity.current) { MaterialTheme.typography.titleLarge.lineHeight.toDp() + MaterialTheme.typography.labelLarge.lineHeight.toDp() + 10.dp }
    TopAppBar(
        title = {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                StatusPill(status, state.hostName, remote = RemoteUi.treatAsRemote(state.remote))
            }
        },
        modifier = Modifier.testTag(TOP_BAR_TAG),
        expandedHeight = AdbuddyTopBarSpec.height(twoLines),
        actions = {
            // Always here (spec §5): cutting is the real guarantee and never hides in a menu. Two actions, never a toggle.
            val isCut = status == LinkStatus.CUT
            // No covered-touch filter here: a cut forced by a covered tap is harmless; restoring goes through a dialog with its guard.
            IconButton(onClick = if (isCut) onRestore else onCut) {
                Icon(
                    painterResource(if (isCut) R.drawable.ic_shield_off else R.drawable.ic_shield),
                    contentDescription = stringResource(if (isCut) R.string.action_restore else R.string.action_cut),
                    tint = if (isCut) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.error,
                )
            }
            IconButton(onClick = onToggleTerminal) {
                Icon(
                    if (terminalShown) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                    contentDescription = stringResource(if (terminalShown) R.string.chat_terminal_hide else R.string.chat_terminal_show),
                )
            }
            Box {
                IconButton(onClick = { menu = true }) {
                    Icon(Icons.Default.MoreVert, contentDescription = stringResource(R.string.action_menu))
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    ObscuredTouchGuard()
                    PauseButton.of(state)?.let { button ->
                        DropdownMenuItem(text = { Text(stringResource(button.text)) }, onClick = { menu = false; onPause(button == PauseButton.PAUSE) })
                    }
                    // Always, and it only cuts (cut() is idempotent): the safe way out never changes meaning.
                    DropdownMenuItem(text = { Text(stringResource(R.string.action_cut)) }, onClick = { menu = false; onCut() })
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_activity)) }, onClick = { menu = false; onActivity() })
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_settings)) }, onClick = { menu = false; onSettings() })
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_new_conversation)) }, onClick = { menu = false; onNewConversation() })
                }
            }
        },
    )
}
```

(El escudo usa los vectores propios de la tarea 6, nada de `Icons.Default.Warning`/`Lock`; no se añade `material-icons-extended`. Imports nuevos: `Arrangement`, `androidx.compose.ui.res.painterResource`.)

`ChatScreen.kt`:
- `ChatActions`: `val toggleCut` → `val cut: () -> Unit, val restore: () -> Unit` en esa posición; `None` con 16 `{}`; añadir `fun copyWith(cut: () -> Unit = this.cut, restore: () -> Unit = this.restore) = ChatActions(send, retry, discard, pickPhotos, pickFile, capture, removeDraft, retryDraft, attachment, pause, cut, restore, openActivity, openSettings, newConversation, interrupt)`.
- Quitar el parámetro `remoteCard` y su llamada; sustituir `LinkBanner`, `PendingQuestionsBanner` y `NotificationsBanner` por:

```kotlin
            HomeNotice.of(bannerStatus, pendingQuestions, notificationsBlocked)?.let { notice ->
                ActionCard(notice, pendingQuestions, onAction = {
                    when (notice) {
                        HomeNotice.QUESTIONS -> onShowQuestions()
                        HomeNotice.CUT -> askRestore = true
                        HomeNotice.NOTIFICATIONS -> onOpenNotificationSettings()
                        HomeNotice.PAUSED -> actions.pause(false)
                        HomeNotice.WAITING -> try {
                            context.startActivity(Intent(Intent.ACTION_VIEW, context.getString(R.string.setup_guide_url).toUri()).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                        } catch (e: ActivityNotFoundException) { /* no browser */ }
                        else -> {}
                    }
                })
            }
```

- `var askRestore by rememberSaveable { mutableStateOf(false) }` junto a `askNew`, y `LaunchedEffect(status) { if (status != LinkStatus.CUT) askRestore = false }` (el diálogo se cierra si el corte termina por otro lado); la cabecera recibe `onCut = actions.cut, onRestore = { askRestore = true }`; al final, junto a `askNew`: `if (askRestore && status == LinkStatus.CUT) RestoreAccessDialog(onRestore = { askRestore = false; actions.restore() }, onDismiss = { askRestore = false })`.
- `ChatScreenPreview` pierde `remoteCard = {}`.

`ChatRoot.kt`: antes de `val actions`, la función local

```kotlin
    fun report(ok: Boolean, restoring: Boolean) {
        val message = when {
            !ok && restoring && CutAccess.needsManualRestore(context) -> restoreManual
            !ok && restoring -> restoreFailed
            !ok -> cutFailed
            restoring -> cutRestored
            else -> cutDone
        }
        scope.launch {
            // Without the permission (or if the change did not stick), the person does it by hand.
            val r = snackbar.notify(message, actionLabel = if (ok) null else developerOptions)
            if (r == SnackbarResult.ActionPerformed) openDeveloperOptions(context)
        }
    }
```

(con `val restoreManual = stringResource(R.string.restore_manual)` junto a las demás cadenas de `ChatRoot`) y en `ChatActions(...)`, `toggleCut = {...}` se sustituye por `cut = { report(ok = CutAccess.cut(context), restoring = false) }, restore = { report(ok = CutAccess.restore(context), restoring = true) },`. La llamada a `ChatScreen` pierde el bloque final `{ RemoteCard(...) }` (R9). Después: `grep -rn "remoteCard\|toggleCut\|LinkBanner\|PendingQuestionsBanner\|NotificationsBanner" app/src` solo puede encontrar `SettingsContent`/`SettingsScreen` (`remoteCard` sigue en Ajustes).

Cadenas (en / es): `restore_confirm_title` "Give the PC access again?" / "¿Volver a dar acceso al PC?"; `restore_confirm_body` "ADBuddy turns debugging back on as it was before the cut, and the agent on the PC can use your phone again. If it cannot tell how it was, it takes you to Developer options." / "ADBuddy vuelve a encender la depuración como estaba antes del corte y el agente del PC podrá volver a usar tu móvil. Si no sabe cómo estaba, te lleva a Opciones de desarrollador."; `restore_manual` "ADBuddy cannot tell how debugging was before the cut. Turn on wireless debugging in Developer options." / "ADBuddy no sabe cómo estaba la depuración antes del corte. Enciende la depuración inalámbrica en Opciones de desarrollador."; `restore_confirm_confirm` "Give access" / "Dar acceso"; `title_agent_state` "%1$s · %2$s" (las dos); `agent_fallback` "The agent" / "El agente"; `cut_done` "Done. The PC can no longer touch your phone." / "Hecho. El PC ya no puede tocar tu móvil."; `cut_restored` "The PC has access again. If you used remote mode, its port stays open until the PC connects or the phone restarts." / "El PC vuelve a tener acceso. Si usabas el modo remoto, su puerto sigue abierto hasta que el PC se conecte o reinicies.".

En `ChatScreensTest.check(subtitle, banner, name)`: la "subtitle" pasa a ser el texto de la pastilla (`status.shortText` o "Connected to my-pc") y el "banner" el título de la tarjeta (`card_*_title`); ajustar las llamadas sin quitar ninguna comprobación de que se ve entero ni de que la cabecera cabe. `bannerLeavesWhenConnecting` y `cutSaysSoInTheBanner` (que en la tarea 3 usan `status_cut`, ya no visible en el chat) pasan a `text(R.string.card_cut_title)`; `aPausedBannerDoesNotSurviveConnecting` a `card_paused_title`, `anOutdatedAppBannerDoesNotSurviveConnecting` a `card_update_app_title` y los de espera a `card_waiting_title`.

- [ ] **Step 4:** `gradle :app:testDebugUnitTest --tests '*ChatScreensTest' --tests '*QuestionsCardTest' --tests '*ChatPendingQuestionsTest' --tests '*ConfirmReopenTest' --tests '*ChatScrollTest' --tests '*AdbuddyTopBar*' --tests '*TouchGuard*' --tests '*Obscured*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'`; regrabar `ChatScreensTest` y los `flows.*` que graban capturas; verificar; batería completa de la CI. Mirar `chat_connected_messages_{light,dark,font_200,font_200_es}.png`, `chat_cut_light.png`, `chat_questions_card_dark.png`.

- [ ] **Step 5: commit** `feat(android): cabecera con pastilla y Cortar siempre visible, una sola tarjeta de aviso, modo remoto en Ajustes` (con el trailer).

---

### Task 9: Terminal más pequeña, estado vacío con sugerencias y burbujas — revisor adbuddy-reviewer

Ruling (spec R15): la terminal empieza **plegada** también en la app (como ya hace la burbuja) y, abierta, ocupa el 35 % (antes 45 %) — por qué: al abrir, la terminal se comía la mitad del chat y es lo menos importante para quien vigila desde el móvil — qué cuesta: un toque en la flecha para verla.

**Files:**
- Modify: `ui/ChatEmptyState.kt` (entero), `ui/ChatScreen.kt` (condición y llamada; pesos `0.45f`/`0.55f` → `0.35f`/`0.65f`), `ui/ChatRoot.kt` (`terminalOpenByDefault: Boolean = false`), `ui/ChatParts.kt` (`Bubble`, líneas ~218-229), `res/values*/strings.xml`
- Test: `screens/ChatScreensTest.kt`

**Interfaces:**
- Consumes: `BubbleShapes` (tarea 5), `R.drawable.ic_launcher_monochrome` (tarea 6).
- Produces: `ChatEmptyState(paused: Boolean, onSuggestion: (String) -> Unit, modifier: Modifier = Modifier)`.

- [ ] **Step 1: test que falla** (en `ChatScreensTest`):

```kotlin
    @Test fun theEmptyChatShowsWhenPausedToo() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(paused = true)))
        compose.onNodeWithText(text(R.string.chat_empty_paused)).assertIsDisplayed()
    }

    @Test fun aSuggestionFillsTheField_neverSends() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"))
        compose.onNodeWithText(text(R.string.suggestion_battery)).performClick()
        compose.onNode(hasSetTextAction()).assert(hasText(text(R.string.suggestion_battery)))
        compose.shot("chat_connected_empty_suggestions", mode)
    }
```

- [ ] **Step 2:** FAIL.

- [ ] **Step 3: implementación.** `ui/ChatEmptyState.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R

/** An empty conversation: the logo, what to do, and three suggestions that fill the field (they never send by themselves). */
@Composable
fun ChatEmptyState(paused: Boolean, onSuggestion: (String) -> Unit, modifier: Modifier = Modifier) {
    // Scrolls: at 200 % text the logo, the line and three chips do not fit a short screen. No covered-touch guard: a
    // suggestion only fills the field (spec §8.5).
    Column(modifier.verticalScroll(rememberScrollState()).padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically), horizontalAlignment = Alignment.CenterHorizontally) {
        Image(
            painterResource(R.drawable.ic_launcher_monochrome), contentDescription = null, modifier = Modifier.size(72.dp),
            colorFilter = ColorFilter.tint(MaterialTheme.colorScheme.primary),
        )
        Text(
            stringResource(if (paused) R.string.chat_empty_paused else R.string.chat_empty_ready),
            style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
        )
        if (!paused) {
            for (id in listOf(R.string.suggestion_battery, R.string.suggestion_volume, R.string.suggestion_screenshot)) {
                val s = stringResource(id)
                SuggestionChip(onClick = { onSuggestion(s) }, label = { Text(s) })
            }
        }
    }
}
```

`ChatScreen`: condición `items.isEmpty() && tools.isEmpty() && (status == LinkStatus.CONNECTED || status == LinkStatus.PAUSED)` y llamada `ChatEmptyState(paused = status == LinkStatus.PAUSED, onSuggestion = { text = it }, modifier = Modifier.weight(weight).fillMaxSize())` (bug 14). `Bubble`: el agente pasa de `surfaceVariant`/`onSurfaceVariant` a `surfaceContainerHigh`/`onSurface`; `shape = if (mine) BubbleShapes.mine else BubbleShapes.agent`.

Cadenas: `chat_empty_ready` "Ask your phone for something. If the agent is about to do something risky, it will ask you first." / "Pídele algo a tu móvil. Si el agente va a hacer algo delicado, te preguntará antes."; `chat_empty_paused` "The agent is paused: it cannot use the phone until you resume." / "El agente está en pausa: no puede usar el móvil hasta que lo reanudes."; `suggestion_battery` "How much battery is left?" / "¿Cuánta batería queda?"; `suggestion_volume` "Turn the volume down" / "Baja el volumen"; `suggestion_screenshot` "Take a screenshot" / "Haz una captura de pantalla".

- [ ] **Step 4:** `--tests '*ChatScreensTest' --tests '*ChatScrollTest' --tests '*InputBarTest' --tests '*StringsParityTest' --tests '*TouchGuardSourceTest' --tests 'tech.hybriq.adbuddy.flows.*'` (si un test de `flows` da por abierta la terminal al entrar en la app, se adapta a plegada: es R15); regrabar `ChatScreensTest` y `flows.*`; verificar. Mirar `chat_connected_empty_suggestions_dark.png` y `chat_connected_messages_light.png`.

- [ ] **Step 5: commit** `feat(android): chat vacío con sugerencias y burbujas con forma propia` (con el trailer).

---

### Task 10: Tarjeta de pregunta del agente con su nombre — revisor opus

**Files:**
- Modify: `ui/PermissionCard.kt` (firma de `PermissionCards`/`PermissionCard`, título de la línea 127, estilo del detalle), `ui/ChatScreen.kt` (pasa el agente), `res/values*/strings.xml` (`perm_title`, `notif_asking`)
- Test: `ui/PermissionCardSemanticsTest.kt` (incluida la línea 57, que busca `getString(R.string.perm_title)` sin argumento), `flows/OnboardingFlowTest.kt:171` (igual), `screens/PermissionCardScreensTest.kt` (nuevo). Con `%1$s`, `getString(R.string.perm_title)` sin argumento ya no coincide: esas dos líneas pasan a `getString(R.string.perm_title, <agente del BridgeState que monta el test>)` (o `agent_fallback` si el estado no trae agente), sin cambiar qué comprueban.

**Interfaces:**
- Produces: `PermissionCards(..., agent: String = "")` y `PermissionCard(..., agent: String = "", modifier, onAnswer)`: en `PermissionCard`, `agent` va **justo antes de `modifier`** (la lambda final `onAnswer` debe seguir siendo el último parámetro; al final, la lambda de `PermissionCards.kt:98` se asignaría a `agent`). La llamada de `PermissionCards` pasa a nombrados: `PermissionCard(first, permissions.size - 1, connected, now, first.id in answeringIds, guardLeft, canAnswer, agent = agent, modifier = modifier) { p, d -> … }`; revisar con `grep -rn "PermissionCard(" app/src` las llamadas posicionales de los tests y nombrar `modifier` donde haga falta.

- [ ] **Step 1: tests que fallan** (en `PermissionCardSemanticsTest`, montando la tarjeta igual que sus tests actuales, añadiendo `agent = …`):

```kotlin
    @Test fun theTitleNamesTheRealAgent() {
        mount(agent = "codex") // el montaje que ya usa el fichero, con el parámetro nuevo
        compose.onNodeWithText(app.getString(R.string.perm_title, "codex")).assertIsDisplayed()
    }

    @Test fun withoutAnAgentNameItSaysTheAgent() {
        mount(agent = "")
        compose.onNodeWithText(app.getString(R.string.perm_title, app.getString(R.string.agent_fallback))).assertIsDisplayed()
    }
```

(Si el fichero no tiene una función de montaje, extraer la que repiten sus tests a `private fun mount(agent: String = "")` sin cambiar lo que comprueban.) `screens/PermissionCardScreensTest.kt`: parametrizado por `ScreenMode` (cabecera como `ActionCardScreensTest`), una `PermissionCard` con detalle de 40 líneas (`adb shell settings put global …`), `shot("permission_card", mode)`, y `assertIsNotEnabled()` sobre el botón de permitir antes de desplazar (sin tocar `PermissionCardScrollTest`).

- [ ] **Step 2:** FAIL (no existe el parámetro `agent`).

- [ ] **Step 3: implementación.** Parámetro `agent` en las dos funciones (`PermissionCards` lo pasa a cada tarjeta); título:

```kotlin
            Text(
                stringResource(R.string.perm_title, agent.ifEmpty { stringResource(R.string.agent_fallback) }),
                style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.tertiary,
                modifier = Modifier.semantics { heading() },
            )
```

El texto del detalle dentro del scroll vigilado pasa a `style = AdbuddyType.codeSmall.copy(textDirection = TextDirection.Ltr)` (explícito, como exige `ConfirmTextDirectionTest`). **No** cambian el `scroll`, el `enabled` de los botones, su orden ni `ObscuredTouchGuard`. En `ChatScreen`: `PermissionCards(state.chat.permissions, …, chat, agent = state.sessionState.agent)`.

Cadenas: `perm_title` "%1$s asks" / "%1$s pregunta"; `notif_asking` "Your agent needs an answer" / "Tu agente necesita una respuesta" (sin argumentos, R10).

- [ ] **Step 4:** `--tests '*PermissionCard*' --tests '*OnboardingFlowTest' --tests '*ConfirmTextDirectionTest' --tests '*ConversationViewTest' --tests '*AskNoticeTest' --tests '*StringsParityTest'`; regrabar `PermissionCardScreensTest`; verificar. Mirar `permission_card_light.png` (azul tinta) y `permission_card_font_200_es.png`.

- [ ] **Step 5: commit** `feat(android): la tarjeta de pregunta nombra al agente real y muestra el detalle en monoespaciada` (con el trailer).

---

### Task 11: Diálogo 🔴: el comando en un bloque monoespaciado — crítica, revisor opus

Solo lo visual. Criterio de aceptación: §8.1-8.6 de la spec con sus tests **sin cambios**.

**Files:**
- Modify: `ui/Screens.kt:196` (el `Text(shown.command, …)`)
- Test: `screens/ConfirmDialogScreensTest.kt` (nuevo)

- [ ] **Step 1: test.** `screens/ConfirmDialogScreensTest.kt`, parametrizado por `ScreenMode`, monta `ConfirmDialog` como `ConfirmDialogLayoutTest` (copiar su construcción), con un comando de 300 caracteres con comillas y `|` → `shot("confirm_dialog", mode)`; otra variante con el ⚠ especial → `shot("confirm_dialog_special", mode)`. Y dos aserciones: hay **un solo** nodo con scroll en el diálogo (`compose.onAllNodes(hasScrollAction()).assertCountEquals(1)`) y el comando está dentro de él (`compose.onNode(hasText(cmd.take(40), substring = true) and hasAnyAncestor(hasScrollAction())).assertExists()`). Mutación: dar al bloque del comando un `verticalScroll` propio → falla el recuento.

- [ ] **Step 2:** `gradle :app:testDebugUnitTest --tests '*ConfirmDialogScreensTest'` → FAIL en verify (no hay PNG todavía).

- [ ] **Step 3: implementación.** Sustituir `Text(shown.command, style = MaterialTheme.typography.bodySmall.copy(textDirection = TextDirection.Ltr))` por:

```kotlin
                    // The command is what will run: monospaced, on a block of its own, INSIDE the same watched scroll
                    // (no scroll of its own: "read to the end" must cover it, spec §8.2).
                    Surface(color = MaterialTheme.colorScheme.surfaceContainerHighest, shape = MaterialTheme.shapes.small, modifier = Modifier.fillMaxWidth()) {
                        Text(shown.command, style = AdbuddyType.code.copy(textDirection = TextDirection.Ltr), modifier = Modifier.padding(12.dp))
                    }
```

(`.copy(textDirection = TextDirection.Ltr)` explícito por si `ConfirmTextDirectionTest` lo busca en el fuente.) Nada más cambia en `ConfirmDialog` ni en `ConfirmHost`.

- [ ] **Step 4:** `--tests 'tech.hybriq.adbuddy.confirm.*' --tests '*ConfirmDialogButtonsSourceTest' --tests '*ConfirmTextDirectionTest' --tests '*TouchGuard*'` → PASS; `--tests '*ConfirmDialogScreensTest' -Padbuddy.screens=record` y verificar. Mirar las ocho `confirm_dialog*_*.png`.

- [ ] **Step 5: commit** `feat(android): el diálogo de confirmación muestra el comando en un bloque monoespaciado` (con el trailer).

---

### Task 12: Ajustes: secciones, idioma, colores del fondo y modo remoto neutro — revisor adbuddy-reviewer

**Files:**
- Create: `ui/theme/ThemePrefs.kt`
- Modify: `ui/SettingsScreen.kt` (`SettingsScreen`, `SettingsContent`), `AdbuddyApp.kt`, `MainActivity.kt`, `ChatBubbleActivity.kt`, `ui/RemoteCard.kt:100-107`, `res/values*/strings.xml`
- Test: `ui/theme/ThemePrefsTest.kt`, `screens/SettingsScreensTest.kt`

**Interfaces:**
- Produces: `class ThemePrefs(context: Context) { var dynamic: Boolean }`; `AdbuddyApp.dynamicColor: MutableStateFlow<Boolean>`; `SettingsContent(protectedCount, protectedConfirmed, access, onOpen, onBack, remoteCard, languageAvailable: Boolean = false, onLanguage: () -> Unit = {}, dynamicColor: Boolean = false, onDynamicColor: (Boolean) -> Unit = {})`.

- [ ] **Step 1: tests que fallan.** `ThemePrefsTest` (`@RunWith(RobolectricTestRunner::class)`): por defecto `false`; tras `ThemePrefs(app).dynamic = true`, `ThemePrefs(app).dynamic` es `true`. En `SettingsScreensTest`: se ven `settings_section_security` y `settings_section_app`; con `languageAvailable = true` la fila `settings_language` se ve y su toque llama a `onLanguage`; tocar `settings_dynamic_color` llama a `onDynamicColor(true)`.

- [ ] **Step 2:** FAIL (compilación).

- [ ] **Step 3: implementación.** `ui/theme/ThemePrefs.kt`:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import android.content.Context
import androidx.core.content.edit

/** Settings, "Use wallpaper colors" (spec R6): off unless the person turns it on. */
class ThemePrefs(context: Context) {
    private val prefs = context.getSharedPreferences("theme", Context.MODE_PRIVATE)
    var dynamic: Boolean
        get() = prefs.getBoolean("dynamic", false)
        set(value) = prefs.edit { putBoolean("dynamic", value) }
}
```

`AdbuddyApp`: `val dynamicColor by lazy { MutableStateFlow(ThemePrefs(this).dynamic) }`. `MainActivity` y `ChatBubbleActivity`: donde llaman a `AdbuddyTheme {`, `val dynamic by app.dynamicColor.collectAsStateWithLifecycle()` y `AdbuddyTheme(dynamic = dynamic) {`. `SettingsScreen` pasa `languageAvailable = LanguageSettings.available()`, `onLanguage = { LanguageSettings.open(context) }`, `dynamicColor = dynamic` (del flujo) y `onDynamicColor = { ThemePrefs(context).dynamic = it; app.dynamicColor.value = it }`.

`SettingsContent` (sin `ObscuredTouchGuard()` de pantalla entera, spec §8.5; el interruptor de colores no concede nada); cabecera de sección

```kotlin
@Composable
private fun SectionHeader(@StringRes title: Int) {
    Text(
        stringResource(title), style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(start = 16.dp, top = 16.dp, bottom = 4.dp).semantics { heading() },
    )
}
```

`SectionHeader(R.string.settings_section_security)`, las dos filas actuales y `remoteCard()`; `SectionHeader(R.string.settings_section_app)`; si `languageAvailable`, `SettingsRow(icon = { Icon(Icons.Default.Settings, contentDescription = null, tint = MaterialTheme.colorScheme.primary) }, title = R.string.settings_language, summary = stringResource(R.string.settings_language_hint), flagged = false, onClick = onLanguage)`; y la fila del interruptor:

```kotlin
            Row(
                Modifier.fillMaxWidth().toggleable(value = dynamicColor, role = Role.Switch, onValueChange = onDynamicColor).padding(horizontal = 16.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.settings_dynamic_color), style = MaterialTheme.typography.bodyLarge)
                    Text(stringResource(R.string.settings_dynamic_color_hint), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = dynamicColor, onCheckedChange = null)
            }
```

`RemoteCard`: en `DISCONNECTED`, contenedor `surfaceContainerHigh` con `onSurfaceVariant` (antes `errorContainer`); `remote_disconnected` "Available when the PC is connected" / "Disponible cuando el PC esté conectado". `RemoteCardSemanticsTest` y `RemoteUiTest` no cambian.

Cadenas: `settings_section_security` "Security" / "Seguridad"; `settings_section_app` "App" / "App" (misma palabra: añadir a `sameInBoth` de `i18n/StringsParityTest.kt` la entrada `"settings_section_app" to "\"App\" is the same word in both languages"`); `settings_dynamic_color` "Use wallpaper colors" / "Usar los colores del fondo de pantalla"; `settings_dynamic_color_hint` "Status colors stay the same" / "Los colores de estado no cambian".

- [ ] **Step 4:** `--tests '*ThemePrefsTest' --tests '*SettingsScreensTest' --tests '*LanguageSettingsTest' --tests '*RemoteCard*' --tests '*RemoteUiTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest' --tests '*TouchGuardSourceTest' --tests '*BubbleFlowTest'`; regrabar y verificar `SettingsScreensTest`. Mirar `settings_confirmed_dark.png` y `_font_200_es`.

- [ ] **Step 5: commit** `feat(android): ajustes por secciones con idioma, colores del fondo y modo remoto` (con el trailer).

---

### Task 13: Actividad con barra de nivel y chips de borrador con nombre — revisor adbuddy-reviewer

**Files:**
- Modify: `ui/ActivityScreen.kt:68-80` (`EntryRow`), `ui/ChatParts.kt:330-333` (`DraftChips`)
- Test: `screens/ActivityScreensTest.kt`, `ui/DraftChipsTest.kt`, `screens/DraftChipsScreensTest.kt`

- [ ] **Step 1: tests que fallan.** `ActivityScreensTest`: una entrada roja tiene un nodo con `contentDescription` = `app.getString(R.string.level_red)` (bug 16; confirmar los nombres con `grep -n "level_" res/values/strings.xml` y el tipo de `JournalEntry.level` en `bridge/`). `DraftChipsTest`: un borrador fallido muestra su nombre **y** su error, cada uno visible (bug 12).

- [ ] **Step 2:** FAIL.

- [ ] **Step 3: implementación.** `EntryRow`:

```kotlin
@Composable
private fun EntryRow(e: JournalEntry, now: Long) {
    val extra = AdbuddyTheme.colors
    val scheme = MaterialTheme.colorScheme
    val (bar, levelName) = when (e.level) {
        "red" -> scheme.error to stringResource(R.string.level_red)
        "yellow" -> extra.warning to stringResource(R.string.level_yellow)
        else -> extra.success to stringResource(R.string.level_green)
    }
    ElevatedCard(Modifier.fillMaxWidth()) {
        // One node for TalkBack: the level's name is read with the entry, not as a lone bar.
        Row(Modifier.height(IntrinsicSize.Min).semantics(mergeDescendants = true) {}) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(bar).semantics { contentDescription = levelName })
            Row(Modifier.padding(12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
                // Aquí va, sin cambios, el Column que hoy sigue al Text(levelIcon(e.level)); ese Text desaparece.
                // El Text de R.string.activity_entry pasa a style = AdbuddyType.codeSmall (color onSurfaceVariant igual).
            }
        }
    }
}
```

(Si `levelIcon` queda sin uso, borrarlo. Si `e.level` no es texto `"red"/"yellow"/"green"`, adaptar solo las ramas del `when`.)

`DraftChips`: el `Text` muestra siempre `d.name` con `maxLines = 1`; si `failed`, debajo `Text(d.error.ifEmpty { stringResource(R.string.draft_failed) }, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error, maxLines = 3, overflow = TextOverflow.Ellipsis)`.

- [ ] **Step 4:** `--tests '*ActivityScreensTest' --tests '*DraftChips*' --tests '*EntryTimeTest'`; regrabar `ActivityScreensTest` y `DraftChipsScreensTest`; verificar. Mirar `activity_entries_light.png` y `draft_chips_dark.png`.

- [ ] **Step 5: commit** `feat(android): diario con barra de nivel accesible y chips de borrador con nombre y error` (con el trailer).

---

### Task 14: Cierre: batería completa, capturas y pendientes — revisor adbuddy-reviewer

**Files:**
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md`

- [ ] **Step 1:** `gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (segundo plano, timeout 20 min) → BUILD SUCCESSFUL.
- [ ] **Step 2:** `git grep -n "system_accent\|android.R.drawable\|toggleCut\|LinkBanner(" android/app/src/main` → nada. `git status --short android/app/src/test/screenshots` → limpio.
- [ ] **Step 3:** mirar una muestra final (`chat_connected_messages_*`, `chat_cut_*`, `consent_*`, `settings_*`, `confirm_dialog_*`, `permission_card_*`) y anotar cualquier lila, texto cortado o contraste dudoso.
- [ ] **Step 4:** en `pendientes-m3-m5.md`: P4 (icono) hecho; bug 17 aparcado (R13); V1-V3 de abajo.
- [ ] **Step 5: commit** `docs: cierre del plan de aspecto visual y estado del acceso` (con el trailer).

---

## Validaciones manuales pendientes (solo lo que exige el móvil real; siempre con permiso explícito del usuario)

- **V1. Diagnóstico de "cortado".** Dato del 2026-10-18: en el Pixel (Android 17, app 0.2.0) `adb shell` da `adb_enabled=1` y `adb_wifi_enabled=1` y la app decía "cortado", así que la app leía otra cosa. Con la build de la tarea 1 instalada (instalar es un cambio en el móvil: pedir permiso), `adb logcat -d -s ADBuddyAccess` junto a `adb shell settings get global adb_enabled` / `adb_wifi_enabled`. Si la app lee `raw:0` (o un error) con `adb shell` diciendo `1`, Android 17 da a la app un valor falso: la regla de la tarea 2 (registro propio + canal) es la definitiva, y si además la relectura de `restore()` falla siempre en el Pixel, se abre un plan aparte (confirmar con el canal en vez de con la lectura). Si lee `raw:1`, el "cortado" venía de otra parte (p. ej. un registro de corte antiguo en 0.2.0) y las líneas `saved.*` lo dirán. En todos los casos, tras la tarea 2 la app no dice "cortado" sin un corte propio.
- **V2. Cortar y volver a dar acceso en el Pixel** (modifica el móvil). **Prerrequisito: V1.** Si V1 muestra lecturas falsas, V2 no se hace con esta build (cortar y reactivar no se pueden confirmar) y se abre el plan aparte; si V1 muestra lecturas fiables: cortar desde el escudo → el PC pierde la conexión y la app dice "Acceso cortado"; "Volver a dar acceso" → vuelven los ajustes que había (en el Pixel, USB e inalámbrica a 1) y el PC reconecta (el puerto inalámbrico puede cambiar).
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+).

## Autorrevisión

- Cobertura de la spec: R1-R4 y diagnóstico → tareas 1-3; R5 → 8; R6 → 5 y 12; R7 → 5; R8 → 7-8; R9 → 8 y 12; R10 → 10; R12 → 12; R13 → 14; §4.3 → 6; §5 → 7-9; §6 → 4, 10-13; §7 → en cada tarea; §8 → criterios de 7, 8, 10, 11.
- Cada tarea compila sola con lo anterior: 1 (nuevo); 2 (usa 1); 3 (independiente; las llamadas a `isCut(context)` ya las cambió 2); 4 (usa `Palette.contrast`, que ya existe); 5; 6; 7 (usa 3 y 5); 8 (usa 2 y 7); 9 (usa 5 y 6); 10, 11, 13 (usan 5); 12 (usa 5).
- Nombres: `AccessDiag.line/read/log`, `CutAccess.PREFS`, `CutAccess.isCut(context)`, `CutAccess.needsManualRestore(context)`, `AccessSwitch.needsManualRestore()`, `R.drawable.ic_shield/ic_shield_off`, `ChatActions.cut/restore/copyWith`, `HomeNotice.of`, `ActionCard(notice, pendingQuestions, onAction)`, `StatusPill(status, host, remote)`, `RestoreAccessDialog(onRestore, onDismiss)`, `AdbuddyTheme.colors`, `AdbuddyType.code/codeSmall`, `BubbleShapes.mine/agent`, `ThemePrefs.dynamic`, `AdbuddyApp.dynamicColor`, `OverlayColors.of`.
