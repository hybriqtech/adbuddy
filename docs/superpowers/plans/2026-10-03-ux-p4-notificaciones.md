# UX P4: notificaciones (la pregunta del agente se oye y el permiso se explica) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el usuario se entere de que algo le espera: un aviso propio, de alta importancia y genérico cuando el agente hace una pregunta de permiso y la app no está delante; el permiso de notificaciones pedido con su motivo (y no en frío encima de la pantalla de configuración); y un banner en el chat si las notificaciones de ADBuddy están bloqueadas, con atajo a los ajustes. Más descripciones para los canales.

**Architecture:** Un planificador (`AskNotice`) decide publicar o retirar el aviso según las preguntas pendientes y si la app está delante (`AdbuddyApp.foreground`); `ConversationNotifier.ask` lo construye (sin botones, sin texto del agente) y `BridgeService` lo aplica. El estado de las notificaciones se lee en una función pura (`NotificationsStatus`) y se enseña en un banner del chat; el permiso se pide desde `ChatRoot` tras un diálogo con el motivo. No cambia el protocolo ni ninguna confirmación.

**Tech Stack:** Kotlin, Jetpack Compose Material 3 (BOM 2026.09.00), Robolectric (SDK 34) + Compose ui-test + Roborazzi, `NotificationManagerCompat`.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.5 confirmaciones y notificaciones, §7.4 notificación de conversación) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos C1, C3, O4, C27 en parte).

## Global Constraints

