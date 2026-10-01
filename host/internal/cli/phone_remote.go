package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// remoteServer is what remoteLink needs of the bridge server.
type remoteServer interface {
	Current(m bridge.Message) bool
}

// tlsPortTimeout bounds reading the wireless debugging port from the phone.
const tlsPortTimeout = 5 * time.Second

// remoteLink is the remote mode's part of the phone link (M5b): it keeps the
// Tailscale address and port the app reports in device.info. Without the app
// srv is nil and nothing happens.
type remoteLink struct {
	store *cfgStore
	adb   phoneapp.ADB
	log   io.Writer

	mu      sync.Mutex
	srv     remoteServer
	warned  map[string]bool
	pending *bridge.DeviceInfo // the latest report not yet saved
	running bool               // a worker is draining pending
}

func newRemoteLink(store *cfgStore, a phoneapp.ADB, log io.Writer) *remoteLink {
	return &remoteLink{store: store, adb: a, log: log, warned: map[string]bool{}}
}

func (r *remoteLink) setServer(srv remoteServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.srv = srv
}

// area is remoteLink's part of the phone link. device.info belongs to uiLink,
// which decodes it once and calls onDeviceInfo; remote.set joins in Task 5.
func (r *remoteLink) area() area { return area{} }

// onDeviceInfo keeps the Tailscale address the app reports, and the port of
// wireless debugging read from the phone (or the one the app reports). No
// address means no change (the message may carry only the protected apps).
func (r *remoteLink) onDeviceInfo(m bridge.Message, info bridge.DeviceInfo) {
	r.mu.Lock()
	srv := r.srv
	r.mu.Unlock()
	if srv == nil || !srv.Current(m) { // no app, or a connection already replaced
		return
	}
	if info.TailscaleIP == "" {
		return
	}
	if !config.ValidTailscaleIP(info.TailscaleIP) {
		r.mu.Lock()
		told := r.warned[info.TailscaleIP]
		r.warned[info.TailscaleIP] = true
		r.mu.Unlock()
		if !told {
			fmt.Fprintf(r.log, "⚠ The phone app reported %s as its Tailscale address, which is not in 100.64.0.0/10: ignored.\n", classify.Printable(info.TailscaleIP))
		}
		return
	}
	// Reading the port runs adb: off the app's message loop, one report at a
	// time and in order (a newer report replaces one still waiting).
	r.mu.Lock()
	r.pending = &info
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
		info := r.pending
		r.pending = nil
		if info == nil {
			r.running = false
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.save(*info)
	}
}

func (r *remoteLink) save(info bridge.DeviceInfo) {
	port := r.readTLSPort()
	if port == 0 && info.WifiADBPort >= 1 && info.WifiADBPort <= 65535 {
		port = info.WifiADBPort
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
	}
}

// readTLSPort asks the phone for its wireless debugging port; 0 when it
// does not say.
func (r *remoteLink) readTLSPort() int {
	ctx, cancel := context.WithTimeout(context.Background(), tlsPortTimeout)
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
