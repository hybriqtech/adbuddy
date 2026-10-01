# ADBuddy m3-bridge · Arreglos del canal que M5 necesita · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Objetivo:** cerrar, antes que el resto de M3, los aplazados de M3.0 en el canal de los que dependen M3 y M5 (`pendientes-m3-m5.md`, sección M3):
- en la app, que ningún oyente ni `onResult` pueda cortar el canal;
- que una respuesta solo resuelva peticiones de su socket;
- un `reset` por área para cuando el host reinicia;
- en el host, `SendTo`: un flujo atado a la conexión que lo pidió.

**Arquitectura:** rama `m3-bridge`, que sale de `main`. Dos tareas independientes y en árboles disjuntos (app y host), que pueden ir en paralelo.

**Tecnologías:** las de M3.0. Nada nuevo en `go.mod` ni en `libs.versions.toml`.

**Spec:** `docs/superpowers/specs/2026-10-01-adbuddy-protocolo-m3-m5-design.md` (§3, §6.2, §7 y §10). Aplazados: `docs/superpowers/plans/pendientes-m3-m5.md` (sección M3, "Del protocolo común").

**Verificación del plan:**
- Todo el código sale de un prototipo que pasa:
  - en el host, `go test ./...`, `go vet` y `gofmt`;
  - en la app, `:app:testDebugUnitTest :app:assembleDebug :app:lintDebug`, con 130 tests.
- Se reprodujo tarea a tarea sobre `main` (0ec286f): cada tarea pasa sola y el resultado es idéntico al prototipo.
- Se comprobó que los tests detectan que falta la protección:
  - sin la protección de los oyentes, falla `aThrowingCallbackDoesNotCutTheChannel`;
  - sin la de `failPending`, fallan `stopFailsThePendingRequests` y `aThrowingOnResultDoesNotStopTheReconnection`, en todas las ejecuciones.
- El plan pasó una revisión de `adbuddy-reviewer-opus` (listo con correcciones). Sus dos importantes están incorporados:
  - un test que no dependa del orden de las peticiones pendientes;
  - `reset` con motivo.
- **Si un paso no sale como dice el plan, detente y explícalo.**

## Restricciones globales

- **Rama `m3-bridge`.** No se integra en `main` sin el ok del supervisor (adbuddy-e5); nunca `push --force`.
- **Go:** `"/c/Program Files/Go/bin/go.exe"` y `"/c/Program Files/Go/bin/gofmt.exe"` (en los pasos, `go` y `gofmt`). Los tests, siempre con `-timeout` y `-count=1`.
- **Gradle:** `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon` (en los pasos, `gradle`) con `JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'`.
- **Ficheros con LF.** Los parches de paso (`mb_*.patch`) se crean con la herramienta Write en la raíz, se aplican con `git apply --whitespace=nowarn` y se borran. Un `git apply` que falla quiere decir que el fichero no está como el plan espera: detente.
- **Commits:** identidad local del repo (hybriqtech); cada mensaje termina con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, exactamente así; `git add` con rutas explícitas.
- **No cambies el protocolo** (`protocol*.go`, `Protocol*.kt` ni `protocol/fixtures`) ni nada fuera de las rutas de tu tarea: lo comparten otras sesiones.
- **El móvil del usuario no se toca.** **Sin subagentes** dentro de una tarea.

## Orden y revisores

| Tarea | Árbol | Depende de | Revisor |
|---|---|---|---|
| 1 · App: callbacks aislados, `pending` por socket y `Reducer.reset` | `android/app/src/{main,test}/kotlin/tech/hybriq/adbuddy/bridge/` | – | `adbuddy-reviewer-opus` (canal) |
| 2 · Host: `SendTo` | `host/internal/bridge/server.go`, `send_test.go` | – | `adbuddy-reviewer-opus` (canal) |

---

### Tarea 1: App: callbacks aislados, `pending` por socket y `Reducer.reset`

**Ficheros:**
- Modificar: en `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/`, `Bridge.kt`, `BridgeState.kt`, `Reducer.kt`, `ConfirmReducer.kt`, `ChatReducer.kt` y `RemoteReducer.kt`
- Modificar (tests): en `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/`, `BridgeTest.kt` y `BridgeStateTest.kt`

