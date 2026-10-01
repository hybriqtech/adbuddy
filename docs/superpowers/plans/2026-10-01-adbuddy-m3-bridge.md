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
  - en la app, `:app:testDebugUnitTest :app:assembleDebug :app:lintDebug`, con 128 tests.
- Se reprodujo tarea a tarea sobre `main` (0ec286f): cada tarea pasa sola y el resultado es idéntico al prototipo.
- Se comprobó que `aThrowingCallbackDoesNotCutTheChannel` falla si se quita la protección de los oyentes.
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
- **`Reducer.reset(s: BridgeState): BridgeState`, sin implementación por defecto:** cada área olvida su parte.
  - `ConfirmReducer` olvida `sessionState`, `pending` y `answering`;
  - `ChatReducer` olvida `chat`;
  - `RemoteReducer` olvida `remote`.
- **`BridgeState` llama a todos los `reset`** con un welcome de sesión nueva y con un `snapshot` ilegible. Antes, un `snapshot` ilegible conservaba `sessionState`; ahora también lo olvida.
- **`Bridge`:**
  - ejecuta cada oyente, cada oyente de tramas, cada `onResult` y el `onMessage` del constructor dentro de `safely`: una excepción ni corta el socket ni para el bucle, ni impide llamar a los demás, y no se registra en ningún log;
  - `onMessage` resuelve `pending[rid]` solo si la petición salió por ese socket.

- [ ] **Paso 1: Escribir los tests que fallan.** Aplica:

```diff
diff --git a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
index ed82aaf..815f7f6 100644
--- a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
+++ b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt
@@ -219,4 +219,35 @@ class BridgeStateTest {
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
+    /** An unreadable snapshot resets every area too: nothing the app had can be trusted. */
+    @Test fun anUnreadableSnapshotResetsEveryArea() {
+        val s = BridgeState(session = "s_1", lastSeq = 1, remote = RemoteState(enabled = true))
+            .apply(msg(1, Protocol.CONFIRM_REQUEST, question))
+            .apply(msg(2, Protocol.SNAPSHOT, """{"pending":"nope"}"""))
+        assertTrue(s.pending.isEmpty())
+        assertEquals(RemoteState(), s.remote)
+        assertEquals(2L, s.lastSeq)
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
index 9c24cd1..4ed7f5d 100644
--- a/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
+++ b/android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
@@ -407,4 +407,53 @@ class BridgeTest {
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
+    /** stop() fails the requests still waiting, and an onResult that throws there does not keep the others from failing. */
+    @Test fun stopFailsThePendingRequests() {
+        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
+        bridge.start(link())
+        connect(bridge, hosts[0])
+        val first = CompletableFuture<Message?>()
+        val second = CompletableFuture<Message?>()
+        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { first.complete(it); throw IllegalStateException("bug") })
+        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { second.complete(it) })
+        hosts[0].next()
+        hosts[0].next()
+        bridge.stop()
+        assertEquals(null, first.get(5, TimeUnit.SECONDS))
+        assertEquals(null, second.get(5, TimeUnit.SECONDS))
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
  - `aNewSessionResetsEveryArea` y `anUnreadableSnapshotResetsEveryArea` fallan, porque ni `chat` ni `remote` se olvidan.
  - `aThrowingCallbackDoesNotCutTheChannel` y `stopFailsThePendingRequests` fallan, porque la excepción corta el canal o deja peticiones sin fallar.

- [ ] **Paso 3: Los reductores.** Aplica:

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
index 7870e1e..6f0884b 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Reducer.kt
@@ -14,4 +14,10 @@ interface Reducer {
 
     /** Takes its part of a readable snapshot. s already has the snapshot's lastSeq. */
     fun snapshot(s: BridgeState, snap: Snapshot): BridgeState
+
+    /**
+     * Forgets its part: what the app knows of the host can no longer be trusted
+     * (the host restarted, or a snapshot could not be read).
+     */
+    fun reset(s: BridgeState): BridgeState
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
index cfd52fb..b91a915 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ConfirmReducer.kt
@@ -30,6 +30,10 @@ object ConfirmReducer : Reducer {
         else -> s
     }
 
+    /** The old questions may be answered already: none is kept, nor the old session's state. */
+    override fun reset(s: BridgeState): BridgeState =
+        s.copy(sessionState = SessionState(), pending = emptyList(), answering = emptySet())
+
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState {
         val entries = snap.journal.mapNotNull {
             try {
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
index 84eeba0..7308242 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt
@@ -15,4 +15,6 @@ object ChatReducer : Reducer {
     override fun event(s: BridgeState, m: Message): BridgeState = s
 
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
+
+    override fun reset(s: BridgeState): BridgeState = s.copy(chat = ChatState())
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
index 38c4a49..3b6698a 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/RemoteReducer.kt
@@ -7,4 +7,6 @@ object RemoteReducer : Reducer {
     override fun event(s: BridgeState, m: Message): BridgeState = s
 
     override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
+
+    override fun reset(s: BridgeState): BridgeState = s.copy(remote = RemoteState())
 }
```

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
index a64327a..305bc2e 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt
@@ -52,21 +52,21 @@ data class BridgeState(
         }
         // A new session (the host restarted) starts over: nothing of the old one holds.
         if (w.session != session) {
-            return copy(
-                session = w.session, lastSeq = 0, sessionState = SessionState(), pending = emptyList(), answering = emptySet(),
-                connection = Connection.CONNECTED, outdated = Outdated.NONE,
-            )
+            return reset().copy(session = w.session, lastSeq = 0, connection = Connection.CONNECTED, outdated = Outdated.NONE)
         }
         return copy(connection = Connection.CONNECTED, outdated = Outdated.NONE)
     }
 
     private fun snapshot(m: Message): BridgeState {
         val seq = if (m.seq > 0) m.seq else lastSeq
-        // Unreadable: the old questions may be answered already, so none is kept.
-        val snap = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq, pending = emptyList(), answering = emptySet())
+        // Unreadable: nothing the app had can be trusted, and the snapshot cannot replace it.
+        val snap = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq).reset()
         return REDUCERS.fold(copy(lastSeq = seq)) { s, r -> r.snapshot(s, snap) }
     }
 
