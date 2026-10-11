# UX P2: onboarding y estados del chat — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el primer arranque y el chat digan siempre qué pasa y qué hacer: el paso 2 de 2 sin un "Listo" que no hace nada, botones del pie que no se aprietan con letra grande, el consejo de "ajustes restringidos" visible, feedback al aceptar o rechazar la configuración, una cabecera de una línea con un banner que lleva el texto accionable, un estado "sin configurar" y un chat vacío que se explica, y un chat que no arrastra al final a quien lee arriba.

**Architecture:** Composables pequeños en `ui/` (`ActionFooter`, `LinkBanner`, `ChatEmptyState`) y una ampliación del enum puro `LinkStatus` (con su texto corto y el estado `NOT_SET_UP`); `ChatScreen` los coloca. Nada cambia en el protocolo ni en la semántica de las confirmaciones. Cada cambio visible lleva su imagen de referencia (Roborazzi: claro, oscuro, 200 %, 200 % en español) y su comprobación de semántica. La pantalla del chat no tenía imágenes: la Task 4 crea `ChatScreensTest`.

**Tech Stack:** Kotlin, Jetpack Compose Material 3 (BOM 2026.09.00), Robolectric (SDK 34) + Compose ui-test + Roborazzi.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (UI de la app) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos O5, O19, O20, O10, C4, O2, C20, C21).

## Global Constraints

- Textos de usuario en inglés (`res/values/strings.xml`) y español (`res/values-es/strings.xml`), con las mismas claves y el mismo orden; "fichero" (no "archivo") en español; `StringsParityTest` y `NoHardcodedTextTest` pasan. Ningún texto literal en Kotlin.
- Nada de este plan cambia la semántica de una confirmación ni el texto de consentimiento de `consent_*` (eso es del paquete P5): el botón "Aceptar" y lo que concede quedan igual; solo se añade feedback tras responder.
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Gradle (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon --offline` (en los pasos, `gradle`). Tarda minutos: timeout amplio o segundo plano; sin `sleep` en primer plano. `android/local.properties` ya existe. Nunca `connected*`, nunca emulador.
- Imágenes: en Windows `testDebugUnitTest` las verifica; para regrabar una clase, `gradle :app:testDebugUnitTest -Padbuddy.screens=record --tests '*NombreDeLaClase'`, mirar con Read cada PNG que cambie y comprobar con `git status` que solo cambian las esperadas. Una imagen que cambia sin esperarlo es un fallo: no se regraba, se cuenta en el informe.
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux` (rama `ux/revision`). La shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...` en cada comando.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el hook de commit lo exige). Sin push.
- Rutas relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` (código), `android/app/src/test/kotlin/tech/hybriq/adbuddy/` (`T/`) y `android/app/src/main/res/` (`res/`), salvo que se diga otra cosa.
- No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md`: las filas van en `docs/superpowers/fragmentos/ux-revision.md`.

---

### Task 1: `ActionFooter` y el paso 2 de 2 sin "Listo" falso (O5, O19)

Hoy el pie del paso 2 de 2 muestra "Ahora no" y "Listo" aunque no se haya activado nada: ambos terminan el onboarding, y "Listo" parece decir que está hecho. Con el acceso sin activar queda solo "Ahora no" (más un recordatorio de dónde activarlo después); "Listo" aparece cuando el acceso está activo. Y con letra grande los dos botones se apilan a ancho completo en vez de aplastarse en una fila.

**Files:**
- Create: `ui/ActionFooter.kt`
- Modify: `ui/NotificationAccessScreen.kt` (la `bottomBar` del paso, `Row` con `SecondaryActionButton` y `Button`), `ui/ProtectedAppsScreen.kt` (la `Row` de la `bottomBar` con `SecondaryActionButton` y `Button`)
- Modify: `res/values/strings.xml`, `res/values-es/strings.xml` (cadena nueva `access_later_hint`, junto a `access_restricted_hint`)
- Test (nuevo): `T/ui/ActionFooterTest.kt`
- Test (modificar): `T/screens/OnboardingScreensTest.kt:83` (`stepTwoNotificationAccess` hoy exige `onboarding_done` visible: pasa a `assertDoesNotExist`; ya existe `stepTwoWithTheAccessOnHasNoNotNow`, ~línea 95, que cubre el caso con el acceso activo: no se añade otro test ni otra imagen) y `T/flows/OnboardingFlowTest.kt:75` (`confirmingTheListThenDoneFinishesTheSteps` pulsa `onboarding_done` en el paso 2, que con el acceso sin activar ya no existe: pasa a pulsar `onboarding_skip` y se renombra `confirmingTheListThenNotNowFinishesTheSteps`)
- Regrabar: `OnboardingScreensTest` (los pasos 1 y 2: pie apilado a 200 %, paso 2 sin "Listo"), `ProtectedAppsScreensTest` (pie a 200 %)

**Interfaces:**
- Produces: `@Composable fun ActionFooter(main: @Composable (Modifier) -> Unit, other: (@Composable (Modifier) -> Unit)? = null, modifier: Modifier = Modifier)` y `const val STACK_FROM_FONT_SCALE = 1.3f` en el mismo fichero. `main` es la acción principal, `other` la secundaria (opcional); cada una recibe el `Modifier` que le toca (a ancho completo cuando se apilan). Con `fontScale <= 1.3` van en una fila alineada al final, `other` a la izquierda de `main`; con más, en columna a ancho completo, `main` arriba y `other` debajo.
- Consumes: `SecondaryActionButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier)` (ya existe, Task 2 de P1).

- [ ] **Step 1: Test que falla** (`T/ui/ActionFooterTest.kt`)

Parametrizado por `ScreenMode` (patrón de `AdbuddyTopBarTest`: `ParameterizedRobolectricTestRunner`, `@GraphicsMode(NATIVE)`, `@Config(qualifiers = PHONE)`, `RuleChain` con `ScreenModeRule(mode)` + `ComponentActivityRule()` + `createComposeRule()`). Dos botones con textos largos, uno principal y uno secundario:

```kotlin
private val main = if (mode.fontScale > STACK_FROM_FONT_SCALE) "Confirmar la lista de apps protegidas" else "Confirmar la lista"
private val other = "Ahora no"

