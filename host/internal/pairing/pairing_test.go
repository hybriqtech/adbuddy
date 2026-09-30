package pairing

import (
	"context"
	"errors"
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
	calls   []string
	stdin   map[string]string
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
		"devices":       {"List of devices attached\n", "List of devices attached\n", "List of devices attached\n192.0.2.10:41234\tdevice\n"},
		"mdns services": {mdnsHeader + "adb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41234\n"},
	}}
	serial, err := Connect(context.Background(), f, "adb-AAA-bbb", 5*time.Second, 0, time.Millisecond)
	if err != nil || serial != "192.0.2.10:41234" {
		t.Fatalf("%q %v (calls %q)", serial, err, f.calls)
	}
	if _, err := Connect(context.Background(), &fakeADB{}, "adb-X", 20*time.Millisecond, time.Hour, 5*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("%v", err)
	}
}
