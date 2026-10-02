package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

const testPath = "/data/app/~~AbC1-_==/tech.hybriq.adbuddy-XyZ2+/=/base.apk"

// dumpsysOf is `dumpsys package` for the app with versionCode code, after a
// hidden system copy with a higher one that must not count.
func dumpsysOf(code int) string {
	return fmt.Sprintf("Packages:\n  Package [tech.hybriq.adbuddy] (1a2b3c):\n    versionCode=%d minSdk=30 targetSdk=37\n    versionName=0.2.0\n"+
		"Hidden system packages:\n  Package [tech.hybriq.adbuddy] (4d5e6f):\n    versionCode=999999 minSdk=30 targetSdk=37\n", code)
}

// installed makes f answer like a phone with apk installed at path with
// versionCode code.
func installed(f *fakeADB, path string, apk []byte, code int) {
	if f.answers == nil {
		f.answers = map[string][]string{}
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + path + "\n"}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", path)] = []string{strconv.Itoa(len(apk)) + " 4242 1700000000\n"}
	enc := base64.StdEncoding.EncodeToString(apk)
	var wrapped strings.Builder
	for len(enc) > 76 {
		wrapped.WriteString(enc[:76] + "\n")
		enc = enc[76:]
	}
	wrapped.WriteString(enc + "\n")
	f.answers["shell "+shq.Join("base64", path)] = []string{wrapped.String()}
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{dumpsysOf(code)}
}

func signedBy(cert []byte) []byte {
	return apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}}, apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: cert}}})
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestVerify(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 5)
	v, err := NewVerifier(fixed(Trust{Release: rel}), 5).Verify(context.Background(), f)
	if err != nil || v.Cert() != rel || v.Dev() || v.path != testPath {
		t.Fatalf("release: %+v %v", v, err)
	}
	// A development build is trusted for its exact bytes, not its key.
	dev := signedBy(other)
	f = &fakeADB{}
	installed(f, testPath, dev, 1)
	v, err = NewVerifier(fixed(Trust{Release: rel, DevAPK: digest(dev)}), 0).Verify(context.Background(), f)
	if err != nil || v.Cert() != apktest.SHA256(other) || !v.Dev() {
		t.Fatalf("development: %+v %v", v, err)
	}
}

// Every way the check can go wrong gives no Verified (fail closed).
func TestVerifyFailsClosed(t *testing.T) {
	rel := apktest.SHA256(ours)
	good := signedBy(ours)
	dev := signedBy(other)
	// Same key as the trusted development build, other bytes.
	sameKey := apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: other}}})
	stat := "shell " + shq.Join("stat", "-c", "%s %i %Y", testPath)
	b64 := "shell " + shq.Join("base64", testPath)
	cases := []struct {
		name  string
		setup func(f *fakeADB)
		trust *Trust // nil: Trust{Release: rel}
		min   int
		is    error
	}{
		{name: "someone else's app", setup: func(f *fakeADB) { installed(f, testPath, signedBy(other), 1) }, is: ErrSigner},
		{name: "same key as the development build", trust: &Trust{Release: rel, DevAPK: digest(dev)},
			setup: func(f *fakeADB) { installed(f, testPath, sameKey, 1) }, is: ErrSigner},
		{name: "nothing trusted", trust: &Trust{}, setup: func(f *fakeADB) { installed(f, testPath, good, 1) }, is: ErrUnpinned},
		{name: "too old", min: 2, setup: func(f *fakeADB) { installed(f, testPath, good, 1) }, is: ErrAppTooOld},
		{name: "not installed", setup: func(f *fakeADB) { f.answers = map[string][]string{"shell pm path tech.hybriq.adbuddy": {""}} }, is: ErrNoTrustedApp},
		{name: "two paths", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\npackage:/data/app/x/split_a.apk\n"}
		}, is: ErrNoTrustedApp},
		{name: "odd path", setup: func(f *fakeADB) { installed(f, "/sdcard/../data/app/x/base.apk", good, 1) }, is: ErrNoTrustedApp},
		{name: "not under /data/app", setup: func(f *fakeADB) { installed(f, "/mnt/expand/u/app/x/base.apk", good, 1) }, is: ErrNoTrustedApp},
		{name: "reinstalled while read", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\n", "package:/data/app/~~N==/tech.hybriq.adbuddy-N==/base.apk\n"}
		}, is: ErrNoTrustedApp},
		{name: "too big", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{strconv.Itoa(maxAPK+1) + " 1 1\n"}
		}, is: ErrNoTrustedApp},
		{name: "size that does not match", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{"7 4242 1700000000\n"}
		}, is: ErrNoTrustedApp},
		{name: "base64 longer than the size", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[b64] = []string{base64.StdEncoding.EncodeToString(append(good, make([]byte, 4096)...)) + "\n"}
		}, is: ErrNoTrustedApp},
		{name: "unreadable base64", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[b64] = []string{"base64: permission denied\n"}
		}, is: ErrNoTrustedApp},
		{name: "versionCode only in another package", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Packages:\n  Package [tech.hybriq.adbuddyx] (1):\n    versionCode=5\n"}
		}, is: ErrNoTrustedApp},
		{name: "odd stat", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{"stat: bad format\n"}
		}, is: ErrNoTrustedApp},
		{name: "updated in place while read", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{strconv.Itoa(len(good)) + " 4242 1700000000\n", strconv.Itoa(len(good)) + " 4343 1700000099\n"}
		}, is: ErrNoTrustedApp},
		{name: "adb fails", setup: func(f *fakeADB) {}, is: ErrNoTrustedApp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeADB{}
			c.setup(f)
			trust := Trust{Release: rel}
			if c.trust != nil {
				trust = *c.trust
			}
			v, err := NewVerifier(fixed(trust), c.min).Verify(context.Background(), f)
			if !errors.Is(err, c.is) || v != (Verified{}) {
				t.Fatalf("got %+v, %v; want %v", v, err, c.is)
			}
		})
	}
}

