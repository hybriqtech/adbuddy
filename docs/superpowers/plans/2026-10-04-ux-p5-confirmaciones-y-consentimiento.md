# UX P5: confirmaciones 🔴 y consentimiento — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **No ejecutar hasta que el líder avise de que #77 (tapjacking + armado de 600 ms, rama `fix/confirm-arming`) está en `main`.** Este plan está escrito contra el código de `origin/fix/confirm-arming`.

**Goal:** Que la tarjeta 🔴 y el consentimiento se entiendan mejor sin debilitar nada: una pregunta 🔴 cerrada sin querer se puede volver a abrir desde un aviso fijo del chat (la misma tarjeta, con su armado y su desplazamiento obligatorio), el diálogo enseña cuánto queda para que caduque, "Rechazar" pasa a ser el botón más visible y "Permitir durante la sesión" el más discreto y con su alcance escrito, y el consentimiento dice con claridad lo que concede la accesibilidad y no aprieta sus botones con letra grande.

**Architecture:** Funciones puras nuevas en `confirm/ConfirmUi` (qué preguntas abiertas están cerradas, segundos que quedan), un banner del chat (`ui/PendingQuestionsBanner`), y cambios de presentación en `ConfirmDialog` y `ConsentScreen`. NO cambia qué se envía, cuándo, ni quién puede aprobar: el aviso fijo solo quita un id del conjunto `closed` (reabre el mismo diálogo, que vuelve a esperar 600 ms desde que aparece y exige leer hasta el final como siempre).

**Tech Stack:** Kotlin, Jetpack Compose Material 3, Robolectric (SDK 34) + Compose ui-test + Roborazzi.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.5, línea ~210-214: botones, «Permitir esto durante la sesión», armado de 600 ms) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos C2, C7, C8, C9, O9, O21).

## Global Constraints