- Textos de usuario en inglés (`res/values/strings.xml`) y español (`res/values-es/strings.xml`), con las mismas claves y el mismo orden; "fichero" (no "archivo") en español; `\'` en el XML inglés; `StringsParityTest` y `NoHardcodedTextTest` pasan. Ningún texto literal en Kotlin.
- SEGURIDAD: el aviso de la pregunta del agente es GENÉRICO: nunca lleva la herramienta, el comando, el detalle ni el resumen de la pregunta, y NO lleva botones de respuesta (se contesta dentro de la app, con la tarjeta y su guarda). Tocarlo abre `MainActivity` sin `EXTRA_CONFIRM`: no salta ninguna guarda. No cambia ninguna confirmación 🔴 ni sus notificaciones (`ConfirmNotifier`), ni el texto de consentimiento `consent_*`. El banner y el permiso solo informan o piden el permiso del sistema. El diálogo de motivo NUNCA se muestra con una pregunta 🔴 o del agente pendiente (no puede quedar encima de una decisión).
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Gradle (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon --offline` (en los pasos, `gradle`). Tarda minutos: timeout amplio o segundo plano; sin `sleep` en primer plano. `android/local.properties` ya existe. Nunca `connected*`, nunca emulador. El PC anda justo de memoria: un solo Gradle a la vez.
- Imágenes: en Windows `testDebugUnitTest` las verifica; para regrabar una clase, `gradle :app:testDebugUnitTest -Padbuddy.screens=record --tests '*NombreDeLaClase'`, mirar con Read cada PNG que cambie y comprobar con `git status` que solo cambian las esperadas. Una imagen que cambia sin esperarlo es un fallo: no se regraba, se cuenta en el informe.
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux`, rama `ux/p4` (no cambiar de rama; nunca `git worktree add`). La shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...` en cada comando.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el hook de commit lo exige). Sin push.
- Rutas relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` (código), `android/app/src/test/kotlin/tech/hybriq/adbuddy/` (`T/`) y `android/app/src/main/res/` (`res/`), salvo que se diga otra cosa.
- No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md`: las filas van en `docs/superpowers/fragmentos/ux-revision.md`.
- Fuera de este plan (van con P5, con revisión opus): `CATEGORY_CALL` y la visibilidad en pantalla bloqueada de la notificación 🔴 (`ConfirmNotifier`), el diálogo 🔴 y todo lo que toque `Screens.kt`.

---

### Task 1: Aviso propio para la pregunta del agente y descripciones de canal (C1, C27 en parte)

Con el móvil en el bolsillo, la pregunta del agente solo cambia el titular de una notificación ongoing de importancia baja con `setOnlyAlertOnce`: no suena ni sale en pantalla y la pregunta caduca sin que nadie se entere. Se añade un aviso aparte, en un canal de importancia alta, genérico y sin botones: aparece cuando hay una pregunta nueva y la app NO está delante, y se retira cuando ya no hay ninguna (o cuando la app pasa a primer plano). Los canales ganan descripción.

**Files:**
- Create: `notify/AskNotice.kt`
- Modify: `notify/ConversationNotifier.kt` (`ask(ctx)` y `cancelAsk(ctx)`), `notify/ConversationView.kt` (`of(...)` usa `AskNotice.pending`), `AdbuddyApp.kt` (canal `ask`, descripciones de los cuatro canales, `CHANNEL_ASK`, y `foreground: StateFlow<Boolean>` con un `ActivityLifecycleCallbacks`), `bridge/BridgeService.kt` (el colector nuevo y `cancelAsk` junto a cada `ConfirmNotifier.clear(this)` de `onCreate` y de `onDestroy`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/notify/AskNoticeTest.kt` (Robolectric: el planificador y el aviso construido), `T/notify/AppForegroundTest.kt`

**Interfaces:**
- Produces (en `AdbuddyApp`): `val foreground: StateFlow<Boolean>`: verdadero mientras haya al menos una actividad de la app iniciada (`onActivityStarted`/`onActivityStopped` cuentan; `registerActivityLifecycleCallbacks` en `onCreate`); la burbuja (`ChatBubbleActivity`) cuenta como app delante.
- Produces: `object AskNotice { fun pending(s: BridgeState): Set<String>; fun apply(ctx: Context, announced: Set<String>, s: BridgeState, foreground: Boolean): Set<String> }`:
  - `pending(s)` = los ids de `s.chat.permissions` si `s.connection == CONNECTED`, si no, vacío. (No es la misma condición que `ConversationView.asking`, que además exige no estar en pausa ni con el acceso cortado: con la sesión en pausa o el acceso cortado el aviso SÍ sale, porque la pregunta caduca igual.)
  - `apply(...)`: calcula `want = if (foreground) emptySet() else pending(s)`; si `want` está vacío y `announced` no lo estaba, llama a `ConversationNotifier.cancelAsk(ctx)`; si `want` tiene algún id que no está en `announced`, llama a `ConversationNotifier.ask(ctx)`; devuelve `want` (el conjunto ya avisado). Efecto: una segunda pregunta mientras sigue la primera vuelve a avisar; abrir la app retira el aviso; y volver a segundo plano con la pregunta aún pendiente avisa otra vez.
- Produces: `ConversationNotifier.ask(ctx: Context)`, `ConversationNotifier.cancelAsk(ctx: Context)`, `const val ASK_ID = 4` (el `DONE_ID` es 3), `AdbuddyApp.CHANNEL_ASK = "ask"`.
- Produces las cadenas (tabla; los `channel_*_description` van justo después de `channel_confirm` y los `notif_ask_*` justo después de `notif_asking`, en el mismo orden en los dos ficheros):

| clave | en | es |
|---|---|---|
| `channel_ask` | `Agent questions` | `Preguntas del agente` |
| `channel_ask_description` | `Tells you when the agent asks your permission, so you can answer in time.` | `Te avisa cuando el agente pide tu permiso, para que puedas contestar a tiempo.` |
| `channel_conversation_description` | `The ongoing notification with the conversation and its buttons.` | `La notificación fija con la conversación y sus botones.` |
| `channel_done_description` | `Tells you when the agent has finished.` | `Te avisa cuando el agente ha terminado.` |
| `channel_confirm_description` | `Questions about risky commands, with Allow and Deny.` | `Preguntas sobre comandos delicados, con Permitir y Rechazar.` |
| `notif_ask_title` | `The agent needs an answer` | `El agente necesita una respuesta` |
| `notif_ask_text` | `Open ADBuddy to answer.` | `Abre ADBuddy para contestar.` |

- [ ] **Step 1: Tests que fallan**

`T/notify/AskNoticeTest.kt` (Robolectric, `RobolectricTestRunner`; mismo apoyo que `ConfirmNotificationTest`: `nm.activeNotifications`). Con `PermissionRequest` como en `PermissionCardScrollTest` (herramienta `"Bash"`, un detalle secreto `"SECRET-DETAIL"`):
1. `pending`: conectado con una pregunta → su id; sin preguntas → vacío; con preguntas y `connection = WAITING` → vacío; con la sesión en pausa (`sessionState.paused = true`) y conectado → su id (documenta la diferencia con `ConversationView.asking`).
2. `apply` con `foreground = false`, una pregunta nueva: queda UNA notificación con `id == ASK_ID` en el canal `CHANNEL_ASK`, título `notif_ask_title`, texto `notif_ask_text`, SIN acciones (`actions` nulo o vacío), con `FLAG_LOCAL_ONLY`, con `contentIntent` que abre `MainActivity` sin `EXTRA_CONFIRM`, y `publicVersion` no nula cuyo título y texto son genéricos; ninguno de `EXTRA_TITLE`, `EXTRA_TEXT`, `EXTRA_BIG_TEXT`, `EXTRA_SUB_TEXT`, `tickerText` ni los de `publicVersion` contiene `"Bash"` ni `"SECRET-DETAIL"` ni el resumen de la pregunta. Devuelve el conjunto con el id.
3. `apply` con `foreground = true`: no publica (y retira un aviso previo); vuelve a `foreground = false` con la pregunta aún pendiente: publica de nuevo.
4. `apply` sin preguntas tras haber avisado: `cancelAsk` retira la notificación; devuelve vacío. Una segunda pregunta con otro id mientras la primera sigue: vuelve a llamar a `ask` (la notificación se republica; comprueba que `ask` se invoca contando con `shadowOf(nm).size()` o con una segunda llamada observable: la notificación sigue una sola).
5. El canal `ask` existe con `IMPORTANCE_HIGH` y descripción `channel_ask_description`; los otros tres canales tienen su descripción (`nm.getNotificationChannel(id).description`). `ConfirmNotifier.clear` no retira el aviso `ASK_ID` (ni al revés).
6. `BridgeService` retira un aviso huérfano: en un test del servicio (mira cómo `T/` crea `BridgeService`; si no hay un test de servicio, comprueba por lectura y deja la responsabilidad en el informe) o, como mínimo, el test de que `cancelAsk` es idempotente sobre un `ASK_ID` ya publicado por un proceso anterior (`nm.notify(ASK_ID, ...)` a mano y luego `cancelAsk`).
`T/notify/AppForegroundTest.kt` (Robolectric): `app.foreground` es falso al empezar; al arrancar y parar una actividad de la app (`Robolectric.buildActivity(MainActivity::class.java).create().start()`, luego `.stop()`) pasa a verdadero y de nuevo a falso; con dos actividades iniciadas (`MainActivity` y `ChatBubbleActivity`), sigue verdadero hasta parar las dos.

- [ ] **Step 2: Ejecutar: fallan** (`AskNotice`, `ASK_ID`, `CHANNEL_ASK`, `foreground`, cadenas sin definir): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/android && gradle :app:testDebugUnitTest --tests '*AskNotice*' --tests '*AppForeground*'`.

- [ ] **Step 3: Implementar**

`notify/AskNotice.kt`:

```kotlin
package tech.hybriq.adbuddy.notify

import android.content.Context
import tech.hybriq.adbuddy.bridge.BridgeState

/**
 * When the "the agent needs an answer" notice is posted and withdrawn. It is for a question the person has not seen:
 * with the app in front the card is already there, so the notice is withdrawn (and posted again if the app goes to
 * the background with the question still pending). A question that comes while another is still open posts it again.
 */
object AskNotice {
    /** The ids of the agent's pending permission questions; none when the host is not connected (a lost link answers nothing). */
    fun pending(s: BridgeState): Set<String> =
        if (s.connection == BridgeState.Connection.CONNECTED) s.chat.permissions.map { it.id }.toSet() else emptySet()

    /** Applies what the state asks for and returns the ids announced now (the next call's [announced]). */
    fun apply(ctx: Context, announced: Set<String>, s: BridgeState, foreground: Boolean): Set<String> {
        val want = if (foreground) emptySet() else pending(s)
        when {
            want.isEmpty() -> if (announced.isNotEmpty()) ConversationNotifier.cancelAsk(ctx)
            want.any { it !in announced } -> ConversationNotifier.ask(ctx)
        }
        return want
    }
}
```

`ConversationView.of`: `asking = AskNotice.pending(s).isNotEmpty() && status.status == LinkStatus.CONNECTED` (conserva la condición de `status`, que anula la pregunta con el acceso cortado o en pausa: el titular de la notificación fija no cambia).

`AdbuddyApp.kt`: `const val CHANNEL_ASK = "ask"`; en `createNotificationChannels()`, los cuatro canales existentes con `description = getString(R.string.channel_..._description)` (`NotificationChannel(...).apply { description = ...; setAllowBubbles(true) }` donde ya había) y el nuevo `NotificationChannel(CHANNEL_ASK, getString(R.string.channel_ask), NotificationManager.IMPORTANCE_HIGH).apply { description = getString(R.string.channel_ask_description); setShowBadge(false) }`. `foreground`:

```kotlin
private val started = AtomicInteger(0)
private val _foreground = MutableStateFlow(false)
val foreground: StateFlow<Boolean> = _foreground.asStateFlow()
// in onCreate():
registerActivityLifecycleCallbacks(object : ActivityLifecycleCallbacks {
    override fun onActivityStarted(a: Activity) { started.incrementAndGet(); _foreground.value = true }
    override fun onActivityStopped(a: Activity) { _foreground.value = started.decrementAndGet() > 0 }
    // the rest: empty
})
```

(`ActivityLifecycleCallbacks` pide implementar todos sus métodos: `Application.ActivityLifecycleCallbacks` con los vacíos.)

`ConversationNotifier`:

```kotlin
const val ASK_ID = 4

/**
 * "The agent needs an answer": a generic heads-up so the question is not missed with the phone in a pocket. It never
 * carries the tool, the command or the detail, and it has no buttons: it is answered in the app, on the card with its guard.
 */
fun ask(ctx: Context) {
    if (!NotificationManagerCompat.from(ctx).areNotificationsEnabled()) return
    val open = PendingIntent.getActivity(ctx, 0, Intent(ctx, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
    val public = NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_ASK)
        .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
        .setContentTitle(ctx.getString(R.string.app_name))
        .setContentText(ctx.getString(R.string.notif_ask_title))
        .build()
    val n = NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_ASK)
        .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
        .setContentTitle(ctx.getString(R.string.notif_ask_title))
        .setContentText(ctx.getString(R.string.notif_ask_text))
        .setCategory(NotificationCompat.CATEGORY_MESSAGE)
        .setPriority(NotificationCompat.PRIORITY_HIGH)
        .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
        .setPublicVersion(public)
        .setLocalOnly(true)
        .setAutoCancel(true)
        .setContentIntent(open)
        .build()
    ctx.getSystemService(NotificationManager::class.java).notify(ASK_ID, n)
}

fun cancelAsk(ctx: Context) = ctx.getSystemService(NotificationManager::class.java).cancel(ASK_ID)
```

`BridgeService`: junto a los demás colectores,

```kotlin
lifecycleScope.launch {
    var announced = emptySet<String>()
    combine(app.bridge.state, app.foreground) { s, fg -> s to fg }.collect { (s, fg) ->
        announced = AskNotice.apply(this@BridgeService, announced, s, fg)
    }
}
```

y `ConversationNotifier.cancelAsk(this)` justo después de cada `ConfirmNotifier.clear(this)` (en `onCreate`, para el aviso huérfano de un proceso anterior, y en `onDestroy`). `Notify.shouldAnnounceDone` ya suprime el "Done" mientras hay pregunta: no se toca. Nota: el aviso no se repinta al cambiar de idioma (`onConfigurationChanged` repinta las otras dos): se apunta en el fragmento.

- [ ] **Step 4: Tests, paridad y commit**

Run: `gradle :app:testDebugUnitTest --tests '*AskNotice*' --tests '*AppForeground*' --tests '*ConversationViewTest' --tests '*ConfirmNotificationTest' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` → PASS; ninguna imagen cambia.

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): aviso propio y generico cuando el agente pide permiso y la app no esta delante, y descripcion de los canales

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Banner de "notificaciones bloqueadas" (C3)

Sin permiso de notificaciones, o con el canal de las confirmaciones o el de las preguntas del agente silenciado, las preguntas solo se ven con la app abierta y el agente espera hasta que caducan. El chat lo dice, con un atajo a los ajustes.

**Files:**
- Create: `notify/NotificationsStatus.kt`, `ui/NotificationsBanner.kt`
- Modify: `ui/ChatScreen.kt` (colocar el banner), `ui/ChatRoot.kt` (calcular el estado y refrescarlo al volver a la app; importar `tech.hybriq.adbuddy.notify.NotificationsStatus` y `tech.hybriq.adbuddy.notify.openNotificationSettings`, `androidx.lifecycle.compose.LifecycleResumeEffect`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/notify/NotificationsStatusTest.kt` (Robolectric), `T/screens/NotificationsBannerScreensTest.kt` (con imágenes); ampliar `T/screens/ChatScreensTest.kt` (el banner en el chat, sin imagen nueva)
- Regrabar (nuevas): `notifications_banner_*` en los cuatro modos

**Interfaces:**
- Produces: `object NotificationsStatus { data class State(val appEnabled: Boolean, val channelBlocked: Boolean) { val blocked: Boolean get() = !appEnabled || channelBlocked }; fun of(ctx: Context): State }`: `appEnabled = NotificationManagerCompat.from(ctx).areNotificationsEnabled()`; `channelBlocked` si alguno de los canales `AdbuddyApp.CHANNEL_CONFIRM` y `AdbuddyApp.CHANNEL_ASK` existe con `importance == IMPORTANCE_NONE` (un canal inexistente no está bloqueado).
- Produces: `fun openNotificationSettings(ctx: Context)` en `notify/NotificationsStatus.kt`: `Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, ctx.packageName).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)` con `startActivity` dentro de `try` (si no hay actividad, los ajustes generales `Settings.ACTION_SETTINGS`).
- Produces: `@Composable fun NotificationsBanner(onOpenSettings: () -> Unit, modifier: Modifier = Modifier)`: un `Surface` de `errorContainer` con el texto `notifications_blocked` y un `TextButton` `notifications_open_settings`; el texto lleva `liveRegion = Polite` y el botón sigue siendo un nodo accionable aparte (no `mergeDescendants` en el `Surface`).
- Modifies: `ChatScreen(..., hostNow, configured, terminalOpenByDefault, notificationsBlocked: Boolean = false, onOpenNotificationSettings: () -> Unit = {}, remoteCard)` (los dos parámetros nuevos con valor por defecto antes de `remoteCard`, que sigue siendo la lambda final; `ChatRoot` los pasa con nombre).
- Produces las cadenas: `notifications_blocked` ("ADBuddy's notifications are off or silenced: you will not be told when the agent asks you something. Turn them on in the system settings." / "Las notificaciones de ADBuddy están desactivadas o silenciadas: no se te avisará cuando el agente te pregunte algo. Actívalas en los ajustes del sistema.", con `\'` en el XML inglés) y `notifications_open_settings` ("Settings" / "Ajustes"), junto a `status_*` o donde queden con las demás del chat, mismo lugar en los dos ficheros.

- [ ] **Step 1: Tests que fallan**

`T/notify/NotificationsStatusTest.kt` (Robolectric; `ShadowNotificationManager`): con las notificaciones activadas (por defecto) y los canales con su importancia normal: `blocked == false`; con `shadowOf(nm).setNotificationsEnabled(false)`: `appEnabled == false` y `blocked == true`; con el canal `confirm` bajado a `IMPORTANCE_NONE` (bajar la importancia con `nm.createNotificationChannel(NotificationChannel(CHANNEL_CONFIRM, "x", IMPORTANCE_NONE))` —Android permite bajarla—, y comprobarla con `getNotificationChannel(...).importance` ANTES de llamar a `of`; no borrar y recrear, que Robolectric restaura los ajustes del canal borrado): `channelBlocked == true`; lo mismo con el canal `ask`; con el canal borrado (`deleteNotificationChannel`, y sin `AdbuddyApp.onCreate` que lo recree dentro del test): no bloqueado.
`T/screens/NotificationsBannerScreensTest.kt` (parametrizado por `ScreenMode`, andamiaje de `SettingsScreensTest`): `NotificationsBanner(onOpenSettings = { opened++ })`: el texto y el botón visibles, `assertTextsFit()`, `LiveRegion == Polite` en el texto, pulsar el botón llama a `onOpenSettings`, `shot("notifications_banner", mode)`.
En `ChatScreensTest`: `ChatScreen(..., notificationsBlocked = true)` muestra `notifications_blocked`; con `false`, no (sin imagen).

- [ ] **Step 2: Ejecutar: fallan** (`NotificationsStatus`, `NotificationsBanner` sin definir): `gradle :app:testDebugUnitTest --tests '*NotificationsStatusTest' --tests '*NotificationsBanner*' --tests '*ChatScreensTest'`.

- [ ] **Step 3: Implementar** `NotificationsStatus`, `openNotificationSettings`, `NotificationsBanner` como arriba. En `ChatScreen`, dentro del `Column` del cuerpo, tras el `LinkBanner`: `if (notificationsBlocked) NotificationsBanner(onOpenNotificationSettings)`. En `ChatRoot`: `var notifications by remember { mutableStateOf(NotificationsStatus.of(context)) }` y `LifecycleResumeEffect(Unit) { notifications = NotificationsStatus.of(context); onPauseOrDispose {} }` para releerlo al volver de los ajustes del sistema; `ChatScreen(..., notificationsBlocked = notifications.blocked, onOpenNotificationSettings = { openNotificationSettings(context) }, ...)`.
- [ ] **Step 4: Ejecutar, regrabar, mirar, commit**

Run: `gradle :app:testDebugUnitTest --tests '*NotificationsStatusTest' --tests '*NotificationsBanner*' --tests '*ChatScreensTest' --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'` (los flujos pasan por `ChatRoot`) → PASS tras grabar las nuevas (`-Padbuddy.screens=record --tests '*NotificationsBannerScreensTest'`); solo se añaden los 4 `notifications_banner_*`; mirar `notifications_banner_light.png` y `notifications_banner_font_200_es.png` (texto completo, botón visible). Ninguna imagen existente cambia (los tests del chat no bloquean las notificaciones).

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): banner en el chat cuando las notificaciones estan bloqueadas, con atajo a los ajustes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: El permiso de notificaciones, con su motivo y en su momento (O4)

