# ADBuddy M2 · App base: canal, confirmaciones en el móvil, Pausar y Cortar acceso · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Objetivo:** que el móvil vea y controle lo que hace el agente (spec §13, M2). El host instala la app con `adb install` y pide el consentimiento en ella. Después concede `WRITE_SECURE_SETTINGS`, activa la accesibilidad sin quitar otros servicios y abre un canal WebSocket por `adb reverse` autenticado con un token. Por ese canal llegan al móvil el diario en directo y las confirmaciones 🔴 (notificación con desbloqueo obligatorio). Desde el móvil se puede Pausar el agente o Cortar el acceso. Además, `adbuddy doctor` diagnostica todo el conjunto y dice cómo arreglar cada problema, y la app y el host comprueban que sus versiones son compatibles. Las dos ideas vienen de revisar ARTEMIS de Google, igual que las reglas nuevas de `AGENTS.md`.

**Arquitectura:** sobre M1c (rama `m1c`, conexión manual por IP:puerto; HEAD d7acb35 y sus arreglos de documentación).
- **Host.** Dos paquetes nuevos:
  - `bridge`: el servidor WS en `127.0.0.1:<chat_port>`, el protocolo v1 de §5.3, el búfer de 2000 eventos, la reanudación con `lastSeq` o `snapshot`, y un `Confirmer` que pregunta al móvil y, si la app no está conectada, a la terminal.
  - `phoneapp`: instalar, consentimiento, permisos, túnel y entrega del token.

  Además, cambios pequeños en `confirm`, `daemon`, `classify`, `config` y `cli`:
  - `Key` para "Permitir durante la sesión";
  - la pausa en el daemon;
  - el canal de la app, prohibido al agente;
  - `chat_port`;
  - `serve` con el canal;
  - `setup` con la app.
- **App.** Proyecto nuevo `android/` en Kotlin con Compose, minSdk 30, sin Gradle wrapper. Piezas:
  - `TokenProvider`: solo atiende al uid 2000;
  - `Bridge`: cliente OkHttp;
  - `BridgeService`: servicio en primer plano `specialUse`;
  - notificaciones de confirmación con `setAuthenticationRequired`;
  - la pantalla de consentimiento;
  - el panel de actividad;
  - Pausar y Cortar acceso;
  - un servicio de accesibilidad mínimo, que se completa en M5.

**Tecnologías:**
- **Host:** las de M1, más `github.com/coder/websocket` v1.8.15 (ISC).
- **App:**
  - AGP 9.4.1, con Kotlin integrado y sin plugin `kotlin-android`;
  - plugins de Kotlin 2.4.20 (compose, serialization);
  - Compose BOM 2026.09.00;
  - OkHttp 5.5.0 (+ mockwebserver3);
  - kotlinx-serialization-json 1.11.0;
  - androidx core-ktx 1.19.1, activity-compose 1.13.0 y lifecycle 2.11.0;
  - Gradle 9.8.0 y JDK 17.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`, sobre todo:
- §3.1, pasos 5-7;
- §4.5 (confirmaciones) y §4.6 (Pausar, Cortar acceso, protecciones);
- §5.2 y §5.3 (transporte, token y protocolo);
- §6 (app);
- §10 (tests).

**Verificación del plan:**
- **Host (tareas 1-7):** se compiló y se probó en Windows 11 con Go 1.27.0 sobre el resultado de M1c antes de escribir el plan. Incluye un extremo a extremo con el binario real y una app simulada por WebSocket: confirmar 🔴 desde el "móvil", pausar y reanudar, y el token nunca en una línea de comandos.
- **App (tareas 8-10):** se compiló y se probó en Windows 11 con Gradle 9.8.0, JDK 17 y el SDK 37, reproduciendo las tareas una a una desde su texto: las tareas 8 y 9 pasan sus tests JVM, y la 10 pasa `testDebugUnitTest`, `assembleDebug` y `lintDebug` sin errores ni avisos. El test instrumentado del `TokenProvider` necesita un emulador y no se ha ejecutado.
- **Cambios de la ejecución:** las revisiones de las tareas 1-3 cambiaron el host respecto a este texto (por ejemplo, `Hello.session` para que el host detecte que se reinició). La app de las tareas 8-10 ya los incluye.
- **Si un paso no sale como dice el plan,** detente y explícalo.

## Restricciones globales

- **Todas las de M1a y M1b siguen vigentes:** módulo `host/`, Go 1.27, CLI con sobre JSON y códigos 0/1/2/75/78/130, LF, TDD, commits con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, identidad local hybriqtech, sin push y sin subagentes.
- **Rama `m2`**, creada desde `m1c`.
- **Mensajes literales del shim**, que el agente reconoce por `AGENTS.md` y M2 no puede cambiar:
  - `ADBuddy host not running (run: adbuddy serve)`;
  - `Denied by user: <título>` (con ` (confirmation expired)` si caducó), cuando alguien, en la terminal o ahora en el móvil, dijo que no o no contestó a tiempo;
  - `Denied: nobody could confirm: <título>`, cuando nadie pudo responder: sin terminal, con `--confirm deny` o sin la app conectada;
  - `Blocked by ADBuddy: <motivo>`.
- **Dependencias nuevas con versión exacta:**
  - en el host, solo `github.com/coder/websocket` v1.8.15;
  - en la app, solo las de `android/gradle/libs.versions.toml`.
- **El móvil del usuario no se toca en ningún test.** El host usa el adb falso. Los tests de la app son JVM o instrumentados en emulador, nunca en el móvil del usuario. Lo que necesita un móvil real queda como validación manual (ver el final).
- **El token nunca va en una línea de comandos** (ni en el PC ni en el móvil): va por stdin a `content write`. El servidor WS escucha solo en `127.0.0.1`, compara el token en tiempo constante y rechaza los `Origin` ajenos.
- **El agente no puede usar el canal de la app.** `content` sobre `tech.hybriq.adbuddy`, y `run-as tech.hybriq.adbuddy`, son siempre 🔴 especial, aunque solo lean.
- **El host hace sus propios cambios en el móvil solo tras el consentimiento en la app:** `pm grant`, excepción de batería y accesibilidad. Sin consentimiento, nada.
- **La app es opcional para el host.** Si no está, no conecta o se rechaza el consentimiento, las 🔴 se confirman en la terminal como en M1. Ningún fallo de la app hace fallar `setup` ni `serve`.
- **CI de Android:** solo en `pull_request` y push a `main`, con filtro de rutas, y solo en Linux (los minutos de macOS cuentan 10 veces).
- **Sin IPs de la red local ni seriales en el repo:** los tests usan `192.0.2.x`.

## Estructura de ficheros

```
host/internal/bridge/          # servidor WS del canal, protocolo v1, reanudación, Confirmer del móvil
host/internal/phoneapp/        # instalar, consentimiento, permisos, túnel y token (comandos del propio host)
host/internal/cli/phone.go     # serve ↔ app: puerto, token, túnel vigilado, pausa, diario al móvil
host/internal/cli/appsetup.go  # setup, pasos 5-6 de §3.1
host/internal/cli/doctor.go    # adbuddy doctor: diagnóstico con Run:/Guidance:
android/                       # la app (Kotlin + Compose), sin Gradle wrapper
.github/workflows/android.yml  # tests unitarios y lint de la app
```

---

### Tarea 1: Canal WebSocket del host (`bridge`)

§5.2-5.3: servidor WS en `127.0.0.1:<puerto>/v1`. El primer mensaje de la app debe ser `hello` con el token (comparado en tiempo constante) en menos de 5 s; si no, se cierra con *policy violation*. Una conexión nueva sustituye a la anterior. Cada evento lleva `seq`; se guardan los últimos 2000 y, al reconectar, se reenvían los posteriores a `lastSeq`, o un `snapshot` si faltan o si el host se reinició. `Confirmer` implementa `confirm.Confirmer`: publica `confirm.request`, espera `confirm.reply` y, si no hay app conectada al preguntar, usa el de reserva (terminal o denegar). "Permitir durante la sesión" solo cubre la misma `Key` (regla + argumentos exactos) y nunca un comando especial. `confirm.Request` gana el campo `Key`. **Compatibilidad de versiones:** el `hello` lleva `protocol` y el `welcome` también. Una app con un protocolo menor que `MinAppProtocol` se cierra con el código `CloseIncompatible` (4001) y `OnReject` avisa en la terminal. La comprobación va siempre después del token: sin token nadie aprende nada del host.

**Ficheros:**
- Crear: `host/internal/bridge/protocol.go`
- Crear: `host/internal/bridge/server.go`
- Crear: `host/internal/bridge/confirm.go`
- Modificar: `host/internal/confirm/confirm.go`
- Test: `host/internal/bridge/bridge_test.go`

**Interfaces:**
- Consume: `confirm.Confirmer`, `confirm.Request`, `confirm.Decision` (M1a).
- Produce (paquete `bridge`): `ProtocolVersion`, `BufferSize`, `MinAppProtocol`, `CloseIncompatible`; `Message{V, Seq, Type, TS, Payload}`; constantes `Type*`; `Hello`, `Welcome`, `SessionState{Agent, State, Activity, Paused}`, `ConfirmRequest{ID, Level, Title, Detail, RawCommand, ExpiresAt, Special, AllowSession}`, `ConfirmReply{ID, Decision}`, `Snapshot{State, Pending, Journal}`, `Notice{Level, Text}`; `NewToken() (string, error)`; `Options{Token, Session, HostVersion, OnMessage, Snapshot, OnConnect, OnReject, Now}`; `Hello.Protocol`, `Welcome.Protocol`; `New(Options) *Server`; `Listen(port) (net.Listener, error)`; `(*Server) Serve(ctx, l) error`, `Connected() bool`, `Publish(typ, payload) error`; `Publisher` (interfaz); `NewConfirmer(app Publisher, fallback confirm.Confirmer) *Confirmer` con `Confirm`, `Reply(json.RawMessage)`, `Pending()`.
- Produce (paquete `confirm`): `Request.Key string`.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/bridge/bridge_test.go`:

```go
package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type harness struct {
	t    *testing.T
	srv  *Server
	url  string
	msgs chan Message
}

func start(t *testing.T, o Options) *harness {
	t.Helper()
	l, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, msgs: make(chan Message, 100)}
	if o.Token == "" {
		o.Token = token
	}
	o.Session = "s_TEST"
	o.OnMessage = func(m Message) { h.msgs <- m }
	h.srv = New(o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.srv.Serve(ctx, l); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	h.url = "ws://" + l.Addr().String() + "/v1"
	return h
}

// dial connects and sends hello; it returns the connection.
func (h *harness) dial(tok string, lastSeq uint64) *websocket.Conn {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.CloseNow() })
	send(h.t, c, TypeHello, Hello{Protocol: ProtocolVersion, Token: tok, LastSeq: lastSeq, AppVersion: "test"})
	return c
}

func send(t *testing.T, c *websocket.Conn, typ string, payload any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(Message{V: ProtocolVersion, Type: typ, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func recv(t *testing.T, c *websocket.Conn) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func waitConnected(t *testing.T, s *Server, want bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.Connected() != want {
		if time.Now().After(deadline) {
			t.Fatalf("Connected() never became %v", want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWrongTokenIsRejected(t *testing.T) {
	h := start(t, Options{})
	for _, tok := range []string{"", "nope", strings.Repeat("0", 64)} {
		c := h.dial(tok, 0)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := c.Read(ctx)
		cancel()
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Errorf("token %q: %v, want policy violation", tok, err)
		}
	}
	if h.srv.Connected() {
		t.Fatal("an app without the token counts as connected")
	}
}

func TestNoHelloIsRejected(t *testing.T) {
	h := start(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send(t, c, TypeChatSend, map[string]string{"text": "hi"})
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("%v, want policy violation", err)
	}
}

func TestForeignOriginIsRejected(t *testing.T) {
	h := start(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}}})
	if err == nil {
		t.Fatal("a web page from another origin could open the socket")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestWelcomeEventsAndResume(t *testing.T) {
	h := start(t, Options{HostVersion: "1.2.3"})
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "one"})
	c := h.dial(token, 0)
	w := recv(t, c)
	var wel Welcome
	json.Unmarshal(w.Payload, &wel)
	if w.Type != TypeWelcome || wel.Protocol != ProtocolVersion || wel.Session != "s_TEST" || wel.FromSeq != 1 || wel.HostVersion != "1.2.3" {
		t.Fatalf("welcome %+v %+v", w, wel)
	}
	if m := recv(t, c); m.Type != TypeNotice || m.Seq != 1 {
		t.Fatalf("backlog %+v", m)
	}
	waitConnected(t, h.srv, true)
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "two"})
	if m := recv(t, c); m.Seq != 2 {
		t.Fatalf("live event %+v", m)
	}
	c.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, h.srv, false)
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "three"})

	c2 := h.dial(token, 2)
	w2 := recv(t, c2)
	json.Unmarshal(w2.Payload, &wel)
	if wel.FromSeq != 3 {
		t.Fatalf("resume from %d, want 3", wel.FromSeq)
	}
	m := recv(t, c2)
	var n Notice
	json.Unmarshal(m.Payload, &n)
	if m.Seq != 3 || n.Text != "three" {
		t.Fatalf("resumed %+v %q", m, n.Text)
	}
}

func TestTooOldOrFutureSeqGetsASnapshot(t *testing.T) {
	h := start(t, Options{Snapshot: func() Snapshot {
		return Snapshot{State: SessionState{State: "idle", Paused: true}, Pending: []ConfirmRequest{}, Journal: []json.RawMessage{}}
	}})
	for i := 0; i < BufferSize+5; i++ {
		h.srv.Publish(TypeNotice, Notice{Text: "x"})
	}
	for _, last := range []uint64{1, BufferSize * 10} {
		c := h.dial(token, last)
		recv(t, c) // welcome
		m := recv(t, c)
		var s Snapshot
		json.Unmarshal(m.Payload, &s)
		if m.Type != TypeSnapshot || !s.State.Paused {
			t.Fatalf("lastSeq %d: %+v", last, m)
		}
		c.Close(websocket.StatusNormalClosure, "")
		waitConnected(t, h.srv, false)
	}
}

func TestNewConnectionReplacesTheOld(t *testing.T) {
	h := start(t, Options{})
	c1 := h.dial(token, 0)
	recv(t, c1)
	waitConnected(t, h.srv, true)
	c2 := h.dial(token, 0)
	recv(t, c2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := c1.Read(ctx); err == nil {
		t.Fatal("the old connection is still open")
	}
	h.srv.Publish(TypeNotice, Notice{Text: "to c2"})
	if m := recv(t, c2); m.Type != TypeNotice {
		t.Fatalf("%+v", m)
	}
}

func TestAppMessagesAreDelivered(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	send(t, c, TypePause, struct{}{})
	select {
	case m := <-h.msgs:
		if m.Type != TypePause {
			t.Fatalf("%+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pause not delivered")
	}
}

// fakeApp records published requests and answers them.
type fakeApp struct {
	mu        sync.Mutex
	connected bool
	published []ConfirmRequest
	onPublish func(ConfirmRequest)
}

func (f *fakeApp) Connected() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.connected }
func (f *fakeApp) Publish(typ string, payload any) error {
	r := payload.(ConfirmRequest)
	f.mu.Lock()
	f.published = append(f.published, r)
	cb := f.onPublish
	f.mu.Unlock()
	if cb != nil {
		go cb(r)
	}
	return nil
}

type fixed confirm.Decision

func (d fixed) Confirm(context.Context, confirm.Request) confirm.Decision { return confirm.Decision(d) }

func reply(c *Confirmer, id, decision string) {
	raw, _ := json.Marshal(ConfirmReply{ID: id, Decision: decision})
	c.Reply(raw)
}

func TestConfirmOnThePhone(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	req := confirm.Request{ID: "j_1", Title: "Restart", Key: "reboot\x00shell\x00reboot", ExpiresAt: time.Now().Add(time.Minute)}
	for decision, want := range map[string]confirm.Decision{"allow": confirm.Allow, "deny": confirm.Deny, "bogus": confirm.Deny} {
		app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, decision) }
		if got := c.Confirm(context.Background(), req); got != want {
			t.Errorf("%s: %v want %v", decision, got, want)
		}
	}
	if len(app.published) != 3 || app.published[0].Title != "Restart" || !app.published[0].AllowSession {
		t.Fatalf("%+v", app.published)
	}
}

func TestAllowSessionCoversOnlyTheSameCommand(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, "allow_session") }
	a := confirm.Request{ID: "j_1", Key: "k1", ExpiresAt: time.Now().Add(time.Minute)}
	if c.Confirm(context.Background(), a) != confirm.Allow {
		t.Fatal("first")
	}
	app.onPublish = nil
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	a.ID = "j_2"
	if c.Confirm(ctx, a) != confirm.Allow {
		t.Fatal("the same command was asked again")
	}
	b := confirm.Request{ID: "j_3", Key: "k2", ExpiresAt: time.Now().Add(time.Minute)}
	if got := c.Confirm(ctx, b); got != confirm.Timeout {
		t.Fatalf("another command was allowed without asking: %v", got)
	}
	// A special command is never allowed for the session.
	app.onPublish = func(r ConfirmRequest) {
		if r.AllowSession {
			t.Error("allow_session offered for a special command")
		}
		reply(c, r.ID, "allow_session")
	}
	s := confirm.Request{ID: "j_4", Key: "k3", Special: true, ExpiresAt: time.Now().Add(time.Minute)}
	c.Confirm(context.Background(), s)
	app.onPublish = nil
	s.ID = "j_5"
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if c.Confirm(ctx2, s) != confirm.Timeout {
		t.Fatal("a special command was remembered for the session")
	}
}

func TestConfirmWithoutAppUsesFallback(t *testing.T) {
	c := NewConfirmer(&fakeApp{}, fixed(confirm.Allow))
	if c.Confirm(context.Background(), confirm.Request{ID: "j_1"}) != confirm.Allow {
		t.Fatal("fallback not used")
	}
}

func TestConfirmCancelAndLateReply(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	ctx, cancel := context.WithCancel(context.Background())
	app.onPublish = func(ConfirmRequest) { cancel() }
	if c.Confirm(ctx, confirm.Request{ID: "j_1", ExpiresAt: time.Now().Add(time.Minute)}) != confirm.Deny {
		t.Fatal("cancelled request not denied")
	}
	reply(c, "j_1", "allow") // late: ignored, no panic
	if len(c.Pending()) != 0 {
		t.Fatal("pending not cleaned")
	}
}

func TestAnOldAppIsToldToUpdate(t *testing.T) {
	rejected := make(chan Hello, 1)
	h := start(t, Options{OnReject: func(hl Hello) { rejected <- hl }})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send(t, c, TypeHello, Hello{Token: token, AppVersion: "0.1.0"}) // no protocol: an app older than the field
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusCode(CloseIncompatible) {
		t.Fatalf("%v, want close %d", err, CloseIncompatible)
	}
	select {
	case hl := <-rejected:
		if hl.AppVersion != "0.1.0" {
			t.Fatalf("%+v", hl)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnReject not called")
	}
	if h.srv.Connected() {
		t.Fatal("an incompatible app counts as connected")
	}
}

func TestAnOldAppWithoutTheTokenLearnsNothing(t *testing.T) {
	h := start(t, Options{})
	c := h.dial("wrong", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("%v: a wrong token must be rejected before the version check", err)
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test ./internal/bridge/ ./internal/confirm/
```

Esperado: no compila: `undefined: Listen`, `undefined: NewConfirmer`, `unknown field Key in struct literal`…

- [ ] **Paso 3: Implementar**

```bash
cd host && go get github.com/coder/websocket@v1.8.15
```

`host/internal/bridge/protocol.go`:

```go
// Package bridge is the channel between the host and the phone app (spec
// §5.2-5.3): a WebSocket server on 127.0.0.1 that the app reaches through
// `adb reverse`, authenticated with a token the host hands over by adb.
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ProtocolVersion is the "v" of every message and the /v1 path.
const ProtocolVersion = 1

// BufferSize is how many events the host keeps to resend after a reconnection.
const BufferSize = 2000

// MinAppProtocol is the oldest app protocol this host talks to. An older app
// is closed with CloseIncompatible, so it can tell the person to update it
// instead of misreading messages.
const MinAppProtocol = 1

// CloseIncompatible is the WebSocket close code for an app that is too old.
const CloseIncompatible = 4001

// Message is the envelope of every message in both directions.
type Message struct {
	V       int             `json:"v"`
	Seq     uint64          `json:"seq,omitempty"` // host → app only; 0 for replies that are not events
	Type    string          `json:"type"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

// Message types (spec §5.3). Only the ones M2 uses; the rest arrive with M3-M5.
const (
	TypeHello         = "hello"
	TypeWelcome       = "welcome"
	TypeSnapshot      = "snapshot"
	TypeSessionState  = "session.state"
	TypeConfirmReq    = "confirm.request"
	TypeConfirmReply  = "confirm.reply"
	TypeJournalEntry  = "journal.entry"
	TypeNotice        = "notice"
	TypePause         = "pause"
	TypeResume        = "resume"
	TypeDeviceInfo    = "device.info"
	TypeSessionNew    = "session.new"
	TypeSessionStop   = "session.stop"
	TypeInterrupt     = "agent.interrupt"
	TypeChatSend      = "chat.send"
	TypeCaptureReq    = "capture.request"
	TypePermReply     = "permission.reply"
	TypeTerminalInput = "terminal.input"
	TypeUIResponse    = "ui.response"
)

// Hello is the first message of the app.
type Hello struct {
	Protocol     int    `json:"protocol"` // the app's protocol version (ProtocolVersion when both match)
	Token        string `json:"token"`
	LastSeq      uint64 `json:"lastSeq"`
	AppVersion   string `json:"appVersion"`
	DeviceSerial string `json:"deviceSerial"`
}

// Welcome answers a valid hello. FromSeq is the first event that follows.
type Welcome struct {
	Protocol    int    `json:"protocol"` // the host's protocol version; the app checks its own minimum
	Session     string `json:"session"`
	FromSeq     uint64 `json:"fromSeq"`
	HostVersion string `json:"hostVersion"`
}

// SessionState is the state the app shows in its header.
type SessionState struct {
	Agent    string `json:"agent"`
	State    string `json:"state"` // idle, working, waiting or stopped
	Activity string `json:"activity,omitempty"`
	Paused   bool   `json:"paused"`
}

// ConfirmRequest asks the app to allow or deny a 🔴 command (spec §4.5).
// Title and Detail come from the rules, never from the agent.
type ConfirmRequest struct {
	ID           string    `json:"id"`
	Level        string    `json:"level"`
	Title        string    `json:"title"`
	Detail       string    `json:"detail"`
	RawCommand   string    `json:"rawCommand"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Special      bool      `json:"special"`
	AllowSession bool      `json:"allowSession"` // whether "allow during the session" may be offered
}

// ConfirmReply is the app's answer.
type ConfirmReply struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // allow, deny or allow_session
}

// Snapshot replaces the events the app missed when there are too many.
type Snapshot struct {
	State   SessionState      `json:"state"`
	Pending []ConfirmRequest  `json:"pending"`
	Journal []json.RawMessage `json:"journal"` // the last journal entries, oldest first
}

// Notice is a message for the person ("disconnected from the agent"…).
type Notice struct {
	Level string `json:"level"` // info, warning or error
	Text  string `json:"text"`
}

// NewToken returns a random 256-bit token in hex (spec §5.2).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

`host/internal/bridge/server.go`:

```go
package bridge

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	helloTimeout = 5 * time.Second
	maxMessage   = 1 << 20 // text messages; attachments (binary frames) arrive in M3
	sendQueue    = 256
	pingEvery    = 20 * time.Second
)