- **Regla fija del líder: ningún cambio puede dar al agente (ni a un toque inyectado) una forma de aprobar una 🔴, ni atajos de un toque.** El aviso de preguntas cerradas NUNCA tiene botón de permitir: solo reabre la tarjeta. Permitir y Permitir durante la sesión siguen esperando `ConfirmUi.ARMING_MS` (600 ms) tras aparecer, tras volver la app al frente y tras reabrirse, y siguen desactivados hasta desplazar el texto hasta el final; Rechazar no espera. `ObscuredTouchGuard` (tapjacking) y el armado de `seguridad` NO se tocan ni se debilitan: sus tests (`ConfirmDialogArmingTest`, `ConfirmHostArmingTest`, `ConfirmArmingTest`, `ObscuredCountsTest`, `ObscuredFlagsTest`, `TouchGuardBehaviorTest`, `TouchGuardSourceTest`) NO se modifican y pasan: los tests nuevos de este plan van SIEMPRE en clases nuevas (`ConfirmReopenTest`, `ConfirmDialogLayoutTest`, ...), nunca ampliando esas.
- Cada cambio de texto de consentimiento o de la tarjeta 🔴 pasa por revisión `adbuddy-reviewer-opus` (plan, cada tarea y revisión final).
- Textos de usuario en inglés (`res/values/strings.xml`) y español (`res/values-es/strings.xml`), con las mismas claves y el mismo orden; "fichero" (no "archivo") en español; `\'` en el XML inglés; `StringsParityTest` y `NoHardcodedTextTest` pasan. Ningún texto literal en Kotlin.
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Gradle (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon --offline` (en los pasos, `gradle`). Tarda minutos: timeout amplio o segundo plano; sin `sleep` en primer plano. `android/local.properties` ya existe. Nunca `connected*`, nunca emulador. El PC anda justo de memoria: un solo Gradle a la vez.
- Imágenes: en Windows `testDebugUnitTest` las verifica; para regrabar una clase, `gradle :app:testDebugUnitTest -Padbuddy.screens=record --tests '*NombreDeLaClase'`, mirar con Read cada PNG que cambie y comprobar con `git status` que solo cambian las esperadas. Una imagen que cambia sin esperarlo es un fallo: no se regraba, se cuenta en el informe.
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux`, rama `ux/p5` (creada desde `origin/fix/confirm-arming`; no cambiar de rama; nunca `git worktree add`). La shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...` en cada comando.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el hook de commit lo exige). Sin push.
- Rutas relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` (código), `android/app/src/test/kotlin/tech/hybriq/adbuddy/` (`T/`) y `android/app/src/main/res/` (`res/`), salvo que se diga otra cosa.
- No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md` ni la spec: las filas van en `docs/superpowers/fragmentos/ux-p5.md` (fichero propio de esta rama, para no chocar con el fragmento acumulado de las otras ramas).
- Esta rama NO parte de P2-P4: `ChatRoot`, `ChatScreen`, `MainActivity` y `ChatBubbleActivity` tienen aquí sus firmas de `main`/`fix/confirm-arming`; los parámetros nuevos llevan valor por defecto y se añaden al final (antes de la lambda final) para que la integración con P2-P4 sea una fusión de pocas líneas.
- Fuera de este plan, con motivo (se dicen en el fragmento): **C10** (la notificación 🔴 en pantalla bloqueada: la spec decide qué botones salen en el bloqueo —Rechazar, y Permitir pide desbloquear— y lo prueba `TestAllowFromTheLockedShadeAsksToUnlock`; una versión pública podría llevar acciones y ocultar el comando, pero decidirlo es de `seguridad` y de quien lleve la spec: pasa a `seguridad`, con su fila en el fragmento); **O1** (el filtro de toques tapados en `ConsentScreen` y `RemoteCard`) ya lo hizo #77, que es la base de esta rama; **`CATEGORY_CALL`** de `ConfirmNotifier` (puede decidir si No molestar deja pasar la 🔴: no se toca sin evidencia); **O12** («pendiente de enviar al PC» en Ajustes: necesita un estado nuevo en `a11y/UiHelpers`, territorio de `latencia`); **C3/O4**: ya están en P4.

---

### Task 1: Una 🔴 cerrada sin querer se puede reabrir desde el chat (C2)

El diálogo 🔴 se cierra con un toque fuera o con Atrás y, si la notificación se descartó o los avisos están bloqueados, la pregunta queda inalcanzable hasta que caduca. Se deja de cerrar al tocar fuera (Atrás sigue cerrando: es la salida segura para llegar a Pausar o Cortar acceso) y el chat enseña un aviso fijo "N acciones delicadas esperan tu respuesta · Ver" mientras haya preguntas abiertas y cerradas; "Ver" solo quita el id de `closed`, es decir, reabre la misma tarjeta.

**Files:**
- Modify: `confirm/ConfirmUi.kt` (función pura `closedWaiting`), `ui/Screens.kt` (`ConfirmDialog`: `DialogProperties(dismissOnClickOutside = false)`), `ui/ChatScreen.kt` (colocar el aviso), `ui/ChatRoot.kt` (calcularlo y el parámetro `onReopen`), `MainActivity.kt` y `ChatBubbleActivity.kt` (pasar `onReopen = { closed = closed - it }`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Create: `ui/PendingQuestionsBanner.kt`
- Test: ampliar `T/confirm/ConfirmUiTest.kt`; nuevo `T/ui/PendingQuestionsBannerTest.kt`; ampliar `T/confirm/ConfirmHostArmingTest.kt` (o clase nueva `T/confirm/ConfirmReopenTest.kt` con su mismo andamiaje); `T/screens/PendingQuestionsBannerScreensTest.kt` con imágenes
- Regrabar (nuevas): `pending_questions_banner_*` en los cuatro modos

**Interfaces:**
- Produces: `ConfirmUi.closedWaiting(state: BridgeState, closed: Set<String>, now: Long): List<ConfirmRequest>` = las preguntas de `state.pending` cuyo id está en `closed` y cuyo `status(it, state, now) == Status.OPEN` (una caducada, enviada o sin conexión no cuenta: no hay nada que contestar).
- Produces: `@Composable fun PendingQuestionsBanner(count: Int, onShow: () -> Unit, modifier: Modifier = Modifier)`: un `Surface` de `errorContainer` con el texto plural `confirm_waiting` y un `TextButton` `confirm_waiting_show`; el texto lleva `liveRegion = Polite`; NO hay ningún botón que responda.
- Modifies: `ChatScreen(..., remoteCard)`: gana `pendingQuestions: Int = 0` y `onShowQuestions: () -> Unit = {}` (con valor por defecto, antes de `remoteCard`, que sigue siendo la lambda final; `ChatRoot` los pasa con nombre); `ChatRoot(app, focusConfirm, closed, firstRun = true, onReopen: (String) -> Unit = {}, onClose)` (parámetro nuevo antes de `onClose`, con valor por defecto; `MainActivity` y `ChatBubbleActivity` lo pasan).
- Produces las cadenas: `confirm_waiting` (plural, con el mismo vocabulario que la etiqueta `confirm_label` «delicate action»: one "1 delicate action is waiting for your answer" / other "%d delicate actions are waiting for your answer"; es: "1 acción delicada espera tu respuesta" / "%d acciones delicadas esperan tu respuesta") y `confirm_waiting_show` ("Show" / "Ver"), junto a `confirm_*`, mismo lugar y orden en los dos ficheros (los plurales con `<plurals name="confirm_waiting">` como `confirm_hidden`).

- [ ] **Step 1: Tests que fallan**
- `ConfirmUiTest`: `closedWaiting` devuelve solo las preguntas cerradas y abiertas (una cerrada pero caducada —`now` pasado su `expiresAt`—, enviada —en `state.answering`— o con el puente sin conexión NO cuenta; una abierta no cerrada tampoco), en el orden de `pending`.
- `PendingQuestionsBannerTest`: con `count = 2`, el texto plural de 2 y el botón; con 1, el singular; pulsar el botón llama a `onShow` una vez; el texto lleva `LiveRegion == Polite`; NO existe ningún nodo con `action_allow` ni `action_allow_session` ni `action_deny`. `PendingQuestionsBannerScreensTest` (4 `ScreenMode`, andamiaje de `SettingsScreensTest`): `assertTextsFit()` y `shot("pending_questions_banner", mode)`.
- Flujo (clase NUEVA `T/confirm/ConfirmReopenTest.kt` con el andamiaje de `ConfirmHostArmingTest`: `ConfirmHost` con `current` y `reply` observables, `closed` en estado; `ConfirmHostArmingTest` no se modifica): (a) con una pregunta abierta y NO cerrada, el diálogo se ve y NO hay banner; (b) tras cerrar el diálogo con Atrás (`onClose`), el diálogo desaparece y `closedWaiting` da 1; con el banner pintado por `ChatScreen`/`PendingQuestionsBanner` (si el flujo no incluye el chat, un test de `ChatScreen` con `pendingQuestions = 1` muestra el banner); (c) al pulsar "Ver" (`onReopen(id)` que quita el id de `closed`), el diálogo vuelve, `reply` NO se ha llamado, y "Permitir" está DESACTIVADO hasta pasados `ConfirmUi.ARMING_MS` (avanzar el reloj 599 ms: sigue desactivado; 601: activo si el texto cabe) y "Rechazar" activo desde el primer fotograma; (d) un toque fuera del diálogo NO lo cierra y Atrás sí. OJO: un `performTouchInput` de Compose ui-test sobre un nodo NO llega a `Dialog.onTouchEvent`, así que «`onClose` no se llama» pasaría también con `dismissOnClickOutside = true`; y `activity.onBackPressedDispatcher` (OnboardingFlowTest) no llega a la ventana del diálogo. Haz Atrás con `Espresso.pressBack()` (viene con `ui-test-junit4`) y el toque fuera con Espresso sobre la raíz del diálogo (`onView(isRoot()).inRoot(isDialog())` y un clic en una esquina), y exige un RED: con el valor por defecto de `AlertDialog` (sin `DialogProperties`) `onClose` SÍ se llama. Si no hay manera de verlo en rojo, el test de (d) pasa a ser de FUENTE (en el estilo de `TouchGuardSourceTest`: `ConfirmDialog` contiene `DialogProperties(dismissOnClickOutside = false)`) y el informe lo dice.
- `ChatScreensTest` (si existe en esta rama) o un test nuevo de `ChatScreen`: con `pendingQuestions = 0`, ningún nodo con `confirm_waiting_show`; con 1, el aviso.

- [ ] **Step 2: Ejecutar: fallan**: `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/android && gradle :app:testDebugUnitTest --tests '*ConfirmUiTest' --tests '*PendingQuestionsBanner*' --tests '*ConfirmReopenTest'`.

- [ ] **Step 3: Implementar**

`ConfirmUi.closedWaiting` (pura, junto a `question`):

```kotlin
/** The open questions the person closed: the chat says they are still waiting (and "Show" opens the same dialog again). */
fun closedWaiting(state: BridgeState, closed: Set<String>, now: Long): List<ConfirmRequest> =
    state.pending.filter { it.id in closed && status(it, state, now) == Status.OPEN }
```

`ConfirmDialog` (`ui/Screens.kt`): `AlertDialog(onDismissRequest = onClose, properties = DialogProperties(dismissOnClickOutside = false), ...)` y el KDoc: «can be closed with Back (and Close when it has no buttons), not with a tap outside: a stray tap no longer hides a question; the chat keeps a notice that reopens it». El resto del diálogo (los botones, `ObscuredTouchGuard`, el armado) NO cambia en esta tarea.

`ui/PendingQuestionsBanner.kt`:

```kotlin
@Composable
fun PendingQuestionsBanner(count: Int, onShow: () -> Unit, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    Surface(color = scheme.errorContainer, contentColor = scheme.onErrorContainer, modifier = modifier.fillMaxWidth()) {
        Row(Modifier.padding(start = 16.dp, end = 8.dp, top = 4.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                pluralStringResource(R.plurals.confirm_waiting, count, count),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = onShow) { Text(stringResource(R.string.confirm_waiting_show)) }
        }
    }
}
```

`ChatScreen`: dentro del `Column` del cuerpo, lo primero (antes de `remoteCard()`): `if (pendingQuestions > 0) PendingQuestionsBanner(pendingQuestions, onShowQuestions)` (al integrar con P4, donde `ChatScreen` pinta `LinkBanner` y `NotificationsBanner` antes de `remoteCard()`, el aviso de preguntas va justo ANTES del de notificaciones; nunca coincide con el `LinkBanner` de WAITING porque sin conexión la pregunta está OFFLINE y el contador es 0; copia el estilo de `NotificationsBanner`). `ChatRoot`: `val now = ...` (el reloj que usa `ConfirmHost`: `app.clock.now()`; recalcúlalo con la misma cadencia que el chat recompone y, además, cuando `state.pending` o `closed` cambien; con un reloj PROPIO de `ChatRoot`: `var now by remember { mutableLongStateOf(app.clock.now()) }` y un `LaunchedEffect(state.pending, closed)` con el mismo bucle de `ConfirmUi.nextExpiry` que `ConfirmHost`; NO se refactoriza ni se toca `ConfirmHost.kt` en esta tarea: su `now` lo reescribe el camino del toque y es código del armado de `seguridad`), `val waiting = ConfirmUi.closedWaiting(state, closed, now).size` y `ChatScreen(..., pendingQuestions = waiting, onShowQuestions = { ConfirmUi.closedWaiting(state, closed, app.clock.now()).forEach { onReopen(it.id) } } (con el reloj del momento del toque, no el capturado en la composición: una pregunta que acaba de caducar ya no se reabre), ...)` (reabre todas las cerradas y abiertas: `ConfirmHost` muestra de una en una, la primera). `MainActivity` y `ChatBubbleActivity`: `ChatRoot(app, focusConfirm, closed, onReopen = { closed = closed - it }) { closed = closed + it }` (ya hay un `closed - it` al tocar la notificación: mismo mecanismo).

- [ ] **Step 4: Ejecutar, regrabar, mirar**

Run: `gradle :app:testDebugUnitTest --tests '*Confirm*' --tests '*PendingQuestionsBanner*' --tests '*Obscured*' --tests '*TouchGuard*' --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS (todos los tests de armado y tapjacking, sin cambios). Grabar `pending_questions_banner_*` (`-Padbuddy.screens=record --tests '*PendingQuestionsBannerScreensTest'`); `git status`: solo se añaden esos 4 PNG. Mirar `pending_questions_banner_light.png` y `_font_200_es.png`.

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): una pregunta roja cerrada sin querer se reabre desde un aviso del chat, y tocar fuera ya no cierra el dialogo

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: El diálogo 🔴 dice cuánto queda para que caduque (C7)

