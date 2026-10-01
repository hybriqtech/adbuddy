package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// remoteServer is what remoteLink needs of the bridge server.
type remoteServer interface {
	Current(m bridge.Message) bool
	PublishRID(typ, rid string, payload any) error
	Publish(typ string, payload any) error
	Reply(req bridge.Message, typ string, payload any) error
}

// tlsPortTimeout bounds reading the wireless debugging port from the phone.
const tlsPortTimeout = 5 * time.Second

// remoteOpTimeout bounds one remote.set and each attempt to expire the mode
// (adb tcpip or usb, and the check of the port after a failure): a hung adb
// must not keep the controller for ever. A variable so a test can shorten it;
// read before a goroutine starts.
var remoteOpTimeout = 30 * time.Second

// maxRemoteQueue is how many remote.set wait behind the one running.
const maxRemoteQueue = 4

// maxWarnedIPs is how many wrong addresses are remembered to warn once about
// each: the app is the one that sends them, so the memory is bounded.
const maxWarnedIPs = 16

// remoteTick is how often serve looks at the remote mode's expiry (a variable
// so a test can shorten it; read before the goroutine starts).
var remoteTick = 30 * time.Second

// maxRemoteBackoff is the longest wait between two attempts to close a port
// that would not close.
const maxRemoteBackoff = 2 * time.Minute

// remoteLink is the remote mode's part of the phone link (M5b): it keeps the
// Tailscale address and port the app reports in device.info, and handles
// remote.set. Without the app srv is nil and nothing happens.
type remoteLink struct {
	ctx   context.Context // serve's: ends every adb call
	store *cfgStore
	ctl   *remote.Controller // nil only in tests: remote.set is then unavailable
	adb   phoneapp.ADB
	log   io.Writer

	// state is the mode as of the last change. The snapshot reads only this:
	// it runs with the server's lock held and must not wait for the store.
	state     atomic.Pointer[remote.State]
	refreshMu sync.Mutex // the pointer never goes back to an older state

	mu      sync.Mutex
	offs    int         // how many changes to off were queued (under mu)
	queue   []remoteJob // remote.set waiting, in order
	working bool        // a worker is draining queue
	srv     remoteServer
	warned  map[string]bool
	pending *pendingInfo // the latest report not yet saved
	running bool         // a worker is draining pending
}

type pendingInfo struct {
	m    bridge.Message
	info bridge.DeviceInfo
}

func newRemoteLink(ctx context.Context, store *cfgStore, ctl *remote.Controller, a phoneapp.ADB, log io.Writer) *remoteLink {
	r := &remoteLink{ctx: ctx, store: store, ctl: ctl, adb: a, log: log, warned: map[string]bool{}}
	r.refresh()
	return r
}

func (r *remoteLink) setServer(srv remoteServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.srv = srv
}

func (r *remoteLink) server() remoteServer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.srv
}

// refresh reads the mode from the controller and keeps it for the snapshot.
// Call it after anything that may have changed the mode, never with the
// server's lock held.
func (r *remoteLink) refresh() remote.State {
	if r.ctl == nil {
		return remote.State{}
	}
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	s := r.ctl.State()
	r.state.Store(&s)
	return s
}

// toBridge is the state as the app sees it. Closing is "on": the port is
// open until `adb usb` has closed it.
func toBridge(s remote.State) bridge.RemoteState {
	if !s.Enabled {
		return bridge.RemoteState{}
	}
	return bridge.RemoteState{Enabled: true, Address: s.Address, ExpiresAt: s.ExpiresAt}
}

// publish tells the app the mode (an event; an answer to remote.set goes with
// its rid instead).
func (r *remoteLink) publish(s remote.State) {
	if srv := r.server(); srv != nil {
		_ = srv.Publish(bridge.TypeRemoteState, toBridge(s))
	}
}

// area is remoteLink's part of the phone link. device.info belongs to uiLink,
// which decodes it once and calls onDeviceInfo.
func (r *remoteLink) area() area {
	return area{
		handlers: map[string]func(bridge.Message){bridge.TypeRemoteSet: r.handleSet},
		snapshot: func(s *bridge.Snapshot) {
			if p := r.state.Load(); p != nil {
				s.Remote = toBridge(*p)
			}
		},
		// attach runs with the server's lock held: publishing waits for it.
		attach: func() {
			if r.ctl != nil {
				go func() { r.publish(r.refresh()) }()
			}
		},
	}
}

// fail answers a remote.set with an error.
func (r *remoteLink) fail(srv remoteServer, m bridge.Message, code, text string) {
	_ = srv.Reply(m, bridge.TypeError, bridge.ErrorReply{Code: code, Text: text})
}