`MainActivity.onCreate` pide `POST_NOTIFICATIONS` en frío, a la vez que se muestra el consentimiento y sin explicación. Pasa a pedirse desde el chat, una sola vez, tras un diálogo que dice para qué sirve, y solo cuando no hay pasos de primer arranque en pantalla, ni una pregunta pendiente, ni estamos en la burbuja.

**Files:**
- Create: `notify/NotificationPermission.kt` (decisión pura), `ui/NotificationsRationaleDialog.kt`
- Modify: `MainActivity.kt` (quitar el `requestPermissions` de `onCreate`, líneas ~31-37, y los imports que queden sin uso), `ChatBubbleActivity.kt` (pasa `askForNotifications = false`), `ui/ChatRoot.kt` (el diálogo y el lanzador del permiso; parámetro nuevo), `bridge/BridgePrefs.kt` (`var notificationsAsked: Boolean`), `res/values/strings.xml`, `res/values-es/strings.xml`
- Test (nuevo): `T/notify/NotificationPermissionTest.kt` (JUnit simple), `T/screens/NotificationsRationaleScreensTest.kt` (con imágenes, sobre el contenido); modificar `T/flows/OnboardingFlowTest.kt`, `T/flows/BubbleFlowTest.kt`, `T/flows/ProtectedAppsFlowTest.kt`
- Regrabar (nuevas): `notifications_rationale_*` en los cuatro modos

