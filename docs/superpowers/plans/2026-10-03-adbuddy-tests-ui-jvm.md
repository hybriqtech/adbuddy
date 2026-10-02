# ADBuddy · Tests de UI en la JVM (validaciones 27 y 34) · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Un implementador a la vez en `android/` (todas las tareas tocan el mismo árbol); implementadores y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`), `adbuddy-reviewer` por tarea salvo la Tarea 5 (`adbuddy-reviewer-opus`: toca la confirmación del acceso a notificaciones) y `adbuddy-reviewer-opus` en la revisión final. Nunca Haiku. **Antes de la Tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido, frente a las validaciones 27 y 34 del plan de M5c** (CLAUDE.md), y el plan se corrige con sus hallazgos.

**Meta:** que lo que las validaciones manuales 27 y 34 de M5c miraban a mano en el Pixel se compruebe en la JVM con `gradle :app:testDebugUnitTest`, sin móvil ni emulador: las pantallas del onboarding, Ajustes, la lista de apps protegidas y el acceso a notificaciones en claro, oscuro y letra al 200 %, más la letra al 200 % en español, el idioma de textos más largos (imágenes de referencia que detectan cualquier píxel cambiado por encima de la tolerancia de color de Roborazzi, más comprobaciones del árbol semántico: textos presentes, no recortados, sin salirse de la pantalla ni solaparse), los flujos de esas pantallas (pasos 1 y 2, "Ahora no", atrás, búsqueda, rotación, descartar cambios, guardar) y el camino de la burbuja hasta Ajustes y el botón de confirmar.

**Arquitectura:** Robolectric ejecuta la app de verdad en la JVM (`AdbuddyApp`, `MainActivity`, `ChatBubbleActivity`, las pantallas con su estado) y el `ui-test` de Compose la maneja por el árbol semántico; Roborazzi (solo la biblioteca, sin su plugin de Gradle) guarda y compara las imágenes que pinta el modo de gráficos nativo de Robolectric. Las imágenes de cada pantalla salen de sus composables sin estado (`ProtectedAppsContent`, `NotificationAccessContent`, `SettingsContent`, que ya existen para las vistas previas) y los flujos, de las actividades reales con el estado del sistema puesto por las sombras de Robolectric (`Settings.Secure`, `ShadowNotificationManager`, el `PackageManager`). **No cambia ni una línea de código de producción**: todo lo nuevo está en `android/app/src/test/`, en el catálogo y en el `build.gradle.kts`.

**Tecnologías:** Robolectric 4.17 (con su Android 14, `android-all-instrumented` `14-robolectric-10818077-i7`), `androidx.compose.ui:ui-test-junit4` (de la BOM 2026.09.00 del proyecto: 1.12.1), Roborazzi 1.76.0 (`io.github.takahirom.roborazzi:roborazzi`). Nada en el código de producción, en el APK ni en `protocol/`.

**Verificación del plan (réplica):** este documento se ha aplicado tal cual, **desde cero y tarea a tarea, con un script**, sobre una copia limpia (`git archive HEAD` de `test/ui-jvm`, 65909f9) con Java 17 y Gradle 9.8.0 en `--offline`: el script escribe los bloques `Crear`, aplica los pares "reemplazar / por" (cada texto "reemplazar" aparece una sola vez en su fichero), ejecuta cada bloque `bash expect=fail|pass` comprobando que falla o pasa como dice el plan (y, si lleva `reason=`, que la salida contiene ese texto) y hace los commits en un repositorio desechable. Resultado en "Autorrevisión del plan".

## Decisión de herramienta (con la evidencia)

Antes de este plan no había **ninguna** biblioteca de tests de Compose, Robolectric ni capturas: ni en `android/gradle/libs.versions.toml` ni en `android/app/build.gradle.kts` (solo `junit`, `mockwebserver`, y `androidx.test` para los instrumentados), y la caché de Gradle (`~/.gradle/caches/modules-2/files-2.1`) solo tenía `androidx.test:core/monitor/runner`, `androidx.test.ext:junit` y `com.android.tools.layoutlib:layoutlib-api` (no el `layoutlib` que pinta). **Sí había red** (Maven Central y Google Maven contestan 200), así que se probó en una copia, nunca en el repositorio:

| Prueba (en la copia) | Resultado |
|---|---|
| Robolectric 4.17 + `ui-test-junit4` + Roborazzi 1.76.0, primera resolución con red | Descarga en 1 min 40 s (unos 160 MB de `org.robolectric`, el más grande `nativeruntime-dist-compat` con los gráficos nativos de todos los sistemas; Roborazzi 1,1 MB). |
| SDK 37 (el `targetSdk`) en Robolectric | `Failed to create a Robolectric sandbox: Android SDK 37 requires Java 21 (have Java 17)`. Con `sdk=36`: lo mismo ("SDK 36 requires Java 21"). |
| `sdk=34` (Android 14) con Java 17 | Funciona. `ChatBubbleActivity` arranca entera bajo Robolectric (`AdbuddyApp.onCreate` incluida) y su menú lleva a Ajustes; la captura (Roboto del propio jar de Android) sale bien en Windows. |
| `createComposeRule()` sin `ui-test-manifest` | `Unable to resolve activity for Intent { … androidx.activity.ComponentActivity }`. Registrando esa actividad en el `PackageManager` de Robolectric (`ComponentActivityRule`, abajo), funciona. |
| El jar de Android de Robolectric | La primera vez Robolectric lo descargó **por su cuenta** a `~/.m2` (151 MB), fuera de Gradle. Pasándolo por una configuración de Gradle (`robolectric.offline` + `robolectric.dependency.dir`) y apartando `~/.m2`, funciona, y con `--offline`. |
| Determinismo | Grabar y comparar en dos ejecuciones: mismos píxeles (`md5` iguales); una imagen cambiada da `… is changed` y una que falta, `The original file(…) was not found`. |
| Comprobaciones de texto | `didOverflowWidth` no sirve (un `Text` que envuelve su contenido es más estrecho que el ancho en el que se maquetó: da "desbordado" en todos); con las líneas con elipsis, las que no caben en alto y las más anchas que la caja, sí. En el modo de gráficos *legacy* de Robolectric el texto no se mide de verdad y nada sale recortado: hace falta `GraphicsMode.NATIVE`. |
| Mutaciones (código de producción tocado y devuelto) | `firstRun = true` en la burbuja: los 3 tests de la burbuja fallan. `remember` en vez de `rememberSaveable` en la selección: falla la rotación. `maxLines = 1` en el resumen de una fila de Ajustes: fallan **solo** los modos de letra al 200 % ("cut short"). 14 dp de relleno en vez de 16 en las tarjetas del acceso: fallan sus imágenes en todos los modos. Sin el filtro por nombre en la búsqueda: falla la búsqueda. `if (true)` en vez de `if (allowedNow)` en el botón de confirmar: falla el test de la revocación antes del toque. |
| Suite completa en la copia, `--offline` | 631 tests en verde (571 que ya había más 60 nuevos), +13 s de reloj en este PC ("Autorrevisión del plan"). |

**Paparazzi** (`app.cash.paparazzi`, 2.0.0-alpha05 la última) se descartó sin instalarlo: pinta con `layoutlib` pero no ejecuta actividades, clics, navegación ni rotaciones, así que la validación 34 (y los flujos de la 27) necesitarían Robolectric igualmente: serían dos pilas, con un plugin de Gradle más y el `layoutlib` entero. **Solo aserciones semánticas** (sin imágenes) se descartó porque Roborazzi cuesta 1,1 MB sobre Robolectric y es lo único que ve un color, un relleno o un icono cambiados.

## Global Constraints