// handleSet turns the remote mode on or off at the app's request. It comes
// from the current authenticated connection only (the bridge's token), and
// every request is answered: with remote.state carrying its rid, or an error.
func (r *remoteLink) handleSet(m bridge.Message) {
	srv := r.server()
	if srv == nil || !srv.Current(m) { // no app, or a connection already replaced
		return
	}
	if m.RID == "" {
		// A request by protocol: with no rid there is nobody to answer.
		fmt.Fprintln(r.log, "⚠ The phone app sent remote.set without a request id: ignored.")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(m.Payload, &req); err != nil || req.Enabled == nil {
		r.fail(srv, m, bridge.CodeInvalid, "remote.set needs {\"enabled\": true|false}.")
		return
	}
	if r.ctl == nil {
		r.fail(srv, m, bridge.CodeUnavailable, "Remote mode is not available on this host.")
		return
	}
	// One worker, in order: a later "off" never runs before an earlier "on".
	if !r.enqueue(remoteJob{srv: srv, m: m, on: *req.Enabled, timeout: remoteOpTimeout}) {
		r.fail(srv, m, bridge.CodeUnavailable, "Too many remote mode requests are waiting: try again in a moment.")
	}
}

// enqueue puts a change behind the ones waiting and starts the worker if it
// is idle; false when the queue is full. Every change of the mode (the app's
// and `adbuddy remote`) goes through here: the last one asked is the last one
// that runs.
func (r *remoteLink) enqueue(job remoteJob) bool {
	r.mu.Lock()
	if len(r.queue) >= maxRemoteQueue {
		r.mu.Unlock()
		return false
	}
	r.queue = append(r.queue, job)
	if !job.on {
		r.offs++
	}
	start := !r.working
	r.working = true
	r.mu.Unlock()
	// adb takes seconds (adbd restarts): never on the app's message loop.
	if start {
		go r.work()
	}
	return true
}

// offCount is how many times the mode was asked to turn off: a yes to an
// older `on` is told from a current one by it not having changed.
func (r *remoteLink) offCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.offs
}

// remoteJob is one change of the mode: a remote.set (srv and m answer it) or
// an `adbuddy remote` (reply gets the result, srv is nil).
type remoteJob struct {
	srv     remoteServer
	m       bridge.Message
	on      bool
	timeout time.Duration
	reply   chan<- remoteResult // buffered: the worker never waits for it
}

// remoteResult is what a change left: the state it returned and its error.
type remoteResult struct {
	state remote.State
	err   error
}

func (r *remoteLink) work() {
	for {
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.working = false
			r.mu.Unlock()
			return
		}
		job := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()
		r.run(job)
	}
}

func (r *remoteLink) run(j remoteJob) {
	ctx, cancel := context.WithTimeout(r.ctx, j.timeout)
	defer cancel()
	var (
		st  remote.State
		err error
	)
	if j.on {
		st, err = r.ctl.Enable(ctx)
	} else {
		st, err = r.ctl.Disable(ctx)
	}
	now := r.refresh()
	if j.reply != nil {
		j.reply <- remoteResult{state: st, err: err}
		// The app is told what the host has, as after a remote.set.
		if !errors.Is(err, remote.ErrNoTailscale) {
			r.publish(now)
		}
		return
	}
	switch {
	case errors.Is(err, remote.ErrNoTailscale):
		r.fail(j.srv, j.m, bridge.CodeUnavailable, err.Error())
	case err != nil:
		r.fail(j.srv, j.m, bridge.CodeFailed, classify.Printable(err.Error()))
		// The attempt may have changed the phone (adbd got the order and the
		// answer was lost): the app is told what the host believes, so it does
		// not show "off" while the port may be open.
		r.publish(now)
	default:
		// remote.state is an event: the answer and the publication are one.
		_ = j.srv.PublishRID(bridge.TypeRemoteState, j.m.RID, toBridge(st))
	}
}

