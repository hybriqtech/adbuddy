# ADBuddy M3b · La app del chat · Plan de implementación

> **Para agentes:** SUB-HABILIDAD OBLIGATORIA: usa `superpowers:subagent-driven-development` (recomendado) o `superpowers:executing-plans` para ejecutar este plan tarea a tarea. Los pasos usan casillas (`- [ ]`).

**Objetivo:** la app Android de M3: base de diseño, pantalla de chat con terminal (xterm.js) y teclas rápidas, adjuntos, descargas y captura, y la notificación de conversación con burbuja y aviso "Listo"; más, en el host, `session.state.activity`.

**Arquitectura:** el código nuevo se parte en piezas puras sin Android (modelos, máquinas de estado, controladores sobre una interfaz `Channel`) que se prueban con tests JVM contra un `FakeChannel`, y una capa fina de Compose, WebView y notificaciones encima. El protocolo no cambia: la app usa los tipos de `bridge/Protocol*.kt` tal cual. Cada reductor y cada controlador va en su propio fichero.

**Tecnologías:** Kotlin 2.4, Jetpack Compose (BOM 2026.09) con Material 3, OkHttp (ya está), kotlinx.serialization, `androidx.webkit` (`WebViewAssetLoader`), xterm.js 5.5.0 + addon-fit 0.10.0 + addon-web-links 0.11.0 (MIT, incluidos en los assets). Host: Go.

**Spec:** `docs/superpowers/specs/2026-10-01-adbuddy-m3-chat-design.md` (§3.5, §5, §6.4, §7, §9-§11). Protocolo: `docs/superpowers/specs/2026-10-01-adbuddy-protocolo-m3-m5-design.md` (§3-§7, §9). Contrato real del host (M3a): `.superpowers/sdd/2026-10-01-adbuddy-m3a/handoff-m3b.md`.

## Restricciones globales

- **Tests:** `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest` (no hay wrapper; es el Gradle 9.8.0 de este PC; la CI usa `gradle` 9.8.0). Host: `cd host && go test ./internal/cli -timeout 5m` (un paquete cada vez, siempre con `-timeout`). **`-race` no corre en este PC:** un valor sustituible por los tests se lee una vez antes de lanzar la gorrutina y se sube pronto a la CI lo que tenga concurrencia.
- **Cada tarea compila y pasa sus tests por sí sola** (`:app:testDebugUnitTest` y `:app:assembleDebug` en las de Android). Ninguna usa clases de una tarea posterior.
- **Ningún reductor ni controlador lanza una excepción, con ningún payload** (pendientes de M3.0): un fallo dentro de `_state.updateAndGet` corta el socket y la app entra en un bucle de reconexión. Los decodificadores usan `Protocol.payload<T>(m) ?: return …`.
- **Texto de la persona y de la terminal: nunca a ningún log** (spec §10). Sin `Log.*` con el texto del chat, del agente ni de los ficheros.
- **Colores:** todos de `MaterialTheme.colorScheme`; ninguno fijo en pantallas (solo en `ui/theme/Palette.kt`).
- **Textos:** cada cadena nueva va en `res/values/strings.xml` (inglés) **y** en `res/values-es/strings.xml`. Los textos del host (`notice`, `error.text`) se muestran tal cual.
- **El WebView no sale a la red** y su puente JS tiene tres métodos (spec §7.3). OSC 52 no se activa.
- **Android mínimo 30** (`minSdk`), `compileSdk` 37. Burbujas (Bubbles API) desde Android 11.
- **Ficheros compartidos con otros agentes** (`COORDINACION.md`): `android/app/build.gradle.kts` (bloque de dependencias: de este plan), `AndroidManifest.xml` (nautilus-19 añade los suyos: commits pequeños y separados, **y apuntar el cambio en `COORDINACION.md` antes de tocarlo**), `BridgeService.kt`, `Bridge.kt`, `ChatReducer.kt`, `phone.go` (de este plan), `pendientes-m3-m5.md` (solo la sección M3). `protocol/` no se toca.
- **Commits:** identidad local `hybriqtech`, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, sin `--force`; nada de push sin el ok del supervisor (`adbuddy-e5`).
- **Proceso (CLAUDE.md):** revisión del plan con `adbuddy-reviewer-opus` antes de la tarea 1; revisores Sonnet por tarea (`adbuddy-reviewer`; `adbuddy-reviewer-opus` solo en T1 si el supervisor lo permite y en la revisión final); `adbuddy-rereviewer` para diffs pequeños; 5 rondas por tarea como máximo; un subagente a la vez.

## Decisiones de este plan

- **Ruling: `session.state.activity` es la tarea 1, en el host** (`phoneLink.Entry`: la última orden de adb con `Source == agent`) — la spec §3.5 lo pide y la notificación "Trabajando: …" lo necesita; en el host ya llega cada entrada del diario — si me equivoco, la notificación solo dice "Trabajando".
- **Ruling: la app abstrae el canal con `interface Channel`** (`Bridge` se adapta con `BridgeChannel`, sin tocar `Bridge.kt`) — cada controlador (chat, subidas, descargas, terminal, captura) se prueba con `FakeChannel` sin red ni Android — si me equivoco, una capa de adaptador de más.
- **Ruling: los mensajes no enviados viven en un `ChatController` local, no en `ChatReducer`** — el reductor solo refleja lo que dice el host (puro, sin estado local que `reset` tenga que decidir si olvida); la lista que ve la persona es `Timeline.of(host, outgoing)` — si me equivoco, el reductor gana un campo.
- **Ruling: la conversación se filtra en `Timeline`**, no en el reductor: se muestran los mensajes cuya `conversation` es la de `session.state` (o vacía) — el host no publica nada que vacíe el chat al hacer `session.new` — si me equivoco, un mensaje viejo se vería un instante.
- **Ruling: el tamaño de la terminal lo manda el host.** xterm.js se redimensiona solo con `termResize` (de `terminal.snapshot` y `terminal.size`); el addon-fit solo propone dimensiones, que van como `terminal.resize` (el host las ignora con `pcAttached`). La vista hace scroll horizontal si el tamaño del host no cabe — si me equivoco, la TUI se ve más pequeña de lo que cabría.
- **Ruling: la etiqueta de "red local o Tailscale" de la cabecera queda para M5b** (necesita `RemoteState`); M3b muestra la línea de `LinkStatus` — si me equivoco, una línea en la cabecera.
- **Ruling: el límite de descarga automática en modo remoto (5 MiB) usa `state.remote.enabled`;** hoy `Downloads.autoLimit` recibe `state.remote.enabled`, que ya existe en `RemoteState` — si me equivoco, el límite de 5 MiB se aplica o no según lo que M5 ponga en `enabled`.
- **Ruling: sin dependencia de iconos ampliados** (`material-icons-extended` pesa mucho): solo iconos de `material-icons-core` o `ImageVector` propios.
- **Riesgo anotado:** Android puede no abrir como burbuja una notificación de servicio en primer plano (`ongoing`). Se implementa como pide la spec (§7.4) y se valida en el Pixel; si no abre, la burbuja sale de una notificación de conversación aparte (id 4) que el servicio publica con cada mensaje nuevo. Va en "Validaciones manuales".

## Mapa de ficheros

Host (T1):
- `host/internal/cli/phone.go` (modifica): campo `activity`, `noteActivity`, `state()`, `sessionInfo`, `Entry`.
- `host/internal/cli/phone_chat_test.go` (modifica): tests de `activity`.

Android, bajo `android/app/src/main/kotlin/tech/hybriq/adbuddy/` (`T` = tarea):
- `bridge/ChatReducer.kt` (modifica, T2): `chat.message` y `snapshot`.
- `bridge/Channel.kt` (nuevo, T3): `Channel` y `BridgeChannel`.
- `chat/ChatController.kt`, `chat/Timeline.kt` (T3); `chat/Links.kt` (T4, `find` en T9); `chat/Composer.kt`, `chat/SessionLabel.kt` (T9).
- `term/TerminalModel.kt`, `term/TerminalFeed.kt`, `term/QuickKeys.kt`, `term/TermBridge.kt` (nuevos, T4); `term/TerminalView.kt` + `assets/term/*` (T8).
- `files/Uploader.kt`, `files/Captures.kt`, `files/Drafts.kt`, `files/FileNames.kt`, `files/UriSource.kt` (T5); `files/FileCache.kt`, `files/Downloads.kt`, `files/FileIntents.kt` (T6).
- `ui/theme/{Palette,Theme}.kt`, `res/values/themes.xml`, `ui/Screens.kt` (modifica) (T7).
- `ui/ChatScreen.kt`, `ui/ActivityScreen.kt`, `ui/ChatParts.kt`, `ChatBubbleActivity.kt`, `MainActivity.kt` (modifica) (T9, T10).
- `notify/{DoneDetector,ConversationView,ConversationNotifier}.kt`, `bridge/BridgeService.kt` (modifica) (T10).
- `AdbuddyApp.kt` (modifica en T3, T4, T5, T6, T10: cada tarea añade sus singletons).

Tests, bajo `android/app/src/test/kotlin/tech/hybriq/adbuddy/` con la misma estructura de paquetes.

---

### Tarea 1 (host): `session.state.activity`

**Ficheros:**
- Modifica: `host/internal/cli/phone.go` (struct `phoneLink` junto a `sess`, `Entry`, `state`, `sessionInfo`)
- Test: `host/internal/cli/phone_chat_test.go`

**Interfaces:**
- Consume: `journal.Entry{Source, Description}`, `journal.SourceAgent`, `journal.SourceUser`, `phoneLink.srv.Publish`, `bridge.SessionState.Activity` (ya existe).
- Produce: `session.state.activity` = la descripción (como mucho 120 runas) de la última orden del agente; vacío al cambiar de conversación o con la sesión parada. Todo `session.state` (evento, respuesta con `rid`, snapshot) lo lleva porque salen de `l.state()`.

- [ ] **Paso 1: escribe los tests que fallan** (al final de `phone_chat_test.go`; imports `encoding/json` y `internal/journal` si faltan)

```go
// nextState reads app messages until the next session.state, skipping the
// journal.entry events that Entry publishes next to it.
func nextState(t *testing.T, r *chatRig) bridge.SessionState {
	t.Helper()
	for range 20 {
		m := readApp(t, r.c)
		if m.Type != bridge.TypeSessionState {
			continue
		}
		var st bridge.SessionState
		if err := json.Unmarshal(m.Payload, &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	t.Fatal("no session.state")
	return bridge.SessionState{}
}

// The state carries what the agent last did on the phone (spec M3 §3.5):
// only the agent's commands, once per change, cut to 120 runes.
func TestSessionStateCarriesTheAgentsLastCommand(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	// A command of the person is not the agent's activity, nor one that did not run.
	r.l.Entry(journal.Entry{Source: journal.SourceUser, Description: "Reboot the phone"})
	r.l.Entry(journal.Entry{Source: journal.SourceAgent, Description: "Wipe the phone", Status: journal.StatusDenied})
	r.l.Entry(journal.Entry{Source: journal.SourceAgent, Description: "Open the Wi-Fi settings"})
	if st := nextState(t, r); st.Activity != "Open the Wi-Fi settings" {
		t.Fatalf("activity %q", st.Activity)
	}
	// The same description again publishes nothing:
	// the next state is the new command's.
	r.l.Entry(journal.Entry{Source: journal.SourceAgent, Description: "Open the Wi-Fi settings", Status: "ok"})
	r.l.Entry(journal.Entry{Source: journal.SourceAgent, Description: strings.Repeat("é", 300)})
	st := nextState(t, r)
	if n := len([]rune(st.Activity)); n != 120 || !strings.HasSuffix(st.Activity, "…") {
		t.Fatalf("activity of %d runes: %q", n, st.Activity)
	}
	// A snapshot says it too.
	if got := r.l.snapshot().State.Activity; got != st.Activity {
		t.Fatalf("snapshot %q", got)
	}
}

// A new conversation starts without activity.
func TestActivityEndsWithTheConversation(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	r.l.Entry(journal.Entry{Source: journal.SourceAgent, Description: "Take a screenshot"})
	if st := nextState(t, r); st.Activity != "Take a screenshot" {
		t.Fatalf("activity %q", st.Activity)
	}
	// What the Manager publishes when session.new starts another conversation.
	r.l.sessionInfo(session.Info{Agent: "claude", State: session.StateIdle, Conversation: "c_other"})
	if st := nextState(t, r); st.Activity != "" || st.Conversation != "c_other" {
		t.Fatalf("state %+v", st)
	}
}
```

- [ ] **Paso 2: comprueba que fallan**

Run: `cd host && go test ./internal/cli -run 'TestSessionStateCarriesTheAgentsLastCommand|TestActivityEndsWithTheConversation' -timeout 2m`
Esperado: FAIL (`activity ""`).

- [ ] **Paso 3: implementa**

En `phoneLink` (junto a `sess session.Info`):

```go
	activity string // the agent's last adb command as the journal describes it (session.state.activity)
```

Constante y método (cerca de `Entry`; `strings` ya está importado en el paquete, añádelo a `phone.go` si falta):

```go
// maxActivity is the longest activity, in runes: it travels in every
// session.state.
const maxActivity = 120

// noteActivity records what the agent last did on the phone and publishes the
// state when it changes: a repeated description publishes nothing. Commands
// that did not run (denied, blocked, timed out) are not what the agent is doing.
func (l *phoneLink) noteActivity(j journal.Entry) {
	switch j.Status {
	case journal.StatusDenied, journal.StatusBlocked, journal.StatusTimeout:
		return
	}
	desc := strings.TrimSpace(j.Description)
	if desc == "" {
		return
	}
	if r := []rune(desc); len(r) > maxActivity {
		desc = string(r[:maxActivity-1]) + "…"
	}
	l.mu.Lock()
	changed := l.activity != desc
	l.activity = desc
	l.mu.Unlock()
	if changed && l.srv != nil {
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	}
}
```

En `Entry`, antes del `if l.srv != nil` final (y fuera del lock):

```go
	if j.Source == journal.SourceAgent {
		l.noteActivity(j)
	}
```

En `state()`, el literal pasa a `bridge.SessionState{Agent: l.agent, State: session.StateStopped, Activity: l.activity, Paused: l.paused, View: "terminal"}` y, justo antes del `return st`, `if st.State == session.StateStopped { st.Activity = "" }` (una entrada del agente con la sesión parada no la rellena). Lo mismo en `stateOf`, tras copiar el estado de `info`.

En `sessionInfo`, dentro del lock y antes de `l.sess = i`:

```go
	if i.Conversation != l.sess.Conversation || i.State == session.StateStopped {
		l.activity = ""
	}
```

- [ ] **Paso 4: pasan los tests, y el resto del paquete**

Run: `cd host && go vet ./internal/cli && go test ./internal/cli -timeout 5m`
Esperado: PASS.

- [ ] **Paso 5: commit**

```bash
git add host/internal/cli/phone.go host/internal/cli/phone_chat_test.go
git commit -m "feat(host): session.state.activity lleva la última orden del agente"
```

---

### Tarea 2: `ChatReducer` (mensajes del host)

**Ficheros:**
- Modifica: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ChatReducerTest.kt`

**Interfaces:**
- Consume: `ChatMessage`, `Protocol.payload`, `Snapshot.chat: List<JsonElement>`, `BridgeState.chat: ChatState`.
- Produce: `ChatReducer.MAX_MESSAGES = 100`; `ChatState.messages` con upsert por `id` en orden de llegada, y reemplazada por la del `snapshot`. Permisos y `tool.activity` siguen sin tocarse (M4).

- [ ] **Paso 1: escribe el test**

```kotlin
package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatReducerTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private fun chat(id: String, text: String, extra: String = "") =
        """{"id":"$id","role":"user","text":"$text","conversation":"c_1"$extra}"""

    @Test fun messagesAreUpsertedById() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "one")))
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, chat("b", "two")))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, chat("a", "one, edited")))
        assertEquals(listOf("a", "b"), s.chat.messages.map { it.id })
        assertEquals("one, edited", s.chat.messages[0].text)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun onlyTheLast100AreKept() {
        var s = BridgeState()
        for (i in 1..130) s = s.apply(msg(i.toLong(), Protocol.CHAT_MESSAGE, chat("m$i", "x")))
        assertEquals(ChatReducer.MAX_MESSAGES, s.chat.messages.size)
        assertEquals("m31", s.chat.messages.first().id)
        assertEquals("m130", s.chat.messages.last().id)
    }

    @Test fun nullAttachmentsAndBadPayloadsNeverThrow() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "x", ""","attachments":null""")))
        assertTrue(s.chat.messages[0].attachments.isEmpty())
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, """{"id":5}"""))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, """{"role":"user"}"""))
        s = s.apply(msg(4, Protocol.CHAT_MESSAGE, "[]"))
        assertEquals(listOf("a"), s.chat.messages.map { it.id })
        assertEquals(4L, s.lastSeq)
    }

    @Test fun snapshotReplacesTheMessagesAndSkipsTheBadOnes() {
        var s = BridgeState(session = "s_1", lastSeq = 5)
        s = s.apply(msg(6, Protocol.CHAT_MESSAGE, chat("old", "old")))
        val snap = """{"state":{},"chat":[${chat("a", "one")},{"id":7},${chat("b", "two")},${chat("a", "one again")}]}"""
        s = s.apply(msg(7, Protocol.SNAPSHOT, snap))
        assertEquals(listOf("b", "a"), s.chat.messages.map { it.id })
        assertEquals("one again", s.chat.messages.last().text)
    }

    @Test fun aNewSessionForgetsTheChatButAReconnectKeepsIt() {
        var s = BridgeState(session = "s_1", lastSeq = 5).apply(msg(6, Protocol.CHAT_MESSAGE, chat("a", "x")))
        assertEquals(1, s.disconnected().chat.messages.size)
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertTrue(s.chat.messages.isEmpty())
    }
}
```

En `snapshotReplacesTheMessagesAndSkipsTheBadOnes` el snapshot trae `a`, `b` y otra vez `a`: gana la última versión de `a`, en el sitio de su última aparición (`b`, `a`).

- [ ] **Paso 2: comprueba que falla**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests '*ChatReducerTest'`
Esperado: FAIL (los mensajes quedan vacíos).

- [ ] **Paso 3: implementa** (`ChatReducer.kt`, sustituye `object ChatReducer`; deja `ChatState` como está)

```kotlin
/** The chat's events: M3 fills the messages; permissions and tool activity are M4's. */
object ChatReducer : Reducer {
    /** What the host keeps and sends in a snapshot (spec M3 §5.3). */
    const val MAX_MESSAGES = 100

    override val types = setOf(
        Protocol.CHAT_MESSAGE, Protocol.TOOL_ACTIVITY, Protocol.PERMISSION_REQUEST, Protocol.PERMISSION_RESOLVED,
    )

    override fun event(s: BridgeState, m: Message): BridgeState = when (m.type) {
        Protocol.CHAT_MESSAGE -> {
            val msg = Protocol.payload<ChatMessage>(m)
            if (msg == null || msg.id.isEmpty()) s else s.copy(chat = s.chat.copy(messages = upsert(s.chat.messages, msg)))
        }
        else -> s
    }

    /** One with an id already seen replaces it where it was (spec protocol §5.1); a new one goes last. */
    internal fun upsert(list: List<ChatMessage>, msg: ChatMessage): List<ChatMessage> {
        val i = list.indexOfFirst { it.id == msg.id }
        return if (i >= 0) list.toMutableList().also { it[i] = msg } else (list + msg).takeLast(MAX_MESSAGES)
    }

    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState {
        val messages = snap.chat.mapNotNull {
            try {
                Protocol.json.decodeFromJsonElement(ChatMessage.serializer(), it).takeIf { m -> m.id.isNotEmpty() }
            } catch (e: IllegalArgumentException) {
                null
            } catch (e: StackOverflowError) {
                null
            }
        }
        // A repeated id keeps its last version, at the place of its last appearance.
        val unique = messages.asReversed().distinctBy { it.id }.asReversed()
        return s.copy(chat = s.chat.copy(messages = unique.takeLast(MAX_MESSAGES)))
    }

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(chat = ChatState())
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
```


- [ ] **Paso 4: pasan los tests del reductor y todos los demás**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest`
Esperado: PASS (incluidos `FixturesTest`: los fixtures de `chat.message` siguen válidos).

- [ ] **Paso 5: commit**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ChatReducerTest.kt
git commit -m "feat(app): ChatReducer guarda los chat.message y los del snapshot"
```

---

### Tarea 3: `Channel`, `ChatController` y `Timeline`

**Ficheros:**
- Crea: `bridge/Channel.kt`, `chat/ChatController.kt`, `chat/Timeline.kt`
- Modifica: `AdbuddyApp.kt` (el singleton `chat`)
- Test: `test/.../bridge/FakeChannel.kt`, `test/.../chat/ChatControllerTest.kt`, `test/.../chat/TimelineTest.kt`

**Interfaces:**
- Consume: `Bridge` (`request`, `send`, `sendFrame`, `queuedBytes`, `listen`, `listenFrames`, `state`), `ChatSend`, `ErrorReply`, `ChatMessage`, `ChatState`, `Attachment`.
- Produce (las usan T4-T10):
  - `interface Channel { fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String?; fun send(type: String, payload: JsonElement): Boolean; fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean; fun queuedBytes(): Long; fun listen(type: String, listener: (Message) -> Unit): () -> Unit; fun listenFrames(listener: (Chunk) -> Unit): () -> Unit; val connected: StateFlow<Boolean> }`
  - `class BridgeChannel(bridge: Bridge, scope: CoroutineScope) : Channel`
  - `enum class SendStatus { SENDING, FAILED }`, `data class Outgoing(val id: String, val text: String, val attachments: List<Attachment>, val status: SendStatus)`
  - `class ChatController(channel: Channel, newId: () -> String = …)`: `outgoing: StateFlow<List<Outgoing>>`, `errors: SharedFlow<String>`, `fun send(text: String, attachments: List<Attachment> = emptyList()): String?`, `fun retry(id: String)`, `fun discard(id: String)`, `fun newConversation()` (T9).
  - `sealed interface TimelineItem { val key: String }` con `Host(msg: ChatMessage)` y `Mine(out: Outgoing)`; `object Timeline { fun of(chat: ChatState, conversation: String, outgoing: List<Outgoing>): List<TimelineItem> }`.
  - `FakeChannel` (test): ver abajo.