**Interfaces:**
- **`Reducer.reset(s: BridgeState, why: Reset): BridgeState`, sin implementación por defecto.** `enum class Reset` tiene cuatro valores: `NEW_SESSION`, `UNREADABLE_SNAPSHOT`, `DISCONNECTED` e `INCOMPATIBLE`.
- **`BridgeState` la llama en los cuatro caminos que antes limpiaban a mano:** welcome de sesión nueva, `snapshot` ilegible, `disconnected()`, y `appOutdated()` o un welcome incompatible. Así M4 no tendrá que tocar `BridgeState`.
- **Qué olvida cada reductor.** El comportamiento de M2 no cambia.
  - `ConfirmReducer`:
    - sesión nueva: `sessionState`, `pending` y `answering`;
    - snapshot ilegible o incompatible: `pending` y `answering`;
    - desconexión: `answering`.
  - `ChatReducer` olvida `chat`, y `RemoteReducer`, `remote`, con una sesión nueva o un snapshot ilegible.
- **`Bridge`:**
  - ejecuta cada oyente, cada oyente de tramas, cada `onResult` y el `onMessage` del constructor dentro de `safely`: una excepción ni corta el socket ni para el bucle, ni impide llamar a los demás, y no se registra en ningún log;
  - `onMessage` resuelve `pending[rid]` solo si la petición salió por ese socket.

- [ ] **Paso 1: Escribir los tests que fallan.** Aplica:

```diff
diff --git a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
index ed82aaf..50ab54c 100644
--- a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
+++ b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
@@ -219,4 +219,48 @@ class BridgeStateTest {
         val next = s.apply(msg(1, Protocol.CHAT_MESSAGE, """{"id":"m_1","role":"user","text":"hi"}"""))
         assertEquals(s.copy(lastSeq = 1), next)
     }
+
+    /** A new session makes every reducer forget its part (Reducer.reset), the chat and the remote mode included. */
+    @Test fun aNewSessionResetsEveryArea() {
+        val s = BridgeState(
+            session = "s_1", lastSeq = 4, sessionState = SessionState(paused = true),
+            chat = ChatState(messages = listOf(ChatMessage(id = "m_1", role = "user"))),
+            remote = RemoteState(enabled = true, address = "192.0.2.7:5555"),
+        ).apply(welcome("s_2"))
+        assertEquals(ChatState(), s.chat)
+        assertEquals(RemoteState(), s.remote)
+        assertFalse(s.sessionState.paused)
+        assertEquals(0L, s.lastSeq)
+    }
+
+    /** An unreadable snapshot drops what it should have replaced, but keeps the session's state (as in M2). */
+    @Test fun anUnreadableSnapshotResetsEveryArea() {
+        val s = BridgeState(session = "s_1", remote = RemoteState(enabled = true))
+            .apply(msg(1, Protocol.SESSION_STATE, """{"state":"working","paused":true}"""))
+            .apply(msg(2, Protocol.CONFIRM_REQUEST, question))
+            .apply(msg(3, Protocol.SNAPSHOT, """{"pending":"nope"}"""))
+        assertTrue(s.pending.isEmpty())
+        assertEquals(RemoteState(), s.remote)
+        assertEquals(SessionState(state = "working", paused = true), s.sessionState)
+        assertEquals(3L, s.lastSeq)
+    }
+
+    /** A dropped connection only forgets what was being sent; the chat and the remote mode stay. */
+    @Test fun aDisconnectionKeepsTheAreas() {
+        val chat = ChatState(messages = listOf(ChatMessage(id = "m_1", role = "user")))
+        val s = BridgeState(session = "s_1", chat = chat, remote = RemoteState(enabled = true))
+            .apply(msg(1, Protocol.CONFIRM_REQUEST, question)).answered("j_1").disconnected()
+        assertEquals(chat, s.chat)
+        assertEquals(RemoteState(enabled = true), s.remote)
+        assertEquals(listOf("j_1"), s.pending.map { it.id })
+        assertTrue(s.answering.isEmpty())
+    }
+
+    /** Every reducer has its own reset: the same session keeps its state. */
+    @Test fun theSameSessionKeepsEveryArea() {
+        val chat = ChatState(messages = listOf(ChatMessage(id = "m_1", role = "user")))
+        val s = BridgeState(session = "s_1", lastSeq = 4, chat = chat).apply(welcome("s_1"))
+        assertEquals(chat, s.chat)
+        assertEquals(4L, s.lastSeq)
+    }
 }
```