+    /** Every reducer forgets its part (Reducer.reset). */
+    private fun reset(): BridgeState = REDUCERS.fold(this) { s, r -> r.reset(s) }
+
     /** The host closed the channel because this app is too old. */
     fun appOutdated(): BridgeState =
         copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP, pending = emptyList(), answering = emptySet())
```

- [ ] **Paso 4: `Bridge.kt`.** Aplica:

```diff
diff --git a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
index 2772356..9802ea1 100644
--- a/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
+++ b/android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt
@@ -157,7 +157,22 @@ class Bridge internal constructor(
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
+     * never risk carrying a message's content.
+     */
+    private inline fun safely(f: () -> Unit) {
+        try {
+            f()
+        } catch (e: Exception) {
+            // A bug of that callback; the channel goes on.
+        }
     }
 
     /** Callbacks of one socket. Only the current socket changes the state; any socket completes its own attempt. */
@@ -175,17 +190,19 @@ class Bridge internal constructor(
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
@@ -194,7 +211,7 @@ class Bridge internal constructor(
             synchronized(lock) {
                 if (webSocket !== socket || _state.value.connection != BridgeState.Connection.CONNECTED) return
             }
-            frameListeners.forEach { it(chunk) }
+            frameListeners.forEach { l -> safely { l(chunk) } }
         }
 
         override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
```

- [ ] **Paso 5: Ver que pasa.** `cd android && gradle :app:testDebugUnitTest :app:assembleDebug :app:lintDebug`. Esperado: `BUILD SUCCESSFUL`, 128 tests, con los 22 de M2 de `BridgeStateTest` sin cambios.

- [ ] **Paso 6: Comprobar que el test muerde.** En `Bridge.kt` cambia `listeners[m.type]?.forEach { l -> safely { l(m) } }` por `listeners[m.type]?.forEach { l -> l(m) }`. `gradle :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.bridge.BridgeTest'` debe fallar en `aThrowingCallbackDoesNotCutTheChannel`. Deshazlo con `git checkout -- android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt` (aún sin commit: vuelve a aplicar el parche del paso 4 si hace falta) y comprueba que pasa.

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
index 5598c9d..fa4528b 100644
--- a/host/internal/bridge/server.go
+++ b/host/internal/bridge/server.go
@@ -267,6 +267,21 @@ func (s *Server) Send(typ string, payload any) error {
 	return s.sendVolatile(0, typ, "", payload)
 }
 
+// SendTo sends a stream message to the connection req came on, like Reply: a
+// terminal subscription or a download belongs to the connection that asked
+// for it (protocol spec §6.2, §7), and another app that replaced it must not
+// get its bytes. If that connection is gone it returns ErrNoApp.
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