- [ ] **Paso 1: crea `FakeChannel` y los tests**

`test/.../bridge/FakeChannel.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonElement

/** A Channel for tests: records what the app sends, and `handler` plays the host. */
class FakeChannel : Channel {
    class Sent(val type: String, val payload: JsonElement, val rid: String?)

    val sent = mutableListOf<Sent>()
    val frames = mutableListOf<Triple<String, Long, ByteArray>>()

    /** Plays the host: gets each request and its callback; false means it could not be sent (request returns null). */
    var handler: (Sent, (Message?) -> Unit) -> Boolean = { _, _ -> true }

    /** Frames refused (sendFrame returns false) from this count on; -1: never. */
    var refuseFramesFrom = -1
    var queued = 0L
    private var next = 0
    private val _connected = MutableStateFlow(true)
    override val connected: StateFlow<Boolean> = _connected
    private val listeners = mutableMapOf<String, MutableList<(Message) -> Unit>>()
    private val frameListeners = mutableListOf<(Chunk) -> Unit>()

    fun setConnected(v: Boolean) { _connected.value = v }

    override fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String? {
        val s = Sent(type, payload, "r${++next}")
        sent += s
        return if (handler(s, onResult)) s.rid else null
    }

    override fun send(type: String, payload: JsonElement): Boolean { sent += Sent(type, payload, null); return true }

    override fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean {
        if (refuseFramesFrom >= 0 && frames.size >= refuseFramesFrom) return false
        frames += Triple(id, offset, data)
        return true
    }

    override fun queuedBytes() = queued

    override fun listen(type: String, listener: (Message) -> Unit): () -> Unit {
        listeners.getOrPut(type) { mutableListOf() } += listener
        return { listeners[type]?.remove(listener) }
    }

    override fun listenFrames(listener: (Chunk) -> Unit): () -> Unit {
        frameListeners += listener
        return { frameListeners.remove(listener) }
    }

    /** A message of the host (a stream message: seq 0). */
    fun emit(type: String, payload: String) {
        val m = reply(type, payload)
        listeners[type]?.toList()?.forEach { it(m) }
    }

    fun emitFrame(c: Chunk) { frameListeners.toList().forEach { it(c) } }

    companion object {
        fun reply(type: String, payload: String, rid: String? = null): Message {
            val r = if (rid == null) "" else ""","rid":"$rid""""
            return Protocol.decode("""{"v":1,"type":"$type","ts":"x"$r,"payload":$payload}""")!!
        }
    }
}
```

`test/.../chat/ChatControllerTest.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertNull
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.ChatSend
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class ChatControllerTest {
    private val ch = FakeChannel()
    private var n = 0
    private val chat = ChatController(ch) { "c${++n}" }

    private fun sendOf(i: Int) = Protocol.json.decodeFromJsonElement(ChatSend.serializer(), ch.sent[i].payload)

    @Test fun aSentMessageWaitsForTheHost() {
        ch.handler = { _, _ -> true } // no answer yet
        val id = chat.send("hello")
        assertEquals("c1", id)
        assertEquals(Protocol.CHAT_SEND, ch.sent[0].type)
        assertEquals(ChatSend("c1", "hello", emptyList()), sendOf(0))
        assertEquals(SendStatus.SENDING, chat.outgoing.value.single().status)
    }

    @Test fun theConfirmingMessageDropsIt() {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.CHAT_MESSAGE, """{"id":"c1","role":"user"}""", "r1")); true }
        chat.send("hello")
        assertEquals(emptyList<Outgoing>(), chat.outgoing.value)
    }

    @Test fun anErrorFailsItAndShowsTheHostsText() = runBlocking {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"busy","text":"Answer the question in the agent's terminal first"}""", "r1")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { chat.errors.collect { seen += it } }
        chat.send("hello")
        job.cancel()
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
        assertEquals(listOf("Answer the question in the agent's terminal first"), seen)
    }

    @Test fun aLostConnectionFailsItAndRetryKeepsTheSameId() {
        ch.handler = { _, cb -> cb(null); true }
        chat.send("hello")
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
        ch.handler = { _, _ -> true }
        chat.retry("c1")
        assertEquals(SendStatus.SENDING, chat.outgoing.value.single().status)
        assertEquals("the host drops a repeated id, so retrying never duplicates", "c1", sendOf(1).id)
    }

    @Test fun nothingToSendIsNotSent() {
        assertNull(chat.send("   "))
        assertEquals(0, ch.sent.size)
        val a = Attachment("00112233445566778899aabbccddeeff", "a.png", "image/png", 3, "ab")
        chat.send("", listOf(a))
        assertEquals(listOf(a.id), sendOf(0).attachments)
    }

    @Test fun anUnsendableRequestFailsAtOnce() {
        ch.handler = { _, _ -> false }
        chat.send("hello")
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
    }

    @Test fun discardRemovesIt() {
        ch.handler = { _, cb -> cb(null); true }
        chat.send("hello")
        chat.discard("c1")
        assertEquals(0, chat.outgoing.value.size)
    }
}
```

`test/.../chat/TimelineTest.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState

class TimelineTest {
    private fun m(id: String, conv: String) = ChatMessage(id, "user", "t", conversation = conv)
    private fun out(id: String, st: SendStatus = SendStatus.SENDING) = Outgoing(id, "t", emptyList(), st)

    @Test fun onlyTheCurrentConversationShows() {
        val chat = ChatState(messages = listOf(m("a", "c_1"), m("b", "c_2"), m("c", "")))
        assertEquals(listOf("ha", "hc"), Timeline.of(chat, "c_1", emptyList()).map { it.key })
        assertEquals("no conversation known yet: all show", listOf("ha", "hb", "hc"), Timeline.of(chat, "", emptyList()).map { it.key })
    }

    @Test fun myMessagesGoAfterAndDisappearWhenTheHostHasThem() {
        val chat = ChatState(messages = listOf(m("a", "c_1")))
        val items = Timeline.of(chat, "c_1", listOf(out("a", SendStatus.SENDING), out("x", SendStatus.FAILED)))
        assertEquals(listOf("ha", "mx"), items.map { it.key })
    }
}
```

- [ ] **Paso 2: comprueba que fallan** (no compilan: faltan `Channel`, `ChatController`, `Timeline`)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests '*ChatControllerTest' --tests '*TimelineTest'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa**

`bridge/Channel.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.serialization.json.JsonElement

/**
 * What the chat, the files, the terminal and the capture need from the
 * channel with the host. Bridge implements it through BridgeChannel; the tests
 * use FakeChannel, so none of them needs a network.
 */
interface Channel {
    /** Bridge.request: the new rid, or null when it could not be sent (and onResult is never called). */
    fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String?
    fun send(type: String, payload: JsonElement): Boolean
    fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean
    fun queuedBytes(): Long
    fun listen(type: String, listener: (Message) -> Unit): () -> Unit
    fun listenFrames(listener: (Chunk) -> Unit): () -> Unit

    /** True while the host has welcomed the app. */
    val connected: StateFlow<Boolean>
}

class BridgeChannel(private val bridge: Bridge, scope: CoroutineScope) : Channel {
    override fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit) = bridge.request(type, payload, onResult)
    override fun send(type: String, payload: JsonElement) = bridge.send(type, payload)
    override fun sendFrame(id: String, offset: Long, data: ByteArray) = bridge.sendFrame(id, offset, data)
    override fun queuedBytes() = bridge.queuedBytes()
    override fun listen(type: String, listener: (Message) -> Unit) = bridge.listen(type, listener)
    override fun listenFrames(listener: (Chunk) -> Unit) = bridge.listenFrames(listener)
    override val connected: StateFlow<Boolean> =
        bridge.state.map { it.connection == BridgeState.Connection.CONNECTED }.stateIn(scope, SharingStarted.Eagerly, false)
}
```

`chat/ChatController.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import java.util.UUID
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.ChatSend
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/** A confirmed message is not here any more: Bridge applies the host's chat.message to the state before it calls onResult. */
enum class SendStatus { SENDING, FAILED }

/** A message written on this phone that the chat shows until the host's copy arrives (spec M3 §7.2). */
data class Outgoing(val id: String, val text: String, val attachments: List<Attachment>, val status: SendStatus)

/**
 * Sends the person's messages (chat.send) and keeps the ones the host has not
 * confirmed. The id is the message's for good: the host drops a repeat, so
 * Retry never duplicates (protocol spec §3).
 */
class ChatController(
    private val channel: Channel,
    private val newId: () -> String = { "c" + UUID.randomUUID().toString().replace("-", "") },
) {
    private val _outgoing = MutableStateFlow<List<Outgoing>>(emptyList())
    val outgoing: StateFlow<List<Outgoing>> = _outgoing.asStateFlow()

    private val _errors = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The host's own text of each refused message, for a snackbar. */
    val errors: SharedFlow<String> = _errors.asSharedFlow()

    /** The id of the message, or null when there was nothing to send. */
    fun send(text: String, attachments: List<Attachment> = emptyList()): String? {
        if (text.isBlank() && attachments.isEmpty()) return null
        val o = Outgoing(newId(), text, attachments, SendStatus.SENDING)
        _outgoing.update { it + o }
        post(o)
        return o.id
    }

    fun retry(id: String) {
        val o = _outgoing.value.firstOrNull { it.id == id && it.status == SendStatus.FAILED } ?: return
        set(id, SendStatus.SENDING)
        post(o)
    }

    fun discard(id: String) = _outgoing.update { l -> l.filterNot { it.id == id } }

    private fun post(o: Outgoing) {
        val payload = Protocol.json.encodeToJsonElement(ChatSend.serializer(), ChatSend(o.id, o.text, o.attachments.map { it.id }))
        val rid = channel.request(Protocol.CHAT_SEND, payload) { m -> done(o.id, m) }
        if (rid == null) set(o.id, SendStatus.FAILED)
    }

    private fun done(id: String, m: Message?) {
        when {
            m == null -> set(id, SendStatus.FAILED) // the connection ended first
            m.type == Protocol.ERROR -> {
                set(id, SendStatus.FAILED)
                Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(it.text.ifEmpty { it.code }) }
            }
            else -> discard(id) // the host's own chat.message is already in the state
        }
    }

    private fun set(id: String, st: SendStatus) = _outgoing.update { l -> l.map { if (it.id == id) it.copy(status = st) else it } }
}
```

`chat/Timeline.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState

sealed interface TimelineItem {
    /** Unique and stable: the LazyColumn key. */
    val key: String

    class Host(val msg: ChatMessage) : TimelineItem { override val key = "h" + msg.id }
    class Mine(val out: Outgoing) : TimelineItem { override val key = "m" + out.id }
}

/** What the conversation shows, oldest first (spec M3 §7.2). Pure. */
object Timeline {
    /**
     * conversation: the one of session.state ("" while unknown: all show). The
     * host publishes nothing that empties the chat on session.new, so the
     * messages of other conversations are filtered here; one without a
     * conversation is always shown.
     */
    fun of(chat: ChatState, conversation: String, outgoing: List<Outgoing>): List<TimelineItem> {
        val shown = chat.messages.filter { conversation.isEmpty() || it.conversation.isEmpty() || it.conversation == conversation }
        val known = chat.messages.mapTo(HashSet()) { it.id }
        return shown.map { TimelineItem.Host(it) } + outgoing.filter { it.id !in known }.map { TimelineItem.Mine(it) }
    }
}
```

`AdbuddyApp.kt`: añade al final de `onCreate` (después de crear `bridge`) y como propiedades:

```kotlin
    /** The channel as the controllers see it. */
    lateinit var channel: Channel
        private set

    /** The messages written on this phone (spec M3 §7.2). */
    lateinit var chat: ChatController
        private set
```
y en `onCreate`, tras `bridge = Bridge(...)`: `channel = BridgeChannel(bridge, scope); chat = ChatController(channel)` (imports `tech.hybriq.adbuddy.bridge.BridgeChannel`, `Channel`, `tech.hybriq.adbuddy.chat.ChatController`).

- [ ] **Paso 4: pasan todos los tests y compila la app**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug`
Esperado: PASS / BUILD SUCCESSFUL.

- [ ] **Paso 5: commit**

```bash
git add android
git commit -m "feat(app): Channel, ChatController y Timeline del chat"
```

---

### Tarea 4: modelo de la terminal, teclas rápidas y puente JS

**Ficheros:**
- Crea: `term/TerminalModel.kt`, `term/TerminalFeed.kt`, `term/QuickKeys.kt`, `term/TermBridge.kt`, `chat/Links.kt`
- Modifica: `AdbuddyApp.kt` (el singleton `terminal`)
- Test: `test/.../term/TerminalModelTest.kt`, `TerminalFeedTest.kt`, `QuickKeysTest.kt`, `TermBridgeTest.kt`

**Interfaces:**
- Consume: `Channel`, `TerminalWatch`, `TerminalSnapshot`, `TerminalOutput`, `TerminalSize`, `TerminalInput`, `TerminalResize`, `Protocol.TERMINAL_*`.
- Produce:
  - `sealed interface TermEffect { class Reset(val cols: Int, val rows: Int, val bytes: ByteArray); class Write(val bytes: ByteArray); class Resize(val cols: Int, val rows: Int); object Resync }`
  - `class TerminalModel`: `fun watch(): TerminalWatch`, `fun onSnapshot(s: TerminalSnapshot): List<TermEffect>`, `fun onOutput(o: TerminalOutput): List<TermEffect>`, `fun onSize(s: TerminalSize): List<TermEffect>`, `fun replay(): List<TermEffect>` (para un WebView nuevo), `fun forget()`.
  - `fun interface TermSink { fun apply(e: TermEffect) }`; `class TerminalFeed(channel: Channel)`: `fun attach(sink: TermSink)` (suscribe: `terminal.watch`, reproduce lo guardado), `fun detach(sink: TermSink)` (`terminal.unwatch`, solo si ese `sink` es el enganchado), `fun input(bytes: ByteArray)`, `fun resize(cols: Int, rows: Int)`.
  - `enum class QuickKey(val label: String, val bytes: ByteArray)`; `object Keys { fun chunks(bytes: ByteArray): List<ByteArray> }`.
  - `class TermBridge(onInput: (ByteArray) -> Unit, onSize: (Int, Int) -> Unit, onLink: (url: String, domain: String) -> Unit)` con `data(s: String)`, `size(cols: Int, rows: Int)`, `link(url: String)`.

**Reglas del modelo (contrato real del host, handoff):** el host acota `terminal.resize` a 200×100; un `terminal.snapshot` de más de ~900 KB viene sin estilos (es igual para la app); con presión no manda salida y luego manda un snapshot; un watch al día recibe `terminal.size`. La salida llega con `offset` = posición de su primer byte.

- [ ] **Paso 1: escribe los tests**

Base64: `java.util.Base64`, no `android.util.Base64` (en los tests unitarios de JVM este último no hace nada).

`TerminalModelTest.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import java.util.Base64
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot

class TerminalModelTest {
    private fun b64(s: String) = Base64.getEncoder().encodeToString(s.toByteArray())
    private fun snap(epoch: String = "e1", offset: Long = 10, text: String = "SCREEN") = TerminalSnapshot(epoch, offset, 80, 24, b64(text))
    private fun out(offset: Long, text: String, epoch: String = "e1") = TerminalOutput(epoch, offset, b64(text))
    private fun text(e: TermEffect) = String((e as TermEffect.Write).bytes)

    @Test fun aSnapshotResetsTheViewAndSetsThePosition() {
        val m = TerminalModel()
        val fx = m.onSnapshot(snap())
        val r = fx.single() as TermEffect.Reset
        assertEquals(80, r.cols); assertEquals(24, r.rows); assertEquals("SCREEN", String(r.bytes))
        assertEquals("e1", m.watch().epoch)
        assertEquals(10L, m.watch().offset)
    }

    @Test fun outputInOrderIsWrittenAndAdvancesTheOffset() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals("abc", text(m.onOutput(out(10, "abc")).single()))
        assertEquals(13L, m.watch().offset)
        assertEquals("de", text(m.onOutput(out(13, "de")).single()))
    }

    @Test fun overlappingOutputWritesOnlyWhatIsNew() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        m.onOutput(out(10, "abc"))
        assertEquals("only what is new: d", "d", text(m.onOutput(out(12, "cd")).single()))
        assertTrue("fully seen", m.onOutput(out(10, "abc")).isEmpty())
        assertEquals(14L, m.watch().offset)
    }

    @Test fun aGapOrAnotherEpochAsksForTheScreenAgain() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(99, "x")))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(10, "x", epoch = "e2")))
        assertEquals("nothing was written", 10L, m.watch().offset)
        // Without a screen yet, any output asks for it too.
        assertEquals(listOf(TermEffect.Resync), TerminalModel().onOutput(out(0, "x")))
    }

    @Test fun aSizeChangeAppliesAtItsByte() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertTrue(m.onSize(TerminalSize("e1", 13, 100, 30)).isEmpty())
        val fx = m.onOutput(out(10, "abcdef"))
        assertEquals(listOf("abc", "def"), fx.filterIsInstance<TermEffect.Write>().map { String(it.bytes) })
        val r = fx.filterIsInstance<TermEffect.Resize>().single()
        assertEquals(100, r.cols); assertEquals(30, r.rows)
        assertTrue("the resize sits between the two writes", fx[1] is TermEffect.Resize)
    }

    @Test fun twoSizesInsideOneChunkApplyAtTheirBytesAndTheLoopEnds() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        m.onSize(TerminalSize("e1", 12, 100, 30)); m.onSize(TerminalSize("e1", 14, 120, 40))
        val fx = m.onOutput(out(10, "abcdef"))
        assertEquals(listOf("Write", "Resize", "Write", "Resize", "Write"), fx.map { it::class.simpleName })
        assertEquals(listOf("ab", "cd", "ef"), fx.filterIsInstance<TermEffect.Write>().map { String(it.bytes) })
        assertEquals(16L, m.watch().offset)
    }

    @Test fun anAbsurdOffsetAsksForTheScreenAgain() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(-5, "x")))
    }

    @Test fun aSizeAlreadyDueAppliesAtOnce() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        val r = m.onSize(TerminalSize("e1", 10, 120, 40)).single() as TermEffect.Resize
        assertEquals(120, r.cols)
    }

    @Test fun replayRebuildsTheScreenForANewView() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10, text = "S"))
        m.onOutput(out(10, "ab")); m.onOutput(out(12, "cd"))
        val fx = m.replay()
        assertEquals("S", String((fx[0] as TermEffect.Reset).bytes))
        assertTrue("replayed output is marked", (fx[0] as TermEffect.Reset).replay && fx.drop(1).all { (it as TermEffect.Write).replay })
        assertEquals("abcd", fx.drop(1).joinToString("") { text(it) })
        assertTrue("nothing to replay before the first snapshot", TerminalModel().replay().isEmpty())
    }

    @Test fun tooMuchToReplayStartsOverFromAFreshSnapshot() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 0, text = "S"))
        var off = 0L
        val chunk = "x".repeat(64 * 1024)
        repeat(20) { m.onOutput(out(off, chunk)); off += chunk.length } // 1.25 MiB > the cap
        assertTrue(m.replay().isEmpty())
        assertEquals("a new watch from nothing: the host sends a snapshot", "", m.watch().epoch)
    }

    @Test fun forgetEmptiesIt() {
        val m = TerminalModel(); m.onSnapshot(snap())
        m.forget()
        assertEquals("", m.watch().epoch); assertEquals(0L, m.watch().offset)
    }
}
```

`TerminalFeedTest.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class TerminalFeedTest {
    private val ch = FakeChannel()
    private val got = mutableListOf<TermEffect>()
    private val feed = TerminalFeed(ch)
    private fun b64(s: String) = Base64.getEncoder().encodeToString(s.toByteArray())
    private fun types() = ch.sent.map { it.type }

    @Test fun attachWatchesAndStreamsToTheSink() {
        feed.attach { got += it }
        assertEquals(listOf(Protocol.TERMINAL_WATCH), types())
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("ab")}"}""")
        assertEquals(listOf("Reset", "Write"), got.map { it::class.simpleName })
    }

    @Test fun detachUnwatchesAndStopsTheSink() {
        val sink = TermSink { got += it }
        feed.attach(sink)
        feed.detach(sink)
        assertEquals(listOf(Protocol.TERMINAL_WATCH, Protocol.TERMINAL_UNWATCH), types())
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":0,"data":"${b64("x")}"}""")
        assertTrue(got.isEmpty())
    }

    @Test fun theSecondWatchResumesFromWhatTheModelHas() {
        val first = TermSink { }
        feed.attach(first)
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        feed.detach(first)
        feed.attach { got += it }
        val w = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.TerminalWatch.serializer(), ch.sent.last().payload)
        assertEquals("e1", w.epoch); assertEquals(5L, w.offset)
        assertEquals("the new view gets the screen without asking the host", "Reset", got.first()::class.simpleName)
    }

    @Test fun aResyncAsksAgainFromWhatItHas() {
        feed.attach { got += it }
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("x")}"}""") // no screen yet
        assertEquals(listOf(Protocol.TERMINAL_WATCH, Protocol.TERMINAL_WATCH), types())
    }

    @Test fun anOldViewCannotDetachTheNewOne() {
        val old = TermSink { }
        feed.attach(old)
        feed.attach { got += it } // a bubble opened over the activity
        feed.detach(old)
        assertEquals("no unwatch: the new view still watches", 0, types().count { it == Protocol.TERMINAL_UNWATCH })
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        assertEquals(1, got.size)
    }

    @Test fun manyResyncsAskOnceUntilTheSnapshotComes() {
        feed.attach { got += it }
        repeat(3) { ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("x")}"}""") }
        assertEquals(2, types().count { it == Protocol.TERMINAL_WATCH }) // the attach and one resync
    }

    @Test fun aReconnectWatchesAgain() {
        feed.attach { }
        feed.onConnectedChanged(true)
        assertEquals(2, types().count { it == Protocol.TERMINAL_WATCH })
    }

    @Test fun inputAndResizeGoToTheHost() {
        feed.input("ls\r".toByteArray())
        feed.resize(300, 5)
        assertEquals(listOf(Protocol.TERMINAL_INPUT, Protocol.TERMINAL_RESIZE), types())
        val r = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.TerminalResize.serializer(), ch.sent[1].payload)
        assertEquals("the host caps at 200x100 and the app asks for what fits", 200, r.cols)
        assertEquals(5, r.rows)
    }
}
```

`QuickKeysTest.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class QuickKeysTest {
    @Test fun theBytesAreTheTerminalsOwn() {
        assertArrayEquals(byteArrayOf(0x1b), QuickKey.ESC.bytes)
        assertArrayEquals(byteArrayOf(0x03), QuickKey.CTRL_C.bytes)
        assertArrayEquals("\u001b[A".toByteArray(), QuickKey.UP.bytes)
        assertArrayEquals("\u001b[B".toByteArray(), QuickKey.DOWN.bytes)
        assertArrayEquals("\t".toByteArray(), QuickKey.TAB.bytes)
        assertArrayEquals("\r".toByteArray(), QuickKey.ENTER.bytes)
        assertEquals(listOf("Esc", "Ctrl+C", "↑", "↓", "Tab", "Enter", "1", "2", "3"), QuickKey.entries.map { it.label })
    }

    @Test fun inputIsSplitIn64KiBChunksAndNeverInsideACharacter() {
        val big = ByteArray(64 * 1024 + 10) { 'a'.code.toByte() }
        assertEquals(listOf(65536, 10), Keys.chunks(big).map { it.size })
        // "é" is 2 bytes: a cut must not fall between them.
        val s = ("a".repeat(65535) + "é" + "z").toByteArray()
        val parts = Keys.chunks(s)
        assertEquals(s.toList(), parts.flatMap { it.toList() })
        assertEquals("é" + "z", String(parts.last()))
    }
}
```

`TermBridgeTest.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TermBridgeTest {
    private val inputs = mutableListOf<String>()
    private val sizes = mutableListOf<Pair<Int, Int>>()
    private val links = mutableListOf<Pair<String, String>>()
    private val b = TermBridge({ inputs += String(it) }, { c, r -> sizes += c to r }, { u, d -> links += u to d })

    @Test fun keyboardDataGoesOutAsUtf8() {
        b.data("ñ\r")
        assertEquals(listOf("ñ\r"), inputs)
        b.data("")
        assertEquals(1, inputs.size)
    }

    @Test fun sizesAreClamped() {
        b.size(500, 1)
        b.size(0, 0)
        assertEquals(listOf(200 to 5, 20 to 5), sizes)
        b.size(80, 24)
        assertEquals(80 to 24, sizes.last())
    }

    @Test fun onlyWebLinksOpenAndTheyShowTheirDomain() {
        b.link("https://Example.com/a?b=c")
        b.link("http://localhost:8080/x")
        b.link("intent://evil#Intent;end")
        b.link("file:///sdcard/x")
        b.link("javascript:alert(1)")
        b.link("https://user:pw@example.com@evil.test/")
        // java.net.URI cannot read a host out of that last one: it is dropped, never shown as example.com.
        assertEquals(listOf("https://Example.com/a?b=c" to "example.com", "http://localhost:8080/x" to "localhost"), links)
    }

    @Test fun veryLongUrlsAreDropped() {
        b.link("https://example.com/" + "a".repeat(5000))
        assertTrue(links.isEmpty())
    }
}
```

- [ ] **Paso 2: comprueba que fallan** (no compilan)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.term.*'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa**

`term/QuickKeys.kt`:

```kotlin
package tech.hybriq.adbuddy.term