```diff
diff --git a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
index 9c24cd1..c44cdb3 100644
--- a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
+++ b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
@@ -407,4 +407,71 @@ class BridgeTest {
         assertEquals(null, frames.poll(200, TimeUnit.MILLISECONDS))
         bridge.stop()
     }
+
+    /** A listener or an onResult that throws does not cut the channel nor keep the others from being called. */
+    @Test fun aThrowingCallbackDoesNotCutTheChannel() {
+        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
+        val others = LinkedBlockingQueue<Message>()
+        bridge.listen(Protocol.ERROR) { throw IllegalStateException("a bug in a listener") }
+        bridge.listen(Protocol.ERROR) { others.add(it) }
+        bridge.start(link())
+        connect(bridge, hosts[0])
+
+        val rid = bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { throw IllegalStateException("a bug in onResult") }
+        assertEquals(rid, hosts[0].next().rid)
+        hosts[0].send("""{"v":1,"rid":"$rid","type":"error","ts":"x","payload":{"code":"failed","text":"x"}}""")
+        assertEquals("the other listener still hears it", rid, others.poll(5, TimeUnit.SECONDS)!!.rid)
+
+        hosts[0].send("""{"v":1,"seq":1,"type":"session.state","ts":"x","payload":{"state":"working"}}""")
+        waitFor("the socket is still alive") { bridge.state.value.sessionState.state == "working" }
+        assertEquals(BridgeState.Connection.CONNECTED, bridge.state.value.connection)
+        assertEquals("no reconnection", 1, server.requestCount)
+        bridge.stop()
+    }
+
+    /**
+     * stop() fails the requests still waiting, and an onResult that throws there
+     * does not keep the others from failing. Both throw: whatever the order of
+     * the pending map, one that throws comes before another.
+     */
+    @Test fun stopFailsThePendingRequests() {
+        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
+        bridge.start(link())
+        connect(bridge, hosts[0])
+        val first = CompletableFuture<Message?>()
+        val second = CompletableFuture<Message?>()
+        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { first.complete(it); throw IllegalStateException("bug") })
+        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { second.complete(it); throw IllegalStateException("bug") })
+        hosts[0].next()
+        hosts[0].next()
+        bridge.stop()
+        assertEquals(null, first.get(5, TimeUnit.SECONDS))
+        assertEquals(null, second.get(5, TimeUnit.SECONDS))
+    }
+
+    /** When the host drops the socket, an onResult that throws in failPending does not stop the reconnection loop. */
+    @Test fun aThrowingOnResultDoesNotStopTheReconnection() {
+        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
+        bridge.start(link())
+        connect(bridge, hosts[0])
+        val failed = CompletableFuture<Message?>()
+        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { failed.complete(it); throw IllegalStateException("bug") })
+        hosts[0].next()
+        hosts[0].ws!!.close(1000, "restart")
+        assertEquals(null, failed.get(5, TimeUnit.SECONDS))
+        assertEquals("the app connects again", Protocol.HELLO, hosts[1].next().type)
+        bridge.stop()
+    }
+
+    /** Before the welcome nothing is sent: the host reads the hello first. */
+    @Test fun nothingIsSentBeforeTheWelcome() {
+        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
+        bridge.start(link())
+        assertEquals(Protocol.HELLO, hosts[0].next().type) // the socket is open, but no welcome yet
+        assertEquals(null, bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { })
+        assertFalse(bridge.send(Protocol.TERMINAL_UNWATCH, JsonObject(emptyMap())))
+        assertFalse(bridge.sendFrame("00112233445566778899aabbccddeeff", 0, "x".toByteArray()))
+        assertEquals("nothing reached the host", null, hosts[0].received.poll(300, TimeUnit.MILLISECONDS))
+        bridge.stop()
+    }
 }
```

- [ ] **Paso 2: Ver que fallan.** `cd android && gradle :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.bridge.*'`.
  - No compila: `Reset` no existe todavía.
  - Una vez escrito `Reducer.kt` (paso 3), `aNewSessionResetsEveryArea` y `anUnreadableSnapshotResetsEveryArea` fallan, porque ni `chat` ni `remote` se olvidan.
  - `aThrowingCallbackDoesNotCutTheChannel`, `stopFailsThePendingRequests` y `aThrowingOnResultDoesNotStopTheReconnection` fallan, porque la excepción corta el canal o deja peticiones sin fallar.