El diálogo solo dice la hora absoluta de caducidad, al final del contenido con scroll; los botones desaparecen al caducar sin aviso previo. Una línea fija sobre los botones dice cuánto queda y se pone en el color de error por debajo de 10 s.

**Files:**
- Modify: `confirm/ConfirmUi.kt` (`secondsLeft`), `ui/Formats.kt` (`countdown`), `ui/Screens.kt` (`ConfirmDialog`: parámetro `now` y la línea), `ui/ConfirmHost.kt` (pasar al diálogo un `displayNow` PROPIO que avance cada segundo mientras haya una pregunta abierta: un estado aparte que SOLO alimenta la cuenta atrás; el `now` que usan `status`, `canSend` y el armado NO se toca), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test: ampliar `T/confirm/ConfirmUiTest.kt` y `T/ui/FormatsTest.kt`; clase nueva `T/confirm/ConfirmDialogLayoutTest.kt` (andamiaje de pintar `ConfirmDialog` de `ConfirmDialogScrollTest`, que no se modifica)

**Interfaces:**
- Produces: `ConfirmUi.secondsLeft(r: ConfirmRequest, now: Long): Long?` = segundos que faltan para `expiry(r)` respecto de `now` (ms del reloj del host, el mismo que `status`), REDONDEADOS HACIA ARRIBA como `PermissionUi.secondsLeft` (`chat/PermissionUi.kt:~30`: si no, enseñaría «0:00» con Permitir aún activo), mínimo 0; `null` si la pregunta no caduca o no se puede leer; `Formats.countdown(seconds: Long): String` = `m:ss` (p. ej. 42 → "0:42", 125 → "2:05", 3 600 → "60:00").
- Modifies: `ConfirmDialog(r, status, clock, armed, now: Long = clock.now(), onClose, onAnswer)` gana `now` DESPUÉS de `armed` y ANTES de `onClose` (`ConfirmHost` pasa `onAnswer` como lambda final y lo demás con nombre: ajusta esa llamada; el valor por defecto deja compilando los tests existentes); con `status == OPEN` y `secondsLeft != null`, una línea con `confirm_expires_in` FUERA de la columna con desplazamiento y siempre visible (ver «Estructura del diálogo» abajo); por debajo de 10 s en `colorScheme.error` y negrita.
- **Estructura del diálogo (Tasks 2 y 3).** Hoy el cuerpo es una `Column(Modifier.verticalScroll(scroll))` y el aviso `confirm_scroll` vive en el `confirmButton`. Las líneas fijas (cuenta atrás y aviso de desplazamiento) NO pueden ir dentro de la parte que desplaza (solo se verían al llegar al final, justo cuando el aviso desaparece) ni fuera sin dejar sitio al desplazamiento (se recortarían con un comando largo). Estructura: `text = { ObscuredTouchGuard(); Column { Column(Modifier.weight(1f, fill = false).verticalScroll(scroll)) { …el contenido de hoy… }; cuentaAtrás; avisoDeDesplazamiento } }`; los botones (`confirmButton`/`dismissButton`) solo llevan botones. Test (clase nueva `T/confirm/ConfirmDialogLayoutTest.kt`): con un comando de 1 200 palabras, `confirm_scroll` y «Expires in» devuelven `assertIsDisplayed()` ANTES de desplazar. La línea `confirm_expires` con la hora absoluta se queda donde está.
- Produces la cadena `confirm_expires_in` ("Expires in %1$s" / "Caduca en %1$s").