/** The quick keys under the terminal (spec M3 §7.2): they go as terminal.input, as the terminal would send them. */
enum class QuickKey(val label: String, val bytes: ByteArray) {
    ESC("Esc", byteArrayOf(0x1b)),
    CTRL_C("Ctrl+C", byteArrayOf(0x03)),
    UP("↑", "\u001b[A".toByteArray()),
    DOWN("↓", "\u001b[B".toByteArray()),
    TAB("Tab", "\t".toByteArray()),
    ENTER("Enter", "\r".toByteArray()),
    ONE("1", "1".toByteArray()),
    TWO("2", "2".toByteArray()),
    THREE("3", "3".toByteArray()),
}

object Keys {
    /** terminal.input carries at most 64 KiB (protocol spec §9): cut into chunks, never inside a UTF-8 character. */
    const val MAX_INPUT = 64 * 1024

    fun chunks(bytes: ByteArray): List<ByteArray> {
        val out = ArrayList<ByteArray>()
        var from = 0
        while (from < bytes.size) {
            var to = minOf(from + MAX_INPUT, bytes.size)
            // A continuation byte is 10xxxxxx: step back to the start of the character.
            while (to < bytes.size && to > from + 1 && (bytes[to].toInt() and 0xC0) == 0x80) to--
            out += bytes.copyOfRange(from, to)
            from = to
        }
        return out
    }
}
```

`chat/Links.kt` (lo comparten la terminal y, en T9, el chat):

```kotlin
package tech.hybriq.adbuddy.chat

import java.net.URI

/** Links in what the agent says (spec M3 §5.1 and §7.3): only http(s), and the domain is always shown before opening one. */
object Links {
    const val MAX_URL = 2048

    /** The host of a web link, lower-case, or null when it is not a plain http(s) link with a readable host. */
    fun domain(url: String): String? {
        if (url.length > MAX_URL) return null
        val uri = try { URI(url) } catch (e: Exception) { return null }
        val scheme = uri.scheme?.lowercase()
        if (scheme != "http" && scheme != "https") return null
        return uri.host?.lowercase()
    }
}
```

`term/TermBridge.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import tech.hybriq.adbuddy.chat.Links

/**
 * What xterm.js may tell the app (spec M3 §7.3): keyboard data, a new size
 * and a link to open. Pure; TerminalView puts the three @JavascriptInterface
 * methods in front of it. Everything it gets comes from a page that shows the
 * agent's output, so each value is checked.
 */
class TermBridge(
    private val onInput: (ByteArray) -> Unit,
    private val onSize: (cols: Int, rows: Int) -> Unit,
    private val onLink: (url: String, domain: String) -> Unit,
) {
    fun data(s: String) {
        if (s.isNotEmpty()) onInput(s.toByteArray(Charsets.UTF_8))
    }

    /** The host caps terminal.resize at 200×100; the app does not ask for less than 20×5. */
    fun size(cols: Int, rows: Int) = onSize(cols.coerceIn(MIN_COLS, MAX_COLS), rows.coerceIn(MIN_ROWS, MAX_ROWS))

    /** Only web links, with the domain the dialog shows. OSC 8 and detected URLs come through here. */
    fun link(url: String) {
        val domain = Links.domain(url) ?: return
        onLink(url, domain)
    }

    companion object {
        const val MIN_COLS = 20
        const val MAX_COLS = 200
        const val MIN_ROWS = 5
        const val MAX_ROWS = 100
    }
}
```

`term/TerminalModel.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import java.util.Base64
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot
import tech.hybriq.adbuddy.bridge.TerminalWatch

/** What the terminal view must do, in order. */
sealed interface TermEffect {
    /** Clean the terminal, size it cols×rows and write bytes: the screen at a snapshot. */
    class Reset(val cols: Int, val rows: Int, val bytes: ByteArray, val replay: Boolean = false) : TermEffect

    /** replay: old output drawn again in a new view; the page must not forward what xterm.js answers to its queries (DSR, DA) as keystrokes. */
    class Write(val bytes: ByteArray, val replay: Boolean = false) : TermEffect
    class Resize(val cols: Int, val rows: Int) : TermEffect

    /** The model lost the thread: watch again from what it has (the host answers with a snapshot). */
    object Resync : TermEffect
}

/**
 * What the app knows of the terminal (protocol spec §7): the position (epoch
 * and offset) and enough to redraw it in a new view without asking the host.
 * Pure and not thread-safe: TerminalFeed calls it under its own lock.
 */
class TerminalModel {
    private var epoch = ""
    private var offset = 0L
    private var haveScreen = false

    /** The last snapshot, and the effects since it, in order: replay(). */
    private var base: TermEffect.Reset? = null
    private val tail = ArrayList<TermEffect>()
    private var tailBytes = 0
    private var replayable = false

    /** Sizes that apply from a byte not written yet, oldest first. */
    private val sizes = ArrayDeque<TerminalSize>()

    /** What to send in terminal.watch: nothing (empty and 0) when a replay is not possible, so the host sends a snapshot. */
    fun watch(): TerminalWatch = if (haveScreen && replayable) TerminalWatch(epoch, offset) else TerminalWatch()

    fun forget() {
        epoch = ""; offset = 0; haveScreen = false; replayable = false
        base = null; tail.clear(); tailBytes = 0; sizes.clear()
    }

    fun onSnapshot(s: TerminalSnapshot): List<TermEffect> {
        val bytes = decode(s.data) ?: return listOf(TermEffect.Resync)
        epoch = s.epoch; offset = s.offset
        haveScreen = true; replayable = true
        val reset = TermEffect.Reset(s.cols, s.rows, bytes)
        base = reset; tail.clear(); tailBytes = 0; sizes.clear()
        return listOf(reset)
    }

    fun onOutput(o: TerminalOutput): List<TermEffect> {
        if (!haveScreen || o.epoch != epoch || o.offset < 0 || o.offset > offset) return listOf(TermEffect.Resync)
        val all = decode(o.data) ?: return listOf(TermEffect.Resync)
        val skip = offset - o.offset
        if (skip >= all.size) return emptyList() // everything was seen already
        return write(all.copyOfRange(skip.toInt(), all.size))
    }

    fun onSize(s: TerminalSize): List<TermEffect> {
        if (!haveScreen || s.epoch != epoch) return emptyList()
        if (s.offset > offset) {
            sizes.addLast(s)
            return emptyList()
        }
        return listOf(resize(s))
    }

    /** The screen again, for a view that was just made. Empty when there is nothing, or too much, to replay. */
    fun replay(): List<TermEffect> {
        val b = base
        if (!haveScreen || !replayable || b == null) return emptyList()
        val first = TermEffect.Reset(b.cols, b.rows, b.bytes, replay = true)
        return listOf<TermEffect>(first) + tail.map { if (it is TermEffect.Write) TermEffect.Write(it.bytes, replay = true) else it }
    }

    private fun resize(s: TerminalSize): TermEffect {
        val e = TermEffect.Resize(s.cols, s.rows)
        keep(e, 0)
        return e
    }

    /** Writes data (which starts at offset), applying the sizes that fall inside it at their byte. */
    private fun write(data: ByteArray): List<TermEffect> {
        val fx = ArrayList<TermEffect>()
        var from = 0
        while (from < data.size) {
            val next = sizes.firstOrNull()
            // next.offset is a position in the stream; data[from] is at `offset`: the cut is relative to from, never below it.
            val cut = if (next == null) data.size else (from + (next.offset - offset).coerceIn(0L, (data.size - from).toLong())).toInt()
            if (cut > from) {
                val w = TermEffect.Write(data.copyOfRange(from, cut))
                fx += w
                keep(w, cut - from)
                offset += cut - from
                from = cut
            }
            if (next != null && next.offset <= offset) {
                sizes.removeFirst()
                fx += resize(next)
            } else if (from >= data.size) {
                break
            }
        }
        return fx
    }

    private fun keep(e: TermEffect, bytes: Int) {
        if (!replayable) return
        tail += e
        tailBytes += bytes
        // Too much to redraw in a hurry: the next watch asks the host for a fresh snapshot instead.
        if (tailBytes > MAX_TAIL) { replayable = false; tail.clear(); tailBytes = 0 }
    }

    private fun decode(b64: String): ByteArray? = try { Base64.getDecoder().decode(b64) } catch (e: IllegalArgumentException) { null }

    companion object {
        /** Output kept after the snapshot to redraw a new view (spec M3 §7.3). */
        const val MAX_TAIL = 1024 * 1024
    }
}
```

`term/TerminalFeed.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import java.util.Base64
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.TerminalInput
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalResize
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot
import tech.hybriq.adbuddy.bridge.TerminalWatch

fun interface TermSink { fun apply(e: TermEffect) }

/**
 * The terminal's stream between the host and the view (protocol spec §7).
 * One per process: the model survives a rotation, so the new view is redrawn
 * from memory. It watches only while a view is attached (the host sends
 * nothing else), and Channel's listeners run on OkHttp's reader thread: the
 * sink must hand over to the main thread by itself.
 */
class TerminalFeed(private val channel: Channel) {
    private val lock = Any()
    private val model = TerminalModel()
    private var sink: TermSink? = null
    private var resyncing = false

    init {
        channel.listen(Protocol.TERMINAL_SNAPSHOT) { m -> Protocol.payload<TerminalSnapshot>(m)?.let { s -> feed { resyncing = false; model.onSnapshot(s) } } }
        channel.listen(Protocol.TERMINAL_OUTPUT) { m -> Protocol.payload<TerminalOutput>(m)?.let { o -> feed { model.onOutput(o) } } }
        channel.listen(Protocol.TERMINAL_SIZE) { m -> Protocol.payload<TerminalSize>(m)?.let { s -> feed { model.onSize(s) } } }
    }

    /**
     * Runs a model step and hands its effects to the view, under the lock: attach() replays under it too, so a
     * Write can never overtake the Reset it belongs after (the sink only posts to the main thread, so it is cheap).
     * A Resync becomes a new watch, once until the snapshot it asks for arrives.
     */
    private fun feed(step: () -> List<TermEffect>) {
        val watch = synchronized(lock) {
            val fx = step()
            val target = sink
            if (target != null) fx.forEach { if (it !is TermEffect.Resync) target.apply(it) }
            if (target != null && !resyncing && fx.any { it is TermEffect.Resync }) {
                resyncing = true
                model.watch()
            } else {
                null
            }
        }
        watch?.let(::sendWatch)
    }

    /** The view appeared: redraw from memory and watch. */
    fun attach(sink: TermSink) {
        val watch = synchronized(lock) {
            this.sink = sink
            resyncing = false
            model.replay().forEach { sink.apply(it) }
            model.watch()
        }
        sendWatch(watch)
    }

    /**
     * The view left: stop the stream. Only the view that is attached can: with a bubble over the activity the new
     * view attaches before the old one detaches, and the old one must not take the new one's stream away.
     */
    fun detach(sink: TermSink) {
        val was = synchronized(lock) { if (this.sink === sink) { this.sink = null; true } else false }
        if (was) channel.send(Protocol.TERMINAL_UNWATCH, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty))
    }

    /** The channel is back (or gone): a view on screen watches again, from what it has. */
    fun onConnectedChanged(connected: Boolean) {
        val watch = synchronized(lock) { if (connected && sink != null) model.watch() else null }
        if (watch != null) sendWatch(watch)
    }

    fun input(bytes: ByteArray) {
        for (c in Keys.chunks(bytes)) {
            channel.send(Protocol.TERMINAL_INPUT, Protocol.json.encodeToJsonElement(TerminalInput.serializer(), TerminalInput(Base64.getEncoder().encodeToString(c))))
        }
    }

    /** The size that fits the view; the host caps it and ignores it while a PC terminal is attached. */
    fun resize(cols: Int, rows: Int) {
        val r = TerminalResize(cols.coerceIn(TermBridge.MIN_COLS, TermBridge.MAX_COLS), rows.coerceIn(TermBridge.MIN_ROWS, TermBridge.MAX_ROWS))
        channel.send(Protocol.TERMINAL_RESIZE, Protocol.json.encodeToJsonElement(TerminalResize.serializer(), r))
    }

    private fun sendWatch(w: TerminalWatch) {
        channel.send(Protocol.TERMINAL_WATCH, Protocol.json.encodeToJsonElement(TerminalWatch.serializer(), w))
    }
}
```

Ajustes que el test impone: en `TerminalFeedTest.aResyncAsksAgainFromWhatItHas`, el primer `Resync` llega con un sink puesto → `sendWatch(model.watch())`. En `inputAndResizeGoToTheHost`, `Keys.chunks("ls\r")` da un solo `terminal.input`. `Protocol.TERMINAL_UNWATCH` va sin payload útil: `Empty` es `{}`.

En `AdbuddyApp.kt`: `lateinit var terminal: TerminalFeed`; en `onCreate`: `terminal = TerminalFeed(channel)` y, como el reconectar re-suscribe, `scope.launch { channel.connected.collect { terminal.onConnectedChanged(it) } }` (import `kotlinx.coroutines.launch`).

- [ ] **Paso 4: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug`
Esperado: PASS / BUILD SUCCESSFUL.

- [ ] **Paso 5: commit**

```bash
git add android
git commit -m "feat(app): modelo de la terminal, teclas rápidas y puente JS (sin WebView)"
```

---

### Tarea 5: subidas, borrador y captura

**Ficheros:**
- Crea: `files/FileNames.kt`, `files/Uploader.kt`, `files/Captures.kt`, `files/Drafts.kt`, `files/UriSource.kt`
- Modifica: `AdbuddyApp.kt` (los singletons `drafts` y `captures`, y `attachUri`)
- Test: `test/.../files/FileNamesTest.kt`, `UploaderTest.kt`, `DraftsTest.kt`

**Interfaces:**
- Consume: `Channel`/`FakeChannel` (T3), `AttachBegin/AttachReady/AttachEnd/AttachDone`, `CaptureReady`, `Attachment`, `Frame.MAX_DATA`, `ErrorCode`, `Empty`.
- Produce:
  - `object FileNames { const val MAX_BYTES = 100; fun clean(raw: String): String }`
  - `interface UploadSource : AutoCloseable { val name: String; val mime: String; val size: Long; val sha256: String; fun open(offset: Long): InputStream; override fun close() {} }`
  - `class UploadFailed(val code: String, val text: String) : Exception`
  - `class Uploader(channel, pause, newId)`: `suspend fun upload(src: UploadSource, id: String = …, onProgress: (Long) -> Unit = {}): Attachment`
  - `class Captures(channel)`: `fun take(onResult: (Attachment?, String?) -> Unit)` (segundo parámetro: el texto del host cuando falla; `null` con `Attachment?` también `null` = se perdió la conexión).
  - `enum class DraftStatus { UPLOADING, DONE, FAILED }`, `data class Draft(val key: String, val name: String, val mime: String, val size: Long, val progress: Long, val status: DraftStatus, val attachment: Attachment? = null, val error: String = "")`
  - `class Drafts(scope, uploader, captures, newKey)`: `items: StateFlow<List<Draft>>`, `errors: SharedFlow<String>`, `add(src): String`, `capture()`, `retry(key)`, `remove(key)`, `clear()`.

- [ ] **Paso 1: escribe los tests**

`FileNamesTest.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FileNamesTest {
    @Test fun onlyTheBaseNameWithoutControlCharacters() {
        assertEquals("passwd", FileNames.clean("../../etc/passwd"))
        assertEquals("x.txt", FileNames.clean("C:\\Users\\me\\x.txt"))
        assertEquals("ab.txt", FileNames.clean("a\u0000b\n.txt"))
        assertEquals("file", FileNames.clean(""))
        assertEquals("file", FileNames.clean(".."))
        assertEquals("bashrc", FileNames.clean(".bashrc"))
    }

    @Test fun longNamesKeepTheirExtensionAndNeverSplitACharacter() {
        val n = FileNames.clean("é".repeat(80) + ".png")
        assertTrue(n.toByteArray().size <= FileNames.MAX_BYTES)
        assertTrue(n.endsWith(".png"))
        assertTrue("no broken character", n.removeSuffix(".png").all { it == 'é' })
        assertEquals("short.txt", FileNames.clean("short.txt"))
    }
}
```

`UploaderTest.kt` (con un `BytesSource` y un host falso en el propio test):

```kotlin
package tech.hybriq.adbuddy.files

import java.io.ByteArrayInputStream
import java.io.InputStream
import java.security.MessageDigest
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class BytesSource(val data: ByteArray, override val name: String = "a.bin", override val mime: String = "application/octet-stream") : UploadSource {
    override val size = data.size.toLong()
    override val sha256 = MessageDigest.getInstance("SHA-256").digest(data).joinToString("") { "%02x".format(it) }
    override fun open(offset: Long): InputStream = ByteArrayInputStream(data).also { it.skip(offset) }
}

class UploaderTest {
    private val id = "00112233445566778899aabbccddeeff"
    private val ch = FakeChannel()
    private val pauses = mutableListOf<Long>()
    private val up = Uploader(ch, pause = { pauses += it }, newId = { id })
    private val data = ByteArray(600 * 1024) { (it % 251).toByte() } // 3 frames: 256 + 256 + 88 KiB
    private val att = """{"attachment":{"id":"$id","name":"a.bin","mime":"application/octet-stream","size":${data.size},"sha256":"x"}}"""

    private fun ready(offset: Long) = FakeChannel.reply(Protocol.ATTACH_READY, """{"id":"$id","offset":$offset}""", "r")
    private fun done() = FakeChannel.reply(Protocol.ATTACH_DONE, att, "r")
    private fun error(code: String) = FakeChannel.reply(Protocol.ERROR, """{"code":"$code","text":"nope"}""", "r")

    /** A host that answers begin with ready(offset) and end with done. */
    private fun host(offset: () -> Long = { 0 }) {
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> cb(ready(offset()))
                Protocol.ATTACH_END -> cb(done())
            }
            true
        }
    }

    @Test fun uploadsInFramesOf256KiBAndReturnsTheAttachment() = runBlocking {
        host()
        val progress = mutableListOf<Long>()
        val a = up.upload(BytesSource(data), onProgress = { progress += it })
        assertEquals(id, a.id)
        assertEquals(listOf(0L, 256L * 1024, 512L * 1024), ch.frames.map { it.second })
        assertEquals(listOf(256 * 1024, 256 * 1024, 88 * 1024), ch.frames.map { it.third.size })
        assertTrue(ch.frames.all { it.first == id })
        assertEquals(data.toList(), ch.frames.flatMap { it.third.toList() })
        assertEquals(data.size.toLong(), progress.last())
        val begin = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.AttachBegin.serializer(), ch.sent[0].payload)
        assertEquals(BytesSource(data).sha256, begin.sha256)
        assertEquals(listOf(Protocol.ATTACH_BEGIN, Protocol.ATTACH_END), ch.sent.map { it.type })
    }

    @Test fun resumesFromTheOffsetTheHostHas() = runBlocking {
        host { 256L * 1024 }
        up.upload(BytesSource(data))
        assertEquals(listOf(256L * 1024, 512L * 1024), ch.frames.map { it.second })
        assertEquals(data.copyOfRange(256 * 1024, data.size).toList(), ch.frames.flatMap { it.third.toList() })
    }

    @Test fun aLostAnswerToEndBeginsAgainAndTheHostSaysItIsDone() = runBlocking {
        var begins = 0
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> { begins++; cb(ready(if (begins == 1) 0 else data.size.toLong())) }
                Protocol.ATTACH_END -> if (ch.sent.count { it.type == Protocol.ATTACH_END } == 1) cb(null) else cb(done())
            }
            true
        }
        val a = up.upload(BytesSource(data))
        assertEquals(id, a.id)
        assertEquals(2, begins)
        assertEquals("the second round sends no bytes", 3, ch.frames.size)
        assertTrue(pauses.isNotEmpty())
    }

    @Test fun aRefusedFrameRetriesFromWhatTheHostHas() = runBlocking {
        var begins = 0
        ch.refuseFramesFrom = 1
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> { begins++; if (begins == 2) ch.refuseFramesFrom = -1; cb(ready(if (begins == 1) 0 else 256L * 1024)) }
                Protocol.ATTACH_END -> cb(done())
            }
            true
        }
        up.upload(BytesSource(data))
        assertEquals(2, begins)
        assertEquals(listOf(0L, 256L * 1024, 512L * 1024), ch.frames.map { it.second })
    }

    @Test fun errorsOfTheHostAreFinal() = runBlocking {
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(error("too_large")); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("too_large", e.code); assertEquals("nope", e.text) }
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(ready(0)) else cb(error("checksum")); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("checksum", e.code) }
    }

    @Test fun anImpossibleOffsetIsInvalid() = runBlocking {
        host { data.size + 1L }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("invalid", e.code) }
    }

    @Test fun waitsWhileTheSocketQueueIsHigh() = runBlocking {
        host()
        ch.queued = 5L * 1024 * 1024
        val slow = Uploader(ch, pause = { pauses += it; ch.queued = 0 }, newId = { id })
        slow.upload(BytesSource(data))
        assertTrue(pauses.isNotEmpty())
        assertEquals(3, ch.frames.size)
    }

    @Test fun givesUpAfterRepeatedLostConnections() = runBlocking {
        ch.handler = { _, cb -> cb(null); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("connection", e.code) }
    }

    @Test fun anEmptyFileHasNoFrames() = runBlocking {
        host()
        up.upload(BytesSource(ByteArray(0)))
        assertEquals(0, ch.frames.size)
        assertEquals(listOf(Protocol.ATTACH_BEGIN, Protocol.ATTACH_END), ch.sent.map { it.type })
    }
}
```

