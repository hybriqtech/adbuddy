package phoneapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// MinAppVersionCode is the oldest app this host deals with (spec M6a §6.2):
// raise it when a release fixes a security bug in the app, so that an older
// copy signed by hybriqtech gets nothing either.
const MinAppVersionCode = 0

var (
	// ErrNoTrustedApp means the app on the phone could not be checked or is
	// not one this host trusts: it gets nothing.
	ErrNoTrustedApp = errors.New("the ADBuddy app on the phone is not trusted, so it gets no token or permissions " +
		"(run `adbuddy setup`, or `adbuddy setup --apk <file>` for a development build)")
	// ErrAppTooOld means the app is older than MinAppVersionCode.
	ErrAppTooOld = errors.New("the ADBuddy app on the phone is too old for this host (install a newer build with `adbuddy setup --apk <file>`; plain `adbuddy setup` does not replace an installed app)")
)

// pullTimeout bounds reading the installed APK (some MB over Wi-Fi).
var pullTimeout = 2 * time.Minute

// basePathRe is where Android 11+ keeps an installed app: one base.apk in a
// randomised folder under /data/app.
var basePathRe = regexp.MustCompile(`^/data/app/[A-Za-z0-9._~=+/-]+/base\.apk$`)

var versionCodeRe = regexp.MustCompile(`(?m)^\s*versionCode=(\d+)`)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// statKeyRe is `stat -c '%s %i %Y'`: size, inode and mtime of base.apk.
var statKeyRe = regexp.MustCompile(`^(\d+) \d+ \d+$`)

// Trust is what the host trusts for the app (spec M6a §6.1).
type Trust struct {
	Release string // SHA-256 of the release certificate: SigningCertSHA256, or a test's
	DevAPK  string // SHA-256 of the one development APK `setup --apk` trusted, if any
}

// DevTrustPath is where TrustDevAPK keeps the development APK's SHA-256.
func DevTrustPath(dataDir string) string {
	return filepath.Join(dataDir, "app", "trusted-dev-apk")
}

// LoadTrust returns release and the development APK trusted in dataDir.
// Only a regular file (not a link) holding one SHA-256, and private on
// Unix, counts: anything else trusts no development build.
func LoadTrust(release, dataDir string) Trust {
	t := Trust{Release: release}
	p := DevTrustPath(dataDir)
	before, err := os.Lstat(p)
	if err != nil || !before.Mode().IsRegular() || runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return t
	}
	f, err := os.Open(p)
	if err != nil {
		return t
	}
	defer f.Close()
	if after, err := f.Stat(); err != nil || !os.SameFile(before, after) {
		return t // swapped for something else between the two looks
	}
	b, err := io.ReadAll(io.LimitReader(f, 128))
	if err != nil {
		return t
	}
	if d := strings.TrimSpace(string(b)); sha256Re.MatchString(d) {
		t.DevAPK = d
	}
	return t
}

// TrustDevAPK saves digest, the SHA-256 of a development APK, as trusted.
func TrustDevAPK(dataDir, digest string) (string, error) {
	if !sha256Re.MatchString(digest) {
		return "", fmt.Errorf("not a SHA-256: %q", digest)
	}
	p := DevTrustPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	return p, WritePrivate(p, []byte(digest+"\n"))
}