- [ ] **Step 1: Tests que fallan**: `secondsLeft`: a 42 s de la caducidad → 42, a 41,2 s → 42 (hacia arriba), a 0,4 s → 1; ya caducada → 0; `expiresAt` ilegible o vacío → `null`. `Formats.countdown`: 0, 9, 42, 60, 125, 3600. En el test de `ConfirmDialog`: con `now` a 42 s de caducar se ve "Expires in 0:42"; con `now` a 9 s se ve "Expires in 0:09" y el nodo lleva el color de error (comprueba por semántica: un `testTag` o por el texto con estilo no es fiable: basta el texto); con `status != OPEN` NO hay línea; los botones y el armado no cambian (los tests de armado pasan sin tocar). El texto de la cuenta atrás NO lleva `LiveRegion` (se anunciaría cada segundo).
- [ ] **Step 2: Ejecutar: fallan**.
- [ ] **Step 3: Implementar** `secondsLeft` (con `expiry(r)`, que ya devuelve ms o `null`), `Formats.countdown`, la línea con la estructura de arriba y, en `ConfirmHost`, un estado `displayNow` que se refresca cada segundo mientras `status == OPEN` (`LaunchedEffect(question?.id, open) { while (open) { displayNow = clock.now(); delay(1000) } }`) y es lo único que recibe `ConfirmDialog(now = displayNow)`; el `now` existente (el que cambia al vencer y el que reescribe el toque) NO se toca.
- [ ] **Step 4: Tests y commit**: `gradle :app:testDebugUnitTest --tests '*Confirm*' --tests '*Formats*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; ninguna imagen cambia.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el dialogo rojo dice cuanto queda para que caduque

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Rechazar es el botón visible; Permitir durante la sesión, el discreto y con su alcance (C8, C9)

En el diálogo 🔴, Rechazar, Permitir y Permitir durante la sesión son `TextButton` iguales, los dos permisos apilados y el aviso de desplazamiento junto a Rechazar. Pasa a: Rechazar relleno (la salida por defecto y el botón más llamativo), Permitir con contorno (`OutlinedButton`: un permiso es una decisión deliberada, no la acción destacada), "Permitir durante la sesión" aparte, más discreto, y el aviso de "desplázate hasta el final" fuera de la parte que desplaza. Ruling: la revisión vio que con `FilledTonalButton` para Rechazar y `Button` para Permitir, Permitir seguía siendo el más llamativo y contradecía el objetivo. La etiqueta dice su alcance: "Permitir este mismo comando durante la sesión".

**Files:**
- Modify: `ui/Screens.kt` (`ConfirmDialog`: los tres botones y el aviso de scroll), `res/values/strings.xml`, `res/values-es/strings.xml` (`action_allow_session`)
- Test: clase nueva `T/confirm/ConfirmDialogLayoutTest.kt` (la misma de la Task 2: se amplía ahí); `ConfirmDialogScrollTest` y `ConfirmDialogArmingTest` NO se modifican y pasan; un test de fuente en el estilo de `TouchGuardSourceTest` que fije la CLASE de cada botón (Rechazar `Button`, Permitir `OutlinedButton`, durante la sesión `TextButton`) leyendo `ConfirmDialog` en `Screens.kt`; sin imágenes (el diálogo se pinta en su propia ventana: se dice en el informe)

**Interfaces:**
- Modifies: `ConfirmDialog` (misma firma): `dismissButton` = Rechazar como `Button` (relleno); `confirmButton` = `Column` con Permitir como `OutlinedButton` y, debajo y separado (`Spacer(8.dp)`), "Permitir durante la sesión" como `TextButton` con `MaterialTheme.typography.labelMedium`; el texto `confirm_scroll` ("Scroll to the end to allow it") pasa del `confirmButton` a la estructura de las Tasks 2/3 (fuera de la parte que desplaza, siempre visible, en negrita), y la cuenta atrás de la Task 2 sube a esa misma estructura si aún estaba en otro sitio. La condición `enabled = !unread && armed` y los `onClick` NO cambian; Rechazar sigue sin esperar ni depender de `armed` ni de `unread`.
- Modifies: `action_allow_session` = "Allow this same command during the session" / "Permitir este mismo comando durante la sesión" (la spec §4.5 dice «Permitir esto durante la sesión» y «solo cubre la misma regla con los mismos argumentos»: el fragmento lo marca como desfase de la spec para el controlador de integración). Se comprueba en `ConfirmNotification*`/`ConfirmUi*` tests que ningún test fije el texto antiguo (`grep -rn "action_allow_session\|during the session\|durante la sesión" android/app/src/test host`; los emutest de `host/internal/emutest`: `txtAllowSession` (`confirm_test.go:~32`) solo se usa en una comprobación NEGATIVA (~203-207), así que con el literal antiguo seguiría pasando sin vigilar nada: la Task 3 lo cambia OBLIGATORIAMENTE al texto nuevo en inglés y avisa de ello en el informe).

- [ ] **Step 1: Tests que fallan**: (a) Rechazar es el botón relleno: el nodo de Rechazar existe, habilitado desde el primer fotograma (armado sin cumplir y texto sin leer), y está ANTES que Permitir en el orden de lectura (`positionInRoot`: Rechazar a la izquierda en LTR, o por encima; elige y comprueba lo que el diseño dé) y es más grande o igual que los otros dos (alto >= 48 dp); (b) "Permitir durante la sesión" va por debajo de Permitir (`positionInRoot.y`) y a más de 8 dp de distancia vertical; (c) el aviso de desplazamiento ya no es hijo del `confirmButton`: existe en el cuerpo mientras hay texto sin leer, y desaparece al llegar al final; (d) los tests de armado y de desplazamiento existentes pasan SIN cambios (Permitir y Permitir durante la sesión desactivados antes de 600 ms y mientras queden líneas; Rechazar activo); (e) el texto nuevo de `action_allow_session` en inglés y español; (f) un test de que ni Permitir ni Permitir durante la sesión se activan por un toque en la zona de Rechazar (y al revés): sus áreas táctiles no se solapan (`boundsInRoot` disjuntas), también a letra 200 % (`ScreenModeRule` en `FONT_200` y `FONT_200_ES`: los botones se reparten en varias líneas y «durante la sesión» no queda pegado a Rechazar).
- [ ] **Step 2: Ejecutar: fallan**.
- [ ] **Step 3: Implementar** como arriba, sin tocar `ObscuredTouchGuard`, `armed`, `unread` ni las llamadas de `onAnswer`.
- [ ] **Step 4: Tests y commit**: `gradle :app:testDebugUnitTest --tests '*Confirm*' --tests '*Obscured*' --tests '*TouchGuard*' --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; el diálogo se pinta en su propia ventana: si no hay imagen posible con `onRoot()`, se hace sobre el contenido o se queda sin imagen y se dice en el informe; ninguna imagen existente cambia.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): Rechazar es el boton visible de la pregunta roja y Permitir durante la sesion dice su alcance

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: El consentimiento dice lo que concede la accesibilidad y no aprieta sus botones (O9, O13/O14 en ConsentScreen)