- Rama `test/ui-jvm`, sobre `main` (a154ec9). Commits con identidad local hybriqtech y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify`, nunca `--force`, sin `git stash`. Sin push: a `main` integra el usuario.
- **El móvil del usuario no se toca y el emulador tampoco** (es de otra sesión): todo corre en la JVM. Lo que sigue necesitando el Pixel queda escrito en "Validaciones manuales pendientes".
- **Ni una línea de producción cambia.** Nada en `android/app/src/main/`; las mutaciones de los pasos "el test muerde" se deshacen en el mismo paso (`git checkout -- android/app/src/main` y `git diff --exit-code android/app/src/main`). Si una pantalla no se pudiera instanciar en la JVM, se para y se pregunta (no hizo falta: las tres tienen ya su `*Content` sin estado y las actividades arrancan bajo Robolectric).
- **Nada nuevo en el APK:** las bibliotecas nuevas son `testImplementation` (más la configuración `robolectricAndroid`, que no entra en ninguna variante); no se añade `ui-test-manifest` (pondría una actividad más en el APK de depuración).
- **Cada tarea compila y pasa sus tests por sí sola**; ninguna usa clases de una tarea posterior.
- **Kotlin:** `export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"`, desde `android/`: `gradle --no-daemon <tareas> --offline` (no hay wrapper). En este PC la caché ya tiene todo lo nuevo; en otra máquina, la primera ejecución de la Tarea 1 va sin `--offline`. Usar `--no-daemon`: `gradle --stop` pararía también los demonios de otras sesiones.
- **Imágenes de referencia** (`android/app/src/test/screenshots/*.png`): se graban con `-Padbuddy.screens=record` **en Windows** y se miran antes de hacer commit (abrirlas con la herramienta Read); nunca se regraban para que un test pase sin mirar qué cambió (las diferencias quedan en `android/app/build/outputs/roborazzi/`, `*_compare.png`).
- Pasar a los revisores límites de tiempo (tope de 40 minutos por ejecución de Gradle).
- `docs/superpowers/plans/pendientes-m3-m5.md` y el plan de M5c están en CRLF en este árbol de trabajo (`core.autocrlf=true` sin atributo `eol` en `docs/`): sus ediciones usan anclas de **una sola línea**, y sus finales de línea no se tocan (no "arreglarlos" a LF). `android/` y `.github/` son LF.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: Robolectric 4.17 + `ui-test-junit4` + Roborazzi 1.76.0 (biblioteca, sin plugin).** — Solo Robolectric ejecuta en la JVM las actividades reales (la burbuja con `firstRun = false`, `MainActivity` con el onboarding), los clics, la navegación, la rotación y los toasts; Roborazzi añade 1,1 MB para las imágenes; Paparazzi no maneja actividades y sería otra pila. Sin el plugin, el modo (grabar, verificar, comparar) lo ponen tres propiedades del sistema desde `build.gradle.kts`, y `:app:testDebugUnitTest` verifica sin tareas nuevas. — Si me equivoco: cambiar la biblioteca de imágenes es tocar `shot()` (un único punto) y regrabar.
2. **Ruling: Robolectric corre el SDK 34 (`robolectric.properties`), no el 37.** — Con Java 17 (el del proyecto y de la CI) Robolectric 4.17 rechaza los SDK 35 a 37 ("requires Java 21"). Las pantallas son Compose y se pintan igual en 34; lo que no se ve en la JVM es lo propio de Android 17 (insets, fuentes del sistema, color dinámico), que queda para el Pixel. — Si me equivoco: subir a 36/37 es dar Java 21 al `Test` (toolchain) en local y en `android.yml`, cambiar `sdk=` y el jar del catálogo, y regrabar.
3. **Ruling: el jar de Android de Robolectric llega por Gradle.** — Robolectric lo descarga solo, a `~/.m2`, fuera de la caché de Gradle y sin respetar `--offline`; con la configuración `robolectricAndroid`, `robolectric.offline=true` y `robolectric.dependency.dir` lo resuelve Gradle como cualquier dependencia y los tests no tocan la red. — Si me equivoco: al subir Robolectric hay que subir también `robolectricAndroidAll` (el nombre exacto sale en el error "Unable to find…" si no casan, que falla cerrado).
4. **Ruling: sin `ui-test-manifest`; `ComponentActivityRule` registra la actividad vacía en el `PackageManager` de Robolectric.** — `ui-test-manifest` como `debugImplementation` mete una actividad más en el APK de depuración (el que el usuario instala en su Pixel con `adbuddy setup`; lo que se publica es el de release); registrarla en la sombra solo existe en el test. — Si me equivoco (una versión de Robolectric sin `addActivityIfNotPresent`): `debugImplementation(ui-test-manifest)` y quitar la regla.
5. **Ruling: las imágenes fallan en Windows con el comparador por defecto de Roborazzi, y en el resto solo se comparan.** — El comparador por defecto (`SimpleImageComparator(maxDistance = 0.007f)`, comprobado con `javap`) tolera hasta un 0,7 % de diferencia de color por píxel y ninguna proporción de píxeles cambiados (`changeThreshold = 0`, también por defecto): en Windows dos ejecuciones dan los mismos píxeles, y una tolerancia de área dejaría pasar un icono de 16 dp que falta. Las fuentes salen del jar de Android de Robolectric (Roboto), no del sistema; pero no se pudo comprobar en Linux ni en macOS si los gráficos nativos dan exactamente los mismos píxeles (la única distribución WSL de este PC es la del runner de CI, aislada: no se usó). Por eso `adbuddy.screens` vale `verify` en Windows (falla con cualquier cambio) y `compare` fuera (escribe las diferencias en `build/outputs/roborazzi` y no falla); `record` graba. Las comprobaciones semánticas fallan en todos los sistemas. Pantalla 411×891 dp a hdpi (las 48 imágenes ocupan 3,3 MB), paleta estática de ADBuddy (`dynamic = false`, como las vistas previas: el color dinámico depende del fondo de cada móvil). En la CI de Linux una diferencia no falla pero sale como aviso (`::warning`) en el resumen del PR, además del artefacto. — Si me equivoco: en `ubuntu-latest` (el runner por defecto de `android.yml`) una regresión visual pasa en verde con solo un aviso hasta resolver la validación 1; en otra máquina Windows (CPU distinta: Skia elige rutas SIMD en tiempo de ejecución) puede salir un fallo falso, que se arregla regrabando allí o con una tolerancia; un Linux que dé los mismos píxeles pasa a `verify` cambiando una línea.
6. **Ruling: los modos se ponen en la configuración, no en el composable.** — `ScreenModeRule` cambia los calificadores (`+night`) y el `fontScale` antes de que arranque la actividad: el tema lee `isSystemInDarkTheme()` y la letra al 200 % pasa por el escalado no lineal del sistema (Android 14+), como en un móvil; un `LocalDensity(fontScale = 2f)` a mano escalaría lineal y mentiría. — Si me equivoco: las imágenes del 200 % cambian y se regraban.
7. **Ruling: "no recortado" se mide en el árbol semántico (`assertTextsFit`), con test propio.** — Cada texto (árbol sin fusionar) no lleva elipsis, cabe en alto, no tiene líneas más anchas que su caja, no se sale de la pantalla en horizontal y no se solapa con otro (con `boundsInRoot`, que recortan los contenedores con scroll: un texto bajo la barra inferior no cuenta). Necesita `GraphicsMode.NATIVE`. Tampoco puede quedar por encima del borde superior (salvo tras un scroll, `scrolled = true`), y el título de una pantalla con barra superior cabe en sus 64 dp (`assertTitleFitsTheTopBar`: la barra de Material 3 no recorta y ninguna otra comprobación lo ve). Un fallo conocido que este plan no arregla va en `KNOWN_UI_ISSUES` con su modo: la comprobación lo deja pasar mientras siga y falla cuando se arregle. Hoy hay uno: "Acceso a las notificaciones" al 200 % en español (89 dp en la barra de 64). `ScreenTestSupportTest` comprueba que caza cada caso. — Si me equivoco: un falso positivo se afina en `assertTextsFit` con su test; un falso negativo lo ve la imagen **una sola vez**, al grabarla y mirarla: después queda dentro de la referencia como si fuera correcto (le pasó al título de dos líneas), así que cada fallo que se vea al grabar va a `KNOWN_UI_ISSUES` o se arregla, nunca se graba sin más.
8. **Ruling: los flujos usan las actividades reales y el estado del sistema por sombras.** — La validación 34 es sobre la burbuja tal cual (`ChatBubbleActivity`), y el onboarding sobre `MainActivity`; el acceso "permitido" se pone como lo pone el sistema (`enabled_notification_listeners`, `NotificationManager`, el listener enlazado con `onListenerConnected()`), sin tocar `NotificationConsent` salvo para leerlo. El `connected` del listener es de todo el proceso: el test lo desenlaza en `@After`. — Si me equivoco: un test que deja estado a otro; se ve como un fallo según el orden.
9. **Ruling: el acceso se prueba en sus cuatro estados más "comprobando", y hay un cuarto modo: letra al 200 % en español.** — La validación 27 dice "los tres estados" porque se escribió antes del ruling 15 de M5c ("permitido en el sistema, sin confirmar aquí"); la pantalla tiene cuatro (`Effective`) y "comprobando" antes de que el sistema conteste. El español es el otro idioma de la app, el de textos más largos y el del móvil del usuario: al 200 % es donde antes se corta algo (el título "Acceso a las notificaciones" ya ocupa dos líneas en la barra superior). Los tests leen los textos de los recursos, así que valen en los dos idiomas. — Si me equivoco: 12 imágenes más (unos 0,9 MB); quitar el modo es borrar una línea del `enum` y sus imágenes.
10. **Ruling: la CI no cambia de orden; avisa de las diferencias y las sube como artefacto.** — `android.yml` ya corre `:app:testDebugUnitTest`; se añaden un paso que escribe un `::warning` (visible en el resumen del PR) si existe `android/app/build/outputs/roborazzi` (solo existe si algo cambió) y otro que sube esa carpeta, los dos salvo que el trabajo se cancele. — Si me equivoco: en Linux una regresión visual sale en verde con un aviso que alguien puede no leer, hasta que la validación 1 pase el valor por defecto a `verify`.

## Mapa de ficheros

**Nuevos (todos de test):** `android/app/src/test/resources/robolectric.properties`; en `android/app/src/test/kotlin/tech/hybriq/adbuddy/`: `screens/ScreenTestSupport.kt`, `screens/ScreenTestSupportTest.kt`, `screens/Samples.kt`, `screens/OnboardingScreensTest.kt`, `screens/SettingsScreensTest.kt`, `screens/ProtectedAppsScreensTest.kt`, `screens/NotificationAccessScreensTest.kt`, `flows/Launcher.kt`, `flows/OnboardingFlowTest.kt`, `flows/ProtectedAppsFlowTest.kt`, `flows/BubbleFlowTest.kt`; y las 48 imágenes de `android/app/src/test/screenshots/`.

**Modificados:** `android/gradle/libs.versions.toml`, `android/app/build.gradle.kts`, `.gitattributes`, `.github/workflows/android.yml`, `android/README.md`, `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md` (validaciones 27 y 34), `docs/superpowers/plans/pendientes-m3-m5.md`.

**No se toca:** nada de `android/app/src/main/`, `host/` ni `protocol/`.

---