- [ ] **Paso 3: Los reductores.** Aplica:

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
index 7870e1e..065036f 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
@@ -14,4 +14,22 @@ interface Reducer {
 
     /** Takes its part of a readable snapshot. s already has the snapshot's lastSeq. */
     fun snapshot(s: BridgeState, snap: Snapshot): BridgeState
+
+    /** Forgets what of its part can no longer be trusted, for the reason why. */
+    fun reset(s: BridgeState, why: Reset): BridgeState
+}
+
+/** Why BridgeState asks every reducer to forget part of what it knows. */
+enum class Reset {
+    /** The host restarted: nothing of the old session holds. */
+    NEW_SESSION,
+
+    /** A snapshot could not be read: what it should have replaced may be stale. */
+    UNREADABLE_SNAPSHOT,
+
+    /** The socket is gone: what was sent on it may never have reached the host. */
+    DISCONNECTED,
+
+    /** The host or the app is too old: nothing more comes until a new welcome. */
+    INCOMPATIBLE,
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
index cfd52fb..076c6d2 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
@@ -30,6 +30,15 @@ object ConfirmReducer : Reducer {
         else -> s
     }
 
+    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
+        // Nothing of the old session holds.
+        Reset.NEW_SESSION -> s.copy(sessionState = SessionState(), pending = emptyList(), answering = emptySet())
+        // The old questions may be answered already, so none is kept.
+        Reset.UNREADABLE_SNAPSHOT, Reset.INCOMPATIBLE -> s.copy(pending = emptyList(), answering = emptySet())
+        // An answer sent on the socket may never have reached the host: its question can be answered again.
+        Reset.DISCONNECTED -> s.copy(answering = emptySet())
+    }
+
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState {
         val entries = snap.journal.mapNotNull {
             try {
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
index 84eeba0..5e30ca2 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
@@ -15,4 +15,9 @@ object ChatReducer : Reducer {
     override fun event(s: BridgeState, m: Message): BridgeState = s
 
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
+
+    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
+        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(chat = ChatState())
+        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
+    }
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
index 38c4a49..3c6718d 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
@@ -7,4 +7,9 @@ object RemoteReducer : Reducer {
     override fun event(s: BridgeState, m: Message): BridgeState = s
 
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
+
+    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
+        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(remote = RemoteState())
+        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
+    }
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
index a64327a..7bec5fb 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
@@ -48,34 +48,34 @@ data class BridgeState(
         val w = Protocol.payload<Welcome>(m)
         // A welcome this app cannot read comes from a host too old (or too odd) to talk to.
         if (w == null || w.protocol < Protocol.MIN_HOST_PROTOCOL) {
-            return copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST, pending = emptyList(), answering = emptySet())
+            return reset(Reset.INCOMPATIBLE).copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST)
         }
         // A new session (the host restarted) starts over: nothing of the old one holds.
         if (w.session != session) {
-            return copy(
-                session = w.session, lastSeq = 0, sessionState = SessionState(), pending = emptyList(), answering = emptySet(),
-                connection = Connection.CONNECTED, outdated = Outdated.NONE,
-            )
+            return reset(Reset.NEW_SESSION).copy(session = w.session, lastSeq = 0, connection = Connection.CONNECTED, outdated = Outdated.NONE)
         }
         return copy(connection = Connection.CONNECTED, outdated = Outdated.NONE)
     }
 
     private fun snapshot(m: Message): BridgeState {
         val seq = if (m.seq > 0) m.seq else lastSeq
-        // Unreadable: the old questions may be answered already, so none is kept.
-        val snap = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq, pending = emptyList(), answering = emptySet())
+        // Unreadable: what it should have replaced may be stale.
+        val snap = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq).reset(Reset.UNREADABLE_SNAPSHOT)
         return REDUCERS.fold(copy(lastSeq = seq)) { s, r -> r.snapshot(s, snap) }
     }
 
+    /** Every reducer forgets what of its part can no longer be trusted (Reducer.reset). */
+    private fun reset(why: Reset): BridgeState = REDUCERS.fold(this) { s, r -> r.reset(s, why) }
+
     /** The host closed the channel because this app is too old. */
     fun appOutdated(): BridgeState =
-        copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP, pending = emptyList(), answering = emptySet())
+        reset(Reset.INCOMPATIBLE).copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP)
 
     /**
      * The socket is gone. An answer sent on it may never have reached the host,
      * so its question can be answered again (the host ignores a repeat).
      */