**Interfaces:**
- Produces: `object NotificationPermission { fun shouldAsk(sdk: Int, granted: Boolean, alreadyAsked: Boolean, stepShown: Boolean, questionPending: Boolean): Boolean }` = `sdk >= 33 && !granted && !alreadyAsked && !stepShown && !questionPending`.
- Produces: `BridgePrefs.notificationsAsked` (Boolean, por defecto `false`; se guarda con el mismo `save(...)` privado que las otras propiedades de la clase, clave `"notifications_asked"`).
- Produces: `@Composable fun NotificationsRationaleContent(modifier: Modifier = Modifier)` (el título y el cuerpo, para poder pintarlos y medirlos sin ventana de diálogo) y `@Composable fun NotificationsRationaleDialog(onContinue: () -> Unit, onSkip: () -> Unit)`: un `AlertDialog` cuyo título es `notifications_rationale_title`, cuyo cuerpo es el `NotificationsRationaleContent`, con botón de confirmar `notifications_rationale_continue` y de rechazar `onboarding_skip` ("Ahora no"); `onDismissRequest = onSkip` (Atrás equivale a "Ahora no") y sin cierre al tocar fuera (`DialogProperties(dismissOnClickOutside = false)`).
- Modifies: `ChatRoot(app, focusConfirm, closed, firstRun = true, terminalOpenByDefault = true, askForNotifications: Boolean = true, onClose)` (parámetro nuevo con valor por defecto, antes de `onClose`, que sigue siendo la lambda final); `ChatBubbleActivity` llama con `askForNotifications = false` (una burbuja de 600 dp con un diálogo estorba, y `BubbleFlowTest` comprueba que la burbuja no enseña el "Ahora no" de los pasos).
- Produces las cadenas: `notifications_rationale_title` ("Get notified?" / "¿Recibir avisos?"), `notifications_rationale_body` ("ADBuddy can tell you when the agent asks your permission or finishes. Without notifications you would only see its questions with the app open, and they expire." / "ADBuddy puede avisarte cuando el agente te pide permiso o termina. Sin notificaciones solo verías sus preguntas con la app abierta, y caducan."), `notifications_rationale_continue` ("Continue" / "Continuar").