`DraftsTest.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class DraftsTest {
    private val id = "00112233445566778899aabbccddeeff"
    private val ch = FakeChannel()
    private var n = 0
    // Unconfined: with the FakeChannel answering at once, each upload ends inside add().
    private val drafts = Drafts(CoroutineScope(Dispatchers.Unconfined), Uploader(ch, pause = {}, newId = { id }), Captures(ch)) { "k${++n}" }
    private val att = """{"attachment":{"id":"$id","name":"a.bin","mime":"image/png","size":3,"sha256":"x"}}"""

    private fun hostOk() {
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> cb(FakeChannel.reply(Protocol.ATTACH_READY, """{"id":"$id","offset":0}""", "r"))
                Protocol.ATTACH_END -> cb(FakeChannel.reply(Protocol.ATTACH_DONE, att, "r"))
                Protocol.CAPTURE_REQUEST -> cb(FakeChannel.reply(Protocol.CAPTURE_READY, att, "r"))
            }
            true
        }
    }

    @Test fun anUploadEndsDoneWithItsAttachment() {
        hostOk()
        val key = drafts.add(BytesSource(byteArrayOf(1, 2, 3), "a.bin"))
        val d = drafts.items.value.single()
        assertEquals(key, d.key)
        assertEquals(DraftStatus.DONE, d.status)
        assertEquals(id, d.attachment?.id)
    }

    @Test fun aFailedUploadShowsTheHostsTextAndRetryTriesAgain() = runBlocking {
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(FakeChannel.reply(Protocol.ERROR, """{"code":"too_large","text":"Too big"}""", "r")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { drafts.errors.collect { seen += it } }
        val key = drafts.add(BytesSource(byteArrayOf(1, 2, 3)))
        assertEquals(DraftStatus.FAILED, drafts.items.value.single().status)
        assertEquals("Too big", drafts.items.value.single().error)
        assertEquals(listOf("Too big"), seen)
        hostOk()
        drafts.retry(key)
        job.cancel()
        assertEquals(DraftStatus.DONE, drafts.items.value.single().status)
    }

    @Test fun aCaptureBecomesADoneDraft() {
        hostOk()
        drafts.capture()
        val d = drafts.items.value.single()
        assertEquals(DraftStatus.DONE, d.status)
        assertEquals("image/png", d.mime)
        assertEquals(Protocol.CAPTURE_REQUEST, ch.sent.single().type)
    }

    @Test fun aFailedCaptureFailsTheDraft() {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"Access is cut"}""", "r")); true }
        drafts.capture()
        assertEquals("Access is cut", drafts.items.value.single().error)
    }

    @Test fun removeAndClearDropDrafts() {
        hostOk()
        val a = drafts.add(BytesSource(byteArrayOf(1)))
        drafts.add(BytesSource(byteArrayOf(2)))
        drafts.remove(a)
        assertEquals(1, drafts.items.value.size)
        drafts.clear()
        assertTrue(drafts.items.value.isEmpty())
    }

    @Test fun removingAnUnknownKeyIsHarmless() {
        drafts.remove("nope")
        assertNull(drafts.items.value.firstOrNull())
    }
}
```

- [ ] **Paso 2: comprueba que fallan** (no compilan)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.files.*'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa**

`files/FileNames.kt`:

```kotlin
package tech.hybriq.adbuddy.files

object FileNames {
    /** The host cuts names to 100 bytes; the app does it first so the draft shows the name that will be kept. */
    const val MAX_BYTES = 100

    /** The base name, without control characters or dots at the ends; "file" when nothing is left. */
    fun clean(raw: String): String {
        val base = raw.substringAfterLast('/').substringAfterLast('\\')
        val name = base.filterNot { it.isISOControl() }.trim().trim('.').trim()
        return if (name.isEmpty()) "file" else cut(name)
    }

    private fun cut(name: String): String {
        if (name.toByteArray().size <= MAX_BYTES) return name
        val dot = name.lastIndexOf('.')
        val ext = if (dot > 0 && name.length - dot <= 11) name.substring(dot) else ""
        val budget = MAX_BYTES - ext.toByteArray().size
        val sb = StringBuilder()
        var used = 0
        for (cp in name.removeSuffix(ext).codePoints().toArray()) {
            val s = String(Character.toChars(cp))
            val n = s.toByteArray().size
            if (used + n > budget) break
            sb.append(s)
            used += n
        }
        return sb.toString() + ext
    }
}
```

`files/Uploader.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.io.InputStream
import java.security.SecureRandom
import kotlin.coroutines.resume
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.AttachBegin
import tech.hybriq.adbuddy.bridge.AttachDone
import tech.hybriq.adbuddy.bridge.AttachEnd
import tech.hybriq.adbuddy.bridge.AttachReady
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Frame
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/** A file ready to upload: its name, size and SHA-256 are known (the app hashes it before it begins). */
interface UploadSource : AutoCloseable {
    val name: String
    val mime: String
    val size: Long
    val sha256: String

    /** The bytes from offset on. */
    fun open(offset: Long): InputStream

    override fun close() {}
}

/** The upload cannot go on. code: a protocol error code, or "connection" after repeated losses; text: the host's, maybe empty. */
class UploadFailed(val code: String, val text: String) : Exception("$code: $text")

/**
 * Uploads one file (protocol spec §6.1): attach.begin, binary frames from the
 * offset the host has, attach.end. A lost connection begins again with the
 * same id and goes on from the host's offset; errors of the host are final.
 */
class Uploader(
    private val channel: Channel,
    private val pause: suspend (ms: Long) -> Unit = { delay(it) },
    val newId: () -> String = { Frame.idString(ByteArray(Frame.ID_LEN).also { SecureRandom().nextBytes(it) }) },
) {
    private sealed interface Round {
        class Done(val attachment: Attachment) : Round
        class Retry(val progressed: Boolean) : Round
    }

    suspend fun upload(src: UploadSource, id: String = newId(), onProgress: (Long) -> Unit = {}): Attachment {
        var failures = 0
        while (true) {
            if (!channel.connected.value) channel.connected.first { it }
            when (val r = round(src, id, onProgress)) {
                is Round.Done -> return r.attachment
                is Round.Retry -> {
                    failures = if (r.progressed) 1 else failures + 1
                    if (failures > MAX_FAILURES) throw UploadFailed("connection", "")
                    pause(RETRY_MS)
                }
            }
        }
    }

    private suspend fun round(src: UploadSource, id: String, onProgress: (Long) -> Unit): Round {
        val begin = call(Protocol.ATTACH_BEGIN, Protocol.json.encodeToJsonElement(AttachBegin.serializer(), AttachBegin(id, src.name, src.mime, src.size, src.sha256)))
            ?: return Round.Retry(false)
        val ready = expect<AttachReady>(begin, Protocol.ATTACH_READY)
        if (ready.offset !in 0..src.size) throw UploadFailed(ErrorCode.INVALID, "")
        var off = ready.offset
        var progressed = false
        onProgress(off)
        if (off < src.size) {
            src.open(off).use { input ->
                val buf = ByteArray(Frame.MAX_DATA)
                while (off < src.size) {
                    // OkHttp closes the socket past 16 MiB queued: wait for room (protocol spec §6.4).
                    while (channel.queuedBytes() > HIGH_WATER) {
                        pause(WAIT_MS)
                        if (!channel.connected.value) return Round.Retry(progressed)
                    }
                    val n = input.read(buf, 0, minOf(buf.size.toLong(), src.size - off).toInt())
                    if (n <= 0) throw UploadFailed(ErrorCode.FAILED, "") // the file changed under the upload
                    if (!channel.sendFrame(id, off, buf.copyOf(n))) return Round.Retry(progressed)
                    off += n
                    progressed = true
                    onProgress(off)
                }
            }
        }
        val end = call(Protocol.ATTACH_END, Protocol.json.encodeToJsonElement(AttachEnd.serializer(), AttachEnd(id))) ?: return Round.Retry(progressed)
        return Round.Done(expect<AttachDone>(end, Protocol.ATTACH_DONE).attachment)
    }

    private inline fun <reified T> expect(m: Message, type: String): T {
        if (m.type == Protocol.ERROR) {
            val e = Protocol.payload<ErrorReply>(m)
            throw UploadFailed(e?.code ?: ErrorCode.FAILED, e?.text ?: "")
        }
        if (m.type != type) throw UploadFailed(ErrorCode.INVALID, "")
        return Protocol.payload<T>(m) ?: throw UploadFailed(ErrorCode.INVALID, "")
    }

    /** The answer to a request, or null when the connection ended first or it could not be sent. */
    private suspend fun call(type: String, payload: JsonElement): Message? = suspendCancellableCoroutine { cont ->
        val rid = channel.request(type, payload) { m -> if (cont.isActive) cont.resume(m) }
        if (rid == null && cont.isActive) cont.resume(null)
    }

    private companion object {
        const val HIGH_WATER = 4L * 1024 * 1024
        const val WAIT_MS = 50L
        const val RETRY_MS = 1_000L
        const val MAX_FAILURES = 5
    }
}
```

`files/Captures.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.CaptureReady
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Protocol

/** Asks the host for a screenshot of this phone (protocol spec §6.3); it comes back as an attachment already in inbox/. */
class Captures(private val channel: Channel) {
    /**
     * onResult(attachment, null) on success; onResult(null, text) when the host
     * refused (its own text, maybe empty); onResult(null, null) when the
     * connection was lost. It runs once, on whichever thread answers.
     */
    fun take(onResult: (Attachment?, String?) -> Unit) {
        val rid = channel.request(Protocol.CAPTURE_REQUEST, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty)) { m ->
            when {
                m == null -> onResult(null, null)
                m.type == Protocol.ERROR -> onResult(null, Protocol.payload<ErrorReply>(m)?.let { it.text.ifEmpty { it.code } } ?: "")
                else -> {
                    val a = if (m.type == Protocol.CAPTURE_READY) Protocol.payload<CaptureReady>(m)?.attachment else null
                    if (a != null) onResult(a, null) else onResult(null, "")
                }
            }
        }
        if (rid == null) onResult(null, null)
    }
}
```

`files/Drafts.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import tech.hybriq.adbuddy.bridge.Attachment

enum class DraftStatus { UPLOADING, DONE, FAILED }

/** An attachment of the message being written: uploading, ready to send (attachment) or failed. */
data class Draft(
    val key: String,
    val name: String,
    val mime: String,
    val size: Long,
    val progress: Long,
    val status: DraftStatus,
    val attachment: Attachment? = null,
    /** The host's text when it refused; empty when there is none (a lost connection, an I/O error). */
    val error: String = "",
)

/**
 * The attachments of the message being written (spec M3 §6.4): at most
 * MAX_PARALLEL uploads at once (the host allows 4 per connection), each
 * with its progress. Sending waits for every draft to be DONE (Composer).
 */
class Drafts(
    private val scope: CoroutineScope,
    private val uploader: Uploader,
    private val captures: Captures,
    private val newKey: () -> String = { UUID.randomUUID().toString() },
) {
    private val _items = MutableStateFlow<List<Draft>>(emptyList())
    val items: StateFlow<List<Draft>> = _items.asStateFlow()

    private val _errors = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The host's text of each refused upload or capture, for a snackbar. */
    val errors: SharedFlow<String> = _errors.asSharedFlow()

    private val gate = Semaphore(MAX_PARALLEL)
    private val lock = Any()
    private val jobs = HashMap<String, Job>()
    private val sources = HashMap<String, UploadSource>()
    private val ids = HashMap<String, String>() // a retry goes on from the host's offset: same id

    fun add(src: UploadSource): String {
        val key = newKey()
        _items.update { it + Draft(key, src.name, src.mime, src.size, 0, DraftStatus.UPLOADING) }
        synchronized(lock) { sources[key] = src }
        start(key, src)
        return key
    }

    fun capture() {
        val key = newKey()
        _items.update { it + Draft(key, CAPTURE_NAME, "image/png", 0, 0, DraftStatus.UPLOADING) }
        captures.take { a, text ->
            when {
                a != null -> update(key) { it.copy(name = a.name, mime = a.mime, size = a.size, progress = a.size, status = DraftStatus.DONE, attachment = a) }
                else -> fail(key, text.orEmpty())
            }
        }
    }

    fun retry(key: String) {
        val src = synchronized(lock) { sources[key] } ?: return
        if (_items.value.firstOrNull { it.key == key }?.status != DraftStatus.FAILED) return
        update(key) { it.copy(status = DraftStatus.UPLOADING, progress = 0, error = "") }
        start(key, src)
    }

    fun remove(key: String) {
        val (job, src) = synchronized(lock) { ids.remove(key); jobs.remove(key) to sources.remove(key) }
        job?.cancel()
        src?.close()
        _items.update { l -> l.filterNot { it.key == key } }
    }

    /** After sending: nothing is kept. */
    fun clear() {
        _items.value.map { it.key }.forEach { remove(it) }
    }

    private fun start(key: String, src: UploadSource) {
        val job = scope.launch {
            gate.withPermit {
                try {
                    val id = synchronized(lock) { ids.getOrPut(key) { uploader.newId() } }
                    val a = uploader.upload(src, id, onProgress = { p -> update(key) { it.copy(progress = p) } })
                    update(key) { it.copy(progress = src.size, status = DraftStatus.DONE, attachment = a) }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: UploadFailed) {
                    fail(key, e.text)
                } catch (e: Exception) { // the scope has no handler: an uncaught one would end the process
                    fail(key, "")
                }
            }
        }
        synchronized(lock) { jobs[key] = job }
    }

    private fun fail(key: String, text: String) {
        update(key) { it.copy(status = DraftStatus.FAILED, error = text) }
        if (text.isNotEmpty()) _errors.tryEmit(text)
    }

    private fun update(key: String, f: (Draft) -> Draft) = _items.update { l -> l.map { if (it.key == key) f(it) else it } }

    companion object {
        const val MAX_PARALLEL = 3
        const val CAPTURE_NAME = "screenshot.png"
    }
}
```

(`Semaphore.withPermit` con `Unconfined`: `start` se ejecuta en línea y termina antes de volver; `jobs[key] = job` se asigna después, lo que es inocuo.)

`files/UriSource.kt` (cola de Android: no se prueba en JVM; lo cubre `assembleDebug` y la validación manual):

```kotlin
package tech.hybriq.adbuddy.files

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import java.io.File
import java.io.FileInputStream
import java.io.InputStream
import java.security.MessageDigest
import java.util.UUID

/**
 * A file picked on the phone, copied to the app's cache (the picker gives a
 * Uri, not a path, and the upload may outlive the permission) and hashed while
 * copying. Never log its name: it is the person's.
 */
class UriSource private constructor(
    private val file: File,
    override val name: String,
    override val mime: String,
    override val size: Long,
    override val sha256: String,
) : UploadSource {
    override fun open(offset: Long): InputStream = FileInputStream(file).also { it.channel.position(offset) }

    override fun close() {
        file.delete()
    }

    companion object {
        /** The most the host takes (spec v1 §5.3). */
        const val MAX_SIZE = 200L * 1024 * 1024

        /** Blocking: call it off the main thread. Throws UploadFailed("too_large", "") or IOException. */
        fun copy(context: Context, uri: Uri): UriSource {
            val resolver = context.contentResolver
            var name = "file"
            var declared = -1L
            resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { c ->
                if (c.moveToFirst()) {
                    c.getString(0)?.let { name = it }
                    if (!c.isNull(1)) declared = c.getLong(1)
                }
            }
            if (declared > MAX_SIZE) throw UploadFailed("too_large", "")
            val dir = File(context.cacheDir, DIR).apply { mkdirs() }
            val out = File(dir, UUID.randomUUID().toString())
            val digest = MessageDigest.getInstance("SHA-256")
            var total = 0L
            try {
                val input = resolver.openInputStream(uri) ?: throw java.io.IOException("cannot open")
                input.use { i -> out.outputStream().use { o ->
                    val buf = ByteArray(64 * 1024)
                    while (true) {
                        val n = i.read(buf)
                        if (n < 0) break
                        total += n
                        if (total > MAX_SIZE) throw UploadFailed("too_large", "")
                        digest.update(buf, 0, n)
                        o.write(buf, 0, n)
                    }
                } }
            } catch (e: Exception) {
                out.delete()
                throw e
            }
            val mime = resolver.getType(uri) ?: "application/octet-stream"
            return UriSource(out, FileNames.clean(name), mime, total, digest.digest().joinToString("") { "%02x".format(it) })
        }

        /** What a dead process left behind. */
        fun clearCache(context: Context) {
            File(context.cacheDir, DIR).deleteRecursively()
        }

        private const val DIR = "uploads"
    }
}
```

`AdbuddyApp.kt`: añade

```kotlin
    lateinit var captures: Captures
        private set
    lateinit var drafts: Drafts
        private set
```
y en `onCreate` (después de `chat`): `UriSource.clearCache(this); captures = Captures(channel); drafts = Drafts(scope, Uploader(channel), captures)`, más (imports `android.net.Uri`, `kotlinx.coroutines.CancellationException`, `kotlinx.coroutines.launch`):

```kotlin
    /**
     * A file or photo picked on the phone: copied, hashed and uploaded as a draft, off the main thread.
     * onFailed gets the host's text, or "" (the screen then says it in its own words); it runs off the main thread too.
     */
    fun attachUri(uri: Uri, onFailed: (String) -> Unit) {
        scope.launch {
            try {
                drafts.add(UriSource.copy(this@AdbuddyApp, uri))
            } catch (e: UploadFailed) {
                onFailed(e.text.ifEmpty { e.code })
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) { // a provider that fails (Drive offline, permission revoked) must not end the process
                onFailed("")
            }
        }
    }
```

- [ ] **Paso 4: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug`
Esperado: PASS / BUILD SUCCESSFUL.

- [ ] **Paso 5: commit**

```bash
git add android
git commit -m "feat(app): subidas con reanudación, borrador de adjuntos y captura"
```

---

### Tarea 6: descargas y caché de ficheros

**Ficheros:**
- Crea: `files/FileCache.kt`, `files/Downloads.kt`, `files/FileIntents.kt`, `res/xml/file_paths.xml`
- Modifica: `AndroidManifest.xml` (el `FileProvider`: **apúntalo antes en `COORDINACION.md`**), `AdbuddyApp.kt` (el singleton `downloads`)
- Test: `test/.../files/FileCacheTest.kt`, `DownloadsTest.kt`

**Interfaces:**
- Consume: `Channel`/`FakeChannel` (T3), `FileNames.clean` (T5), `FileGet`, `FileEnd`, `Attachment`, `Chunk`.
- Produce:
  - `class FileCache(dir: File, maxBytes: Long = 500 MiB)`: `finalFile(a)`, `partFile(a)`, `has(a)`, `touch(f)`, `trim()`.
  - `sealed interface DownloadState { class Running(done: Long, size: Long); class Done(file: File); class Failed(text: String) }`
  - `class Downloads(channel, cache, scope, io)`: `states: StateFlow<Map<String, DownloadState>>`, `get(a)`, `cancel(id)`; `companion fun autoLimit(a: Attachment, remote: Boolean): Boolean`.
  - `object FileIntents { fun view(context, file, mime); fun share(context, file, mime) }`

- [ ] **Paso 1: escribe los tests**

`FileCacheTest.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import tech.hybriq.adbuddy.bridge.Attachment

class FileCacheTest {
    @get:Rule val tmp = TemporaryFolder()

    private fun att(id: String, name: String = "a.png", size: Long = 3) = Attachment(id.padEnd(32, '0'), name, "image/png", size, "x")

    @Test fun namesAreSanitisedAndStayInsideTheFolder() {
        val c = FileCache(tmp.root)
        val f = c.finalFile(att("ab", "../../evil.png"))
        assertEquals(tmp.root, f.parentFile)
        assertEquals("ab".padEnd(32, '0') + "-evil.png", f.name)
    }

    @Test fun hasNeedsTheRightSize() {
        val c = FileCache(tmp.root)
        val a = att("aa", size = 3)
        assertFalse(c.has(a))
        c.finalFile(a).writeBytes(byteArrayOf(1, 2))
        assertFalse(c.has(a))
        c.finalFile(a).writeBytes(byteArrayOf(1, 2, 3))
        assertTrue(c.has(a))
    }

