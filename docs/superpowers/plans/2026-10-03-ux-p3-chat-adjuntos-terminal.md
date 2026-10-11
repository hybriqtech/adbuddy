# UX P3: chat (parar), adjuntos, terminal y diario — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pequeñas mejoras de uso del chat: poder parar al agente desde el móvil, ver que un adjunto se está preparando, un chip de adjunto con el botón de quitar de tamaño táctil, un terminal que avisa de que no hay conexión, avisos que no desaparecen antes de leerse, un diario que dice qué nivel y a qué hora, y la burbuja sin el terminal desplegado.

**Architecture:** Cambios locales en `ui/` y `term/` (composables y un `ChatActions` con una acción más); nada cambia en el protocolo (`agent.interrupt` ya existe en la app y en el host) ni en las confirmaciones. Cada cambio visible lleva su comprobación de semántica y, donde la pantalla ya tiene imágenes de referencia o se crean nuevas, su captura en la JVM (claro, oscuro, 200 %, 200 % en español).

**Tech Stack:** Kotlin, Jetpack Compose Material 3 (BOM 2026.09.00), Robolectric (SDK 34) + Compose ui-test + Roborazzi.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (UI de la app; `agent.interrupt` en la tabla de mensajes, línea ~308) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos C5, C17, C23, C18, C11, C12, C26).

## Global Constraints

- Textos de usuario en inglés (`res/values/strings.xml`) y español (`res/values-es/strings.xml`), con las mismas claves y el mismo orden; "fichero" (no "archivo") en español; `\'` en el XML inglés; `StringsParityTest` y `NoHardcodedTextTest` pasan. Ningún texto literal en Kotlin.
- Nada de este plan cambia la semántica de una confirmación ni el texto de consentimiento. "Parar" solo reduce lo que hace el agente (no concede nada) y no pide confirmación; el agente no puede pulsarlo: ADBuddy es app protegida para él.
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Gradle (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon --offline` (en los pasos, `gradle`). Tarda minutos: timeout amplio o segundo plano; sin `sleep` en primer plano. `android/local.properties` ya existe. Nunca `connected*`, nunca emulador. El PC anda justo de memoria: un solo Gradle a la vez.
- Imágenes: en Windows `testDebugUnitTest` las verifica; para regrabar una clase, `gradle :app:testDebugUnitTest -Padbuddy.screens=record --tests '*NombreDeLaClase'`, mirar con Read cada PNG que cambie y comprobar con `git status` que solo cambian las esperadas. Una imagen que cambia sin esperarlo es un fallo: no se regraba, se cuenta en el informe.
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux`, rama `ux/p3` (no cambiar de rama; nunca `git worktree add`). La shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...` en cada comando.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el hook de commit lo exige). Sin push.
- Rutas relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` (código), `android/app/src/test/kotlin/tech/hybriq/adbuddy/` (`T/`) y `android/app/src/main/res/` (`res/`), salvo que se diga otra cosa.
- No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md`: las filas van en `docs/superpowers/fragmentos/ux-revision.md`.

---

### Task 1: Botón "Parar" (C5)

Con el agente trabajando y el terminal oculto, no hay forma de interrumpirlo desde el móvil. En la barra de entrada, cuando el agente trabaja y no hay NADA redactado (ni texto ni borradores, aunque estos aún se estén subiendo), el botón de enviar pasa a ser "Parar" y manda `agent.interrupt`; tras pulsarlo queda desactivado unos segundos (un segundo toque mandaría un segundo ESC). Si hay algo escrito o un borrador, sigue siendo "Enviar" (se puede mandar un mensaje mientras trabaja, y quien redacta nunca ve "Parar" por error).

**Files:**
- Modify: `ui/ChatScreen.kt` (`ChatActions`: acción nueva `interrupt`; la llamada a `InputBar`), `ui/ChatParts.kt` (`InputBar`), `ui/ChatRoot.kt` (el `ChatActions(...)` de `remember(app)`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test: ampliar `T/screens/ChatScreensTest.kt` (con imágenes) y crear `T/ui/InputBarTest.kt` (sin imágenes)
- Regrabar (nuevas): `chat_working_*` en los cuatro modos

**Interfaces:**
- Modifies: `ChatActions(..., newConversation, interrupt: () -> Unit)` (la acción nueva va AL FINAL de la lista; `ChatActions.None` lleva un `{}` más) y `InputBar(text, onText, drafts, onPhoto, onFile, onCapture, onSend, working: Boolean = false, onStop: () -> Unit = {})` (parámetros nuevos al FINAL con valor por defecto: la vista previa y los tests existentes llaman posicionalmente). Actualizar el KDoc de `InputBar` (`ChatParts.kt:~338`).
- Consumes: `Composer.canSend(text, drafts): Boolean`, `SessionLabel.of(state) == SessionLabel.WORKING`, `Bridge.send(type: String, payload: JsonElement): Boolean`, `Protocol.AGENT_INTERRUPT`, `Empty` y `Protocol.json` (como `TerminalFeed.sendWatch`/`unwatch`: `channel.send(Protocol.TERMINAL_UNWATCH, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty))`).
- Produces: la cadena `chat_stop` ("Stop" / "Parar").

- [ ] **Step 1: Tests que fallan**

`T/ui/InputBarTest.kt` (Robolectric + `createComposeRule`, `@GraphicsMode(NATIVE)`, `@Config(qualifiers = PHONE)`, `ComponentActivityRule`; mismo andamiaje que `SecondaryActionButtonTest`). Cinco tests con `InputBar(text, {}, drafts, {}, {}, {}, onSend = { sent++ }, working, onStop = { stopped++ })`:
1. `working = false`, texto "hi": existe `chat_send` (por `onNodeWithContentDescription`), no existe `chat_stop`; al pulsar, `sent == 1`, `stopped == 0`.
2. `working = true`, texto vacío y sin adjuntos: existe `chat_stop` y NO `chat_send`; al pulsar, `stopped == 1`, `sent == 0`; y el botón está habilitado (`assertIsEnabled`).
3. `working = true`, texto "hi": sigue `chat_send` (y no `chat_stop`): se puede escribir mientras trabaja.
4. `working = true`, texto vacío, con un borrador terminado (`Draft(...)` con `status = DraftStatus.DONE`; el enum es `UPLOADING, DONE, FAILED` en `files/Drafts.kt:25`; `attachment` no hace falta para `canSend`, y si se quiere uno realista, `Attachment("0".repeat(32), "a.png", "image/png", 10, "x")`): `chat_send` (habilitado), no `chat_stop`.
5. `working = true`, texto "hi" y un borrador `UPLOADING`: `chat_send` DESHABILITADO (`canSend` es falso mientras se sube) y NO existe `chat_stop` (quien redacta no ve "Parar").
6. Doble toque: `working = true`, vacío, `mainClock.autoAdvance = false`: pulsar `chat_stop` una vez pone `stopped == 1` y el botón queda deshabilitado (`assertIsNotEnabled`); un segundo `performClick` no suma; tras `mainClock.advanceTimeBy(3_500)` vuelve a estar habilitado.
En `ChatScreensTest`: `chat_working`: `BridgeState(connection = CONNECTED, hostName = "my-pc", sessionState = SessionState(state = "working"), chat = ChatState(messages = [user "Turn the volume down"]))`: subtítulo `session_working`, el botón `chat_stop` visible (`onNodeWithContentDescription`), `assertTextsFit()`, `shot("chat_working", mode)`.

- [ ] **Step 2: Ejecutar: fallan** (`working`, `onStop`, `chat_stop` sin definir): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/android && gradle :app:testDebugUnitTest --tests '*InputBarTest' --tests '*ChatScreensTest'`.