-    fun disconnected(): BridgeState = copy(connection = Connection.WAITING, answering = emptySet())
+    fun disconnected(): BridgeState = reset(Reset.DISCONNECTED).copy(connection = Connection.WAITING)
 
     /**
      * Marks a question the person just answered here. It stays in pending until
```

- [ ] **Paso 4: `Bridge.kt`.** Aplica:

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
index 2772356..e05e8ea 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
@@ -157,7 +157,23 @@ class Bridge internal constructor(
             val ids = pending.filterValues { it.socket === ws }.keys
             ids.mapNotNull { pending.remove(it) }
         }
-        failed.forEach { it.onResult(null) }
+        failed.forEach { p -> safely { p.onResult(null) } }
+    }
+
+    /**
+     * Runs a callback of another part of the app (a listener, onResult). Its
+     * exception stays there: it must not cut the socket (OkHttp's reader),
+     * stop the reconnection loop (failPending runs in it) nor leave the other
+     * callbacks uncalled. It is not logged: the logs of this process must
+     * never risk carrying a message's content. An Error (TODO(), out of
+     * memory) still ends the process.
+     */
+    private inline fun safely(f: () -> Unit) {
+        try {
+            f()
+        } catch (e: Exception) {
+            // A bug of that callback; the channel goes on.
+        }
     }
 
     /** Callbacks of one socket. Only the current socket changes the state; any socket completes its own attempt. */
@@ -175,17 +191,19 @@ class Bridge internal constructor(
             val m = Protocol.decode(text) ?: return
             val (next, answered) = synchronized(lock) {
                 if (webSocket !== socket) return
-                _state.updateAndGet { it.apply(m) } to m.rid?.let { pending.remove(it) }
+                // Only a request sent on this socket: one of an older socket already failed or is failing.
+                val waiting = m.rid?.let { rid -> pending[rid]?.takeIf { it.socket === webSocket }?.let { pending.remove(rid) } }
+                _state.updateAndGet { it.apply(m) } to waiting
             }
             if (next.connection == BridgeState.Connection.INCOMPATIBLE) {
                 webSocket.close(1000, "host too old")
-                answered?.onResult?.invoke(null)
+                answered?.let { p -> safely { p.onResult(null) } }
                 return
             }
             if (m.type == Protocol.WELCOME) attempt.welcomed = true
-            onMessage(m, next)
-            answered?.onResult?.invoke(m)
-            listeners[m.type]?.forEach { it(m) }
+            safely { onMessage(m, next) }
+            answered?.let { p -> safely { p.onResult(m) } }
+            listeners[m.type]?.forEach { l -> safely { l(m) } }
         }
 
         override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
@@ -194,7 +212,7 @@ class Bridge internal constructor(
             synchronized(lock) {
                 if (webSocket !== socket || _state.value.connection != BridgeState.Connection.CONNECTED) return
             }
-            frameListeners.forEach { it(chunk) }
+            frameListeners.forEach { l -> safely { l(chunk) } }
         }
 
         override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
```

- [ ] **Paso 5: Ver que pasa.** `cd android && gradle :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`. Esperado: `BUILD SUCCESSFUL`, 130 tests, con los 22 de M2 de `BridgeStateTest` sin cambios.

- [ ] **Paso 6: Comprobar que los tests muerden.** En `Bridge.kt`, dos mutaciones, una cada vez:
  - cambia `listeners[m.type]?.forEach { l -> safely { l(m) } }` por `listeners[m.type]?.forEach { l -> l(m) }`: `gradle :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.bridge.BridgeTest' --rerun` debe fallar en `aThrowingCallbackDoesNotCutTheChannel`;
  - cambia `failed.forEach { p -> safely { p.onResult(null) } }` por `failed.forEach { p -> p.onResult(null) }`: deben fallar `stopFailsThePendingRequests` y `aThrowingOnResultDoesNotStopTheReconnection`.

  Deshaz cada una a mano, volviendo a escribir la línea original (no uses `git checkout`, que se llevaría todo el paso 4), y comprueba con `git diff --stat` y una ejecución más que todo vuelve a pasar.

- [ ] **Paso 7: Commit.**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
git commit -m "fix(app): oyentes aislados, respuestas por socket y reset por área en el canal

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tarea 2: Host: `SendTo`

**Ficheros:**
- Modificar: `host/internal/bridge/server.go`
- Modificar (tests): `host/internal/bridge/send_test.go`

**Interfaces:**
- **Produce `(*bridge.Server).SendTo(req bridge.Message, typ string, payload any) error`:** un mensaje de flujo que solo va a la conexión por la que llegó `req`, como `Reply`.
- **Errores:**
  - `ErrClass` si el tipo no es de flujo;
  - `ErrNoApp` si `req` no viene de `OnMessage` o su conexión ya no está.

- [ ] **Paso 1: Escribir el test que falla.** Aplica:

