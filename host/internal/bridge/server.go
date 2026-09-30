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

// helloTimeout is how long a new connection has to send its hello. A var so a
// test can shorten it; New copies it into the Server.
var helloTimeout = 5 * time.Second

// beforeDispatch runs in the reader for each message about to reach
// OnMessage. A var so a test can hold a message there; New copies it.
var beforeDispatch = func(Message) {}

const (
	helloLimit = 4 << 10 // read limit until the token is checked
	maxMessage = 1 << 20 // text messages; attachments (binary frames) arrive in M3
	sendQueue  = 256
	pingEvery  = 20 * time.Second
)

// Options configure a Server.
type Options struct {
	Token       string
	Session     string
	HostVersion string
	// OnMessage receives the app's messages after a valid hello, one at a
	// time. Its payload comes from the app: validate before use. Its TS is
	// what the app sent, or zero when that was not a valid time.
	OnMessage func(Message)
	// Snapshot describes the current state when the app missed too much, and
	// for PublishSnapshot. It is called with the server's lock held: it must
	// not call Publish, PublishSnapshot or Connected.
	Snapshot func() Snapshot
	// OnConnect is called with true when an app authenticates and none was
	// connected, and with false when the app is gone and no other took its
	// place: replacing a connection calls neither. Calls come one at a time
	// and in order, and may call Publish and Connected.
	OnConnect func(bool)
	// OnReject is called when an authenticated app is too old to talk to.
	OnReject func(Hello)
	Now      func() time.Time
}

// Server is the WebSocket server of the app channel. It serves one app at a
// time: a new authenticated connection replaces the previous one.
type Server struct {
	o              Options
	helloTimeout   time.Duration
	beforeDispatch func(Message)

	// connMu orders attach and detach together with their OnConnect calls,
	// which run outside mu.
	connMu    sync.Mutex
	announced bool // the last OnConnect call was true; guarded by connMu

	mu     sync.Mutex
	token  string // the only token accepted now (SetToken)
	seq    uint64
	ring   []Message // the last BufferSize events, oldest first
	client *client
}

type client struct {
	send   chan Message
	cancel context.CancelFunc
	token  string // the token it authenticated with
	conn   *websocket.Conn
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
	return &Server{o: o, helloTimeout: helloTimeout, beforeDispatch: beforeDispatch, token: o.Token}
}

// SetToken makes tok the only token accepted from now on, and closes the
// app connected with another one: a token handed over before is worth
// nothing once a new one is (spec §5.2: a new token on each start or
// connection). From then on the closed app's messages reach OnMessage no more,
// even while its close is under way.
func (s *Server) SetToken(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = tok
	if s.client != nil && s.client.token != tok {
		// Closed with a policy violation, like a wrong token; detach then
		// announces the app gone. Close waits for the app's answer: not
		// under the lock.
		go s.client.conn.Close(websocket.StatusPolicyViolation, "token changed")
		s.client = nil
	}
}

// validToken reports whether tok is the current token, in constant time.
func (s *Server) validToken(tok string) bool {
	s.mu.Lock()
	cur := s.token
	s.mu.Unlock()
	return cur != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(cur)) == 1
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

// current reports whether cl is still the connected app.
func (s *Server) current(cl *client) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client == cl
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
	s.sendLocked(s.keepLocked(typ, raw))
	return nil
}

// PublishSnapshot sends the current state (Options.Snapshot) as an event,
// like Publish: the app replaces what it shows with it, now or when it
// resumes. The snapshot is taken under the server's lock, so no event
// published meanwhile falls between what it shows and its number.
func (s *Server) PublishSnapshot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(s.o.Snapshot())
	s.sendLocked(s.keepLocked(TypeSnapshot, raw))
}

// keepLocked numbers an event and keeps it to resend. Call it with mu held.
func (s *Server) keepLocked(typ string, raw json.RawMessage) Message {
	s.seq++
	m := Message{V: ProtocolVersion, Seq: s.seq, Type: typ, TS: s.o.Now().UTC(), Payload: raw}
	s.ring = append(s.ring, m)
	if len(s.ring) > BufferSize {
		s.ring = s.ring[len(s.ring)-BufferSize:]
	}
	return m
}

// sendLocked queues m for the connected app, if any. Call it with mu held.
func (s *Server) sendLocked(m Message) {
	if s.client != nil {
		select {
		case s.client.send <- m:
		default: // too slow: drop it; it reconnects and resumes from lastSeq
			s.client.cancel()
			s.client = nil
		}
	}
}