// Options configure a Server.
type Options struct {
	Token       string
	Session     string
	HostVersion string
	// OnMessage receives the app's messages after a valid hello, one at a
	// time. Its payload comes from the app: validate before use.
	OnMessage func(Message)
	// Snapshot describes the current state when the app missed too much.
	Snapshot func() Snapshot
	// OnConnect is called with true when an app authenticates and false when it leaves.
	OnConnect func(bool)
	// OnReject is called when an authenticated app is too old to talk to.
	OnReject func(Hello)
	Now      func() time.Time
}

// Server is the WebSocket server of the app channel. It serves one app at a
// time: a new authenticated connection replaces the previous one.
type Server struct {
	o Options

	mu     sync.Mutex
	seq    uint64
	ring   []Message // the last BufferSize events, oldest first
	client *client
}

type client struct {
	send   chan Message
	cancel context.CancelFunc
}

// New returns a Server.
func New(o Options) *Server {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.OnMessage == nil {
		o.OnMessage = func(Message) {}
	}
	if o.OnConnect == nil {
		o.OnConnect = func(bool) {}
	}
	if o.OnReject == nil {
		o.OnReject = func(Hello) {}
	}
	if o.Snapshot == nil {
		o.Snapshot = func() Snapshot { return Snapshot{Pending: []ConfirmRequest{}, Journal: []json.RawMessage{}} }
	}
	return &Server{o: o}
}

// Listen opens the server's port on the loopback interface only (spec §5.2).
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// Serve serves /v1 on l until ctx ends.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1", s.handle)
	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
		s.mu.Lock()
		if s.client != nil {
			s.client.cancel()
		}
		s.mu.Unlock()
	}()
	err := hs.Serve(l)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Connected reports whether an authenticated app is connected.
func (s *Server) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil
}

// Publish sends an event to the app (if connected) and keeps it to resend
// after a reconnection.
func (s *Server) Publish(typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	m := Message{V: ProtocolVersion, Seq: s.seq, Type: typ, TS: s.o.Now().UTC(), Payload: raw}
	s.ring = append(s.ring, m)
	if len(s.ring) > BufferSize {
		s.ring = s.ring[len(s.ring)-BufferSize:]
	}
	if s.client != nil {
		select {
		case s.client.send <- m:
		default: // too slow: drop it; it reconnects and resumes from lastSeq
			s.client.cancel()
		}
	}
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	// No Origin exceptions: a web page on the phone cannot open this socket
	// (coder/websocket rejects a foreign Origin), and without the token no
	// other app gets past the hello.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(maxMessage)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	hello, err := readHello(ctx, c)
	if err != nil {
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	if subtle.ConstantTimeCompare([]byte(hello.Token), []byte(s.o.Token)) != 1 || s.o.Token == "" {
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}

	if hello.Protocol < MinAppProtocol {
		// Only after the token: nobody learns anything about the host without it.
		c.Close(websocket.StatusCode(CloseIncompatible), "update the ADBuddy app")
		s.o.OnReject(hello)
		return
	}

	cl := &client{send: make(chan Message, sendQueue), cancel: cancel}
	backlog, first := s.attach(cl, hello.LastSeq)
	defer s.detach(cl)
	s.o.OnConnect(true)
	defer s.o.OnConnect(false)

	welcome, _ := json.Marshal(Welcome{Protocol: ProtocolVersion, Session: s.o.Session, FromSeq: first, HostVersion: s.o.HostVersion})
	if err := write(ctx, c, Message{V: ProtocolVersion, Type: TypeWelcome, TS: s.o.Now().UTC(), Payload: welcome}); err != nil {
		return
	}
	for _, m := range backlog {
		if err := write(ctx, c, m); err != nil {
			return
		}
	}

	go func() {
		defer cancel()
		for {
			var m Message
			if err := readJSON(ctx, c, &m); err != nil {
				return
			}
			if m.V != ProtocolVersion || m.Type == "" || m.Type == TypeHello {
				continue
			}
			s.o.OnMessage(m)
		}
	}()
	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			c.Close(websocket.StatusGoingAway, "bye")
			return
		case m := <-cl.send:
			if err := write(ctx, c, m); err != nil {
				return
			}
		case <-tick.C:
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

// attach makes cl the connected app and returns what it missed: the events
// after lastSeq, or a snapshot when they are no longer kept (or the host
// restarted and the numbering began again).
func (s *Server) attach(cl *client, lastSeq uint64) (backlog []Message, first uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.cancel()
	}
	s.client = cl
	oldest := s.seq + 1
	if len(s.ring) > 0 {
		oldest = s.ring[0].Seq
	}
	if lastSeq > s.seq || lastSeq+1 < oldest {
		snap, _ := json.Marshal(s.o.Snapshot())
		// The snapshot takes the next number, so the app's lastSeq moves past
		// everything it replaces.
		s.seq++
		m := Message{V: ProtocolVersion, Seq: s.seq, Type: TypeSnapshot, TS: s.o.Now().UTC(), Payload: snap}
		s.ring = append(s.ring, m)
		if len(s.ring) > BufferSize {
			s.ring = s.ring[len(s.ring)-BufferSize:]
		}
		return []Message{m}, m.Seq
	}
	for _, m := range s.ring {
		if m.Seq > lastSeq {
			backlog = append(backlog, m)
		}
	}
	return backlog, lastSeq + 1
}

func (s *Server) detach(cl *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == cl {
		s.client = nil
	}
}

func readHello(ctx context.Context, c *websocket.Conn) (Hello, error) {
	hctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	var m Message
	if err := readJSON(hctx, c, &m); err != nil {
		return Hello{}, err
	}
	if m.V != ProtocolVersion || m.Type != TypeHello {
		return Hello{}, fmt.Errorf("expected hello v%d, got %q v%d", ProtocolVersion, m.Type, m.V)
	}
	var h Hello
	if err := json.Unmarshal(m.Payload, &h); err != nil {
		return Hello{}, err
	}
	return h, nil
}

func readJSON(ctx context.Context, c *websocket.Conn, v any) error {
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageText {
			continue // binary frames (attachments) arrive in M3
		}
		return json.Unmarshal(data, v)
	}
}

func write(ctx context.Context, c *websocket.Conn, m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, data)
}
```

`host/internal/bridge/confirm.go`:

```go
package bridge

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// Publisher is the part of Server a Confirmer uses.
type Publisher interface {
	Connected() bool
	Publish(typ string, payload any) error
}

// Confirmer asks on the phone (spec §4.5). When no app is connected at the
// moment of asking it uses Fallback (the terminal, or deny).
type Confirmer struct {
	App      Publisher
	Fallback confirm.Confirmer

	mu      sync.Mutex
	pending map[string]pendingReq
	session map[string]bool // keys allowed for the rest of the session
}

type pendingReq struct {
	req   ConfirmRequest
	reply chan confirm.Decision
	key   string
}

// NewConfirmer returns a Confirmer that asks on app, or fallback.
func NewConfirmer(app Publisher, fallback confirm.Confirmer) *Confirmer {
	return &Confirmer{App: app, Fallback: fallback, pending: map[string]pendingReq{}, session: map[string]bool{}}
}

// Confirm implements confirm.Confirmer.
func (c *Confirmer) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	// "Allow during the session" covers only the same rule with the same
	// arguments, and never a command that may cut the connection.
	allowSession := r.Key != "" && !r.Special
	c.mu.Lock()
	if allowSession && c.session[r.Key] {
		c.mu.Unlock()
		return confirm.Allow
	}
	c.mu.Unlock()
	if !c.App.Connected() {
		return c.Fallback.Confirm(ctx, r)
	}
	req := ConfirmRequest{ID: r.ID, Level: "red", Title: r.Title, Detail: r.Detail, RawCommand: r.RawCommand,
		ExpiresAt: r.ExpiresAt.UTC(), Special: r.Special, AllowSession: allowSession}
	p := pendingReq{req: req, reply: make(chan confirm.Decision, 1), key: r.Key}
	c.mu.Lock()
	c.pending[r.ID] = p
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, r.ID)
		c.mu.Unlock()
	}()
	if err := c.App.Publish(TypeConfirmReq, req); err != nil {
		return confirm.Deny
	}
	select {
	case d := <-p.reply:
		return d
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return confirm.Timeout
		}
		return confirm.Deny
	}
}

// Reply handles a confirm.reply from the app. Replies for unknown or already
// answered ids are ignored.
func (c *Confirmer) Reply(raw json.RawMessage) {
	var rep ConfirmReply
	if json.Unmarshal(raw, &rep) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pending[rep.ID]
	if !ok {
		return
	}
	d := confirm.Deny
	switch rep.Decision {
	case "allow":
		d = confirm.Allow
	case "allow_session":
		d = confirm.Allow
		if p.req.AllowSession {
			c.session[p.key] = true
		}
	}
	delete(c.pending, rep.ID)
	p.reply <- d
}

// Pending lists the questions waiting for an answer, oldest first, for a snapshot.
func (c *Confirmer) Pending() []ConfirmRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ConfirmRequest, 0, len(c.pending))
	for _, p := range c.pending {
		out = append(out, p.req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
```

`host/internal/confirm/confirm.go`. Cambio:

Antes:

```go
	Special    bool // may cut the connection or affect ADBuddy
}
```

Después:

```go
	Special    bool   // may cut the connection or affect ADBuddy
	Key        string // the rule and the exact arguments: what "allow during the session" covers
}
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test ./internal/bridge/ ./internal/confirm/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): canal WebSocket con la app, con token, reanudación y confirmaciones en el móvil

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 2: Pausa y clave de confirmación en el daemon

§4.6 "Pausar": mientras la pausa está activa, el daemon rechaza cualquier comando del shim (126, entrada `blocked` en el diario) y cualquier undo (`PAUSED`). Además rellena `Key` en cada confirmación, para que "Permitir durante la sesión" cubra exactamente la misma regla con los mismos argumentos.

**Ficheros:**
- Modificar: `host/internal/daemon/daemon.go`
- Modificar: `host/internal/daemon/undo.go`
- Test: `host/internal/daemon/pause_test.go`

**Interfaces:**
- Consume: `confirm.Request.Key` (tarea 1).
- Produce (paquete `daemon`): `Options.Paused func() bool` (nil = nunca); `PausedMessage`.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/daemon/pause_test.go`:

```go
package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

func TestPausedRefusesEverything(t *testing.T) {
	e := newEnv(t)
	paused := true
	e.d.o.Paused = func() bool { return paused }
	s, exit := e.exec(t, "shell", "getprop", "ro.product.model")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") {
		t.Fatalf("green while paused: %+v %q", exit, s.errOut.String())
	}
	if len(e.adb.runs) != 0 {
		t.Fatalf("adb ran while paused: %v", e.adb.runs)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "paused") {
		t.Fatalf("journal %+v", j)
	}
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if u.Code != ExitDenied || u.ErrorCode != "PAUSED" {
		t.Fatalf("undo while paused: %+v", u)
	}
	paused = false
	e.adb.out["shell getprop ro.product.model"] = "Pixel\n"
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("after resume: %+v", exit)
	}
}

func TestConfirmationKeyIsRuleAndExactArguments(t *testing.T) {
	e := newEnv(t)
	e.exec(t, "shell", "reboot")
	e.exec(t, "shell", "reboot", "bootloader")
	if len(e.conf.asked) != 2 {
		t.Fatalf("asked %d", len(e.conf.asked))
	}
	a, b := e.conf.asked[0].Key, e.conf.asked[1].Key
	if a == "" || a == b || !strings.HasPrefix(a, e.last(t).Rule) {
		t.Fatalf("keys %q %q", a, b)
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test ./internal/daemon/
```

Esperado: no compila: `e.d.o.Paused undefined`; y, con el campo, el test falla porque se ejecuta adb estando en pausa.

- [ ] **Paso 3: Implementar**

`host/internal/daemon/daemon.go`. Cambio:

Antes:

```go
	UndoneLog string
	Now       func() time.Time
}
```

Después:

```go
	UndoneLog string
	// Paused reports whether the person paused the agent from the phone:
	// then every command and undo is refused (spec §4.6). Nil: never paused.
	Paused func() bool
	Now    func() time.Time
}
```

`host/internal/daemon/daemon.go`. Cambio:

Antes:

```go
	if o.OnWarning == nil {
		o.OnWarning = func(string) {}
	}
```

Después:

```go
	if o.OnWarning == nil {
		o.OnWarning = func(string) {}
	}
	if o.Paused == nil {
		o.Paused = func() bool { return false }
	}
```

`host/internal/daemon/daemon.go`. Cambio:

Antes:

```go
	g, args := classify.SplitGlobal(req.Argv)
	if len(g.Forbidden) > 0 {
```

Después:

```go
	if d.o.Paused() {
		return d.block(e, s, PausedMessage)
	}
	g, args := classify.SplitGlobal(req.Argv)
	if len(g.Forbidden) > 0 {
```

`host/internal/daemon/daemon.go`. Cambio:

Antes:

```go
			ExpiresAt: now.Add(d.o.ConfirmTimeout), Special: res.Special,
		})
```

Después:

```go
			ExpiresAt: now.Add(d.o.ConfirmTimeout), Special: res.Special,
			Key: e.Rule + "\x00" + strings.Join(req.Argv, "\x00"),
		})
```

`host/internal/daemon/daemon.go`. Cambio:

Antes:

```go
// block refuses a command without running it.
```

Después:

```go
// PausedMessage explains why nothing runs while paused.
const PausedMessage = "paused from the phone: nothing runs until you resume it there"

// block refuses a command without running it.
```

`host/internal/daemon/undo.go`. Cambio:

Antes:

```go
	d.changes.Lock()
	defer d.changes.Unlock()
	entries, _, err := d.o.Journal.Load()
```

Después:

```go
	if d.o.Paused() {
		return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: PausedMessage}
	}
	d.changes.Lock()
	defer d.changes.Unlock()
	entries, _, err := d.o.Journal.Load()
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test ./internal/daemon/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): pausa desde el móvil y clave de confirmación por regla y argumentos

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 3: El canal de la app queda fuera del alcance del agente

El `TokenProvider` solo distingue al usuario shell, y el agente también es el usuario shell. Por eso el clasificador del host protege el canal: `content …` sobre `tech.hybriq.adbuddy` (incluso `query`/`read`, y también con `%2e` o mayúsculas) y `run-as tech.hybriq.adbuddy …` son siempre 🔴 especial. Sin esto, el agente podría cambiar el token de la app o leer sus ficheros privados en una compilación de depuración.

**Ficheros:**
- Modificar: `host/internal/classify/special.go`
- Modificar: `host/internal/classify/shell.go`
- Test: `host/internal/classify/app_channel_test.go`

**Interfaces:**
- Consume y produce: nada nuevo; cambia `specialReason` (`special.go`) y `evalWords` (`shell.go`).

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/classify/app_channel_test.go`:

```go
package classify

import "testing"

func TestTheAppChannelIsOffLimits(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"content call --uri content://tech.hybriq.adbuddy.bridge --method handshake --extra token:s:x",
		"content call --uri content://tech.hybriq.adbuddy.bridge --method getSetupConsent",
		"content write --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content read --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content query --uri content://tech.hybriq.adbuddy.bridge",
		"content query --uri content://tech.hybriq.adbuddy%2ebridge",
		"content query --uri content://TECH.HYBRIQ.ADBUDDY.bridge",
		"run-as tech.hybriq.adbuddy cat shared_prefs/bridge.xml",
		"run-as tech.hybriq.adbuddy ls",
		"am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP",
		"am force-stop tech.hybriq.adbuddy",
		"pm revoke tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS",
		"pm disable-user tech.hybriq.adbuddy",
		"appops set tech.hybriq.adbuddy RUN_IN_BACKGROUND ignore",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	for _, line := range []string{
		"pm list packages tech.hybriq.adbuddy",
		"dumpsys package tech.hybriq.adbuddy",
		"content query --uri content://settings/global",
	} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", line, r.Level, r.Primary.Reason)
		}
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test ./internal/classify/
```

Esperado: `TestTheAppChannelIsOffLimits` falla: `content query …tech.hybriq.adbuddy.bridge` y `run-as tech.hybriq.adbuddy …` salen 🟢.

- [ ] **Paso 3: Implementar**

`host/internal/classify/special.go`. Cambio:

Antes:

```go
	case "content":
		// Uri.getAuthority() decodes %xx, so compare the decoded words.
		if level != Green && strings.Contains(strings.ToLower(percentDecode(strings.Join(v, " "))), "settings") {
```

Después:

```go
	case "content":
		// ADBuddy's own channel (token, consent): even reading it is off
		// limits for the agent (its private files: see run-as in shell.go).
		// Uri.getAuthority() decodes %xx, so compare the decoded words.
		if strings.Contains(strings.ToLower(percentDecode(strings.Join(v, " "))), ADBuddyPackage) {
			return "talks to the ADBuddy app's own channel or files"
		}
		if level != Green && strings.Contains(strings.ToLower(percentDecode(strings.Join(v, " "))), "settings") {
```

`host/internal/classify/shell.go`. Cambio:

Antes:

```go
	if inner, handled, reason := unwrap(ws); handled {
		if reason != "" {
```

Después:

```go
	if ws[0].Value == "run-as" && len(ws) > 1 && strings.Contains(strings.ToLower(ws[1].Value), ADBuddyPackage) {
		f := redFinding("run-as.adbuddy", "reads or changes the ADBuddy app's private files", vals, textUnknown)
		f.Special = true
		return []Finding{f}
	}
	if inner, handled, reason := unwrap(ws); handled {
		if reason != "" {
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test ./internal/classify/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): el agente no puede usar el canal ni los ficheros de la app

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 4: La app desde el host (`phoneapp`)

Los comandos que el propio host ejecuta en el móvil (§3.1 pasos 5-7), nunca a través del shim:
- `dumpsys package` para saber la versión instalada;
- `adb install -r`;
- `content call` (`requestSetup`, `getSetupConsent`, `ping`);
- `am start` de la pantalla de consentimiento;
- `pm grant … WRITE_SECURE_SETTINGS`;
- `dumpsys deviceidle whitelist +…`: sin optimización de batería, la app puede arrancar su servicio en primer plano cuando recibe el token, y el canal sigue vivo con la pantalla apagada;
- añadir el servicio de accesibilidad sin quitar los que ya hubiera;
- `adb reverse`;
- la entrega del token.

El token va por stdin a `content write …/handshake`, nunca en una línea de comandos: en el PC y en el móvil otros procesos, el agente incluido, pueden leer las líneas de comandos, y con el token el agente podría aprobarse sus propias confirmaciones. Tras escribirlo se comprueba con `ping` (con reintentos) que la app lo tomó. `Download` baja el APK de la última release y comprueba su SHA-256.

**Ficheros:**
- Crear: `host/internal/phoneapp/phoneapp.go`
- Crear: `host/internal/phoneapp/release.go`
- Test: `host/internal/phoneapp/phoneapp_test.go`

**Interfaces:**
- Produce (paquete `phoneapp`): `Package`, `Authority`, `Activity`, `SetupAction`, `A11yService`, `SecureWrites`; `ADB` (interfaz como `adb.Runner`); `InstalledVersion`, `Install`, `Call`, `AskConsent(ctx, a, host, timeout, every)`, `ErrConsentDenied`, `ErrConsentTimeout`, `Grant`, `AllowBackground`, `EnableAccessibility` (devuelve la lista anterior), `MergeServices`, `Tunnel`, `TunnelOpen`, `Handshake(ctx, a, token, port, host)`; `ReleaseURL`, `APKName`, `Download(ctx, client, base, dir)`, `ErrNoRelease`, `ErrChecksum`; `HostName(name)`.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/phoneapp/phoneapp_test.go`:

```go
package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeADB struct {
	answers map[string][]string // joined args -> successive stdouts (the last one repeats)
	calls   []string
	stdin   string // what the last call read from stdin
}

func (f *fakeADB) Run(_ context.Context, _ string, args []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	key := strings.Join(args, " ")
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	f.calls = append(f.calls, key)
	outs, ok := f.answers[key]
	if !ok {
		return 1, nil
	}
	io.WriteString(stdout, outs[0])
	if len(outs) > 1 {
		f.answers[key] = outs[1:]
	}
	return 0, nil
}

func TestInstalledVersion(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell dumpsys package tech.hybriq.adbuddy": {"Packages:\r\n  Package [tech.hybriq.adbuddy] (1a2b):\r\n    versionCode=3 minSdk=30\r\n    versionName=0.2.0\r\n"}}}
	v, err := InstalledVersion(context.Background(), f)
	if err != nil || v != "0.2.0" {
		t.Fatalf("%q %v", v, err)
	}
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Unable to find package: tech.hybriq.adbuddy\n"}
	if v, _ := InstalledVersion(context.Background(), f); v != "" {
		t.Fatalf("not installed: %q", v)
	}
}

func TestCallParsesTheBundle(t *testing.T) {
	key := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method requestSetup --extra 'host:s:My PC'"
	f := &fakeADB{answers: map[string][]string{key: {"Result: Bundle[{ok=true, appVersion=0.2.0}]\r\n"}}}
	res, err := Call(context.Background(), f, "requestSetup", "host:s:My PC")
	if err != nil || res["appVersion"] != "0.2.0" {
		t.Fatal(res, err, f.calls)
	}
	f.answers[key] = []string{"Result: Bundle[{error=not the shell user}]\n"}
	if _, err := Call(context.Background(), f, "requestSetup", "host:s:My PC"); err == nil || !strings.Contains(err.Error(), "not the shell user") {
		t.Fatalf("refusal: %v", err)
	}
	f.answers[key] = []string{"Error while accessing provider\n"}
	if _, err := Call(context.Background(), f, "requestSetup", "host:s:My PC"); err == nil {
		t.Fatal("no bundle accepted")
	}
}

func TestHandshakeSendsTheTokenOnStdinOnly(t *testing.T) {
	ping := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method ping"
	f := &fakeADB{answers: map[string][]string{
		"shell content write --uri content://tech.hybriq.adbuddy.bridge/handshake": {""},
		ping: {"Result: Bundle[{ok=true, port=0}]\n", "Result: Bundle[{ok=true, port=47123}]\n"},
	}}
	pingEvery = time.Millisecond
	if err := Handshake(context.Background(), f, "s3cret", 47123, "My PC"); err != nil {
		t.Fatal(err, f.calls)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "s3cret") {
			t.Fatalf("the token is in a command line: %q", c)
		}
	}
	if !strings.Contains(f.stdin, `"token":"s3cret"`) || !strings.Contains(f.stdin, `"port":47123`) {
		t.Fatalf("stdin %q", f.stdin)
	}
	f.answers[ping] = []string{"Result: Bundle[{ok=true, port=1}]\n"}
	if err := Handshake(context.Background(), f, "s3cret", 47123, "My PC"); err == nil {
		t.Fatal("a handshake the app did not take was accepted")
	}
}

func TestAskConsent(t *testing.T) {
	call := func(m string, extra ...string) string {
		s := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method " + m
		for _, e := range extra {
			s += " --extra " + e
		}
		return s
	}
	answers := func(consents ...string) map[string][]string {
		var outs []string
		for _, c := range consents {
			outs = append(outs, "Result: Bundle[{consent="+c+"}]\n")
		}
		return map[string][]string{
			call("requestSetup", "'host:s:My PC'"):                                                    {"Result: Bundle[{ok=true}]\n"},
			"shell am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP": {"Starting: Intent\n"},
			call("getSetupConsent"):                                                                   outs,
		}
	}
	f := &fakeADB{answers: answers("pending", "pending", "granted")}
	if err := AskConsent(context.Background(), f, "My PC", time.Second, time.Millisecond); err != nil {
		t.Fatal(err, f.calls)
	}
	f = &fakeADB{answers: answers("pending", "denied")}
	if err := AskConsent(context.Background(), f, "My PC", time.Second, time.Millisecond); !errors.Is(err, ErrConsentDenied) {
		t.Fatalf("denied: %v", err)
	}
	f = &fakeADB{answers: answers("pending")}
	if err := AskConsent(context.Background(), f, "My PC", 20*time.Millisecond, time.Millisecond); !errors.Is(err, ErrConsentTimeout) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestEnableAccessibilityKeepsOtherServices(t *testing.T) {
	get := "shell settings get secure enabled_accessibility_services"
	on := "shell settings put secure accessibility_enabled 1"
	cases := map[string]string{
		"null":                      A11yService,
		"":                          A11yService,
		"com.x/.Svc":                "com.x/.Svc:" + A11yService,
		"com.x/.Svc:" + A11yService: "",
	}
	for prev, want := range cases {
		put := "shell settings put secure enabled_accessibility_services " + want
		f := &fakeADB{answers: map[string][]string{get: {prev + "\n"}, put: {""}, on: {""}}}
		got, err := EnableAccessibility(context.Background(), f)
		if err != nil || got != prev {
			t.Errorf("%q: %q %v %v", prev, got, err, f.calls)
		}
		wrote := false
		for _, c := range f.calls {
			wrote = wrote || strings.HasPrefix(c, "shell settings put secure enabled_accessibility_services")
		}
		if wrote != (want != "") {
			t.Errorf("%q: wrote=%v calls=%v", prev, wrote, f.calls)
		}
		if f.calls[len(f.calls)-1] != on {
			t.Errorf("%q: accessibility not turned on: %v", prev, f.calls)
		}
	}
}

func TestAllowBackground(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell dumpsys deviceidle whitelist +tech.hybriq.adbuddy": {"Added: tech.hybriq.adbuddy\n"}}}
	if err := AllowBackground(context.Background(), f); err != nil {
		t.Fatal(err, f.calls)
	}
}

func TestTunnel(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"reverse tcp:47123 tcp:47123": {""},
		"reverse --list":              {"UsbFfs tcp:47123 tcp:47123\n", "\n"},
	}}
	if err := Tunnel(context.Background(), f, 47123); err != nil {
		t.Fatal(err)
	}
	if ok, _ := TunnelOpen(context.Background(), f, 47123); !ok {
		t.Fatal("open tunnel not seen")
	}
	if ok, _ := TunnelOpen(context.Background(), f, 47123); ok {
		t.Fatal("closed tunnel seen as open")
	}
}

func TestInstall(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"install -r /tmp/a.apk": {"Performing Streamed Install\nSuccess\n"}}}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	f.answers["install -r /tmp/a.apk"] = []string{"Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err == nil {
		t.Fatal("failure accepted")
	}
}

func TestDownloadChecksTheSHA256(t *testing.T) {
	apk := []byte("PK fake apk")
	sum := sha256.Sum256(apk)
	good := hex.EncodeToString(sum[:])
	var sidecar string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adbuddy.apk":
			w.Write(apk)
		case "/adbuddy.apk.sha256":
			io.WriteString(w, sidecar)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	sidecar = good + "  adbuddy.apk\n"
	p, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(apk) || p != filepath.Join(dir, "adbuddy.apk") {
		t.Fatalf("%s %q", p, b)
	}
	sidecar = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir()); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad sum: %v", err)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/none/", t.TempDir()); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no release: %v", err)
	}
}

