package cli

import (
	"context"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// How the watcher of the phone's connection waits (variables so tests can
// shorten them; serve reads them before starting the goroutine).
var (
	watchEvery   = 10 * time.Second // between two looks at `adb devices`
	watchMisses  = 2                // looks in a row without the phone before trying other addresses
	watchTryWait = 8 * time.Second  // bounds each look at `adb devices` and each `adb connect` of a candidate
)

// candidateAddrs are the addresses worth trying to reach the phone at, in
// order: the one that worked last time, the phone's Tailscale address with its
// wireless debugging port, and, while the remote mode is on, its Tailscale IP
// at port 5555 (the one the remote mode opens). Without duplicates. The
// address of the home network is not a candidate: its IP changes.
func candidateAddrs(cfg config.Config, remoteOn bool) []string {
	var out []string
	add := func(a string) {
		if a == "" || slices.Contains(out, a) {
			return
		}
		out = append(out, a)
	}
	add(cfg.Address)
	if cfg.TailscaleIP != "" && cfg.TailscalePort != 0 {
		add(net.JoinHostPort(cfg.TailscaleIP, strconv.Itoa(cfg.TailscalePort)))
	}
	if remoteOn && cfg.TailscaleIP != "" {
		add(net.JoinHostPort(cfg.TailscaleIP, remote.Port))
	}
	return out
}

// phoneWatch is what watchPhone needs.
type phoneWatch struct {
	Active   *activePhone
	Runner   adb.Runner // for host commands (devices, connect, disconnect); its Serial is ignored
	Serial   string     // the phone's saved name (config.toml serial) this session was started for
	Cfg      func() config.Config
	RemoteOn func() bool // the remote mode is on (even if it expired and is closing)
	Log      io.Writer

	// Zero values take the watch* variables when watchPhone starts: a caller
	// that starts the goroutine passes them read beforehand, so that a test
	// changing the variables does not race with the goroutine.
	Every   time.Duration
	Misses  int
	TryWait time.Duration
}

// watchPhone keeps the connection to the phone: when it disappears from
// `adb devices` for Misses looks in a row, it looks for it again (under
// another name, or at the candidate addresses) and switches the active phone
// to it, but ONLY once its identity has been verified (ro.serialno against
// its guid). It never opens or closes a port (no applyInvariant): that is the
// remote mode's job. It ends with ctx.
func watchPhone(ctx context.Context, w phoneWatch) {
	wt := newWatcher(w)
	t := time.NewTicker(wt.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			wt.step(ctx)
		}
	}
}

// watcher is the state of watchPhone between two looks.
type watcher struct {
	phoneWatch
	every    time.Duration
	misses   int
	tryWait  time.Duration
	guid     string
	canCheck bool            // the saved name carries a serial number that tells phones apart
	count    int             // looks in a row without the phone
	warned   bool            // "not reachable" was said and the phone has not come back
	wrong    map[string]bool // addresses said to be another phone since the phone was last listed
}

func newWatcher(w phoneWatch) *watcher {
	wt := &watcher{phoneWatch: w, every: w.Every, misses: w.Misses, tryWait: w.TryWait, wrong: map[string]bool{}}
	if wt.every <= 0 {
		wt.every = watchEvery
	}
	if wt.misses <= 0 {
		wt.misses = watchMisses
	}
	if wt.tryWait <= 0 {
		wt.tryWait = watchTryWait
	}
	wt.guid, _ = pairing.GuidOf(w.Serial)
	_, wt.canCheck = pairing.GuidSerialNo(wt.guid)
	return wt
}

func (w *watcher) hostRunner() adb.Runner {
	r := w.Runner
	r.Serial = ""
	return r
}

// step is one look at the connection. A hung `adb devices` is cut at
// tryWait: otherwise the watcher would stop for good inside one step.
func (w *watcher) step(ctx context.Context) {
	octx, cancel := context.WithTimeout(ctx, w.tryWait)
	online, err := w.hostRunner().Online(octx)
	cancel()
	if err != nil || ctx.Err() != nil {
		return // adb itself failing says nothing about the phone
	}
	if hasSerial(online, w.Active.Serial()) {
		if w.warned {
			tell(w.Log, "phone_watch.the_phone_is_reachable_again")
		}
		w.count, w.warned = 0, false
		clear(w.wrong)
		return
	}
	w.count++
	if w.count < w.misses {
		return
	}
	// Nothing below changes the active phone unless that phone was verified
	// to be the one in config.toml: a guid can be announced by anything on the
	// network and an address can reach another phone.
	if !w.canCheck {
		w.warnOnce(i18n.M("phone_watch.not_listed", unverifiedWhy(w.Serial), w.Active.Serial()))
		return
	}
	// mDNS brought it back under another name.
	for _, d := range online {
		if !pairing.IsGuidSerial(w.guid, d.Serial) {
			continue
		}
		verified, err := verifyAt(ctx, w.Runner, d.Serial, w.guid)
		if ctx.Err() != nil {
			return
		}
		if err == nil && verified {
			w.switchTo(d.Serial, d.Serial)
			return
		}
		w.skip(d.Serial, err)
	}
	var tried []string
	if w.Cfg().Serial == w.Serial { // the saved addresses are those of that phone
		tried = candidateAddrs(w.Cfg(), w.RemoteOn())
	}
	for _, addr := range tried {
		active, verified, err := connectAddr(ctx, w.Runner, addr, w.guid, w.tryWait)
		if ctx.Err() != nil {
			return
		}
		if err == nil && verified {
			w.switchTo(active, addr)
			return
		}
		switch {
		case hasCode(err, "WRONG_PHONE"):
			w.skip(addr, err) // verifyAt already disconnected it
		case err == nil:
			w.disconnect(ctx, addr) // connected but not verified: not used
		case !hasCode(err, "CONNECT_FAILED"):
			w.disconnect(ctx, addr) // connected, but its identity could not be read
		}
	}
	if len(tried) == 0 {
		w.warnOnce(i18n.M("phone_watch.not_reachable_none"))
	} else {
		w.warnOnce(i18n.M("phone_watch.not_reachable", strings.Join(tried, ", ")))
	}
}

// switchTo makes serial the active phone. Only called with a verified one.
func (w *watcher) switchTo(serial, where string) {
	w.Active.Switch(serial)
	w.count, w.warned = 0, false
	clear(w.wrong)
	tell(w.Log, "phone_watch.the_phone_is_now_at", where)
}

func (w *watcher) warnOnce(msg i18n.Msg) {
	if w.warned {
		return
	}
	w.warned = true
	tell(w.Log, "phone_watch.warn", msg)
}

// skip says once per address that it is another phone.
func (w *watcher) skip(addr string, err error) {
	if !hasCode(err, "WRONG_PHONE") || w.wrong[addr] {
		return
	}
	w.wrong[addr] = true
	tell(w.Log, "connect.warning_the_phone_at", classify.Printable(addr), wrongPhoneWarningText)
}

func (w *watcher) disconnect(ctx context.Context, addr string) {
	dctx, cancel := context.WithTimeout(ctx, disconnectWait)
	defer cancel()
	_, _, _, _ = w.hostRunner().Output(dctx, "disconnect", addr)
}