- [ ] **Step 3: Implementar**

Cadena, junto a `chat_send` en los dos ficheros: en `<string name="chat_stop">Stop</string>`, es `<string name="chat_stop">Parar</string>`.

`InputBar`: añadir los dos parámetros al final y cambiar el botón:

```kotlin
val canSend = Composer.canSend(text, drafts)
// Anything being written (text, or a draft even if still uploading) keeps Send: Stop is only for an empty bar, so a person
// composing never interrupts the agent by mistake.
val hasContent = text.isNotBlank() || drafts.isNotEmpty()
// After a tap the button rests a moment: agent.interrupt has no reply, so a second tap would send a second Esc.
var stopping by remember(working) { mutableStateOf(false) }
LaunchedEffect(stopping) { if (stopping) { delay(3_000); stopping = false } }
if (working && !hasContent) {
    FilledIconButton(onClick = { stopping = true; onStop() }, enabled = !stopping) {
        // A square, the usual "stop": Icons.Default.Close already means "discard" in the draft chips, and Stop is in icons-extended (not in the app).
        val tint = LocalContentColor.current
        Box(Modifier.size(14.dp).background(tint).semantics { contentDescription = stopLabel })
    }
} else {
    FilledIconButton(onClick = onSend, enabled = canSend) {
        Icon(Icons.AutoMirrored.Filled.Send, contentDescription = stringResource(R.string.chat_send))
    }
}
```