func TestHostName(t *testing.T) {
	for in, want := range map[string]string{"DESKTOP-AB12": "DESKTOP-AB12", "pc; reboot": "pc reboot", "$(x)": "x", "": "PC", "ñ": "PC"} {
		if got := HostName(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test ./internal/phoneapp/
```

Esperado: no compila: `undefined: InstalledVersion`, `undefined: Handshake`…

- [ ] **Paso 3: Implementar**

`host/internal/phoneapp/phoneapp.go`:

```go
// Package phoneapp manages the ADBuddy app on the phone from the host (spec
// §3.1 steps 5-7): install it with `adb install`, ask for consent in the app,
// grant WRITE_SECURE_SETTINGS, enable the accessibility service without
// removing the ones already on, open the tunnel (`adb reverse`) and hand over
// the token. These commands are the host's own: they never go through the
// shim, and the agent's attempts to do the same are blocked (package classify).
package phoneapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Identifiers shared with the app (android/): keep them in sync.
const (
	Package      = "tech.hybriq.adbuddy"
	Authority    = "tech.hybriq.adbuddy.bridge"
	Activity     = Package + "/.MainActivity"
	SetupAction  = "tech.hybriq.adbuddy.action.SETUP"
	A11yService  = Package + "/" + Package + ".a11y.AgentAccessibilityService"
	SecureWrites = "android.permission.WRITE_SECURE_SETTINGS"
)

// ADB runs adb against the active phone (an adb.Runner with Serial set).
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

func run(ctx context.Context, a ADB, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", args, nil, &out, &errb)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(out.String(), "\r", "")
	if code != 0 {
		return text, fmt.Errorf("adb %s: exit %d: %s", args[0], code, strings.TrimSpace(errb.String()+" "+text))
	}
	return text, nil
}

func shell(ctx context.Context, a ADB, words ...string) (string, error) {
	out, err := run(ctx, a, "shell", shq.Join(words...))
	return strings.TrimSpace(out), err
}

// InstalledVersion returns the app's versionName, or "" when it is not installed.
func InstalledVersion(ctx context.Context, a ADB) (string, error) {
	out, err := shell(ctx, a, "dumpsys", "package", Package)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "versionName="); ok {
			return v, nil
		}
	}
	return "", nil
}

// Install installs (or updates, keeping data) the APK at path.
func Install(ctx context.Context, a ADB, path string) error {
	out, err := run(ctx, a, "install", "-r", path)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("adb install: %s", strings.TrimSpace(out))
	}
	return nil
}

// bundleRe reads `content call` output: "Result: Bundle[{a=1, b=x}]".
var bundleRe = regexp.MustCompile(`Bundle\[\{(.*)\}\]`)

// Call calls a method of the app's provider and returns the result bundle.
// Only the shell user (adb) may call it; the app refuses anyone else.
func Call(ctx context.Context, a ADB, method string, extras ...string) (map[string]string, error) {
	words := []string{"content", "call", "--uri", "content://" + Authority, "--method", method}
	for _, e := range extras {
		words = append(words, "--extra", e)
	}
	out, err := shell(ctx, a, words...)
	if err != nil {
		return nil, err
	}
	m := bundleRe.FindStringSubmatch(out)
	if m == nil {
		return nil, fmt.Errorf("the app did not answer %s: %s", method, out)
	}
	res := map[string]string{}
	for _, kv := range strings.Split(m[1], ", ") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			res[k] = v
		}
	}
	if e := res["error"]; e != "" {
		return res, fmt.Errorf("the app refused %s: %s", method, e)
	}
	return res, nil
}

// Consent is the answer of the consent screen.
type Consent string

const (
	ConsentPending Consent = "pending"
	ConsentGranted Consent = "granted"
	ConsentDenied  Consent = "denied"
)

// ErrConsentDenied means the person said no in the app.
var ErrConsentDenied = errors.New("setup was refused in the app")

// ErrConsentTimeout means nobody answered in time.
var ErrConsentTimeout = errors.New("no answer in the app")

// AskConsent opens the consent screen (spec §3.1 step 6.1) and waits for the answer.
func AskConsent(ctx context.Context, a ADB, hostName string, timeout, every time.Duration) error {
	if _, err := Call(ctx, a, "requestSetup", "host:s:"+hostName); err != nil {
		return err
	}
	if _, err := shell(ctx, a, "am", "start", "-n", Activity, "-a", SetupAction); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		res, err := Call(ctx, a, "getSetupConsent")
		if err != nil {
			return err
		}
		switch Consent(res["consent"]) {
		case ConsentGranted:
			return nil
		case ConsentDenied:
			return ErrConsentDenied
		}
		if time.Now().After(deadline) {
			return ErrConsentTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

// Grant gives the app WRITE_SECURE_SETTINGS (for "Cut access").
func Grant(ctx context.Context, a ADB) error {
	_, err := shell(ctx, a, "pm", "grant", Package, SecureWrites)
	return err
}

// AllowBackground exempts the app from battery optimisation, so it may start
// its foreground service when the host hands over the token and keep the
// channel open while the screen is off (Android 12+ restricts both).
func AllowBackground(ctx context.Context, a ADB) error {
	_, err := shell(ctx, a, "dumpsys", "deviceidle", "whitelist", "+"+Package)
	return err
}

// EnableAccessibility adds the app's service to the enabled ones without
// removing any (spec §3.1 step 6.2) and turns accessibility on. It returns
// the previous list so the caller can report it.
func EnableAccessibility(ctx context.Context, a ADB) (previous string, err error) {
	previous, err = shell(ctx, a, "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return "", err
	}
	list := MergeServices(previous, A11yService)
	if list != previous {
		if _, err := shell(ctx, a, "settings", "put", "secure", "enabled_accessibility_services", list); err != nil {
			return previous, err
		}
	}
	_, err = shell(ctx, a, "settings", "put", "secure", "accessibility_enabled", "1")
	return previous, err
}

// MergeServices adds svc to a colon-separated list of components ("null"
// or empty = none) unless it is already there.
func MergeServices(list, svc string) string {
	if list == "" || list == "null" {
		return svc
	}
	for _, s := range strings.Split(list, ":") {
		if strings.EqualFold(s, svc) {
			return list
		}
	}
	return list + ":" + svc
}

// Tunnel makes 127.0.0.1:port on the phone reach 127.0.0.1:port on the PC.
func Tunnel(ctx context.Context, a ADB, port int) error {
	p := "tcp:" + strconv.Itoa(port)
	_, err := run(ctx, a, "reverse", p, p)
	return err
}

// TunnelOpen reports whether the reverse for port is in place.
func TunnelOpen(ctx context.Context, a ADB, port int) (bool, error) {
	out, err := run(ctx, a, "reverse", "--list")
	if err != nil {
		return false, err
	}
	want := "tcp:" + strconv.Itoa(port) + " tcp:" + strconv.Itoa(port)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), want) {
			return true, nil
		}
	}
	return false, nil
}

// Handshake hands the token and port to the app (spec §5.2). They travel on
// stdin (`content write`), never in a command line: on the PC and on the
// phone other processes, the agent's included, can read command lines, and
// with the token the agent could answer its own confirmations.
func Handshake(ctx context.Context, a ADB, token string, port int, hostName string) error {
	body, err := json.Marshal(map[string]any{"token": token, "port": port, "host": hostName})
	if err != nil {
		return err
	}
	var out, errb bytes.Buffer
	cmd := shq.Join("content", "write", "--uri", "content://"+Authority+"/handshake")
	code, err := a.Run(ctx, "", []string{"shell", cmd}, bytes.NewReader(body), &out, &errb)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the app refused the handshake: %s", strings.TrimSpace(errb.String()))
	}
	// Whether the app took it is checked with ping: `content write` does not
	// always report errors in its exit code.
	// The app stores the handshake on its own thread once the pipe closes:
	// give it a moment before deciding it did not take it.
	var got string
	for i := 0; i < pingTries; i++ {
		res, err := Call(ctx, a, "ping")
		if err != nil {
			return err
		}
		if got = res["port"]; got == strconv.Itoa(port) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pingEvery):
		}
	}
	return fmt.Errorf("the app did not take the handshake (port %q)", got)
}

// How long Handshake waits for the app to take the token.
var (
	pingTries = 10
	pingEvery = 200 * time.Millisecond
)
```

`host/internal/phoneapp/release.go`:

```go
package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ReleaseURL is where the latest APK of the app is published (spec §3.1 step 5).
const ReleaseURL = "https://github.com/hybriqtech/adbuddy/releases/latest/download/"

// APKName is the release asset; APKName+".sha256" holds its SHA-256.
const APKName = "adbuddy.apk"

// maxAPK bounds the download.
const maxAPK = 200 << 20

// ErrNoRelease means there is no published APK yet.
var ErrNoRelease = errors.New("no published release of the app")

// ErrChecksum means the downloaded APK does not match its published SHA-256.
var ErrChecksum = errors.New("the APK does not match its SHA-256")

// Download fetches the latest APK into dir, checks it against the published
// SHA-256 and returns its path. base is ReleaseURL (or a test server).
func Download(ctx context.Context, client *http.Client, base, dir string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	sum, err := get(ctx, client, base+APKName+".sha256", 1024)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", fmt.Errorf("unexpected %s.sha256: %q", APKName, sum)
	}
	want := strings.ToLower(fields[0])
	data, err := get(ctx, client, base+APKName, maxAPK)
	if err != nil {
		return "", err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return "", ErrChecksum
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, APKName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", url, limit)
	}
	return data, nil
}

// HostName is the PC's name as the app shows it: letters, digits, spaces,
// dots, dashes and underscores only, at most 40 characters.
func HostName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if b.Len() >= 40 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		return s
	}
	return "PC"
}
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test ./internal/phoneapp/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): instalar y preparar la app del móvil; token por stdin y comprobado con ping

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 5: `serve` con la app: puerto, token, túnel, pausa y diario en el móvil

`chat_port` en `config.toml` (§5.2: se elige en el primer uso entre 20000 y 59999 y queda fijo). `serve`, si la app está instalada:
- genera un token nuevo;
- arranca el canal;
- abre el túnel y entrega el token;
- cada 15 s comprueba que el `reverse` sigue ahí (adb lo pierde al reconectar el móvil) y, si no, lo rehace;
- manda cada entrada del diario al móvil;
- aplica Pausar/Reanudar;
- confirma en el móvil cuando la app está conectada, y en la terminal cuando no.

Sin app, todo sigue como en M1. El extremo a extremo simula la app con un cliente WebSocket. Toma el token del registro del adb falso, que es su stdin, y comprueba que no aparece en ninguna línea de comandos.

**Ficheros:**
- Crear: `host/internal/cli/phone.go`
- Modificar: `host/internal/config/config.go`
- Modificar: `host/internal/cli/serve.go`
- Modificar: `host/cmd/adbuddy/e2e_test.go`
- Test: `host/internal/config/chatport_test.go`
- Test: `host/cmd/adbuddy/phone_e2e_test.go`

**Interfaces:**
- Consume: `bridge` (1), `daemon.Options.Paused` (2), `phoneapp` (4).
- Produce: `config.Config.ChatPort` (`chat_port`, 1024-65535 o 0); en `cli`: `chatPort(*env)`, `startPhoneLink(ctx, app, *env, adb, session, agent, fallback) (*phoneLink, error)` con `Confirmer()`, `Paused()`, `Entry(journal.Entry)`; en el e2e, `startWith(t, config, extra...)`.

