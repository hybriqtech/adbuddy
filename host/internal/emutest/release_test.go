package emutest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// releaseAPK is the unsigned APK of `gradle :app:assembleRelease` (R8 and
// resource shrinking), as release-check.yml builds it, relative to this
// package.
var releaseAPK = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "release", "app-release-unsigned.apk")

// releaseHint says how to build it, with release-check's test version.
const releaseHint = "build it first: cd android && gradle --offline :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101"

// shrunkAway is a class of the app that R8 renames or inlines in the release
// build (no keep rule names it) and that the debug build has: its descriptor
// in a dex tells an unshrunk APK.
const shrunkAway = "Ltech/hybriq/adbuddy/ui/RemoteUi;"

// sdkTools are the SDK's and the JDK's tools the tests build and sign APKs
// with, never the release key (spec M6a §10: only its owner touches it).
type sdkTools struct {
	aapt2, zipalign, apksigner string // build-tools
	androidJar                 string // a platform's android.jar, for aapt2
	keytool, javaHome          string // the JDK
}

// exe adds Windows' extension to a tool's name.
func exe(name, ext string) string {
	if runtime.GOOS == "windows" {
		return name + ext
	}
	return name
}

// sdkRoot is the Android SDK, found as scripts/emulator.sh finds it, or "".
func sdkRoot() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk"),
		filepath.Join(home, "Android", "Sdk"), filepath.Join(home, "Library", "Android", "sdk")} {
		if d == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(d, "platform-tools")); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// versionLess compares two dotted versions ("36.0.0" < "37.0.0"; "android-37.0" < "android-38").