    @Test fun trimRemovesTheOldestFirstUntilItFits() {
        val c = FileCache(tmp.root, maxBytes = 10)
        fun put(name: String, size: Int, age: Long): File = File(tmp.root, name).apply { writeBytes(ByteArray(size)); setLastModified(1_000_000L + age) }
        val old = put("old", 6, 1)
        val mid = put("mid", 6, 2)
        val new = put("new", 6, 3)
        c.trim()
        assertFalse(old.exists()); assertFalse(mid.exists()); assertTrue(new.exists())
    }

    @Test fun trimNeverTouchesPartialDownloads() {
        val c = FileCache(tmp.root, maxBytes = 1)
        val part = File(tmp.root, "x.part").apply { writeBytes(ByteArray(10)) }
        c.trim()
        assertTrue(part.exists())
    }

    @Test fun trimLeavesAnUnderLimitCacheAlone() {
        val c = FileCache(tmp.root, maxBytes = 100)
        val f = File(tmp.root, "x").apply { writeBytes(ByteArray(10)) }
        c.trim()
        assertTrue(f.exists())
    }
}
```

`DownloadsTest.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.security.MessageDigest
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Chunk
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.FileGet
import tech.hybriq.adbuddy.bridge.Protocol

class DownloadsTest {
    @get:Rule val tmp = TemporaryFolder()

    private val ch = FakeChannel()
    private val id = "00112233445566778899aabbccddeeff"
    private val data = ByteArray(300) { it.toByte() }
    private fun sha(b: ByteArray) = MessageDigest.getInstance("SHA-256").digest(b).joinToString("") { "%02x".format(it) }
    private val att = Attachment(id, "photo.png", "image/png", data.size.toLong(), sha(data))
    private lateinit var cache: FileCache
    private lateinit var dl: Downloads

    private fun make() {
        cache = FileCache(tmp.root)
        dl = Downloads(ch, cache, CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined)
    }

    private fun getOf(i: Int) = Protocol.json.decodeFromJsonElement(FileGet.serializer(), ch.sent[i].payload)
    private fun end(size: Int = data.size) = FakeChannel.reply(Protocol.FILE_END, """{"id":"$id","size":$size}""", "r")

    /** A host that serves `content` from the offset asked, in 100-byte frames, then file.end. */
    private fun serve(content: () -> ByteArray = { data }) {
        ch.handler = { s, cb ->
            val g = Protocol.json.decodeFromJsonElement(FileGet.serializer(), s.payload)
            val c = content()
            var off = g.offset.toInt()
            while (off < c.size) { val n = minOf(100, c.size - off); ch.emitFrame(Chunk(id, off.toLong(), c.copyOfRange(off, off + n))); off += n }
            cb(end(c.size))
            true
        }
    }

    @Test fun aDownloadEndsInTheCacheWithTheRightBytes() {
        make(); serve()
        dl.get(att)
        val st = dl.states.value[id] as DownloadState.Done
        assertEquals(data.toList(), st.file.readBytes().toList())
        assertEquals(0L, getOf(0).offset)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun aCachedFileIsNotAskedAgain() {
        make(); serve()
        dl.get(att)
        val n = ch.sent.size
        dl.get(att)
        assertEquals(n, ch.sent.size)
        assertTrue(dl.states.value[id] is DownloadState.Done)
    }

    @Test fun aPartialFileResumesFromItsLength() {
        make()
        cache.partFile(att).writeBytes(data.copyOfRange(0, 100))
        serve()
        dl.get(att)
        assertEquals(100L, getOf(0).offset)
        assertEquals(data.toList(), (dl.states.value[id] as DownloadState.Done).file.readBytes().toList())
    }

    @Test fun framesThatDoNotFollowAreDropped() {
        make()
        ch.handler = { _, cb ->
            ch.emitFrame(Chunk(id, 50, data.copyOfRange(50, 100)))   // a gap: dropped
            ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100)))
            ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100)))     // a repeat: dropped
            ch.emitFrame(Chunk(id, 100, data.copyOfRange(100, 300)))
            cb(end()); true
        }
        dl.get(att)
        assertEquals(data.toList(), (dl.states.value[id] as DownloadState.Done).file.readBytes().toList())
    }

    @Test fun aBadHashIsAskedAgainFromZeroOnceThenFails() {
        make()
        var calls = 0
        serve { calls++; if (calls == 1) ByteArray(data.size) else data }
        dl.get(att)
        assertEquals(2, ch.sent.size)
        assertEquals("the retry starts from 0", 0L, getOf(1).offset)
        assertTrue(dl.states.value[id] is DownloadState.Done)

        tmp.root.listFiles()!!.forEach { it.delete() }
        ch.sent.clear(); make()
        serve { ByteArray(data.size) }
        dl.get(att)
        assertEquals(2, ch.sent.size)
        assertTrue(dl.states.value[id] is DownloadState.Failed)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun anErrorFailsAndDropsWhatWasWritten() {
        make()
        ch.handler = { _, cb -> ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100))); cb(FakeChannel.reply(Protocol.ERROR, """{"code":"not_found","text":"Gone"}""", "r")); true }
        dl.get(att)
        assertEquals("Gone", (dl.states.value[id] as DownloadState.Failed).text)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun aLostConnectionKeepsThePartialForNextTime() {
        make()
        ch.handler = { _, cb -> ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100))); cb(null); true }
        dl.get(att)
        assertTrue(dl.states.value[id] is DownloadState.Failed)
        assertEquals(100L, cache.partFile(att).length())
        serve()
        dl.get(att)
        assertEquals(100L, getOf(1).offset)
        assertTrue(dl.states.value[id] is DownloadState.Done)
    }

    @Test fun anIdThatIsNotHexIsRefusedAndTouchesNoFile() {
        make(); serve()
        dl.get(att.copy(id = "../../x"))
        assertEquals(0, ch.sent.size)
        assertTrue(dl.states.value["../../x"] is DownloadState.Failed)
        assertEquals(0, tmp.root.listFiles()!!.size)
    }

    @Test fun autoLimitIsTenMiBOfImagesAndFiveInRemoteMode() {
        val mib = 1024L * 1024
        fun a(mime: String, size: Long) = Attachment(id, "x", mime, size, "x")
        assertTrue(Downloads.autoLimit(a("image/jpeg", 10 * mib), remote = false))
        assertFalse(Downloads.autoLimit(a("image/jpeg", 10 * mib + 1), remote = false))
        assertTrue(Downloads.autoLimit(a("image/png", 5 * mib), remote = true))
        assertFalse(Downloads.autoLimit(a("image/png", 5 * mib + 1), remote = true))
        assertFalse(Downloads.autoLimit(a("application/pdf", 1), remote = false))
    }
}
```

- [ ] **Paso 2: comprueba que fallan** (no compilan)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests '*FileCacheTest' --tests '*DownloadsTest'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa**

`files/FileCache.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.io.File
import tech.hybriq.adbuddy.bridge.Attachment

/**
 * The downloaded files, in the app's cache (spec M3 §6.4). The name is the
 * attachment's id plus its cleaned name: nothing the host or the agent says
 * can place a file outside the folder. Past maxBytes the oldest go first.
 */
class FileCache(private val dir: File, private val maxBytes: Long = 500L * 1024 * 1024) {
    init { dir.mkdirs() }

    fun finalFile(a: Attachment) = File(dir, a.id + "-" + FileNames.clean(a.name))
    fun partFile(a: Attachment) = File(dir, a.id + ".part")

    fun has(a: Attachment): Boolean = finalFile(a).let { it.isFile && it.length() == a.size }

    /** A file just used or opened is the newest. */
    fun touch(f: File) { f.setLastModified(System.currentTimeMillis()) }

    fun trim() {
        // A .part is a download in progress (or one to resume): never trimmed from under it.
        val files = dir.listFiles()?.filter { it.isFile && !it.name.endsWith(".part") } ?: return
        var total = files.sumOf { it.length() }
        for (f in files.sortedBy { it.lastModified() }) {
            if (total <= maxBytes) break
            total -= f.length()
            f.delete()
        }
    }
}
```

`files/Downloads.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import java.io.File
import java.io.FileOutputStream
import java.security.MessageDigest
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Chunk
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.FileCancel
import tech.hybriq.adbuddy.bridge.Frame
import tech.hybriq.adbuddy.bridge.FileGet
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

sealed interface DownloadState {
    class Running(val done: Long, val size: Long) : DownloadState
    class Done(val file: File) : DownloadState

    /** text: the host's, or empty (a lost connection: what was downloaded stays, and tapping again goes on). */
    class Failed(val text: String) : DownloadState
}

/**
 * Downloads attachments (protocol spec §6.2): file.get from the bytes already
 * in `<id>.part`, each frame written at its offset (one that does not follow
 * what is there is dropped, as those of a cancelled download still arriving),
 * and at file.end the size and SHA-256 are checked. A wrong hash is asked once
 * more from 0. Frames arrive on OkHttp's reader thread; the hash is checked
 * off it.
 */
class Downloads(
    private val channel: Channel,
    private val cache: FileCache,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    private class Active(val a: Attachment, var written: Long, val retried: Boolean)

    private val lock = Any()
    private val active = HashMap<String, Active>()
    private val _states = MutableStateFlow<Map<String, DownloadState>>(emptyMap())
    val states: StateFlow<Map<String, DownloadState>> = _states.asStateFlow()

    init { channel.listenFrames { onChunk(it) } }

    fun get(a: Attachment) = synchronized(lock) {
        // The id comes from the host and names files: 32 lower-case hex digits or nothing (never a path).
        if (Frame.parseId(a.id) == null) {
            set(a.id, DownloadState.Failed(""))
            return@synchronized
        }
        if (active.containsKey(a.id)) return@synchronized
        if (cache.has(a)) {
            cache.touch(cache.finalFile(a))
            set(a.id, DownloadState.Done(cache.finalFile(a)))
        } else {
            start(a, retried = false, fromZero = false)
        }
    }

    fun cancel(id: String) = synchronized(lock) {
        if (active.remove(id) != null) {
            channel.send(Protocol.FILE_CANCEL, Protocol.json.encodeToJsonElement(FileCancel.serializer(), FileCancel(id)))
            _states.update { it - id }
        }
    }

    /** Call it holding the lock. */
    private fun start(a: Attachment, retried: Boolean, fromZero: Boolean) {
        val part = cache.partFile(a)
        if (fromZero || (part.isFile && part.length() > a.size)) part.delete()
        val have = if (part.isFile) part.length() else 0L
        active[a.id] = Active(a, have, retried)
        set(a.id, DownloadState.Running(have, a.size))
        val rid = channel.request(Protocol.FILE_GET, Protocol.json.encodeToJsonElement(FileGet.serializer(), FileGet(a.id, have))) { m ->
            scope.launch(io) { finish(a.id, m) }
        }
        if (rid == null) {
            active.remove(a.id)
            set(a.id, DownloadState.Failed(""))
        }
    }

    private fun onChunk(c: Chunk) = synchronized(lock) {
        val act = active[c.id] ?: return@synchronized
        if (c.offset != act.written || act.written + c.data.size > act.a.size) return@synchronized
        try {
            FileOutputStream(cache.partFile(act.a), true).use { it.write(c.data) }
        } catch (e: java.io.IOException) {
            active.remove(c.id)
            set(c.id, DownloadState.Failed(""))
            return@synchronized
        }
        act.written += c.data.size
        set(c.id, DownloadState.Running(act.written, act.a.size))
    }

    private fun finish(id: String, m: Message?) {
        val act = synchronized(lock) { active[id] } ?: return
        val part = cache.partFile(act.a)
        when {
            m == null -> fail(id, "", keep = true)
            m.type == Protocol.ERROR -> fail(id, Protocol.payload<ErrorReply>(m)?.let { it.text.ifEmpty { it.code } } ?: "", keep = false)
            else -> {
                val ok = part.isFile && part.length() == act.a.size && sha256(part) == act.a.sha256
                synchronized(lock) {
                    if (active[id] !== act) return // cancelled meanwhile
                    when {
                        ok -> {
                            val f = cache.finalFile(act.a)
                            f.delete()
                            if (part.renameTo(f)) {
                                active.remove(id)
                                set(id, DownloadState.Done(f))
                                scope.launch(io) { cache.trim() }
                            } else {
                                fail(id, "", keep = false)
                            }
                        }
                        !act.retried -> { active.remove(id); start(act.a, retried = true, fromZero = true) } // wrong bytes: once more from 0
                        else -> fail(id, "", keep = false)
                    }
                }
            }
        }
    }

    private fun fail(id: String, text: String, keep: Boolean) = synchronized(lock) {
        val act = active.remove(id) ?: return@synchronized
        if (!keep) cache.partFile(act.a).delete()
        set(id, DownloadState.Failed(text))
    }

    private fun set(id: String, s: DownloadState) = _states.update { it + (id to s) }

    private fun sha256(f: File): String {
        val d = MessageDigest.getInstance("SHA-256")
        f.inputStream().use { i ->
            val buf = ByteArray(64 * 1024)
            while (true) { val n = i.read(buf); if (n < 0) break; d.update(buf, 0, n) }
        }
        return d.digest().joinToString("") { "%02x".format(it) }
    }

    companion object {
        private const val MIB = 1024L * 1024

        /** Images download by themselves up to 10 MiB, 5 MiB in remote mode (protocol spec §6.2); the rest waits for a tap. */
        fun autoLimit(a: Attachment, remote: Boolean): Boolean = a.mime.startsWith("image/") && a.size <= (if (remote) 5 else 10) * MIB
    }
}
```

`files/FileIntents.kt`:

```kotlin
package tech.hybriq.adbuddy.files

import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import java.io.File

/** Opens or shares a downloaded file with a temporary read permission (spec M3 §6.4); the file never leaves the app's cache. */
object FileIntents {
    private fun uri(context: Context, file: File) = FileProvider.getUriForFile(context, context.packageName + ".files", file)

    /** false when no app opens that type. */
    fun view(context: Context, file: File, mime: String): Boolean {
        val u = uri(context, file)
        val i = Intent(Intent.ACTION_VIEW).setDataAndType(u, mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        return try { context.startActivity(i); true } catch (e: ActivityNotFoundException) { false }
    }

    fun share(context: Context, file: File, mime: String): Boolean {
        val u = uri(context, file)
        val send = Intent(Intent.ACTION_SEND).setType(mime).putExtra(Intent.EXTRA_STREAM, u).apply { clipData = ClipData.newRawUri("", u) }
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        return try { context.startActivity(Intent.createChooser(send, null).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)); true } catch (e: ActivityNotFoundException) { false }
    }
}
```

`res/xml/file_paths.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<paths>
    <!-- Only the downloads folder of the cache (FileCache), never the rest of it. -->
    <cache-path name="downloads" path="downloads/" />
</paths>
```

`AndroidManifest.xml`, dentro de `<application>` (junto al `TokenProvider`):

```xml
        <!-- Opens and shares the downloaded attachments; no other file of the app is reachable through it. -->
        <provider
            android:name="androidx.core.content.FileProvider"
            android:authorities="${applicationId}.files"
            android:exported="false"
            android:grantUriPermissions="true">
            <meta-data
                android:name="android.support.FILE_PROVIDER_PATHS"
                android:resource="@xml/file_paths" />
        </provider>
```

`AdbuddyApp.kt`: `lateinit var downloads: Downloads` y, en `onCreate`: `downloads = Downloads(channel, FileCache(File(cacheDir, "downloads")), scope)`.

- [ ] **Paso 4: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`
Esperado: PASS / BUILD SUCCESSFUL.

- [ ] **Paso 5: commit**

```bash
git add android
git commit -m "feat(app): descargas con reanudación y verificación, caché y FileProvider"
```

---

### Tarea 7: base de diseño (tema, modo oscuro, insets, jerarquía)

**Ficheros:**
- Crea: `ui/theme/Palette.kt`, `ui/theme/Theme.kt`, `res/values/themes.xml`, `res/values-night/themes.xml`
- Modifica: `android/gradle/libs.versions.toml` y `android/app/build.gradle.kts` (solo el bloque de dependencias: `androidx.compose.material:material-icons-core`, con la versión de la BOM; material3 ya no la trae y hoy la app no usa iconos), `ui/Screens.kt` (`ConsentScreen`, `HomeScreen`, `EntryRow`), `MainActivity.kt`, `AndroidManifest.xml` (solo `android:theme` de `<application>`: **apúntalo en `COORDINACION.md`**), `res/values/strings.xml` y `values-es/strings.xml`
- Test: `test/.../ui/theme/PaletteContrastTest.kt`

**Interfaces:**
- Consume: nada nuevo.
- Produce: `@Composable fun AdbuddyTheme(dark: Boolean = isSystemInDarkTheme(), dynamic: Boolean = true, content: @Composable () -> Unit)`; `annotation class AdbuddyPreviews` (claro, oscuro y letra al 200 %); `object Palette { val light: Map<String, Int>; val dark: Map<String, Int>; fun contrast(a: Int, b: Int): Double }`. Las pantallas de T9 usan el tema y las previews.

**Antes de escribir pantallas, invoca la skill `frontend-design`** y anota en el ledger la dirección elegida (la spec §7.1 la pide). Reglas que no cambian: Material 3, colores solo de `MaterialTheme.colorScheme`, insets respetados, escala tipográfica de M3, cabecera + contenido + acciones al pie.

- [ ] **Paso 1: escribe el test de contraste** (la única parte de diseño que se automatiza: la paleta propia de Android 11 y de los dispositivos sin color dinámico debe leerse)

```kotlin
package tech.hybriq.adbuddy.ui.theme

import org.junit.Assert.assertTrue
import org.junit.Test

class PaletteContrastTest {
    // The pairs that carry text or controls (WCAG AA: 4.5 for text).
    private val pairs = listOf(
        "onPrimary" to "primary", "onPrimaryContainer" to "primaryContainer",
        "onSecondary" to "secondary", "onSecondaryContainer" to "secondaryContainer",
        "onSurface" to "surface", "onSurface" to "background", "onSurfaceVariant" to "surfaceVariant",
        "onError" to "error", "onErrorContainer" to "errorContainer",
        "primary" to "surface", "error" to "surface",
    )

    private fun check(name: String, scheme: Map<String, Int>) {
        for ((fg, bg) in pairs) {
            val c = Palette.contrast(scheme.getValue(fg), scheme.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
    }

    @Test fun lightPaletteIsReadable() = check("light", Palette.light)
    @Test fun darkPaletteIsReadable() = check("dark", Palette.dark)

    @Test fun contrastOfBlackOnWhiteIs21() {
        assertTrue(Math.abs(Palette.contrast(0xFF000000.toInt(), 0xFFFFFFFF.toInt()) - 21.0) < 0.01)
    }
}
```

- [ ] **Paso 2: comprueba que falla** (no compila)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests '*PaletteContrastTest'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa**

`ui/theme/Palette.kt` (los valores son un punto de partida teal; si el test de contraste rechaza un par, **ajusta el color, no el umbral**):

```kotlin
package tech.hybriq.adbuddy.ui.theme

import kotlin.math.pow

/**
 * ADBuddy's own palette, used on Android 11 (no Material You) and wherever the
 * dynamic color is off. Plain ARGB values so the contrast of every pair can be
 * tested on the JVM (PaletteContrastTest). Nowhere else in the app has a fixed color.
 */
object Palette {
    val light: Map<String, Int> = mapOf(
        "primary" to 0xFF006B5E.toInt(), "onPrimary" to 0xFFFFFFFF.toInt(),
        "primaryContainer" to 0xFF9FF2E1.toInt(), "onPrimaryContainer" to 0xFF00201B.toInt(),
        "secondary" to 0xFF4A6360.toInt(), "onSecondary" to 0xFFFFFFFF.toInt(),
        "secondaryContainer" to 0xFFCCE8E3.toInt(), "onSecondaryContainer" to 0xFF051F1D.toInt(),
        "background" to 0xFFF4FBF8.toInt(), "onBackground" to 0xFF161D1B.toInt(),
        "surface" to 0xFFF4FBF8.toInt(), "onSurface" to 0xFF161D1B.toInt(),
        "surfaceVariant" to 0xFFDAE5E1.toInt(), "onSurfaceVariant" to 0xFF3F4947.toInt(),
        "outline" to 0xFF6F7977.toInt(),
        "error" to 0xFFBA1A1A.toInt(), "onError" to 0xFFFFFFFF.toInt(),
        "errorContainer" to 0xFFFFDAD6.toInt(), "onErrorContainer" to 0xFF410002.toInt(),
    )

    val dark: Map<String, Int> = mapOf(
        "primary" to 0xFF82D5C5.toInt(), "onPrimary" to 0xFF00382F.toInt(),
        "primaryContainer" to 0xFF005046.toInt(), "onPrimaryContainer" to 0xFF9FF2E1.toInt(),
        "secondary" to 0xFFB1CCC7.toInt(), "onSecondary" to 0xFF1C3532.toInt(),
        "secondaryContainer" to 0xFF334B48.toInt(), "onSecondaryContainer" to 0xFFCCE8E3.toInt(),
        "background" to 0xFF0E1513.toInt(), "onBackground" to 0xFFDEE4E1.toInt(),
        "surface" to 0xFF0E1513.toInt(), "onSurface" to 0xFFDEE4E1.toInt(),
        "surfaceVariant" to 0xFF3F4947.toInt(), "onSurfaceVariant" to 0xFFBEC9C5.toInt(),
        "outline" to 0xFF899390.toInt(),
        "error" to 0xFFFFB4AB.toInt(), "onError" to 0xFF690005.toInt(),
        "errorContainer" to 0xFF93000A.toInt(), "onErrorContainer" to 0xFFFFDAD6.toInt(),
    )

    /** WCAG contrast ratio of two ARGB colors (alpha ignored). */
    fun contrast(a: Int, b: Int): Double {
        val la = luminance(a)
        val lb = luminance(b)
        return (maxOf(la, lb) + 0.05) / (minOf(la, lb) + 0.05)
    }

    private fun luminance(c: Int): Double {
        fun ch(v: Int): Double {
            val s = v / 255.0
            return if (s <= 0.03928) s / 12.92 else ((s + 0.055) / 1.055).pow(2.4)
        }
        return 0.2126 * ch((c shr 16) and 0xFF) + 0.7152 * ch((c shr 8) and 0xFF) + 0.0722 * ch(c and 0xFF)
    }
}
```