> El test de extremo a extremo de M1 añade una llamada de solo lectura: `serve` pregunta con `dumpsys package tech.hybriq.adbuddy` si la app está instalada.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/config/chatport_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChatPort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	os.WriteFile(p, []byte("chat_port = 47123\n"), 0o600)
	c, err := Load(p)
	if err != nil || c.ChatPort != 47123 {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{"chat_port = 80\n", "chat_port = 70000\n"} {
		os.WriteFile(p, []byte(bad), 0o600)
		if _, err := Load(p); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
```

`host/cmd/adbuddy/phone_e2e_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// phone is a fake phone app speaking the bridge protocol.
type phone struct {
	t *testing.T
	c *websocket.Conn
}

func (p *phone) send(typ string, payload any) {
	p.t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: typ, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.c.Write(ctx, websocket.MessageText, data); err != nil {
		p.t.Fatal(err)
	}
}

// until reads messages until one of type typ arrives.
func (p *phone) until(typ string) bridge.Message {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := p.c.Read(ctx)
		if err != nil {
			p.t.Fatalf("waiting for %s: %v", typ, err)
		}
		var m bridge.Message
		json.Unmarshal(data, &m)
		if m.Type == typ {
			return m
		}
	}
}

func TestPhoneAppConfirmsAndPauses(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startWith(t, "chat_port = "+port+"\n",
		testutil.FakeRule{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n"},
		testutil.FakeRule{Match: `^reverse tcp:` + port + ` tcp:` + port + `$`},
		testutil.FakeRule{Match: `^reverse --list$`, Stdout: "dev1 tcp:" + port + " tcp:" + port + "\n"},
		testutil.FakeRule{Match: `^shell content write --uri content://tech\.hybriq\.adbuddy\.bridge/handshake$`, EchoStdin: true},
		testutil.FakeRule{Match: `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method ping$`, Stdout: "Result: Bundle[{ok=true, port=" + port + "}]\n"},
		testutil.FakeRule{Match: `^shell reboot$`},
	)

	// The token reached the app on stdin, never in a command line.
	var token string
	waitFor(t, func() bool {
		calls, _ := testutil.ReadLog(h.logPath)
		for _, c := range calls {
			if strings.Contains(c.Joined(), "content write") {
				var hs struct {
					Token string
					Port  int
				}
				json.Unmarshal([]byte(c.Stdin), &hs)
				token = hs.Token
			}
		}
		return token != ""
	}, h.serveErr)
	for _, c := range h.adbCalls() {
		if strings.Contains(c, token) {
			t.Fatalf("the token is in a command line: %q", c)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+port+"/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	p := &phone{t: t, c: c}
	p.send(bridge.TypeHello, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: token, AppVersion: "0.2.0"})
	p.until(bridge.TypeWelcome)
	waitFor(t, func() bool { return strings.Contains(h.serveErr.String(), "phone app is connected") }, h.serveErr)

	// A 🔴 command asks on the phone; allowing it runs it.
	done := make(chan int)
	go func() {
		_, _, code := h.adb("", "shell", "reboot")
		done <- code
	}()
	m := p.until(bridge.TypeConfirmReq)
	var req bridge.ConfirmRequest
	json.Unmarshal(m.Payload, &req)
	if req.RawCommand != "adb shell reboot" || !req.Special || req.AllowSession {
		t.Fatalf("request %+v", req)
	}
	p.send(bridge.TypeConfirmReply, bridge.ConfirmReply{ID: req.ID, Decision: "allow"})
	if code := <-done; code != 0 {
		t.Fatalf("allowed command exit %d", code)
	}
	for {
		m = p.until(bridge.TypeJournalEntry)
		var e struct{ ID, Status string }
		json.Unmarshal(m.Payload, &e)
		if e.ID == req.ID && e.Status == "ok" {
			break
		}
	}
	if strings.Contains(h.serveErr.String(), "Code: ") {
		t.Fatal("the terminal asked too")
	}

	// Pause refuses everything until resume.
	p.send(bridge.TypePause, struct{}{})
	p.until(bridge.TypeSessionState)
	if _, errOut, code := h.adb("", "shell", "getprop", "ro.product.model"); code != 126 || !strings.Contains(errOut, "paused") {
		t.Fatalf("paused: %d %q", code, errOut)
	}
	p.send(bridge.TypeResume, struct{}{})
	p.until(bridge.TypeSessionState)
	if out, _, code := h.adb("", "shell", "getprop", "ro.product.model"); code != 0 || out != "Pixel 9 Pro XL\n" {
		t.Fatalf("resumed: %d %q", code, out)
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test -count=1 ./...
```

Esperado: no compila: `unknown field ChatPort`, `undefined: startWith`, `undefined: startPhoneLink`.

- [ ] **Paso 3: Implementar**

`host/internal/cli/phone.go`:

```go
package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// tunnelCheck is how often serve checks that the chat tunnel is still in
// place: adb drops the reverse when the phone reconnects.
var tunnelCheck = 15 * time.Second

// keptEntries is how many journal entries a snapshot carries.
const keptEntries = 50

// phoneLink connects serve with the phone app (spec §5): the bridge server,
// confirmations on the phone and the pause.
type phoneLink struct {
	srv   *bridge.Server // nil when the app is not installed
	conf  *bridge.Confirmer
	log   io.Writer
	agent string

	mu      sync.Mutex
	paused  bool
	entries []json.RawMessage
}

// chatPort returns the configured port of the app channel, choosing and
// saving one on first use (spec §5.2: fixed, so the reverse never changes).
func chatPort(e *env) (int, error) {
	if e.cfg.ChatPort != 0 {
		return e.cfg.ChatPort, nil
	}
	for range 20 {
		n, err := rand.Int(rand.Reader, big.NewInt(40000))
		if err != nil {
			return 0, err
		}
		port := 20000 + int(n.Int64())
		l, err := bridge.Listen(port)
		if err != nil {
			continue // in use
		}
		l.Close()
		e.cfg.ChatPort = port
		if err := config.Save(e.dirs.ConfigFile(), e.cfg); err != nil {
			return 0, err
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free port found for the app channel")
}

// startPhoneLink starts the app channel when the app is installed. Without
// the app it returns a link that confirms with fallback and never pauses.
func startPhoneLink(ctx context.Context, app *App, e *env, a phoneapp.ADB, session, agentName string, fallback confirm.Confirmer) (*phoneLink, error) {
	l := &phoneLink{log: app.Stderr, agent: agentName}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if err != nil {
		return nil, NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
	}
	if version == "" {
		fmt.Fprintln(app.Stderr, "The ADBuddy app is not on the phone: 🔴 confirmations happen here. Run `adbuddy setup` to install it.")
		l.conf = bridge.NewConfirmer(noApp{}, fallback)
		return l, nil
	}
	port, err := chatPort(e)
	if err != nil {
		return nil, NewError("CHAT_PORT_FAILED", err.Error(), "Set chat_port in "+e.dirs.ConfigFile()+".")
	}
	token, err := bridge.NewToken()
	if err != nil {
		return nil, NewError("TOKEN_FAILED", err.Error(), "")
	}
	listener, err := bridge.Listen(port)
	if err != nil {
		return nil, NewError("CHAT_PORT_IN_USE", err.Error(), "Another program uses port "+fmt.Sprint(port)+": change chat_port in "+e.dirs.ConfigFile()+".")
	}
	l.srv = bridge.New(bridge.Options{
		Token: token, Session: session, HostVersion: Version,
		OnMessage: l.onMessage,
		Snapshot:  l.snapshot,
		OnReject: func(h bridge.Hello) {
			fmt.Fprintf(app.Stderr, "⚠ The ADBuddy app on the phone (%s) is too old for this host: run `adbuddy setup` to update it.\n", classify.Printable(h.AppVersion))
		},
		OnConnect: func(on bool) {
			if on {
				fmt.Fprintln(app.Stderr, "📱 The phone app is connected: 🔴 confirmations go to the phone.")
			} else {
				fmt.Fprintln(app.Stderr, "📱 The phone app disconnected: 🔴 confirmations happen here.")
			}
		},
	})
	l.conf = bridge.NewConfirmer(l.srv, fallback)
	go l.srv.Serve(ctx, listener)
	host, _ := os.Hostname()
	host = phoneapp.HostName(host)
	connect := func() error {
		if err := phoneapp.Tunnel(ctx, a, port); err != nil {
			return err
		}
		return phoneapp.Handshake(ctx, a, token, port, host)
	}
	if err := connect(); err != nil {
		fmt.Fprintf(app.Stderr, "⚠ Could not reach the phone app (%v): 🔴 confirmations happen here until it connects.\n", err)
	}
	go func() {
		t := time.NewTicker(tunnelCheck)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if ok, err := phoneapp.TunnelOpen(ctx, a, port); err == nil && !ok {
					if err := connect(); err != nil && ctx.Err() == nil {
						fmt.Fprintf(app.Stderr, "⚠ Could not reopen the phone app's channel: %v\n", err)
					}
				}
			}
		}
	}()
	return l, nil
}

// noApp is a Publisher with no app behind it.
type noApp struct{}

func (noApp) Connected() bool                     { return false }
func (noApp) Publish(string, any) error           { return nil }
func (l *phoneLink) Confirmer() confirm.Confirmer { return l.conf }

// Paused reports whether the person paused the agent from the phone.
func (l *phoneLink) Paused() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.paused
}

// Entry sends a journal entry to the app's activity panel.
func (l *phoneLink) Entry(j journal.Entry) {
	raw, err := json.Marshal(j)
	if err != nil {
		return
	}
	l.mu.Lock()
	l.entries = append(l.entries, raw)
	if len(l.entries) > keptEntries {
		l.entries = l.entries[len(l.entries)-keptEntries:]
	}
	l.mu.Unlock()
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeJournalEntry, json.RawMessage(raw))
	}
}

func (l *phoneLink) state() bridge.SessionState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bridge.SessionState{Agent: l.agent, State: "idle", Paused: l.paused}
}

func (l *phoneLink) snapshot() bridge.Snapshot {
	st := l.state()
	l.mu.Lock()
	entries := append([]json.RawMessage{}, l.entries...)
	l.mu.Unlock()
	return bridge.Snapshot{State: st, Pending: l.conf.Pending(), Journal: entries}
}

// onMessage handles what the app sends. Everything it carries comes from the
// phone: only known types with checked payloads do anything.
func (l *phoneLink) onMessage(m bridge.Message) {
	switch m.Type {
	case bridge.TypeConfirmReply:
		l.conf.Reply(m.Payload)
	case bridge.TypePause, bridge.TypeResume:
		l.mu.Lock()
		l.paused = m.Type == bridge.TypePause
		l.mu.Unlock()
		if m.Type == bridge.TypePause {
			fmt.Fprintln(l.log, "⏸ Paused from the phone: every adb command of the agent is refused until you resume.")
		} else {
			fmt.Fprintln(l.log, "▶ Resumed from the phone.")
		}
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	default:
		_ = l.srv.Publish(bridge.TypeNotice, bridge.Notice{Level: "info",
			Text: "This version of the ADBuddy host does not handle " + classify.Printable(m.Type) + " yet."})
	}
}
```

`host/internal/config/config.go`. Cambio:

Antes:

```go
	Language       string   `toml:"language"`        // "en" or "es"; empty = from LANG
}
```

Después:

```go
	Language       string   `toml:"language"`        // "en" or "es"; empty = from LANG
	ChatPort       int      `toml:"chat_port"`       // port of the app channel on 127.0.0.1 (PC and phone); 0 = chosen on first use
}
```

`host/internal/config/config.go`. Cambio:

Antes:

```go
	switch c.Language {
	case "", "en", "es":
```

Después:

```go
	if c.ChatPort != 0 && (c.ChatPort < 1024 || c.ChatPort > 65535) {
		return fmt.Errorf("chat_port must be between 1024 and 65535, got %d", c.ChatPort)
	}
	switch c.Language {
	case "", "en", "es":
```

`host/internal/cli/serve.go`. Cambio:

Antes:

```go
			var outMu sync.Mutex
			session := journal.NewID("s_", time.Now())
			d := daemon.New(daemon.Options{
				Classifier: cls, ADB: runner, Journal: journal.OpenSigned(workspace.JournalDir(ws), []byte(secret)), Confirmer: confirmer,
```

Después:

```go
			agentName := e.cfg.Agent
			if agentName == "" {
				agentName = agent.Known[0]
			}
			var outMu sync.Mutex
			session := journal.NewID("s_", time.Now())
			link, err := startPhoneLink(ctx, app, &e, runner, session, agentName, confirmer)
			if err != nil {
				return err
			}
			d := daemon.New(daemon.Options{
				Classifier: cls, ADB: runner, Journal: journal.OpenSigned(workspace.JournalDir(ws), []byte(secret)), Confirmer: link.Confirmer(),
				Paused: link.Paused,
```

`host/internal/cli/serve.go`. Cambio:

Antes:

```go
					if app.JSON {
						_ = writeJSON(app.Stdout, j)
					} else {
						fmt.Fprintln(app.Stdout, formatEntry(j))
					}
				},
```

Después:

```go
					if app.JSON {
						_ = writeJSON(app.Stdout, j)
					} else {
						fmt.Fprintln(app.Stdout, formatEntry(j))
					}
					link.Entry(j)
				},
```

`host/internal/cli/serve.go`. Cambio:

Antes:

```go
			agentName := e.cfg.Agent
			if agentName == "" {
				agentName = agent.Known[0]
			}
			fmt.Fprintf(app.Stderr, "ADBuddy is serving
```

Después:

```go
			fmt.Fprintf(app.Stderr, "ADBuddy is serving
```

`host/cmd/adbuddy/e2e_test.go`. Cambio:

Antes:

```go
func start(t *testing.T) *harness {
	t.Helper()
```

Después:

```go
func start(t *testing.T) *harness {
	t.Helper()
	return startWith(t, "")
}

// startWith starts serve with extra fake adb rules first and, when config is
// not empty, that config.toml.
func startWith(t *testing.T, config string, extra ...testutil.FakeRule) *harness {
	t.Helper()
```

`host/cmd/adbuddy/e2e_test.go`. Cambio:

Antes:

```go
	script, err := testutil.WriteScript(tmp,
		testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"},
```

Después:

```go
	rules := append(extra,
		testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"},
```

`host/cmd/adbuddy/e2e_test.go`. Cambio:

Antes:

```go
		testutil.FakeRule{Match: "^shell cat$", EchoStdin: true},
	)
	if err != nil {
		t.Fatal(err)
	}
```

Después:

```go
		testutil.FakeRule{Match: "^shell cat$", EchoStdin: true},
	)
	script, err := testutil.WriteScript(tmp, rules...)
	if err != nil {
		t.Fatal(err)
	}
	if config != "" {
		os.MkdirAll(filepath.Join(tmp, "cfg"), 0o700)
		if err := os.WriteFile(filepath.Join(tmp, "cfg", "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
```

`host/cmd/adbuddy/e2e_test.go`. Cambio:

Antes:

```go
	want := []string{"devices", "shell getprop service.adb.tcp.port", "shell getprop ro.product.model",
```

Después:

```go
	want := []string{"devices", "shell getprop service.adb.tcp.port", "shell dumpsys package tech.hybriq.adbuddy", "shell getprop ro.product.model",
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test -count=1 ./...
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): serve conecta con la app: token, túnel vigilado, pausa y confirmaciones en el móvil

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 6: `setup` instala y prepara la app

Los pasos 5-6 de §3.1 dentro de `adbuddy setup`, tras leer el perfil:
1. Si la app no está instalada, la instala: desde `--apk` o desde la última release, con SHA-256.
2. Abre la pantalla de consentimiento y espera la respuesta hasta 5 min.
3. Solo si se acepta: concede `WRITE_SECURE_SETTINGS`, la quita de la optimización de batería y activa su accesibilidad, conservando los servicios que ya hubiera.

La app es opcional. Sin release publicada, si no se instala, si se rechaza o si nadie responde, `setup` lo explica y termina bien; las 🔴 se confirmarán en la terminal. `--no-app` se salta todo esto.

**Ficheros:**
- Crear: `host/internal/cli/appsetup.go`
- Modificar: `host/internal/cli/setup.go`
- Modificar: `host/internal/cli/setup_test.go`
- Test: `host/internal/cli/appsetup_test.go`

**Interfaces:**
- Consume: `phoneapp` (4), `chatPort` (5).
- Produce: `adbuddy setup --apk <fichero> --no-app`; `setupResult.App` (`appResult{Version, Installed, Consent, Accessibility, Note}`, JSON `app`); `ADBUDDY_RELEASE_URL` para tests.

> Los tests de `setup` de M1b apuntan `ADBUDDY_RELEASE_URL` a un puerto cerrado, para que ninguno descargue nada de GitHub.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/cli/appsetup_test.go`:

```go
package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

func appRules(consent string) []testutil.FakeRule {
	provider := `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method `
	return append([]testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n"},
		{Match: provider + `requestSetup --extra `, Stdout: "Result: Bundle[{ok=true}]\n"},
		{Match: `^shell am start -n tech\.hybriq\.adbuddy/\.MainActivity -a tech\.hybriq\.adbuddy\.action\.SETUP$`, Stdout: "Starting: Intent\n"},
		{Match: provider + `getSetupConsent$`, Stdout: "Result: Bundle[{consent=" + consent + "}]\n"},
		{Match: `^shell pm grant tech\.hybriq\.adbuddy android\.permission\.WRITE_SECURE_SETTINGS$`},
		{Match: `^shell dumpsys deviceidle whitelist \+tech\.hybriq\.adbuddy$`, Stdout: "Added: tech.hybriq.adbuddy\n"},
		{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "com.x/.Svc\n"},
		{Match: `^shell settings put secure `},
	}, phoneRules()...)
}

func setupJSON(t *testing.T, out string) setupResult {
	t.Helper()
	var env struct {
		OK   bool
		Data setupResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK {
		t.Fatalf("%v %q", err, out)
	}
	return env.Data
}

func TestSetupSetsUpTheApp(t *testing.T) {
	e := newSetupEnv(t, appRules("granted"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	r := setupJSON(t, out).App
	if !r.Installed || r.Version != "0.2.0" || r.Consent != "granted" || !r.Accessibility {
		t.Fatalf("%+v\n%s", r, errOut)
	}
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{
		"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS",
		"shell dumpsys deviceidle whitelist +tech.hybriq.adbuddy",
		"shell settings put secure enabled_accessibility_services com.x/.Svc:tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService",
		"shell settings put secure accessibility_enabled 1",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	cfg, err := config.Load(filepath.Join(e.cfgDir, "config.toml"))
	if err != nil || cfg.ChatPort < 20000 || cfg.ChatPort >= 60000 {
		t.Fatalf("chat port %d %v", cfg.ChatPort, err)
	}
}

func TestSetupWithoutConsentChangesNothingOnThePhone(t *testing.T) {
	e := newSetupEnv(t, appRules("denied"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if r := setupJSON(t, out).App; r.Consent != "denied" || r.Accessibility {
		t.Fatalf("%+v", r)
	}
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		if j := c.Joined(); strings.Contains(j, "pm grant") || strings.Contains(j, "settings put") || strings.Contains(j, "deviceidle") {
			t.Fatalf("changed the phone without consent: %q", j)
		}
	}
}

func TestSetupWithoutReleaseSkipsTheApp(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if r := setupJSON(t, out).App; r.Installed || r.Consent != "skipped" {
		t.Fatalf("%+v", r)
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test -count=1 ./internal/cli/
```

Esperado: no compila: `unknown field App in struct literal`, `undefined: appResult`.

- [ ] **Paso 3: Implementar**

`host/internal/cli/appsetup.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// consentWait is how long setup waits for the answer in the app.
var consentWait = 5 * time.Minute

// appResult is the app part of `adbuddy setup --json`.
type appResult struct {
	Version       string `json:"version,omitempty"`
	Installed     bool   `json:"installed"`
	Consent       string `json:"consent"` // granted, denied, timeout or skipped
	Accessibility bool   `json:"accessibility"`
	Note          string `json:"note,omitempty"`
}

func releaseURL(app *App) string {
	if u := app.Getenv("ADBUDDY_RELEASE_URL"); u != "" {
		return u
	}
	return phoneapp.ReleaseURL
}

// setupApp runs steps 5-6 of spec §3.1: install the app (from apk, or the
// latest release) unless it is there, ask for consent in the app, then grant
// WRITE_SECURE_SETTINGS and enable its accessibility service. The app is
// optional for the host: every problem here is reported and setup goes on
// (🔴 confirmations then happen in the terminal).
func setupApp(ctx context.Context, app *App, e *env, a phoneapp.ADB, apk string, say func(string, ...any)) appResult {
	res := appResult{Consent: "skipped"}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if err != nil {
		res.Note = err.Error()
		say("⚠ Could not check the ADBuddy app: %v\n", err)
		return res
	}
	if version == "" || apk != "" {
		if apk == "" {
			say("Getting the ADBuddy app…\n")
			apk, err = phoneapp.Download(ctx, nil, releaseURL(app), filepath.Join(e.dirs.Data, "app"))
			if err != nil {
				res.Note = err.Error()
				if errors.Is(err, phoneapp.ErrNoRelease) {
					say("There is no published version of the ADBuddy app yet: skipping it (pass --apk <file> to install one).\n")
				} else {
					say("⚠ Could not get the ADBuddy app (%v): skipping it; 🔴 confirmations will happen in the terminal.\n", err)
				}
				return res
			}
		} else if _, err := os.Stat(apk); err != nil {
			res.Note = err.Error()
			say("⚠ %v: skipping the app.\n", err)
			return res
		}
		say("Installing the ADBuddy app on the phone…\n")
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.Note = err.Error()
			say("⚠ Could not install the app: %v\n", err)
			return res
		}
		if version, err = phoneapp.InstalledVersion(ctx, a); err != nil || version == "" {
			res.Note = "the app does not appear as installed"
			say("⚠ The app does not appear as installed.\n")
			return res
		}
	}
	res.Installed, res.Version = true, version
	if _, err := chatPort(e); err != nil {
		say("⚠ %v\n", err)
	}

	host, _ := os.Hostname()
	say("Open the phone: the ADBuddy app explains what this PC will do. Accept there to continue.\n")
	switch err := phoneapp.AskConsent(ctx, a, phoneapp.HostName(host), consentWait, time.Second); {
	case errors.Is(err, phoneapp.ErrConsentDenied):
		res.Consent = "denied"
		say("You said no in the app: ADBuddy works without it, and 🔴 confirmations happen in the terminal. Run `adbuddy setup` again to change it.\n")
		return res
	case errors.Is(err, phoneapp.ErrConsentTimeout):
		res.Consent = "timeout"
		say("No answer in the app: skipping it for now. Run `adbuddy setup` again when you have the phone at hand.\n")
		return res
	case err != nil:
		res.Consent, res.Note = "skipped", err.Error()
		say("⚠ Could not ask in the app: %v\n", err)
		return res
	}
	res.Consent = "granted"
	if err := phoneapp.Grant(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not give the app the permission to cut access: %v\n", err)
	}
	if err := phoneapp.AllowBackground(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not exempt the app from battery optimisation (it may disconnect with the screen off): %v\n", err)
	}
	if prev, err := phoneapp.EnableAccessibility(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not turn on the app's accessibility service: %v\n", err)
	} else {
		res.Accessibility = true
		if prev != "" && prev != "null" {
			say("Accessibility services already on were kept: %s\n", prev)
		}
	}
	say("The ADBuddy app is ready (version %s).\n", version)
	return res
}

// fmtApp is the human summary line of the app.
func fmtApp(r appResult) string {
	if !r.Installed {
		return "app: not installed"
	}
	return fmt.Sprintf("app %s (%s)", r.Version, r.Consent)
}
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
	ADB       string         `json:"adb"`
}
```

Después:

```go
	ADB       string         `json:"adb"`
	App       appResult      `json:"app"`
}
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
	var adbFlag, agentFlag, wsFlag, connectFlag string
	var useCode, yes bool
```

Después:

```go
	var adbFlag, agentFlag, wsFlag, connectFlag, apkFlag string
	var useCode, yes, noApp bool
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
				say("Phone: %s, Android %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android))
```

Después:

```go
				say("Phone: %s, Android %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android))

				// The phone app (spec §3.1 steps 5-6).
				appRes := appResult{Consent: "skipped", Note: "--no-app"}
				if !noApp {
					appRes = setupApp(ctx, app, &e, runner, apkFlag, say)
				}
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
				res := setupResult{Device: prof, Serial: saved, Active: serial, Address: cfg.Address, Workspace: ws, Agent: chosen, ADB: adbPath}
```

Después:

```go
				res := setupResult{Device: prof, Serial: saved, Active: serial, Address: cfg.Address, Workspace: ws, Agent: chosen, ADB: adbPath, App: appRes}
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
				fmt.Fprintf(app.Stdout, "Ready: %s (Android %s), environment %s, agent %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android), ws, chosen)
```

Después:

```go
				fmt.Fprintf(app.Stdout, "Ready: %s (Android %s), environment %s, agent %s, %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android), ws, chosen, fmtApp(appRes))
```

`host/internal/cli/setup.go`. Cambio:

Antes:

```go
	cmd.Flags().BoolVar(&yes, "yes", false, "accept the recommended choices without asking")
```

Después:

```go
	cmd.Flags().BoolVar(&yes, "yes", false, "accept the recommended choices without asking")
	cmd.Flags().StringVar(&apkFlag, "apk", "", "install this ADBuddy app APK instead of the latest release")
	cmd.Flags().BoolVar(&noApp, "no-app", false, "do not install or set up the phone app")
```

`host/internal/cli/setup_test.go`. Cambio:

Antes:

```go
	t.Setenv("ADBUDDY_SOURCE", "")
```

Después:

```go
	t.Setenv("ADBUDDY_SOURCE", "")
	t.Setenv("ADBUDDY_RELEASE_URL", "http://127.0.0.1:1/") // no app release reachable: setup skips the app
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test -count=1 ./internal/cli/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): setup instala la app y, con consentimiento, concede permisos y activa la accesibilidad

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 7: `adbuddy doctor`: diagnóstico con arreglos guiados

Idea tomada de ARTEMIS (`mobile_diagnose`). `adbuddy doctor` repasa en orden de dependencias todo lo que ADBuddy necesita:
1. configuración;
2. el adb propio (versión 37 o posterior);
3. el servidor adb privado;
4. `serve`;
5. el móvil (conectado, que sea el de la configuración, sin puerto de red abierto);
6. la app (instalada, con permiso de Cortar acceso, accesibilidad, excepción de batería y canal abierto);
7. el entorno del agente (AGENTS.md, shim y diario escribible).

Cada problema trae una línea `Run:` (un comando seguro que puede ejecutar la persona o el agente) o una línea `Guidance:` (algo que solo puede hacer la persona). Al final da un veredicto: ready, degraded o broken, con salida 1 si es broken.

Solo lee: no conecta, no concede permisos y no cambia nada en el móvil. `--fix` solo reinicia el servidor adb privado cuando es eso lo que falla y `serve` no está en marcha, y se niega en la sesión del agente. `AGENTS.md` enseña al agente a usarlo, e incluye dos reglas nuevas: acotar siempre los comandos que no terminan solos (`logcat -d`, `top -n 1`…) y no pedir nunca credenciales en el chat.

**Ficheros:**
- Crear: `host/internal/cli/doctor.go`
- Modificar: `host/internal/cli/root.go`
- Modificar: `host/internal/workspace/templates/AGENTS.md`
- Test: `host/internal/cli/doctor_test.go`

**Interfaces:**
- Consume: `configuredADB`, `serveWorkspace`, `unverifiedWhy` (M1c); `pairing.GuidOf`/`IsGuidSerial`/`GuidSerialNo`/`VerifyIdentity`; `device.TCPPort`; `phoneapp` (4); `journal.Store.Writable`.
- Produce: `adbuddy doctor [--json] [--fix]`, con JSON `{verdict, fixed?, checks: [{name, status: ok|warn|fail|skip, detail, run?, guidance?}]}`; las reglas 13 y 14 y la línea de `adbuddy doctor` en `templates/AGENTS.md`.

- [ ] **Paso 1: Escribir los tests que fallan**

`host/internal/cli/doctor_test.go`:

```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

const doctorSerial = "adb-AAAA1111-bbb._adb-tls-connect._tcp"

// doctorRules answer like a healthy phone with the app set up; extra rules go first.
func doctorRules(extra ...testutil.FakeRule) []testutil.FakeRule {
	return append(extra,
		testutil.FakeRule{Match: `^version$`, Stdout: "Android Debug Bridge version 1.0.41\nVersion 37.0.1-15733141\n"},
		testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n" + doctorSerial + "\tdevice\n"},
		testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"},
		testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "\n"},
		testutil.FakeRule{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n      android.permission.WRITE_SECURE_SETTINGS: granted=true\n"},
		testutil.FakeRule{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService\n"},
		testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,tech.hybriq.adbuddy,10123\n"},
		testutil.FakeRule{Match: `^(kill-server|start-server)$`},
	)
}

// doctorEnv writes a config for a set-up phone and creates its environment.
func doctorEnv(t *testing.T, rules []testutil.FakeRule) setupEnv {
	t.Helper()
	e := newSetupEnv(t, rules)
	cfg := config.Default()
	cfg.Serial, cfg.ADBPath, cfg.Workspace = doctorSerial, fakeADB(t), e.ws
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if _, err := workspace.Init(workspace.Options{Dir: e.ws, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	return e
}

func doctorJSON(t *testing.T, out string) doctorReport {
	t.Helper()
	var env struct {
		OK   bool
		Data doctorReport
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK {
		t.Fatalf("%v %q", err, out)
	}
	return env.Data
}

func statusOf(rep doctorReport, name string) check {
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	return check{Status: "missing"}
}

func TestDoctorHealthyPhoneWithoutServeIsDegraded(t *testing.T) {
	doctorEnv(t, doctorRules())
	out, errOut, code := runWithInput(t, "", "doctor", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	rep := doctorJSON(t, out)
	for _, name := range []string{"config", "adb", "adb server", "phone", "identity", "network port", "phone app", "environment"} {
		if c := statusOf(rep, name); c.Status != checkOK {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn || c.Guidance == "" {
		t.Errorf("serve: %+v", c)
	}
	if rep.Verdict != "degraded" {
		t.Fatalf("verdict %s", rep.Verdict)
	}
}

func TestDoctorOnlyReads(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	runWithInput(t, "", "doctor")
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		j := c.Joined()
		for _, bad := range []string{"put", "grant", "connect", "usb", "install", "reverse tcp", "kill-server", "whitelist +"} {
			if strings.Contains(j, bad) {
				t.Errorf("doctor changed something: %q", j)
			}
		}
	}
}

func TestDoctorBrokenCases(t *testing.T) {
	cases := map[string]struct {
		rule     testutil.FakeRule
		name     string
		wantHelp string
	}{
		"no phone":    {testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}, "phone", "Wireless debugging"},
		"old adb":     {testutil.FakeRule{Match: `^version$`, Stdout: "Version 35.0.2-1\n"}, "adb", "adbuddy setup"},
		"wrong phone": {testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n"}, "identity", "adbuddy connect"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, doctorRules(tc.rule))
			out, _, code := runWithInput(t, "", "doctor", "--json")
			if code != 1 {
				t.Fatalf("exit %d, want 1\n%s", code, out)
			}
			rep := doctorJSON(t, out)
			c := statusOf(rep, tc.name)
			if rep.Verdict != "broken" || c.Status != checkFail || !strings.Contains(c.Run+c.Guidance, tc.wantHelp) {
				t.Fatalf("%s %+v", rep.Verdict, c)
			}
		})
	}
}

func TestDoctorAppNotSetUpIsOnlyAWarning(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"},
	))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "phone app")
	if code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "battery exemption") || c.Run != "adbuddy setup" {
		t.Fatalf("exit %d %+v", code, c)
	}
}

func TestDoctorWithoutSetup(t *testing.T) {
	newSetupEnv(t, doctorRules())
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	if code != 1 || statusOf(rep, "config").Run != "adbuddy setup" {
		t.Fatalf("exit %d %+v", code, rep)
	}
}

func TestDoctorHumanOutput(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}))
	out, _, _ := runWithInput(t, "", "doctor")
	for _, want := range []string{"✓ adb: 37.0.1", "✗ phone:", "    Guidance: Turn on Wireless debugging", "Verdict: broken"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestDoctorFixRestartsTheAdbServer(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n", Times: 1}))
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	if code != 0 || len(rep.Fixed) != 1 || statusOf(rep, "adb server").Status != checkOK {
		t.Fatalf("exit %d %+v", code, rep)
	}
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "kill-server") || !strings.Contains(all, "start-server") {
		t.Fatalf("server not restarted:\n%s", all)
	}
}

func TestDoctorFixIsForThePersonOnly(t *testing.T) {
	doctorEnv(t, doctorRules())
	t.Setenv("ADBUDDY_SOURCE", "agent")
	_, errOut, code := runWithInput(t, "", "doctor", "--fix")
	if code != 1 || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("exit %d %q", code, errOut)
	}
	if _, _, code := runWithInput(t, "", "doctor", "--json"); code != 0 {
		t.Fatalf("the agent cannot run doctor: exit %d", code)
	}
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd host && go test -count=1 ./internal/cli/ ./internal/workspace/
```

Esperado: no compila: `undefined: doctorReport`, `undefined: check`…

- [ ] **Paso 3: Implementar**

`host/internal/cli/doctor.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// Check statuses. A failed check stops ADBuddy from working; a warning
// leaves it working with something missing (degraded).
const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
	checkSkip = "skip"
)

// check is one line of `adbuddy doctor`. Run is a single safe local command
// the person or the agent may run; Guidance is something only the person can
// do (on the phone, in another terminal); they are never both empty on a
// failure or a warning.
type check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Run      string `json:"run,omitempty"`
	Guidance string `json:"guidance,omitempty"`
}

// doctorReport is the output of `adbuddy doctor`.
type doctorReport struct {
	Verdict string   `json:"verdict"` // ready, degraded or broken
	Fixed   []string `json:"fixed,omitempty"`
	Checks  []check  `json:"checks"`
}

// Doctor time limits: every adb call is bounded, so doctor always answers.
var (
	doctorCallWait = 10 * time.Second
	adbVersionRe   = regexp.MustCompile(`Version (\d+)\.(\d+)\.(\d+)`)
)

func newDoctorCmd(app *App) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that ADBuddy, adb, the phone and the app are ready, and say how to fix what is not",
		Long: `Check, in order, everything ADBuddy needs: configuration, ADBuddy's adb and its
private server, the phone (connected, the one you set up, no open network port),
the agent environment, ` + "`adbuddy serve`" + ` and the phone app. Every problem comes with
a "Run:" line (a safe command) or a "Guidance:" line (something only you can do).
It only reads: nothing on the phone changes. With --fix it also restarts
ADBuddy's private adb server when that is what fails and serve is not running.
Exit code: 0 ready or degraded, 1 broken.`,
		Example: "  adbuddy doctor\n  adbuddy doctor --json\n  adbuddy doctor --fix",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fix && app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewError("AGENT_SESSION", "`adbuddy doctor --fix` cannot be run from the agent's session", "Run `adbuddy doctor` (without --fix) and follow its Run: lines, or ask the person to run --fix.")
			}
			ctx := cmd.Context()
			rep := runDoctor(ctx, app)
			if fix {
				if fixed := doctorFix(ctx, app, rep); len(fixed) > 0 {
					rep = runDoctor(ctx, app)
					rep.Fixed = fixed
				}
			}
			if app.JSON {
				if err := WriteData(app.Stdout, rep); err != nil {
					return err
				}
			} else {
				printReport(app, rep)
			}
			if rep.Verdict == "broken" {
				return ExitWith(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "also restart ADBuddy's private adb server when it is what fails (never while serve runs)")
	return cmd
}

func printReport(app *App, rep doctorReport) {
	icon := map[string]string{checkOK: "✓", checkWarn: "⚠", checkFail: "✗", checkSkip: "·"}
	for _, f := range rep.Fixed {
		fmt.Fprintf(app.Stdout, "↻ %s\n", f)
	}
	for _, c := range rep.Checks {
		fmt.Fprintf(app.Stdout, "%s %s: %s\n", icon[c.Status], c.Name, classify.Printable(c.Detail))
		if c.Run != "" {
			fmt.Fprintf(app.Stdout, "    Run: %s\n", c.Run)
		}
		if c.Guidance != "" {
			fmt.Fprintf(app.Stdout, "    Guidance: %s\n", c.Guidance)
		}
	}
	switch rep.Verdict {
	case "ready":
		fmt.Fprintln(app.Stdout, "Verdict: ready.")
	case "degraded":
		fmt.Fprintln(app.Stdout, "Verdict: degraded — ADBuddy works; the ⚠ lines are optional or can wait.")
	default:
		fmt.Fprintln(app.Stdout, "Verdict: broken — fix the ✗ lines first, from the top.")
	}
}

// runDoctor runs every check. It never changes anything, on the PC or on the
// phone (starting ADBuddy's private adb server aside, as any adb call does).
func runDoctor(ctx context.Context, app *App) (rep doctorReport) {
	add := func(c check) { rep.Checks = append(rep.Checks, c) }
	defer func() { rep.Verdict = verdict(rep.Checks) }()

	e, err := loadEnv(app)
	if err != nil {
		var ce *Error
		fix := "Run `adbuddy setup`."
		if errors.As(err, &ce) && ce.Fix != "" {
			fix = ce.Fix
		}
		add(check{Name: "config", Status: checkFail, Detail: err.Error(), Guidance: fix})
		return rep
	}
	if e.cfg.Serial == "" && e.cfg.Address == "" {
		add(check{Name: "config", Status: checkFail, Detail: "no phone set up yet (" + e.dirs.ConfigFile() + ")", Run: "adbuddy setup"})
	} else {
		add(check{Name: "config", Status: checkOK, Detail: e.dirs.ConfigFile()})
	}

	adbPath := configuredADB(e, "")
	if adbPath == "" {
		add(check{Name: "adb", Status: checkFail, Detail: "ADBuddy's adb is not installed", Run: "adbuddy setup"})
		return rep
	}
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	add(checkADB(ctx, runner, adbPath))

	online, err := callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) })
	if err != nil {
		add(check{Name: "adb server", Status: checkFail,
			Detail:   fmt.Sprintf("ADBuddy's adb server (port %d) does not answer: %v", e.cfg.ADBServerPort, err),
			Run:      "adbuddy doctor --fix",
			Guidance: "If it keeps failing, stop other adb tools using port " + strconv.Itoa(e.cfg.ADBServerPort) + "."})
		return rep
	}
	add(check{Name: "adb server", Status: checkOK, Detail: fmt.Sprintf("port %d, %d phone(s) connected", e.cfg.ADBServerPort, len(online))})

	running, ping, perr := servePing(ctx, e.dirs.Data)
	switch {
	case perr != nil:
		add(check{Name: "serve", Status: checkFail, Detail: perr.Error(), Guidance: "Stop the other `adbuddy serve`, or delete " + ipc.SocketPath(e.dirs.Data) + " if none is running."})
	case running:
		add(check{Name: "serve", Status: checkOK, Detail: "running, session " + ping.Session + ", phone " + ping.Serial})
	default:
		add(check{Name: "serve", Status: checkWarn, Detail: "not running: the agent cannot use the phone", Guidance: "Start `adbuddy serve` in a terminal and keep it open."})
	}

	active, guid := findConfiguredPhone(online, e.cfg.Serial, e.cfg.Address)
	if active == "" {
		how := "Turn on Wireless debugging on the phone and keep it on the same Wi-Fi as this PC."
		if e.cfg.Address != "" {
			how = "Turn on Wireless debugging on the phone. On another network (Tailscale), run `adbuddy connect <ip:port>` with the address shown on that screen."
		}
		add(check{Name: "phone", Status: checkFail, Detail: "the phone you set up is not connected to ADBuddy's adb server", Guidance: how})
	} else {
		add(check{Name: "phone", Status: checkOK, Detail: active})
		phone := runner
		phone.Serial = active
		add(checkIdentity(ctx, phone, guid))
		add(checkRemotePort(ctx, phone))
		add(checkApp(ctx, phone, e, running))
	}

	ws := e.workspace("")
	if running {
		ws = serveWorkspace(ping)
	}
	add(checkWorkspace(ws))
	return rep
}

func verdict(cs []check) string {
	v := "ready"
	for _, c := range cs {
		switch c.Status {
		case checkFail:
			return "broken"
		case checkWarn:
			v = "degraded"
		}
	}
	return v
}

// callWithin bounds one adb call.
func callWithin[T any](ctx context.Context, f func(context.Context) (T, error)) (T, error) {
	c, cancel := context.WithTimeout(ctx, doctorCallWait)
	defer cancel()
	return f(c)
}

func checkADB(ctx context.Context, runner adb.Runner, path string) check {
	out, err := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, code, err := runner.Output(c, "version")
		if err == nil && code != 0 {
			err = fmt.Errorf("exit %d", code)
		}
		return o, err
	})
	if err != nil {
		return check{Name: "adb", Status: checkFail, Detail: path + ": " + err.Error(), Run: "adbuddy setup"}
	}
	m := adbVersionRe.FindStringSubmatch(out)
	if m == nil {
		return check{Name: "adb", Status: checkWarn, Detail: path + ": unknown version", Run: "adbuddy setup"}
	}
	major, _ := strconv.Atoi(m[1])
	version := m[1] + "." + m[2] + "." + m[3]
	if major < 37 {
		return check{Name: "adb", Status: checkFail, Detail: path + ": version " + version + ", ADBuddy needs 37 or newer (adb Wi-Fi 2.0)", Run: "adbuddy setup"}
	}
	return check{Name: "adb", Status: checkOK, Detail: version + " (" + path + ")"}
}

func servePing(ctx context.Context, dataDir string) (bool, ipc.Ping, error) {
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	p, err := ipc.PingDaemon(pctx, dataDir)
	switch {
	case err == nil:
		return true, p, nil
	case errors.Is(err, ipc.ErrNotRunning):
		return false, ipc.Ping{}, nil
	}
	return false, ipc.Ping{}, fmt.Errorf("the ADBuddy socket does not answer: %v", err)
}

// findConfiguredPhone finds the phone of config.toml among the connected
// ones, by its stable name (with or without " (N)") or by its saved address.
// It never connects anything.
func findConfiguredPhone(online []adb.Device, serial, addr string) (active, guid string) {
	guid, _ = pairing.GuidOf(serial)
	for _, d := range online {
		if (guid != "" && pairing.IsGuidSerial(guid, d.Serial)) || (guid == "" && serial != "" && d.Serial == serial) {
			return d.Serial, guid
		}
	}
	for _, d := range online {
		if addr != "" && d.Serial == addr {
			return d.Serial, guid
		}
	}
	return "", guid
}

func checkIdentity(ctx context.Context, phone adb.Runner, guid string) check {
	if _, ok := pairing.GuidSerialNo(guid); !ok {
		return check{Name: "identity", Status: checkWarn, Detail: "cannot be checked: " + unverifiedWhy(guid),
			Guidance: "Nothing to do unless you have several phones; `adbuddy undo` will not mix them up."}
	}
	_, err := callWithin(ctx, func(c context.Context) (struct{}, error) { return struct{}{}, pairing.VerifyIdentity(c, phone, guid) })
	switch {
	case errors.Is(err, pairing.ErrWrongPhone):
		return check{Name: "identity", Status: checkFail, Detail: "the phone at " + phone.Serial + " is not the one you set up",
			Guidance: "Check the IP and port on the phone you set up; run `adbuddy connect <ip:port>` with them."}
	case err != nil:
		return check{Name: "identity", Status: checkWarn, Detail: "could not read the phone's serial number: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	}
	return check{Name: "identity", Status: checkOK, Detail: "it is the phone you set up"}
}

func checkRemotePort(ctx context.Context, phone adb.Runner) check {
	port, err := callWithin(ctx, func(c context.Context) (string, error) { return device.TCPPort(c, phone) })
	switch {
	case err != nil:
		return check{Name: "network port", Status: checkWarn, Detail: "could not read service.adb.tcp.port: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	case port == "" || port == "0":
		return check{Name: "network port", Status: checkOK, Detail: "closed (no adb tcpip mode)"}
	}
	return check{Name: "network port", Status: checkWarn, Detail: "adb listens on port " + port + " (tcpip mode): `adbuddy serve` closes it with `adb usb` when it starts",
		Guidance: "Nothing to do: serve closes it. Remote mode (Tailscale over mobile data) arrives in a later version."}
}

// checkApp: the phone app is optional; everything here is a warning at most.
func checkApp(ctx context.Context, phone adb.Runner, e env, serveRunning bool) check {
	version, err := callWithin(ctx, func(c context.Context) (string, error) { return phoneapp.InstalledVersion(c, phone) })
	if err != nil {
		return check{Name: "phone app", Status: checkWarn, Detail: "could not check: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	}
	if version == "" {
		return check{Name: "phone app", Status: checkWarn, Detail: "not installed: 🔴 confirmations happen in the terminal running serve", Run: "adbuddy setup"}
	}
	var missing []string
	pkg, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "dumpsys package "+phoneapp.Package)
		return o, err
	})
	if !strings.Contains(pkg, phoneapp.SecureWrites+": granted=true") {
		missing = append(missing, "permission for Cut access")
	}
	a11y, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "settings get secure enabled_accessibility_services")
		return o, err
	})
	if !strings.Contains(strings.ToLower(a11y), strings.ToLower(phoneapp.A11yService)) {
		missing = append(missing, "accessibility service")
	}
	idle, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "dumpsys deviceidle whitelist")
		return o, err
	})
	if !strings.Contains(idle, phoneapp.Package) {
		missing = append(missing, "battery exemption")
	}
	if len(missing) > 0 {
		return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but without: " + strings.Join(missing, ", "),
			Run: "adbuddy setup", Guidance: "Accept the consent screen in the ADBuddy app when setup asks."}
	}
	if serveRunning && e.cfg.ChatPort != 0 {
		open, err := callWithin(ctx, func(c context.Context) (bool, error) { return phoneapp.TunnelOpen(c, phone, e.cfg.ChatPort) })
		if err == nil && !open {
			return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but the channel with serve is not open",
				Guidance: "Open the ADBuddy app on the phone; serve reopens the channel within 15 seconds."}
		}
	}
	return check{Name: "phone app", Status: checkOK, Detail: version + " installed and set up"}
}

func checkWorkspace(ws string) check {
	var missing []string
	for _, f := range []string{"AGENTS.md", filepath.Join(".adbuddy", "bin", exeName("adb"))} {
		if _, err := os.Stat(filepath.Join(ws, f)); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return check{Name: "environment", Status: checkFail, Detail: ws + ": missing " + strings.Join(missing, ", "), Run: "adbuddy env init --dir " + quoteArg(ws)}
	}
	if err := journal.Open(workspace.JournalDir(ws)).Writable(); err != nil {
		return check{Name: "environment", Status: checkFail, Detail: "the journal cannot be written (" + err.Error() + "): nothing 🟡 or 🔴 will run",
			Guidance: "Check the permissions and free space of " + workspace.JournalDir(ws) + "."}
	}
	return check{Name: "environment", Status: checkOK, Detail: ws}
}

// doctorFix applies the only automatic fix: restarting ADBuddy's private adb
// server when it does not answer and no serve is using it.
func doctorFix(ctx context.Context, app *App, rep doctorReport) []string {
	var failing bool
	for _, c := range rep.Checks {
		if c.Name == "adb server" && c.Status == checkFail {
			failing = true
		}
		if c.Name == "serve" && c.Status == checkOK {
			return nil // never under a running serve
		}
	}
	if !failing {
		return nil
	}
	e, err := loadEnv(app)
	if err != nil {
		return nil
	}
	runner := adb.Runner{Path: configuredADB(e, ""), ServerPort: e.cfg.ADBServerPort}
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "kill-server")
		return struct{}{}, err
	})
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "start-server")
		return struct{}{}, err
	})
	return []string{fmt.Sprintf("restarted ADBuddy's adb server (port %d)", e.cfg.ADBServerPort)}
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// quoteArg quotes a path for the Run: line when it has spaces.
func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
```

`host/internal/cli/root.go`. Cambio:

Antes:

```go
	root.AddCommand(newVersionCmd(app), newServeCmd(app), newRunCmd(app), newJournalCmd(app), newUndoCmd(app), newEnvCmd(app), newConnectCmd(app), setup)
```

Después:

```go
	root.AddCommand(newVersionCmd(app), newServeCmd(app), newRunCmd(app), newJournalCmd(app), newUndoCmd(app), newEnvCmd(app), newConnectCmd(app), newDoctorCmd(app), setup)
```

`host/internal/workspace/templates/AGENTS.md`. Cambio:

Antes:

```markdown
- `adbuddy undo [last|<id>|session]`: undo the last change, one journal entry, or the whole session.
```

Después:

```markdown
- `adbuddy undo [last|<id>|session]`: undo the last change, one journal entry, or the whole session.
- `adbuddy doctor [--json]`: when adb or adbuddy fails in a way you do not understand, run it before guessing. Run its `run` lines yourself (they are safe); relay its `guidance` lines to the user word for word, because only they can do them.
```

`host/internal/workspace/templates/AGENTS.md`. Cambio:

Antes:

```markdown
12. Do not try to bypass ADBuddy (another adb binary, another adb server, scripts that call adb). It protects the user's phone; working around it breaks their trust.
```

Después:

```markdown
12. Do not try to bypass ADBuddy (another adb binary, another adb server, scripts that call adb). It protects the user's phone; working around it breaks their trust.
13. Always bound commands that never end on their own, or they hang your shell: `logcat -d` (not `logcat`), `top -n 1`, `screenrecord --time-limit 30 …`, `monkey` with an event count, `getevent -c <n>`.
14. Never ask the user to type a password, a PIN or an API key in the chat: ADBuddy never needs them, and the phone asks for its own when it must.
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd host && go mod tidy && go vet ./... && gofmt -l . && go test -count=1 ./internal/cli/ ./internal/workspace/
```

Esperado: `ok` en todos los paquetes, `gofmt -l .` sin salida y `go.mod` solo con las dependencias de las restricciones globales.

- [ ] **Paso 5: Commit**

```bash
git add host
git commit -F - <<'EOF'
feat(host): adbuddy doctor con arreglos guiados y reglas nuevas en AGENTS.md

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 8: Proyecto Android y protocolo de la app

Esqueleto de `android/`: Gradle con catálogo de versiones, AGP 9 con Kotlin integrado (sin `org.jetbrains.kotlin.android`), minSdk 30, targetSdk/compileSdk 37 y sin Gradle wrapper. Incluye el manifiesto completo de M2, los textos en inglés y español, y una configuración de red que solo permite texto plano a `127.0.0.1`. Añade el protocolo (espejo de `host/internal/bridge`), `BridgeState` (el estado que se reconstruye a partir de los mensajes: una pregunta desaparece cuando llega la entrada final del diario con su id) y la validación del handshake. Todo esto es lógica pura con tests JVM. El `hello` lleva la versión de protocolo de la app y el `welcome` la del host: con un host demasiado antiguo el estado pasa a `INCOMPATIBLE` (`Outdated.HOST`).

**Ficheros:**
- Crear: `android/settings.gradle.kts`
- Crear: `android/build.gradle.kts`
- Crear: `android/gradle.properties`
- Crear: `android/gradle/libs.versions.toml`
- Crear: `android/app/build.gradle.kts`
- Crear: `android/app/proguard-rules.pro`
- Crear: `android/app/src/main/AndroidManifest.xml`
- Crear: `android/app/src/main/res/values/strings.xml`
- Crear: `android/app/src/main/res/values-es/strings.xml`
- Crear: `android/app/src/main/res/xml/network_security_config.xml`
- Crear: `android/app/src/main/res/xml/accessibility_service.xml`
- Crear: `android/app/src/main/res/xml/data_extraction_rules.xml`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Protocol.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Handshake.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ProtocolTest.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/HandshakeTest.kt`

**Interfaces:**
- Produce (paquete `tech.hybriq.adbuddy.bridge`): `Protocol` (`VERSION`, tipos, `json`, `encode`, `encodeElement`, `decode`, `payload<T>`); `Message`, `Hello`, `Welcome`, `SessionState`, `ConfirmRequest`, `ConfirmReply`, `Notice`, `JournalEntry`, `Inverse`, `Snapshot`, `Decision`; `BridgeState` (`apply(Message)`, `answered(id)`, `Connection`); `Handshake`, `HandshakeParser.parse/readLimited`; `Caller.isShell(uid)`.

> El manifiesto ya declara las clases de la tarea 10: hasta esa tarea, `assembleDebug` fallará por clases que faltan. En esta tarea solo se ejecutan los tests unitarios.

> Gradle 9.8.0 y JDK 17 deben estar instalados. Usa `gradle --no-daemon`; con `GRADLE_USER_HOME` apuntando dentro de `android/.gradle/`, si no quieres caché global.

- [ ] **Paso 1: Escribir los tests que fallan**

`android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ProtocolTest.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ProtocolTest {
    // A message exactly as host/internal/bridge writes it.
    private val hostConfirm = """{"v":1,"seq":7,"type":"confirm.request","ts":"2026-09-30T01:02:03Z","payload":{"id":"j_1","level":"red","title":"Restart the phone","detail":"may restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z","special":true,"allowSession":false,"future":"ignored"}}"""

    @Test fun decodesHostMessages() {
        val m = Protocol.decode(hostConfirm)!!
        assertEquals(7L, m.seq)
        val r = Protocol.payload<ConfirmRequest>(m)!!
        assertEquals("adb shell reboot", r.rawCommand)
        assertTrue(r.special)
        assertFalse(r.allowSession)
    }

    @Test fun rejectsOtherVersionsAndGarbage() {
        assertNull(Protocol.decode("""{"v":2,"type":"notice","payload":{}}"""))
        assertNull(Protocol.decode("""{"v":1,"type":"","payload":{}}"""))
        assertNull(Protocol.decode("not json"))
    }

    @Test fun encodesHelloAndReplies() {
        val hello = Protocol.encode(Protocol.HELLO, Hello(token = "t", lastSeq = 3, session = "s1", appVersion = "0.2.0"), "2026-09-30T00:00:00Z")
        assertTrue(hello, hello.contains(""""type":"hello"""") && hello.contains(""""lastSeq":3""") && hello.contains(""""v":1"""))
        assertTrue(hello, hello.contains(""""session":"s1""""))
        val reply = Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply("j_1", Decision.ALLOW_SESSION), "x")
        assertTrue(reply, reply.contains(""""decision":"allow_session""""))
    }
}
```

`android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class BridgeStateTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private val question = """{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}"""

    @Test fun questionsLeaveWhenTheirEntryIsFinal() {
        var s = BridgeState()
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1,"protocol":1}"""))
        assertEquals(BridgeState.Connection.CONNECTED, s.connection)
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        assertEquals(1, s.pending.size)
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"running","level":"red"}"""))
        assertEquals("a running record keeps the question", 1, s.pending.size)
        s = s.apply(msg(3, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"denied","level":"red"}"""))
        assertTrue(s.pending.isEmpty())
        assertEquals(1, s.journal.size)
        assertEquals("denied", s.journal[0].status)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun snapshotReplacesEverything() {
        var s = BridgeState(session = "s_1", lastSeq = 9)
        s = s.apply(msg(10, Protocol.SNAPSHOT, """{"state":{"paused":true},"pending":[$question],"journal":[{"id":"j_0","status":"ok"}]}"""))
        assertTrue(s.sessionState.paused)
        assertEquals(listOf("j_1"), s.pending.map { it.id })
        assertEquals(listOf("j_0"), s.journal.map { it.id })
        assertEquals(10L, s.lastSeq)
    }

    @Test fun anotherSessionStartsOver() {
        var s = BridgeState(session = "s_1", lastSeq = 50).apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertEquals(0L, s.lastSeq)
        assertTrue(s.pending.isEmpty())
    }

    @Test fun badPayloadsAreIgnored() {
        val s = BridgeState().apply(msg(4, Protocol.CONFIRM_REQUEST, """{"id":5}"""))
        assertTrue(s.pending.isEmpty())
        assertEquals(4L, s.lastSeq)
    }

    @Test fun anOldHostIsIncompatible() {
        val s = BridgeState().apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1}"""))
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertEquals(BridgeState.Outdated.HOST, s.outdated)
    }

    @Test fun anOldAppIsIncompatible() {
        val s = BridgeState(pending = listOf(Protocol.json.decodeFromString(ConfirmRequest.serializer(), question))).appOutdated()
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertEquals(BridgeState.Outdated.APP, s.outdated)
        assertTrue(s.pending.isEmpty())
    }
}
```

`android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/HandshakeTest.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import java.io.ByteArrayInputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HandshakeTest {
    private val token = "0123456789abcdef".repeat(4)
    private val nonce = "fedcba9876543210".repeat(2)

    @Test fun acceptsWhatTheHostWrites() {
        val h = HandshakeParser.parse("""{"host":"DESKTOP-1","nonce":"$nonce","port":47123,"token":"$token"}""".toByteArray())!!
        assertEquals(47123, h.port)
        assertEquals(nonce, h.nonce)
        assertEquals("DESKTOP-1", h.host)
    }

    @Test fun refusesAnythingOff() {
        for (bad in listOf(
            """{"host":"x","nonce":"$nonce","port":47123,"token":"short"}""",
            """{"host":"x","nonce":"$nonce","port":80,"token":"$token"}""",
            """{"host":"x","nonce":"$nonce","port":47123}""",
            """{"host":"x","port":47123,"token":"$token"}""",
            """{"host":"x","nonce":"XYZ","port":47123,"token":"$token"}""",
            "not json",
            "{" + " ".repeat(5000) + "}",
        )) assertNull(bad, HandshakeParser.parse(bad.toByteArray()))
    }

    @Test fun oddHostNamesBecomePC() {
        val h = HandshakeParser.parse("""{"host":"<b>evil</b>","nonce":"$nonce","port":47123,"token":"$token"}""".toByteArray())!!
        assertEquals("PC", h.host)
    }

    @Test fun readLimitedStopsEarly() {
        val big = ByteArray(1 shl 20)
        assertTrue(HandshakeParser.readLimited(ByteArrayInputStream(big)).size <= 4096 + 1024)
    }

    @Test fun onlyTheShellUser() {
        assertTrue(Caller.isShell(2000))
        assertFalse(Caller.isShell(0))
        assertFalse(Caller.isShell(10123))
    }
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.bridge.ProtocolTest' --tests 'tech.hybriq.adbuddy.bridge.BridgeStateTest' --tests 'tech.hybriq.adbuddy.bridge.HandshakeTest'
```

Esperado: `gradle :app:testDebugUnitTest` no compila: `Unresolved reference: Protocol` (los tests existen y el código no).

- [ ] **Paso 3: Implementar**

`android/settings.gradle.kts`:

```kotlin
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "adbuddy"
include(":app")
```

`android/build.gradle.kts`:

```kotlin
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
}
```

`android/gradle.properties`:

```properties
org.gradle.jvmargs=-Xmx2g -Dfile.encoding=UTF-8
android.useAndroidX=true
android.nonTransitiveRClass=true
```

`android/gradle/libs.versions.toml`:

```toml
[versions]
agp = "9.4.1"
kotlin = "2.4.20"
composeBom = "2026.09.00"
activityCompose = "1.13.0"
coreKtx = "1.19.1"
lifecycle = "2.11.0"
okhttp = "5.5.0"
serialization = "1.11.0"
junit = "4.13.2"
androidxTestRunner = "1.7.0"
androidxTestExt = "1.3.0"

[libraries]
androidx-core-ktx = { module = "androidx.core:core-ktx", version.ref = "coreKtx" }
androidx-activity-compose = { module = "androidx.activity:activity-compose", version.ref = "activityCompose" }
androidx-lifecycle-runtime-compose = { module = "androidx.lifecycle:lifecycle-runtime-compose", version.ref = "lifecycle" }
androidx-lifecycle-service = { module = "androidx.lifecycle:lifecycle-service", version.ref = "lifecycle" }
compose-bom = { module = "androidx.compose:compose-bom", version.ref = "composeBom" }
compose-material3 = { module = "androidx.compose.material3:material3" }
compose-ui = { module = "androidx.compose.ui:ui" }
compose-ui-tooling-preview = { module = "androidx.compose.ui:ui-tooling-preview" }
okhttp = { module = "com.squareup.okhttp3:okhttp", version.ref = "okhttp" }
okhttp-mockwebserver = { module = "com.squareup.okhttp3:mockwebserver3", version.ref = "okhttp" }
kotlinx-serialization-json = { module = "org.jetbrains.kotlinx:kotlinx-serialization-json", version.ref = "serialization" }
junit = { module = "junit:junit", version.ref = "junit" }
androidx-test-runner = { module = "androidx.test:runner", version.ref = "androidxTestRunner" }
androidx-test-ext-junit = { module = "androidx.test.ext:junit", version.ref = "androidxTestExt" }

[plugins]
android-application = { id = "com.android.application", version.ref = "agp" }
kotlin-compose = { id = "org.jetbrains.kotlin.plugin.compose", version.ref = "kotlin" }
kotlin-serialization = { id = "org.jetbrains.kotlin.plugin.serialization", version.ref = "kotlin" }
```

`android/app/build.gradle.kts`:

```kotlin
// AGP 9 compiles Kotlin by itself (built-in Kotlin): no org.jetbrains.kotlin.android plugin.
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "tech.hybriq.adbuddy"
    compileSdk = 37

    defaultConfig {
        applicationId = "tech.hybriq.adbuddy"
        minSdk = 30
        targetSdk = 37
        versionCode = 1
        versionName = "0.2.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.service)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
}
```

`android/app/proguard-rules.pro`:

```
# kotlinx.serialization keeps what it needs through its own consumer rules.
# The accessibility service, the provider and the receivers are referenced
# from the manifest, which R8 already keeps.
```

`android/app/src/main/AndroidManifest.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:tools="http://schemas.android.com/tools">

    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_SPECIAL_USE" />
    <!-- Granted by the host over adb (pm grant) for "Cut access" (spec §4.6). -->
    <uses-permission
        android:name="android.permission.WRITE_SECURE_SETTINGS"
        tools:ignore="ProtectedPermissions" />

    <application
        android:name=".AdbuddyApp"
        android:allowBackup="false"
        android:dataExtractionRules="@xml/data_extraction_rules"
        android:label="@string/app_name"
        android:icon="@android:drawable/sym_def_app_icon"
        android:supportsRtl="true"
        android:theme="@android:style/Theme.Material.NoActionBar"
        android:usesCleartextTraffic="false"
        android:networkSecurityConfig="@xml/network_security_config">

        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:launchMode="singleTop">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
            <!-- The host opens the consent screen with am start (shell). -->
            <intent-filter>
                <action android:name="tech.hybriq.adbuddy.action.SETUP" />
                <category android:name="android.intent.category.DEFAULT" />
            </intent-filter>
        </activity>

        <!-- Exported so `adb shell content …` reaches it; it answers only the
             shell user (uid 2000). -->
        <provider
            android:name=".bridge.TokenProvider"
            android:authorities="tech.hybriq.adbuddy.bridge"
            android:exported="true"
            tools:ignore="ExportedContentProvider" />

        <service
            android:name=".bridge.BridgeService"
            android:exported="false"
            android:foregroundServiceType="specialUse">
            <property
                android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE"
                android:value="Keeps the local channel with the ADBuddy host on the user's PC open, to show and confirm what the coding agent does on this phone." />
        </service>

        <receiver
            android:name=".confirm.ConfirmReceiver"
            android:exported="false" />

        <service
            android:name=".a11y.AgentAccessibilityService"
            android:exported="false"
            android:label="@string/a11y_label"
            android:permission="android.permission.BIND_ACCESSIBILITY_SERVICE">
            <intent-filter>
                <action android:name="android.accessibilityservice.AccessibilityService" />
            </intent-filter>
            <meta-data
                android:name="android.accessibilityservice"
                android:resource="@xml/accessibility_service" />
        </service>
    </application>
</manifest>
```

`android/app/src/main/res/values/strings.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name" translatable="false">ADBuddy</string>
    <string name="a11y_label">ADBuddy agent helper</string>
    <string name="a11y_description">Lets the coding agent on your PC read the screen and tap for you, always shown on screen and with a Pause button. It never reads passwords and never runs in protected apps.</string>

    <string name="channel_status">Connection with the PC</string>
    <string name="channel_confirm">Confirmations</string>
    <string name="status_connected">Connected to %1$s</string>
    <string name="status_connecting">Connecting to the PC…</string>
    <string name="status_waiting">Waiting for the PC (run adbuddy serve)</string>
    <string name="status_paused">Paused: the agent cannot use the phone</string>
    <string name="status_update_app">This app is too old for the ADBuddy on your PC: run adbuddy setup on the PC to update it</string>
    <string name="status_update_host">The ADBuddy on your PC is too old for this app: update it on the PC</string>

    <string name="action_allow">Allow</string>
    <string name="action_deny">Deny</string>
    <string name="action_allow_session">Allow during the session</string>
    <string name="action_pause">Pause</string>
    <string name="action_resume">Resume</string>
    <string name="action_cut">Cut access</string>
    <string name="action_restore">Restore access</string>

    <string name="confirm_title">The agent wants to: %1$s</string>
    <string name="confirm_special">This may cut the connection with the PC or affect ADBuddy.</string>
    <string name="confirm_expires">Expires at %1$s</string>

    <string name="consent_title">Set up ADBuddy with %1$s</string>
    <string name="consent_intro">The PC "%1$s" asks to finish setting up ADBuddy. If you accept, it will:</string>
    <string name="consent_grant">Give ADBuddy the permission to change secure settings, used only by "Cut access" to turn off debugging from the phone.</string>
    <string name="consent_battery">Exempt ADBuddy from battery optimisation, so the channel with the PC stays open.</string>
    <string name="consent_a11y">Turn on ADBuddy\'s accessibility service, keeping any other one you use.</string>
    <string name="consent_note">Nothing leaves your network: the PC and the phone talk through the adb connection you just paired.</string>
    <string name="consent_accept">Accept</string>
    <string name="consent_decline">Not now</string>
    <string name="consent_done">Done. You can go back to the PC.</string>

    <string name="home_title" translatable="false">ADBuddy</string>
    <string name="home_activity">Activity</string>
    <string name="home_empty">Nothing yet. Every adb command of the agent will appear here.</string>
    <string name="cut_done">Debugging is off: the PC cannot reach the phone.</string>
    <string name="cut_restored">Debugging is back on. If remote mode was on, its network port stays open until the PC connects or the phone restarts.</string>
    <string name="cut_failed">ADBuddy lacks the permission: run adbuddy setup on the PC.</string>
    <string name="filter_all">All</string>
</resources>
```

`android/app/src/main/res/values-es/strings.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="a11y_label">Ayudante del agente de ADBuddy</string>
    <string name="a11y_description">Permite que el agente de código de tu PC lea la pantalla y toque por ti, siempre visible en pantalla y con un botón de Pausa. Nunca lee contraseñas ni actúa en apps protegidas.</string>

    <string name="channel_status">Conexión con el PC</string>
    <string name="channel_confirm">Confirmaciones</string>
    <string name="status_connected">Conectado a %1$s</string>
    <string name="status_connecting">Conectando con el PC…</string>
    <string name="status_waiting">Esperando al PC (ejecuta adbuddy serve)</string>
    <string name="status_paused">En pausa: el agente no puede usar el móvil</string>
    <string name="status_update_app">Esta app es demasiado antigua para el ADBuddy de tu PC: ejecuta adbuddy setup en el PC para actualizarla</string>
    <string name="status_update_host">El ADBuddy de tu PC es demasiado antiguo para esta app: actualízalo en el PC</string>

    <string name="action_allow">Permitir</string>
    <string name="action_deny">Rechazar</string>
    <string name="action_allow_session">Permitir durante la sesión</string>
    <string name="action_pause">Pausar</string>
    <string name="action_resume">Reanudar</string>
    <string name="action_cut">Cortar acceso</string>
    <string name="action_restore">Reactivar acceso</string>

    <string name="confirm_title">El agente quiere: %1$s</string>
    <string name="confirm_special">Puede cortar la conexión con el PC o afectar a ADBuddy.</string>
    <string name="confirm_expires">Caduca a las %1$s</string>

    <string name="consent_title">Configurar ADBuddy con %1$s</string>
    <string name="consent_intro">El PC "%1$s" pide terminar de configurar ADBuddy. Si aceptas, va a:</string>
    <string name="consent_grant">Dar a ADBuddy el permiso para cambiar ajustes seguros, que solo usa "Cortar acceso" para apagar la depuración desde el móvil.</string>
    <string name="consent_battery">Quitar a ADBuddy de la optimización de batería, para que el canal con el PC siga abierto.</string>
    <string name="consent_a11y">Activar el servicio de accesibilidad de ADBuddy, sin quitar ninguno de los que ya uses.</string>
    <string name="consent_note">Nada sale de tu red: el PC y el móvil hablan por la conexión adb que acabas de emparejar.</string>
    <string name="consent_accept">Aceptar</string>
    <string name="consent_decline">Ahora no</string>
    <string name="consent_done">Listo. Puedes volver al PC.</string>

    <string name="home_activity">Actividad</string>
    <string name="home_empty">Nada todavía. Aquí aparecerá cada comando adb del agente.</string>
    <string name="cut_done">La depuración está apagada: el PC no puede llegar al móvil.</string>
    <string name="cut_restored">La depuración vuelve a estar activa. Si el modo remoto estaba activo, su puerto de red sigue abierto hasta que el PC se conecte o reinicies el móvil.</string>
    <string name="cut_failed">ADBuddy no tiene el permiso: ejecuta adbuddy setup en el PC.</string>
    <string name="filter_all">Todo</string>
</resources>
```

`android/app/src/main/res/xml/network_security_config.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<!-- The only plain-text traffic is the channel with the host through
     `adb reverse` on the phone's own loopback (spec §5.2). -->
<network-security-config>
    <base-config cleartextTrafficPermitted="false" />
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">127.0.0.1</domain>
    </domain-config>
</network-security-config>
```

`android/app/src/main/res/xml/accessibility_service.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<!-- M2 only needs the service enabled and bound (spec §3.1 step 6, M2
     validation on another maker's Android 13-15); reading and acting on the
     screen arrive in M5. -->
<accessibility-service xmlns:android="http://schemas.android.com/apk/res/android"
    android:accessibilityEventTypes="typeWindowStateChanged"
    android:accessibilityFeedbackType="feedbackGeneric"
    android:notificationTimeout="100"
    android:canRetrieveWindowContent="true"
    android:description="@string/a11y_description" />
```

`android/app/src/main/res/xml/data_extraction_rules.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<!-- Nothing leaves the phone: the link token and the consent stay on this device
     (Android 12+ ignores allowBackup for device-to-device transfers). -->
<data-extraction-rules>
    <cloud-backup>
        <exclude domain="root" />
        <exclude domain="file" />
        <exclude domain="database" />
        <exclude domain="sharedpref" />
        <exclude domain="external" />
    </cloud-backup>
    <device-transfer>
        <exclude domain="root" />
        <exclude domain="file" />
        <exclude domain="database" />
        <exclude domain="sharedpref" />
        <exclude domain="external" />
    </device-transfer>
</data-extraction-rules>
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Protocol.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement

/** The app channel protocol (spec §5.3), version 1. Mirrors host/internal/bridge. */
object Protocol {
    const val VERSION = 1

    /** The oldest host protocol this app talks to. */
    const val MIN_HOST_PROTOCOL = 1

    /** Close code the host uses for an app that is too old (bridge.CloseIncompatible). */
    const val CLOSE_INCOMPATIBLE = 4001

    const val HELLO = "hello"
    const val WELCOME = "welcome"
    const val SNAPSHOT = "snapshot"
    const val SESSION_STATE = "session.state"
    const val CONFIRM_REQUEST = "confirm.request"
    const val CONFIRM_REPLY = "confirm.reply"
    const val JOURNAL_ENTRY = "journal.entry"
    const val NOTICE = "notice"
    const val PAUSE = "pause"
    const val RESUME = "resume"

    val json = Json {
        ignoreUnknownKeys = true // newer hosts may add fields
        explicitNulls = false
        encodeDefaults = true
    }

    fun encodeElement(type: String, payload: JsonElement, ts: String): String =
        json.encodeToString(Message.serializer(), Message(v = VERSION, type = type, ts = ts, payload = payload))

    inline fun <reified T> encode(type: String, payload: T, ts: String): String =
        encodeElement(type, json.encodeToJsonElement(payload), ts)

    fun decode(text: String): Message? = try {
        json.decodeFromString(Message.serializer(), text).takeIf { it.v == VERSION && it.type.isNotEmpty() }
    } catch (e: IllegalArgumentException) { // SerializationException is one
        null
    }

    inline fun <reified T> payload(m: Message): T? = try {
        json.decodeFromJsonElement<T>(m.payload)
    } catch (e: IllegalArgumentException) {
        null
    }
}

@Serializable
data class Message(
    val v: Int,
    val seq: Long = 0,
    val type: String,
    val ts: String = "",
    val payload: JsonElement = JsonObject(emptyMap()),
)

@Serializable
data class Hello(
    val token: String,
    val lastSeq: Long,
    val session: String = "", // the session of the last welcome: another one means the host restarted
    val appVersion: String,
    val deviceSerial: String = "",
    val protocol: Int = Protocol.VERSION,
)

@Serializable
data class Welcome(val session: String, val fromSeq: Long, val hostVersion: String = "", val protocol: Int = 0)

@Serializable
data class SessionState(val agent: String = "", val state: String = "idle", val activity: String = "", val paused: Boolean = false)

/** A 🔴 command waiting for an answer. Title and detail come from the host's rules, never from the agent. */
@Serializable
data class ConfirmRequest(
    val id: String,
    val level: String = "red",
    val title: String,
    val detail: String = "",
    val rawCommand: String,
    val expiresAt: String,
    val special: Boolean = false,
    val allowSession: Boolean = false,
)

@Serializable
data class ConfirmReply(val id: String, val decision: String)

@Serializable
data class Notice(val level: String = "info", val text: String)

/** The fields of a journal entry the app shows (spec §4.4). */
@Serializable
data class JournalEntry(
    val id: String,
    val ts: String = "",
    val session: String = "",
    val source: String = "",
    val argv: List<String> = emptyList(),
    val level: String = "",
    val description: String = "",
    val status: String = "",
    val reason: String = "",
    val inverse: Inverse? = null,
    val undoneBy: String? = null,
)

@Serializable
data class Inverse(val argv: List<String> = emptyList())

@Serializable
data class Snapshot(
    val state: SessionState = SessionState(),
    val pending: List<ConfirmRequest> = emptyList(),
    val journal: List<JsonElement> = emptyList(),
)

object Decision {
    const val ALLOW = "allow"
    const val DENY = "deny"
    const val ALLOW_SESSION = "allow_session"
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

/** What the app knows about the host, rebuilt from the channel's messages. Pure, so it is unit-tested. */
data class BridgeState(
    val connection: Connection = Connection.WAITING,
    val hostName: String = "",
    val session: String = "",
    val lastSeq: Long = 0,
    val sessionState: SessionState = SessionState(),
    val pending: List<ConfirmRequest> = emptyList(),
    val journal: List<JournalEntry> = emptyList(), // newest last
    val outdated: Outdated = Outdated.NONE,
) {
    /** INCOMPATIBLE: the app or the host must be updated; the app stops reconnecting. */
    enum class Connection { WAITING, CONNECTING, CONNECTED, INCOMPATIBLE }

    /** Which side is too old when the connection is INCOMPATIBLE. */
    enum class Outdated { NONE, APP, HOST }

    companion object {
        const val MAX_JOURNAL = 500
    }

    /** Applies one message from the host. */
    fun apply(m: Message): BridgeState {
        val seq = if (m.seq > lastSeq) m.seq else lastSeq
        return when (m.type) {
            Protocol.WELCOME -> {
                val w = Protocol.payload<Welcome>(m) ?: return this
                if (w.protocol < Protocol.MIN_HOST_PROTOCOL) {
                    return copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST)
                }
                // A new session (the host restarted) starts over.
                if (w.session != session) copy(session = w.session, lastSeq = 0, pending = emptyList(), connection = Connection.CONNECTED)
                else copy(connection = Connection.CONNECTED)
            }
            Protocol.SNAPSHOT -> {
                val s = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq)
                val entries = s.journal.mapNotNull {
                    try { Protocol.json.decodeFromJsonElement(JournalEntry.serializer(), it) } catch (e: IllegalArgumentException) { null }
                }
                copy(lastSeq = seq, sessionState = s.state, pending = s.pending, journal = entries.takeLast(MAX_JOURNAL))
            }
            Protocol.SESSION_STATE -> copy(lastSeq = seq, sessionState = Protocol.payload<SessionState>(m) ?: sessionState)
            Protocol.CONFIRM_REQUEST -> {
                val r = Protocol.payload<ConfirmRequest>(m) ?: return copy(lastSeq = seq)
                copy(lastSeq = seq, pending = pending.filter { it.id != r.id } + r)
            }
            Protocol.JOURNAL_ENTRY -> {
                val e = Protocol.payload<JournalEntry>(m) ?: return copy(lastSeq = seq)
                // A final record of a pending question means it was answered (here, on the PC or by expiry).
                val stillPending = if (e.status != "running") pending.filter { it.id != e.id } else pending
                copy(lastSeq = seq, pending = stillPending, journal = (journal.filter { it.id != e.id } + e).takeLast(MAX_JOURNAL))
            }
            else -> copy(lastSeq = seq)
        }
    }

    /** The host closed the channel because this app is too old. */
    fun appOutdated(): BridgeState = copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP, pending = emptyList())

    /** Removes a question the person just answered here. */
    fun answered(id: String): BridgeState = copy(pending = pending.filter { it.id != id })
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Handshake.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

/** The handshake the host writes on stdin (`content write …/handshake`, spec §5.2). */
@Serializable
data class Handshake(val token: String, val port: Int, val host: String = "", val nonce: String = "")

object HandshakeParser {
    private const val MAX_BYTES = 4096
    private val TOKEN = Regex("^[0-9a-f]{64}$")
    private val HOST = Regex("^[A-Za-z0-9 ._-]{1,40}$")
    private val NONCE = Regex("^[0-9a-f]{32}$")

    /** Reads at most MAX_BYTES + 1 bytes (InputStream.readNBytes needs API 33). */
    fun readLimited(input: java.io.InputStream): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        val buf = ByteArray(1024)
        while (out.size() <= MAX_BYTES) {
            val n = input.read(buf)
            if (n < 0) break
            out.write(buf, 0, n)
        }
        return out.toByteArray()
    }

    /** Returns the handshake, or null when anything is off: then the app keeps the previous one. */
    fun parse(bytes: ByteArray): Handshake? {
        if (bytes.size > MAX_BYTES) return null
        val h = try {
            Protocol.json.decodeFromString(Handshake.serializer(), bytes.decodeToString())
        } catch (e: IllegalArgumentException) {
            return null
        }
        if (!TOKEN.matches(h.token) || h.port !in 1024..65535 || !NONCE.matches(h.nonce)) return null
        val host = if (HOST.matches(h.host)) h.host else "PC"
        return h.copy(host = host)
    }
}

/** Only the shell user (adb) may talk to the provider (spec §5.2). */
object Caller {
    /** android.os.Process.SHELL_UID, a hidden constant. */
    const val SHELL_UID = 2000

    fun isShell(uid: Int): Boolean = uid == SHELL_UID
}
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest --tests 'tech.hybriq.adbuddy.bridge.ProtocolTest' --tests 'tech.hybriq.adbuddy.bridge.BridgeStateTest' --tests 'tech.hybriq.adbuddy.bridge.HandshakeTest'
```

Esperado: `BUILD SUCCESSFUL` y todos los tests en verde.

- [ ] **Paso 5: Commit**

```bash
git add android
git commit -F - <<'EOF'
feat(android): proyecto de la app, protocolo v1 y estado del canal

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 9: Cliente del canal y preferencias del enlace

`Bridge` es el cliente OkHttp de `ws://127.0.0.1:<puerto>/v1`:
- manda `hello` con el token, `lastSeq` y la `session` del último `welcome` (si no coincide, el host sabe que se reinició y manda un `snapshot`);
- aplica cada mensaje a `BridgeState`;
- reconecta con espera exponencial (de 0,5 s hasta 30 s; vuelve a 1 s tras haber estado conectado);
- responde `confirm.reply` y envía `pause`/`resume`;
- si el host cierra con 4001 (app demasiado antigua) o su `welcome` es de un protocolo antiguo, deja de reconectar y el estado dice qué hay que actualizar.

`BridgePrefs` guarda en las preferencias privadas el enlace (token, puerto y nombre del PC) y el consentimiento. `AdbuddyApp` crea los canales de notificación y guarda el `Bridge`. El test JVM usa MockWebServer como host falso.

**Ficheros:**
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/BuildConfigInfo.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgePrefs.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/AdbuddyApp.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt`

**Interfaces:**
- Consume: `Protocol`, `BridgeState` (8).
- Produce: `BridgePrefs` (`link: Link?`, `consent`, `consentHost`), `BridgePrefs.Link(token, port, host)`; `BuildConfigInfo.versionName(context)`; `Bridge(scope, appVersion, onMessage)` con `state: StateFlow<BridgeState>`, `start(Link)`, `stop()`, `reply(id, decision): Boolean`, `pause(Boolean): Boolean`; `AdbuddyApp.bridge`, `CHANNEL_STATUS`, `CHANNEL_CONFIRM`.

- [ ] **Paso 1: Escribir los tests que fallan**

`android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The client against a fake host speaking the protocol. */
class BridgeTest {
    private val server = MockWebServer()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val fromApp = LinkedBlockingQueue<String>()
    private var hostSide: WebSocket? = null
    private val token = "0123456789abcdef".repeat(4)

    @Before fun setUp() {
        server.enqueue(MockResponse.Builder().webSocketUpgrade(object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) { hostSide = webSocket }
            override fun onMessage(webSocket: WebSocket, text: String) { fromApp.add(text) }
            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { webSocket.close(1000, null) }
        }).build())
        server.start()
    }

    @After fun tearDown() {
        scope.cancel()
        server.close()
    }

    @Test fun helloQuestionAndReply() {
        val bridge = Bridge(scope, "0.2.0") { _, _ -> }
        bridge.start(BridgePrefs.Link(token, server.port, "PC"))

        val hello = Protocol.decode(fromApp.poll(5, TimeUnit.SECONDS)!!)!!
        assertEquals(Protocol.HELLO, hello.type)
        assertEquals(token, Protocol.payload<Hello>(hello)!!.token)
        assertEquals(Protocol.VERSION, Protocol.payload<Hello>(hello)!!.protocol)

        hostSide!!.send("""{"v":1,"type":"welcome","ts":"x","payload":{"session":"s_1","fromSeq":1,"protocol":1}}""")
        hostSide!!.send("""{"v":1,"seq":1,"type":"confirm.request","ts":"x","payload":{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}}""")
        val deadline = System.currentTimeMillis() + 5000
        while (bridge.state.value.pending.isEmpty() && System.currentTimeMillis() < deadline) Thread.sleep(10)
        assertEquals(1, bridge.state.value.pending.size)

        assertTrue(bridge.reply("j_1", Decision.DENY))
        val reply = Protocol.decode(fromApp.poll(5, TimeUnit.SECONDS)!!)!!
        assertEquals(Protocol.CONFIRM_REPLY, reply.type)
        val r = Protocol.payload<ConfirmReply>(reply)
        assertNotNull(r)
        assertEquals("deny", r!!.decision)
        assertTrue(bridge.state.value.pending.isEmpty())
        bridge.stop()
    }

    @Test fun aHostThatSaysUpdateStopsTheRetries() {
        val bridge = Bridge(scope, "0.1.0") { _, _ -> }
        bridge.start(BridgePrefs.Link(token, server.port, "PC"))
        fromApp.poll(5, TimeUnit.SECONDS)!! // hello
        hostSide!!.close(Protocol.CLOSE_INCOMPATIBLE, "update the ADBuddy app")
        val deadline = System.currentTimeMillis() + 5000
        while (bridge.state.value.connection != BridgeState.Connection.INCOMPATIBLE && System.currentTimeMillis() < deadline) Thread.sleep(10)
        assertEquals(BridgeState.Connection.INCOMPATIBLE, bridge.state.value.connection)
        assertEquals(BridgeState.Outdated.APP, bridge.state.value.outdated)
        Thread.sleep(1500) // longer than the first retry
        assertEquals("no reconnection after an incompatible close", 1, server.requestCount)
        bridge.stop()
    }
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest
```

Esperado: no compila: `Unresolved reference: Bridge`.

- [ ] **Paso 3: Implementar**

`android/app/src/main/kotlin/tech/hybriq/adbuddy/BuildConfigInfo.kt`:

```kotlin
package tech.hybriq.adbuddy

import android.content.Context

/** The app's version, read from the package (AGP 9 does not generate BuildConfig by default). */
object BuildConfigInfo {
    fun versionName(context: Context): String =
        context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: ""
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgePrefs.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import android.content.Context
import androidx.core.content.edit

/**
 * What the host handed over (token, port, name) and the setup consent. Kept in
 * the app's private storage: the release build is not debuggable, so no other
 * app nor `run-as` can read it (and the host's classifier blocks `run-as` on
 * ADBuddy anyway).
 */
class BridgePrefs(context: Context) {
    private val prefs = context.getSharedPreferences("bridge", Context.MODE_PRIVATE)

    /** nonce: what the host sent with this token; ping returns it so the host knows the app took it. */
    data class Link(val token: String, val port: Int, val host: String, val nonce: String = "")

    var link: Link?
        get() {
            val token = prefs.getString("token", null) ?: return null
            val port = prefs.getInt("port", 0)
            if (port !in 1024..65535) return null
            return Link(token, port, prefs.getString("host", "") ?: "", prefs.getString("nonce", "") ?: "")
        }
        set(value) = prefs.edit(commit = true) {
            if (value == null) {
                remove("token"); remove("port"); remove("host"); remove("nonce")
            } else {
                putString("token", value.token); putInt("port", value.port); putString("host", value.host)
                putString("nonce", value.nonce)
            }
        }

    /** pending (asked, no answer yet), granted, denied, or "" (never asked). */
    var consent: String
        get() = prefs.getString("consent", "") ?: ""
        set(value) = prefs.edit(commit = true) { putString("consent", value) }

    var consentHost: String
        get() = prefs.getString("consentHost", "") ?: ""
        set(value) = prefs.edit(commit = true) { putString("consentHost", value) }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

/**
 * The app end of the channel (spec §5.2): a WebSocket client to
 * ws://127.0.0.1:<port>/v1, which `adb reverse` carries to the host. One per
 * process; BridgeService keeps the process alive while it runs.
 */
class Bridge(
    private val scope: CoroutineScope,
    private val appVersion: String,
    private val onMessage: (Message, BridgeState) -> Unit,
) {
    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .build()

    private val _state = MutableStateFlow(BridgeState())
    val state: StateFlow<BridgeState> = _state.asStateFlow()

    private var socket: WebSocket? = null
    private var loop: Job? = null
    @Volatile private var link: BridgePrefs.Link? = null

    /** Connects with link, and keeps reconnecting until stop(). */
    fun start(link: BridgePrefs.Link) {
        this.link = link
        _state.update { it.copy(hostName = link.host) }
        loop?.cancel()
        socket?.close(1000, "new link")
        loop = scope.launch {
            var backoff = 500L
            while (true) {
                val closed = CompletableDeferred<Unit>()
                connect(link, closed)
                closed.await()
                if (_state.value.connection == BridgeState.Connection.INCOMPATIBLE) break // retrying cannot help
                val wasConnected = _state.value.connection == BridgeState.Connection.CONNECTED
                _state.update { it.copy(connection = BridgeState.Connection.WAITING) }
                // Back off while the host is not there (serve not running, PC asleep).
                backoff = if (wasConnected) 1_000L else (backoff * 2).coerceAtMost(30_000L)
                delay(backoff)
            }
        }
    }

    fun stop() {
        loop?.cancel()
        socket?.close(1000, "bye")
        socket = null
        _state.update { it.copy(connection = BridgeState.Connection.WAITING) }
    }

    private fun connect(link: BridgePrefs.Link, closed: CompletableDeferred<Unit>) {
        _state.update { it.copy(connection = BridgeState.Connection.CONNECTING, outdated = BridgeState.Outdated.NONE) }
        val req = Request.Builder().url("ws://127.0.0.1:${link.port}/v1").build()
        socket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                val hello = Hello(token = link.token, lastSeq = _state.value.lastSeq, session = _state.value.session, appVersion = appVersion)
                webSocket.send(Protocol.encode(Protocol.HELLO, hello, now()))
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val m = Protocol.decode(text) ?: return
                val next = _state.updateAndGetCompat { it.apply(m) }
                if (next.connection == BridgeState.Connection.INCOMPATIBLE) {
                    webSocket.close(1000, "host too old")
                    return
                }
                onMessage(m, next)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                if (code == Protocol.CLOSE_INCOMPATIBLE) _state.update { it.appOutdated() }
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (code == Protocol.CLOSE_INCOMPATIBLE) _state.update { it.appOutdated() }
                closed.complete(Unit)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                closed.complete(Unit)
            }
        })
    }

    /** Sends the answer to a 🔴 question. Returns false when the host is not reachable. */
    fun reply(id: String, decision: String): Boolean {
        val ok = socket?.send(Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply(id, decision), now())) ?: false
        if (ok) _state.update { it.answered(id) }
        return ok
    }

    fun pause(paused: Boolean): Boolean =
        socket?.send(Protocol.encodeElement(if (paused) Protocol.PAUSE else Protocol.RESUME, JsonObject(emptyMap()), now())) ?: false

    private fun now() = Instant.now().toString()

    private inline fun MutableStateFlow<BridgeState>.updateAndGetCompat(f: (BridgeState) -> BridgeState): BridgeState {
        while (true) {
            val prev = value
            val next = f(prev)
            if (compareAndSet(prev, next)) return next
        }
    }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/AdbuddyApp.kt`:

```kotlin
package tech.hybriq.adbuddy

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import tech.hybriq.adbuddy.bridge.Bridge

class AdbuddyApp : Application() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /** The channel with the host, shared by the service, the receivers and the UI. */
    lateinit var bridge: Bridge
        private set

    override fun onCreate() {
        super.onCreate()
        bridge = Bridge(scope, BuildConfigInfo.versionName(this)) { _, _ -> }
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(CHANNEL_STATUS, getString(R.string.channel_status), NotificationManager.IMPORTANCE_LOW))
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONFIRM, getString(R.string.channel_confirm), NotificationManager.IMPORTANCE_HIGH))
    }

    companion object {
        const val CHANNEL_STATUS = "status"
        const val CHANNEL_CONFIRM = "confirm"
    }
}
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest
```

Esperado: `BUILD SUCCESSFUL` y todos los tests en verde.

- [ ] **Paso 5: Commit**

```bash
git add android
git commit -F - <<'EOF'
feat(android): cliente del canal con reconexión y preferencias del enlace

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 10: `TokenProvider`, servicio en primer plano e interfaz

Esta tarea completa la app de §3.1 (paso 6.1), §4.5-4.6, §5.2 y §6. Sus piezas se usan entre sí (el proveedor arranca el servicio, el servicio abre `MainActivity` y sincroniza las notificaciones), por eso van juntas.
- **`TokenProvider`:** `ContentProvider` exportado que solo atiende al uid 2000: `call` devuelve `error=not the shell user` a cualquier otro, y `openFile` lanza `SecurityException`. Métodos: `ping` (versión y puerto), `requestSetup` (guarda el nombre del PC y deja el consentimiento en `pending`) y `getSetupConsent`. `openFile("/handshake", "w")` devuelve una tubería; el JSON que escribe el host se valida (token de 64 hex, puerto 1024-65535, nombre saneado), se guarda en `BridgePrefs` y se arranca `BridgeService`.
- **`BridgeService`:** servicio en primer plano `specialUse` (con tipo solo desde Android 14) que mantiene vivo el proceso, con una notificación fija con el estado y el botón Pausar/Reanudar.
- **Confirmaciones:** cada pregunta 🔴 es una notificación de prioridad alta con Rechazar, Permitir y, si el host lo ofrece, Permitir durante la sesión. Permitir exige desbloquear (`setAuthenticationRequired`, Android 12+). La notificación caduca con la pregunta y desaparece cuando se responde en otro sitio. Con la app abierta, la misma pregunta sale en un diálogo.
- **Consentimiento:** la pantalla solo aparece mientras el host espera, con lo que hará el PC.
- **Inicio:** estado de la conexión, Pausar/Reanudar, Cortar/Reactivar acceso (S1: `adb_wifi_enabled=0` y `adb_enabled=0`) y el diario con filtro por nivel.
- **Accesibilidad:** el servicio mínimo que activa el host; su trabajo llega en M5.

**Ficheros:**
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/TokenProvider.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeService.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/confirm/ConfirmNotifier.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/confirm/ConfirmReceiver.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/control/CutAccess.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/AgentAccessibilityService.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/MainActivity.kt`
- Crear: `android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt`
- Test: `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TokenProviderTest.kt`

**Interfaces:**
- Consume: todo lo anterior.
- Produce: `TokenProvider`; `BridgeService.start(context)`, `ACTION_PAUSE`/`ACTION_RESUME`; `ConfirmNotifier.sync(context, state)`, `ConfirmReceiver` (`ACTION_REPLY`, `EXTRA_ID`, `EXTRA_DECISION`); `CutAccess.cut/restore/isCut`; `AgentAccessibilityService`; `MainActivity` (`ACTION_SETUP`, `EXTRA_CONFIRM`); `ConsentScreen`, `HomeScreen`, `ConfirmDialog`.

> Tras esta tarea el proyecto debe compilar entero y sin avisos de lint: `gradle :app:assembleDebug :app:lintDebug`.

> El test instrumentado del `TokenProvider` necesita un emulador (`gradle :app:connectedDebugAndroidTest`); nunca el móvil del usuario.

- [ ] **Paso 1: Escribir los tests que fallan**

`android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TokenProviderTest.kt`:

```kotlin
package tech.hybriq.adbuddy

import android.net.Uri
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Spec §10: the TokenProvider refuses anyone but the shell user. The test
 * runs in the app's own process (not uid 2000), like any other app would.
 */
@RunWith(AndroidJUnit4::class)
class TokenProviderTest {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val uri = Uri.parse("content://tech.hybriq.adbuddy.bridge")

    @Test fun callsFromAnAppAreRefused() {
        for (method in listOf("ping", "requestSetup", "getSetupConsent")) {
            val res = context.contentResolver.call(uri, method, null, null)!!
            assertEquals(method, "not the shell user", res.getString("error"))
        }
    }

    @Test fun handshakeFromAnAppIsRefused() {
        try {
            context.contentResolver.openOutputStream(Uri.parse("content://tech.hybriq.adbuddy.bridge/handshake"))!!.use {
                it.write("{}".toByteArray())
            }
            fail("an app could write the handshake")
        } catch (e: SecurityException) {
            // expected
        }
    }
}
```

- [ ] **Paso 2: Ejecutarlos y ver que fallan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug
```

Esperado: `assembleDebug` falla: el manifiesto declara `TokenProvider`, `BridgeService`, `MainActivity`, `ConfirmReceiver` y `AgentAccessibilityService`, que aún no existen.

- [ ] **Paso 3: Implementar**

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/TokenProvider.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.net.Uri
import android.os.Binder
import android.os.Bundle
import android.os.ParcelFileDescriptor
import java.io.FileNotFoundException
import tech.hybriq.adbuddy.BuildConfigInfo

/**
 * The host's door into the app (spec §5.2, §3.1 step 6). Exported, so adb can
 * reach it, but it answers only the shell user (uid 2000): every other app
 * gets an error.
 *
 * - `content write --uri content://tech.hybriq.adbuddy.bridge/handshake` with
 *   the token, port and PC name as JSON on stdin (never in a command line).
 * - `content call … --method requestSetup --extra host:s:<name>` opens a
 *   consent request; `getSetupConsent` answers pending, granted or denied.
 * - `content call … --method ping` answers the port the app will use and the
 *   nonce of the last handshake it took (not a secret: it proves which one).
 */
class TokenProvider : ContentProvider() {
    private lateinit var prefs: BridgePrefs

    override fun onCreate(): Boolean {
        prefs = BridgePrefs(context!!)
        return true
    }

    override fun call(method: String, arg: String?, extras: Bundle?): Bundle {
        if (!Caller.isShell(Binder.getCallingUid())) return error("not the shell user")
        return when (method) {
            "ping" -> Bundle().apply {
                putBoolean("ok", true)
                putString("appVersion", BuildConfigInfo.versionName(context!!))
                putInt("port", prefs.link?.port ?: 0)
                putString("nonce", prefs.link?.nonce ?: "")
            }
            "requestSetup" -> {
                val host = extras?.getString("host")?.takeIf { Regex("^[A-Za-z0-9 ._-]{1,40}$").matches(it) } ?: "PC"
                prefs.consentHost = host
                prefs.consent = "pending"
                Bundle().apply { putBoolean("ok", true) }
            }
            "getSetupConsent" -> Bundle().apply { putString("consent", prefs.consent.ifEmpty { "pending" }) }
            else -> error("unknown method")
        }
    }

    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor {
        if (!Caller.isShell(Binder.getCallingUid())) throw SecurityException("not the shell user")
        if (uri.path != "/handshake" || !mode.startsWith("w")) throw FileNotFoundException(uri.toString())
        val (read, write) = ParcelFileDescriptor.createReliablePipe()
        val ctx = context!!.applicationContext
        Thread {
            val bytes = ParcelFileDescriptor.AutoCloseInputStream(read).use { HandshakeParser.readLimited(it) }
            val h = HandshakeParser.parse(bytes)
            if (h != null) {
                prefs.link = BridgePrefs.Link(h.token, h.port, h.host, h.nonce)
                try {
                    BridgeService.start(ctx)
                } catch (e: IllegalStateException) {
                    // Android 12+ may refuse a foreground service started from the
                    // background (ForegroundServiceStartNotAllowedException): the
                    // link is saved, and the service starts when the app is opened.
                    android.util.Log.w("ADBuddy", "could not start the channel service", e)
                }
            }
        }.start()
        return write
    }

    private fun error(message: String) = Bundle().apply { putString("error", message) }

    // The provider exposes no table.
    override fun query(uri: Uri, p: Array<String>?, s: String?, a: Array<String>?, o: String?): Cursor? = null
    override fun getType(uri: Uri): String? = null
    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun delete(uri: Uri, s: String?, a: Array<String>?): Int = 0
    override fun update(uri: Uri, v: ContentValues?, s: String?, a: Array<String>?): Int = 0
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeService.kt`:

```kotlin
package tech.hybriq.adbuddy.bridge

import android.app.Notification
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.confirm.ConfirmReceiver

/**
 * Keeps the channel with the host open (spec §6: foreground service of type
 * specialUse) and shows its state in a persistent notification with Pause.
 */
class BridgeService : LifecycleService() {

    override fun onCreate() {
        super.onCreate()
        // specialUse exists from Android 14; before that the service has no type.
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0
        ServiceCompat.startForeground(this, NOTIFICATION_ID, statusNotification(BridgeState()), type)
        val app = application as AdbuddyApp
        lifecycleScope.launch {
            app.bridge.state.collect { s ->
                getSystemService(android.app.NotificationManager::class.java).notify(NOTIFICATION_ID, statusNotification(s))
                ConfirmNotifier.sync(this@BridgeService, s)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        val app = application as AdbuddyApp
        when (intent?.action) {
            ACTION_PAUSE -> app.bridge.pause(true)
            ACTION_RESUME -> app.bridge.pause(false)
            else -> BridgePrefs(this).link?.let { app.bridge.start(it) } ?: stopSelf()
        }
        return START_STICKY
    }

    override fun onDestroy() {
        (application as AdbuddyApp).bridge.stop()
        super.onDestroy()
    }

    private fun statusNotification(s: BridgeState): Notification {
        val text = when {
            s.connection == BridgeState.Connection.INCOMPATIBLE && s.outdated == BridgeState.Outdated.APP -> getString(R.string.status_update_app)
            s.connection == BridgeState.Connection.INCOMPATIBLE -> getString(R.string.status_update_host)
            s.sessionState.paused -> getString(R.string.status_paused)
            s.connection == BridgeState.Connection.CONNECTED -> getString(R.string.status_connected, s.hostName)
            s.connection == BridgeState.Connection.CONNECTING -> getString(R.string.status_connecting)
            else -> getString(R.string.status_waiting)
        }
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val b = NotificationCompat.Builder(this, AdbuddyApp.CHANNEL_STATUS)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
        if (s.connection == BridgeState.Connection.CONNECTED) {
            val paused = s.sessionState.paused
            val action = if (paused) ACTION_RESUME else ACTION_PAUSE
            val pi = PendingIntent.getService(this, if (paused) 2 else 1,
                Intent(this, BridgeService::class.java).setAction(action), PendingIntent.FLAG_IMMUTABLE)
            b.addAction(0, getString(if (paused) R.string.action_resume else R.string.action_pause), pi)
        }
        return b.build()
    }

    companion object {
        const val NOTIFICATION_ID = 1
        const val ACTION_PAUSE = "tech.hybriq.adbuddy.action.PAUSE"
        const val ACTION_RESUME = "tech.hybriq.adbuddy.action.RESUME"

        /** Starts (or restarts with the new link) the service. */
        fun start(context: Context) {
            context.startForegroundService(Intent(context, BridgeService::class.java))
        }
    }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/confirm/ConfirmNotifier.kt`:

```kotlin
package tech.hybriq.adbuddy.confirm

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * Shows each pending 🔴 question as a high-priority notification (spec §4.5)
 * with Allow, Deny and, when the host offers it, Allow during the session.
 * Allowing needs the phone unlocked (setAuthenticationRequired, Android 12+).
 * Questions answered elsewhere or expired disappear.
 */
object ConfirmNotifier {
    private val shown = mutableSetOf<String>()

    @Synchronized
    fun sync(context: Context, state: BridgeState) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val ids = state.pending.map { it.id }.toSet()
        for (id in shown - ids) nm.cancel(id, NOTIFICATION_ID)
        shown.retainAll(ids)
        for (r in state.pending) {
            if (r.id in shown) continue
            val expires = try { Instant.parse(r.expiresAt) } catch (e: java.time.format.DateTimeParseException) { continue }
            val left = expires.toEpochMilli() - System.currentTimeMillis()
            if (left <= 0) continue
            nm.notify(r.id, NOTIFICATION_ID, build(context, r, left, expires))
            shown += r.id
        }
    }

    private fun build(context: Context, r: ConfirmRequest, left: Long, expires: Instant): android.app.Notification {
        val time = DateTimeFormatter.ofPattern("HH:mm:ss").withZone(ZoneId.systemDefault()).format(expires)
        val body = buildString {
            append(r.rawCommand)
            if (r.detail.isNotEmpty()) append("\n").append(r.detail)
            if (r.special) append("\n⚠ ").append(context.getString(R.string.confirm_special))
            append("\n").append(context.getString(R.string.confirm_expires, time))
        }
        val open = PendingIntent.getActivity(context, r.id.hashCode(),
            Intent(context, MainActivity::class.java).putExtra(MainActivity.EXTRA_CONFIRM, r.id),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val b = NotificationCompat.Builder(context, AdbuddyApp.CHANNEL_CONFIRM)
            .setSmallIcon(android.R.drawable.ic_dialog_alert)
            .setContentTitle(context.getString(R.string.confirm_title, r.title))
            .setContentText(r.rawCommand)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setContentIntent(open)
            .setTimeoutAfter(left)
            .setOnlyAlertOnce(true)
            .addAction(action(context, r.id, Decision.DENY, R.string.action_deny, auth = false))
            .addAction(action(context, r.id, Decision.ALLOW, R.string.action_allow, auth = true))
        if (r.allowSession) b.addAction(action(context, r.id, Decision.ALLOW_SESSION, R.string.action_allow_session, auth = true))
        return b.build()
    }

    private fun action(context: Context, id: String, decision: String, label: Int, auth: Boolean): NotificationCompat.Action {
        val intent = Intent(context, ConfirmReceiver::class.java)
            .setAction(ConfirmReceiver.ACTION_REPLY)
            .putExtra(ConfirmReceiver.EXTRA_ID, id)
            .putExtra(ConfirmReceiver.EXTRA_DECISION, decision)
        val pi = PendingIntent.getBroadcast(context, (id + decision).hashCode(), intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        return NotificationCompat.Action.Builder(0, context.getString(label), pi)
            .setAuthenticationRequired(auth) // unlocking is required to allow (Android 12+)
            .build()
    }

    const val NOTIFICATION_ID = 2
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/confirm/ConfirmReceiver.kt`:

```kotlin
package tech.hybriq.adbuddy.confirm

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.Decision

/** Receives the notification buttons (not exported: only this app's PendingIntents reach it). */
class ConfirmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_REPLY) return
        val id = intent.getStringExtra(EXTRA_ID) ?: return
        val decision = intent.getStringExtra(EXTRA_DECISION)
            ?.takeIf { it == Decision.ALLOW || it == Decision.DENY || it == Decision.ALLOW_SESSION } ?: return
        (context.applicationContext as AdbuddyApp).bridge.reply(id, decision)
        context.getSystemService(NotificationManager::class.java).cancel(id, ConfirmNotifier.NOTIFICATION_ID)
    }

    companion object {
        const val ACTION_REPLY = "tech.hybriq.adbuddy.action.CONFIRM_REPLY"
        const val EXTRA_ID = "id"
        const val EXTRA_DECISION = "decision"
    }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/control/CutAccess.kt`:

```kotlin
package tech.hybriq.adbuddy.control

import android.content.ContentResolver
import android.provider.Settings

/**
 * "Cut access" (spec §4.6): turns off wireless debugging and then USB/adb
 * debugging with WRITE_SECURE_SETTINGS, without needing the PC. Validated in
 * S1: adb_wifi_enabled=0 alone does not close tcpip mode; both together do.
 */
object CutAccess {
    private const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden

    /** Returns false when the permission is missing (setup was not accepted). */
    fun cut(resolver: ContentResolver): Boolean = try {
        Settings.Global.putInt(resolver, ADB_WIFI_ENABLED, 0)
        Settings.Global.putInt(resolver, Settings.Global.ADB_ENABLED, 0)
        true
    } catch (e: SecurityException) {
        false
    }

    /** Undoes cut(). A tcpip port that was open comes back until the host runs `adb usb` (spec §4.6). */
    fun restore(resolver: ContentResolver): Boolean = try {
        Settings.Global.putInt(resolver, Settings.Global.ADB_ENABLED, 1)
        Settings.Global.putInt(resolver, ADB_WIFI_ENABLED, 1)
        true
    } catch (e: SecurityException) {
        false
    }

    fun isCut(resolver: ContentResolver): Boolean =
        Settings.Global.getInt(resolver, Settings.Global.ADB_ENABLED, 1) == 0
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/AgentAccessibilityService.kt`:

```kotlin
package tech.hybriq.adbuddy.a11y

import android.accessibilityservice.AccessibilityService
import android.util.Log
import android.view.accessibility.AccessibilityEvent

/**
 * ADBuddy's accessibility service. In M2 it only has to exist and bind, so the
 * host can enable it over adb (spec §3.1 step 6) and M2 can validate that on
 * another maker's Android 13-15. Reading the screen, tapping and the overlay
 * arrive in M5.
 */
class AgentAccessibilityService : AccessibilityService() {
    override fun onServiceConnected() {
        Log.i(TAG, "connected")
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) = Unit

    override fun onInterrupt() = Unit

    companion object {
        private const val TAG = "ADBuddyA11y"
    }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/MainActivity.kt`:

```kotlin
package tech.hybriq.adbuddy

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.bridge.BridgeService
import tech.hybriq.adbuddy.ui.ConsentScreen
import tech.hybriq.adbuddy.ui.HomeScreen

class MainActivity : ComponentActivity() {
    private var showConsent by mutableStateOf(false)
    private var focusConfirm by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handle(intent)
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        val app = application as AdbuddyApp
        val prefs = BridgePrefs(this)
        // Opening the app (re)starts the channel when the host already handed over a link.
        if (prefs.link != null) BridgeService.start(this)
        setContent {
            MaterialTheme {
                val state by app.bridge.state.collectAsStateWithLifecycle()
                if (showConsent) {
                    ConsentScreen(
                        host = prefs.consentHost,
                        onAnswer = { accepted ->
                            prefs.consent = if (accepted) "granted" else "denied"
                            if (accepted) Toast.makeText(this, R.string.consent_done, Toast.LENGTH_LONG).show()
                            showConsent = false
                        },
                    )
                } else {
                    HomeScreen(state = state, bridge = app.bridge, focusConfirm = focusConfirm)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent?) {
        // The consent screen only shows while the host is waiting for it.
        if (intent?.action == ACTION_SETUP && BridgePrefs(this).consent == "pending") showConsent = true
        focusConfirm = intent?.getStringExtra(EXTRA_CONFIRM)
    }

    companion object {
        const val ACTION_SETUP = "tech.hybriq.adbuddy.action.SETUP"
        const val EXTRA_CONFIRM = "confirm"
    }
}
```

`android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt`:

```kotlin
package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.control.CutAccess

/** Consent for the PC to finish the setup (spec §3.1 step 6.1). */
@Composable
fun ConsentScreen(host: String, onAnswer: (Boolean) -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.consent_title, host), style = MaterialTheme.typography.headlineSmall)
        Text(stringResource(R.string.consent_intro, host))
        for (item in listOf(R.string.consent_grant, R.string.consent_battery, R.string.consent_a11y)) {
            Text("• " + stringResource(item))
        }
        Text(stringResource(R.string.consent_note), style = MaterialTheme.typography.bodySmall)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(onClick = { onAnswer(false) }) { Text(stringResource(R.string.consent_decline)) }
            Button(onClick = { onAnswer(true) }) { Text(stringResource(R.string.consent_accept)) }
        }
    }
}

/** Connection, Pause, Cut access and the activity panel (spec §6). */
@Composable
fun HomeScreen(state: BridgeState, bridge: Bridge, focusConfirm: String?) {
    val context = LocalContext.current
    var message by remember { mutableStateOf<String?>(null) }
    var filter by remember { mutableStateOf<String?>(null) }
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.home_title), style = MaterialTheme.typography.headlineSmall)
        Text(
            when {
                state.connection == BridgeState.Connection.INCOMPATIBLE && state.outdated == BridgeState.Outdated.APP -> stringResource(R.string.status_update_app)
                state.connection == BridgeState.Connection.INCOMPATIBLE -> stringResource(R.string.status_update_host)
                state.sessionState.paused -> stringResource(R.string.status_paused)
                state.connection == BridgeState.Connection.CONNECTED -> stringResource(R.string.status_connected, state.hostName)
                state.connection == BridgeState.Connection.CONNECTING -> stringResource(R.string.status_connecting)
                else -> stringResource(R.string.status_waiting)
            },
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.connection == BridgeState.Connection.CONNECTED) {
                val paused = state.sessionState.paused
                Button(onClick = { bridge.pause(!paused) }) {
                    Text(stringResource(if (paused) R.string.action_resume else R.string.action_pause))
                }
            }
            val cut = CutAccess.isCut(context.contentResolver)
            val cutFailed = stringResource(R.string.cut_failed)
            val cutRestored = stringResource(R.string.cut_restored)
            val cutDone = stringResource(R.string.cut_done)
            OutlinedButton(onClick = {
                val ok = if (cut) CutAccess.restore(context.contentResolver) else CutAccess.cut(context.contentResolver)
                message = when {
                    !ok -> cutFailed
                    cut -> cutRestored
                    else -> cutDone
                }
            }) { Text(stringResource(if (cut) R.string.action_restore else R.string.action_cut)) }
        }
        message?.let { Text(it, style = MaterialTheme.typography.bodySmall) }

        Text(stringResource(R.string.home_activity), style = MaterialTheme.typography.titleMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text(stringResource(R.string.filter_all)) })
            for (level in listOf("green", "yellow", "red")) {
                FilterChip(selected = filter == level, onClick = { filter = level }, label = { Text(levelIcon(level)) })
            }
        }
        val entries = state.journal.asReversed().filter { filter == null || it.level == filter }
        if (entries.isEmpty()) Text(stringResource(R.string.home_empty))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(entries, key = { it.id }) { EntryRow(it) }
        }
    }
    val question = state.pending.firstOrNull { it.id == focusConfirm } ?: state.pending.firstOrNull()
    if (question != null) ConfirmDialog(question) { decision -> bridge.reply(question.id, decision) }
}

/** The same question as the notification, when the app is open (spec §4.5). */
@Composable
fun ConfirmDialog(r: ConfirmRequest, onAnswer: (String) -> Unit) {
    AlertDialog(
        onDismissRequest = {},
        title = { Text(stringResource(R.string.confirm_title, r.title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(r.rawCommand, style = MaterialTheme.typography.bodySmall)
                if (r.detail.isNotEmpty()) Text(r.detail)
                if (r.special) Text("⚠ " + stringResource(R.string.confirm_special))
            }
        },
        confirmButton = {
            Column {
                TextButton(onClick = { onAnswer(Decision.ALLOW) }) { Text(stringResource(R.string.action_allow)) }
                if (r.allowSession) TextButton(onClick = { onAnswer(Decision.ALLOW_SESSION) }) { Text(stringResource(R.string.action_allow_session)) }
            }
        },
        dismissButton = { TextButton(onClick = { onAnswer(Decision.DENY) }) { Text(stringResource(R.string.action_deny)) } },
    )
}

@Composable
private fun EntryRow(e: JournalEntry) {
    Column(Modifier.fillMaxWidth()) {
        Text("${levelIcon(e.level)} ${e.description}")
        Text("${e.status} · adb ${e.argv.joinToString(" ")}", style = MaterialTheme.typography.bodySmall)
        e.inverse?.takeIf { e.undoneBy == null }?.let {
            Text("↩ adb ${it.argv.joinToString(" ")}", style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun levelIcon(level: String) = when (level) {
    "green" -> "🟢"
    "yellow" -> "🟡"
    "red" -> "🔴"
    else -> "·"
}
```

- [ ] **Paso 4: Ejecutar y ver que pasan**

```bash
cd android && gradle --no-daemon :app:testDebugUnitTest :app:assembleDebug :app:lintDebug
```

Esperado: `BUILD SUCCESSFUL` y todos los tests en verde.

- [ ] **Paso 5: Commit**

```bash
git add android
git commit -F - <<'EOF'
feat(android): TokenProvider, servicio en primer plano, confirmaciones con desbloqueo, consentimiento, panel y Cortar acceso

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

### Tarea 11: CI de la app, finales de línea y documentación

La CI compila y prueba la app en Linux, solo en `pull_request` y en push a `main` con cambios en `android/`. `.gitattributes` fija LF en los ficheros de la app. El README del host explica la app, y la lista de antes de publicar recoge la clave de firma y la release del APK.

**Ficheros:**
- Crear: `.github/workflows/android.yml`, `android/.gitignore`, `android/README.md`
- Modificar: `.gitattributes`, `host/README.md`, `docs/pre-public-checklist.md`

- [ ] **Paso 1: Crear la CI, el `.gitignore` y el README de la app**

`.github/workflows/android.yml`:

```yaml
name: android

# Like host.yml: only on pull requests and pushes to main, never on every
# branch push. Linux only (macOS minutes count 10 times).
on:
  pull_request:
    paths: ["android/**", ".github/workflows/android.yml"]
  push:
    branches: [main]
    paths: ["android/**", ".github/workflows/android.yml"]

permissions:
  contents: read

jobs:
  unit:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: android
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@v4
        with:
          gradle-version: "9.8.0"
      - name: Unit tests and lint
        run: gradle --no-daemon :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
```

`android/.gitignore`:

```
.gradle/
build/
local.properties
*.jks
*.keystore
.idea/
*.iml
```

`android/README.md`:

````markdown
# ADBuddy for Android

The phone side of ADBuddy (spec §6): it shows what the coding agent on your PC
does with your phone, asks you before anything irreversible, and lets you
pause the agent or cut its access at any time. Milestone M2: consent,
confirmations, activity panel, Pause and Cut access. The chat arrives in M3.

The app is installed and set up by the host (`adbuddy setup`), over the adb
connection you paired: you do not need to install it by hand.

## Build

Requirements: JDK 17, Android SDK platform 37, and Gradle 9.8 (there is no
Gradle wrapper in the repository yet).

```sh
cd android
gradle :app:testDebugUnitTest     # unit tests (protocol, state, handshake, WebSocket client)
gradle :app:assembleDebug         # app/build/outputs/apk/debug/app-debug.apk
adbuddy setup --apk app/build/outputs/apk/debug/app-debug.apk
```

Instrumented tests (`gradle :app:connectedDebugAndroidTest`) need an emulator:
never run them on your own phone.

## How it talks to the host

- The host writes a random 256-bit token, the port and the PC's name to the
  app's provider on stdin (`adb shell content write --uri
  content://tech.hybriq.adbuddy.bridge/handshake`). The provider only answers
  the shell user (uid 2000), and the host's classifier blocks the agent from
  using it.
- `adb reverse tcp:<port> tcp:<port>` makes `127.0.0.1:<port>` on the phone
  reach the host on the PC. The app connects to `ws://127.0.0.1:<port>/v1`
  and sends the token in its `hello`.
- Everything the host sends (titles, commands, journal entries) is data shown
  as text, never instructions.
````

- [ ] **Paso 2: Finales de línea y documentación**

`.gitattributes`. Cambio:

Antes:

```
.github/**/*.yml text eol=lf
```

Después:

```
.github/**/*.yml text eol=lf

# android/: LF (Gradle, Kotlin and Android resources)
android/**/*.kt text eol=lf
android/**/*.kts text eol=lf
android/**/*.xml text eol=lf
android/**/*.toml text eol=lf
android/**/*.properties text eol=lf
android/**/*.pro text eol=lf
android/**/*.md text eol=lf
android/.gitignore text eol=lf
```

`host/README.md`. Cambio:

Antes:

```markdown
- ADBuddy protects against mistakes and prompt injection, not against an agent determined to find another adb. The real kill switch arrives with the app ("Cut access", M2).
```

Después:

```markdown
- ADBuddy protects against mistakes and prompt injection, not against an agent determined to find another adb. The real kill switch is in the phone app: "Cut access" turns debugging off from the phone, without the PC.

## Phone app

`adbuddy setup` installs the ADBuddy app (the latest release, or `--apk <file>`) and asks for your consent in it. Only if you accept, it gives the app the permission for "Cut access", exempts it from battery optimisation and turns on its accessibility service, keeping any other one. From then on `adbuddy serve` connects to the app through `adb reverse` on `127.0.0.1:<chat_port>`: 🔴 confirmations go to the phone (unlocking is required to allow), the app shows the journal live, and you can pause the agent from the phone. When the app is not connected, confirmations happen in the terminal as before. The agent cannot use the app's channel: `content` and `run-as` on `tech.hybriq.adbuddy` always ask first.
```

`docs/pre-public-checklist.md`. Cambio:

Antes:

```markdown
- [ ] **Capturas reales** en `docs/setup/`
```

Después:

```markdown
- [ ] **Clave de firma del APK** de release, fuera del repositorio (secreto de GitHub Actions) y con copia de seguridad: perderla impide actualizar la app instalada.
- [ ] **Release del APK** con `adbuddy.apk` y `adbuddy.apk.sha256` (lo que descarga `adbuddy setup`).
- [ ] **Capturas reales** en `docs/setup/`
```

- [ ] **Paso 3: Comprobar que no hay datos personales ni finales CRLF**

```bash
git grep -nE "adb-[A-Za-z0-9]{6,}-|192\.168\.|(^|[^0-9])10\.[0-9]+\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\." -- host android docs/setup .github
git ls-files --eol android | grep -v "i/lf" || true
```

Esperado: el `git grep` no devuelve nada (los tests usan `192.0.2.x`), y todos los ficheros de `android/` se guardan con LF.

- [ ] **Paso 4: Commit**

```bash
git add .github/workflows/android.yml android/.gitignore android/README.md .gitattributes host/README.md docs/pre-public-checklist.md
git commit -F - <<'EOF'
chore: CI de la app, LF en android/ y documentación de M2

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

## Autorrevisión del plan frente a la spec

| Requisito (spec) | Tarea |
|---|---|
| §3.1.5: instalar la app con `adb install` (release con checksum o `--apk`); si ya está, se salta | 4, 6 |
| §3.1.6: consentimiento en la app (`requestSetup`), `pm grant WRITE_SECURE_SETTINGS` y accesibilidad sin quitar las otras | 4, 6, 9, 10 |
| §3.1.7: túnel `adb reverse` y entrega del token | 4, 5 |
| §4.5: `confirm.request` al móvil, notificación de prioridad alta, Permitir / Rechazar / Permitir durante la sesión (misma regla y argumentos), desbloqueo en Android 12+, diálogo con la app abierta, terminal de reserva, caducidad | 1, 2, 5, 10 |
| §4.6 Pausar: el daemon bloquea todo lo del shim | 2, 5, 9, 10 |
| §4.6 Cortar acceso: `adb_wifi_enabled=0` y `adb_enabled=0`; Reactivar, con el aviso del puerto remoto | 10 |
| §5.2: solo `127.0.0.1`, puerto fijo, token de 256 bits por arranque, `TokenProvider` solo con uid 2000, `hello`/`welcome`, reanudación con búfer de 2000 o `snapshot` | 1, 4, 5, 9, 10 |
| §5.3: sobre `{v, seq, type, ts, payload}`; los tipos de M2 (`session.state`, `confirm.request`/`reply`, `journal.entry`, `notice`, `pause`/`resume`) | 1, 8 |
| §6: servicio en primer plano `specialUse`, permisos, pantallas de consentimiento y actividad, textos EN/ES, minSdk 30 | 8-10 |
| §9 (fallos): diagnóstico guiado de configuración, adb, móvil, app y entorno, con `Run:` y `Guidance:` (idea de ARTEMIS) | 7 |
| §5.3: compatibilidad de versiones entre la app y el host (cierre 4001, estado INCOMPATIBLE) | 1, 8, 9, 10 |
| §10: el `TokenProvider` rechaza a quien no es shell (test instrumentado); una app sin token no entra (test del servidor); protocolo contra una app simulada (e2e); el token no aparece en líneas de comandos (e2e) | 1, 5, 10 |

Fuera de M2 (con su hito):
- chat, PTY y `attach` (M3);
- hooks de Claude Code (M4);
- lectura de pantalla, toques y superposición con la accesibilidad, y el modo remoto (M5);
- apps protegidas: la lista del onboarding y el bloqueo con app en primer plano necesitan el servicio de accesibilidad de M5;
- "Mantener la depuración activa" en Android 11-16 (M5);
- firma de release, IzzyOnDroid y releases del APK (M6).

## Validaciones manuales pendientes (necesitan al usuario y un móvil)

1. **Test instrumentado del `TokenProvider`** en un emulador (`gradle :app:connectedDebugAndroidTest`). **Automatizada en M2.5b:** `TokenProviderTest` corre en el emulador (Android 14, API 34, con `am instrument` desde `TestInstrumentedSuite`; `connectedDebugAndroidTest` no sirve: habla con el servidor adb 5037) y `host/internal/emutest` comprueba que el shell recibe respuesta; el job `emulator` de la CI los ejecuta.
2. **`content write` en Android 11-17.** La entrega del token por stdin depende de `adb shell content write --uri … < stdin`. Hay que comprobar que existe y que llega al `openFile` del proveedor en el Pixel (Android 17) y en un Android 11-13. Si no, la alternativa es la de ARTEMIS: un receptor de broadcast protegido con `android:permission="android.permission.WRITE_SECURE_SETTINGS"` (solo el usuario shell o el sistema pueden enviarlo). Pero ahí el token va en la línea de comandos de `am broadcast` un instante. Si se adopta, hay que rotar el token en cada conexión y decidirlo como ruling. **Automatizada en M2.5b para Android 14** (`TestTheHandshakeOnStdinReachesTheApp`, en el emulador android-34). **Sigue manual en Android 17 (el Pixel) y en Android 11-13:** la imagen android-37.0 del emulador reinicia el sistema en bucle (`surfaceflinger`), y otra imagen de sistema la decide el usuario (spec de M2.5, ruling de M2.5b).
3. **Arrancar el servicio en primer plano desde el proveedor**, con la app fuera de la optimización de batería y en segundo plano: Android 12+ lo restringe. Si falla, el host puede lanzar `am start-foreground-service -n tech.hybriq.adbuddy/.bridge.BridgeService` tras el handshake. **Automatizada en M2.5b para Android 14** (`TestTheServiceIsInTheForegroundAfterTheHandshake`, en el emulador android-34). **Sigue manual en Android 17 (el Pixel):** las restricciones de servicios en primer plano cambian en Android 15-17.
4. **Accesibilidad por adb en un Android 13-15 de otro fabricante** (criterio 4 de §1), con otro servicio ya activo, para comprobar que no se quita. **Sigue manual** (M2.5b): necesita un móvil de otro fabricante; y los ajustes restringidos de Android 13-17 no se activan en el emulador de Android 14.
5. **Confirmar desde la notificación con el móvil bloqueado:** debe pedir el desbloqueo antes de Permitir, y Rechazar no debe pedirlo. **Sigue manual** (M2.5b): necesita la pantalla de bloqueo y a una persona.
6. **Cortar acceso y Reactivar desde la app**, con y sin el modo `tcpip` (S1 lo probó solo con los interruptores de Ajustes). **Automatizada en M2.5b en local**, con y sin el modo `tcpip` (`TestCutAccessAndReactivate` lanza `CutAccessOnEmulatorTest`, que corta y reactiva con el código de la app, y comprueba desde el PC que adb se va y vuelve; Android 14). Solo corre con `ADBUDDY_EMULATOR_CUT=1` en un emulador de desarrollo; la CI no la ejecuta: tras un corte que nadie deshace el AVD arranca sin adb (`adb_enabled=0` queda en sus datos) y hay que borrar los datos del AVD. La prueba de `tcpip` es débil: en el emulador `adbd` escucha siempre en 5555 (es su transporte). **Sigue manual en el Pixel:** el corte con la depuración inalámbrica real (`adb_wifi_enabled`) y que el puerto `tcpip` quede cerrado.
7. **Prueba de extremo a extremo real:** `adbuddy setup --apk …` → `adbuddy serve` → un 🔴 desde el agente → Permitir en el móvil. **Sigue manual** (M2.5b, ruling del plan): automatizarla en el emulador exigiría preparar `config.toml` a mano (sin el emparejamiento por QR o código), simular los toques de consentimiento y Permitir (`uiautomator dump` e `input tap`) y un agente real con credenciales en la CI; cuesta mucho y demuestra poco más que lo ya cubierto. Las piezas sí están cubiertas en el emulador: instalación y verificación del APK, handshake, servicio en primer plano y Cortar acceso (este último en local).
8. **Permitir desde la notificación tras desbloquear desde el bouncer:** `ConfirmReceiver` exige `KeyguardManager.isDeviceLocked == false` (revisión de la Tarea 10). Comprobar que, al desbloquear desde la propia notificación, el móvil ya cuenta como desbloqueado cuando llega Permitir. Si no, Permitir desde la notificación no enviaría nada (desde el diálogo sí). **Sigue manual** (M2.5b): necesita el bouncer y a una persona.
9. **La notificación de confirmación no llega a relojes ni espejos** (`setLocalOnly`): comprobar con un reloj Wear o con Phone Link que no aparece. **Sigue manual** (M2.5b): necesita un reloj o Phone Link.
10. **Cambios del ajuste oculto `adb_wifi_enabled`:** comprobar que el `ContentObserver` de la app los recibe (Cortar acceso y Reactivar actualizan el estado aunque se cambien desde Ajustes). **Automatizada en M2.5b para Android 14** (`AdbWifiObserverTest`: un cambio desde el shell y otro con la identidad de permisos del shell llegan al observador de la app; además, la app puede leer el ajuste, que `CutAccess` guarda antes de cortar). **Sigue manual en Android 17:** que una app con targetSdk 37 pueda leer `adb_wifi_enabled`.
