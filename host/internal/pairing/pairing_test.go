package pairing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeADB answers by the joined arguments. answers[key] is a list consumed
// in order; the last answer repeats.
type fakeADB struct {
	mu      sync.Mutex
	answers map[string][]string
	fail    map[string]string
	// failTimes[key] calls fail before the answers start.
	failTimes map[string]int
	calls     []string
	stdin     map[string]string
}

func (f *fakeADB) Run(_ context.Context, _ string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		if f.stdin == nil {
			f.stdin = map[string]string{}
		}
		f.stdin[key] = string(b)
	}
	if n := f.failTimes[key]; n > 0 {
		f.failTimes[key] = n - 1
		io.WriteString(stderr, "error: flaky")
		return 1, nil
	}
	if msg, ok := f.fail[key]; ok {
		io.WriteString(stderr, msg)
		return 1, nil
	}
	list := f.answers[key]
	if len(list) == 0 {
		return 0, nil
	}
	io.WriteString(stdout, list[0])
	if len(list) > 1 {
		f.answers[key] = list[1:]
	}
	return 0, nil
}

const mdnsHeader = "List of discovered mdns services\r\n"

func TestParseServices(t *testing.T) {
	out := mdnsHeader + "adb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41234\r\nADBuddy-x\t_adb-tls-pairing._tcp\t192.0.2.10:37000\r\n\r\n"
	ss := ParseServices(out)
	if len(ss) != 2 || ss[0] != (Service{"adb-AAA-bbb", ConnectType, "192.0.2.10:41234"}) || ss[1].Type != PairingType {
		t.Fatalf("%+v", ss)
	}
}

func TestQR(t *testing.T) {
	a, err := NewQR()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewQR()
	if a == b {
		t.Fatal("QR codes must be random")
	}
	if !regexp.MustCompile(`^WIFI:T:ADB;S:ADBuddy-[A-Za-z0-9]{8};P:[A-Za-z0-9]{16};;$`).MatchString(a.Payload()) {
		t.Fatalf("payload %q", a.Payload())
	}
	art, err := RenderQR(a.Payload(), true)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(art, "\n"), "\n")
	plain := strings.NewReplacer("\x1b[30;47m", "", "\x1b[0m", "").Replace(lines[0])
	width := utf8.RuneCountInString(plain)
	if width < 25 || len(lines) != (width+1)/2 || !strings.HasPrefix(lines[0], "\x1b[30;47m") {
		t.Fatalf("%d lines of width %d", len(lines), width)
	}
	for _, l := range lines {
		if utf8.RuneCountInString(strings.NewReplacer("\x1b[30;47m", "", "\x1b[0m", "").Replace(l)) != width {
			t.Fatal("ragged QR")
		}
	}
	plainArt, _ := RenderQR(a.Payload(), false)
	if strings.Contains(plainArt, "\x1b") {
		t.Fatal("escape codes without color")
	}
}

func TestPairQRTypesThePasswordOnStdin(t *testing.T) {
	q := QR{Name: "ADBuddy-abc", Password: "secret"}
	f := &fakeADB{answers: map[string][]string{
		"mdns services":         {mdnsHeader, mdnsHeader + "ADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.10:37000\n"},
		"pair 192.0.2.10:37000": {"Enter pairing code: Successfully paired to 192.0.2.10:37000 [guid=adb-AAA-bbb]\n"},
	}}
	guid, err := PairQR(context.Background(), f, q, 5*time.Second)
	if err != nil || guid != "adb-AAA-bbb" {
		t.Fatalf("%q %v", guid, err)
	}
	if f.stdin["pair 192.0.2.10:37000"] != "secret\n" {
		t.Fatalf("stdin %q", f.stdin)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "secret") {
			t.Fatalf("password in argv: %q", c)
		}
	}
}