```diff
diff --git a/host/internal/bridge/send_test.go b/host/internal/bridge/send_test.go
index 9e703f1..4008393 100644
--- a/host/internal/bridge/send_test.go
+++ b/host/internal/bridge/send_test.go
@@ -281,3 +281,33 @@ func TestSnapshotListsAreNeverNull(t *testing.T) {
 		}
 	}
 }
+
+// SendTo sends a stream message to the connection that asked for it, and
+// nowhere once another one replaced it.
+func TestSendToGoesToTheConnectionThatAsked(t *testing.T) {
+	h := start(t, Options{})
+	c1 := h.dial(token, 0)
+	recv(t, c1)
+	send(t, c1, TypeTerminalWatch, TerminalWatch{})
+	watch := wantMsg(t, h, TypeTerminalWatch)
+	if err := h.srv.SendTo(watch, TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("x")}); err != nil {
+		t.Fatal(err)
+	}
+	if m := recv(t, c1); m.Type != TypeTerminalOutput || m.Seq != 0 || m.RID != "" {
+		t.Fatalf("got %+v", m)
+	}
+	if err := h.srv.SendTo(watch, TypeError, ErrorReply{}); !errors.Is(err, ErrClass) {
+		t.Fatalf("a reply through SendTo: %v, want ErrClass", err)
+	}
+	if err := h.srv.SendTo(Message{}, TypeTerminalOutput, TerminalOutput{}); !errors.Is(err, ErrNoApp) {
+		t.Fatalf("a built request: %v, want ErrNoApp", err)
+	}
+
+	c2 := h.dial(token, 0)
+	recv(t, c2)
+	waitUntilReplaced(t, c1)
+	if err := h.srv.SendTo(watch, TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("y")}); !errors.Is(err, ErrNoApp) {
+		t.Fatalf("to the replaced connection: %v, want ErrNoApp", err)
+	}
+	quiet(t, c2)
+}
```

- [ ] **Paso 2: Ver que falla.** `cd host && go test -count=1 -timeout 120s ./internal/bridge/`. Esperado: no compila (`h.srv.SendTo undefined`).

- [ ] **Paso 3: `SendTo`.** Aplica:

```diff
diff --git a/host/internal/bridge/server.go b/host/internal/bridge/server.go
index 5598c9d..5d2b0ef 100644
--- a/host/internal/bridge/server.go
+++ b/host/internal/bridge/server.go
@@ -198,11 +198,11 @@ func (s *Server) Connected() bool {
 	return s.client != nil
 }
 
-// ErrNoApp is the error of Send when no app is connected, and of Reply when
-// the connection that asked is gone.
+// ErrNoApp is the error of Send when no app is connected, and of Reply and
+// SendTo when the connection that asked is gone.
 var ErrNoApp = errors.New("the phone app is not connected")
 
-// ErrClass is the error of Publish, Reply and Send for a type of another
+// ErrClass is the error of Publish, Reply, Send and SendTo for a type of another
 // class (HostClasses): an event sent as a stream would never be resent, and a
 // stream message published as an event would be (a screen tap repeated after
 // a reconnection).
@@ -267,6 +267,22 @@ func (s *Server) Send(typ string, payload any) error {
 	return s.sendVolatile(0, typ, "", payload)
 }
 
+// SendTo sends a stream message to the connection req came on, like Reply: a
+// terminal subscription belongs to the connection that asked for it (protocol
+// spec §7), and another app that replaced it must not get its output. If that
+// connection is gone it returns ErrNoApp. Like Send, it drops an app too slow
+// to take it. (Downloads go as binary frames and end with file.end, a Reply.)
+func (s *Server) SendTo(req Message, typ string, payload any) error {
+	if HostClasses[typ] != ClassStream {
+		return fmt.Errorf("send %s: %w", typ, ErrClass)
+	}
+	if req.conn == 0 {
+		// Not a request as OnMessage gives it: it has no connection to go to.
+		return ErrNoApp
+	}
+	return s.sendVolatile(req.conn, typ, "", payload)
+}
+
 // sendVolatile queues a message that is not kept: for the connection gen, or
 // for the current one when gen is 0.
 func (s *Server) sendVolatile(gen uint64, typ, rid string, payload any) error {
```

- [ ] **Paso 4: Ver que pasa.** `cd host && gofmt -l . && go vet ./... && go test -count=1 -timeout 900s ./...`. Esperado: todo `ok`.

- [ ] **Paso 5: Commit.**

```bash
git add host/internal/bridge/server.go host/internal/bridge/send_test.go
git commit -m "feat(host): SendTo, flujos atados a la conexión que los pidió

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