(con `val stopLabel = stringResource(R.string.chat_stop)` antes. El botón se reconoce por su descripción, no por el icono: si `semantics` en el `Box` no basta para `onNodeWithContentDescription`, poner la descripción en el `FilledIconButton` con `Modifier.semantics { contentDescription = stopLabel }`. Imports: `LaunchedEffect`, `delay`, `Box`, `size`, `background`, `LocalContentColor`.)

`ui/ChatScreen.kt`: `ChatActions` gana `val interrupt: () -> Unit` al final y `None` un `{}` más; en `ChatScreen`, la llamada a `InputBar` pasa `working = SessionLabel.of(state) == SessionLabel.WORKING` y `onStop = actions.interrupt` (importar `tech.hybriq.adbuddy.chat.SessionLabel` si falta). `ui/ChatRoot.kt`: en el `ChatActions(...)` añadir `interrupt = { app.bridge.send(Protocol.AGENT_INTERRUPT, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty)) }` (importar `Protocol` y `Empty` del paquete `bridge`).

- [ ] **Step 4: Ejecutar, regrabar, mirar**

Run: `gradle :app:testDebugUnitTest --tests '*InputBarTest' --tests '*ChatScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS tras grabar las nuevas (`-Padbuddy.screens=record --tests '*ChatScreensTest'`; `git status`: solo se añaden los 4 `chat_working_*`, ninguna existente cambia). Mirar `chat_working_light.png` y `chat_working_font_200_es.png`.

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): boton Parar en la barra de entrada mientras el agente trabaja

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Chips de adjuntos: progreso mientras se preparan y botón de quitar táctil (C17, C23)

Un adjunto que se está preparando (tamaño aún 0) no enseña nada, y el botón de quitar del chip mide 18 dp. El chip pasa a ser propio: etiqueta, progreso (indeterminado si aún no hay tamaño) y un botón de quitar de 48 dp (hermano de la etiqueta: tocar la X nunca es un reintento).

**Files:**
- Modify: `ui/ChatParts.kt` (`DraftChips` y su vista previa)
- Test (nuevo): `T/ui/DraftChipsTest.kt`; crear `T/screens/DraftChipsScreensTest.kt` con imágenes
- Regrabar (nuevas): `draft_chips_*` en los cuatro modos

**Interfaces:**
- Consumes: `DraftChips(drafts: List<Draft>, onRemove: (String) -> Unit, onRetry: (String) -> Unit)`; `Draft(key, name, mime, size, progress, status, ...)` con `DraftStatus.{UPLOADING, FAILED, ...}` (mira `files/Drafts.kt`: el orden de los campos de `Draft` se ve en `DraftChipsPreview`: `Draft("a", "photo.jpg", "image/jpeg", 1000, 400, DraftStatus.UPLOADING)`), `R.string.chat_discard`, `R.string.draft_failed`.
- Produces: la misma firma de `DraftChips`; el botón de quitar es un `IconButton` de 48 dp con `contentDescription = chat_discard`.

- [ ] **Step 1: Tests que fallan**

`T/ui/DraftChipsTest.kt` (mismo andamiaje que `InputBarTest`):
1. Un borrador `UPLOADING` con `size = 0` (preparándose): hay un nodo con `ProgressBarRangeInfo` indeterminado (`SemanticsProperties.ProgressBarRangeInfo == ProgressBarRangeInfo.Indeterminate`): `onNode(hasProgressBarRangeInfo(ProgressBarRangeInfo.Indeterminate)).assertExists()`.
2. Un borrador `UPLOADING` con `size = 1000`, `progress = 400`: progreso determinado (no indeterminado) y `assertExists` de un nodo con `ProgressBarRangeInfo` de valor 0,4.
3. El botón de quitar mide al menos 48 dp en cada dimensión: `onNodeWithContentDescription(text(R.string.chat_discard))`, `fetchSemanticsNode().size` convertido con `displayMetrics.density` es `>= 48 dp`; y al pulsarlo, `onRemove` recibe la clave.
4. Un borrador `FAILED`: tocar el chip llama a `onRetry` con la clave; el botón de quitar sigue ahí y llama a `onRemove`, no a `onRetry` (el toque en la X no es un reintento).
`T/screens/DraftChipsScreensTest.kt` (parametrizado por `ScreenMode`, andamiaje de `SettingsScreensTest`): pinta TRES `DraftChips(listOf(x))` apilados en una `Column` (uno por estado: preparándose, subiendo al 40 %, fallido, con nombres de fichero cortos como `a.jpg`; cada chip mide como mucho ~240 dp, así que ninguno sale de la pantalla: `assertTextsFit` falla con "off screen sideways" si hay varios en una fila con scroll) y comprueba `assertTextsFit()` y `shot("draft_chips", mode)`. El texto del chip fallido admite hasta 4 líneas (`maxLines = if (failed) 4 else 1`), para que a 200 % no se corte con elipsis.

- [ ] **Step 2: Ejecutar: fallan** (el chip actual no tiene progreso indeterminado ni botón de 48 dp): `gradle :app:testDebugUnitTest --tests '*DraftChips*'`.

- [ ] **Step 3: Implementar**

Sustituir el `InputChip` por un chip propio, manteniendo la barra horizontal de chips:

```kotlin
@Composable
fun DraftChips(drafts: List<Draft>, onRemove: (String) -> Unit, onRetry: (String) -> Unit) {
    if (drafts.isEmpty()) return
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        for (d in drafts) {
            val failed = d.status == DraftStatus.FAILED
            Surface(
                shape = MaterialTheme.shapes.small,
                color = if (failed) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceVariant,
                contentColor = if (failed) MaterialTheme.colorScheme.onErrorContainer else MaterialTheme.colorScheme.onSurfaceVariant,
            ) {
                Row(Modifier.padding(start = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    // The retry tap is on the label only: the X is its sibling, so tapping it can never retry.
                    Column(
                        Modifier.widthIn(max = 180.dp).padding(vertical = 8.dp)
                            .then(if (failed) Modifier.clickable(role = Role.Button, onClick = { onRetry(d.key) }) else Modifier),
                    ) {
                        Text(
                            if (failed) d.error.ifEmpty { stringResource(R.string.draft_failed) } else d.name,
                            style = MaterialTheme.typography.labelLarge, maxLines = if (failed) 4 else 1, overflow = TextOverflow.Ellipsis,
                        )
                        if (d.status == DraftStatus.UPLOADING) {
                            // Size 0 is a file still being prepared (or a screenshot being taken): an unknown amount, not an empty one.
                            if (d.size > 0) LinearProgressIndicator(progress = { (d.progress.toFloat() / d.size).coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth())
                            else LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                        }
                    }
                    IconButton(onClick = { onRemove(d.key) }) {
                        Icon(Icons.Default.Close, contentDescription = stringResource(R.string.chat_discard))
                    }
                }
            }
        }
    }
}
```

(Ajustar imports: quitar `InputChip` si ya no se usa; añadir `Role`, `Surface`, `IconButton`, `LinearProgressIndicator`, `clickable`. `IconButton` mide 48 dp por defecto. El clic de reintento va en la columna de la etiqueta, no en el `Surface`, para que el `IconButton` no quede dentro de un nodo clicable y su toque nunca sea un reintento.)

- [ ] **Step 4: Ejecutar, regrabar, mirar, commit**

Run: `gradle :app:testDebugUnitTest --tests '*DraftChips*' --tests '*InputBarTest' --tests '*ChatScreensTest'` → PASS tras grabar las nuevas (`-Padbuddy.screens=record --tests '*DraftChipsScreensTest'`); solo se añaden los 4 `draft_chips_*`; mirar `draft_chips_light.png` y `draft_chips_font_200_es.png` (chips legibles, progreso visible, la X del tamaño del dedo). Ninguna imagen existente cambia (el chat no tiene borradores en sus pantallas).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): chips de adjuntos con progreso mientras se preparan y boton de quitar de 48 dp

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: El terminal avisa de que no hay conexión (C18)

El terminal acepta teclas sin conexión y las ignora sin decirlo. Sin conexión con el PC, las teclas rápidas se desactivan y sobre el terminal aparece un aviso.

**Files:**
- Modify: `ui/ChatParts.kt` (`QuickKeysRow`; nuevo `TerminalOfflineNotice`), `ui/ChatScreen.kt` (el bloque del terminal y `QuickKeysRow`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/ui/TerminalOfflineTest.kt`
- (El `TerminalView` usa un WebView: este plan NO lo pinta en la JVM. El aviso y las teclas son composables propios que se prueban solos; el cableado en `ChatScreen` lo cubre la revisión y la vista previa.)

**Interfaces:**
- Modifies: `QuickKeysRow(enabled: Boolean = true, onKey: (QuickKey) -> Unit)` (`enabled` PRIMERO con valor por defecto, para que la lambda final siga siendo la última: la vista previa `QuickKeysRow {}` de `ChatParts.kt:~463` sigue compilando sin cambios).
- Produces: `@Composable fun TerminalOfflineNotice(modifier: Modifier = Modifier)` y la cadena `terminal_offline` ("No connection with the PC: keys are not sent" / "Sin conexión con el PC: las teclas no se envían").
- Consumes: `BridgeState.Connection.CONNECTED`.

- [ ] **Step 1: Tests que fallan** (`T/ui/TerminalOfflineTest.kt`, andamiaje de `InputBarTest`)
1. `QuickKeysRow(onKey = { keys += it }, enabled = true)`: pulsar el chip "Esc" (`onNodeWithText("Esc")`) añade `QuickKey.ESC`.
2. `QuickKeysRow(enabled = false) { keys += it }`: hay exactamente `QuickKey.entries.size` nodos clicables (`onAllNodes(hasClickAction()).assertCountEquals(QuickKey.entries.size)`) y todos están deshabilitados (`.assertAll(isNotEnabled())`), pulsar no añade nada.
3. `TerminalOfflineNotice()` muestra `terminal_offline` (`assertIsDisplayed`) con `assertTextsFit()` en los cuatro modos (parametrizar este test con `ScreenMode`) y lleva `LiveRegion == Polite` (como `LinkBanner`).

- [ ] **Step 2: Ejecutar: fallan** (parámetro `enabled` y `TerminalOfflineNotice` sin definir): `gradle :app:testDebugUnitTest --tests '*TerminalOfflineTest'`.

- [ ] **Step 3: Implementar**

Cadena, junto a `chat_terminal_*` en los dos ficheros (mismo lugar): en `<string name="terminal_offline">No connection with the PC: keys are not sent</string>`, es `<string name="terminal_offline">Sin conexión con el PC: las teclas no se envían</string>`.

`QuickKeysRow(enabled = true, onKey)`: `AssistChip(onClick = { onKey(k) }, enabled = enabled, label = { Text(k.label) })`. Y:

```kotlin
/** Over the terminal while there is no channel to the PC: what is typed would not arrive. */
@Composable
fun TerminalOfflineNotice(modifier: Modifier = Modifier) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer, contentColor = MaterialTheme.colorScheme.onErrorContainer,
        modifier = modifier.fillMaxWidth().semantics { liveRegion = LiveRegionMode.Polite },
    ) {
        Text(stringResource(R.string.terminal_offline), style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp))
    }
}
```

`ChatScreen`: con `val connected = state.connection == BridgeState.Connection.CONNECTED`; el terminal se pinta dentro de un `Box(Modifier.weight(0.45f).fillMaxSize())` con `TerminalView(...)` y, si `!connected`, `TerminalOfflineNotice(Modifier.align(Alignment.TopCenter))` encima; `QuickKeysRow(enabled = connected) { terminal!!.input(it.bytes) }`; dentro del `Box`, `TerminalView(...)` lleva `Modifier.fillMaxSize()` (el `weight` es del `Box`). El peso `0.45f` y el resto del diseño no cambian.

- [ ] **Step 4: Tests y commit**

Run: `gradle :app:testDebugUnitTest --tests '*TerminalOfflineTest' --tests '*ChatScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; ninguna imagen cambia (las pantallas del chat se pintan con `terminal = null`).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el terminal avisa de que no hay conexion y las teclas rapidas se desactivan

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Avisos largos que no desaparecen antes de leerse (C11)

`cut_restored` avisa de que el puerto de red sigue abierto y se va a los 4 s. Los avisos del host y los errores se leen igual de rápido. Los avisos largos pasan a duración larga.

**Files:**
- Modify: `ui/ChatRoot.kt` (los CUATRO sitios que muestran avisos: el `LaunchedEffect(app)` con `merge(...).collect { snackbar.showSnackbar(it) }`, el `attach(uri)` con `snackbar.showSnackbar(text.ifEmpty { attachFailed })`, el `toggleCut` y el `onMessage` de `RemoteCard`)
- Test (nuevo): `T/ui/SnackbarDurationTest.kt`

**Interfaces:**
- Produces: en `ui/SnackbarPolicy.kt`: `object SnackbarPolicy { const val LONG_FROM = 60; fun duration(text: String, hasAction: Boolean): SnackbarDuration }` (con acción, `Indefinite`; sin acción, `Long` si el texto pasa de 60 caracteres, `Short` en otro caso) y `suspend fun SnackbarHostState.notify(text: String, actionLabel: String? = null): SnackbarResult` (usa `duration(...)` y pone `withDismissAction = duration == Indefinite`, para que un aviso sin fin tenga cierre propio y no tape la barra de entrada).
- Consumes: `SnackbarHostState.showSnackbar(message, actionLabel = null, withDismissAction = false, duration: SnackbarDuration)`.

- [ ] **Step 1: Test que falla** (`T/ui/SnackbarDurationTest.kt`): `SnackbarPolicy.duration("Done", false) == Short`; con un texto de 61 caracteres y sin acción, `Long`; el límite exacto (60 caracteres) es `Short`; con acción (cualquier texto) `Indefinite`. Y, con Robolectric + `createComposeRule` y un `SnackbarHostState` pintado en un `SnackbarHost`, que `notify("x", actionLabel = "Open")` muestra el aviso con acción y un botón de cierre (`withDismissAction`): el nodo con descripción `Dismiss`/equivalente del `Snackbar` existe, y que `notify("Done")` (corto, sin acción) desaparece tras `mainClock.advanceTimeBy(5_000)`.
- [ ] **Step 2: Ejecutar: falla** (`SnackbarPolicy` sin definir).
- [ ] **Step 3: Implementar** `ui/SnackbarPolicy.kt` con lo anterior y usar `notify` en los cuatro sitios de `ChatRoot.kt`. Un aviso largo (`Long`, hasta 10 s) bloquearía al colector del `merge(...)` y los emisores (`tryEmit` con buffer 8 en `AdbuddyApp`, `ChatController` y `Drafts`) descartarían avisos al llenarse: el `collect` lanza cada aviso en su propia corrutina (`scope.launch { snackbar.notify(it) }`; `showSnackbar` ya los encola por su cuenta, en orden). En `toggleCut`, el `showSnackbar(message, actionLabel = if (ok) null else developerOptions)` pasa a `notify(message, actionLabel = ...)` (con la acción —el atajo a las opciones de desarrollador— `showSnackbar` ya era `Indefinite`; el cambio real es el cierre propio). `cut_restored` (largo, sin acción) pasa a `Long`.
- [ ] **Step 4: Tests y commit**

Run: `gradle :app:testDebugUnitTest --tests '*SnackbarDurationTest' --tests '*ChatScreensTest'` → PASS; ninguna imagen cambia.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): los avisos largos del chat duran lo que su lectura y los que llevan accion esperan a que se actue

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Diario con nivel en texto, hora y vacíos distintos (C26)

Los filtros solo tienen 🟢🟡🔴 sin etiqueta, el texto de "nada todavía" sale también cuando solo está vacío el filtro, no se ve la hora (y el diario guarda todo el historial: una hora sin fecha de una entrada de la semana pasada engaña) y el inverso no dice cómo se deshace.

**Files:**
- Modify: `ui/ActivityScreen.kt`, `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/screens/ActivityScreensTest.kt` (con imágenes); `T/ui/FormatsTest.kt` si se añade un formateador
- Regrabar (nuevas): `activity_*` en los cuatro modos

**Interfaces:**
- Consumes: `ActivityScreen(state: BridgeState, onBack: () -> Unit)`, `JournalEntry(id, ts, session, serial, source, argv, level, description, status, inverse, undoneBy)` (`ts` en ISO-8601 con zona; mira `JournalEntry` y cómo la app lo llena), `Formats.time(epochMs, style)`.
- Produces: las cadenas `level_green` ("Safe" / "Seguro"), `level_yellow` ("Caution" / "Cuidado"), `level_red` ("Delicate" / "Delicado") (cortas, para que los cuatro filtros quepan), `activity_filter_empty` ("Nothing at this level yet." / "Nada de este nivel todavía."), `activity_undo_hint` con `%1$s` = el id de la entrada ("Undo it on the PC: adbuddy undo %1$s" / "Para deshacerlo, en el PC: adbuddy undo %1$s": sin id, `adbuddy undo` deshace lo ÚLTIMO, no esta entrada; `host/internal/cli/undo.go:57`) y `internal fun entryTime(ts: String, now: Long = System.currentTimeMillis(), zone: ZoneId = ZoneId.systemDefault()): String?`: la hora corta local si la entrada es de hoy, la fecha corta y la hora si no, `null` si `ts` no se puede leer. El id solo se pinta si encaja en `^j_[0-9A-Za-z]+$` (el diario puede llevar texto que el agente influye); si no encaja, no se muestra el aviso de deshacer.

- [ ] **Step 1: Tests que fallan** (`T/screens/ActivityScreensTest.kt`, parametrizado por `ScreenMode`, andamiaje de `SettingsScreensTest`; pinta `ActivityScreen(state, onBack = {})` dentro de `AdbuddyTheme(dynamic = false)`)
1. `activity_entries`: tres entradas con ids `j_1`, `j_2`, `j_3` (verde "Read the battery level", amarilla "Change the screen timeout" con `inverse`, roja "Delete a file"), con `ts` RFC3339 en horas DISTINTAS (el reloj se inyecta: `ActivityScreen(state, onBack, now = { FIXED })` con `now` como parámetro al final con valor por defecto, para que "hoy" sea determinista y las imágenes no cambien con el día; una de las tres, de otro día, muestra fecha y hora): los chips de filtro tienen texto de nivel (`level_green`, `level_yellow`, `level_red`) además del emoji (`onNodeWithText` con `substring = true`), cada fila muestra su hora (`entryTime` con el mismo `now` y `zone` fijos; horas distintas para que `onNodeWithText` no encuentre nodos duplicados), la entrada con inverso muestra `activity_inverse` y `activity_undo_hint` con `j_2`; `assertTextsFit()`; `shot("activity_entries", mode)`. Los filtros van en un `FlowRow` (`@OptIn(ExperimentalLayoutApi::class)`, como `PermissionCard.kt:~133`; con `verticalArrangement = Arrangement.spacedBy(8.dp)`) para que a 200 % y en español no se deformen ni salgan de la pantalla.
2. `activity_empty`: sin entradas: `home_empty` visible; `shot("activity_empty", mode)`.
3. `activity_filter_empty` (sin imagen): con entradas solo verdes, pulsar el chip rojo: `activity_filter_empty` visible y `home_empty` NO.
Y tests de `entryTime` (JUnit simple, `internal`): independiente de la zona del PC (`entryTime("2026-10-03T18:03:11+02:00", now, zone) == entryTime("2026-10-03T16:03:11Z", now, zone)`, con una `zone` fija), hoy da solo la hora, otro día da fecha y hora, y uno ilegible (`"nope"`, `""`) da `null` (y la fila no muestra hora ni falla). Un id que no encaja en `^j_[0-9A-Za-z]+$` (p. ej. `"x; rm"`) no muestra el aviso de deshacer.

- [ ] **Step 2: Ejecutar: fallan** (cadenas sin definir): `gradle :app:testDebugUnitTest --tests '*ActivityScreensTest'`.

- [ ] **Step 3: Implementar**

Cadenas en los dos ficheros, junto a `filter_all` (en) / equivalente (es), mismo orden. En `ActivityScreen.kt`: los `FilterChip` de nivel pasan a `label = { Text(levelIcon(level) + " " + levelName(level)) }` con `levelName` un `@Composable` que mapea `green/yellow/red` a las cadenas; el vacío distingue `state.journal.isEmpty()` (`home_empty`) de "hay entradas pero el filtro las oculta" (`activity_filter_empty`); la fila de `EntryRow` añade una línea propia `labelMedium` con la hora (`entryTime(e.ts, now)`, solo si no es `null`; `activity_entry` no cambia: tocarla toca la paridad en/es); y el inverso se acompaña de `activity_undo_hint` con el id (`labelMedium`, solo si el id encaja). `entryTime`: `runCatching { OffsetDateTime.parse(ts).toInstant().toEpochMilli() }.getOrNull()?.let { ms -> val today = Instant.ofEpochMilli(now).atZone(zone).toLocalDate() == Instant.ofEpochMilli(ms).atZone(zone).toLocalDate(); if (today) Formats.time(ms, FormatStyle.SHORT, zone) else <fecha corta + hora con DateTimeFormatter.ofLocalizedDateTime(FormatStyle.SHORT).withZone(zone)> }`.

- [ ] **Step 4: Ejecutar, regrabar, mirar, commit**

Run: `gradle :app:testDebugUnitTest --tests '*ActivityScreensTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS tras grabar (`-Padbuddy.screens=record --tests '*ActivityScreensTest'`); solo se añaden `activity_entries_*` y `activity_empty_*`; mirar `activity_entries_light.png` y `activity_entries_font_200_es.png`.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el diario dice el nivel con palabras, la hora y que hacer para deshacer

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: La burbuja arranca con el terminal plegado (C12)

La burbuja de conversación mide unos 600 dp de alto y comparte el diseño del chat: con el terminal desplegado por defecto, la lista de mensajes queda en unas decenas de dp. En la burbuja, el terminal empieza plegado (se despliega con la flecha de siempre); en la app a pantalla completa sigue desplegado.

**Files:**
- Modify: `ui/ChatScreen.kt` (`terminalShown`), `ui/ChatRoot.kt` (parámetro nuevo), `ChatBubbleActivity.kt` (`firstRun = false` ya lo distingue)
- Test: ampliar `T/screens/ChatScreensTest.kt` (sin imagen nueva) y OBLIGATORIAMENTE `T/flows/BubbleFlowTest.kt` (pinta `ChatBubbleActivity` con `createAndroidComposeRule<ChatBubbleActivity>`): sin una aserción ahí, quitar `terminalOpenByDefault = false` de `ChatBubbleActivity.kt:30` no rompería ningún test y es lo único que implementa C12

**Interfaces:**
- Modifies: `ChatScreen(..., hostNow, configured, remoteCard)` gana `terminalOpenByDefault: Boolean = true` justo después de `configured` (con valor por defecto; los llamantes existentes usan argumentos posicionales hasta `hostNow` y con nombre `configured`) y `ChatRoot(app, focusConfirm, closed, firstRun = true, terminalOpenByDefault: Boolean = true, onClose)` (parámetro nuevo con valor por defecto, ANTES de `onClose` para que la lambda final siga siendo la última; `ChatBubbleActivity` pasa `terminalOpenByDefault = false`).

- [ ] **Step 1: Test que falla**: en `ChatScreensTest`, `ChatScreen` pintada con `terminalOpenByDefault = false` y un `terminal` no nulo es complicado en la JVM (WebView): comprobar en su lugar el estado del botón de la cabecera, que es lo que depende de `terminalShown`: con `terminalOpenByDefault = false` el botón de la cabecera tiene la descripción `chat_terminal_show` (plegado) y con `true` (por defecto) `chat_terminal_hide`; ver `ChatHeader` (`terminalShown` decide la descripción). Se prueba con `terminal = null`: `terminalShown` sigue cambiando el icono aunque no haya terminal (`terminalOn = terminalShown && terminal != null` solo gobierna el cuerpo).
- [ ] **Step 2: Ejecutar: falla** (`terminalOpenByDefault` sin definir).
- [ ] **Step 3: Implementar**: `var terminalShown by rememberSaveable { mutableStateOf(terminalOpenByDefault) }`; `ChatRoot` pasa el valor a `ChatScreen`; `ChatBubbleActivity`: `ChatRoot(app, focusConfirm = null, closed = closed, firstRun = false, terminalOpenByDefault = false, onClose = { ... })`.
- [ ] **Step 4: Tests y commit**

En `BubbleFlowTest`: la burbuja muestra el botón con descripción `chat_terminal_show` (terminal plegado: `onNodeWithContentDescription(...).assertIsDisplayed()`) y NO `chat_terminal_hide`; y, en `T/flows/` o `ChatScreensTest`, con el valor por defecto (la app normal) aparece `chat_terminal_hide`.

Run: `gradle :app:testDebugUnitTest --tests '*ChatScreensTest' --tests '*BubbleFlowTest'` → PASS; ninguna imagen cambia (las imágenes del chat usan el valor por defecto).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): la burbuja arranca con el terminal plegado

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Cierre del paquete

