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