@Test fun sideBySideWithNormalTextAndStackedWithLargeText() {
    compose.setContent {
        AdbuddyTheme(dynamic = false) {
            ActionFooter(
                main = { m -> Button(onClick = {}, modifier = m) { Text(main) } },
                other = { m -> SecondaryActionButton(other, onClick = {}, modifier = m) },
            )
        }
    }
    val a = compose.onNodeWithText(main).fetchSemanticsNode().boundsInRoot
    val b = compose.onNodeWithText(other).fetchSemanticsNode().boundsInRoot
    if (mode.fontScale > STACK_FROM_FONT_SCALE) {
        // Stacked: the main action above, both as wide as the footer's content, neither squeezed.
        assertTrue("main above other", a.bottom <= b.top + 0.5f)
        assertEquals("same left", a.left, b.left, 0.5f)
        assertEquals("same width", a.width, b.width, 0.5f)
    } else {
        // Side by side: the same row, the secondary action at the left of the main one.
        assertEquals("same row", a.center.y, b.center.y, 2f)
        assertTrue("other left of main", b.right <= a.left + 0.5f)
    }
    compose.assertTextsFit()
}

@Test fun withoutASecondaryActionTheMainOneIsAlone() {
    compose.setContent { AdbuddyTheme(dynamic = false) { ActionFooter(main = { m -> Button(onClick = {}, modifier = m) { Text(main) } }) } }
    compose.onNodeWithText(main).assertIsDisplayed()
    compose.assertTextsFit()
}
```

(`mode` es el parámetro del test; usar `boundsInRoot` de los nodos. El primer test ordena las dos mediciones con la misma tolerancia que usan los demás tests de pantallas.)

- [ ] **Step 2: Ejecutar: falla** (`ActionFooter` sin definir): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/android && gradle :app:testDebugUnitTest --tests '*ActionFooterTest'`.

- [ ] **Step 3: Implementar `ui/ActionFooter.kt`**

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp

/** From this text size on, the footer's actions are stacked: two buttons in a row would squeeze each other's words. */
const val STACK_FROM_FONT_SCALE = 1.3f

/**
 * The actions at the foot of a step: in a row at the end, or (large text) stacked at full width with the main one on top.
 * Each action receives the Modifier it has to take (full width when stacked).
 */
@Composable
fun ActionFooter(
    main: @Composable (Modifier) -> Unit,
    other: (@Composable (Modifier) -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    if (LocalDensity.current.fontScale > STACK_FROM_FONT_SCALE) {
        Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            main(Modifier.fillMaxWidth())
            other?.invoke(Modifier.fillMaxWidth())
        }
    } else {
        Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End), verticalAlignment = Alignment.CenterVertically) {
            other?.invoke(Modifier)
            main(Modifier)
        }
    }
}
```

- [ ] **Step 4: Usarlo**

`ui/NotificationAccessScreen.kt`, la `bottomBar` del paso (dentro del `Surface`): el `Row` se sustituye por

```kotlin
Column(
    Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal = 24.dp, vertical = 12.dp),
    verticalArrangement = Arrangement.spacedBy(8.dp),
) {
    if (effective == Effective.ON) {
        ActionFooter(main = { m -> Button(onClick = onDone, modifier = m) { Text(stringResource(R.string.onboarding_done)) } })
    } else {
        // Nothing is on yet: "Done" would only look like it. The way out says so, and where to turn it on later.
        Text(stringResource(R.string.access_later_hint), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        ActionFooter(main = { m -> SecondaryActionButton(stringResource(R.string.onboarding_skip), onSkip, m) })
    }
}
```

(Importar lo que falte: `Column`, `ActionFooter`; quitar imports sin uso. En el estado "comprobando" `effective` no es `ON`, así que muestra "Ahora no": es lo correcto, no se sabe aún.)

`ui/ProtectedAppsScreen.kt`, la `Row` del pie (la que lleva `SecondaryActionButton` y `Button(onClick = onSave, enabled = canSave)`) se sustituye por:

```kotlin
ActionFooter(
    main = { m ->
        Button(onClick = onSave, enabled = canSave, modifier = m) {
            Text(stringResource(if (proposal) R.string.protected_confirm else R.string.protected_save))
        }
    },
    other = if (step != null) { { m -> SecondaryActionButton(stringResource(R.string.onboarding_skip), onSkip, m) } } else null,
)
```

Cadena nueva, en `res/values/strings.xml` y `res/values-es/strings.xml` junto a `access_restricted_hint`:
- en: `<string name="access_later_hint">You can turn it on later in ADBuddy\'s settings (the ⋮ menu), under Notification access.</string>`
- es: `<string name="access_later_hint">Podrás activarlo después en los ajustes de ADBuddy (menú ⋮), en Acceso a las notificaciones.</string>`

- [ ] **Step 5: Aserciones de pantalla**

En `T/screens/OnboardingScreensTest.kt`, `stepTwoNotificationAccess` (acceso sin activar): la línea 83, que exige `onboarding_done` visible, pasa a `onNodeWithText(text(R.string.onboarding_done)).assertDoesNotExist()`; se añaden `onNodeWithText(text(R.string.onboarding_skip)).assertIsDisplayed()` y `onNodeWithText(text(R.string.access_later_hint)).assertIsDisplayed()`. `stepTwoWithTheAccessOnHasNoNotNow` ya comprueba `Listo` visible y `Ahora no` ausente con el acceso activo: se deja como está (y NO se pide `access_later_hint` en ese caso). En `T/flows/OnboardingFlowTest.kt:75`, `confirmingTheListThenDoneFinishesTheSteps` pulsa `onboarding_done` en el paso 2: pasa a pulsar `onboarding_skip` y se renombra `confirmingTheListThenNotNowFinishesTheSteps`; el resto de la aserción (se llega al chat y el onboarding queda hecho) no cambia. Los `assertTextsFit()` existentes siguen.

- [ ] **Step 6: Ejecutar, regrabar, mirar**

Run: `gradle :app:testDebugUnitTest --tests '*ActionFooterTest' --tests '*OnboardingScreensTest' --tests '*ProtectedAppsScreensTest' --tests '*OnboardingFlowTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'`
Expected: `ActionFooterTest` PASS; fallan solo las imágenes del paso 2 de 2 (`onboarding_2_notification_access_*`, 6, ahora con un solo botón y el recordatorio), las de 200 % del paso 1 y de apps protegidas (pie apilado, que la `bottomBar` pinta siempre: todas las `onboarding_1_protected_apps_*font_200*` y las `protected_apps_*font_200*`, `_rows_` incluidas); las de claro y oscuro de apps protegidas NO deben cambiar. `OnboardingFlowTest` pasa con su cambio de la línea 75. Regrabar `OnboardingScreensTest` y `ProtectedAppsScreensTest`, mirar con Read como mínimo `onboarding_2_notification_access_light.png`, `onboarding_2_notification_access_font_200_es.png`, `onboarding_1_protected_apps_font_200_es.png` y `protected_apps_settings_font_200.png`: el pie apilado ocupa el ancho, las palabras no se cortan.

- [ ] **Step 7: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): pie de los pasos que se apila con letra grande y paso 2 de 2 sin Listo falso

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: El consejo de "ajustes restringidos" en una tarjeta (O20)