// WritePrivate writes data to p (0600) through a new temporary file renamed
// over it, so it never writes through a link someone left at p.
func WritePrivate(p string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ForgetDevAPK stops trusting the development APK; removed says whether
// one was trusted.
func ForgetDevAPK(dataDir string) (removed bool, err error) {
	err = os.Remove(DevTrustPath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Verified is the proof that the app installed on the phone is one this
// host trusts. Only Verifier.Verify makes a non-zero one (its fields are
// not exported); the functions that give the app something (the token,
// consent, permissions) take it and refuse the zero value (spec M6a §6.3).
type Verified struct {
	path   string // the base.apk that was checked
	key    string // its stat (size, inode, mtime): an update in place changes it
	cert   string // its certificate's SHA-256
	digest string // its SHA-256
	dev    bool   // the trusted development APK, not a release
}

// Cert is the SHA-256 of the app's certificate.
func (v Verified) Cert() string { return v.cert }

// Dev reports whether the app is the development build setup --apk trusted.
func (v Verified) Dev() bool { return v.dev }

// still checks, right before acting, that v is not the zero value and that
// the app checked is still the one installed: same path (a reinstall moves
// it to a new folder) and same stat (an update in place would change it).
func (v Verified) still(ctx context.Context, a ADB) error {
	if v.path == "" || v.key == "" || v.cert == "" {
		return fmt.Errorf("%w (not checked)", ErrNoTrustedApp)
	}
	p, err := appPath(ctx, a)
	if err != nil {
		return err
	}
	if p != v.path {
		return fmt.Errorf("%w (the app was reinstalled after it was checked)", ErrNoTrustedApp)
	}
	k, _, err := statKey(ctx, a, p)
	if err != nil {
		return err
	}
	if k != v.key {
		return fmt.Errorf("%w (the app changed after it was checked)", ErrNoTrustedApp)
	}
	return nil
}

// Verifier checks the installed app against the Trust that load returns,
// read again on every call (deleting trusted-dev-apk takes effect at once).
// It remembers the last definite answer for the app's path, stat and trust,
// so serve's reconnections do not read the APK again until something
// changes. Failures of adb are not remembered.
type Verifier struct {
	load    func() Trust
	minCode int

	mu  sync.Mutex
	key string   // path, stat and trust of the remembered answer
	v   Verified // the remembered answer
	err error

	// ok is the last good answer, for Same. Verify holds mu while it reads
	// the APK (up to pullTimeout), and Same, which the channel asks for
	// every connection, must not wait for that: hence an atomic pointer.
	ok atomic.Pointer[Verified]
}

// NewVerifier returns a Verifier for the trust load returns (LoadTrust)
// and minCode (MinAppVersionCode).
func NewVerifier(load func() Trust, minCode int) *Verifier {
	return &Verifier{load: load, minCode: minCode}
}

// Same returns nil when the app installed now is the one the last good
// Verify checked (same path and stat) and its trust still holds. Serve
// asks it for every app that connects with the token (bridge
// Options.Admit): an update with the same key keeps the app's data, the
// token included, but not its path and stat (spec M6a §6.3).
func (vr *Verifier) Same(ctx context.Context, a ADB) error {
	last := vr.ok.Load()
	if last == nil {
		return fmt.Errorf("%w (not checked)", ErrNoTrustedApp)
	}
	if last.dev && vr.load().DevAPK != last.digest {
		return fmt.Errorf("%w (the development build is no longer trusted)", ErrNoTrustedApp)
	}
	return last.still(ctx, a)
}

// Verify reads the installed app (pm path, stat, base64) and checks who
// signed it and its version code (spec M6a §6.2). It is safe without
// verifying any cryptography: Android verified that APK when it installed
// it, and Signers reads the same block Android did. Every failure is an
// ErrNoTrustedApp or an ErrAppTooOld, and gives no Verified.
func (vr *Verifier) Verify(ctx context.Context, a ADB) (Verified, error) {
	path, err := appPath(ctx, a)
	if err != nil {
		return Verified{}, err
	}
	sk, n, err := statKey(ctx, a, path)
	if err != nil {
		return Verified{}, err
	}
	trust := vr.load()
	key := strings.Join([]string{path, sk, trust.Release, trust.DevAPK}, "|")
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.key == key {
		return vr.v, vr.err
	}
	v, err := vr.check(ctx, a, trust, path, sk, n)
	if err == nil || errors.Is(err, ErrSigner) || errors.Is(err, ErrAppTooOld) || errors.Is(err, ErrUnpinned) {
		vr.key, vr.v, vr.err = key, v, err // definite: remembered for this app and trust
		if err != nil {
			vr.ok.Store(nil) // a rejected app leaves no good answer for Same
		}
	}
	if err == nil {
		vr.ok.Store(&v)
	}
	return v, err
}

func (vr *Verifier) check(ctx context.Context, a ADB, trust Trust, path, sk string, n int64) (Verified, error) {
	if trust.Release == "" && trust.DevAPK == "" {
		return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, ErrUnpinned)
	}
	data, err := readAPK(ctx, a, path, n)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (reading the app: %w)", ErrNoTrustedApp, err)
	}
	signers, err := Signers(data)
	if err != nil {
		return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, fmt.Errorf("%w (%v)", ErrSigner, err))
	}
	digest := digestOf(data)
	dev := false
	if err := CheckSigner(data, trust.Release); err != nil {
		if trust.DevAPK == "" || digest != trust.DevAPK {
			return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, err)
		}
		dev = true
	}
	dump, err := shell(ctx, a, "dumpsys", "package", Package)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (dumpsys: %w)", ErrNoTrustedApp, err)
	}
	code, err := versionCodeOf(dump)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (%v)", ErrNoTrustedApp, err)
	}
	if code < vr.minCode {
		return Verified{}, fmt.Errorf("%w (version code %d, at least %d)", ErrAppTooOld, code, vr.minCode)
	}
	v := Verified{path: path, key: sk, cert: signers[0], digest: digest, dev: dev}
	if err := v.still(ctx, a); err != nil {
		return Verified{}, fmt.Errorf("%w (the app changed while it was checked)", err)
	}
	return v, nil
}