// The verifier checks a path once: the same app is not read again, a
// reinstalled one (a new path on Android 11+) is.
func TestVerifierRemembersThePath(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(other), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	for i := 0; i < 3; i++ {
		if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) {
			t.Fatalf("%d: %v", i, err)
		}
	}
	if n := countCalls(f, "shell "+shq.Join("base64", testPath)); n != 1 {
		t.Fatalf("read the app %d times", n)
	}
	newPath := "/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk"
	installed(f, newPath, signedBy(ours), 1)
	if v, err := vr.Verify(context.Background(), f); err != nil || v.path != newPath {
		t.Fatalf("after reinstall: %+v %v", v, err)
	}
}

// A failure of adb is not remembered: the next call tries again.
func TestVerifierRetriesAfterAnADBFailure(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	key := "shell " + shq.Join("base64", testPath)
	saved := f.answers[key]
	delete(f.answers, key) // the fake exits 1 without an answer
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("first: %v", err)
	}
	f.answers[key] = saved
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatalf("second: %v", err)
	}
}

func TestStill(t *testing.T) {
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	if err := (Verified{}).still(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("zero Verified: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("a zero Verified ran %q", f.calls)
	}
	v := Verified{path: testPath, key: strconv.Itoa(len(signedBy(ours))) + " 4242 1700000000", cert: "x"}
	if err := v.still(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:/data/app/~~B==/tech.hybriq.adbuddy-C==/base.apk\n"}
	if err := v.still(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("reinstalled: %v", err)
	}
}

func TestDevTrust(t *testing.T) {
	dir := t.TempDir()
	if got := LoadTrust("r", dir); got != (Trust{Release: "r"}) {
		t.Fatalf("empty: %+v", got)
	}
	d := digest([]byte("apk"))
	p, err := TrustDevAPK(dir, d)
	if err != nil || p != DevTrustPath(dir) {
		t.Fatal(p, err)
	}
	if got := LoadTrust("r", dir); got != (Trust{Release: "r", DevAPK: d}) {
		t.Fatalf("saved: %+v", got)
	}
	if _, err := TrustDevAPK(dir, "not hex"); err == nil {
		t.Fatal("bad digest saved")
	}
	if removed, err := ForgetDevAPK(dir); !removed || err != nil {
		t.Fatalf("forget: %v %v", removed, err)
	}
	if removed, err := ForgetDevAPK(dir); removed || err != nil {
		t.Fatalf("forget again: %v %v", removed, err)
	}
	os.MkdirAll(filepath.Dir(DevTrustPath(dir)), 0o700)
	os.WriteFile(DevTrustPath(dir), []byte("garbage\n"), 0o600)
	if got := LoadTrust("r", dir); got.DevAPK != "" {
		t.Fatalf("garbage trusted: %+v", got)
	}
	if runtime.GOOS != "windows" {
		os.WriteFile(DevTrustPath(dir), []byte(d+"\n"), 0o644)
		os.Chmod(DevTrustPath(dir), 0o644)
		if got := LoadTrust("r", dir); got.DevAPK != "" {
			t.Fatalf("world-readable file trusted: %+v", got)
		}
		os.Remove(DevTrustPath(dir))
		target := filepath.Join(dir, "elsewhere")
		os.WriteFile(target, []byte(d+"\n"), 0o600)
		if err := os.Symlink(target, DevTrustPath(dir)); err == nil {
			if got := LoadTrust("r", dir); got.DevAPK != "" {
				t.Fatalf("link trusted: %+v", got)
			}
		}
	}
}

// fixed is a trust loader that always returns t.
func fixed(t Trust) func() Trust { return func() Trust { return t } }

// Same refuses an app updated in place (same path, other stat), any app
// before a good Verify, and a development build no longer trusted.
func TestSame(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("before Verify: %v", err)
	}
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if err := vr.Same(context.Background(), f); err != nil {
		t.Fatalf("same app: %v", err)
	}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", testPath)] = []string{"99 1 2\n"}
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("updated in place: %v", err)
	}

	dev := signedBy(other)
	trust := Trust{DevAPK: digest(dev)}
	f = &fakeADB{}
	installed(f, testPath, dev, 1)
	vr = NewVerifier(func() Trust { return trust }, 0)
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	trust.DevAPK = "" // trusted-dev-apk deleted while serve runs
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("development build no longer trusted: %v", err)
	}
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("Verify after the trust went: %v", err)
	}
}