- [ ] **Step 1: Tests que fallan**

`NotificationPermissionTest`: `shouldAsk(34, granted = false, alreadyAsked = false, stepShown = false, questionPending = false)` es true; falso con `sdk = 32`, con el permiso concedido, ya preguntado, con un paso de primer arranque en pantalla y con una pregunta pendiente (cada uno por separado).
`NotificationsRationaleScreensTest` (parametrizado por `ScreenMode`): sobre `NotificationsRationaleContent` (un diálogo se pinta en su propia ventana y `shot()`/`assertTextsFit()` usan `onRoot()`/`isRoot()`, que con dos raíces fallan): título y cuerpo visibles, `assertTextsFit()`, `shot("notifications_rationale", mode)`. Un test aparte, sin imagen, del `NotificationsRationaleDialog` completo con `onNode(isDialog())`: los dos botones existen, "Continuar" llama a `onContinue`, "Ahora no" a `onSkip`, y Atrás (`Espresso.pressBack()` o `onDismissRequest` por semántica: `compose.runOnUiThread { activity.onBackPressedDispatcher.onBackPressed() }`) llama a `onSkip`.
Flujos (los que llegan al chat tras los pasos y que `grep -rn "action_menu" android/app/src/test` devuelve: `OnboardingFlowTest`, `BubbleFlowTest`, `ProtectedAppsFlowTest`; `ConsentFeedbackFlowTest` no pasa por el chat y no se toca): `createAndroidComposeRule` lanza la actividad ANTES de cualquier `@Before`, así que el permiso se concede en el `init { ... }` de cada clase (como ya hace `OnboardingFlowTest` con su estado previo): `shadowOf(RuntimeEnvironment.getApplication()).grantPermissions(Manifest.permission.POST_NOTIFICATIONS)`. Tests nuevos:
- `OnboardingFlowTest` (en una clase aparte si hace falta no conceder el permiso en su `init`): con el permiso denegado (`denyPermissions`) y `notificationsAsked = false`, el diálogo NO aparece mientras el paso 1 o el 2 siguen en pantalla; tras terminarlos aparece `notifications_rationale_title`; "Ahora no" lo cierra, deja `BridgePrefs(app).notificationsAsked == true` y no vuelve a salir al recomponer ni al recrear la actividad (`compose.activityRule.scenario.recreate()`); y con una pregunta 🔴 pendiente (un estado del puente con `pending` o con un permiso del agente) el diálogo NO aparece.
- `BubbleFlowTest`: `notifications_rationale_title` no existe en la burbuja (con el permiso denegado y sin preguntar).