El consejo que más hace falta cuando el interruptor del sistema sale en gris es hoy la línea más pequeña y tenue, debajo del botón. Pasa a una tarjeta con icono de información, entre el estado y el botón que abre los ajustes.

**Files:**
- Modify: `ui/NotificationAccessScreen.kt` (el `Text(stringResource(R.string.access_restricted_hint) ...)` del cuerpo y su condición `!checking && (effective == OFF || UNKNOWN)`)
- Test: ampliar `T/screens/NotificationAccessScreensTest.kt` (aserción del consejo en los estados `OFF` y `UNKNOWN`, ausencia en los demás) y `T/screens/OnboardingScreensTest.kt` si pinta `OFF`
- Regrabar: `NotificationAccessScreensTest` (`access_not_allowed_*`, `access_unknown_*`, y sus `_below_`) y las del paso 2 si el acceso está apagado en ellas

**Interfaces:**
- Consumes: `Effective` (`OFF, UNKNOWN, NEEDS_CONFIRMATION, ON`), `StateCard(effective)` (privado, mismo fichero).
- Produces: `@Composable private fun RestrictedHint()` (privado, mismo fichero).

- [ ] **Step 1: Test que falla**

En `NotificationAccessScreensTest`, en los tests de `OFF` ("no permitido") y `UNKNOWN`, comprobar que el consejo es un único nodo fusionado con su icono (el icono no se anuncia: el nodo del consejo tiene exactamente el texto `access_restricted_hint`, sin `contentDescription` aparte) y que el orden vertical es estado → consejo → botón de abrir los ajustes. Se mide con `positionInRoot` (a 200 % el consejo y el botón quedan bajo el pliegue y `boundsInRoot` los recortaría: es lo que hace `ScreenTestSupport.assertTextsFit`):

```kotlin
fun top(id: Int) = compose.onNodeWithText(text(id)).fetchSemanticsNode().positionInRoot.y
fun bottom(id: Int) = compose.onNodeWithText(text(id)).fetchSemanticsNode().let { it.positionInRoot.y + it.size.height }
assertTrue("state, then the hint, then the button", bottom(R.string.access_state_denied) <= top(R.string.access_restricted_hint) + 0.5f && bottom(R.string.access_restricted_hint) <= top(R.string.access_open) + 0.5f)
```

(Para `UNKNOWN`, `access_state_unknown`.) En `NEEDS_CONFIRMATION` y `ON`: `onNodeWithText(text(R.string.access_restricted_hint)).assertDoesNotExist()` (como hoy).

- [ ] **Step 2: Ejecutar: falla** (hoy el consejo está debajo del botón): `gradle :app:testDebugUnitTest --tests '*NotificationAccessScreensTest'`.

- [ ] **Step 3: Implementar**

Mover el bloque del consejo justo debajo de la `StateCard`, dentro de `if (!checking && (effective == Effective.OFF || effective == Effective.UNKNOWN))`, como:

```kotlin
@Composable
private fun RestrictedHint() {
    val scheme = MaterialTheme.colorScheme
    Surface(modifier = Modifier.semantics(mergeDescendants = true) {}, color = scheme.secondaryContainer, contentColor = scheme.onSecondaryContainer, shape = MaterialTheme.shapes.medium) {
        Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.Top) {
            Icon(Icons.Default.Info, contentDescription = null)
            Text(stringResource(R.string.access_restricted_hint), style = MaterialTheme.typography.bodyMedium)
        }
    }
}
```

y `if (...) RestrictedHint()` en el cuerpo, tras `if (checking) CheckingCard() else StateCard(effective)` y los botones de confirmar/retirar, y antes del `FilledTonalButton(onClick = onOpen)`. Quitar el `Text` pequeño anterior. El resto (la pista de códigos, `access_codes_hint`) queda donde está.

- [ ] **Step 4: Tests, regrabar, mirar, commit**

Run: `gradle :app:testDebugUnitTest --tests '*NotificationAccessScreensTest' --tests '*OnboardingScreensTest' --tests '*OnboardingFlowTest'` → fallan solo las imágenes de los estados `OFF`/`UNKNOWN` de Acceso (`access_not_allowed_*`, `access_unknown_*`, con sus `_below_`) y las 6 `onboarding_2_notification_access_*` otra vez (el paso 2 usa `Effective.OFF`); aparecer o desaparecer una `_below_` de esas pantallas es normal y una `_below_` que quede huérfana se borra a mano (`record` no las elimina); regrabar solo esas clases, mirar `access_not_allowed_light.png` y `access_not_allowed_font_200_es.png` (la tarjeta con icono, el orden estado, consejo, botón) y comprobar con `git status` que no cambia ninguna de `NEEDS_CONFIRMATION`, `allowed` ni `checking`.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el consejo de ajustes restringidos en una tarjeta con icono, junto al estado

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Feedback al aceptar o rechazar la configuración (O10)