func TestPairFailures(t *testing.T) {
	f := &fakeADB{
		answers: map[string][]string{"pair 192.0.2.10:1": {"Failed: Wrong password or connection was dropped.\n"}},
		fail:    map[string]string{"pair 192.0.2.10:2": "error: protocol fault"},
	}
	if _, err := Pair(context.Background(), f, "192.0.2.10:1", "123456"); err == nil || !strings.Contains(err.Error(), "Wrong password") {
		t.Fatalf("%v", err)
	}
	if _, err := Pair(context.Background(), f, "192.0.2.10:2", "123456"); err == nil || !strings.Contains(err.Error(), "protocol fault") {
		t.Fatalf("%v", err)
	}
	if _, err := WaitService(context.Background(), &fakeADB{answers: map[string][]string{"mdns services": {mdnsHeader}}}, "ADBuddy-x", PairingType, 20*time.Millisecond, 5*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("%v", err)
	}
}

func TestConnectWaitsForAutoConnect(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"devices": {"List of devices attached\n", "List of devices attached\nadb-AAA-bbb._adb-tls-connect._tcp\toffline\n", "List of devices attached\nadb-AAA-bbb._adb-tls-connect._tcp\tdevice\n"},
	}}
	serial, err := Connect(context.Background(), f, "adb-AAA-bbb", 5*time.Second, time.Hour, time.Millisecond)
	if err != nil || serial != "adb-AAA-bbb._adb-tls-connect._tcp" {
		t.Fatalf("%q %v", serial, err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "connect") {
			t.Fatal("must not connect explicitly while adb connects on its own")
		}
	}
}

func TestConnectFallsBackToExplicitConnect(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"devices":                  {"List of devices attached\n", "List of devices attached\n", "List of devices attached\n192.0.2.10:41234\tdevice\n"},
		"mdns services":            {mdnsHeader + "adb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41234\n"},
		"connect 192.0.2.10:41234": {"connected to 192.0.2.10:41234\n"},
	}}
	serial, err := Connect(context.Background(), f, "adb-AAA-bbb", 5*time.Second, 0, time.Millisecond)
	if err != nil || serial != "192.0.2.10:41234" {
		t.Fatalf("%q %v (calls %q)", serial, err, f.calls)
	}
	if _, err := Connect(context.Background(), &fakeADB{}, "adb-X", 20*time.Millisecond, time.Hour, 5*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("%v", err)
	}
}

// funcADB runs fn for every call.
type funcADB func(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error)

func (f funcADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
	return f(ctx, args, stdout, stderr)
}

// hangingADB blocks until ctx ends and then fails as a killed adb does.
var hangingADB = funcADB(func(ctx context.Context, _ []string, _, _ io.Writer) (int, error) {
	<-ctx.Done()
	return -1, errors.New("signal: killed")
})

// within fails the test if fn does not return in d.
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("did not return in time")
	}
}

func TestCancelDuringAnAdbCallIsReportedAsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	killed := funcADB(func(context.Context, []string, io.Writer, io.Writer) (int, error) {
		cancel()
		return -1, errors.New("signal: killed")
	})
	if _, err := WaitService(ctx, killed, "ADBuddy-x", PairingType, time.Minute, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitService: %v", err)
	}
	if _, err := Connect(ctx, killed, "adb-AAA-bbb", time.Minute, 0, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect: %v", err)
	}
	if _, err := Pair(ctx, killed, "192.0.2.10:37000", "123456"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Pair: %v", err)
	}
}

func TestTimeoutsBoundHangingAdb(t *testing.T) {
	within(t, 5*time.Second, func() {
		if _, err := WaitService(context.Background(), hangingADB, "ADBuddy-x", PairingType, 50*time.Millisecond, time.Millisecond); !errors.Is(err, ErrTimeout) {
			t.Errorf("WaitService: %v", err)
		}
		if _, err := Connect(context.Background(), hangingADB, "adb-AAA-bbb", 50*time.Millisecond, 0, time.Millisecond); !errors.Is(err, ErrTimeout) {
			t.Errorf("Connect: %v", err)
		}
	})
	old := pairTimeout
	pairTimeout = 50 * time.Millisecond
	t.Cleanup(func() { pairTimeout = old })
	within(t, 5*time.Second, func() {
		if _, err := Pair(context.Background(), hangingADB, "192.0.2.10:37000", "123456"); !errors.Is(err, ErrTimeout) {
			t.Errorf("Pair: %v", err)
		}
	})
}