- [ ] **Step 2: Ejecutar: fallan** (`NotificationPermission`, `notificationsAsked`, `NotificationsRationale*` sin definir): `gradle :app:testDebugUnitTest --tests '*NotificationPermissionTest' --tests '*NotificationsRationale*' --tests '*OnboardingFlowTest' --tests '*BubbleFlowTest'`.

- [ ] **Step 3: Implementar**

`BridgePrefs`: `var notificationsAsked: Boolean get() = prefs.getBoolean("notifications_asked", false); set(v) = save("notifications_asked") { putBoolean("notifications_asked", v) }` (imitando el formato de las otras propiedades del fichero).

`NotificationPermission` y `NotificationsRationaleDialog` como arriba. `MainActivity.kt`: borrar el bloque `if (Build.VERSION.SDK_INT >= TIRAMISU && checkSelfPermission(...) != GRANTED) { requestPermissions(...) }` y los imports que queden sin uso. `ui/ChatRoot.kt` (imports: `android.Manifest`, `android.content.pm.PackageManager`, `android.os.Build`, `androidx.core.content.ContextCompat`, `tech.hybriq.adbuddy.bridge.BridgePrefs`, `tech.hybriq.adbuddy.notify.NotificationPermission`, `androidx.activity.result.contract.ActivityResultContracts` si faltan):

