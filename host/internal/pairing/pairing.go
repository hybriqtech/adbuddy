// Package pairing pairs and connects a phone over wireless debugging
// (spec §3.1 steps 3-4): a QR code the phone scans, or IP:port plus the
// 6-digit code; then the connection found by mDNS.
//
// mDNS discovery uses adb's own resolver (`adb mdns services`) through
// ADBuddy's private adb server, so the host needs no mDNS code of its own.
// Everything mDNS reports comes from the local network: names and addresses
// are checked before use, and adb's text is made printable before it is shown.
package pairing

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
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
// Lines whose name has control or format characters, or whose address is
// not IP:port, are dropped.
func ParseServices(out string) []Service {
	var ss []Service
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 3 {
			continue
		}
		s := Service{Name: f[0], Type: strings.TrimSpace(f[1]), Addr: strings.TrimSpace(f[2])}
		if strings.HasPrefix(s.Type, "_adb") && cleanName(s.Name) && validAddr(s.Addr) {
			ss = append(ss, s)
		}
	}
	return ss
}

func cleanName(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}

// validAddr accepts only IP:port with a port in 1-65535.
func validAddr(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || !cleanName(addr) {
		return false
	}
	if _, err := netip.ParseAddr(host); err != nil {
		return false
	}
	p, err := strconv.ParseUint(port, 10, 16)
	return err == nil && p > 0
}