// watchRemote closes the remote mode when it expires, every `every`, with
// opTimeout for each attempt. A port that will not close is said once and
// tried again with growing pauses (at most maxRemoteBackoff): each attempt is
// an `adb usb`, which restarts adbd and the phone's connection.
func watchRemote(ctx context.Context, log io.Writer, ctl *remote.Controller, link *remoteLink, every, opTimeout time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	maxSkip := max(1, int(maxRemoteBackoff/every))
	warned := false
	wait, skip := 0, 0 // ticks to let pass before trying again
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if skip > 0 {
			skip--
			continue
		}
		actx, cancel := context.WithTimeout(ctx, opTimeout)
		s, changed, err := ctl.Expire(actx)
		cancel()
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			link.refresh() // Closing: the snapshot says on
			if !warned {
				warned = true
				fmt.Fprintf(log, "⚠ Remote mode expired but port 5555 could not be closed: %s; trying again.\n", classify.Printable(err.Error()))
			}
			wait = min(max(1, wait*2), maxSkip)
			skip = wait
		default:
			warned, wait, skip = false, 0, 0
			if changed {
				fmt.Fprintln(log, "Remote mode expired: port 5555 closed.")
				link.refresh()
				link.publish(s)
			}
		}
	}
}

// remoteOnNotice is what serve says at start when the remote mode is on.
func remoteOnNotice(s remote.State) string {
	if s.Closing {
		return "Remote mode expired but port 5555 could not be closed yet: serve keeps trying."
	}
	return "Remote mode is on until " + s.ExpiresAt.Local().Format("15:04") + " (port 5555 open)."
}

// stillOnNotice is what serve says when it stops with the remote mode on.
func stillOnNotice(s remote.State) string {
	const rest = "Nothing closes it while serve is stopped: run `adbuddy remote off` (it closes the port directly) or start serve again."
	if s.Closing {
		return "Remote mode expired but port 5555 could not be closed yet. " + rest
	}
	return "Remote mode is still on (port 5555 stays open until " + s.ExpiresAt.Local().Format("15:04") + "). " + rest
}

// onDeviceInfo keeps the Tailscale address the app reports, and the port of
// wireless debugging read from the phone (or the one the app reports). No
// address means no change (the message may carry only the protected apps).
func (r *remoteLink) onDeviceInfo(m bridge.Message, info bridge.DeviceInfo) {
	srv := r.server()
	if srv == nil || !srv.Current(m) { // no app, or a connection already replaced
		return
	}
	if info.TailscaleIP == "" {
		return
	}
	if !config.ValidTailscaleIP(info.TailscaleIP) {
		r.mu.Lock()
		told := r.warned[info.TailscaleIP] || len(r.warned) >= maxWarnedIPs
		if !told {
			r.warned[info.TailscaleIP] = true
		}
		r.mu.Unlock()
		if !told {
			fmt.Fprintf(r.log, "⚠ The phone app reported %s as its Tailscale address, which is not in 100.64.0.0/10: ignored.\n", classify.Printable(info.TailscaleIP))
		}
		return
	}
	// Reading the port runs adb: off the app's message loop, one report at a
	// time and in order (a newer report replaces one still waiting).
	r.mu.Lock()
	r.pending = &pendingInfo{m: m, info: info}
	start := !r.running
	r.running = true
	r.mu.Unlock()
	if start {
		go r.drain()
	}
}

func (r *remoteLink) drain() {
	for {
		r.mu.Lock()
		p := r.pending
		r.pending = nil
		if p == nil {
			r.running = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.save(p.m, p.info)
	}
}

func (r *remoteLink) save(m bridge.Message, info bridge.DeviceInfo) {
	port := r.readTLSPort()
	if port == 0 && info.WifiADBPort >= 1 && info.WifiADBPort <= 65535 {
		port = info.WifiADBPort
	}
	// The connection may have been replaced while adb answered.
	if srv := r.server(); srv == nil || !srv.Current(m) {
		return
	}
	cur := r.store.Config()
	if cur.TailscaleIP == info.TailscaleIP && (port == 0 || cur.TailscalePort == port) {
		return
	}
	err := r.store.Update(func(c *config.Config) {
		c.TailscaleIP = info.TailscaleIP
		if port > 0 {
			c.TailscalePort = port
		}
	})
	if err != nil {
		fmt.Fprintf(r.log, "⚠ Could not save the phone's Tailscale address (%s).\n", classify.Printable(err.Error()))
		return
	}
	// The address is part of the state while the mode is on.
	if before := r.state.Load(); before != nil && before.Enabled {
		if after := r.refresh(); after.Address != before.Address {
			r.publish(after)
		}
	}
}

// readTLSPort asks the phone for its wireless debugging port; 0 when it
// does not say.
func (r *remoteLink) readTLSPort() int {
	ctx, cancel := context.WithTimeout(r.ctx, tlsPortTimeout)
	defer cancel()
	var out, errb bytes.Buffer
	code, err := r.adb.Run(ctx, "", []string{"shell", "getprop service.adb.tls.port"}, nil, &out, &errb)
	if err != nil || code != 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
}