**Files:**
- Modify: `docs/superpowers/fragmentos/ux-revision.md` (nueva sección "P3")

- [ ] **Step 1: Suite completa**: `gradle :app:testDebugUnitTest :app:lintDebug` (desde `android/`) → PASS entero.
- [ ] **Step 2: Fragmento**: sección "P3 · chat, adjuntos, terminal y diario": qué cambió por tarea, imágenes nuevas (`chat_working_*`, `draft_chips_*`, `activity_*`) y que no cambia ninguna existente, aplazado con motivo (C19, accesibilidad, tamaño del terminal y teclas ←, →, Shift+Tab: piden ajustes en el JS del terminal y en las teclas rápidas; C16, la captura que se hace de ADBuddy mismo: pide decidir el comportamiento; C12, persistir la preferencia del terminal y uno solo a la vez con poca altura: pide diseño; la acción "Restaurar" tras `cut_done` de C11: toca el corta-accesos, va con P5), y las validaciones manuales pendientes (el botón Parar contra el agente real; el aviso del terminal sin conexión sobre el WebView real, que la JVM no pinta; el diario con horas reales).
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P3 de UX`, con el trailer exacto).

## Validaciones manuales pendientes (Pixel)

Solo para el fragmento; ninguna se pide al usuario ahora: "Parar" contra un agente real trabajando (el host recibe `agent.interrupt` y la sesión lo refleja), el aviso de "sin conexión" sobre el terminal real (el WebView no se pinta en la JVM) y las teclas desactivadas, y las horas del diario con zonas horarias reales. Lo demás está cubierto por tests de semántica y por las imágenes en la JVM.