// matchName reports whether got is the mDNS instance want, possibly renamed
// by mDNS after a clash ("want (2)").
func matchName(want, got string) bool {
	if got == want {
		return true
	}
	rest, ok := strings.CutPrefix(got, want+" (")
	if !ok {
		return false
	}
	n, ok := strings.CutSuffix(rest, ")")
	if !ok || n == "" || len(n) > 4 {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsGuidSerial reports whether serial is the adb serial of the phone guid
// connected through mDNS ("<guid>._adb-tls-connect._tcp", or "<guid> (N)…"
// when mDNS renamed it after a clash).
func IsGuidSerial(guid, serial string) bool {
	inst, ok := strings.CutSuffix(serial, "."+ConnectType)
	return ok && matchName(guid, inst)
}

var guidOnlyRe = regexp.MustCompile(`^adb-[A-Za-z0-9._-]{1,128}$`)

// GuidOf returns the phone's guid when serial is its mDNS serial (see
// IsGuidSerial), so the phone can be found again under whatever serial adb
// lists it with.
func GuidOf(serial string) (string, bool) {
	inst, ok := strings.CutSuffix(serial, "."+ConnectType)
	if !ok {
		return "", false
	}
	if i := strings.LastIndex(inst, " ("); i >= 0 && matchName(inst[:i], inst) {
		inst = inst[:i]
	}
	if !guidOnlyRe.MatchString(inst) {
		return "", false
	}
	return inst, true
}

// run runs adb and returns its stdout. If ctx ended meanwhile, it returns
// ctx.Err() (a killed adb reports "signal: killed" instead).
func run(ctx context.Context, a ADB, stdin io.Reader, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", args, stdin, &out, &errb)
	if cerr := ctx.Err(); cerr != nil {
		return "", cerr
	}
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := classify.Printable(strings.TrimSpace(errb.String() + " " + out.String()))
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

// maxPollErrors is how many adb failures in a row a wait tolerates (adb may
// fail while its server starts).
const maxPollErrors = 3

// poll runs step every `every` until it is done, ctx ends or adb fails more
// than maxPollErrors times in a row. The last wait is cut to ctx's deadline.
func poll(ctx context.Context, every time.Duration, step func() (bool, error)) error {
	errs := 0
	for {
		done, err := step()
		if err == nil && done {
			return nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if errs++; errs > maxPollErrors {
				return err
			}
		} else {
			errs = 0
		}
		wait := every
		if dl, ok := ctx.Deadline(); ok {
			wait = max(min(wait, time.Until(dl)), 0)
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// timeoutErr turns the end of our own deadline into ErrTimeout; the caller's
// cancel or deadline is returned as is.
func timeoutErr(parent context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil {
		return ErrTimeout
	}
	return err
}

// waitServices polls until services with that name (or "name (N)") and type
// appear, and returns all of them.
func waitServices(ctx context.Context, a ADB, name, typ string, timeout, every time.Duration) ([]Service, error) {
	if name == "" {
		return nil, errors.New("no service name to wait for")
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var found []Service
	err := poll(ctx, every, func() (bool, error) {
		ss, err := Services(ctx, a)
		if err != nil {
			return false, err
		}
		found = found[:0]
		for _, s := range ss {
			if s.Type == typ && matchName(name, s.Name) {
				found = append(found, s)
			}
		}
		return len(found) > 0, nil
	})
	if err != nil {
		return nil, timeoutErr(parent, err)
	}
	return found, nil
}

// WaitService polls until a service with that name (or "name (N)", as mDNS
// renames clashes) and type appears. It gives up with ErrTimeout after
// timeout, even if adb hangs.
func WaitService(ctx context.Context, a ADB, name, typ string, timeout, every time.Duration) (Service, error) {
	ss, err := waitServices(ctx, a, name, typ, timeout, every)
	if err != nil {
		return Service{}, err
	}
	return ss[0], nil
}

// QR is a pairing QR code: the phone scans it in
// Wireless debugging → Pair device with QR code.
type QR struct {
	Name     string // mDNS instance the phone announces: ADBuddy-<random>
	Password string
}

// String hides the password from %v and %s.
func (q QR) String() string { return q.Name }

// GoString hides the password from %#v.
func (q QR) GoString() string { return fmt.Sprintf("pairing.QR{Name:%q}", q.Name) }

// randReader is the source of randomness (tests replace it).
var randReader io.Reader = rand.Reader

// pairTimeout bounds one `adb pair`.
var pairTimeout = 30 * time.Second

const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// randomString draws n characters of alphabet without bias: bytes past the
// last whole multiple of the alphabet size are skipped.
func randomString(n int) (string, error) {
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		b := buf[:n-len(out)]
		if _, err := io.ReadFull(randReader, b); err != nil {
			return "", err
		}
		for _, c := range b {
			if int(c) < limit {
				out = append(out, alphabet[int(c)%len(alphabet)])
			}
		}
	}
	return string(out), nil
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

var guidRe = regexp.MustCompile(`\[guid=(adb-[A-Za-z0-9._-]{1,128})\]`)

// Pair runs `adb pair <addr>` and types the code on its stdin (a code in
// argv would be visible to other processes). It returns the phone's adb
// guid, the mDNS name of its connect service. adb gets pairTimeout to answer.
func Pair(ctx context.Context, a ADB, addr, code string) (string, error) {
	if !validAddr(addr) {
		return "", fmt.Errorf("%q is not an IP address and port", classify.Printable(addr))
	}
	if strings.ContainsAny(code, "\r\n") {
		return "", errors.New("the pairing code cannot contain line breaks")
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, pairTimeout)
	defer cancel()
	out, err := run(ctx, a, strings.NewReader(code+"\n"), "pair", addr)
	if err != nil {
		if timeoutErr(parent, err) == ErrTimeout {
			return "", fmt.Errorf("adb pair %s did not answer in %s: %w", addr, pairTimeout, ErrTimeout)
		}
		return "", err
	}
	if !strings.Contains(out, "Successfully paired") {
		return "", fmt.Errorf("adb pair: %s", classify.Printable(strings.TrimSpace(out)))
	}
	m := guidRe.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("adb pair did not report the phone's guid: %s", classify.Printable(strings.TrimSpace(out)))
	}
	return m[1], nil
}

// PairQR waits for the phone that scanned q to announce its pairing service
// and pairs with it. Anyone on the network can announce the same name, so
// every matching service is tried: only the phone knows the password.
func PairQR(ctx context.Context, a ADB, q QR, timeout time.Duration) (string, error) {
	ss, err := waitServices(ctx, a, q.Name, PairingType, timeout, time.Second)
	if err != nil {
		return "", err
	}
	var last error
	for _, s := range ss {
		guid, err := Pair(ctx, a, s.Addr, q.Password)
		if err == nil {
			return guid, nil
		}
		if cerr := ctx.Err(); cerr != nil {
			return "", cerr
		}
		last = err
	}
	return "", last
}

// Device is one line of `adb devices`.
type Device struct{ Serial, State string }

func devices(ctx context.Context, a ADB) ([]Device, error) {
	out, err := run(ctx, a, nil, "devices")
	if err != nil {
		return nil, err
	}
	var ds []Device
	// "serial\tstate": split on the tab, since an mDNS serial may have a
	// space ("adb-<guid> (2)._adb-tls-connect._tcp").
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.HasPrefix(line, "List of") || strings.HasPrefix(line, "*") {
			continue
		}
		serial, rest, ok := strings.Cut(line, "\t")
		state := strings.Fields(rest)
		if ok && serial != "" && len(state) > 0 {
			ds = append(ds, Device{Serial: serial, State: state[0]})
		}
	}
	return ds, nil
}

// connectRetryPolls is how often (in polls) `adb connect` is tried again
// while the phone does not show up.
const connectRetryPolls = 5

// Connect waits until the paired phone (guid) is online in state "device"
// and returns its serial. adb connects paired phones found by mDNS on its
// own (15-20 s on a fresh server); after autoConnectWait the connect service
// is looked up and `adb connect` is run explicitly, again every few polls
// while the phone is missing. It gives up with ErrTimeout after timeout,
// even if adb hangs.
func Connect(ctx context.Context, a ADB, guid string, timeout, autoConnectWait, every time.Duration) (string, error) {
	if guid == "" {
		return "", errors.New("no phone guid to connect to")
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	connectedTo, serial := "", ""
	polls, lastTry := 0, 0
	err := poll(ctx, every, func() (bool, error) {
		ds, err := devices(ctx, a)
		if err != nil {
			return false, err
		}
		for _, d := range ds {
			if d.State == "device" && (IsGuidSerial(guid, d.Serial) || connectedTo != "" && d.Serial == connectedTo) {
				serial = d.Serial
				return true, nil
			}
		}
		polls++
		if time.Since(start) >= autoConnectWait && (lastTry == 0 || polls-lastTry >= connectRetryPolls) {
			lastTry = polls
			if addr := connectService(ctx, a, guid); addr != "" {
				connectedTo = addr
			}
		}
		return false, nil
	})
	if err != nil {
		return "", timeoutErr(parent, err)
	}
	return serial, nil
}

// connectService runs `adb connect` to the first connect service of guid
// that answers "connected to" (or "already connected to"), and returns its
// address ("" if none did).
func connectService(ctx context.Context, a ADB, guid string) string {
	ss, err := Services(ctx, a)
	if err != nil {
		return ""
	}
	for _, s := range ss {
		if s.Type != ConnectType || !matchName(guid, s.Name) {
			continue
		}
		if out, err := run(ctx, a, nil, "connect", s.Addr); err == nil && strings.Contains(out, "connected to") {
			return s.Addr
		}
	}
	return ""
}

// ErrWrongPhone means the phone at an address is not the paired one.
var ErrWrongPhone = errors.New("this is not the paired phone")

// guidSerialRe is the guid adb gives a phone: adb-<serialno>-<suffix>.
var guidSerialRe = regexp.MustCompile(`^adb-([A-Za-z0-9][A-Za-z0-9._-]{0,127})-([A-Za-z0-9]{1,32})$`)

// GuidSerialNo returns the serial number (ro.serialno) in the middle of the
// phone's guid, adb-<serialno>-<suffix>; false if guid has another form or
// its serial number does not tell phones apart ("unknown", fewer than 4
// characters): then the phone's identity cannot be checked.
func GuidSerialNo(guid string) (string, bool) {
	m := guidSerialRe.FindStringSubmatch(guid)
	if m == nil || len(m[1]) < 4 || strings.EqualFold(m[1], "unknown") {
		return "", false
	}
	return m[1], true
}

// VerifyIdentity checks that the phone a talks to (an ADB already aimed at
// the active serial) is the one with this guid: its ro.serialno must be the
// guid's serial number (without case). An address may reach another phone
// (another IP, a port reused), and a guid is the only name of the paired
// one. A guid without the form adb-<serialno>-<suffix> cannot be checked:
// the answer is nil (see GuidSerialNo) and the caller warns.
func VerifyIdentity(ctx context.Context, a ADB, guid string) error {
	want, ok := GuidSerialNo(guid)
	if !ok {
		return nil
	}
	out, err := run(ctx, a, nil, "shell", "getprop", "ro.serialno")
	if err != nil {
		return fmt.Errorf("cannot read the phone's serial number: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(out), want) {
		return ErrWrongPhone
	}
	return nil
}

// ErrInvalidAddr means an address is not IP:port with a port in 1-65535.
var ErrInvalidAddr = errors.New("not an IP address and port")

// connectAddrPoll is how often ConnectAddr asks `adb devices` (tests shorten it).
var connectAddrPoll = 500 * time.Millisecond

// ConnectAddr runs `adb connect addr` (a is a host ADB, with no serial) and
// waits until addr is online in state "device", at most timeout even if adb
// hangs. It returns the serial adb lists the phone with: addr itself. It is
// the way in when mDNS does not reach the phone (Tailscale, another network).
func ConnectAddr(ctx context.Context, a ADB, addr string, timeout time.Duration) (string, error) {
	if !validAddr(addr) {
		return "", fmt.Errorf("%q: %w", classify.Printable(addr), ErrInvalidAddr)
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := run(ctx, a, nil, "connect", addr)
	if err != nil {
		if timeoutErr(parent, err) == ErrTimeout {
			return "", fmt.Errorf("adb connect %s did not answer in %s: %w", addr, timeout, ErrTimeout)
		}
		return "", err
	}
	if !connectedTo(out, addr) {
		msg := classify.Printable(strings.TrimSpace(out))
		if msg == "" {
			msg = "no answer"
		}
		return "", fmt.Errorf("adb connect %s: %s", addr, msg)
	}
	state := ""
	err = poll(ctx, connectAddrPoll, func() (bool, error) {
		ds, err := devices(ctx, a)
		if err != nil {
			return false, err
		}
		for _, d := range ds {
			if d.Serial == addr {
				state = d.State
				return d.State == "device", nil
			}
		}
		return false, nil
	})
	if err != nil {
		err = timeoutErr(parent, err)
		if err == ErrTimeout && state != "" {
			return "", fmt.Errorf("the phone at %s stayed %s: %w", addr, classify.Printable(state), ErrTimeout)
		}
		return "", err
	}
	return addr, nil
}

// connectedTo reports whether `adb connect addr` said it is connected
// ("connected to addr" or "already connected to addr"): adb reports a
// failure ("failed to connect to …") with exit code 0 too.
func connectedTo(out, addr string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "already ")
		if line == "connected to "+addr {
			return true
		}
	}
	return false
}