### Task 1: Robolectric, Compose ui-test y Roborazzi, y las comprobaciones de texto

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupport.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupportTest.kt`, `android/app/src/test/resources/robolectric.properties`
- Modify: `android/gradle/libs.versions.toml`, `android/app/build.gradle.kts`, `.gitattributes`

**Interfaces:** `PHONE` (calificadores de la pantalla), `ScreenMode` (`LIGHT`, `DARK`, `FONT_200`, `FONT_200_ES`), `screenModes()`, `ScreenModeRule(mode)`, `ComponentActivityRule()`, `SemanticsNodeInteractionsProvider.shot(name, mode)`, `SemanticsNodeInteractionsProvider.assertTextsFit(scrolled = false)`, `SemanticsNodeInteractionsProvider.assertTitleFitsTheTopBar(title, known = false)`, `KNOWN_UI_ISSUES` y `knownUiIssue(id, mode)`; propiedad de Gradle `adbuddy.screens` (`record`, `verify`, `compare`).

- [ ] **Step 1: El test de las comprobaciones y las comprobaciones.** Son código de test los dos: el test dice qué tiene que cazar `assertTextsFit` (elipsis, líneas que no caben, texto fuera de la pantalla por un lado o por arriba, solapes) y `assertTitleFitsTheTopBar` (un título más alto que la barra, y un fallo conocido que ya no está), y que un texto con sitio pasa.

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupportTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Text
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * assertTextsFit catches what it says it catches (and passes a plain text): the screen tests lean on it. It needs
 * the native graphics: in Robolectric's legacy mode text is not really measured and nothing is ever cut short.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ScreenTestSupportTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private fun failure(): String = assertThrows(AssertionError::class.java) { compose.assertTextsFit() }.message.orEmpty()

    @Test fun aTextWithRoomFits() {
        compose.setContent { Text("Protected apps") }
        compose.assertTextsFit()
    }

    @Test fun anEllipsisIsCutShort() {
        compose.setContent { Box(Modifier.width(60.dp)) { Text("A text far too long for its box", maxLines = 1, overflow = TextOverflow.Ellipsis) } }
        assertTrue(failure().startsWith("cut short"))
    }

    @Test fun linesThatDoNotFitAreCutShort() {
        compose.setContent { Box(Modifier.width(60.dp)) { Text("A text far too long for its box", maxLines = 1) } }
        assertTrue(failure().startsWith("cut short"))
    }

    @Test fun aTextOffTheScreenSidewaysIsCaught() {
        compose.setContent { Text("Gone", Modifier.offset(x = 500.dp)) }
        assertTrue(failure().startsWith("off screen sideways"))
    }

    @Test fun aTextAboveTheScreenIsCaughtUnlessTheListWasScrolled() {
        compose.setContent { Text("Gone", Modifier.offset(y = (-30).dp)) }
        assertTrue(failure().startsWith("off screen at the top"))
        compose.assertTextsFit(scrolled = true)
    }

    /** Eight lines of text: taller than the 64 dp bar. */
    private val TALL = listOf("One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight").joinToString(separator = "\n")

    @Test fun aTitleTallerThanTheTopBarIsCaught() {
        compose.setContent { Text(TALL) }
        val error = assertThrows(AssertionError::class.java) { compose.assertTitleFitsTheTopBar(TALL) }
        assertTrue(error.message.orEmpty().startsWith("title sticks out"))
        // As a known issue it passes while it still sticks out.
        compose.assertTitleFitsTheTopBar(TALL, known = true)
    }

    @Test fun aKnownIssueThatIsFixedFails() {
        compose.setContent { Text("Notification access") }
        compose.assertTitleFitsTheTopBar("Notification access")
        val error = assertThrows(AssertionError::class.java) { compose.assertTitleFitsTheTopBar("Notification access", known = true) }
        assertTrue(error.message.orEmpty().startsWith("known issue gone"))
    }

    @Test fun twoTextsDrawnOverEachOtherAreCaught() {
        compose.setContent {
            Box {
                Text("Allowed")
                Text("Unknown")
            }
        }
        assertTrue(failure().startsWith("overlap"))
    }
}
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupport.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import android.content.ComponentName
import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsNodeInteractionsProvider
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.text.TextLayoutResult
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Assert.assertTrue
import org.junit.rules.ExternalResource
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.R

/** The phone the screens are drawn on: a Pixel-sized portrait screen, at 1.5x so the golden images stay small. */
const val PHONE = "w411dp-h891dp-hdpi"

/**
 * The ways each screen is looked at: light, dark and text at 200 % (validation 27), and text at 200 % in Spanish, the
 * app's other language and the one with the longest texts. The default language of the tests is English.
 */
enum class ScreenMode(val qualifiers: String, val fontScale: Float) {
    LIGHT("+notnight", 1f),
    DARK("+night", 1f),
    FONT_200("+notnight", 2f),
    FONT_200_ES("+es-rES-notnight", 2f),
}

/** For ParameterizedRobolectricTestRunner: one run per mode. */
fun screenModes(): List<Array<Any>> = ScreenMode.entries.map { arrayOf<Any>(it) }

/**
 * Puts the mode in the configuration before the activity starts (outer rule), so the theme (isSystemInDarkTheme)
 * and the text (the system's non-linear font scaling) read it as on a phone.
 */
class ScreenModeRule(private val mode: ScreenMode) : ExternalResource() {
    override fun before() {
        RuntimeEnvironment.setQualifiers(mode.qualifiers)
        RuntimeEnvironment.setFontScale(mode.fontScale)
    }
}

/**
 * createComposeRule() starts an empty androidx ComponentActivity, which the app's manifest does not declare.
 * Robolectric is told about it here, instead of adding ui-test-manifest (an extra activity in the debug APK).
 */
class ComponentActivityRule : ExternalResource() {
    override fun before() {
        val app = RuntimeEnvironment.getApplication()
        shadowOf(app.packageManager).addActivityIfNotPresent(ComponentName(app, ComponentActivity::class.java))
    }
}

/**
 * A golden image (src/test/screenshots/<name>_<mode>.png). -Padbuddy.screens=record records it; on Windows every other
 * run compares it and fails when any pixel changes by more than Roborazzi's default tolerance (0.7 % of colour per pixel,
 * no share of pixels allowed to change); elsewhere it is only compared (app/build.gradle.kts). Rendering is
 * Robolectric's native graphics, with the fonts of its own Android jar: nothing comes from the host.
 */
fun SemanticsNodeInteractionsProvider.shot(name: String, mode: ScreenMode) {
    onRoot().captureRoboImage("src/test/screenshots/${name}_${mode.name.lowercase()}.png")
}

/**
 * Known UI faults that the screen tests show and this plan does not fix (they belong to the app): per mode, the string
 * resources of the titles that do not fit the top bar. The check lets them through and fails once one fits, so the
 * entry goes with the fix.
 * - access_title at 200 % in Spanish: "Acceso a las notificaciones" takes two lines (89 dp) in the 64 dp top bar and
 *   sticks out below it (validation 27 of M5c).
 */
val KNOWN_UI_ISSUES: Map<ScreenMode, Set<Int>> get() = mapOf(ScreenMode.FONT_200_ES to setOf(R.string.access_title))

fun knownUiIssue(id: Int, mode: ScreenMode): Boolean = id in KNOWN_UI_ISSUES[mode].orEmpty()

/**
 * Material 3's TopAppBar is 64 dp high and does not clip its title: a title taller than that (two lines at 200 %)
 * sticks out of the bar, which no other check sees (the text itself is whole and touches nothing). known: the title
 * is in KNOWN_UI_ISSUES, so it must still stick out (a failure tells to remove the entry).
 */
fun SemanticsNodeInteractionsProvider.assertTitleFitsTheTopBar(title: String, known: Boolean = false) {
    val node = onAllNodes(hasText(title), useUnmergedTree = true).fetchSemanticsNodes().minBy { it.positionInRoot.y }
    val bar = 64 * RuntimeEnvironment.getApplication().resources.displayMetrics.density
    val bottom = node.positionInRoot.y + node.size.height
    if (known) assertTrue("known issue gone: \"$title\" fits the top bar now; remove it from KNOWN_UI_ISSUES", bottom > bar + 0.5f)
    else assertTrue("title sticks out of the 64 dp top bar: \"$title\" ends at $bottom px, the bar at $bar px", bottom <= bar + 0.5f)
}

private fun SemanticsNode.texts(): List<String> = config.getOrNull(SemanticsProperties.Text).orEmpty().map { it.text }

private fun SemanticsNode.layout(): TextLayoutResult? {
    val action = config.getOrNull(SemanticsActions.GetTextLayoutResult)?.action ?: return null
    val results = mutableListOf<TextLayoutResult>()
    return if (action(results)) results.firstOrNull() else null
}

private fun SemanticsNode.withDescendants(): List<SemanticsNode> = listOf(this) + children.flatMap { it.withDescendants() }

/**
 * didOverflowWidth is no use here: a Text that wraps its content is narrower than the width it was laid out in.
 * Cut short is a line with an ellipsis, lines that do not fit the height, or a line wider than the text's box.
 */
private fun cutShort(layout: TextLayoutResult): Boolean =
    layout.didOverflowHeight || (0 until layout.lineCount).any { layout.isLineEllipsized(it) || layout.getLineRight(it) > layout.size.width + 0.5f }

/**
 * What a person would see as broken text, over every text on screen (the unmerged tree): a text cut short, a text
 * that leaves the screen sideways or above its top, or two texts drawn over each other. boundsInRoot is clipped by the
 * scrolling containers, so a text scrolled out of view (or under a bottom bar) does not count as overlapping.
 * scrolled: after a scroll a list moves texts above its top on purpose, so the top is not checked.
 */
fun SemanticsNodeInteractionsProvider.assertTextsFit(scrolled: Boolean = false) {
    val root = onNode(isRoot(), useUnmergedTree = true).fetchSemanticsNode()
    val width = root.boundsInRoot.width
    val texts = root.withDescendants().filter { it.texts().isNotEmpty() && it.layout() != null }
    assertTrue("no text on screen", texts.isNotEmpty())
    for (node in texts) {
        val text = node.texts().joinToString()
        assertTrue("cut short: \"$text\"", !cutShort(node.layout()!!))
        // Unclipped: boundsInRoot would be cut to the screen and hide it.
        val left = node.positionInRoot.x
        val right = left + node.size.width
        assertTrue("off screen sideways: \"$text\" from $left to $right (screen width $width)", left >= -0.5f && right <= width + 0.5f)
        if (scrolled) continue
        val top = node.positionInRoot.y
        assertTrue("off screen at the top: \"$text\" at $top", top >= -0.5f)
    }
    for (i in texts.indices) for (j in i + 1 until texts.size) {
        val a = texts[i].boundsInRoot
        val b = texts[j].boundsInRoot
        val overlap = a.intersect(b)
        assertTrue(
            "overlap: \"${texts[i].texts().joinToString()}\" $a and \"${texts[j].texts().joinToString()}\" $b",
            overlap.width <= 0.5f || overlap.height <= 0.5f,
        )
    }
}
```

- [ ] **Step 2: Ver que falla.** Sin las bibliotecas, no compila: `Unresolved reference` de `test` (Compose ui-test), `github` (Roborazzi) y `robolectric`.

```bash expect=fail reason="Unresolved reference 'robolectric'"
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ScreenTestSupportTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -o "Unresolved reference '[a-z]*'" /tmp/adbuddy-red.log | sort -u; exit $s
```

- [ ] **Step 3: Las bibliotecas y la configuración.**

**Editar `android/gradle/libs.versions.toml`:** (cambio 1 de 2) reemplazar

```toml
webkit = "1.14.0"
```

por

```toml
webkit = "1.14.0"
# Tests of the screens in the JVM (plan tests-ui-jvm). robolectricAndroidAll is the Android 14 jar that this Robolectric
# runs (SDK 34, src/test/resources/robolectric.properties): it changes with robolectric.
robolectric = "4.17"
robolectricAndroidAll = "14-robolectric-10818077-i7"
roborazzi = "1.76.0"
```

**Editar `android/gradle/libs.versions.toml`:** (cambio 2 de 2) reemplazar

```toml
androidx-test-ext-junit = { module = "androidx.test.ext:junit", version.ref = "androidxTestExt" }
```

por

```toml
androidx-test-ext-junit = { module = "androidx.test.ext:junit", version.ref = "androidxTestExt" }
compose-ui-test-junit4 = { module = "androidx.compose.ui:ui-test-junit4" }
robolectric = { module = "org.robolectric:robolectric", version.ref = "robolectric" }
robolectric-android-all = { module = "org.robolectric:android-all-instrumented", version.ref = "robolectricAndroidAll" }
roborazzi = { module = "io.github.takahirom.roborazzi:roborazzi", version.ref = "roborazzi" }
```

**Editar `android/app/build.gradle.kts`:** (cambio 1 de 3) reemplazar

```kotlin
    buildFeatures {
        compose = true
    }
```

por

```kotlin
    buildFeatures {
        compose = true
    }

    // Robolectric reads the merged resources and manifest (the screen tests).
    testOptions {
        unitTests.isIncludeAndroidResources = true
    }
```

**Editar `android/app/build.gradle.kts`:** (cambio 2 de 3) reemplazar

```kotlin
dependencies {
    implementation(libs.androidx.core.ktx)
```

por

```kotlin
// Screen tests (plan tests-ui-jvm). Robolectric's Android jar (SDK 34: src/test/resources/robolectric.properties) comes
// through Gradle like any other dependency, so --offline works once it is in the cache: the tests never download it.
val robolectricAndroid: Configuration by configurations.creating { isTransitive = false }
val robolectricJars = layout.buildDirectory.dir("robolectric-jars")
val syncRobolectricJars = tasks.register<Sync>("syncRobolectricJars") {
    from(robolectricAndroid)
    into(robolectricJars)
}
// Golden images (src/test/screenshots): -Padbuddy.screens=record records them; verify (the default on Windows, where they
// are recorded) fails on any changed pixel; compare (the default elsewhere) writes the differences to build/outputs/roborazzi
// and never fails, until the same images are shown to come out on that system too.
val screens = providers.gradleProperty("adbuddy.screens")
    .getOrElse(if (System.getProperty("os.name").startsWith("Windows")) "verify" else "compare")
require(screens in setOf("record", "verify", "compare")) { "adbuddy.screens is record, verify or compare, not $screens" }
tasks.withType<Test>().configureEach {
    dependsOn(syncRobolectricJars)
    systemProperty("robolectric.offline", "true")
    systemProperty("robolectric.dependency.dir", robolectricJars.get().asFile.absolutePath)
    inputs.files(fileTree("src/test/screenshots")).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("roborazzi.test.record", (screens == "record").toString())
    systemProperty("roborazzi.test.verify", (screens == "verify").toString())
    systemProperty("roborazzi.test.compare", (screens == "compare").toString())
}

dependencies {
    implementation(libs.androidx.core.ktx)
```