Rechazar no dice nada; aceptar dice "Puedes volver al PC" justo antes de enseñar el paso 1 de 2 en el móvil. No cambia qué concede "Aceptar" ni los textos `consent_*` de la pantalla: solo el aviso posterior.

**Files:**
- Modify: `MainActivity.kt` (el `onAnswer` de `ConsentScreen`, línea ~55-59), `res/values/strings.xml`, `res/values-es/strings.xml` (junto a `consent_done`)
- Test: ampliar `T/flows/OnboardingFlowTest.kt` o crear `T/flows/ConsentFeedbackFlowTest.kt` (con `flows/Launcher.kt` como apoyo si sirve)

**Interfaces:**
- Produces: las cadenas `consent_done_steps` y `consent_declined`.
- Consumes: `Onboarding.of(context).done()`, `BridgePrefs.consent` (`"pending"`, `"granted"`, `"denied"`), `MainActivity.ACTION_SETUP`.

- [ ] **Step 1: Test que falla**

Un test de flujo con Robolectric: `BridgePrefs(app).consent = "pending"` (y `consentHost = "my-pc"`), arrancar `MainActivity` con `Intent(app, MainActivity::class.java).setAction(MainActivity.ACTION_SETUP)`, y comprobar, con `org.robolectric.shadows.ShadowToast.getTextOfLatestToast()`:
1. Pulsar `consent_decline` ("Not now"): `prefs.consent == "denied"`, el último toast es `text(R.string.consent_declined)` y se ve el paso 1 de 2 o el chat (la pantalla del consentimiento ya no está).
2. Pulsar `consent_accept` con el onboarding sin hacer (`Onboarding.of(app).done() == false`): `prefs.consent == "granted"` y el último toast es `text(R.string.consent_done_steps)`.
3. Pulsar `consent_accept` con el onboarding ya hecho (`Onboarding.of(app).finish()` antes): el último toast es `text(R.string.consent_done)` (el de siempre).

(`createAndroidComposeRule<MainActivity>()` no admite un `Intent`: este test usa `createEmptyComposeRule()` y `ActivityScenario.launch<MainActivity>(Intent(app, MainActivity::class.java).setAction(MainActivity.ACTION_SETUP))`, con `BridgePrefs(app).consent = "pending"` y `consentHost = "my-pc"` fijados ANTES del `launch`, y `Onboarding.of(app).finish()` antes de lanzar en el caso 3. Mira `flows/Launcher.kt` por si ya lo resuelve. Los tres casos son tres tests. Ruling de diseño: tras rechazar, el onboarding sigue si no estaba hecho (comportamiento de hoy: el aviso no lo cambia); `consent_done_steps` dice "dos pasos" porque son `OnboardingStep.total = 2`.)

- [ ] **Step 2: Ejecutar: falla** (`consent_declined` / `consent_done_steps` sin definir).

- [ ] **Step 3: Implementar**

Cadenas, junto a `consent_done` en los dos ficheros:
- en: `<string name="consent_done_steps">Done. Two quick steps remain here; then go back to the PC.</string>` y `<string name="consent_declined">Nothing was set up. To try again, run adbuddy setup on the PC.</string>`
- es: `<string name="consent_done_steps">Listo. Quedan dos pasos rápidos aquí; después vuelve al PC.</string>` y `<string name="consent_declined">No se ha configurado nada. Para repetirlo, ejecuta adbuddy setup en el PC.</string>`

`MainActivity.kt`, el `onAnswer`:

```kotlin
onAnswer = { accepted ->
    prefs.consent = if (accepted) "granted" else "denied"
    // The first-run steps follow in the app only when they have not been done: the message says what comes next.
    val message = when {
        !accepted -> R.string.consent_declined
        Onboarding.of(this).done() -> R.string.consent_done
        else -> R.string.consent_done_steps
    }
    Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    showConsent = false
},
```

(`Onboarding` está en `ui/`: importar `tech.hybriq.adbuddy.ui.Onboarding`. El consentimiento rechazado también muestra aviso, como pide el hallazgo.)

- [ ] **Step 4: Tests y commit**