func versionLess(a, b string) bool {
	num := func(s string) []int {
		var n []int
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
			v, _ := strconv.Atoi(f)
			n = append(n, v)
		}
		return n
	}
	return slices.Compare(num(a), num(b)) < 0
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"36.0.0", "37.0.0", true}, {"37.0.0", "36.0.0", false}, {"9.0.0", "10.0.0", true},
		{"android-34", "android-37.0", true}, {"android-37.0", "android-37.0", false},
	} {
		if got := versionLess(c.a, c.b); got != c.less {
			t.Errorf("versionLess(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// newest returns the entry of dir, newest version first, that ok accepts, or "".
func newest(dir string, ok func(string) bool) string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.SortFunc(names, func(a, b string) int {
		if versionLess(b, a) {
			return -1
		}
		if versionLess(a, b) {
			return 1
		}
		return 0
	})
	for _, n := range names {
		if p := filepath.Join(dir, n); ok(p) {
			return p
		}
	}
	return ""
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// findTools returns the tools, or skips the test saying what is missing.
func findTools(t *testing.T) sdkTools {
	t.Helper()
	sdk := sdkRoot()
	if sdk == "" {
		t.Skip("no Android SDK: set ANDROID_HOME")
	}
	var tl sdkTools
	bt := newest(filepath.Join(sdk, "build-tools"), func(d string) bool {
		return exists(filepath.Join(d, exe("aapt2", ".exe"))) && exists(filepath.Join(d, exe("zipalign", ".exe"))) &&
			exists(filepath.Join(d, exe("apksigner", ".bat")))
	})
	if bt == "" {
		t.Skip("no build-tools with aapt2, zipalign and apksigner in " + sdk + ": sdkmanager \"build-tools;37.0.0\"")
	}
	tl.aapt2, tl.zipalign, tl.apksigner = filepath.Join(bt, exe("aapt2", ".exe")), filepath.Join(bt, exe("zipalign", ".exe")), filepath.Join(bt, exe("apksigner", ".bat"))
	platform := newest(filepath.Join(sdk, "platforms"), func(d string) bool { return exists(filepath.Join(d, "android.jar")) })
	if platform == "" {
		t.Skip("no platform with android.jar in " + sdk + ": sdkmanager \"platforms;android-37.0\"")
	}
	tl.androidJar = filepath.Join(platform, "android.jar")
	if jh := os.Getenv("JAVA_HOME"); jh != "" && exists(filepath.Join(jh, "bin", exe("keytool", ".exe"))) {
		tl.javaHome = jh
	} else if kt, err := exec.LookPath("keytool"); err == nil {
		tl.javaHome = filepath.Dir(filepath.Dir(kt))
	} else {
		t.Skip("no JDK: set JAVA_HOME to a JDK 17 (keytool, and java for apksigner)")
	}
	tl.keytool = filepath.Join(tl.javaHome, "bin", exe("keytool", ".exe"))
	return tl
}

// run runs one of the tools with JAVA_HOME and its java first in PATH (and
// env on top), and returns its output; it fails the test on an error.
func (tl sdkTools) run(t *testing.T, env []string, tool string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = append(os.Environ(), "JAVA_HOME="+tl.javaHome,
		"PATH="+filepath.Join(tl.javaHome, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q: %v\n%s", filepath.Base(tool), args, err, out)
	}
	return string(out)
}

// newKey creates a throwaway signing key (RSA 2048, one day) in dir, as
// release-check does, and returns the keystore and its password. Nothing of
// it outlives the test (t.TempDir).
func (tl sdkTools) newKey(t *testing.T, dir, cn string) (ks, pass string) {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	pass = "throwaway-" + hex.EncodeToString(b)
	ks = filepath.Join(dir, cn+".p12")
	tl.run(t, []string{"KS_PASS=" + pass}, tl.keytool, "-genkeypair", "-keystore", ks, "-storetype", "PKCS12",
		"-storepass:env", "KS_PASS", "-alias", "adbuddy", "-keyalg", "RSA", "-keysize", "2048", "-validity", "1",
		"-dname", "CN="+cn)
	return ks, pass
}

// sign aligns in and signs it into out with the key, with apksigner's
// scheme flags (--v1-signing-enabled …).
func (tl sdkTools) sign(t *testing.T, in, out, ks, pass string, schemes ...string) {
	t.Helper()
	aligned := out + ".aligned"
	tl.run(t, nil, tl.zipalign, "-f", "4", in, aligned)
	args := append([]string{"sign", "--ks", ks, "--ks-type", "PKCS12", "--ks-pass", "env:KS_PASS", "--ks-key-alias", "adbuddy"}, schemes...)
	tl.run(t, []string{"KS_PASS=" + pass}, tl.apksigner, append(args, "--out", out, aligned)...)
}

// dexHas reports whether any classes*.dex of the APK holds s.
func dexHas(t *testing.T, apk []byte, s string) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(apk), int64(len(apk)))
	if err != nil {
		t.Fatalf("read the APK as a zip: %v", err)
	}
	dexes := 0
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "classes") || !strings.HasSuffix(f.Name, ".dex") {
			continue
		}
		dexes++
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	if dexes == 0 {
		t.Fatal("the APK has no classes*.dex")
	}
	return false
}

// releaseSigned signs the release APK with a throwaway key made for this
// test (never the release key) and returns the signed APK and the SHA-256
// of its certificate, which the test then trusts as the release one. It
// skips without the APK or the tools, and fails if the APK is not shrunk.
func releaseSigned(t *testing.T) (apk, cert string) {
	t.Helper()
	if !exists(releaseAPK) {
		t.Skip("no release APK at " + releaseAPK + "; " + releaseHint)
	}
	tl := findTools(t)
	dir := t.TempDir()
	ks, pass := tl.newKey(t, dir, "emutest-release")
	apk = filepath.Join(dir, "adbuddy.apk")
	tl.sign(t, releaseAPK, apk, ks, pass, "--v1-signing-enabled", "false", "--v2-signing-enabled", "true", "--v3-signing-enabled", "true")
	data, err := os.ReadFile(apk)
	if err != nil {
		t.Fatal(err)
	}
	certs, err := phoneapp.Signers(data)
	if err != nil || len(certs) == 0 {
		t.Fatalf("the host does not read the signed release APK: %v", err)
	}
	if err := phoneapp.CheckSigner(data, certs[0]); err != nil {
		t.Fatalf("CheckSigner with its own certificate: %v", err)
	}
	if dexHas(t, data, shrunkAway) {
		t.Fatalf("%s is in the release APK's dex: it was not shrunk by R8 (isMinifyEnabled in android/app/build.gradle.kts?)", shrunkAway)
	}
	return apk, certs[0]
}

// Validation 2 of M6a, its part on the PC: the release APK (R8) signed as
// release-check signs it, with a throwaway key; every block's signer is that
// key (the host's own parser, spec M6a §5) and R8 did shrink it. The debug
// APK, when built, is the control: the class shrunkAway is in its dex.
func TestTheReleaseAPKIsShrunkAndSignedWithAThrowawayKey(t *testing.T) {
	releaseSigned(t)
	if data, err := os.ReadFile(appAPK); err == nil {
		if !dexHas(t, data, shrunkAway) {
			t.Errorf("%s is not in the debug APK either: the check would not tell a shrunk APK", shrunkAway)
		}
	} else {
		t.Logf("no debug APK for the control (%v)", err)
	}
}

// installRelease installs the signed release APK on a clean slate and
// returns the proof the host gives it, trusting cert as the release
// certificate (not a development APK: Dev is false).
func (e *emu) installRelease(t *testing.T, apk, cert string) phoneapp.Verified {
	t.Helper()
	_, _, _, _ = e.adb.Output(e.ctx, "uninstall", phoneapp.Package)
	if err := phoneapp.Install(e.ctx, e.adb, apk); err != nil {
		t.Fatalf("install the release APK: %v", err)
	}
	e.uninstallAtCleanup(t, phoneapp.Package)
	v, err := phoneapp.NewVerifier(func() phoneapp.Trust { return phoneapp.Trust{Release: cert} }, phoneapp.MinAppVersionCode).Verify(e.ctx, e.adb)
	if err != nil {
		t.Fatalf("verify the installed release app: %v", err)
	}
	if v.Dev() || v.Cert() != cert {
		t.Fatalf("verified as dev %v with certificate %s, want the release certificate %s", v.Dev(), v.Cert(), cert)
	}
	return v
}

// Validation 2 of M6a: the release app (R8) works on API 34. Installed as
// setup installs a release, checked by the host against its certificate, it
// says hello and sends device.info (kotlinx.serialization, app to host); it
// shows a chat.message (host to app), answers a ui.request with its id
// (both ways, with a rid) and shows a confirm.request as its notification.
func TestTheReleaseAppTalksOverTheLink(t *testing.T) {
	e := newEmu(t)
	apk, cert := releaseSigned(t)
	l := e.linkTo(t, e.installRelease(t, apk, cert))
	// The versionName is not asserted: the APK on disk may have been built
	// with any (the one in build.gradle.kts or -Padbuddy.versionName); what is
	// tested is that the R8 build says hello with a version and a protocol the
	// host accepts.
	if h := l.Hellos()[0]; h.AppVersion == "" || h.Protocol < bridge.MinAppProtocol {
		t.Errorf("hello: appVersion %q, protocol %d; want a version and protocol >= %d", h.AppVersion, h.Protocol, bridge.MinAppProtocol)
	}
	if l.Info.ProtectedApps == nil {
		t.Error("device.info without protectedApps")
	}

	// Before the question: its dialog would cover the chat.
	l.openChat(t)
	const said = "Hello from the release check"
	l.send(t, bridge.TypeChatMessage, bridge.ChatMessage{ID: "m_release", Role: "assistant", Text: said, Attachments: []bridge.Attachment{}})
	e.waitNode(t, "the host's chat message", 15*time.Second, func(n uiNode) bool { return strings.Contains(n.Text, said) })

	r := l.uiRequest(t, "u_release", bridge.UIOpForeground, bridge.Empty{})
	if !r.OK && (r.Error == nil || r.Error.Code == bridge.CodeInvalid) {
		t.Errorf("ui.response to foreground: %+v", r)
	}

	l.send(t, bridge.TypeConfirmReq, bridge.ConfirmRequest{
		ID: "j_release", Level: "red", Title: "Restart the phone", Detail: "may restart",
		RawCommand: "adb shell reboot", ExpiresAt: time.Now().Add(2 * time.Minute).UTC(),
	})
	var rec string
	if !poll(20*time.Second, func() bool {
		rec = notificationRecord(e.shell(t, "dumpsys notification --noredact"), phoneapp.Package, "confirm")
		return strings.Contains(rec, "Restart the phone")
	}) {
		t.Errorf("no question on the confirm channel 20 s after the confirm.request:\n%s", rec)
	}
}