func TestWaitIsCutToTheTimeLeft(t *testing.T) {
	empty := &fakeADB{answers: map[string][]string{"mdns services": {mdnsHeader}, "devices": {"List of devices attached\n"}}}
	within(t, 5*time.Second, func() {
		if _, err := WaitService(context.Background(), empty, "ADBuddy-x", PairingType, 30*time.Millisecond, time.Hour); !errors.Is(err, ErrTimeout) {
			t.Errorf("WaitService: %v", err)
		}
		if _, err := Connect(context.Background(), empty, "adb-AAA-bbb", 30*time.Millisecond, time.Hour, time.Hour); !errors.Is(err, ErrTimeout) {
			t.Errorf("Connect: %v", err)
		}
	})
}

func TestConnectMatchesTheGuidExactly(t *testing.T) {
	if _, err := Connect(context.Background(), &fakeADB{}, "", time.Second, 0, time.Millisecond); err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("empty guid: %v", err)
	}
	other := &fakeADB{answers: map[string][]string{
		"devices":       {"List of devices attached\nadb-AAA-bbbXYZ._adb-tls-connect._tcp\tdevice\nadb-AAA-bbb._adb-tls-connect._tcp.evil\tdevice\nxadb-AAA-bbb._adb-tls-connect._tcp\tdevice\n"},
		"mdns services": {mdnsHeader + "adb-AAA-bbbXYZ\t_adb-tls-connect._tcp\t192.0.2.66:41234\n"},
	}}
	if s, err := Connect(context.Background(), other, "adb-AAA-bbb", 30*time.Millisecond, 0, time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("another phone accepted: %q %v", s, err)
	}
	for _, c := range other.calls {
		if strings.HasPrefix(c, "connect") {
			t.Fatalf("connected to another guid: %q", c)
		}
	}
	renamed := &fakeADB{answers: map[string][]string{
		"devices": {"List of devices attached\nadb-AAA-bbb (2)._adb-tls-connect._tcp\tdevice\n"},
	}}
	if s, err := Connect(context.Background(), renamed, "adb-AAA-bbb", time.Second, time.Hour, time.Millisecond); err != nil || s != "adb-AAA-bbb (2)._adb-tls-connect._tcp" {
		t.Fatalf("%q %v", s, err)
	}
	svc := &fakeADB{answers: map[string][]string{"mdns services": {mdnsHeader + "adb-AAA-bbbXYZ\t_adb-tls-connect._tcp\t192.0.2.66:1\nadb-AAA-bbb (2)\t_adb-tls-connect._tcp\t192.0.2.10:2\n"}}}
	if s, err := WaitService(context.Background(), svc, "adb-AAA-bbb", ConnectType, time.Second, time.Millisecond); err != nil || s.Addr != "192.0.2.10:2" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestConnectChecksTheOutputAndRetries(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"devices":                  {"List of devices attached\n192.0.2.10:41234\tdevice\n"},
		"mdns services":            {mdnsHeader + "adb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41234\n"},
		"connect 192.0.2.10:41234": {"failed to connect to 192.0.2.10:41234\n", "already connected to 192.0.2.10:41234\n"},
	}}
	serial, err := Connect(context.Background(), f, "adb-AAA-bbb", 5*time.Second, 0, time.Millisecond)
	if err != nil || serial != "192.0.2.10:41234" {
		t.Fatalf("%q %v", serial, err)
	}
	connects, polls := 0, 0
	for _, c := range f.calls {
		switch c {
		case "connect 192.0.2.10:41234":
			connects++
		case "devices":
			polls++
		}
	}
	if connects != 2 || polls != 7 {
		t.Fatalf("%d connects over %d polls: %q", connects, polls, f.calls)
	}
	// Several matching services: only the first that answers is used.
	two := &fakeADB{answers: map[string][]string{
		"devices":              {"List of devices attached\n", "List of devices attached\n192.0.2.10:1\tdevice\n"},
		"mdns services":        {mdnsHeader + "adb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:1\nadb-AAA-bbb (2)\t_adb-tls-connect._tcp\t192.0.2.11:2\n"},
		"connect 192.0.2.10:1": {"connected to 192.0.2.10:1\n"},
		"connect 192.0.2.11:2": {"connected to 192.0.2.11:2\n"},
	}}
	if s, err := Connect(context.Background(), two, "adb-AAA-bbb", 5*time.Second, 0, time.Millisecond); err != nil || s != "192.0.2.10:1" {
		t.Fatalf("%q %v", s, err)
	}
	for _, c := range two.calls {
		if c == "connect 192.0.2.11:2" {
			t.Fatal("connected to a second service")
		}
	}
}