`ui/theme/Theme.kt`:

```kotlin
package tech.hybriq.adbuddy.ui.theme

import android.content.res.Configuration
import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview

private fun Map<String, Int>.c(name: String) = Color(getValue(name))

private val lightScheme: ColorScheme = Palette.light.let {
    lightColorScheme(
        primary = it.c("primary"), onPrimary = it.c("onPrimary"), primaryContainer = it.c("primaryContainer"), onPrimaryContainer = it.c("onPrimaryContainer"),
        secondary = it.c("secondary"), onSecondary = it.c("onSecondary"), secondaryContainer = it.c("secondaryContainer"), onSecondaryContainer = it.c("onSecondaryContainer"),
        background = it.c("background"), onBackground = it.c("onBackground"), surface = it.c("surface"), onSurface = it.c("onSurface"),
        surfaceVariant = it.c("surfaceVariant"), onSurfaceVariant = it.c("onSurfaceVariant"), outline = it.c("outline"),
        error = it.c("error"), onError = it.c("onError"), errorContainer = it.c("errorContainer"), onErrorContainer = it.c("onErrorContainer"),
    )
}

private val darkScheme: ColorScheme = Palette.dark.let {
    darkColorScheme(
        primary = it.c("primary"), onPrimary = it.c("onPrimary"), primaryContainer = it.c("primaryContainer"), onPrimaryContainer = it.c("onPrimaryContainer"),
        secondary = it.c("secondary"), onSecondary = it.c("onSecondary"), secondaryContainer = it.c("secondaryContainer"), onSecondaryContainer = it.c("onSecondaryContainer"),
        background = it.c("background"), onBackground = it.c("onBackground"), surface = it.c("surface"), onSurface = it.c("onSurface"),
        surfaceVariant = it.c("surfaceVariant"), onSurfaceVariant = it.c("onSurfaceVariant"), outline = it.c("outline"),
        error = it.c("error"), onError = it.c("onError"), errorContainer = it.c("errorContainer"), onErrorContainer = it.c("onErrorContainer"),
    )
}

/** Material 3, light and dark, with Material You on Android 12+ and ADBuddy's palette elsewhere (spec M3 §7.1). */
@Composable
fun AdbuddyTheme(dark: Boolean = isSystemInDarkTheme(), dynamic: Boolean = true, content: @Composable () -> Unit) {
    val context = LocalContext.current
    val scheme = when {
        dynamic && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> darkScheme
        else -> lightScheme
    }
    MaterialTheme(colorScheme = scheme, content = content)
}


/** Every screen has these three previews: light, dark and text at 200 % (spec M3 §7.1). */
@Preview(name = "light", showBackground = true)
@Preview(name = "dark", uiMode = Configuration.UI_MODE_NIGHT_YES, showBackground = true)
@Preview(name = "font 200%", fontScale = 2f, showBackground = true)
annotation class AdbuddyPreviews
```

`res/values/themes.xml` y `res/values-night/themes.xml` (ventana antes de que Compose pinte: sin fogonazo blanco en oscuro):

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <style name="Theme.Adbuddy" parent="@android:style/Theme.Material.Light.NoActionBar">
        <item name="android:windowBackground">#FFF4FBF8</item>
    </style>
</resources>
```
```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <style name="Theme.Adbuddy" parent="@android:style/Theme.Material.NoActionBar">
        <item name="android:windowBackground">#FF0E1513</item>
    </style>
</resources>
```
(Es el único sitio, junto a `Palette`, con colores fijos: son los mismos `background` de la paleta, y la ventana no puede leer un tema Compose.) En el manifiesto: `android:theme="@style/Theme.Adbuddy"`.

`MainActivity.kt`: `enableEdgeToEdge()` (import `androidx.activity.enableEdgeToEdge`) como primera línea de `onCreate` tras `super.onCreate`, y `MaterialTheme { … }` pasa a `AdbuddyTheme { … }`.

`ui/Screens.kt` — qué cambia (mantén las firmas que usa `MainActivity`):
- `ConsentScreen`: `Scaffold` con `bottomBar` = fila de acciones (`OutlinedButton` "Ahora no" con `borderStroke` de `outline` y texto `onSurface`, y `Button` "Aceptar"), contenido con `verticalScroll` y el relleno de `Scaffold`, cabecera (icono `Icons.Default.Lock` en un círculo `primaryContainer` + `headlineSmall`), la lista de lo que se concede como tres filas con icono (`Lock`, `Notifications`, `Settings`: solo iconos de `material-icons-core`) y la nota de privacidad aparte en una `Surface` `surfaceVariant` con `bodySmall`.
- `HomeScreen`: envuelto en `Scaffold`, con `Modifier.padding(inner)`; cada `EntryRow` pasa a una `ElevatedCard` con el icono de nivel, el texto en `bodyLarge` y el estado en `labelMedium`; los títulos con `titleMedium`.
- Ambas con `@AdbuddyPreviews` (una función `private` de preview por pantalla, con datos de ejemplo).
- Nada de `Color(...)` ni `Color.Red`: `grep -n "Color(" android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt` no debe encontrar nada.

- [ ] **Paso 4: pasan los tests, compila y lint**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`
Esperado: PASS / BUILD SUCCESSFUL.

- [ ] **Paso 5: commit**

```bash
git add android
git commit -m "feat(app): base de diseño: tema claro y oscuro, borde a borde y jerarquía"
```

---

### Tarea 8: vista de terminal (xterm.js en un WebView sin red)

**Ficheros:**
- Crea: `term/TermWebPolicy.kt`, `term/TerminalView.kt`, `assets/term/{index.html,term.js,term.css,xterm.js,xterm.css,addon-fit.js,addon-web-links.js,LICENSE-xterm.txt,VENDOR.md,.gitattributes}`
- Modifica: `android/gradle/libs.versions.toml` y `android/app/build.gradle.kts` (dependencia `androidx.webkit`)
- Test: `test/.../term/TermWebPolicyTest.kt`, `VendoredAssetsTest.kt`

**Interfaces:**
- Consume: `TerminalFeed`, `TermSink`, `TermEffect`, `TermBridge` (T4), `AdbuddyTheme` (T7).
- Produce: `object TermWebPolicy { const val ORIGIN = "https://appassets.androidplatform.net"; const val PAGE = "$ORIGIN/assets/term/index.html"; fun allowed(url: String): Boolean }`; `@Composable fun TerminalView(feed: TerminalFeed, onLink: (url: String, domain: String) -> Unit, modifier: Modifier = Modifier)`.

- [ ] **Paso 1: trae los ficheros de xterm.js** (versiones fijadas; MIT)

```bash
# $SCRATCH: la carpeta temporal de la sesión; $REPO: la raíz del repo
mkdir -p "$SCRATCH/xterm" && cd "$SCRATCH/xterm"
npm pack @xterm/xterm@5.5.0 @xterm/addon-fit@0.10.0 @xterm/addon-web-links@0.11.0
for f in *.tgz; do mkdir -p "${f%.tgz}" && tar -xzf "$f" -C "${f%.tgz}"; done
A="$REPO/android/app/src/main/assets/term"
mkdir -p "$A"
cp xterm-xterm-5.5.0/package/lib/xterm.js xterm-xterm-5.5.0/package/css/xterm.css \
   xterm-addon-fit-0.10.0/package/lib/addon-fit.js xterm-addon-web-links-0.11.0/package/lib/addon-web-links.js "$A/"
cp xterm-xterm-5.5.0/package/LICENSE "$A/LICENSE-xterm.txt"
# Git no debe tocar los saltos de línea de lo vendorizado: el test compara hashes.
printf '* -text\n' > "$A/.gitattributes"
cd "$A" && sha256sum xterm.js xterm.css addon-fit.js addon-web-links.js
```

(Los nombres exactos de los ficheros dentro de cada paquete se comprueban con `tar -tzf`; si alguno ya no existe en esa versión, **para y avisa**: no cambies de versión sin pasar por la revisión.) Escribe `VENDOR.md` con las versiones, el origen (`npm pack`), la licencia (MIT) y una línea `sha256  fichero` por cada uno (tal cual la imprime `sha256sum` ejecutado dentro de la carpeta, con el nombre sin ruta).

- [ ] **Paso 2: escribe los tests**

`TermWebPolicyTest.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TermWebPolicyTest {
    @Test fun onlyTheBundledTerminalAssetsLoad() {
        assertTrue(TermWebPolicy.allowed("https://appassets.androidplatform.net/assets/term/index.html"))
        assertTrue(TermWebPolicy.allowed("https://appassets.androidplatform.net/assets/term/xterm.js"))
    }

    @Test fun everythingElseIsBlocked() {
        for (u in listOf(
            "https://example.com/assets/term/index.html",
            "https://appassets.androidplatform.net/assets/other.html",
            "https://appassets.androidplatform.net/assets/term/../secret",
            "https://appassets.androidplatform.net/assets/term/%2e%2e/secret",
            "https://appassets.androidplatform.net.evil.com/assets/term/index.html",
            "http://appassets.androidplatform.net/assets/term/index.html",
            "file:///android_asset/term/index.html", "content://x/y", "data:text/html,hi", "javascript:alert(1)", "",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }
}
```

`VendoredAssetsTest.kt` (lee los ficheros de verdad; los tests unitarios corren con `android/app` como directorio de trabajo):

```kotlin
package tech.hybriq.adbuddy.term

import java.io.File
import java.security.MessageDigest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class VendoredAssetsTest {
    private val dir = File("src/main/assets/term")
    private fun sha(f: File) = MessageDigest.getInstance("SHA-256").digest(f.readBytes()).joinToString("") { "%02x".format(it) }

    @Test fun vendoredFilesMatchTheirRecordedHash() {
        val recorded = File(dir, "VENDOR.md").readLines().mapNotNull { Regex("^([0-9a-f]{64})\\s+(\\S+)$").find(it.trim())?.destructured }
            .associate { (h, n) -> n to h }
        for (n in listOf("xterm.js", "xterm.css", "addon-fit.js", "addon-web-links.js")) {
            assertEquals("hash of $n", recorded[n], sha(File(dir, n)))
        }
    }

    @Test fun thePageLoadsOnlyItsOwnFilesUnderAStrictPolicy() {
        val html = File(dir, "index.html").readText()
        assertTrue(html.contains("Content-Security-Policy"))
        assertTrue(html.contains("default-src 'none'"))
        assertFalse("no external URL", Regex("(src|href)=\"(https?:)?//").containsMatchIn(html))
        val js = File(dir, "term.js").readText()
        assertFalse("no network from our own script", Regex("fetch\\(|XMLHttpRequest|WebSocket|importScripts|eval\\(").containsMatchIn(js))
    }
}
```

- [ ] **Paso 3: comprueba que fallan** (`TermWebPolicy` no existe; los assets aún no están completos)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.term.*'`
Esperado: FAIL.

- [ ] **Paso 4: implementa**

`gradle/libs.versions.toml`: en `[versions]` `webkit = "1.14.0"` y en `[libraries]` `androidx-webkit = { module = "androidx.webkit:webkit", version.ref = "webkit" }`; `app/build.gradle.kts`: `implementation(libs.androidx.webkit)` (si 1.14.0 no resuelve en Google Maven, usa la última estable y anótalo en el ledger).

`term/TermWebPolicy.kt`:

```kotlin
package tech.hybriq.adbuddy.term

/** What the terminal's WebView may load: only the bundled files under /assets/term/ (spec M3 §7.3). */
object TermWebPolicy {
    const val ORIGIN = "https://appassets.androidplatform.net"
    const val PREFIX = "$ORIGIN/assets/term/"
    const val PAGE = PREFIX + "index.html"

    fun allowed(url: String): Boolean {
        if (!url.startsWith(PREFIX)) return false
        val rest = url.removePrefix(PREFIX)
        // No way out of the folder, however it is written.
        return rest.isNotEmpty() && !rest.contains("..") && !rest.contains('%') && !rest.contains('\\')
    }
}
```

`assets/term/index.html`:

```html
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:">
<link rel="stylesheet" href="xterm.css">
<link rel="stylesheet" href="term.css">
</head>
<body>
<div id="t"></div>
<script src="xterm.js"></script>
<script src="addon-fit.js"></script>
<script src="addon-web-links.js"></script>
<script src="term.js"></script>
</body>
</html>
```

`assets/term/term.css`:

```css
html, body { margin: 0; height: 100%; overflow: hidden; }
/* The scrolling box: also what the fit addon measures, so the proposed size is the view's, not the terminal's. */
#t { width: 100vw; height: 100vh; overflow: auto; }
```

`assets/term/term.js` (las tres llamadas al puente `window.adbuddy` son `data`, `size` y `link`; nada más):

```js
(function () {
  'use strict';
  var host = window.adbuddy;
  var term = new Terminal({
    fontSize: 12,
    scrollback: 2000,
    cursorBlink: true,
    allowProposedApi: false,
    // OSC 8 links: only activated by a tap, and the app shows the domain first.
    linkHandler: { allowNonHttpProtocols: false, activate: function (e, uri) { host.link(uri); } }
  });
  // No clipboard addon is loaded: OSC 52 (a program writing to the clipboard) does nothing.
  var fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new WebLinksAddon.WebLinksAddon(function (e, uri) { host.link(uri); }));
  term.open(document.getElementById('t'));

  function bytes(b64) {
    var s = atob(b64), a = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
    return a;
  }
  // The host rules the size (protocol spec §7): the view only proposes one.
  function propose() {
    var d = fit.proposeDimensions();
    if (d && d.cols > 0 && d.rows > 0) host.size(d.cols, d.rows);
  }

  // Replayed output may hold queries (DSR, DA) that xterm.js answers through onData: those answers are not the person's keys.
  var quiet = 0;
  function put(b64, isReplay) {
    if (isReplay) { quiet++; term.write(bytes(b64), function () { quiet--; }); } else { term.write(bytes(b64)); }
  }
  window.termReset = function (cols, rows, b64, isReplay) { term.reset(); term.resize(cols, rows); put(b64, isReplay); };
  window.termWrite = function (b64, isReplay) { put(b64, isReplay); };
  window.termResize = function (cols, rows) { term.resize(cols, rows); };
  window.termTheme = function (bg, fg, cursor) {
    term.options.theme = { background: bg, foreground: fg, cursor: cursor };
    document.body.style.background = bg;
  };
  window.termFocus = function () { term.focus(); };

  term.onData(function (d) { if (quiet === 0) host.data(d); });
  new ResizeObserver(propose).observe(document.getElementById('t'));
  propose();
})();
```

`term/TerminalView.kt`:

```kotlin
package tech.hybriq.adbuddy.term

import android.annotation.SuppressLint
import android.graphics.Color as AColor
import android.webkit.JavascriptInterface
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.webkit.WebViewAssetLoader
import java.io.ByteArrayInputStream
import java.util.Base64

/** The three methods the page may call (spec M3 §7.3). They run on a WebView thread, not the main one. */
private class JsHost(private val bridge: TermBridge) {
    @JavascriptInterface fun data(s: String) = bridge.data(s)
    @JavascriptInterface fun size(cols: Int, rows: Int) = bridge.size(cols, rows)
    @JavascriptInterface fun link(url: String) = bridge.link(url)
}

/**
 * Draws the effects of the model in the page. What comes before the page has loaded waits, and so does the theme
 * (the first one is set before the page exists). After the renderer died it draws nothing: a new view replaces it.
 */
private class WebSink(private val web: WebView) : TermSink {
    private var ready = false
    private var dead = false
    private var theme = ""
    private val queue = ArrayList<TermEffect>()

    fun loaded() { web.post { if (dead) return@post; ready = true; draw(theme); queue.forEach(::draw); queue.clear() } }
    fun setTheme(js: String) { web.post { theme = js; if (ready && !dead) draw(js) } }
    fun die() { web.post { dead = true; queue.clear() } }

    override fun apply(e: TermEffect) {
        web.post { if (!dead) { if (ready) draw(e) else queue += e } }
    }

    private fun draw(js: String) { if (js.isNotEmpty()) web.evaluateJavascript(js, null) }

    private fun b64(b: ByteArray) = Base64.getEncoder().encodeToString(b)

    // Base64 has no quote nor backslash, so it is safe inside a JS string.
    private fun draw(e: TermEffect) = when (e) {
        is TermEffect.Reset -> draw("termReset(${e.cols},${e.rows},'${b64(e.bytes)}',${e.replay})")
        is TermEffect.Write -> draw("termWrite('${b64(e.bytes)}',${e.replay})")
        is TermEffect.Resize -> draw("termResize(${e.cols},${e.rows})")
        TermEffect.Resync -> Unit // TerminalFeed handles it
    }
}

private fun hex(argb: Int) = "#%06X".format(argb and 0xFFFFFF)

/**
 * The agent's terminal (spec M3 §7.3): xterm.js from the app's own assets,
 * in a WebView that cannot reach the network, the files or any content. It
 * watches the terminal only while it is on screen.
 */
@SuppressLint("SetJavaScriptEnabled") // xterm.js needs it; nothing else is loaded (TermWebPolicy)
@Composable
fun TerminalView(feed: TerminalFeed, onLink: (url: String, domain: String) -> Unit, modifier: Modifier = Modifier) {
    val colors = MaterialTheme.colorScheme
    val themeJs = "termTheme('${hex(colors.surface.toArgb())}','${hex(colors.onSurface.toArgb())}','${hex(colors.primary.toArgb())}')"
    // A crash of the renderer leaves the WebView unusable (onRenderProcessGone): a new key makes a new view.
    var generation by remember { mutableIntStateOf(0) }

    key(generation) {
        val holder = remember { arrayOfNulls<WebSink>(1) }
        AndroidView(
            modifier = modifier,
            factory = { context ->
                val loader = WebViewAssetLoader.Builder().addPathHandler("/assets/", WebViewAssetLoader.AssetsPathHandler(context)).build()
                val onMain = android.os.Handler(android.os.Looper.getMainLooper())
                WebView(context).apply {
                    setBackgroundColor(AColor.TRANSPARENT)
                    settings.apply {
                        javaScriptEnabled = true
                        allowFileAccess = false
                        allowContentAccess = false
                        domStorageEnabled = false
                        blockNetworkLoads = true
                        mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
                        setSupportMultipleWindows(false)
                        setGeolocationEnabled(false)
                    }
                    val bridge = TermBridge(
                        onInput = feed::input,
                        onSize = feed::resize,
                        onLink = { url, domain -> onMain.post { onLink(url, domain) } },
                    )
                    addJavascriptInterface(JsHost(bridge), "adbuddy")
                    val sink = WebSink(this)
                    holder[0] = sink
                    webViewClient = object : WebViewClient() {
                        override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                            val url = request.url.toString()
                            return (if (TermWebPolicy.allowed(url)) loader.shouldInterceptRequest(request.url) else null) ?: blocked()
                        }

                        // The page never navigates: every attempt is consumed here.
                        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest) = true
                        override fun onPageFinished(view: WebView, url: String) = sink.loaded()

                        // Returning true keeps the app alive; this view cannot be used again, so a new one takes its place.
                        override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                            sink.die()
                            feed.detach(sink)
                            view.post { generation++ }
                            return true
                        }
                    }
                    loadUrl(TermWebPolicy.PAGE)
                }
            },
            update = { holder[0]?.setTheme(themeJs) },
            onRelease = { it.destroy() },
        )

        // The stream runs only while the view is on screen (protocol spec §7).
        LifecycleStartEffect(feed, generation) {
            val sink = holder[0]
            if (sink != null) feed.attach(sink)
            onStopOrDispose { if (sink != null) feed.detach(sink) }
        }
    }
}

private fun blocked() = WebResourceResponse("text/plain", "utf-8", 403, "Blocked", emptyMap(), ByteArrayInputStream(ByteArray(0)))
```

`onRenderProcessGone` devuelve `true` para que un fallo del proceso de renderizado no mate la app; la vista muerta se sustituye por otra nueva (`generation`) y `onRelease` destruye la vieja.

- [ ] **Paso 5: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`
Esperado: PASS / BUILD SUCCESSFUL. El APK de debug lleva los assets (`unzip -l app/build/outputs/apk/debug/app-debug.apk | grep assets/term`).

- [ ] **Paso 6: commit**

```bash
git add android
git commit -m "feat(app): vista de terminal con xterm.js en un WebView sin red"
```

---

### Tarea 9: pantalla de chat, actividad y avisos

**Ficheros:**
- Crea: `chat/Composer.kt`, `chat/SessionLabel.kt`, `ui/ChatScreen.kt`, `ui/ChatParts.kt`, `ui/ChatRoot.kt`, `ui/ActivityScreen.kt`, `ui/ConfirmHost.kt`
- Modifica: `chat/Links.kt` (T4: añade `find`), `chat/ChatController.kt` (añade `newConversation`), `ui/Screens.kt` (se va `HomeScreen`; se queda `ConsentScreen`), `MainActivity.kt`, `AdbuddyApp.kt` (el flujo `notices`), `AndroidManifest.xml` (solo `windowSoftInputMode` de `MainActivity`: **apúntalo en `COORDINACION.md`**), `res/values/strings.xml` y `values-es/strings.xml`
- Test: `test/.../chat/LinksTest.kt`, `ComposerTest.kt`, `SessionLabelTest.kt`, `ChatControllerTest.kt` (añade un caso)

**Interfaces:**
- Consume: todo lo de T2-T8 (`Timeline`, `ChatController`, `Drafts`, `Downloads`, `FileIntents`, `TerminalFeed`, `TerminalView`, `QuickKey`, `AdbuddyTheme`, `AdbuddyPreviews`), `LinkStatus`, `PauseButton`, `CutAccess`, `ConfirmUi`, `ConfirmDialog` (de `Screens.kt`).
- Produce:
  - `object Links { fun find(text: String): List<Span> }` con `data class Span(val start: Int, val end: Int, val url: String)`
  - `object Composer { const val MAX_TEXT_BYTES = 64 * 1024; const val MAX_ATTACHMENTS = 10; fun canSend(text: String, drafts: List<Draft>): Boolean }`
  - `enum class SessionLabel { OFFLINE, WORKING, WAITING, IDLE, STOPPED }` con `companion fun of(state: BridgeState): SessionLabel`
  - `ChatController.newConversation()`
  - `@Composable fun ChatRoot(app: AdbuddyApp, focusConfirm: String?, closed: Set<String>, onClose: (String) -> Unit)` (la usan `MainActivity` y, en T10, `ChatBubbleActivity`)
  - `AdbuddyApp.notices: SharedFlow<String>`

**Ruling de esta tarea:** las preguntas 🔴 siguen siendo el diálogo de M2 (`ConfirmDialog`), que ahora vive en `ConfirmHost` y se muestra sobre cualquier pantalla; la spec §7.2 dice "en línea (M2)" y M2 no las pone en la conversación. Si el usuario las quiere como tarjetas en el chat, es un cambio de M4 con los permisos del agente.

- [ ] **Paso 1: escribe los tests**

`LinksTest.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LinksTest {
    private fun urls(t: String) = Links.find(t).map { it.url }

    @Test fun findsWebLinksAndTrimsSentencePunctuation() {
        assertEquals(listOf("https://a.com/x"), urls("see https://a.com/x."))
        assertEquals(listOf("https://a.com"), urls("(https://a.com)"))
        assertEquals(listOf("https://en.wikipedia.org/wiki/A_(b)"), urls("https://en.wikipedia.org/wiki/A_(b)"))
        assertEquals(listOf("http://a.com/1", "https://b.org/2?q=1"), urls("http://a.com/1, then https://b.org/2?q=1!"))
    }

    @Test fun ignoresWhatIsNotAWebLink() {
        assertEquals(emptyList<String>(), urls("ftp://x.com file:///etc/passwd javascript:alert(1) http:// https://"))
    }

    @Test fun spansPointAtTheLinkInTheText() {
        val t = "go to https://a.com/x now"
        val s = Links.find(t).single()
        assertEquals("https://a.com/x", t.substring(s.start, s.end))
    }

    @Test fun aNonAsciiHostIsNeverShownAsAnAsciiOne() {
        // java.net.URI reads no host out of a non-ASCII name: the link is dropped, so a look-alike never gets a dialog.
        assertNull(Links.domain("https://ex\u0430mple.com/"))
    }

    @Test fun domainIsLowerCaseAndNullWithoutAHost() {
        assertEquals("example.com", Links.domain("https://Example.com/a"))
        assertNull(Links.domain("https://"))
        assertNull(Links.domain("mailto:a@b.c"))
    }
}
```

`ComposerTest.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus

