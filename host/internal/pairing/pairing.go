// Package pairing pairs and connects a phone over wireless debugging
// (spec §3.1 steps 3-4): a QR code the phone scans, or IP:port plus the
// 6-digit code; then the connection found by mDNS.
//
// mDNS discovery uses adb's own resolver (`adb mdns services`) through
// ADBuddy's private adb server, so the host needs no mDNS code of its own.
package pairing

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// mDNS service types of wireless debugging.
const (
	PairingType = "_adb-tls-pairing._tcp"
	ConnectType = "_adb-tls-connect._tcp"
)

// ADB runs adb against ADBuddy's private server (implemented by adb.Runner).
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Service is one line of `adb mdns services`.
type Service struct {
	Name, Type, Addr string
}

// ParseServices parses `adb mdns services` ("name\ttype\tip:port" lines).
func ParseServices(out string) []Service {
	var ss []Service
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) == 3 && strings.HasPrefix(f[1], "_adb") {
			ss = append(ss, Service{Name: strings.TrimSpace(f[0]), Type: strings.TrimSpace(f[1]), Addr: strings.TrimSpace(f[2])})
		}
	}
	return ss
}

func run(ctx context.Context, a ADB, stdin io.Reader, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", args, stdin, &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(errb.String() + " " + out.String())
		return "", fmt.Errorf("adb %s: %s", args[0], msg)
	}
	return out.String(), nil
}

// Services lists the wireless-debugging services adb sees on the network.
func Services(ctx context.Context, a ADB) ([]Service, error) {
	out, err := run(ctx, a, nil, "mdns", "services")
	if err != nil {
		return nil, err
	}
	return ParseServices(out), nil
}

// ErrTimeout means the phone did not show up in time.
var ErrTimeout = errors.New("timed out waiting for the phone")

// WaitService polls until a service with that name (a prefix for connect
// services) and type appears.
func WaitService(ctx context.Context, a ADB, name, typ string, timeout, every time.Duration) (Service, error) {
	deadline := time.Now().Add(timeout)
	for {
		ss, err := Services(ctx, a)
		if err != nil {
			return Service{}, err
		}
		for _, s := range ss {
			if s.Type == typ && (s.Name == name || (typ == ConnectType && strings.HasPrefix(s.Name, name))) {
				return s, nil
			}
		}
		if time.Now().After(deadline) {
			return Service{}, ErrTimeout
		}
		select {
		case <-ctx.Done():
			return Service{}, ctx.Err()
		case <-time.After(every):
		}
	}
}

// QR is a pairing QR code: the phone scans it in
// Wireless debugging → Pair device with QR code.
type QR struct {
	Name     string // mDNS instance the phone announces: ADBuddy-<random>
	Password string
}

const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

// NewQR creates a fresh name and password. Both use only letters and digits,
// so the Wi-Fi QR format needs no escaping.
func NewQR() (QR, error) {
	name, err := randomString(8)
	if err != nil {
		return QR{}, err
	}
	pw, err := randomString(16)
	if err != nil {
		return QR{}, err
	}
	return QR{Name: "ADBuddy-" + name, Password: pw}, nil
}

// Payload is the QR content (spec §3.1): WIFI:T:ADB;S:<name>;P:<password>;;
func (q QR) Payload() string { return "WIFI:T:ADB;S:" + q.Name + ";P:" + q.Password + ";;" }

var guidRe = regexp.MustCompile(`\[guid=([^\]\s]+)\]`)

// Pair runs `adb pair <addr>` and types the code on its stdin (a code in
// argv would be visible to other processes). It returns the phone's adb
// guid, the mDNS name of its connect service.
func Pair(ctx context.Context, a ADB, addr, code string) (string, error) {
	out, err := run(ctx, a, strings.NewReader(code+"\n"), "pair", addr)
	if err != nil {
		return "", err
	}
	if !strings.Contains(out, "Successfully paired") {
		return "", fmt.Errorf("adb pair: %s", strings.TrimSpace(out))
	}
	m := guidRe.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("adb pair did not report the phone's guid: %s", strings.TrimSpace(out))
	}
	return m[1], nil
}

// PairQR waits for the phone that scanned q to announce its pairing service
// and pairs with it.
func PairQR(ctx context.Context, a ADB, q QR, timeout time.Duration) (string, error) {
	s, err := WaitService(ctx, a, q.Name, PairingType, timeout, time.Second)
	if err != nil {
		return "", err
	}
	return Pair(ctx, a, s.Addr, q.Password)
}

// Device is one line of `adb devices`.
type Device struct{ Serial, State string }

func devices(ctx context.Context, a ADB) ([]Device, error) {
	out, err := run(ctx, a, nil, "devices")
	if err != nil {
		return nil, err
	}
	var ds []Device
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && !strings.HasPrefix(line, "List of") && !strings.HasPrefix(line, "*") {
			ds = append(ds, Device{Serial: f[0], State: f[1]})
		}
	}
	return ds, nil
}

// Connect waits until the paired phone (guid) is online in state "device"
// and returns its serial. adb connects paired phones found by mDNS on its
// own (15-20 s on a fresh server); after autoConnectWait the connect service
// is looked up and `adb connect` is run explicitly.
func Connect(ctx context.Context, a ADB, guid string, timeout, autoConnectWait, every time.Duration) (string, error) {
	start := time.Now()
	deadline := start.Add(timeout)
	connectedTo := ""
	for {
		ds, err := devices(ctx, a)
		if err != nil {
			return "", err
		}
		for _, d := range ds {
			if d.State == "device" && (strings.HasPrefix(d.Serial, guid) || (connectedTo != "" && d.Serial == connectedTo)) {
				return d.Serial, nil
			}
		}
		if connectedTo == "" && time.Since(start) >= autoConnectWait {
			if ss, err := Services(ctx, a); err == nil {
				for _, s := range ss {
					if s.Type == ConnectType && strings.HasPrefix(s.Name, guid) {
						if _, err := run(ctx, a, nil, "connect", s.Addr); err == nil {
							connectedTo = s.Addr
						}
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return "", ErrTimeout
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(every):
		}
	}
}