// statKey returns `stat -c '%s %i %Y'` of path (size, inode and mtime) and
// the size, which must be at most maxAPK.
func statKey(ctx context.Context, a ADB, path string) (string, int64, error) {
	out, err := shell(ctx, a, "stat", "-c", "%s %i %Y", path)
	if err != nil {
		return "", 0, fmt.Errorf("%w (stat: %w)", ErrNoTrustedApp, err)
	}
	m := statKeyRe.FindStringSubmatch(out)
	if m == nil {
		return "", 0, fmt.Errorf("%w (unexpected stat %q)", ErrNoTrustedApp, out)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n <= 0 || n > maxAPK {
		return "", 0, fmt.Errorf("%w (unexpected size %q)", ErrNoTrustedApp, m[1])
	}
	return out, n, nil
}

// readAPK reads the n bytes of path with base64, its output bounded to
// what n bytes take in base64 with toybox's line breaks.
func readAPK(ctx context.Context, a ADB, path string, n int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	enc := (n + 2) / 3 * 4
	out := &boundedBuffer{max: int(enc + 2*(enc/76) + 16)} // CR LF after each line at worst
	var errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"shell", shq.Join("base64", path)}, nil, out, &errb)
	switch {
	case out.over:
		return nil, errors.New("base64 printed more than the app's size")
	case err != nil:
		return nil, err
	case code != 0:
		return nil, fmt.Errorf("base64: exit %d: %s", code, strings.TrimSpace(errb.String()))
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out.buf.String()), ""))
	if err != nil || int64(len(data)) != n {
		return nil, errors.New("the app read back does not match its size")
	}
	return data, nil
}

// boundedBuffer keeps at most max bytes; past that it refuses the write
// and remembers it.
type boundedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.over || b.buf.Len()+len(p) > b.max {
		b.over = true
		return 0, errors.New("output too long")
	}
	return b.buf.Write(p)
}

// versionCodeOf reads the versionCode of the app's own "Package [...]"
// block of `dumpsys package`, not of a hidden system copy.
func versionCodeOf(dump string) (int, error) {
	head := "Package [" + Package + "]"
	i := strings.Index(dump, head)
	if i < 0 {
		return 0, errors.New("no package block in dumpsys")
	}
	block := dump[i+len(head):]
	for _, end := range []string{"Package [", "Hidden system packages:"} {
		if j := strings.Index(block, end); j >= 0 {
			block = block[:j]
		}
	}
	m := versionCodeRe.FindStringSubmatch(block)
	if m == nil {
		return 0, errors.New("no versionCode in dumpsys")
	}
	return strconv.Atoi(m[1])
}

func digestOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// appPath returns where the app's base.apk is, or an ErrNoTrustedApp.
func appPath(ctx context.Context, a ADB) (string, error) {
	out, err := shell(ctx, a, "pm", "path", Package)
	if err != nil {
		return "", fmt.Errorf("%w (pm path: %w)", ErrNoTrustedApp, err)
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		return "", fmt.Errorf("%w (pm path gave %d lines)", ErrNoTrustedApp, len(lines))
	}
	p, ok := strings.CutPrefix(lines[0], "package:")
	if !ok || !basePathRe.MatchString(p) || strings.Contains(p, "..") {
		return "", fmt.Errorf("%w (unexpected path %q)", ErrNoTrustedApp, lines[0])
	}
	return p, nil
}
