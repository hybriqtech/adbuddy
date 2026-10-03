package emutest

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// What the tests of the terminal need to know about the emulator's network:
// whether it is off for real, what the app's uid has moved outside the
// loopback, and a place on the PC to send traffic to. T1
// (.superpowers/sdd/2026-10-03-test-link-files-terminal/t1-mediciones.md §7)
// measured the formats read here; each reader fails loudly when a format is
// not the one measured, so that a test never passes on an empty reading.

// hostFromEmulator is the PC as the emulator sees it (its alias of the host's
// loopback).
const hostFromEmulator = "10.0.2." + "2"

// netstatsTolerance is the bytes the app's uid may move between two readings
// of netstats (each way) and still count as "nothing": the WebView's first
// start moves ~7 KB rx and ~2 KB tx towards Google on its own (T1 §7,
// cr_VariationsUtils), so what the tolerance must tell apart is that, not
// zero; T1 measured exactly 0 B in 20-30 s of terminal activity.
const netstatsTolerance = 1024

// uidBytes is what the history of netstats holds for one uid.
type uidBytes struct {
	Rx, Tx int64 // sum of rb and tb of every row of every block of the uid
	Blocks int   // blocks "ident=[...] uid=<uid> ..." found
	Rows   int   // history rows ("st=... rb=... tb=...") read
}

var (
	identUIDRe = regexp.MustCompile(`\buid=(\d+)\b`)
	statRowRe  = regexp.MustCompile(`(^|\s)st=\d+`)
	rbRe       = regexp.MustCompile(`\brb=(\d+)`)
	tbRe       = regexp.MustCompile(`\btb=(\d+)`)
)

// parseNetstatsUID adds up rb and tb of the history of uid in the output of
// `dumpsys netstats --full --uid`, as T1's nsuid.sh did: a line with "ident="
// and "uid=<uid>" starts the uid's block, every row "st=... rb=... tb=..."
// until the next "ident=" line counts. It does NOT read the "BPF map content"
// table (mUidStatsMap): that one counts the loopback, and the channel of the
// app goes through it (100 MiB sent = 113 MB there, 1.95 KB in the history).
//
// The dump of a phone always has other uids' blocks (the system's): none at
// all means the format is not the one measured and is an error, and so is a
// row with no rb or no tb. A uid with no block just has no traffic yet (the
// history was empty before the chat first opened): Blocks says so, and the
// callers decide whether that is acceptable. A uid with blocks but no readable
// row is an error: it would read as 0 bytes for ever.
func parseNetstatsUID(dump string, uid int) (uidBytes, error) {
	var res uidBytes
	anyBlock := false
	in := false
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.Contains(line, "ident=") {
			in = false
			if m := identUIDRe.FindStringSubmatch(line); m != nil {
				anyBlock = true
				if n, _ := strconv.Atoi(m[1]); n == uid {
					in = true
					res.Blocks++
				}
			}
			continue
		}
		if !in || !statRowRe.MatchString(line) {
			continue
		}
		rb, tb := rbRe.FindStringSubmatch(line), tbRe.FindStringSubmatch(line)
		if rb == nil || tb == nil {
			return res, fmt.Errorf("a history row of uid %d has no rb or tb: %q", uid, line)
		}
		r, _ := strconv.ParseInt(rb[1], 10, 64)
		w, _ := strconv.ParseInt(tb[1], 10, 64)
		res.Rx += r
		res.Tx += w
		res.Rows++
	}
	if !anyBlock {
		return res, fmt.Errorf("no block \"ident=[...] uid=N\" in the dump of netstats (format changed?): %d bytes", len(dump))
	}
	if res.Blocks > 0 && res.Rows == 0 {
		// A block with no row we can read would give 0 bytes and every delta
		// would pass on it: the format is not the measured one.
		return res, fmt.Errorf("uid %d has %d blocks in the dump of netstats but no history row \"st=... rb=... tb=...\" (format changed?)", uid, res.Blocks)
	}
	return res, nil
}