func TestParseServicesDropsHostileEntries(t *testing.T) {
	out := mdnsHeader +
		"\x1b[31mADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.66:37000\n" +
		"ADBuddy-abc​\t_adb-tls-pairing._tcp\t192.0.2.66:37001\n" +
		"ADBuddy-abc\tx\t_adb-tls-pairing._tcp\t192.0.2.66:37002\n" +
		"ADBuddy-abc\t_adb-tls-pairing._tcp\t-s\n" +
		"ADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.66:0\n" +
		"ADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.66:70000\n" +
		"ADBuddy-abc\t_adb-tls-pairing._tcp\thost.example:37000\n" +
		"ADBuddy-abc \t_adb-tls-pairing._tcp\t192.0.2.66:37003\n" +
		"ADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.10:37000\n"
	ss := ParseServices(out)
	if len(ss) != 2 || ss[0].Name != "ADBuddy-abc " || ss[1] != (Service{"ADBuddy-abc", PairingType, "192.0.2.10:37000"}) {
		t.Fatalf("%+q", ss)
	}
	s, err := WaitService(context.Background(), &fakeADB{answers: map[string][]string{"mdns services": {out}}}, "ADBuddy-abc", PairingType, time.Second, time.Millisecond)
	if err != nil || s.Addr != "192.0.2.10:37000" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestPairQRTriesEveryCandidate(t *testing.T) {
	q := QR{Name: "ADBuddy-abc", Password: "secret"}
	// A name with "\n" injected a fake line before the phone's own.
	f := &fakeADB{answers: map[string][]string{
		"mdns services":         {mdnsHeader + "\nADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.66:37000\n\t_adb-tls-pairing._tcp\t192.0.2.66:1\nADBuddy-abc\t_adb-tls-pairing._tcp\t192.0.2.10:37000\n"},
		"pair 192.0.2.66:37000": {"Failed: Wrong password or connection was dropped.\n"},
		"pair 192.0.2.10:37000": {"Successfully paired to 192.0.2.10:37000 [guid=adb-AAA-bbb]\n"},
	}}
	guid, err := PairQR(context.Background(), f, q, 5*time.Second)
	if err != nil || guid != "adb-AAA-bbb" {
		t.Fatalf("%q %v (calls %q)", guid, err, f.calls)
	}
}

func TestAdbTextIsPrintable(t *testing.T) {
	f := &fakeADB{
		answers: map[string][]string{"pair 192.0.2.10:1": {"Failed: \x1b[2Jboom\n"}},
		fail:    map[string]string{"pair 192.0.2.10:2": "error: \x1b]0;title\x07"},
	}
	for _, addr := range []string{"192.0.2.10:1", "192.0.2.10:2"} {
		if _, err := Pair(context.Background(), f, addr, "123456"); err == nil || strings.ContainsAny(err.Error(), "\x1b\x07") {
			t.Fatalf("%s: %q", addr, err)
		}
	}
}

func TestPollingToleratesAFewAdbErrors(t *testing.T) {
	f := &fakeADB{
		answers:   map[string][]string{"mdns services": {mdnsHeader + "ADBuddy-x\t_adb-tls-pairing._tcp\t192.0.2.10:37000\n"}},
		failTimes: map[string]int{"mdns services": 3},
	}
	if _, err := WaitService(context.Background(), f, "ADBuddy-x", PairingType, 5*time.Second, time.Millisecond); err != nil {
		t.Fatalf("3 errors: %v", err)
	}
	f = &fakeADB{failTimes: map[string]int{"devices": 4}}
	if _, err := Connect(context.Background(), f, "adb-AAA-bbb", 5*time.Second, time.Hour, time.Millisecond); err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("4 errors: %v", err)
	}
}