Run: `gradle :app:testDebugUnitTest --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS (ninguna imagen cambia).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): aviso al rechazar la configuracion y al aceptar que dice lo que falta

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Cabecera de una línea, banner con el texto accionable y estado "sin configurar" (C4, O2)

Hoy la cabecera del chat mete el estado largo (con comandos como `adbuddy setup --apk <fichero>`) en dos líneas de una barra de altura fija, y se corta con elipsis. Pasa a una línea corta ("Sin conexión", "Hay que actualizar"…) y un banner debajo con el texto completo. Y antes de `adbuddy setup` el móvil dice "ejecuta adbuddy serve", que no sirve: ahora dice que falta configurar.

**Files:**
- Modify: `ui/LinkStatus.kt` (enum `LinkStatus` y `LinkStatus.of`), `ui/ChatParts.kt` (`ChatHeader`), `ui/ChatScreen.kt` (colocar el banner y pasar `configured`), `ui/ChatRoot.kt` (calcular `configured`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Create: `ui/LinkBanner.kt`
- Test (nuevo): `T/screens/ChatScreensTest.kt`; ampliar `T/ui/LinkStatusTest.kt`; ver `T/screens/ScreenTestSupport.kt` (`PHONE`, `ScreenModeRule`, `shot`, `assertTextsFit`)
- Regrabar (nuevas, en `src/test/screenshots/`, nombres exactos): `chat_connected_messages_*`, `chat_offline_waiting_*`, `chat_cut_*`, `chat_update_app_*`, `chat_not_set_up_*` en los cuatro modos (`_light`, `_dark`, `_font_200`, `_font_200_es`)

**Interfaces:**
- Modifies: `LinkStatus` pasa a `enum class LinkStatus(@param:StringRes val text: Int, @param:StringRes val shortText: Int)` con el valor nuevo `NOT_SET_UP`; `LinkStatus.of(state: BridgeState, cut: Boolean, configured: Boolean = true): LinkStatus` (el servicio de notificación sigue llamándolo con dos argumentos: solo corre con enlace).
- Produces: `@Composable fun LinkBanner(status: LinkStatus, modifier: Modifier = Modifier)` (no pinta nada para `CONNECTED` ni `CONNECTING`); `LinkStatus.bannerWorthy: Boolean` (`CUT`, `UPDATE_APP`, `UPDATE_HOST`, `PAUSED`, `WAITING`, `NOT_SET_UP`); `ChatHeader(..., configured: Boolean = true)` (al FINAL de la lista, con valor por defecto: `ChatScreen` la llama con argumentos posicionales y la vista previa no pasa `configured`) y `ChatScreen(..., hostNow, configured: Boolean = true, remoteCard)` (justo antes de `remoteCard`, después de `hostNow`: `ChatRoot` llama con argumentos posicionales hasta `hostNow`).

Tabla de cadenas nuevas (junto a las `status_*`, mismo orden en los dos ficheros):

| clave | en | es |
|---|---|---|
| `status_not_set_up` | `This phone is not set up yet: on the PC, run adbuddy setup` | `Este móvil aún no está configurado: en el PC, ejecuta adbuddy setup` |
| `status_short_cut` | `Access cut` | `Acceso cortado` |
| `status_short_update` | `Update needed` | `Hay que actualizar` |
| `status_short_connecting` | `Connecting…` | `Conectando…` |
| `status_short_waiting` | `Waiting for the PC` | `Esperando al PC` |
| `status_short_paused` | `Paused` | `En pausa` |
| `status_short_not_set_up` | `Not set up` | `Sin configurar` |
| `status_short_connected` | `Connected` | `Conectado` |

- [ ] **Step 1: Tests que fallan**

`T/ui/LinkStatusTest.kt`: añadir
- `LinkStatus.of(BridgeState(), cut = false, configured = false) == NOT_SET_UP` (WAITING sin enlace),
- `LinkStatus.of(BridgeState(connection = CONNECTED), cut = false, configured = false) == CONNECTED` (si ya hay canal, hay configuración),
- `cut` gana a `NOT_SET_UP`,
- `LinkStatus.entries.filter { it.bannerWorthy }` es exactamente `CUT, UPDATE_APP, UPDATE_HOST, PAUSED, WAITING, NOT_SET_UP`,
- y cada valor tiene `text` y `shortText` distintos de 0 (con `StringsParityTest` garantiza ambos idiomas).

`T/screens/ChatScreensTest.kt` (parametrizado por `ScreenMode`; mismo andamiaje que `SettingsScreensTest`: `ParameterizedRobolectricTestRunner`, `@GraphicsMode(NATIVE)`, `@Config(qualifiers = PHONE)`, `RuleChain(ScreenModeRule(mode), ComponentActivityRule(), createComposeRule())`). Un `show(state, cut = false, configured = true)` que pinta `ChatScreen(state, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut, terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None, configured = configured, remoteCard = {})` dentro de `AdbuddyTheme(dynamic = false)`. Estados y comprobaciones (todos con `assertTextsFit()` y `shot(nombre, mode)`):
1. `chat_connected_messages`: `BridgeState(connection = CONNECTED, hostName = "my-pc", chat = ChatState(messages = [user "Turn the volume down", assistant "Done. The media volume is now at 20%."]))`: título = `app_name` (sin agente), subtítulo `session_idle`, NO hay banner: ningún nodo lleva `SemanticsProperties.LiveRegion` (`onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.LiveRegion)).assertCountEquals(0)`).
2. `chat_offline_waiting`: `BridgeState()` (WAITING): subtítulo `status_short_waiting` en una línea y banner con `status_waiting` completo; los dos visibles; `status_waiting` no es el subtítulo.
3. `chat_cut`: `BridgeState(connection = CONNECTED, ...)` con `cut = true`: subtítulo `session_idle`, banner `status_cut`.
4. `chat_update_app`: `BridgeState().appOutdated()`: subtítulo `status_short_update`, banner `status_update_app` COMPLETO (`assertTextsFit` comprueba que no se corta: esta es la prueba de C4).
5. `chat_not_set_up`: `BridgeState()` con `configured = false`: subtítulo `status_short_not_set_up`, banner `status_not_set_up`.
Y una comprobación de estructura (sin imagen): que el banner del caso 4 lleva `LiveRegion == Polite` y que el subtítulo de la cabecera tiene una sola línea (`TextLayoutResult.lineCount == 1`; `ScreenTestSupport.layout()` es privado: un helper nuevo en el propio test con `SemanticsActions.GetTextLayoutResult`). Y que la cabecera cabe en su barra: `compose.assertTitleFitsTheTopBar(<texto del subtítulo>)` (la cabecera lleva `TOP_BAR_TAG`, ver el Step 3), también a 200 %.

- [ ] **Step 2: Ejecutar: fallan** (`configured`, `shortText`, `LinkBanner` sin definir): `gradle :app:testDebugUnitTest --tests '*LinkStatusTest' --tests '*ChatScreensTest'`.

- [ ] **Step 3: Implementar `LinkStatus`**

`ui/LinkStatus.kt`: el enum queda así (el resto del fichero, `PauseButton` y `StatusView`, no cambia salvo la llamada de `StatusView.of`, que sigue con dos argumentos):

```kotlin
enum class LinkStatus(@param:StringRes val text: Int, @param:StringRes val shortText: Int) {
    CUT(R.string.status_cut, R.string.status_short_cut),
    UPDATE_APP(R.string.status_update_app, R.string.status_short_update),
    UPDATE_HOST(R.string.status_update_host, R.string.status_short_update),
    PAUSED(R.string.status_paused, R.string.status_short_paused),
    CONNECTED(R.string.status_connected, R.string.status_short_connected),
    CONNECTING(R.string.status_connecting, R.string.status_short_connecting),
    WAITING(R.string.status_waiting, R.string.status_short_waiting),
    NOT_SET_UP(R.string.status_not_set_up, R.string.status_short_not_set_up),
    ;

    /** The long text goes in a banner (it may carry a command to run); CONNECTED and CONNECTING need none. */
    val bannerWorthy: Boolean get() = this != CONNECTED && this != CONNECTING