`consent_a11y` ("Activar el servicio de accesibilidad de ADBuddy…") infravalora lo que concede: el agente podrá leer la pantalla y tocar. La nota de privacidad es el texto más pequeño. Con letra grande, el pie sigue en una fila. NO cambia qué se concede ni cómo se responde: solo cómo se explica y se dispone.

**Files:**
- Modify: `ui/Screens.kt` (`ConsentScreen`: el título como cabecera, `consent_note` en `bodyMedium`, el pie con `ActionFooter` y `SecondaryActionButton`), `res/values/strings.xml`, `res/values-es/strings.xml` (`consent_a11y`)
- Test: nuevo `T/screens/ConsentScreensTest.kt` (con imágenes); `T/flows/` no cambia (`ConsentFeedbackFlowTest` es de P2 y no existe en esta rama)
- Regrabar (nuevas): `consent_*` en los cuatro modos

**Interfaces:**
- Consumes: `ActionFooter(main, other = null, modifier)` y `STACK_FROM_FONT_SCALE` (`ui/ActionFooter.kt`, de P2, commit 30a345d8 de `ux/p2`; en esta rama NO existen: cópialos TAL CUAL con `git show origin/ux/p2:android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/ActionFooter.kt` y su test con `git show origin/ux/p2:android/app/src/test/kotlin/tech/hybriq/adbuddy/ui/ActionFooterTest.kt`, para que la fusión con P2 sea idéntica), `SecondaryActionButton(text, onClick, modifier)` (de P1, ya en `main`).
- Modifies: `consent_a11y` = "Turn on ADBuddy\'s accessibility service: it lets the agent read the screen, tap and type on it, showing a frame and a Pause button while it acts, and never in ADBuddy or in protected apps. Any other service you use stays on." / "Activar el servicio de accesibilidad de ADBuddy: permite al agente leer la pantalla, tocarla y escribir en ella, con un marco y un botón de Pausa mientras actúa, y nunca en ADBuddy ni en las apps protegidas. Los demás servicios que uses se quedan activados." (verificado por la revisión: el servicio también escribe texto —`setText`, `UiExecutor.kt:~255-271`—; clic y escritura se niegan sin marco; el marco se oculta unos segundos tras la última acción; y la LECTURA (`dump`) no mira si el marco apareció: por eso el texto dice «mientras actúa» y no «visible». Esa asimetría —leer sin marco— se pasa a `seguridad` en el fragmento. Si al implementar algo de esto ya no es exacto, el texto se corrige y se avisa.)
- Los otros textos `consent_*` (título, intro, grant, battery, decline, accept, done, done_steps, declined) NO cambian. `ObscuredTouchGuard()` y `onAnswer(Boolean)` (aceptar = `true`, rechazar = `false`) NO cambian.