// An app updated in place (same path, other stat) is read again, not
// answered from the cache.
func TestVerifierReadsAgainWhenTheAppChangesInPlace(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	installed(f, testPath, signedBy(other), 1) // same path, other size
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) {
		t.Fatalf("updated in place: %v", err)
	}
	if n := countCalls(f, "shell "+shq.Join("base64", testPath)); n != 2 {
		t.Fatalf("read the app %d times, want 2", n)
	}
}

// With a release certificate set, deleting trusted-dev-apk stops trusting
// the development build at once (the trust is part of the cache key).
func TestForgettingTheDevelopmentBuildTakesEffect(t *testing.T) {
	dev := signedBy(other)
	trust := Trust{Release: apktest.SHA256(ours), DevAPK: digest(dev)}
	f := &fakeADB{}
	installed(f, testPath, dev, 1)
	vr := NewVerifier(func() Trust { return trust }, 0)
	if v, err := vr.Verify(context.Background(), f); err != nil || !v.Dev() {
		t.Fatalf("trusted: %+v %v", v, err)
	}
	trust.DevAPK = ""
	if v, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) || v != (Verified{}) {
		t.Fatalf("forgotten: %+v %v", v, err)
	}
}

// gateADB holds back the base64 read until release is closed.
type gateADB struct {
	ADB
	started, release chan struct{}
	once             sync.Once
}

func (g *gateADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if strings.Contains(strings.Join(args, " "), "base64") {
		g.once.Do(func() { close(g.started) })
		<-g.release
	}
	return g.ADB.Run(ctx, dir, args, stdin, stdout, stderr)
}

// Same answers while a Verify is still reading the APK: the channel asks
// it for every connection and must not queue behind a slow read.
func TestSameDoesNotWaitForVerify(t *testing.T) {
	rel := apktest.SHA256(ours)
	current := &fakeADB{}
	installed(current, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	updated := &fakeADB{}
	installed(updated, "/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk", signedBy(ours), 1)
	g := &gateADB{ADB: updated, started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	open := func() { releaseOnce.Do(func() { close(g.release) }) }
	defer open()
	done := make(chan error, 1)
	go func() {
		_, err := vr.Verify(context.Background(), g)
		done <- err
	}()
	<-g.started // Verify holds its lock, stuck reading the new app
	if err := within(t, 5*time.Second, func() error { return vr.Same(context.Background(), current) }); err != nil {
		t.Fatalf("Same while Verify reads: %v", err)
	}
	open()
	if err := <-done; err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func countCalls(f *fakeADB, key string) int {
	n := 0
	for _, c := range f.calls {
		if c == key {
			n++
		}
	}
	return n
}

// A definitive failure of Verify drops the last good answer: Same must not
// keep admitting an app that Verify has just rejected. A transient failure
// (adb) keeps it: Same still checks the stat itself.
func TestDefiniteFailureForgetsTheLastGoodAnswer(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// Transient: the APK cannot be read this time.
	key := "shell " + shq.Join("base64", testPath)
	other1 := signedBy(other)
	installed(f, testPath, other1, 1)
	delete(f.answers, key)
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) || errors.Is(err, ErrSigner) {
		t.Fatalf("transient: %v", err)
	}
	// The good answer survived: the good app, back in place, is still the same.
	installed(f, testPath, signedBy(ours), 1)
	if err := vr.Same(context.Background(), f); err != nil {
		t.Fatalf("Same after a transient failure: %v", err)
	}
	installed(f, testPath, other1, 1)
	delete(f.answers, key)
	// Definitive: it is signed by someone else.
	installed(f, testPath, other1, 1)
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) {
		t.Fatalf("definitive: %v", err)
	}
	// The stat of the rejected app is the one now installed: without the
	// fix, Same only compares path and stat with the old good answer and
	// would fail anyway, so put the old good stat back in place.
	installed(f, testPath, signedBy(ours), 1)
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("Same after a definitive failure: %v", err)
	}
}