**Editar `android/app/build.gradle.kts`:** (cambio 3 de 3) reemplazar

```kotlin
    testImplementation(libs.okhttp.mockwebserver)
```

por

```kotlin
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(platform(libs.compose.bom))
    testImplementation(libs.compose.ui.test.junit4)
    testImplementation(libs.robolectric)
    testImplementation(libs.roborazzi)
    robolectricAndroid(libs.robolectric.android.all)
```

**Crear `android/app/src/test/resources/robolectric.properties`:**

```properties
sdk=34
```

**Editar `.gitattributes`:** reemplazar

```text
android/.gitignore text eol=lf
```

por

```text
android/.gitignore text eol=lf
# The golden images of the screen tests: never touched by line-ending conversion
android/**/*.png binary
```

- [ ] **Step 4: Ver que pasa.** (En otra máquina sin estas bibliotecas en la caché, esta primera vez sin `--offline`.)

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ScreenTestSupportTest'
```

- [ ] **Step 5: La suite entera, el APK y lint** (como en `android.yml`). La suite no cambia de resultado: los tests existentes no usan Robolectric.

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest --offline
```

- [ ] **Step 6: Commit.**

```bash
git add .gitattributes android/gradle/libs.versions.toml android/app/build.gradle.kts android/app/src/test/resources/robolectric.properties android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupport.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ScreenTestSupportTest.kt
git commit -m "$(cat <<'EOF'
test(android): Robolectric, Compose ui-test and Roborazzi for the screens

Robolectric 4.17 on SDK 34 (Java 17), its Android jar through Gradle so
--offline works, and Roborazzi without its plugin: adbuddy.screens picks
record, verify (Windows) or compare. assertTextsFit, with its own test,
finds texts cut short, off screen or drawn over each other.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Imágenes del onboarding y de Ajustes

**Revisor:** `adbuddy-reviewer` (mira las 20 imágenes).

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/Samples.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/OnboardingScreensTest.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/SettingsScreensTest.kt`
- Create (generadas en el Step 3): `android/app/src/test/screenshots/onboarding_1_protected_apps_{light,dark,font_200,font_200_es}.png`, `onboarding_2_notification_access_{…}.png`, `settings_proposed_{…}.png`, `settings_confirmed_{…}.png`, `onboarding_1_protected_apps_rows_{…}.png`

**Interfaces:** usa `PHONE`, `ScreenMode`, `screenModes()`, `ScreenModeRule`, `ComponentActivityRule`, `shot`, `assertTextsFit` (Tarea 1); crea `SAMPLE_ROWS` (la usa la Tarea 3).

- [ ] **Step 1: Los tests.** Validación 27: paso 1 de 2 (con su "Ahora no" y el botón de confirmar) y paso 2 de 2 del onboarding; Ajustes con la lista propuesta y el acceso por confirmar marcados (aviso rojo con icono) y la tarjeta del modo remoto con su interruptor, y Ajustes con la lista confirmada, el acceso permitido y el PC desconectado (la tarjeta sin interruptor); el paso 2 con el acceso ya permitido no ofrece "Ahora no". En cada modo: textos presentes, `assertTextsFit`, el título dentro de la barra, la imagen, y lo que queda por debajo se alcanza con scroll; en la lista del paso 1, tras cada scroll se vuelven a comprobar los textos de las filas (nombre, paquete, motivo) y se graba una segunda imagen con las filas a la vista (`onboarding_1_protected_apps_rows`): al 200 % la cabecera y la barra inferior tapan la lista entera.

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/Samples.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import tech.hybriq.adbuddy.ui.ProtectedAppsUi
import tech.hybriq.adbuddy.ui.ProtectedAppsUi.Reason

/** The rows of the samples: a code receiver, a guess and an app nobody proposed (as in the previews). */
val SAMPLE_ROWS = listOf(
    ProtectedAppsUi.Row("com.google.android.gm", "Gmail", true, Reason.CODES),
    ProtectedAppsUi.Row("com.example.mybank", "Mi Banco", true, Reason.GUESS),
    ProtectedAppsUi.Row("com.whatsapp", "WhatsApp", false, Reason.NONE),
)
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/OnboardingScreensTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToKey
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.NotificationAccessContent
import tech.hybriq.adbuddy.ui.OnboardingStep
import tech.hybriq.adbuddy.ui.ProtectedAppsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: the two first-run steps, in every ScreenMode (golden images and the texts on screen). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class OnboardingScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int, vararg args: Any): String = RuntimeEnvironment.getApplication().getString(id, *args)

    @Test fun stepOneProtectedApps() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ProtectedAppsContent(OnboardingStep.PROTECTED_APPS, false, false, {}, SAMPLE_ROWS, 2, true, "", {}, { _, _ -> }, true, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_proposal)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_count, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_confirm)).assertIsDisplayed().assertIsEnabled()
        compose.assertTextsFit()
        compose.shot("onboarding_1_protected_apps", mode)
        // A lazy list only composes the rows in view: scroll by key, then look, and check the rows' texts too.
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.google.android.gm")
        compose.onNodeWithText("Gmail").assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
        compose.shot("onboarding_1_protected_apps_rows", mode)
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.example.mybank")
        compose.onNodeWithText("Mi Banco").assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
    }

    @Test fun stepTwoNotificationAccess() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                NotificationAccessContent(OnboardingStep.NOTIFICATION_ACCESS, false, false, Effective.OFF, {}, {}, {}, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_done)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("onboarding_2_notification_access", mode)
        compose.onNodeWithText(text(R.string.access_open)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_codes_hint)).performScrollTo().assertIsDisplayed()
    }

    @Test fun stepTwoWithTheAccessOnHasNoNotNow() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                NotificationAccessContent(OnboardingStep.NOTIFICATION_ACCESS, false, true, Effective.ON, {}, {}, {}, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_done)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        compose.assertTextsFit()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/SettingsScreensTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.RemoteState
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.RemoteCard
import tech.hybriq.adbuddy.ui.SettingsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: Settings (its rows, the "proposed" warning and the remote-mode card), in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class SettingsScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun show(count: Int, confirmed: Boolean, access: Effective, state: BridgeState) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                SettingsContent(count, confirmed, access, onOpen = {}, onBack = {}, remoteCard = { RemoteCard(state, app.bridge, app.clock, onMessage = {}) })
            }
        }
    }

    @Test fun aProposedListAndAnAccessToConfirmAreFlagged() {
        show(12, false, Effective.NEEDS_CONFIRMATION, BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = false)))
        compose.onNodeWithText(text(R.string.settings_protected_proposed, 12)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_off)).assertIsDisplayed()
        compose.onNode(isToggleable()).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_proposed", mode)
    }

    @Test fun aConfirmedListAndTheAccessOnWithThePcAway() {
        show(3, true, Effective.ON, BridgeState())
        compose.onNodeWithText(text(R.string.settings_protected_confirmed, 3)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_disconnected)).assertIsDisplayed()
        // Not connected: no switch, which would read as "off".
        compose.onNode(isToggleable()).assertDoesNotExist()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_confirmed", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