```kotlin
val prefs = remember(context) { BridgePrefs(context) }
var asked by remember { mutableStateOf(prefs.notificationsAsked) }
val requestPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { notifications = NotificationsStatus.of(context) }
// Composed BEFORE ConfirmHost and never with a question open: the reason must not sit over a decision.
val granted = ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
val questionPending = state.pending.isNotEmpty() || state.chat.permissions.isNotEmpty()
if (askForNotifications && route == Route.CHAT && NotificationPermission.shouldAsk(Build.VERSION.SDK_INT, granted, asked, stepShown = step != null, questionPending = questionPending)) {
    NotificationsRationaleDialog(
        onContinue = { prefs.notificationsAsked = true; asked = true; requestPermission.launch(Manifest.permission.POST_NOTIFICATIONS) },
        onSkip = { prefs.notificationsAsked = true; asked = true },
    )
}
// ... then ConfirmHost(...), as now.
```

(El diálogo va justo antes de `ConfirmHost(state, ...)`, que sigue siendo lo último de `ChatRoot`. `ChatRoot` solo se compone sin la pantalla de consentimiento —`MainActivity` pinta una u otra—, por eso no hay parámetro de consentimiento. `POST_NOTIFICATIONS` solo existe desde la API 33: `shouldAsk` exige `sdk >= 33` y el lanzador solo se invoca entonces; si `Manifest.permission.POST_NOTIFICATIONS` da aviso de lint por `minSdk`, protege la llamada con `Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU` o `@RequiresApi`.)