// localHost reports whether the request's Host is how the app reaches the
// server: 127.0.0.1 or localhost, with the port it listens on. Any other name
// is a page whose name was rebound to 127.0.0.1 (DNS rebinding).
func localHost(r *http.Request) bool {
	la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	_, port, err := net.SplitHostPort(la.String())
	if err != nil {
		return false
	}
	return r.Host == "127.0.0.1:"+port || r.Host == "localhost:"+port
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !localHost(r) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	// No Origin exceptions: a web page on the phone cannot open this socket
	// (coder/websocket rejects a foreign Origin), and without the token no
	// other app gets past the hello.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(helloLimit)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	hello, err := s.readHello(ctx, c)
	if err != nil {
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	if !s.validToken(hello.Token) {
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	c.SetReadLimit(maxMessage)

	if hello.Protocol < MinAppProtocol {
		// Only after the token: nobody learns anything about the host without it.
		c.Close(websocket.StatusCode(CloseIncompatible), "update the ADBuddy app")
		s.o.OnReject(hello)
		return
	}

	cl := &client{send: make(chan Message, sendQueue), cancel: cancel, token: hello.Token, conn: c}
	backlog, first, ok := s.attach(cl, hello)
	if !ok { // the token changed since it was checked
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	defer s.detach(cl)

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
			typ, data, err := c.Read(ctx)
			if err != nil {
				return // the connection is gone
			}
			if typ != websocket.MessageText {
				continue // binary frames (attachments) arrive in M3
			}
			m, err := decode(data)
			if err != nil {
				continue // a malformed message does not cut the channel
			}
			if m.V != ProtocolVersion || m.Type == "" || m.Type == TypeHello {
				continue
			}
			s.beforeDispatch(m)
			if !s.current(cl) {
				// Revoked by SetToken, replaced or dropped: its close may
				// take a while, but nothing it sends counts any more. Keep
				// reading until that close ends the connection, so it is
				// not closed first as going away.
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

// attach makes cl the connected app and returns what it missed (see swap).
// It calls OnConnect(true) unless an app was already announced. It fails
// (ok false) when cl's token is no longer the current one.
func (s *Server) attach(cl *client, h Hello) (backlog []Message, first uint64, ok bool) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	backlog, first, ok = s.swap(cl, h)
	if !ok {
		return nil, 0, false
	}
	if !s.announced {
		s.announced = true
		s.o.OnConnect(true)
	}
	return backlog, first, true
}

// swap replaces the connected app with cl and returns the events after the
// hello's lastSeq, or a snapshot when they are no longer kept or when the
// app's lastSeq belongs to another host: a restarted host has another session
// and numbers its events from 1 again.
func (s *Server) swap(cl *client, h Hello) (backlog []Message, first uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if subtle.ConstantTimeCompare([]byte(cl.token), []byte(s.token)) != 1 {
		return nil, 0, false
	}
	if s.client != nil {
		s.client.cancel()
	}
	s.client = cl
	oldest := s.seq + 1
	if len(s.ring) > 0 {
		oldest = s.ring[0].Seq
	}
	restarted := h.LastSeq > 0 && h.Session != s.o.Session
	if restarted || h.LastSeq > s.seq || h.LastSeq+1 < oldest {
		snap, _ := json.Marshal(s.o.Snapshot())
		// The snapshot takes the next number, so the app's lastSeq moves past
		// everything it replaces.
		m := s.keepLocked(TypeSnapshot, snap)
		return []Message{m}, m.Seq, true
	}
	for _, m := range s.ring {
		if m.Seq > h.LastSeq {
			backlog = append(backlog, m)
		}
	}
	return backlog, h.LastSeq + 1, true
}

// detach removes cl if it is still the connected app and reports whether it
// did. It calls OnConnect(false) when that leaves no app, and also when cl
// was already dropped for being slow (Publish) and nobody took its place.
func (s *Server) detach(cl *client) bool {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.mu.Lock()
	removed := s.client == cl
	if removed {
		s.client = nil
	}
	gone := s.client == nil
	s.mu.Unlock()
	if gone && s.announced {
		s.announced = false
		s.o.OnConnect(false)
	}
	return removed
}

// readHello reads the first message. If it does not arrive in time the
// connection is closed with a policy violation, like a wrong hello: a
// deadline on the Read itself would drop it without a close code.
func (s *Server) readHello(ctx context.Context, c *websocket.Conn) (Hello, error) {
	t := time.AfterFunc(s.helloTimeout, func() { c.Close(websocket.StatusPolicyViolation, "no hello") })
	defer t.Stop()
	var data []byte
	for data == nil {
		typ, d, err := c.Read(ctx)
		if err != nil {
			return Hello{}, err
		}
		if typ == websocket.MessageText {
			data = d
		}
	}
	m, err := decode(data)
	if err != nil {
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

// decode reads a message from the app. Its ts is not checked: an invalid one
// leaves TS zero instead of losing the message.
func decode(data []byte) (Message, error) {
	var in struct {
		V       int             `json:"v"`
		Type    string          `json:"type"`
		TS      json.RawMessage `json:"ts"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return Message{}, err
	}
	m := Message{V: in.V, Type: in.Type, Payload: in.Payload}
	_ = json.Unmarshal(in.TS, &m.TS)
	return m, nil
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