func TestPairValidatesItsInput(t *testing.T) {
	f := &fakeADB{}
	for _, c := range []struct{ addr, code string }{
		{"192.0.2.10:37000", "123456\npair 192.0.2.66:1"},
		{"192.0.2.10:37000", "123\r456"},
		{"-s", "123456"},
		{"192.0.2.10", "123456"},
	} {
		if _, err := Pair(context.Background(), f, c.addr, c.code); err == nil {
			t.Errorf("%q %q accepted", c.addr, c.code)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("adb called: %q", f.calls)
	}
}

func TestPairReadsOnlyAWellFormedGuid(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"pair 192.0.2.10:1": {"Successfully paired to 192.0.2.10:1 [guid=adb-AAA\x1bbbb]\n"},
		"pair 192.0.2.10:2": {"Successfully paired to 192.0.2.10:2 [guid=adb-AAA-bbb.x_y]\n"},
	}}
	if g, err := Pair(context.Background(), f, "192.0.2.10:1", "123456"); err == nil {
		t.Fatalf("guid %q accepted", g)
	}
	if g, err := Pair(context.Background(), f, "192.0.2.10:2", "123456"); err != nil || g != "adb-AAA-bbb.x_y" {
		t.Fatalf("%q %v", g, err)
	}
}

func TestRandomStringRejectsBiasedBytes(t *testing.T) {
	old := randReader
	t.Cleanup(func() { randReader = old })
	// 255 is past the last whole multiple of the alphabet size: skipped.
	randReader = bytes.NewReader([]byte{255, 0, 254, 1})
	s, err := randomString(2)
	if err != nil || s != alphabet[0:2] {
		t.Fatalf("%q %v", s, err)
	}
}

func TestQRHidesThePassword(t *testing.T) {
	q := QR{Name: "ADBuddy-abc", Password: "hunter2hunter2xx"}
	if s := fmt.Sprintf("%v %+v %#v %s", q, q, q, q); strings.Contains(s, q.Password) {
		t.Fatalf("password printed: %s", s)
	}
}

func TestQRHasAFourModuleQuietZone(t *testing.T) {
	art, err := RenderQR("WIFI:T:ADB;S:ADBuddy-abc;P:x;;", false)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(art, "\n"), "\n")
	for i, l := range lines {
		r := []rune(l)
		blank := func(s []rune) bool { return strings.TrimLeft(string(s), " ") == "" }
		if (i < 2 || i >= len(lines)-2) && !blank(r) || !blank(r[:4]) || !blank(r[len(r)-4:]) {
			t.Fatalf("line %d %q", i, l)
		}
	}
}