class ComposerTest {
    private fun d(st: DraftStatus) = Draft("k", "a", "image/png", 1, 1, st, Attachment("0".repeat(32), "a", "image/png", 1, "x"))

    @Test fun needsTextOrAnAttachment() {
        assertFalse(Composer.canSend("", emptyList()))
        assertFalse(Composer.canSend("  \n ", emptyList()))
        assertTrue(Composer.canSend("hi", emptyList()))
        assertTrue(Composer.canSend("", listOf(d(DraftStatus.DONE))))
    }

    @Test fun waitsForEveryUploadAndRefusesAFailedOne() {
        assertFalse(Composer.canSend("hi", listOf(d(DraftStatus.DONE), d(DraftStatus.UPLOADING))))
        assertFalse(Composer.canSend("hi", listOf(d(DraftStatus.FAILED))))
    }

    @Test fun respectsTheProtocolLimits() {
        assertTrue(Composer.canSend("a".repeat(Composer.MAX_TEXT_BYTES), emptyList()))
        assertFalse(Composer.canSend("a".repeat(Composer.MAX_TEXT_BYTES + 1), emptyList()))
        assertFalse("é counts as 2 bytes", Composer.canSend("é".repeat(Composer.MAX_TEXT_BYTES / 2 + 1), emptyList()))
        assertTrue(Composer.canSend("", List(Composer.MAX_ATTACHMENTS) { d(DraftStatus.DONE) }))
        assertFalse(Composer.canSend("", List(Composer.MAX_ATTACHMENTS + 1) { d(DraftStatus.DONE) }))
        assertEquals(10, Composer.MAX_ATTACHMENTS)
    }
}
```

`SessionLabelTest.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.SessionState

class SessionLabelTest {
    private fun s(c: BridgeState.Connection, st: String) = BridgeState(connection = c, sessionState = SessionState(state = st))