const netstatsFixture = `Active interfaces:
Dev stats:
  Pending bytes: 0
  History since boot:
Uid stats:
  Pending bytes: 0
  History since boot:
  ident=[{type=WIFI, subType=COMBINED, networkId=WiFi}] uid=1000 set=DEFAULT tag=0x0 metered=false roaming=false defaultNetwork=true
    NetworkStatsHistory: bucketDuration=7200
      st=1759500000 rb=500 rp=3 tb=100 tp=2 op=0
  ident=[{type=WIFI, subType=COMBINED, networkId=WiFi}] uid=10193 set=DEFAULT tag=0x0 metered=false roaming=false defaultNetwork=true
    NetworkStatsHistory: bucketDuration=7200
      st=1759500000 rb=6766 rp=9 tb=1814 tp=8 op=0
      st=1759507200 rb=234 rp=1 tb=186 tp=1 op=0
  ident=[{type=WIFI, subType=COMBINED, networkId=WiFi}] uid=101930 set=DEFAULT tag=0x0 metered=false roaming=false defaultNetwork=true
    NetworkStatsHistory: bucketDuration=7200
      st=1759500000 rb=99999 rp=9 tb=99999 tp=8 op=0
  ident=[{type=MOBILE, subType=COMBINED}] uid=10193 set=FOREGROUND tag=0x0 metered=true roaming=false defaultNetwork=true
    NetworkStatsHistory: bucketDuration=7200
      st=1759500000 rb=1 rp=1 tb=2 tp=1 op=0
`

func TestParseNetstatsUID(t *testing.T) {
	got, err := parseNetstatsUID(netstatsFixture, 10193)
	if err != nil {
		t.Fatal(err)
	}
	// 6766+234+1 and 1814+186+2: the other uids' rows (1000 and 101930) are not the app's.
	if want := (uidBytes{Rx: 7001, Tx: 2002, Blocks: 2, Rows: 3}); got != want {
		t.Errorf("parseNetstatsUID = %+v, want %+v", got, want)
	}
	if none, err := parseNetstatsUID(netstatsFixture, 10777); err != nil || none != (uidBytes{}) {
		t.Errorf("a uid without traffic = %+v, %v; want zeros and no error", none, err)
	}
	if _, err := parseNetstatsUID("nothing of netstats here\n", 10193); err == nil {
		t.Error("a dump with no ident block was accepted")
	}
	if _, err := parseNetstatsUID("  ident=[x] uid=10193 set=DEFAULT\n      st=1 rp=1 tp=1\n", 10193); err == nil {
		t.Error("a history row without rb/tb was accepted")
	}
	// Blocks of the uid with no row in the measured format: not "0 bytes".
	other := "  ident=[x] uid=10193 set=DEFAULT\n    NetworkStatsHistory: bucketDuration=7200\n      1759500000: rx=5 tx=6\n  ident=[y] uid=1000 set=DEFAULT\n      st=1 rb=1 tb=1\n"
	if _, err := parseNetstatsUID(other, 10193); err == nil {
		t.Error("a uid with blocks and no readable row was read as 0 bytes")
	}
}

// readNetstats forces netstats to poll ("Forced poll": the history is updated
// at once, T1) and returns what the history holds for uid.
func (e *emu) readNetstats(t *testing.T, uid int) uidBytes {
	t.Helper()
	dump := e.shell(t, "dumpsys netstats --poll >/dev/null; dumpsys netstats --full --uid")
	b, err := parseNetstatsUID(dump, uid)
	if err != nil {
		t.Fatalf("netstats of uid %d: %v", uid, err)
	}
	return b
}

// appUID is the app's uid, as the app itself says it (run-as, as T1 used it
// for the traffic).
func (e *emu) appUID(t *testing.T) int {
	t.Helper()
	out := e.runAs(t, "id -u")
	uid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || uid < 10000 {
		t.Fatalf("run-as id -u = %q, want the app's uid (>= 10000)", out)
	}
	return uid
}

var defaultNetRe = regexp.MustCompile(`Active default network: (\S+)`)

// defaultNetwork returns what `dumpsys connectivity` says is the active
// default network ("none" with no network), or fails if the line is not there.
func (e *emu) defaultNetwork(t *testing.T) string {
	t.Helper()
	out := e.shell(t, "dumpsys connectivity")
	m := defaultNetRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no \"Active default network:\" in dumpsys connectivity (format changed?): %.300s", out)
	}
	return m[1]
}