- [ ] **Step 1: Tests que fallan** (`T/screens/ConsentScreensTest.kt`, parametrizado por `ScreenMode`, andamiaje de `SettingsScreensTest`): `ConsentScreen(host = "my-pc", onAnswer = { answers += it })`: título, intro, las tres filas y la nota visibles (la nota con `bodyMedium`: su altura de línea es la de `bodyMedium`, comprobable por el layout del texto), `assertTextsFit()` (con `scrolled = true` si hace falta: la pantalla scrollea), el título es cabecera (`SemanticsProperties.Heading`), `consent_a11y` es la cadena nueva, `shot("consent", mode)`; pulsar `consent_accept` da `answers == [true]` y `consent_decline` `[false]` (el orden y los textos de los botones no cambian); a 200 % los dos botones se apilan (misma izquierda y mismo ancho, Aceptar arriba; nota para el fragmento: Aceptar queda entonces a todo el ancho, una zona mayor para un toque accidental, y el consentimiento no tiene armado —ya era así—) y a 100 % están en una fila, Rechazar a la izquierda; ambos son `Button`/`OutlinedButton` accionables; el `ObscuredTouchGuard` sigue (los tests de tapjacking existentes pasan sin cambios).
- [ ] **Step 2: Ejecutar: fallan**.
- [ ] **Step 3: Implementar**: el título con `Modifier.semantics { heading() }`; la nota con `style = MaterialTheme.typography.bodyMedium`; el pie con `ActionFooter(main = { m -> Button(onClick = { onAnswer(true) }, modifier = m) { Text(stringResource(R.string.consent_accept)) } }, other = { m -> SecondaryActionButton(stringResource(R.string.consent_decline), { onAnswer(false) }, m) })` dentro del mismo `Surface(tonalElevation = 3.dp)` con su padding; el texto nuevo de `consent_a11y`.
- [ ] **Step 4: Ejecutar, regrabar, mirar**: `gradle :app:testDebugUnitTest --tests '*Consent*' --tests '*Obscured*' --tests '*TouchGuard*' --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; grabar `consent_*` (solo imágenes nuevas); mirar `consent_light.png` y `consent_font_200_es.png`.
- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el consentimiento dice lo que concede la accesibilidad y apila sus botones con letra grande

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: "Permitido en el sistema, sin confirmar aquí" en palabras claras (O21)

El título del estado del acceso a notificaciones es jerga y "aquí" es ambiguo. Solo texto.

**Files:**
- Modify: `res/values/strings.xml`, `res/values-es/strings.xml` (`access_state_confirm`)
- Test: los que fijan el texto (`grep -rn "access_state_confirm" android/app/src/test`): `SettingsScreensTest` (pinta el estado en `settings_proposed_*`, ~44-53) y `BubbleFlowTest` (~118, 146) lo buscan
- Regrabar: `access_needs_confirmation_*`, `settings_proposed_*` y `onboarding_2_notification_access_*` si pintan ese estado (míralo en el diff de imágenes: solo cambian las que enseñan el título)

**Interfaces:**
- Modifies: `access_state_confirm` = "Android allows it; your confirmation is missing" / "Android lo permite; falta tu confirmación". No cambia la semántica del estado ni del botón de confirmar (el detalle `access_detail_confirm` ya lo explica y NO se toca).

- [ ] **Step 1: Test que falla**: el test de pantalla del estado `NEEDS_CONFIRMATION` (en `NotificationAccessScreensTest`) pasa a esperar el texto nuevo (usa `text(R.string.access_state_confirm)`: no hay literal que cambiar; añade una aserción literal del inglés nuevo para fijar el contrato); `assertTextsFit()` sigue.
- [ ] **Step 2: Implementar** las dos cadenas.
- [ ] **Step 3: Ejecutar, regrabar, mirar**: `gradle :app:testDebugUnitTest --tests '*NotificationAccess*' --tests '*Onboarding*' --tests '*SettingsScreensTest' --tests '*BubbleFlowTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'`; regrabar solo las clases cuyas imágenes cambien; `git status`: solo imágenes que enseñan el título del estado `needs_confirmation` (`access_needs_confirmation_*`, `settings_proposed_*` y las del paso 2 si lo pintan); mirar `access_needs_confirmation_light.png` y `_font_200_es.png` (el título cabe: ahora es más largo).
- [ ] **Step 4: Commit** (`feat(app): el estado de acceso sin confirmar se dice en palabras claras`, con el trailer exacto).

---

### Task 6: Cierre del paquete

**Files:**
- Create: `docs/superpowers/fragmentos/ux-p5.md`

- [ ] **Step 1: Suite completa**: `gradle :app:testDebugUnitTest :app:lintDebug` (desde `android/`) → PASS entero. (Además, `host/internal/emutest` no se ejecuta: el emulador es del CI; si algún emutest busca un texto que esta rama cambia —`grep -rn "Allow during the session\|Permitir durante\|Allowed in the system\|accessibility service" host/internal/emutest`—, se ajusta SOLO ese literal y se dice en el informe.)
- [ ] **Step 2: Fragmento** (`docs/superpowers/fragmentos/ux-p5.md`): qué cambió por tarea; imágenes nuevas (`pending_questions_banner_*`, `consent_*`) y regrabadas (las del estado sin confirmar); la regla fija y cómo se cumple (el aviso solo reabre; armado y tapjacking intactos; sus tests sin cambios); aplazado con motivo (C10, `CATEGORY_CALL`, O12, C3/O4 en P4); O1 (tapjacking del consentimiento y de `RemoteCard`) ya lo hizo #77, la base de esta rama; la asimetría de la accesibilidad (leer la pantalla con `dump` sin que haya marco, aunque tocar y escribir sí lo exigen) se pasa a `seguridad` para que decida; C10 pasa a `seguridad` (la spec decide los botones en el bloqueo, no que se vea el comando: una versión pública podría llevar acciones); desfases para el controlador de integración: la spec §4.5 línea ~210 («Permitir esto durante la sesión» pasa a «Permitir este mismo comando durante la sesión») y, si procede, `docs/safety.md`/`docs/usage.md` donde citen las etiquetas; notas honestas (tocar fuera ya no cierra el diálogo; Atrás sí; el aviso del chat solo se ve en el chat: en Ajustes o Actividad con una pregunta cerrada solo está la notificación; la cuenta atrás no se anuncia cada segundo a TalkBack); y las validaciones manuales (el aviso reabre la tarjeta tras cerrarla con Atrás; Rechazar relleno y Permitir durante la sesión aparte en un Pixel; el consentimiento a 200 % con TalkBack).
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P5 de UX`, con el trailer exacto).

## Validaciones manuales pendientes (Pixel)

Solo para el fragmento; ninguna se pide al usuario ahora: cerrar una 🔴 con Atrás y reabrirla desde el aviso del chat (la tarjeta vuelve con "Permitir" apagado 600 ms), el aspecto de los botones de la 🔴 con letra grande, y el consentimiento con TalkBack leyendo la frase de la accesibilidad.