    @Test fun theStateOnlyCountsWhileConnected() {
        assertEquals(SessionLabel.OFFLINE, SessionLabel.of(s(BridgeState.Connection.WAITING, "working")))
        assertEquals(SessionLabel.WORKING, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "working")))
        assertEquals(SessionLabel.WAITING, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "waiting")))
        assertEquals(SessionLabel.IDLE, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "idle")))
        assertEquals(SessionLabel.STOPPED, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "stopped")))
        assertEquals("an unknown state is not an alarm", SessionLabel.IDLE, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "???")))
    }
}
```

Caso nuevo en `ChatControllerTest.kt`:

```kotlin
    @Test fun aNewConversationAsksTheHostAndShowsItsRefusal() = runBlocking {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"Could not start the agent"}""", "r1")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { chat.errors.collect { seen += it } }
        chat.newConversation()
        job.cancel()
        assertEquals(Protocol.SESSION_NEW, ch.sent.single().type)
        assertEquals(listOf("Could not start the agent"), seen)
    }
```

- [ ] **Paso 2: comprueba que fallan** (no compilan)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.chat.*'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa la parte pura**

`chat/Links.kt` (añade al `object Links`):

```kotlin
    data class Span(val start: Int, val end: Int, val url: String)

    private val URL = Regex("https?://[^\\s<>\"]+")
    private val TRAILING = ".,;:!?'\""

    /** The web links of a text, in order (spec M3 §5.1: plain text with links). The sentence's own punctuation is not part of one. */
    fun find(text: String): List<Span> = URL.findAll(text).mapNotNull { m ->
        val start = m.range.first
        var end = m.range.last + 1
        while (end > start) {
            val c = text[end - 1]
            val unbalanced = (c == ')' && count(text, start, end, '(') < count(text, start, end, ')')) ||
                (c == ']' && count(text, start, end, '[') < count(text, start, end, ']')) ||
                (c == '}' && count(text, start, end, '{') < count(text, start, end, '}'))
            if (c in TRAILING || unbalanced) end-- else break
        }
        val url = text.substring(start, end)
        if (domain(url) == null) null else Span(start, end, url)
    }.toList()

    private fun count(t: String, from: Int, to: Int, ch: Char) = (from until to).count { t[it] == ch }
```

`chat/Composer.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus

/** When the Send button works (protocol spec §9 limits; spec M3 §6.4: sending waits for every upload). */
object Composer {
    const val MAX_TEXT_BYTES = 64 * 1024
    const val MAX_ATTACHMENTS = 10

    fun canSend(text: String, drafts: List<Draft>): Boolean {
        if (drafts.size > MAX_ATTACHMENTS || drafts.any { it.status != DraftStatus.DONE }) return false
        if (text.toByteArray().size > MAX_TEXT_BYTES) return false
        return text.isNotBlank() || drafts.isNotEmpty()
    }
}
```

`chat/SessionLabel.kt`:

```kotlin
package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.bridge.BridgeState

/** What the header says the agent is doing: only while the host is connected. */
enum class SessionLabel {
    OFFLINE, WORKING, WAITING, IDLE, STOPPED;

    companion object {
        fun of(state: BridgeState): SessionLabel = when {
            state.connection != BridgeState.Connection.CONNECTED -> OFFLINE
            else -> when (state.sessionState.state) {
                "working" -> WORKING
                "waiting" -> WAITING
                "stopped" -> STOPPED
                else -> IDLE
            }
        }
    }
}
```

`ChatController.kt` (añade):

```kotlin
    /** Starts another conversation (session.new); the host's refusal goes to `errors`. The new state arrives as session.state. */
    fun newConversation() {
        val payload = Protocol.json.encodeToJsonElement(SessionNew.serializer(), SessionNew())
        channel.request(Protocol.SESSION_NEW, payload) { m ->
            if (m != null && m.type == Protocol.ERROR) Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(it.text.ifEmpty { it.code }) }
        }
    }
```
(import `tech.hybriq.adbuddy.bridge.SessionNew`).

- [ ] **Paso 4: la capa de pantalla**

Los avisos del host. `AdbuddyApp.kt`:

```kotlin
    private val _notices = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The text of each `notice` of the host (spec M2 §4, pending of M2), for a snackbar: it is shown as the host wrote it. */
    val notices: SharedFlow<String> = _notices.asSharedFlow()
```
y en `onCreate`, tras crear `channel`: `channel.listen(Protocol.NOTICE) { m -> Protocol.payload<Notice>(m)?.text?.takeIf { it.isNotEmpty() }?.let(_notices::tryEmit) }`. (Los `error` de las peticiones los muestran los controladores que las hacen; no se escuchan aquí para no verlos dos veces.)

Mueve de `Screens.kt` (sin cambiar su lógica, solo de sitio y de nombre):
- el bloque de `HomeScreen` desde `var now by remember { mutableLongStateOf(clock.now()) }` hasta el final del `if (question != null) { … }` → `ui/ConfirmHost.kt`: `@Composable fun ConfirmHost(state: BridgeState, bridge: Bridge, clock: HostClock, focusConfirm: String?, closed: Set<String>, onClose: (String) -> Unit)`. `ConfirmDialog` y `ConfirmUi` no cambian.
- la lista del diario (filtros, `LazyColumn`, `EntryRow`, `levelIcon`) → `ui/ActivityScreen.kt`: `@Composable fun ActivityScreen(state: BridgeState, onBack: () -> Unit)`, con `Scaffold`, `TopAppBar` con flecha atrás (`Icons.AutoMirrored.Filled.ArrowBack`) y `@AdbuddyPreviews`. La pausa y Cortar acceso ya no están aquí: pasan al menú de la cabecera del chat.
- `openDeveloperOptions` pasa a `ChatRoot.kt`.
`HomeScreen` se borra; `ConsentScreen` se queda.

`ui/ChatParts.kt` — piezas sin estado propio (cada una con su `@AdbuddyPreviews`); la intención, que se ajusta a lo que compile:

```kotlin
/** Header: the agent, what it is doing, the link; terminal toggle and the menu. */
@Composable fun ChatHeader(state: BridgeState, cut: Boolean, terminalShown: Boolean, onToggleTerminal: () -> Unit,
                           onPause: (Boolean) -> Unit, onToggleCut: () -> Unit, onActivity: () -> Unit, onNewConversation: () -> Unit)
```
- Título: `state.sessionState.agent` o `R.string.app_name`. Subtítulo (`bodySmall`): el texto de `SessionLabel` (`R.string.session_working`, `_waiting`, `_idle`, `_stopped`, y para `OFFLINE` el de `LinkStatus.of(state, cut)`). Debajo, la línea de `LinkStatus` si no es `CONNECTED`.
- Acciones: `IconButton` que pliega la terminal (`KeyboardArrowUp`/`KeyboardArrowDown`), y menú `MoreVert` con: Pausar/Reanudar (`PauseButton.of(state)`, solo conectado), Cortar/Restaurar acceso, Actividad, Nueva conversación.

```kotlin
/** One message: mine to the right, the agent's to the left; text with links, attachment cards, and Retry for a failed one. */
@Composable fun MessageRow(item: TimelineItem, downloads: Map<String, DownloadState>, onLink: (String) -> Unit,
                           onAttachment: (Attachment) -> Unit, onRetry: (String) -> Unit, onDiscard: (String) -> Unit)

/** The text with its links as tappable spans (LinkAnnotation.Clickable; Links.find); a tap asks onLink(url), which shows the domain first. */
@Composable fun LinkedText(text: String, onLink: (String) -> Unit, modifier: Modifier = Modifier)

/** A file: name, size, thumbnail (the base64 JPEG of Attachment.thumb, decoded with BitmapFactory, never a remote image) and its state: tap to download / progress / tap to open. */
@Composable fun AttachmentCard(a: Attachment, state: DownloadState?, onTap: () -> Unit)

@Composable fun DraftChips(drafts: List<Draft>, onRemove: (String) -> Unit, onRetry: (String) -> Unit)

/** + (photo, file), screenshot, the text field (up to 5 lines) and Send; Send is `enabled = Composer.canSend(text, drafts)`. */
@Composable fun InputBar(text: String, onText: (String) -> Unit, drafts: List<Draft>, onPhoto: () -> Unit, onFile: () -> Unit,
                         onCapture: () -> Unit, onSend: () -> Unit)

/** Esc, Ctrl+C, ↑, ↓, Tab, Enter, 1, 2, 3 (QuickKey.entries), horizontally scrollable. */
@Composable fun QuickKeysRow(onKey: (QuickKey) -> Unit)

/** "Open <domain>?": the domain in bold and the full URL under it, Open and Cancel. */
@Composable fun LinkDialog(url: String, domain: String, onOpen: () -> Unit, onDismiss: () -> Unit)
```

`ui/ChatScreen.kt`:

```kotlin
/** What the screen can do; ChatRoot fills it from the app's controllers, a preview with empty lambdas. */
class ChatActions(
    val send: (String) -> Unit, val retry: (String) -> Unit, val discard: (String) -> Unit,
    val pickPhotos: () -> Unit, val pickFile: () -> Unit, val capture: () -> Unit,
    val removeDraft: (String) -> Unit, val retryDraft: (String) -> Unit,
    val attachment: (Attachment) -> Unit, val pause: (Boolean) -> Unit, val toggleCut: () -> Unit,
    val openActivity: () -> Unit, val newConversation: () -> Unit,
)

@Composable
fun ChatScreen(
    state: BridgeState, outgoing: List<Outgoing>, drafts: List<Draft>, downloads: Map<String, DownloadState>,
    cut: Boolean, terminal: TerminalFeed?, snackbar: SnackbarHostState, actions: ChatActions,
)
```
Diseño (spec §7.2): `Scaffold(topBar = ChatHeader, snackbarHost = SnackbarHost(snackbar))` y, dentro, `Column(Modifier.padding(inner).imePadding())`:
1. terminal arriba (`TerminalView(feed, onLink = { u, d -> link = u to d }, Modifier.weight(0.45f))`) mientras `terminalShown` (`rememberSaveable`, por defecto `true`) y `terminal != null` (las previews pasan `null`);
2. la conversación: `LazyColumn` con `Timeline.of(state.chat, state.sessionState.conversation, outgoing)`, claves `item.key`, y un `LaunchedEffect(items.size)` que baja al último;
3. si hay terminal a la vista, `QuickKeysRow { terminal.input(it.bytes) }`;
4. `DraftChips`;
5. `InputBar`; el texto es `rememberSaveable`; al enviar se vacía.
`LinkDialog` y el diálogo de "Nueva conversación" (confirmación) se muestran desde aquí con estado propio. Todo color sale de `MaterialTheme.colorScheme`: mensajes míos `primaryContainer`, del agente `surfaceVariant`, un envío fallido `errorContainer`.

`ui/ChatRoot.kt`:

```kotlin
/** The app's main screen: the chat, the activity panel, and the 🔴 questions over both. */
@Composable
fun ChatRoot(app: AdbuddyApp, focusConfirm: String?, closed: Set<String>, onClose: (String) -> Unit) {
    val context = LocalContext.current
    val state by app.bridge.state.collectAsStateWithLifecycle()
    val outgoing by app.chat.outgoing.collectAsStateWithLifecycle()
    val drafts by app.drafts.items.collectAsStateWithLifecycle()
    val downloads by app.downloads.states.collectAsStateWithLifecycle()
    val cut by remember(context) { CutAccess.observe(context) }.collectAsStateWithLifecycle(initialValue = CutAccess.isCut(context.contentResolver))
    val snackbar = remember { SnackbarHostState() }
    var showActivity by rememberSaveable { mutableStateOf(false) }
    // …
}
```
Lo que hace `ChatRoot` (cada punto es un comportamiento que el revisor comprueba):
- **Avisos:** un `LaunchedEffect(Unit)` que une `app.notices`, `app.chat.errors` y `app.drafts.errors` (`merge`) y muestra cada texto en el `snackbar`; el de "no se pudo añadir el fichero" (texto vacío de `attachUri`) usa `R.string.attach_failed`.
- **Pickers sin permisos:** `rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(Composer.MAX_ATTACHMENTS))` para fotos y `OpenDocument()` (`arrayOf("*/*")`) para ficheros; cada `Uri` → `app.attachUri(uri) { text -> scope.launch { snackbar.showSnackbar(text.ifEmpty { attachFailed }) } }`.
- **Enviar:** `app.chat.send(text, drafts.mapNotNull { it.attachment })` y, después, `app.drafts.clear()`.
- **Descarga automática:** `LaunchedEffect(state.chat.messages)`: por cada adjunto de un mensaje del agente con `Downloads.autoLimit(a, remote = state.remote.enabled)`, `app.downloads.get(a)`. Pulsar un adjunto: sin descarga, `app.downloads.get(a)`; en curso, `app.downloads.cancel(a.id)`; hecha, `FileIntents.view(...)` y, si ninguna app lo abre, `FileIntents.share(...)`.
- **Cortar acceso:** como estaba en `HomeScreen` (`CutAccess.cut`/`restore`, los mismos mensajes `cut_done`, `cut_failed`…), pero con los avisos por el `snackbar` y la salida a `openDeveloperOptions` como acción del snackbar.
- **Navegación:** `showActivity` alterna `ChatScreen` y `ActivityScreen`; `BackHandler(showActivity) { showActivity = false }`.
- **Preguntas 🔴:** `ConfirmHost(state, app.bridge, app.clock, focusConfirm, closed, onClose)` después de las dos pantallas, así que salen estés donde estés.

`MainActivity.kt`: `setContent { AdbuddyTheme { if (showConsent) ConsentScreen(...) else ChatRoot(app, focusConfirm, closed) { closed = closed + it } } }`. En el manifiesto, `android:windowSoftInputMode="adjustResize"` en `MainActivity`.

Cadenas nuevas (inglés; el español va en `values-es`): `session_working` "Working", `session_waiting` "Waiting for you", `session_idle` "Ready", `session_stopped` "Stopped", `chat_hint` "Message the agent", `chat_send`, `chat_attach`, `chat_photo` "Photo", `chat_file` "File", `chat_capture` "Screenshot", `chat_terminal_show`/`hide`, `chat_retry`, `chat_discard`, `chat_not_sent` "Not sent", `chat_sending` "Sending…", `menu_activity`, `menu_new_conversation`, `new_conversation_title`/`_body`/`_confirm`, `attach_failed` "The file could not be added", `draft_failed`, `link_open_title` "Open %1$s?", `link_open`, `file_tap_download`, `file_downloading`, `file_open`. Los títulos no repiten la información que ya da el contenido.

- [ ] **Paso 5: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`
Esperado: PASS / BUILD SUCCESSFUL. Además: `grep -rn "Color(" android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/` solo encuentra `Palette`/`Theme` (los colores fijos están en `ui/theme/`), y `grep -rn "Log\\." android/app/src/main/kotlin/tech/hybriq/adbuddy/{chat,files,term,ui}` no encuentra nada.

- [ ] **Paso 6: commit**

```bash
git add android
git commit -m "feat(app): pantalla de chat con terminal, adjuntos y descargas; actividad aparte"
```

---

### Tarea 10: notificación de conversación, burbuja y aviso "Listo"

**Ficheros:**
- Crea: `notify/DoneDetector.kt`, `notify/ConversationView.kt`, `notify/ConversationNotifier.kt`, `ChatBubbleActivity.kt`
- Modifica: `bridge/BridgeService.kt`, `AdbuddyApp.kt` (canales), `AndroidManifest.xml` (la actividad de la burbuja: **apúntalo en `COORDINACION.md`**), `res/values/strings.xml` y `values-es/strings.xml`
- Test: `test/.../notify/DoneDetectorTest.kt`, `ConversationViewTest.kt`

**Interfaces:**
- Consume: `BridgeState`, `StatusView`/`LinkStatus` (M2), `ChatController.send`, `Captures.take`, `CutAccess`, `ChatRoot` (T9), `AdbuddyTheme`.
- Produce:
  - `class DoneDetector(minWorkMs: Long = 10_000)`: `fun onState(s: SessionState, nowMs: Long): Boolean`, `fun reset()`.
  - `data class LastMessage(val id: String, val fromUser: Boolean, val text: String)`, `data class Headline(@StringRes val res: Int, val arg: String? = null)`, `data class ConversationView(val status: StatusView, val agent: String, val state: String, val activity: String, val last: LastMessage?)` con `fun headline(): Headline` y `companion fun of(state: BridgeState, cut: Boolean): ConversationView`.
  - `object Notify { fun doneLine(chat: ChatState, conversation: String): String? }`.
  - `object ConversationNotifier { fun build(ctx: Context, v: ConversationView, ts: Long): Notification; fun done(ctx: Context, line: String?) }`.

- [ ] **Paso 1: escribe los tests**

`DoneDetectorTest.kt`:

```kotlin
package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.SessionState

class DoneDetectorTest {
    private val d = DoneDetector()
    private fun st(s: String) = SessionState(state = s)

    @Test fun workEndingAfterTenSecondsIsNews() {
        assertFalse(d.onState(st("working"), 0))
        assertFalse(d.onState(st("working"), 5_000))
        assertTrue(d.onState(st("waiting"), 12_000))
    }

    @Test fun shortWorkIsNot() {
        d.onState(st("working"), 0)
        assertFalse(d.onState(st("waiting"), 9_999))
    }

    @Test fun idleEndsItToo_andTenSecondsExactlyCounts() {
        d.onState(st("working"), 0)
        assertTrue(d.onState(st("idle"), 10_000))
    }

    @Test fun workCountsFromItsFirstWorkingState() {
        d.onState(st("working"), 0)
        d.onState(st("working"), 8_000)
        assertTrue(d.onState(st("waiting"), 11_000))
    }

    @Test fun onlyOnceAndNotWithoutWork() {
        d.onState(st("working"), 0)
        assertTrue(d.onState(st("waiting"), 20_000))
        assertFalse(d.onState(st("waiting"), 30_000))
        assertFalse(d.onState(st("idle"), 40_000))
    }

    @Test fun aStoppedAgentOrAResetForgetsTheWork() {
        d.onState(st("working"), 0)
        assertFalse(d.onState(st("stopped"), 5_000))
        assertFalse(d.onState(st("waiting"), 20_000))
        d.onState(st("working"), 30_000)
        d.reset()
        assertFalse(d.onState(st("waiting"), 50_000))
    }
}
```

`ConversationViewTest.kt`:

```kotlin
package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.SessionState

class ConversationViewTest {
    private fun msg(id: String, role: String, text: String, conv: String = "c_1", atts: List<Attachment> = emptyList()) =
        ChatMessage(id, role, text, atts, conv)

    private fun state(st: String = "working", activity: String = "Open Settings", msgs: List<ChatMessage> = emptyList(),
                      conn: BridgeState.Connection = BridgeState.Connection.CONNECTED) =
        BridgeState(connection = conn, hostName = "PC", sessionState = SessionState(agent = "claude", state = st, activity = activity, conversation = "c_1"),
            chat = ChatState(messages = msgs))

    @Test fun workingShowsWhatTheAgentIsDoing() {
        val v = ConversationView.of(state(), cut = false)
        assertEquals(Headline(R.string.notif_working_on, "Open Settings"), v.headline())
        assertEquals(Headline(R.string.notif_working), ConversationView.of(state(activity = ""), cut = false).headline())
        assertEquals("activity is only news while working", "", ConversationView.of(state("waiting"), cut = false).activity)
        assertEquals(Headline(R.string.notif_waiting), ConversationView.of(state("waiting"), cut = false).headline())
        assertEquals(Headline(R.string.notif_stopped), ConversationView.of(state("stopped"), cut = false).headline())
    }

    @Test fun withoutAConnectionTheLinkStatusSpeaks() {
        val v = ConversationView.of(state(conn = BridgeState.Connection.WAITING), cut = false)
        assertEquals(Headline(R.string.status_waiting), v.headline())
        assertEquals(Headline(R.string.status_cut), ConversationView.of(state(), cut = true).headline())
    }

    @Test fun theLastMessageOfTheCurrentConversation() {
        val msgs = listOf(msg("a", "user", "hi"), msg("b", "assistant", "hello", conv = "c_0"), msg("c", "assistant", "done", conv = ""))
        val last = ConversationView.of(state(msgs = msgs), cut = false).last!!
        assertEquals(LastMessage("c", fromUser = false, text = "done"), last)
        assertNull(ConversationView.of(state(msgs = listOf(msg("x", "user", "old", conv = "c_0"))), cut = false).last)
    }

    @Test fun anAttachmentOnlyMessageShowsItsFileNames_andLongTextIsCut() {
        val a = Attachment("0".repeat(32), "photo.png", "image/png", 1, "x")
        assertEquals("photo.png", ConversationView.of(state(msgs = listOf(msg("a", "assistant", "", atts = listOf(a)))), cut = false).last!!.text)
        val long = ConversationView.of(state(msgs = listOf(msg("a", "assistant", "x".repeat(1000)))), cut = false).last!!.text
        assertEquals(ConversationView.MAX_TEXT, long.length)
        assertTrue(long.endsWith("…"))
    }

    @Test fun equalStatesGiveEqualViews_soTheNotificationIsNotPostedAgain() {
        assertEquals(ConversationView.of(state(), cut = false), ConversationView.of(state(), cut = false))
    }

    @Test fun doneLineIsTheFirstLineOfTheLastAnswer() {
        val chat = ChatState(messages = listOf(msg("a", "assistant", "\n\n  Fixed the Wi-Fi toggle.\nMore detail"), msg("b", "user", "thanks")))
        assertEquals("Fixed the Wi-Fi toggle.", Notify.doneLine(chat, "c_1"))
        assertNull(Notify.doneLine(ChatState(messages = listOf(msg("b", "user", "thanks"))), "c_1"))
        assertNull(Notify.doneLine(chat, "c_2"))
        assertEquals(120, Notify.doneLine(ChatState(messages = listOf(msg("a", "assistant", "y".repeat(500)))), "")!!.length)
    }
}
```

- [ ] **Paso 2: comprueba que fallan** (no compilan)

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.notify.*'`
Esperado: FAIL de compilación.

- [ ] **Paso 3: implementa la parte pura**

`notify/DoneDetector.kt`:

```kotlin
package tech.hybriq.adbuddy.notify

import tech.hybriq.adbuddy.bridge.SessionState

/**
 * "Done" (spec M3 §7.4): the agent goes from working to waiting or idle after
 * at least minWorkMs of work. Pure: the caller gives the time.
 */
class DoneDetector(private val minWorkMs: Long = 10_000) {
    private var since: Long? = null

    /** True when this state ends a stretch of work long enough (once). */
    fun onState(s: SessionState, nowMs: Long): Boolean = when (s.state) {
        "working" -> {
            if (since == null) since = nowMs
            false
        }
        "waiting", "idle" -> {
            val t = since
            since = null
            t != null && nowMs - t >= minWorkMs
        }
        else -> { // stopped or unknown: whatever was going on is over
            since = null
            false
        }
    }

    /** The connection was lost: what was seen no longer counts. */
    fun reset() { since = null }
}
```

`notify/ConversationView.kt`:

```kotlin
package tech.hybriq.adbuddy.notify

import androidx.annotation.StringRes
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.StatusView

data class LastMessage(val id: String, val fromUser: Boolean, val text: String)

/** A string resource and its one argument, so the text is chosen where there is a Context and tested here without one. */
data class Headline(@param:StringRes val res: Int, val arg: String? = null)

/**
 * Everything the conversation notification shows. A data class so the service
 * posts the notification again only when it changes (like StatusView in M2).
 */
data class ConversationView(
    val status: StatusView,
    val agent: String,
    val state: String,
    val activity: String,
    val last: LastMessage?,
) {
    fun headline(): Headline = when {
        status.status != LinkStatus.CONNECTED -> Headline(status.status.text)
        state == "working" && activity.isNotEmpty() -> Headline(R.string.notif_working_on, activity)
        state == "working" -> Headline(R.string.notif_working)
        state == "waiting" -> Headline(R.string.notif_waiting)
        state == "stopped" -> Headline(R.string.notif_stopped)
        else -> Headline(R.string.status_connected, status.host)
    }

    companion object {
        const val MAX_TEXT = 300

        fun of(s: BridgeState, cut: Boolean): ConversationView {
            val conv = s.sessionState.conversation
            val m = s.chat.messages.lastOrNull { conv.isEmpty() || it.conversation.isEmpty() || it.conversation == conv }
            val text = m?.let { it.text.ifEmpty { it.attachments.joinToString { a -> a.name } } }
            val last = if (m == null || text.isNullOrEmpty()) null else LastMessage(m.id, m.role == "user", cutText(text))
            val working = s.sessionState.state == "working"
            return ConversationView(StatusView.of(s, cut), s.sessionState.agent, s.sessionState.state, if (working) s.sessionState.activity else "", last)
        }

        private fun cutText(t: String) = if (t.length <= MAX_TEXT) t else t.take(MAX_TEXT - 1) + "…"
    }
}

object Notify {
    private const val MAX_LINE = 120

    /** The first line of the agent's last answer in this conversation, for "Done"; null when it said nothing. */
    fun doneLine(chat: ChatState, conversation: String): String? {
        val m = chat.messages.lastOrNull {
            it.role == "assistant" && (conversation.isEmpty() || it.conversation.isEmpty() || it.conversation == conversation)
        } ?: return null
        val line = m.text.lineSequence().map { it.trim() }.firstOrNull { it.isNotEmpty() } ?: return null
        return if (line.length <= MAX_LINE) line else line.take(MAX_LINE)
    }
}
```

- [ ] **Paso 4: la capa de Android**

Cadenas (inglés; español en `values-es`): `channel_conversation` "Conversation with the agent", `channel_done` "Agent finished", `notif_working` "Working…", `notif_working_on` "Working: %1$s", `notif_waiting` "Waiting for you", `notif_stopped` "The agent stopped", `notif_you` "You", `notif_reply` "Reply", `notif_reply_hint` "Message the agent", `notif_capture` "Send screenshot", `notif_done_title` "Done", `notif_done_default` "The agent finished", `notif_capture_failed`, `notif_cut_failed`. (`headline.res` con `arg` se formatea con `getString(res, arg)`; sin `arg`, `getString(res)`.)

`AdbuddyApp.kt`: el canal de la notificación fija pasa a ser el de conversación. **Un canal no cambia sus ajustes una vez creado** y el usuario ya tiene instalada una app con el canal `status`: se crea uno nuevo y se borra el viejo.

```kotlin
    companion object {
        /** The conversation notification (replaces "status"): bubbles allowed, low importance, never a sound. */
        const val CHANNEL_CONVERSATION = "conversation"
        const val CHANNEL_DONE = "done"
        const val CHANNEL_CONFIRM = "confirm"
        private const val LEGACY_STATUS = "status"
    }
```
y en `onCreate`:

```kotlin
        nm.deleteNotificationChannel(LEGACY_STATUS)
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONVERSATION, getString(R.string.channel_conversation), NotificationManager.IMPORTANCE_LOW).apply { setAllowBubbles(true) })
        nm.createNotificationChannel(NotificationChannel(CHANNEL_DONE, getString(R.string.channel_done), NotificationManager.IMPORTANCE_DEFAULT))
```
(`grep -rn CHANNEL_STATUS android/` y cámbialo todo a `CHANNEL_CONVERSATION`.)

`notify/ConversationNotifier.kt` — lo que hace, punto por punto (cada uno lo comprueba el revisor leyendo el código; no hay test de JVM de Android):
- `build(ctx, v, ts)`: `MessagingStyle(me)` con `Person` "Tú" y `Person` del agente (`key = SHORTCUT_ID`, `setImportant(true)`); un solo mensaje: el último (`v.last`) con `ts`, o, si no hay, el titular (`headline`) como mensaje del agente. `setSubText(titular)`, `setCategory(CATEGORY_MESSAGE)`, `setShortcutId` y `setLocusId`, `setOngoing(true)`, `setOnlyAlertOnce(true)`, `setContentIntent(abrir MainActivity)`.
- **Acceso directo de conversación:** antes de notificar, `ShortcutManagerCompat.pushDynamicShortcut` con `ShortcutInfoCompat.Builder(ctx, SHORTCUT_ID)` (`setLongLived(true)`, `setIsConversation()`, `setPerson(agente)`, `setLocusId`, un intent que abre `MainActivity`).
- **Burbuja:** `NotificationCompat.BubbleMetadata.Builder(pendingIntent, icon)` con `PendingIntent.getActivity(ctx, 5, Intent(ctx, ChatBubbleActivity::class.java), FLAG_MUTABLE or FLAG_UPDATE_CURRENT)`, `setDesiredHeight(600)`, icono `IconCompat.createWithResource(ctx, android.R.drawable.sym_action_chat)`.
- **Privacidad en la pantalla de bloqueo:** `setVisibility(VISIBILITY_PRIVATE)` y una `setPublicVersion` con solo el titular sin el texto del último mensaje.
- **Acciones** (todas a `BridgeService` con `PendingIntent.getService`, un `requestCode` distinto cada una):
  - **Responder:** `RemoteInput` (`KEY_REPLY = "reply"`), `PendingIntent` con `FLAG_MUTABLE` (el sistema rellena la respuesta), `setSemanticAction(SEMANTIC_ACTION_REPLY)`, `setAllowGeneratedReplies(false)` y **`setAuthenticationRequired(true)`**: desde la pantalla de bloqueo no se inyecta nada al agente sin desbloquear.
  - **Enviar captura:** `ACTION_CAPTURE`, también con `setAuthenticationRequired(true)`.
  - **Pausar/Reanudar:** como en M2 (`PauseButton`).
  - **Android 11 (API 30):** `setAuthenticationRequired` solo existe desde la 31. `BridgeService` ignora `ACTION_REPLY` y `ACTION_CAPTURE` si `KeyguardManager.isDeviceLocked`, en todas las versiones: es la defensa real.
  - **Cortar acceso:** `ACTION_CUT`, **sin** exigir desbloqueo (recorta lo que el agente puede hacer, así que es siempre seguro y urgente).
- `done(ctx, line)`: canal `CHANNEL_DONE`, título `notif_done_title`, texto `line ?: notif_done_default`, `setAutoCancel(true)`, abre `MainActivity`; se publica con `NotificationManager.notify` (no con `NotificationManagerCompat.notify`: `lintDebug` da `MissingPermission`) y solo si `NotificationManagerCompat.from(ctx).areNotificationsEnabled()`.
- El acceso directo se publica una sola vez por nombre de agente (`@Volatile private var pushedFor`), no en cada notificación.

`ChatBubbleActivity.kt`:

```kotlin
package tech.hybriq.adbuddy

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import tech.hybriq.adbuddy.ui.ChatRoot
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The chat inside a bubble (Bubbles API, spec M3 §7.4). Opens no consent screen: that is MainActivity's. */
class ChatBubbleActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val app = application as AdbuddyApp
        setContent { AdbuddyTheme { ChatRoot(app, focusConfirm = null, closed = emptySet(), onClose = {}) } }
    }
}
```

`AndroidManifest.xml` (junto a `MainActivity`):

```xml
        <!-- The bubble of the conversation notification: embeddable and resizable, as the Bubbles API asks. -->
        <activity
            android:name=".ChatBubbleActivity"
            android:exported="false"
            android:allowEmbedded="true"
            android:resizeableActivity="true"
            android:documentLaunchMode="always"
            android:windowSoftInputMode="adjustResize" />
```

`bridge/BridgeService.kt` — cambios:
- `shownStatus: StatusView?` → `shown: ConversationView?`; `statusNotification(v)` → `ConversationNotifier.build(this, v, ts)`, con `lastId`/`lastTs` (la hora a la que se vio por primera vez el `last.id`, `System.currentTimeMillis()`).
- El `combine(app.bridge.state, CutAccess.observe(this))` pasa a `ConversationView.of(s, cut)`; se vuelve a notificar solo si `v != shown`.
- **Aviso "Listo":** otro `launch` que recoge `app.bridge.state` con un `DoneDetector`: con `connection != CONNECTED` llama a `reset()`; si no, `if (detector.onState(s.sessionState, SystemClock.elapsedRealtime())) ConversationNotifier.done(this, Notify.doneLine(s.chat, s.sessionState.conversation))`.
- `onStartCommand`, casos nuevos:
  - `ACTION_REPLY` (y `ACTION_CAPTURE`) se ignoran con el móvil bloqueado. `ACTION_REPLY`: `RemoteInput.getResultsFromIntent(intent)?.getCharSequence(ConversationNotifier.KEY_REPLY)?.toString()` → `app.chat.send(text)`, y **vuelve a publicar la notificación al momento** (si no, la rueda de "enviando" de la respuesta directa no se quita hasta el siguiente cambio de estado).
  - `ACTION_CAPTURE`: `app.captures.take { a, text -> if (a != null) app.chat.send("", listOf(a)) else toast(text) }`.
  - `ACTION_CUT`: `if (!CutAccess.cut(this)) toast(R.string.notif_cut_failed)`.
  - (`toast` publica en el hilo principal con un `Handler`.)

- [ ] **Paso 5: pasan los tests y compila**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`
Esperado: PASS / BUILD SUCCESSFUL (`LinkStatusTest` sigue pasando: `StatusView` no cambia).

- [ ] **Paso 6: commit**

```bash
git add android
git commit -m "feat(app): notificación de conversación con respuesta directa, burbuja y aviso Listo"
```

---

### Tarea 11: documentación, pendientes y cierre

**Ficheros:**
- Modifica: `android/README.md`, `docs/superpowers/plans/pendientes-m3-m5.md` (solo la sección M3), este plan (la lista de validaciones, abajo, ya está al final)

- [ ] **Paso 1: `android/README.md`**: sustituye la frase "The chat arrives in M3." por lo que hay (chat con terminal, adjuntos, descargas, captura, notificación de conversación, burbuja, aviso "Listo"), y añade una sección **"Chat and terminal"** con: de dónde salen xterm.js y sus versiones (`assets/term/VENDOR.md`), que el WebView no sale a la red y qué métodos tiene el puente, el límite de 200 MiB, la caché de 500 MiB, que el chat y la terminal no escriben nada en ningún log, y que la respuesta directa de la notificación es un `RemoteInput`: lo pueden disparar los servicios de escucha de notificaciones y los relojes que el usuario haya autorizado (se anota también en la spec §10).
- [ ] **Paso 2: `pendientes-m3-m5.md`** (solo M3): marca como hecho **"La app muestra los `notice` del host"**, **"Diseño de la app"** (con su skill) y **`session.state.activity`** (T1); y apunta lo aplazado de M3b:
  - la etiqueta "red local o Tailscale" de la cabecera y el límite de 5 MiB en modo remoto (M5b, con `RemoteState`);
  - las preguntas 🔴 como tarjetas en el chat (M4, con los permisos del agente);
  - `onRenderProcessGone` solo evita el cierre: la terminal queda en blanco hasta recomponer;
  - las descargas escriben en el hilo del lector de OkHttp (256 KiB por trama): si se ve lag en los pings con almacenamiento lento, pasarlas a un escritor aparte;
  - `Busy` y los diálogos de permisos de Codex/Gemini (ya anotado para M4);
  - si Android no abre como burbuja la notificación de un servicio en primer plano (riesgo del plan), la notificación de conversación aparte (id 4).
- [ ] **Paso 3: comprobación final**

Run: `cd android && C:/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug` y `cd host && go vet ./... && go test ./internal/cli -timeout 5m`.
Esperado: todo en verde. `git grep -nE "[0-9]{1,3}(\.[0-9]{1,3}){3}|emulator-|[0-9A-Z]{10,}" -- docs android | grep -v "127.0.0.1\|0.0.0.0"` revisado a mano (seriales e IPs) antes de cualquier push.

- [ ] **Paso 4: commit**

```bash
git add android/README.md docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "docs(m3b): README de la app y pendientes tras M3b"
```

- [ ] **Paso 5: cierre de M3** (no es de este plan: lo hace el controlador): revisión final con `adbuddy-reviewer-opus` de toda la rama frente a la spec, PR, **ok del supervisor antes de cualquier push**, y las validaciones manuales de abajo en el Pixel del usuario, con su permiso explícito.

---

## Validaciones manuales pendientes

En el Pixel del usuario, con su permiso explícito para cada una (nada de esto se prueba sin que lo pida). Se suman a las del plan de M3a (al final de `2026-10-01-adbuddy-m3a-host.md`) y a las de la spec M3 §11.

1. **Chat con Claude Code:** un mensaje con varias líneas y Unicode llega como un solo mensaje y aparece a los dos lados; cortar el Wi-Fi, enviar (queda "No enviado") y Reintentar no lo duplica.
2. **Terminal:** colores y repintado de la TUI; teclas rápidas (Esc, ↑/↓, Enter, 1/2/3 en un diálogo de permisos de Claude); girar el móvil; volver tras perder la conexión (pantalla actual, sin basura); con un PC enganchado, scroll horizontal y el tamaño del PC; un enlace de la terminal pide confirmar el dominio.
3. **Ficheros:** una foto y un fichero de ida (a `inbox/`, con hash comprobado); un fichero de ~100 MiB que se corta y sigue desde donde iba; `adbuddy send` de vuelta (una imagen baja sola, un PDF al pulsar; abrir y compartir); la caché no pasa de 500 MiB.
4. **Captura:** desde el botón y desde la notificación; con "Cortar acceso" activo, el texto del host.
5. **Notificación:** estado "Trabajando: …" con la última orden; responder desde la notificación (en pantalla bloqueada pide desbloquear); Enviar captura; Pausar; Cortar acceso (sin desbloquear); **abrir como burbuja** (riesgo anotado en las decisiones: si no abre, la notificación de conversación aparte); aviso "Listo" tras ≥ 10 s de trabajo y silenciar su canal.
6. **Reconexión** con la app en segundo plano y con el servicio recién reiniciado.
7. **Diseño:** todas las pantallas en claro, oscuro y letra grande en Android 17 (color dinámico); en Android 11 (paleta propia) si hay un móvil o, si no, en un emulador cuando se pueda (M2.5b); nada bajo las barras del sistema; el campo de texto sube con el teclado.
8. **Nada de M2 se debilita:** la confirmación 🔴 sale en el chat y desde la notificación; Pausar y Cortar acceso funcionan desde el menú y la notificación; el texto del chat no llega al shim. **Con la burbuja expandida sobre otra app y un 🔴 en ella** (revisión final): `input tap` y `uiautomator dump` del agente quedan bloqueados por la guarda de M5 (`guardedOnScreen` del servicio de accesibilidad y `Resumed()` de dumpsys), con el servicio de accesibilidad activado y desactivado; si alguna capa no ve ADBuddy delante, un toque del agente podría pulsar Permitir.
9. **Diálogo de confianza de Claude Code:** con él en pantalla, `chat.send` da "busy" con el texto del host.
10. **WebView:** sin conexión a internet (modo avión con el adb por Wi-Fi local) la terminal sigue funcionando; nada pide red.

---

## Ejecución

1. **Antes de la tarea 1:** revisión de este plan con `adbuddy-reviewer-opus` (código incluido, frente a la spec M3 y a la del protocolo) y corrección con sus hallazgos; **avisar al supervisor antes de lanzar opus** (`adbuddy-e5` decide el presupuesto) y, hasta que lo permita, solo se trabaja en la sesión.
2. **Ledger:** `.superpowers/sdd/2026-10-01-adbuddy-m3b/progress.md`, con cada decisión como `Ruling: qué — por qué — qué cuesta si me equivoco`. Los briefs por tarea se extraen a mano con Python (`task-brief` no sirve con "Tarea N"), como en M3a: `.superpowers/sdd/2026-10-01-adbuddy-m3a/task-N-brief.md`.
3. **Orden:** T1 (host) y T2-T8 no dependen entre sí salvo lo declarado en cada **Interfaces**: T3 antes de T4-T6 y T9; T5 antes de T9 y T10; T4 antes de T8; T7 antes de T8 y T9; T8 antes de T9. Orden de ejecución recomendado: T1, T2, T3, T4, T5, T6, T7, T8, T9, T10, T11. Un subagente a la vez; `adbuddy-implementer` (Sonnet) para todas; `adbuddy-implementer-opus` solo si el supervisor lo permite en las rondas 4-5.
4. **Revisores:** `adbuddy-reviewer` (Sonnet) por tarea; `adbuddy-reviewer-opus` para T1 (host) y T10 (acciones de la notificación que inyectan texto al agente) si el presupuesto lo permite, y para la revisión final. Con límites de tiempo: `go test -timeout 5m`; `gradle --no-daemon` con tope de 15 minutos.
5. **CI:** subir pronto a la CI de ubuntu lo que tenga concurrencia (T1 y los controladores T3-T6), porque `-race` no corre aquí. Push solo de ramas como copia y con el ok del supervisor; a `main` integra el usuario.