    companion object {
        /**
         * cut: ... (comentario existente). configured: the host has handed this phone a link (BridgePrefs.link);
         * without it and without a channel the phone was never set up: "run adbuddy serve" would not help.
         */
        fun of(state: BridgeState, cut: Boolean, configured: Boolean = true): LinkStatus = when {
            cut -> CUT
            state.connection == BridgeState.Connection.INCOMPATIBLE && state.outdated == BridgeState.Outdated.APP -> UPDATE_APP
            state.connection == BridgeState.Connection.INCOMPATIBLE -> UPDATE_HOST
            state.connection == BridgeState.Connection.CONNECTED && state.sessionState.paused -> PAUSED
            state.connection == BridgeState.Connection.CONNECTED -> CONNECTED
            state.connection == BridgeState.Connection.CONNECTING -> CONNECTING
            !configured -> NOT_SET_UP
            else -> WAITING
        }
    }
}
```

No hay ningún `when` exhaustivo sobre `LinkStatus` (los usos son `==` y `!=` en `ConversationView`, `ChatParts` y `LinkStatus`): añadir `NOT_SET_UP` no rompe nada. El servicio de notificación nunca llega a `NOT_SET_UP` porque llama a `of` con `configured = true` por defecto.

- [ ] **Step 4: `LinkBanner` y la cabecera**

`ui/LinkBanner.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