func TestDefaultNetRe(t *testing.T) {
	for in, want := range map[string]string{
		"Active default network: none\n":      "none",
		"x\nActive default network: 102\ny\n": "102",
	} {
		if m := defaultNetRe.FindStringSubmatch(in); m == nil || m[1] != want {
			t.Errorf("defaultNetRe(%q) = %v, want %q", in, m, want)
		}
	}
}

// setNetwork turns wifi and mobile data on or off (both: the emulator has two
// default networks, T1 §7) and waits up to 60 s for dumpsys connectivity to
// say so: "Active default network: none" is the claim every test of "no
// network" rests on, so a network that does not go off fails here.
func (e *emu) setNetwork(t *testing.T, on bool) {
	t.Helper()
	verb := "disable"
	if on {
		verb = "enable"
	}
	e.shell(t, "svc wifi "+verb)
	e.shell(t, "svc data "+verb)
	var got string
	if !poll(60*time.Second, func() bool {
		got = e.defaultNetwork(t)
		return (got == "none") == !on
	}) {
		t.Fatalf("svc wifi/data %s: Active default network is %q after 60 s", verb, got)
	}
}

// netCleanup puts the network back at cleanup, whatever the test did. It
// enables wifi and data unconditionally, whatever they were before: that is
// right for the emulator's AVD and the CI's, which have both on (T1 §7); these
// tests never run on a phone (newEmu refuses any serial but an emulator's).
func (e *emu) netCleanup(t *testing.T) {
	t.Helper()
	e.atCleanup(t, "shell", "svc wifi enable")
	e.atCleanup(t, "shell", "svc data enable")
}

// sink is a TCP server on the PC's loopback that takes what it is sent (the
// emulator reaches it through its alias for the PC) and counts it.
type sink struct {
	ln    net.Listener
	conns atomic.Int64 // connections accepted
	bytes atomic.Int64 // bytes read from connections that have closed
}

func newSink(t *testing.T) *sink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sink{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns.Add(1)
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(time.Minute))
				n, _ := io.Copy(io.Discard, c)
				s.bytes.Add(n)
			}()
		}
	}()
	return s
}

func (s *sink) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// nc sends the shell's nc a line to the sink. It returns nc's exit code: 0 when
// the sink was reached, 1 with "nc: connect: Network is unreachable" without
// network (T1 §7). `ping` says nothing: it fails with the network on too (the
// emulator does not forward ICMP). An error of adb itself (device offline, no
// device) is not an answer of nc: it fails the test.
func (e *emu) nc(t *testing.T, s *sink) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", fmt.Sprintf("echo adbuddy | nc -w 2 %s %d 2>&1", hostFromEmulator, s.port()))
	if err != nil {
		t.Fatalf("adb shell nc: %v", err)
	}
	if strings.TrimSpace(errOut) != "" || strings.Contains(out, "device offline") || strings.Contains(out, "error: ") {
		t.Fatalf("adb itself failed running nc (exit %d): %s %s", code, strings.TrimSpace(out), strings.TrimSpace(errOut))
	}
	return code, strings.TrimSpace(out)
}

// ncUnreachable reports whether out, the output of a failed nc, is the
// kernel's "Network is unreachable" (T1 §7): the proof that the failure is the
// missing network and nothing else (a wrong address says "timed out" or
// "refused").
func ncUnreachable(out string) bool { return strings.Contains(out, "unreachable") }

func TestNcUnreachable(t *testing.T) {
	if !ncUnreachable("nc: connect: Network is unreachable") {
		t.Error("the measured message was not recognised")
	}
	for _, out := range []string{"", "nc: connect: Connection refused", "nc: connect: Connection timed out", "error: device offline"} {
		if ncUnreachable(out) {
			t.Errorf("%q counted as network unreachable", out)
		}
	}
}

// appTraffic makes the app's own uid send n bytes to the sink (run-as ... nc:
// netstats counts them for the app, T1 §7), and waits for the sink to have
// read them.
func (e *emu) appTraffic(t *testing.T, s *sink, n int) {
	t.Helper()
	before := s.bytes.Load()
	e.runAs(t, fmt.Sprintf(`sh -c 'head -c %d /dev/zero | nc -w 3 %s %d'`, n, hostFromEmulator, s.port()))
	if !poll(20*time.Second, func() bool { return s.bytes.Load()-before >= int64(n) }) {
		t.Fatalf("the sink read %d of the %d bytes the app's uid sent", s.bytes.Load()-before, n)
	}
}