- [ ] **Step 4: Ejecutar los tests de pantalla y de flujo; regrabar solo lo nuevo**

Run: `gradle :app:testDebugUnitTest --tests '*NotificationPermissionTest' --tests '*NotificationsRationale*' --tests '*Flow*' --tests '*StringsParityTest' --tests '*NoHardcodedTextTest'`; grabar `notifications_rationale_*`; `git status`: solo imágenes nuevas. Mirar `notifications_rationale_light.png` y `notifications_rationale_font_200_es.png`.

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add android && git commit -m "feat(app): el permiso de notificaciones se pide una vez, con su motivo y fuera de la pantalla de configuracion

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Cierre del paquete

**Files:**
- Modify: `docs/superpowers/fragmentos/ux-revision.md` (nueva sección "P4")

- [ ] **Step 1: Suite completa**: `gradle :app:testDebugUnitTest :app:lintDebug` (desde `android/`) → PASS entero.
- [ ] **Step 2: Fragmento**: sección "P4 · notificaciones": qué cambió por tarea; imágenes nuevas (`notifications_banner_*`, `notifications_rationale_*`) y que no cambia ninguna existente; qué cierra y qué deja desfasado, SIN tocar esos ficheros: cierra el pendiente de `pendientes-m3-m5.md` (línea ~325: canal alto para el aviso) y deja desfasada la spec de M4 (`docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md` ~líneas 125-127: «el aviso es silencioso en M4…»), para que el controlador de integración las actualice; aplazado con motivo (`CATEGORY_CALL`, visibilidad de la 🔴 en pantalla bloqueada y botón Rechazar en el bloqueo: van con P5, opus; la fila de «notificaciones» en Ajustes que pedía O4: el banner del chat ya cubre el aviso permanente y la fila pediría un estado más en Ajustes; los iconos del sistema y de lanzador; el `LinkDialog` que no sobrevive a la rotación; el botón del terminal con ↑/↓); notas honestas (el aviso de la pregunta del agente es genérico y sin botones; con la sesión en pausa o el acceso cortado SÍ sale, porque la pregunta caduca igual; una segunda pregunta mientras sigue la primera avisa otra vez, y una reconexión con la pregunta aún pendiente también; no se repinta al cambiar de idioma; el permiso se pide una sola vez y el banner es el recordatorio permanente; sin diálogo en la burbuja); y las validaciones manuales pendientes (el aviso con la app en segundo plano y la pantalla apagada, que suene y salga en pantalla sin mostrar nada de la pregunta; el sistema de permisos real tras el diálogo; el banner al desactivar las notificaciones desde los ajustes y volver).
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P4 de UX`, con el trailer exacto).

## Validaciones manuales pendientes (Pixel)

Solo para el fragmento; ninguna se pide al usuario ahora: el aviso de la pregunta del agente con la app en segundo plano y la pantalla apagada (suena y sale en pantalla, sin mostrar nada de la pregunta), el diálogo del sistema tras el motivo (Android 13+), y el banner al desactivar las notificaciones en los ajustes y volver. Lo demás está cubierto por tests de semántica, de la notificación construida y por las imágenes en la JVM.