```

- [ ] **Step 2: Ver que fallan.** En Windows se verifica, y aún no hay imágenes.

```bash expect=fail reason="was not found"
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingScreensTest' --tests '*SettingsScreensTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'tests completed' /tmp/adbuddy-red.log; grep -h -o 'The original file([^)]*) was not found' app/build/test-results/testDebugUnitTest/*.xml | head -3; exit $s
```

- [ ] **Step 3: Grabar las imágenes.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingScreensTest' --tests '*SettingsScreensTest' -Padbuddy.screens=record && ls app/src/test/screenshots/ | grep -c png | grep -qx 20
```

- [ ] **Step 4: Mirarlas** (Read de los PNG; nada se da por bueno sin verlo). En las 20: los textos enteros y legibles; oscuro de verdad oscuro; al 200 % la letra grande y nada pisado; `*_font_200_es` en español y las demás en inglés. En concreto: `onboarding_1_protected_apps_font_200_es.png` ("Paso 1 de 2", el aviso de propuesta, la barra inferior con "Ahora no" y "Confirmar la lista" en dos líneas, sin solaparse), `onboarding_2_notification_access_dark.png` (tarjeta "Not allowed" con su icono, "Not now" y "Done"), `settings_proposed_light.png` (las dos filas con el triángulo rojo, la tarjeta "Remote mode" con "Off" y el interruptor), `settings_confirmed_font_200.png` (sin avisos rojos salvo "Not connected to the PC" en la tarjeta, sin interruptor), `onboarding_1_protected_apps_rows_font_200_es.png` (las filas con su casilla, nombre, paquete y motivo, enteras). Si algo se ve mal, es un hallazgo de UI: se informa al controlador, va a `KNOWN_UI_ISSUES` si una comprobación puede verlo, y **no** se arregla en este plan.

- [ ] **Step 5: Ver que pasan, verificando.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingScreensTest' --tests '*SettingsScreensTest'
```

- [ ] **Step 6: El test muerde** (la letra al 200 %): con `maxLines = 1` en el resumen de las filas de Ajustes, el texto que al 200 % ocupa dos líneas sale cortado, y solo fallan los dos modos al 200 % (al 100 % cabe en una y la imagen no cambia).

```bash expect=fail reason="aProposedListAndAnAccessToConfirmAreFlagged[FONT_200] FAILED"
cd android && sed -i 's/Text(summary, color =/Text(summary, maxLines = 1, color =/' app/src/main/kotlin/tech/hybriq/adbuddy/ui/SettingsScreen.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*SettingsScreensTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; grep -h -o -m1 'cut short: [^&]*' app/build/test-results/testDebugUnitTest/*.xml; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main && cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*SettingsScreensTest'
```

- [ ] **Step 7: Commit.**

```bash
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/Samples.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/OnboardingScreensTest.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/SettingsScreensTest.kt android/app/src/test/screenshots/
git commit -m "$(cat <<'EOF'
test(android): golden images of the first-run steps and Settings

Validation 27 in the JVM: both steps and Settings (the proposed list and
the access to confirm flagged, the remote-mode card) in light, dark, text
at 200 % and 200 % in Spanish, with the texts whole, on screen and apart.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Imágenes de la lista de apps protegidas y del acceso a notificaciones

**Revisor:** `adbuddy-reviewer` (mira las 28 imágenes).

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ProtectedAppsScreensTest.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/NotificationAccessScreensTest.kt`
- Create (generadas en el Step 3): `android/app/src/test/screenshots/protected_apps_settings_{…}.png`, `protected_apps_no_match_{…}.png`, `access_allowed_{…}.png`, `access_needs_confirmation_{…}.png`, `access_not_allowed_{…}.png`, `access_unknown_{…}.png`, `access_checking_{…}.png`

**Interfaces:** usa lo de la Tarea 1 y `SAMPLE_ROWS` (Tarea 2).

- [ ] **Step 1: Los tests.** La lista desde Ajustes (flecha atrás, sin paso ni "Ahora no" ni aviso de propuesta, "Guardar" activo, una app de códigos marcada y otra sin marcar) y una búsqueda sin resultados; el acceso desde Ajustes en sus cuatro estados y "comprobando", cada uno con los botones que le tocan y sin los que no (confirmar solo en "sin confirmar", retirar solo con la marca, también con el estado desconocido, el aviso de ajustes restringidos solo en "no permitido" y "desconocido"). Tras cada scroll de la lista se comprueban otra vez los textos de las filas, y el título de cada pantalla dentro de la barra superior (con el fallo conocido del 200 % en español en `KNOWN_UI_ISSUES`).

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ProtectedAppsScreensTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToKey
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.ProtectedAppsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: the list of protected apps from Settings (with a change to save, and a search with no match). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ProtectedAppsScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int, vararg args: Any): String = RuntimeEnvironment.getApplication().getString(id, *args)

    @Test fun fromSettingsWithAChangeToSave() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { ProtectedAppsContent(null, false, false, {}, SAMPLE_ROWS, 2, false, "", {}, { _, _ -> }, true, {}, {}, {}) }
        }
        compose.onNodeWithContentDescription(text(R.string.action_back)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.protected_proposal)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsDisplayed().assertIsEnabled()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.protected_title), knownUiIssue(R.string.protected_title, mode))
        compose.shot("protected_apps_settings", mode)
        // A lazy list only composes the rows in view: scroll by key, then look.
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.google.android.gm")
        compose.onNodeWithText("Gmail").assertIsDisplayed().assertIsOn()
        compose.onNodeWithText(text(R.string.protected_reason_codes)).assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.whatsapp")
        compose.onNodeWithText("WhatsApp").assertIsDisplayed().assertIsOff()
        compose.assertTextsFit(scrolled = true)
    }

    @Test fun aSearchWithNoMatchSaysSo() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { ProtectedAppsContent(null, false, false, {}, emptyList(), 2, false, "zzz", {}, { _, _ -> }, false, {}, {}, {}) }
        }
        compose.onNodeWithText("zzz").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_empty)).performScrollTo().assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("protected_apps_no_match", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/NotificationAccessScreensTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.NotificationAccessContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Validation 27: the notification access screen from Settings in each of its four states (the validation says three:
 * it was written before ruling 15 of M5c added "allowed in the system, not confirmed here") and while it is checking.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationAccessScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int): String = RuntimeEnvironment.getApplication().getString(id)

    private fun show(effective: Effective, confirmed: Boolean, checking: Boolean = false) {
        compose.setContent { AdbuddyTheme(dynamic = false) { NotificationAccessContent(null, checking, confirmed, effective, {}, {}, {}, {}, {}, {}) } }
    }

    /** The state's title and detail, the buttons it shows and the ones it must not; then the texts and the image. */
    private fun check(name: String, title: Int, detail: Int?, shown: List<Int>, hidden: List<Int>) {
        compose.onNodeWithText(text(title)).assertIsDisplayed()
        if (detail != null) compose.onNodeWithText(text(detail)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.access_title), knownUiIssue(R.string.access_title, mode))
        compose.shot(name, mode)
        for (id in shown) compose.onNodeWithText(text(id)).performScrollTo().assertIsDisplayed()
        for (id in hidden) compose.onNodeWithText(text(id)).assertDoesNotExist()
    }

    @Test fun allowed() {
        show(Effective.ON, confirmed = true)
        check(
            "access_allowed", R.string.access_state_granted, R.string.access_detail_granted,
            shown = listOf(R.string.access_withdraw, R.string.access_open),
            hidden = listOf(R.string.access_confirm, R.string.access_restricted_hint),
        )
    }

    @Test fun allowedInTheSystemNotConfirmedHere() {
        show(Effective.NEEDS_CONFIRMATION, confirmed = false)
        check(
            "access_needs_confirmation", R.string.access_state_confirm, R.string.access_detail_confirm,
            shown = listOf(R.string.access_confirm, R.string.access_open),
            hidden = listOf(R.string.access_withdraw, R.string.access_restricted_hint),
        )
    }

    @Test fun notAllowed() {
        show(Effective.OFF, confirmed = false)
        check(
            "access_not_allowed", R.string.access_state_denied, R.string.access_detail_denied,
            shown = listOf(R.string.access_open, R.string.access_restricted_hint),
            hidden = listOf(R.string.access_confirm, R.string.access_withdraw),
        )
    }

    @Test fun unknown() {
        show(Effective.UNKNOWN, confirmed = false)
        check(
            "access_unknown", R.string.access_state_unknown, R.string.access_detail_unknown,
            shown = listOf(R.string.access_open, R.string.access_restricted_hint),
            hidden = listOf(R.string.access_confirm, R.string.access_withdraw),
        )
    }

    @Test fun unknownWithTheMarkStillOffersToWithdrawIt() {
        // Withdrawing only takes permission away: it shows whenever the mark is there, even with the state unknown.
        show(Effective.UNKNOWN, confirmed = true)
        compose.onNodeWithText(text(R.string.access_state_unknown)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_withdraw)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
        compose.assertTextsFit()
    }

    @Test fun checking() {
        show(Effective.UNKNOWN, confirmed = false, checking = true)
        check(
            "access_checking", R.string.access_state_checking, null,
            shown = listOf(R.string.access_open),
            hidden = listOf(R.string.access_state_unknown, R.string.access_confirm, R.string.access_withdraw, R.string.access_restricted_hint),
        )
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
```

- [ ] **Step 2: Ver que fallan** (sin imágenes).

```bash expect=fail reason="was not found"
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ProtectedAppsScreensTest' --tests '*NotificationAccessScreensTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'tests completed' /tmp/adbuddy-red.log; grep -h -o 'The original file([^)]*) was not found' app/build/test-results/testDebugUnitTest/*.xml | head -3; exit $s
```

- [ ] **Step 3: Grabar.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ProtectedAppsScreensTest' --tests '*NotificationAccessScreensTest' -Padbuddy.screens=record && ls app/src/test/screenshots/ | grep -c png | grep -qx 48
```

- [ ] **Step 4: Mirarlas** (Read de los PNG). En concreto: `protected_apps_settings_font_200_es.png` (flecha atrás, buscador, "Guardar" en la barra inferior), `protected_apps_no_match_dark.png` ("zzz" en el buscador y "No app matches."), `access_needs_confirmation_font_200_es.png` (el título "Acceso a las notificaciones" en dos líneas y fuera de los 64 dp de la barra: **hallazgo conocido** de `KNOWN_UI_ISSUES`, se informa y no se da por bueno), y las cinco del acceso en oscuro (`access_*_dark.png`): cada estado con su icono y su color (marca, información, cruz, aviso rojo, refrescar), y "comprobando" sin decir ni permitido ni no permitido. Lo que se vea mal se informa; no se arregla aquí.

- [ ] **Step 5: Ver que pasan, verificando.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ProtectedAppsScreensTest' --tests '*NotificationAccessScreensTest'
```

- [ ] **Step 6: El test muerde** (un cambio que ningún texto delata): 14 dp de relleno en vez de 16 en las tarjetas del acceso hace fallar sus imágenes.

```bash expect=fail reason="is changed"
cd android && sed -i 's/padding(16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)/padding(14.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)/' app/src/main/kotlin/tech/hybriq/adbuddy/ui/NotificationAccessScreen.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*NotificationAccessScreensTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'tests completed' /tmp/adbuddy-red.log; grep -h -o 'access_[a-z_0-9]*\.png is changed' app/build/test-results/testDebugUnitTest/*.xml | head -3; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main && rm -rf android/app/build/outputs/roborazzi && cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*NotificationAccessScreensTest'
```

- [ ] **Step 7: Commit.**

```bash
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/ProtectedAppsScreensTest.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/screens/NotificationAccessScreensTest.kt android/app/src/test/screenshots/
git commit -m "$(cat <<'EOF'
test(android): golden images of the protected apps and notification access

Validation 27 in the JVM: the list from Settings (a change to save, a
search with no match) and the access screen in its four states and while
checking, each with only its own buttons, in every mode.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Flujos de la validación 27 (onboarding y lista de apps)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/Launcher.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/OnboardingFlowTest.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/ProtectedAppsFlowTest.kt`

**Interfaces:** `installLauncherApps(vararg apps: Pair<String, String>)` (apps con icono en el lanzador, como las lee `ProtectedApps.launcherApps`); usa `PHONE` (Tarea 1).

- [ ] **Step 1: Los tests.** En `MainActivity` (primer arranque): "Ahora no" del paso 1 lleva al 2, atrás vuelve al 1, "Ahora no" dos veces llega al chat y marca el onboarding hecho sin confirmar la lista; confirmar la lista y "Hecho" lo terminan; atrás en el paso 1 sale de la app sin saltarse los pasos. En la lista de apps desde Ajustes (por la burbuja, que no tiene onboarding), con una lista ya confirmada: la búsqueda encuentra por el nombre ("banco" solo está en "Mi Banco", no en su paquete), sin distinguir mayúsculas, y dice cuándo no hay nada; marcar una app y rotar conserva la selección y "Guardar" sigue activo; salir con cambios pregunta, con la flecha y con el atrás del sistema, y "Descartar" los tira (la lista guardada no cambia) y "Cancelar" los deja; guardar confirma la lista, avisa de que el PC la recibirá al conectar (no hay PC en el test) y Ajustes dice "Lista confirmada".

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/Launcher.kt`:**

```kotlin
package tech.hybriq.adbuddy.flows

import android.content.Intent
import android.content.pm.ActivityInfo
import android.content.pm.ApplicationInfo
import android.content.pm.ResolveInfo
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf

/** Apps with an icon in the launcher, as ProtectedApps.launcherApps reads them (package to label). */
fun installLauncherApps(vararg apps: Pair<String, String>) {
    val app = RuntimeEnvironment.getApplication()
    val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
    for ((pkg, label) in apps) {
        shadowOf(app.packageManager).addResolveInfoForIntent(
            launcher,
            ResolveInfo().apply {
                nonLocalizedLabel = label
                activityInfo = ActivityInfo().apply {
                    packageName = pkg
                    name = "$pkg.Main"
                    applicationInfo = ApplicationInfo().apply { packageName = pkg }
                }
            },
        )
    }
}
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/OnboardingFlowTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.flows

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.Onboarding

/**
 * Validation 27, the first-run steps in the app (MainActivity, firstRun = true): step 1 and step 2, "Not now",
 * and Back (from step 2 to step 1; from step 1 it leaves the app, never into the chat past the steps).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class OnboardingFlowTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun step(n: Int) = compose.onNodeWithText(text(R.string.step_progress, n, 2))

    private fun pressBack() = compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }

    @Test fun notNowTwiceReachesTheChatAndBackReturnsAStep() {
        step(1).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        step(2).assertIsDisplayed()
        pressBack()
        step(1).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        step(2).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        step(1).assertDoesNotExist()
        step(2).assertDoesNotExist()
        assertTrue(Onboarding.of(app).done())
        // "Not now" on step 1 saved nothing: the list is still a proposal.
        assertFalse(ProtectedApps.of(app).confirmed)
    }

    @Test fun confirmingTheListThenDoneFinishesTheSteps() {
        // The apps are read off the main thread: the button turns on once they are there.
        compose.waitUntil(5_000) { compose.onAllNodes(hasText(text(R.string.protected_confirm)) and isEnabled()).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.protected_confirm)).performClick()
        step(2).assertIsDisplayed()
        assertTrue(ProtectedApps.of(app).confirmed)
        compose.onNodeWithText(text(R.string.onboarding_done)).performClick()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        assertTrue(Onboarding.of(app).done())
    }

    @Test fun backOnTheFirstStepLeavesWithoutSkippingIt() {
        step(1).assertIsDisplayed()
        pressBack()
        assertTrue(compose.activity.isFinishing)
        assertFalse(Onboarding.of(app).done())
    }
}
```

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/ProtectedAppsFlowTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.flows

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToKey
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.shadows.ShadowToast
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.screens.PHONE

/**
 * Validation 27, the list of apps from Settings (reached from the bubble, which has no first-run steps): the search,
 * a rotation that keeps the selection, and leaving with changes (discard or keep them), and saving.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ProtectedAppsFlowTest {
    @get:Rule val compose = createAndroidComposeRule<ChatBubbleActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    @Before fun apps() {
        installLauncherApps("com.example.mybank" to "Mi Banco", "com.whatsapp" to "WhatsApp", "org.example.notes" to "Notes")
        // A list the person already confirmed: the screen opens in "edit" mode, with Save off until something changes.
        ProtectedApps.of(app).confirm(setOf("com.example.mybank"))
    }

    private fun openProtectedApps() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        // The apps are read off the main thread: wait for the rows.
        compose.waitUntil(5_000) { compose.onAllNodes(hasText("Mi Banco")).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun row(pkg: String, label: String) = compose.run {
        onNode(hasScrollToKeyAction()).performScrollToKey(pkg)
        onNodeWithText(label)
    }

    private fun settingsShown() = compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()

    @Test fun theSearchFiltersByNameAndSaysWhenNothingMatches() {
        openProtectedApps()
        // "banco" is only in the name (the package is com.example.mybank), as a person searches.
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("banco")
        compose.onNodeWithText("Mi Banco").assertIsDisplayed()
        compose.onNodeWithText("WhatsApp").assertDoesNotExist()
        compose.onNodeWithText("Notes").assertDoesNotExist()
        compose.onNodeWithText("banco").performTextClearance()
        // Lower case finds the name in any case.
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("whatsapp")
        compose.onNodeWithText("WhatsApp").assertIsDisplayed()
        compose.onNodeWithText("Mi Banco").assertDoesNotExist()
        compose.onNodeWithText("whatsapp").performTextClearance()
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("zzz")
        compose.onNodeWithText(text(R.string.protected_empty)).assertIsDisplayed()
    }

    @Test fun aRotationKeepsTheSelection() {
        openProtectedApps()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsNotEnabled()
        row("com.whatsapp", "WhatsApp").assertIsOff().performClick()
        row("com.whatsapp", "WhatsApp").assertIsOn()

        RuntimeEnvironment.setQualifiers("+land")
        compose.activityRule.scenario.recreate()

        compose.waitUntil(5_000) { compose.onAllNodes(hasText(text(R.string.protected_save))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.protected_intro)).assertExists()
        row("com.whatsapp", "WhatsApp").assertIsOn()
        row("com.example.mybank", "Mi Banco").assertIsOn()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsEnabled()
    }

    @Test fun leavingWithChangesAsksAndDiscardDropsThem() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_discard_confirm)).performClick()
        settingsShown()
        assertEquals(setOf("com.example.mybank"), ProtectedApps.of(app).list())
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.waitUntil(5_000) { compose.onAllNodes(hasText("Mi Banco")).fetchSemanticsNodes().isNotEmpty() }
        row("com.whatsapp", "WhatsApp").assertIsOff()
    }

    @Test fun theSystemBackWithChangesAsksToo() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        // Recomposed with the change (a person's Back comes long after the tap; the test's would not).
        row("com.whatsapp", "WhatsApp").assertIsOn()
        // The usual gesture on the phone: the system's Back, not the arrow.
        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_discard_confirm)).performClick()
        settingsShown()
        assertEquals(setOf("com.example.mybank"), ProtectedApps.of(app).list())
    }

    @Test fun cancelKeepsTheChangesOnScreen() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithText(text(R.string.action_cancel)).performClick()
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertDoesNotExist()
        row("com.whatsapp", "WhatsApp").assertIsOn()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsEnabled()
    }

    @Test fun savingConfirmsTheListAndSaysThePcGetsItLater() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithText(text(R.string.protected_save)).performClick()
        settingsShown()
        val saved = ProtectedApps.of(app)
        assertTrue(saved.confirmed)
        assertTrue("com.whatsapp" in saved.list())
        // No PC connected in the test: the list waits for the next connection, and the person is told so.
        assertEquals(text(R.string.protected_saved_later), ShadowToast.getTextOfLatestToast())
        compose.onNodeWithText(text(R.string.settings_protected_confirmed, saved.list().size)).assertIsDisplayed()
    }
}
```

- [ ] **Step 2: Ver que pasan** (prueban código que ya existe: el fallo "esperado" se provoca en el Step 3).

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingFlowTest' --tests '*ProtectedAppsFlowTest'
```

- [ ] **Step 3: Los tests muerden.** (a) Si atrás en el paso 2 terminara el onboarding en vez de volver al paso 1, falla el flujo de "Ahora no"; (b) si la selección se guardara con `remember` en vez de `rememberSaveable`, la rotación la perdería; (c) si la búsqueda solo mirara el paquete, "banco" no encontraría "Mi Banco".

```bash expect=fail reason="notNowTwiceReachesTheChatAndBackReturnsAStep FAILED"
cd android && sed -i 's/NotificationAccessScreen(first, onBack = { step = first.previous() }/NotificationAccessScreen(first, onBack = finish/' app/src/main/kotlin/tech/hybriq/adbuddy/ui/ChatRoot.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingFlowTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main
```

```bash expect=fail reason="aRotationKeepsTheSelection FAILED"
cd android && sed -i 's/var selection by rememberSaveable(stateSaver = SelectionSaver) {/var selection by remember {/' app/src/main/kotlin/tech/hybriq/adbuddy/ui/ProtectedAppsScreen.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ProtectedAppsFlowTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main
```

```bash expect=fail reason="theSearchFiltersByNameAndSaysWhenNothingMatches FAILED"
cd android && sed -i 's/q.isEmpty() || it.label.lowercase().contains(q) || /q.isEmpty() || /' app/src/main/kotlin/tech/hybriq/adbuddy/ui/ProtectedAppsUi.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*ProtectedAppsFlowTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main && cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*OnboardingFlowTest' --tests '*ProtectedAppsFlowTest'
```

- [ ] **Step 4: Commit.**

```bash
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/Launcher.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/OnboardingFlowTest.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/ProtectedAppsFlowTest.kt
git commit -m "$(cat <<'EOF'
test(android): first-run steps and protected apps flows in the JVM

Validation 27: Not now, Back and Done through both steps in MainActivity;
the list from Settings searched, rotated with the selection kept, left
with changes (discard or cancel) and saved while the PC is away.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: La burbuja llega a Ajustes y el botón de confirmar funciona (validación 34)

**Revisor:** `adbuddy-reviewer-opus` (la confirmación del acceso a notificaciones es un control de seguridad: ruling 15 de M5c).

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/BubbleFlowTest.kt`

**Interfaces:** usa `PHONE` (Tarea 1).

- [ ] **Step 1: Los tests.** En `ChatBubbleActivity` tal cual (`firstRun = false`): no monta el onboarding ni lo da por hecho; el menú lleva a Ajustes, y de ahí a la lista de apps (sin "Paso 1 de 2") y al acceso a notificaciones (sin "Paso 2 de 2"), y atrás vuelve hasta el chat; con el acceso permitido en el sistema y el listener enlazado, la pantalla dice "Permitido en el sistema, sin confirmar aquí", el botón "Dejar que el agente lea mis notificaciones" pone la marca (`NotificationConsent`) y la pantalla pasa a "Permitido" con "retirar" y sin el botón; y si el sistema retira el acceso después de pintar el botón y antes del toque, el toque vuelve a preguntar al sistema, **no** pone la marca y la pantalla pasa a "Desconocido" (la guarda `allowedNow` del ruling 15 de M5c). El test no pone la marca: solo la lee.

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/BubbleFlowTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.flows

import android.app.NotificationManager
import android.content.ComponentName
import android.provider.Settings
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.AgentNotificationListener
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.Onboarding

/**
 * Validation 34 (ruling 16 of M5c): the chat bubble (ChatBubbleActivity, firstRun = false) never shows the first-run
 * steps, and its menu reaches Settings, the protected apps and the notification access screen, whose confirm button
 * works. What needs a phone stays manual: `adbuddy tap` and `adb shell input tap` against ADBuddy in front.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class BubbleFlowTest {
    @get:Rule val compose = createAndroidComposeRule<ChatBubbleActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private var listener: AgentNotificationListener? = null

    @After fun unbind() {
        // The listener's state is a process-wide flow: leave it unbound for the next test (this also clears the mark).
        listener?.onListenerDisconnected()
    }

    private fun openSettings() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()
    }

    private fun back() {
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
    }

    @Test fun theBubbleNeverShowsTheFirstRunSteps() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        // And it does not mark them done either: the app still shows them on its first run.
        assertFalse(Onboarding.of(app).done())
    }

    @Test fun settingsLeadsToTheProtectedAppsAndTheNotificationAccess() {
        openSettings()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.onNodeWithText(text(R.string.protected_intro)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        back()
        compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_title)).performClick()
        compose.onNodeWithText(text(R.string.access_intro)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertDoesNotExist()
        back()
        back()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
    }

    private val component get() = ComponentName(app, AgentNotificationListener::class.java)

    /** The system allows the listener and has it bound (what the person does in the system's settings); the confirm button shows. */
    private fun allowInTheSystemAndOpenTheAccess() {
        Settings.Secure.putString(app.contentResolver, "enabled_notification_listeners", component.flattenToString())
        shadowOf(app.getSystemService(NotificationManager::class.java)).setNotificationListenerAccessGranted(component, true)
        listener = Robolectric.buildService(AgentNotificationListener::class.java).create().get().also { it.onListenerConnected() }
        assertFalse(NotificationConsent.of(app).confirmed())
        openSettings()
        compose.onNodeWithText(text(R.string.access_title)).performClick()
        compose.waitUntil(5_000) { compose.onAllNodes(hasText(text(R.string.access_confirm))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
    }

    @Test fun theConfirmButtonLetsTheAgentRead() {
        allowInTheSystemAndOpenTheAccess()
        val consent = NotificationConsent.of(app)
        compose.onNodeWithText(text(R.string.access_confirm)).performScrollTo().performClick()

        compose.waitUntil(5_000) { consent.confirmed() }
        compose.waitUntil(5_000) { compose.onAllNodes(hasText(text(R.string.access_withdraw))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
        assertTrue(consent.confirmed())
    }

    @Test fun theConfirmButtonAsksTheSystemAgainAtTheClick() {
        allowInTheSystemAndOpenTheAccess()
        // Revoked in the system after the screen drew the button, before the tap (only the system's own answer changes,
        // so nothing repaints the screen): the click reads the system again and must not set the mark.
        shadowOf(app.getSystemService(NotificationManager::class.java)).setNotificationListenerAccessGranted(component, false)
        compose.onNodeWithText(text(R.string.access_confirm)).performScrollTo().performClick()
        // The click found the system no longer saying "allowed" and read the state again: the two sources now disagree.
        compose.waitUntil(5_000) { compose.onAllNodes(hasText(text(R.string.access_state_unknown))).fetchSemanticsNodes().isNotEmpty() }
        assertFalse(NotificationConsent.of(app).confirmed())
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
    }
}
```

- [ ] **Step 2: Ver que pasan.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*BubbleFlowTest'
```

- [ ] **Step 3: El test muerde:** con `firstRun = true`, la burbuja monta el onboarding.

```bash expect=fail reason="theBubbleNeverShowsTheFirstRunSteps FAILED"
cd android && sed -i 's/firstRun = false/firstRun = true/' app/src/main/kotlin/tech/hybriq/adbuddy/ChatBubbleActivity.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*BubbleFlowTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main
```

Y sin la segunda pregunta al sistema en el toque (`if (true)` en vez de `if (allowedNow)`), el toque pondría la marca con el acceso ya retirado:

```bash expect=fail reason="theConfirmButtonAsksTheSystemAgainAtTheClick FAILED"
cd android && sed -i 's/if (allowedNow) withContext/if (true) withContext/' app/src/main/kotlin/tech/hybriq/adbuddy/ui/NotificationAccessScreen.kt && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*BubbleFlowTest' > /tmp/adbuddy-red.log 2>&1; s=$?; grep -E 'FAILED|tests completed' /tmp/adbuddy-red.log; exit $s
```

```bash expect=pass
git checkout -- android/app/src/main && git diff --exit-code android/app/src/main && cd android && gradle --no-daemon :app:testDebugUnitTest --offline --tests '*BubbleFlowTest'
```

- [ ] **Step 4: Commit.**

```bash
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/flows/BubbleFlowTest.kt
git commit -m "$(cat <<'EOF'
test(android): the bubble reaches Settings and the confirm button works

Validation 34 in the JVM: ChatBubbleActivity never shows the first-run
steps, its menu reaches Settings, the protected apps and the notification
access, and with the access allowed the confirm button sets the mark.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: CI, documentación y validaciones

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Modify: `.github/workflows/android.yml`, `android/README.md`, `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md`, `docs/superpowers/plans/pendientes-m3-m5.md`

- [ ] **Step 1: La CI sube las diferencias.**

**Editar `.github/workflows/android.yml`:** reemplazar

```yaml
        run: gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest
```

por

```yaml
        run: gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest
      # The screen tests' golden images that came out different (android/README.md, "Screen tests"): verify fails on
      # Windows, compare only reports elsewhere, so it is said in the run's summary too. The folder exists only when
      # something changed.
      - name: Warn about screen differences
        if: ${{ !cancelled() }}
        run: |
          if [ -d app/build/outputs/roborazzi ]; then
            echo "::warning title=Screen differences::golden images differ (compare mode does not fail off Windows): see the screen-differences artifact"
          fi
      - name: Screen differences
        if: ${{ !cancelled() }}
        uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: screen-differences
          path: android/app/build/outputs/roborazzi
          if-no-files-found: ignore
```

- [ ] **Step 2: README.**

**Editar `android/README.md`:** reemplazar

```markdown
Instrumented tests (`gradle :app:connectedDebugAndroidTest`) need an emulator:
never run them on your own phone.
```

por

```markdown
Instrumented tests (`gradle :app:connectedDebugAndroidTest`) need an emulator:
never run them on your own phone.

### Screen tests

`:app:testDebugUnitTest` also draws the screens in the JVM, with Robolectric (SDK 34, which runs on Java 17), Compose's
`ui-test` and Roborazzi: `src/test/kotlin/.../screens` (the first-run steps, Settings, the protected apps and the
notification access in light, dark, text at 200 % and text at 200 % in Spanish) and `.../flows` (the first-run steps in `MainActivity`, the
protected apps list, and the chat bubble reaching Settings). Each screen is checked in its semantics (the texts are
there, whole, on screen and apart) and against a golden image in `src/test/screenshots/`.

- `-Padbuddy.screens=record` records the images. Record on Windows, only the class you changed
  (`--tests '*SettingsScreensTest'`), and look at every image that changed before you commit it. Recording is
  deterministic: images that did not change stay the same in git.
- On Windows the default is `verify`: a pixel changed beyond Roborazzi's colour tolerance (0.7 %) fails, and
  `build/outputs/roborazzi/*_compare.png` shows where.
- Elsewhere the default is `compare`: the differences are written there too, but do not fail (the images have not been
  shown to come out the same on Linux or macOS). CI warns about it in the run's summary and uploads that folder as
  `screen-differences`.
- A UI fault the tests see but that is not fixed yet goes in `KNOWN_UI_ISSUES` (`screens/ScreenTestSupport.kt`): it
  is let through while it lasts and fails once fixed, so it is never recorded as if it were right.

Robolectric's Android jar is a Gradle dependency (`robolectricAndroid` in `app/build.gradle.kts`), so `--offline` works
once the cache has it; the tests never download anything.
```

- [ ] **Step 3: Validaciones 27 y 34 del plan de M5c** (en CRLF: anclas de una línea).

**Editar `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md`:** (cambio 1 de 2) reemplazar

```markdown
con el host apagado, el aviso dice que la recibirá al conectar.
```

por

```markdown
con el host apagado, el aviso dice que la recibirá al conectar. **En la JVM desde el plan `2026-10-03-adbuddy-tests-ui-jvm.md`** (`screens/` y `flows/` de los tests de la app, con imágenes de referencia): todas las pantallas en claro, oscuro, letra al 200 % y letra al 200 % en español (el acceso, en sus cuatro estados y "comprobando"), los pasos con "Ahora no" y atrás, la búsqueda, la rotación, descartar cambios y el aviso con el host apagado. **Sigue para el Pixel:** el color dinámico y las fuentes del sistema, Android 17 (la JVM pinta Android 14), los insets reales y la navegación por gestos, la rotación del móvil, y que el PC reciba la lista (`protected.json`, `adbuddy foreground`). **Hallazgo (plan tests-ui-jvm):** al 200 % en español el título "Acceso a las notificaciones" ocupa dos líneas y se sale de la barra de 64 dp (`access_*_font_200_es.png`, `KNOWN_UI_ISSUES`). Comprobar en el Pixel si pisa la barra de estado o el contenido; el arreglo es de la app.
```

**Editar `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md`:** (cambio 2 de 2) reemplazar

```markdown
Comprobar que la burbuja no monta el onboarding.
```

por

```markdown
Comprobar que la burbuja no monta el onboarding. **En la JVM** (`flows/BubbleFlowTest`, plan `2026-10-03-adbuddy-tests-ui-jvm.md`): la burbuja sin onboarding, el camino a Ajustes, Apps protegidas y la pantalla de acceso, y el botón de confirmar. **Sigue para el Pixel:** todo lo de `adbuddy tap`, `adb shell input tap` y la guarda (`dumpsys`), y la burbuja de verdad (Bubbles API).
```

**Editar `docs/superpowers/plans/pendientes-m3-m5.md`:** reemplazar

```markdown
    - **La burbuja de M4 llega a Ajustes** (también a `Route.PROTECTED_APPS` y a `NotificationAccessScreen`): decisión explícita (ruling de T13): el toque de confirmar es del usuario, ADBuddy es protegida y la guarda bloquea con ADBuddy delante. Desmarcar apps desde la burbuja amplía lo que el agente ve: matizar el KDoc de `ChatBubbleActivity` (validación 34).
```

por

```markdown
    - **La burbuja de M4 llega a Ajustes** (también a `Route.PROTECTED_APPS` y a `NotificationAccessScreen`): decisión explícita (ruling de T13): el toque de confirmar es del usuario, ADBuddy es protegida y la guarda bloquea con ADBuddy delante. Desmarcar apps desde la burbuja amplía lo que el agente ve: matizar el KDoc de `ChatBubbleActivity` (validación 34).
    - **Validaciones 27 y 34 en la JVM** (plan `2026-10-03-adbuddy-tests-ui-jvm.md`): Robolectric (SDK 34, por Java 17), Compose `ui-test` y Roborazzi; imágenes de referencia verificadas al píxel en Windows y solo comparadas en Linux y macOS hasta comprobar que salen iguales (validación 1 de ese plan). Lo que sigue necesitando el Pixel está en las validaciones 27 y 34 de M5c. `android.yml` corre hoy en `ubuntu-latest` salvo que `CI_RUNNER_ANDROID` diga otra cosa: ahí las imágenes solo avisan (`::warning`).
    - **Hallazgo de UI (plan tests-ui-jvm):** al 200 % en español el título "Acceso a las notificaciones" ocupa dos líneas y se sale de la barra superior de 64 dp (`KNOWN_UI_ISSUES` en `screens/ScreenTestSupport.kt`). Comprobarlo en el Pixel (validación 27 de M5c); el arreglo es de la app y, al hacerlo, se quita la entrada y se regraban sus imágenes.
    - **Sesiones que toquen el onboarding, Ajustes, la lista de apps protegidas o el acceso a notificaciones:** en Windows `testDebugUnitTest` verifica sus imágenes; en la misma tarea, regrabar solo la clase afectada (`-Padbuddy.screens=record --tests '*SettingsScreensTest'`), mirar cada imagen que cambie y hacer commit de ellas.
```

- [ ] **Step 4: Todo lo de la CI, con la suite entera.**

```bash expect=pass
cd android && gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest --offline
```

- [ ] **Step 5: Commit.**

```bash
git add .github/workflows/android.yml android/README.md docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "$(cat <<'EOF'
docs, ci: screen tests in the JVM and what stays for the phone

android.yml uploads the golden images that came out different; the
README says how to record and verify them; validations 27 and 34 of M5c
say what the JVM covers now and what still needs the Pixel.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
)"
```

---

## Validaciones manuales pendientes

1. **Imágenes en Linux (y macOS), en el primer PR de esta rama:** `android.yml` corre en `ubuntu-latest` salvo que `CI_RUNNER_ANDROID` diga otra cosa (`gh variable get CI_RUNNER_ANDROID`). ¿Sale el aviso "Screen differences" y el artefacto `screen-differences`? Si no sale (mismos píxeles que en Windows), pasar el valor por defecto a `verify` en todos los sistemas (una línea de `app/build.gradle.kts`). Si sale, lo decide el usuario: una tolerancia, imágenes por sistema o que `android.yml` corra en el runner Windows propio.
2. **Otra máquina Windows:** si la CI corre en un runner Windows que no es este PC y una imagen falla sin cambio de código, mirar el `_compare.png` (diferencias de suavizado de Skia por CPU) y decidir entre regrabar allí o una tolerancia mínima.
3. **Lo que sigue en el Pixel** (validaciones 27 y 34 de M5c, ya reescritas en la Tarea 6): color dinámico, fuentes y Android 17 reales, insets y gestos, rotación física, la lista en el PC, y todo lo de `adbuddy tap`, `adb shell input tap` y la guarda.

## Autorrevisión del plan

### Réplica tarea a tarea

Un script (`replay.py`, en la carpeta temporal de la sesión, nunca en el repo) lee este documento en orden: escribe cada bloque `Crear`, aplica cada par "reemplazar / por" (falla si el texto no aparece exactamente una vez; en los ficheros CRLF pasa los saltos a CRLF), ejecuta cada bloque `bash expect=…` comprobando el código de salida y, si lleva `reason=`, que la salida contenga ese texto, y ejecuta los bloques de commit. La copia sale de `git archive HEAD` de `test/ui-jvm` (691807a, tras la revisión Opus del plan) en una carpeta del scratchpad, con un repositorio desechable con identidad hybriqtech. Entorno: Windows 11, JDK 17.0.20.1, Gradle 9.8.0 `--no-daemon --offline` (la caché ya tenía las bibliotecas nuevas, descargadas en la prueba de la herramienta).

**Resultado de la última pasada, desde cero y ya con las correcciones de la revisión Opus:** 26 ejecuciones, **todas como se esperaba** (10 `expect=fail` que fallan por la razón escrita y 16 `expect=pass` que pasan), los 6 commits del plan con el trailer exacto, ningún "reemplazar" ambiguo ni ausente, y `git diff --exit-code android/app/src/main` limpio tras cada mutación. Las pasadas anteriores encontraron dos fallos **del propio plan**, ya corregidos: el RED de la Tarea 1 buscaba `Unresolved reference 'robolectric'` en las cinco primeras líneas de error y salía antes `'test'` (ahora lista las referencias sin repetir), y los bloques de RED perdían el código de salida de Gradle tras un `| grep` (ahora lo guardan en `s` antes de filtrar).

### Tiempo, tamaño y descargas

- **Suite** (`:app:testDebugUnitTest --rerun`, todo compilado, reloj de pared con el arranque de Gradle): antes 571 tests en 24,9 s; después 644 (73 nuevos) en 36,7 s: **+12 s** en este PC (entre +12 y +13 s en las pasadas completas).
- **Primera descarga** en una máquina o un runner sin caché: ~160 MB de `org.robolectric` (casi todo `nativeruntime-dist-compat`) más el jar de Android 14 (151 MB), una vez; Roborazzi 1,1 MB.
- **Imágenes:** 48 PNG, 3,3 MB en el repositorio. Grabar es determinista: regrabar solo cambia en git las imágenes que cambian.

### Cobertura frente a las validaciones

| Lo que se miraba a mano | Dónde queda |
|---|---|
| 27: onboarding paso 1 de 2 y 2 de 2, "Ahora no", atrás | Imágenes (Tarea 2) y `OnboardingFlowTest` (Tarea 4: "Ahora no", atrás del 2 al 1, atrás en el 1 sale sin saltarse los pasos, confirmar y "Hecho") |
| 27: Ajustes (filas, aviso de propuesta, tarjeta del modo remoto) | `SettingsScreensTest` (Tarea 2) y la pantalla con estado en `ProtectedAppsFlowTest`/`BubbleFlowTest` |
| 27: lista de apps (búsqueda, rotación que conserva la selección, descartar cambios) | `ProtectedAppsScreensTest` (Tarea 3, con las filas comprobadas tras cada scroll) y `ProtectedAppsFlowTest` (Tarea 4: búsqueda por nombre, rotación con `+land` y `recreate()`, descartar con la flecha y con el atrás del sistema, cancelar, guardar) |
| 27: los estados del acceso | `NotificationAccessScreensTest` (Tarea 3): los cuatro, "comprobando" y "desconocido" con la marca puesta |
| 27: claro, oscuro, letra al 200 % | Los cuatro modos de `ScreenMode` (más el 200 % en español) |
| 27: con el host apagado, el aviso dice que la recibirá al conectar | `savingConfirmsTheListAndSaysThePcGetsItLater` (toast `protected_saved_later`) |
| 27: el PC recibe la lista y `adbuddy foreground` la marca | **Sigue manual** (host y móvil) |
| 34: la burbuja abre Ajustes, Apps protegidas y el acceso; el botón de confirmar funciona; no monta el onboarding | `BubbleFlowTest` (Tarea 5), también con el acceso retirado entre el dibujo y el toque (no pone la marca) |
| 34: `adbuddy tap` da `protected`, `adb shell input tap` lo bloquea la guarda, `dumpsys` con el host caído | **Sigue manual** (necesita el host y el móvil) |

### Revisión de placeholders y tipos

- Sin "TBD", "TODO" ni "similar a la Tarea N": todo el código sale de ficheros que compilan y pasan (la réplica los escribe byte a byte desde este documento).
- **Nombres coherentes entre tareas:** `PHONE`, `ScreenMode`, `screenModes()`, `ScreenModeRule`, `ComponentActivityRule`, `shot`, `assertTextsFit`, `assertTitleFitsTheTopBar`, `KNOWN_UI_ISSUES`, `knownUiIssue` (Tarea 1) → Tareas 2 y 3 (y `PHONE` → 4 y 5); `SAMPLE_ROWS` (Tarea 2) → Tarea 3; `installLauncherApps` (Tarea 4) → Tarea 4. Ninguna tarea usa nada de una posterior.

### Hallazgo de UI visto al grabar (no se arregla en este plan)

- **Letra al 200 % en español:** el título "Acceso a las notificaciones" ocupa dos líneas, 89 dp (134 px a hdpi), dentro de la barra superior de 64 dp, que no lo recorta: en la JVM empieza en el borde de arriba y sobresale por debajo de la barra. `assertTextsFit` no lo veía (el texto cabe en su propia caja y no pisa a otro); ahora lo ve `assertTitleFitsTheTopBar` y queda en `KNOWN_UI_ISSUES` (pasa mientras siga, falla cuando se arregle), y escrito en la validación 27 de M5c y en `pendientes-m3-m5.md` (Tarea 6). En inglés al 200 % los títulos caben.

### Correcciones de la revisión Opus del plan (ya incorporadas)

| Hallazgo | Dónde está resuelto |
|---|---|
| I1: la búsqueda "por nombre" se probaba por paquete ("bank" casa con `com.example.mybank`) | Tarea 4: busca "banco" (solo en la etiqueta) y "whatsapp" en minúsculas; mutación nueva en el Step 3 (sin el filtro por etiqueta, falla) |
| I2: al 200 % ninguna fila de la lista pasaba `assertTextsFit` | Tareas 2 y 3: `assertTextsFit(scrolled = true)` tras cada `performScrollToKey`, e imagen `onboarding_1_protected_apps_rows` (4 más) |
| I3: el título de dos líneas solo constaba en la autorrevisión, y la Tarea 3 lo daba por bueno | Validación 27 de M5c y `pendientes-m3-m5.md` (Tarea 6); la Tarea 3 (Step 4) lo marca como hallazgo conocido; `assertTitleFitsTheTopBar` y `KNOWN_UI_ISSUES`, con su test (Tarea 1). La comprobación del borde superior que proponía la revisión también está, pero no ve este caso: en la JVM el título empieza en y = 0 y sobresale por **abajo** (medido), así que se mide contra la altura de la barra |
| I4: el botón de confirmar solo en el camino feliz | Tarea 5: `theConfirmButtonAsksTheSystemAgainAtTheClick` (acceso retirado en `NotificationManager` antes del toque: sin marca, y la pantalla pasa a "Desconocido"); mutación `if (true)` en vez de `if (allowedNow)` |
| I5: en Linux la comparación ni falla ni avisa | Tarea 6: paso `::warning` en `android.yml`; ruling 10 con el coste real; validación 1 en el primer PR, la decide el usuario; en pendientes, en qué runner corre hoy |
| M1: "al píxel" | Meta, ruling 5, KDoc de `shot` y README: comparador por defecto de Roborazzi (0,7 % de color por píxel, ninguna proporción de píxeles); fuera las opciones explícitas, que repetían el valor por defecto |
| M2: historial | README: grabar solo la clase afectada; autorrevisión corregida |
| M3: coste del ruling 7 | Ruling 7: la imagen ve un falso negativo una sola vez, al grabar; lo que se vea va a `KNOWN_UI_ISSUES` |
| M4: ruling 4 | El APK de depuración es el del Pixel del usuario; lo publicado es el de release |
| M5: atrás del sistema con cambios | Tarea 4: `theSystemBackWithChangesAsksToo`. Sin esperar a que el toque recomponga, el atrás del test llegaba antes y salía sin preguntar; el test espera a ver la casilla marcada (una persona nunca es tan rápida) |
| M6: estados sin cubrir | Tarea 2: paso 2 con el acceso permitido, sin "Ahora no"; Tarea 3: "desconocido" con la marca ofrece retirar (solo semántica, sin imagen) |
| M7: sesiones paralelas | `pendientes-m3-m5.md` (Tarea 6): regrabar la clase afectada y mirar las imágenes en la misma tarea |
| M8: CRLF | Global Constraints: no tocar los finales de línea de esos dos ficheros |
| M9: tope de Gradle | 40 minutos |

### Riesgos abiertos

- En `ubuntu-latest` (el runner por defecto de `android.yml`) una regresión visual sale en verde con un aviso, hasta resolver la validación 1; las comprobaciones semánticas sí fallan en todos los sistemas.
- Robolectric pinta Android 14 con la paleta estática: lo propio de Android 17, el color dinámico, las fuentes del sistema y los insets reales siguen en el Pixel (ruling 2).
- Los flujos esperan hasta 5 s a lecturas en `Dispatchers.IO` (`waitUntil`): en un runner muy cargado podrían ir justos; subir el tope es una constante por test.
- `AgentNotificationListener.connected` es de todo el proceso: `BubbleFlowTest` lo desenlaza en `@After`; un test nuevo que lo enlace debe hacer lo mismo.
- Subir Robolectric obliga a subir `robolectricAndroidAll` a la vez (ruling 3); si no casan, el test falla cerrado con el nombre del jar que pide.