// GuidOf reads the phone's guid back from its mDNS serial, also when mDNS
// renamed it after a clash; anything else has no guid.
func TestGuidOf(t *testing.T) {
	for serial, want := range map[string]string{
		"adb-AAA-bbb._adb-tls-connect._tcp":         "adb-AAA-bbb",
		"adb-AAA-bbb (2)._adb-tls-connect._tcp":     "adb-AAA-bbb",
		"adb-A.B_c-d._adb-tls-connect._tcp":         "adb-A.B_c-d",
		"192.0.2.10:41000":                          "",
		"SERIAL0001":                                "",
		"adb-AAA-bbb._adb-tls-pairing._tcp":         "",
		"adb-AAA-bbb._adb-tls-connect._tcp.evil":    "",
		"xadb-AAA-bbb._adb-tls-connect._tcp":        "",
		"adb-AAA bbb._adb-tls-connect._tcp":         "",
		"adb-AAA-bbb (x)._adb-tls-connect._tcp":     "",
		"adb-AAA-bbb (12345)._adb-tls-connect._tcp": "",
		"._adb-tls-connect._tcp":                    "",
	} {
		got, ok := GuidOf(serial)
		if got != want || ok != (want != "") {
			t.Errorf("GuidOf(%q) = %q, %v; want %q", serial, got, ok, want)
		}
		if ok && !IsGuidSerial(got, serial) {
			t.Errorf("IsGuidSerial(%q, %q) = false", got, serial)
		}
	}
}

// VerifyIdentity compares the phone's ro.serialno with the middle of the
// guid adb built from it (adb-<serialno>-<suffix>), without case.
func TestVerifyIdentity(t *testing.T) {
	phone := func(serialno string) *fakeADB {
		return &fakeADB{answers: map[string][]string{"shell getprop ro.serialno": {serialno + "\n"}}}
	}
	if err := VerifyIdentity(context.Background(), phone("AAA111"), "adb-AAA111-bbbCCC"); err != nil {
		t.Fatalf("same phone: %v", err)
	}
	if err := VerifyIdentity(context.Background(), phone("aaa111"), "adb-AAA111-bbbCCC"); err != nil {
		t.Fatalf("same phone, other case: %v", err)
	}
	for _, other := range []string{"ZZZ999", "AAA1110", ""} {
		if err := VerifyIdentity(context.Background(), phone(other), "adb-AAA111-bbbCCC"); !errors.Is(err, ErrWrongPhone) {
			t.Fatalf("serialno %q: %v", other, err)
		}
	}
	// A guid that does not look like adb-<serialno>-<suffix> cannot be
	// checked: nil, and adb is not even asked.
	for _, guid := range []string{"", "AAA111", "adb-AAA111", "adb--bbb", "adb-AAA111-", "mdns-AAA111-bbb"} {
		f := phone("ZZZ999")
		if err := VerifyIdentity(context.Background(), f, guid); err != nil {
			t.Fatalf("guid %q: %v", guid, err)
		}
		if _, ok := GuidSerialNo(guid); ok {
			t.Fatalf("guid %q has a serial number", guid)
		}
		if len(f.calls) != 0 {
			t.Fatalf("guid %q: adb called %q", guid, f.calls)
		}
	}
	if s, ok := GuidSerialNo("adb-AAA111-bbbCCC"); !ok || s != "AAA111" {
		t.Fatalf("GuidSerialNo: %q %v", s, ok)
	}
	// getprop failing is an error, never a match nor ErrWrongPhone.
	f := &fakeADB{fail: map[string]string{"shell getprop ro.serialno": "error: device offline"}}
	if err := VerifyIdentity(context.Background(), f, "adb-AAA111-bbbCCC"); err == nil || errors.Is(err, ErrWrongPhone) || !strings.Contains(err.Error(), "device offline") {
		t.Fatalf("getprop failing: %v", err)
	}
}