/** What is wrong with the link, in full, under the header: the text may carry a command to run, so it is never cut. */
@Composable
fun LinkBanner(status: LinkStatus, modifier: Modifier = Modifier) {
    if (!status.bannerWorthy) return
    val scheme = MaterialTheme.colorScheme
    // A broken or cut link is an error; waiting, paused and not set up are states to act on, not faults.
    val problem = status == LinkStatus.CUT || status == LinkStatus.UPDATE_APP || status == LinkStatus.UPDATE_HOST
    Surface(
        color = if (problem) scheme.errorContainer else scheme.secondaryContainer,
        contentColor = if (problem) scheme.onErrorContainer else scheme.onSecondaryContainer,
        modifier = modifier.fillMaxWidth().semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
    ) {
        Text(stringResource(status.text), style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
    }
}
```

(Quitar el `import ...Row` si sobra. `status_connected` lleva `%1$s` pero nunca llega aquí por `bannerWorthy`.)

`ui/ChatParts.kt`, `ChatHeader`: añadir el parámetro `configured: Boolean` tras `cut`, `val status = LinkStatus.of(state, cut, configured)`, y el título pasa a una sola línea de subtítulo:

```kotlin
Text(
    stringResource(
        when (label) {
            SessionLabel.WORKING -> R.string.session_working
            SessionLabel.WAITING -> R.string.session_waiting
            SessionLabel.IDLE -> R.string.session_idle
            SessionLabel.STOPPED -> R.string.session_stopped
            // Not connected: the short state; what to do about it is in the banner under the bar.
            SessionLabel.OFFLINE -> status.shortText
        },
    ),
    style = MaterialTheme.typography.bodySmall,
    color = MaterialTheme.colorScheme.onSurfaceVariant,
    maxLines = 1,
    overflow = TextOverflow.Ellipsis,
)
```

y se borra el bloque `if (label != SessionLabel.OFFLINE && status != LinkStatus.CONNECTED) { Text(... status.text ... error ...) }` (esa información la lleva ahora el banner).

Altura de la cabecera (sigue siendo un `TopAppBar`, que no recorta): como `AdbuddyTopBar`, la barra toma la altura del contenido y lleva la etiqueta que mide el test. En `ChatHeader`: `val titleLine = with(LocalDensity.current) { (MaterialTheme.typography.titleMedium.lineHeight + MaterialTheme.typography.bodySmall.lineHeight).toDp() }` y, en el `TopAppBar`, `modifier = Modifier.testTag(TOP_BAR_TAG)` y `expandedHeight = AdbuddyTopBarSpec.height(titleLine)` (la fórmula `max(64 dp, x + 8 dp)` da 64 dp a 1x: las imágenes de claro y oscuro del chat se graban ya con ella). Importar `testTag`, `LocalDensity`, `AdbuddyTopBarSpec`, `TOP_BAR_TAG` (de `ui/AdbuddyTopBar.kt`).

`ui/ChatScreen.kt`: `ChatScreen` gana `configured: Boolean = true` después de `hostNow` y antes de `remoteCard`; la cabecera la recibe con nombre (`ChatHeader(..., configured = configured)`, que la declara al final con valor por defecto; la llamada actual de `ChatScreen` es posicional hasta `onNewConversation`); y, dentro del `Column(...)` del cuerpo, lo primero: `LinkBanner(LinkStatus.of(state, cut, configured))`, antes de `remoteCard()`. `ui/ChatRoot.kt` (importar `tech.hybriq.adbuddy.bridge.BridgePrefs`): `val configured = remember(state.connection) { BridgePrefs(context).link != null }` y se pasa a `ChatScreen(..., configured = configured)` (llamada con nombre).

- [ ] **Step 5: Tests, paridad, regrabar, mirar**

Run: `gradle :app:testDebugUnitTest --tests '*LinkStatusTest' --tests '*ChatScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest' --tests '*ConversationViewTest' --tests '*ChatControllerTest'` (más el resto de la suite al final). Con `-Padbuddy.screens=record --tests '*ChatScreensTest'` se graban las imágenes nuevas; mirar con Read, como mínimo, `chat_update_app_font_200_es.png` (el banner con el texto completo, el subtítulo en una línea), `chat_offline_waiting_light.png`, `chat_cut_dark.png` y `chat_not_set_up_font_200.png`. Si un `assertTextsFit` o `assertTitleFitsTheTopBar` falla por la cabecera, la causa es del diseño (no se baja el criterio): ajustar el diseño y contar el cambio en el informe. Ninguna otra imagen existente debe cambiar.

- [ ] **Step 6: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): cabecera de una linea, banner con el texto accionable y estado sin configurar

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: El chat vacío se explica (C20)

Un chat conectado y sin mensajes es hoy una pantalla en blanco. Con el enlace en pie dice dónde escribir; los demás casos (sin configurar, sin conexión, corte, actualización, pausa) ya los explica el banner de la Task 4 y no se repite aquí; con `CONNECTING` (un instante) solo está el subtítulo "Conectando…": se acepta, es transitorio.

**Files:**
- Create: `ui/ChatEmptyState.kt`
- Modify: `ui/ChatScreen.kt` (la `LazyColumn`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test: ampliar `T/screens/ChatScreensTest.kt`
- Regrabar: `chat_connected_empty_*` (nuevas)

**Interfaces:**
- Consumes: `LinkStatus.of(state, cut, configured)` (Task 4), `ChatScreen(..., configured)`.
- Produces: `@Composable fun ChatEmptyState(modifier: Modifier = Modifier)` (el modificador lleva el peso y el centrado que le da `ChatScreen`).

- [ ] **Step 1: Test que falla**

En `ChatScreensTest`: `chat_connected_empty` (`BridgeState(connection = CONNECTED, hostName = "my-pc")`, sin mensajes, sin herramientas): `onNodeWithText(text(R.string.chat_empty_ready)).assertIsDisplayed()`, `assertTextsFit()`, `shot("chat_connected_empty", mode)`. Y en los estados sin conexión (`BridgeState()`), con corte y con actualización: `onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()` (el banner ya lo cuenta). Y con mensajes (`chat_connected_messages`): tampoco aparece.

- [ ] **Step 2: Ejecutar: falla** (`chat_empty_ready` sin definir).

- [ ] **Step 3: Implementar**

Cadena, junto a `home_empty` en los dos ficheros:
- en: `<string name="chat_empty_ready">Write to the agent below. Risky commands ask for your confirmation.</string>`
- es: `<string name="chat_empty_ready">Escribe al agente aquí abajo. Los comandos delicados piden tu confirmación.</string>`

`ui/ChatEmptyState.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R

/** The conversation is empty and the link is up: where to start. (Without a link the banner says what to do.) */
@Composable
fun ChatEmptyState(modifier: Modifier = Modifier) {
    Box(modifier.padding(24.dp), contentAlignment = Alignment.Center) {
        Text(
            stringResource(R.string.chat_empty_ready),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}
```

En `ChatScreen`, cuando `items.isEmpty() && tools.isEmpty() && LinkStatus.of(state, cut, configured) == LinkStatus.CONNECTED` la `LazyColumn` se sustituye por `ChatEmptyState(Modifier.weight(if (terminalOn) 0.55f else 1f).fillMaxSize())` (mismo peso que tendría la lista, para que el terminal y la barra de entrada no se muevan). Con la lista llena o sin conexión, igual que hoy.

- [ ] **Step 4: Tests, regrabar, mirar, commit**

Run: `gradle :app:testDebugUnitTest --tests '*ChatScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'`. Grabar solo la nueva (`-Padbuddy.screens=record --tests '*ChatScreensTest'` y comprobar con `git status` que solo se añade `chat_connected_empty_*` y no cambia ninguna de la Task 4); mirar `chat_connected_empty_font_200_es.png`.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el chat conectado y vacio dice donde escribir

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: El chat no arrastra al final a quien lee arriba (C21)

`ChatScreen` hace `animateScrollToItem(rows - 1)` en cada cambio: quien lee el historial arriba es devuelto al final cuando llega un mensaje. Pasa a seguir solo si ya estaba abajo; si no, aparece un aviso "Mensajes nuevos" que lleva al final al tocarlo.

**Files:**
- Modify: `ui/ChatScreen.kt` (el `LaunchedEffect(rows)` y la lista), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/ui/ChatScrollTest.kt` (Robolectric + `createComposeRule`, `@GraphicsMode(NATIVE)`, `@Config(qualifiers = PHONE)`, `ComponentActivityRule`; sin imágenes)

**Interfaces:**
- Produces: la cadena `chat_new_messages` ("New messages" / "Mensajes nuevos").

Diseño (revisado: no se guarda un `wasAtEnd` derivado de `canScrollForward`, porque cuando la lista crece `canScrollForward` pasa a true y mezcla "el contenido creció" con "el lector subió"): las filas nuevas van DEBAJO y no mueven el viewport, así que "el lector estaba abajo" equivale a "la fila que era la última termina dentro del viewport". Se calcula en el propio `LaunchedEffect(rows)` con `layoutInfo`, con el recuento de filas ya vistas (`seen`) como referencia.

- [ ] **Step 1: Tests que fallan** (`T/ui/ChatScrollTest.kt`)

El estado vive en un `mutableStateOf<BridgeState>` con 30 mensajes alternando `user`/`assistant` con texto `"message $i"` (`ChatState(messages = ...)`, `ChatMessage(id, role, text)`: mira la vista previa de `ChatScreen.kt`) y `ChatScreen` pintada con él (como `ChatScreensTest.show`: `outgoing = emptyList()`, `drafts = emptyList()`, `downloads = emptyMap()`, `cut = false`, `terminal = null`, `snackbar`, `actions = ChatActions.None`, `remoteCard = {}`). Para desplazar la lista, usar `onNode(hasScrollToIndexAction())` (no `hasScrollAction()`: el campo de texto de la barra de entrada también puede exponer `scrollBy` y el matcher encontraría dos nodos). Casos, uno por test:
1. Al empezar, el último mensaje (`"message 29"`) es visible (la lista arranca abajo, como hoy) y no hay aviso (`onNodeWithText(text(R.string.chat_new_messages)).assertDoesNotExist()`).
2. Se sube con `performScrollToIndex(0)`; se añade `"message 30"` al estado y `waitForIdle()`: `"message 30"` no es visible, el aviso SÍ lo es y la lista no se movió (el primer mensaje sigue visible).
3. Tras el caso 2, se toca el aviso: `"message 30"` pasa a ser visible y el aviso desaparece (`assertDoesNotExist`).
4. Estando abajo, un mensaje nuevo (`"message 31"`) se sigue sin tocar nada y sin aviso.
5. (Falla con un diseño basado en `canScrollForward`.) Abajo, la última burbuja CRECE en sitio (mismo id, texto largo: `"message 29"` pasa a un texto de 300 caracteres con el mismo `id`), luego llega `"message 30"`: se sigue sin aviso.
6. (Falla si se cuenta cualquier bajada de filas como novedad.) Con una tira de herramientas visible (estado con una herramienta en marcha: `ToolStrip.of(state.chat, state.sessionState.state)` no vacío; mira `ToolStripTest` para construirla) y el lector ARRIBA, la tira desaparece (una fila menos): no aparece el aviso.

- [ ] **Step 2: Ejecutar: falla** (hoy la lista salta al final en el caso 2 y no hay aviso): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/android && gradle :app:testDebugUnitTest --tests '*ChatScrollTest'`.

- [ ] **Step 3: Implementar**

Cadena, junto a las del chat, en los dos ficheros: en `<string name="chat_new_messages">New messages</string>`, es `<string name="chat_new_messages">Mensajes nuevos</string>` (mismo lugar y orden en los dos).

En `ChatScreen`, sustituir `LaunchedEffect(rows) { if (rows > 0) listState.animateScrollToItem(rows - 1) }` por:

```kotlin
// Follows what arrives only while the reader is at the end. New rows go below and do not move the viewport, so "was at
// the end" is "the row that was last ends inside the viewport"; a drop in rows (the tool strip going) is not news.
var seen by remember { mutableIntStateOf(0) }
var unseen by remember { mutableStateOf(false) }
LaunchedEffect(rows) {
    val info = listState.layoutInfo
    val wasAtEnd = seen == 0 || info.visibleItemsInfo.any { it.index == seen - 1 && it.offset + it.size <= info.viewportEndOffset }
    val grew = rows > seen
    seen = rows // before animating: a cancelled animation does not leave the old count
    if (rows == 0) return@LaunchedEffect
    if (wasAtEnd) {
        unseen = false
        listState.animateScrollToItem(rows - 1)
    } else if (grew) {
        unseen = true
    }
}
LaunchedEffect(listState) { snapshotFlow { !listState.canScrollForward }.collect { if (it) unseen = false } }
```

y el aviso, sobre la lista (dentro de un `Box` que contenga la `LazyColumn` y el aviso, con el mismo peso que tenía la lista):

```kotlin
if (unseen) {
    val scope = rememberCoroutineScope()
    FilledTonalButton(
        onClick = { scope.launch { listState.animateScrollToItem(rows - 1) } },
        modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 8.dp),
    ) { Text(stringResource(R.string.chat_new_messages)) }
}
```

(Imports si faltan: `mutableIntStateOf`, `snapshotFlow`, `rememberCoroutineScope`, `FilledTonalButton`, `Box`, `launch`. Si algún caso no se estabiliza con este diseño, se ajusta el diseño, no el test; el comportamiento de los seis casos es el contrato. Al volver de Ajustes el chat recompone con `seen = 0` y salta al final, como ya hacía: queda anotado en el fragmento.)

- [ ] **Step 4: Tests y commit**

Run: `gradle :app:testDebugUnitTest --tests '*ChatScrollTest' --tests '*ChatScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; ninguna imagen existente debe cambiar (las de las Tasks 4 y 5 siguen verificándose).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el chat sigue lo nuevo solo si el lector esta abajo y avisa si no

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Cierre del paquete

**Files:**
- Modify: `docs/superpowers/fragmentos/ux-revision.md` (nueva sección "P2")
- Modify: `.superpowers/sdd/2026-10-03-ux/progress.md` (lo hace el controlador)

- [ ] **Step 1: Suite completa**

Run: `gradle :app:testDebugUnitTest :app:lintDebug` (desde `android/`) → PASS entero (lint falla por traducción que falte).

- [ ] **Step 2: Fragmento**

Añadir a `docs/superpowers/fragmentos/ux-revision.md` la sección "P2 · onboarding y estados del chat": qué cambió (pie apilado y paso 2 sin "Listo" falso, tarjeta de ajustes restringidos, avisos al responder el consentimiento, cabecera de una línea con banner, `NOT_SET_UP`, chat vacío, scroll que no arrastra), imágenes regrabadas y creadas (las nuevas `chat_*`), y lo que sigue necesitando el Pixel (el banner con TalkBack: se anuncia una vez; el aviso `consent_done_steps`/`consent_declined` en el flujo real de `adbuddy setup`; que `BridgePrefs.link` se rellena al terminar `setup` y el estado "sin configurar" desaparece con la app abierta). Pendiente de P2, aplazado con motivo: C13 (tarjeta compacta del modo remoto apagado: pide diseño propio), indicador de "pensando" de C20 (pide un estado nuevo en el chat), aviso de depuración inalámbrica apagada de O2 (pide leer `Settings.Global.adb_wifi_enabled` y decidir qué mostrar).
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P2 de UX`, con el trailer exacto).

Aplazado de P2 con motivo (el controlador lo apunta en el ledger como `Ruling:`; el fragmento lo nombra): C13 (tarjeta compacta del modo remoto apagado: pide diseño propio); indicador de "pensando" y distinguir "reconectando" de "esperando al PC" de C20 (piden estados nuevos en el chat); aviso de depuración inalámbrica apagada y enlace a `troubleshooting.md` de O2 (piden leer `Settings.Global.adb_wifi_enabled` y decidir qué mostrar); pista de "cómo retomarlo" en el paso 1 de O10 (el paso 2 la lleva en `access_later_hint`); pie de `ConsentScreen` apilado con letra grande y `SecondaryActionButton` allí (P5, que toca `Screens.kt`).

## Validaciones manuales pendientes (Pixel)

Solo para el fragmento; ninguna se pide al usuario ahora: el banner del chat con TalkBack (se anuncia una vez al aparecer), los avisos `consent_done_steps` y `consent_declined` en el flujo real de `adbuddy setup`, y que `BridgePrefs.link` se rellena al terminar `setup` y el estado "sin configurar" desaparece con la app abierta. Lo demás está cubierto por tests de semántica y por las imágenes en la JVM.