// ConnectAddr runs `adb connect` and waits for the address to be online.
func TestConnectAddr(t *testing.T) {
	old := connectAddrPoll
	connectAddrPoll = time.Millisecond
	t.Cleanup(func() { connectAddrPoll = old })
	const addr = "100.64.0.5:41234"

	t.Run("connected", func(t *testing.T) {
		f := &fakeADB{answers: map[string][]string{
			"connect " + addr: {"connected to " + addr + "\n"},
			"devices":         {"List of devices attached\n", "List of devices attached\n" + addr + "\toffline\n", "List of devices attached\n" + addr + "\tdevice\n"},
		}}
		serial, err := ConnectAddr(context.Background(), f, addr, 5*time.Second)
		if err != nil || serial != addr {
			t.Fatalf("%q %v", serial, err)
		}
		if f.calls[0] != "connect "+addr {
			t.Fatalf("calls %q", f.calls)
		}
	})
	t.Run("already connected", func(t *testing.T) {
		f := &fakeADB{answers: map[string][]string{
			"connect " + addr: {"already connected to " + addr + "\n"},
			"devices":         {"List of devices attached\n" + addr + "\tdevice\n"},
		}}
		if serial, err := ConnectAddr(context.Background(), f, addr, 5*time.Second); err != nil || serial != addr {
			t.Fatalf("%q %v", serial, err)
		}
	})
	t.Run("connect fails", func(t *testing.T) {
		for _, out := range []string{
			"failed to connect to '" + addr + "': Connection refused\n",
			"cannot connect to " + addr + ": No route to host\n",
			"",
		} {
			f := &fakeADB{answers: map[string][]string{
				"connect " + addr: {out},
				"devices":         {"List of devices attached\n" + addr + "\tdevice\n"},
			}}
			if _, err := ConnectAddr(context.Background(), f, addr, 5*time.Second); err == nil || errors.Is(err, ErrTimeout) {
				t.Fatalf("%q: %v", out, err)
			}
		}
		f := &fakeADB{fail: map[string]string{"connect " + addr: "error: protocol fault"}}
		if _, err := ConnectAddr(context.Background(), f, addr, 5*time.Second); err == nil || !strings.Contains(err.Error(), "protocol fault") {
			t.Fatalf("exit 1: %v", err)
		}
	})
	t.Run("never online", func(t *testing.T) {
		f := &fakeADB{answers: map[string][]string{
			"connect " + addr: {"connected to " + addr + "\n"},
			"devices":         {"List of devices attached\n" + addr + "\tunauthorized\n"},
		}}
		_, err := ConnectAddr(context.Background(), f, addr, 50*time.Millisecond)
		if !errors.Is(err, ErrTimeout) || !strings.Contains(err.Error(), "unauthorized") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("invalid address", func(t *testing.T) {
		for _, bad := range []string{"", "100.64.0.5", "phone.local:41234", "100.64.0.5:0", "100.64.0.5:65536", "100.64.0.5:x", "100.64.0.5:41234\n"} {
			f := &fakeADB{}
			if _, err := ConnectAddr(context.Background(), f, bad, time.Second); !errors.Is(err, ErrInvalidAddr) {
				t.Fatalf("%q: %v", bad, err)
			}
			if len(f.calls) != 0 {
				t.Fatalf("%q: adb called %q", bad, f.calls)
			}
		}
	})
}

// Final review of M1c, M1: a serial number that does not tell phones apart
// ("unknown", or shorter than 4 characters) cannot be checked.
func TestWeakSerialNumbersCannotBeChecked(t *testing.T) {
	for _, guid := range []string{"adb-unknown-abc123", "adb-UNKNOWN-abc123", "adb-ABC-xyz", "adb-A-xyz"} {
		f := &fakeADB{answers: map[string][]string{"shell getprop ro.serialno": {"ZZZZ9999\n"}}}
		if _, ok := GuidSerialNo(guid); ok {
			t.Errorf("%q has a usable serial number", guid)
		}
		if err := VerifyIdentity(context.Background(), f, guid); err != nil || len(f.calls) != 0 {
			t.Errorf("%q: %v, calls %q", guid, err, f.calls)
		}
	}
	if s, ok := GuidSerialNo("adb-ABCD-xyz"); !ok || s != "ABCD" {
		t.Fatalf("4 characters: %q %v", s, ok)
	}
}
