package emutest

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Validation 3 of M6a, what API 34 can show: the Android behaviour the host's
// trust model relies on (spec M6a §6.2). pm path gives one base.apk under
// /data/app; stat -c '%s %i %Y' has the shape the host parses; base64 reads
// the APK back byte for byte (--no-incremental: adb does not keep reading the
// file after the check); an update in place (install -r) changes base.apk's
// stat (the cache key). The host's refusal of the §5 attack shapes is
// phoneapp/apksig_test.go's; installing a real attacker (API 30, targetSdk 29)
// and the two older images (API 30 and 33, not installed) stay for the manual
// check before the first release (docs/releasing.md).
func TestTheInstalledAppReadsBackByteForByte(t *testing.T) {
	e := newEmu(t)
	abs, want := readAPK(t, appAPK)
	e.installApp(t)

	// pm path: exactly one package:/data/app/<...>/base.apk.
	out := e.shell(t, shq.Join("pm", "path", phoneapp.Package))
	lines := nonEmptyLines(out)
	if len(lines) != 1 {
		t.Fatalf("pm path gave %d lines: %q", len(lines), out)
	}
	path, ok := strings.CutPrefix(lines[0], "package:")
	if !ok || !basePath(path) {
		t.Fatalf("pm path = %q, want one package:/data/app/<...>/base.apk", lines[0])
	}

	// stat -c '%s %i %Y': size, inode, mtime, as statKey parses it.
	stat := e.shell(t, shq.Join("stat", "-c", "%s %i %Y", path))
	size, inode, mtime, okStat := threeInts(stat)
	if !okStat {
		t.Fatalf("stat = %q, want three integers (size inode mtime)", stat)
	}
	if size != int64(len(want)) {
		t.Errorf("stat size %d, local APK %d bytes", size, len(want))
	}

	// base64: the APK read back is byte for byte the file installed.
	got := e.base64(t, path, size)
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Errorf("the APK read back (%d bytes) is not the one installed (%d bytes) from %s", len(got), len(want), abs)
	}

	// install -r keeps the data and changes base.apk's stat (the cache key).
	if out := e.run(t, "install", "-r", "--no-incremental", abs); !strings.Contains(out, "Success") {
		t.Fatalf("install -r: %s", out)
	}
	// mtime is whole seconds: wait for the inode or the mtime to move.
	var stat2 string
	if !poll(5*time.Second, func() bool {
		path2 := firstPath(e.shell(t, shq.Join("pm", "path", phoneapp.Package)))
		stat2 = e.shell(t, shq.Join("stat", "-c", "%s %i %Y", path2))
		_, i2, m2, ok := threeInts(stat2)
		return ok && (i2 != inode || m2 != mtime)
	}) {
		t.Errorf("stat %q unchanged after install -r (was inode %d mtime %d): the cache key would not move", stat2, inode, mtime)
	}
}

// Validation 3 of M6a, the provider's authority: Android installs no second
// app that claims tech.hybriq.adbuddy.bridge (INSTALL_FAILED_CONFLICTING_PROVIDER),
// even with ADBuddy's own provider disabled, so content://…bridge stays the
// verified app's (spec M6a §6.3, Me-4).
func TestASecondProviderOnTheAuthorityIsRefused(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	conflict := e.buildConflictAPK(t)
	e.uninstallAtCleanup(t, conflictPackage)

	refused := func(what string) {
		out, errOut, _, _ := e.adb.Output(e.ctx, "install", "-r", "--no-incremental", conflict)
		msg := strings.ReplaceAll(out+errOut, "\r", "")
		if !strings.Contains(msg, "INSTALL_FAILED_CONFLICTING_PROVIDER") {
			t.Errorf("installing a second provider on the authority %s: %q", what, strings.TrimSpace(msg))
		}
	}
	refused("with ADBuddy's provider enabled")

	// With ADBuddy's provider disabled the authority is still taken.
	e.atCleanup(t, "shell", shq.Join("pm", "enable", providerComponent))
	e.shell(t, shq.Join("pm", "disable", providerComponent))
	refused("with ADBuddy's provider disabled")
}

// base64 reads the n bytes of path on the emulator and decodes them.
func (e *emu) base64(t *testing.T, path string, n int64) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, pullOnEmu)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", shq.Join("base64", path))
	if err != nil || code != 0 {
		t.Fatalf("base64 %s: exit %d, %v: %s", path, code, err, strings.TrimSpace(errOut))
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out), ""))
	if err != nil {
		t.Fatalf("decode base64 of %s: %v", path, err)
	}
	if int64(len(data)) != n {
		t.Fatalf("base64 of %s gave %d bytes, stat said %d", path, len(data), n)
	}
	return data
}

// pullOnEmu bounds reading the APK over the emulator's adb (local, fast).
const pullOnEmu = 2 * time.Minute

const (
	conflictPackage   = "tech.hybriq.adbuddy.conflict"
	providerComponent = phoneapp.Package + "/.bridge.TokenProvider"
)

// conflictManifest declares a provider on ADBuddy's authority, in another
// package. hasCode="false": no dex, so aapt2 alone builds it.
const conflictManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="` + conflictPackage + `">
  <application android:hasCode="false">
    <provider android:name="android.content.ContentProvider" android:authorities="` + phoneapp.Authority + `" android:exported="false"/>
  </application>
</manifest>
`

// buildConflictAPK builds and signs (throwaway key, never the release one) a
// minimal APK that claims ADBuddy's provider authority, or skips without the
// SDK tools.
func (e *emu) buildConflictAPK(t *testing.T) string {
	t.Helper()
	tl := findTools(t)
	dir := t.TempDir()
	manifest := filepath.Join(dir, "AndroidManifest.xml")
	if err := os.WriteFile(manifest, []byte(conflictManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	unsigned := filepath.Join(dir, "conflict-unsigned.apk")
	tl.run(t, nil, tl.aapt2, "link", "-o", unsigned, "--manifest", manifest, "-I", tl.androidJar, "--min-sdk-version", "30")
	signed := filepath.Join(dir, "conflict.apk")
	ks, pass := tl.newKey(t, dir, "emutest-conflict")
	tl.sign(t, unsigned, signed, ks, pass, "--v1-signing-enabled", "false", "--v2-signing-enabled", "true", "--v3-signing-enabled", "true")
	return signed
}

// nonEmptyLines splits out into its non-empty, trimmed lines.
func nonEmptyLines(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// basePath reports whether p is a /data/app/<...>/base.apk path.
func basePath(p string) bool {
	return strings.HasPrefix(p, "/data/app/") && strings.HasSuffix(p, "/base.apk") && !strings.Contains(p, "..")
}

// firstPath returns the first package: path of pm path output.
func firstPath(out string) string {
	for _, l := range nonEmptyLines(out) {
		if p, ok := strings.CutPrefix(l, "package:"); ok {
			return p
		}
	}
	return ""
}

// threeInts parses "a b c" into three int64 (stat -c '%s %i %Y').
func threeInts(s string) (a, b, c int64, ok bool) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if a, err = strconv.ParseInt(f[0], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if b, err = strconv.ParseInt(f[1], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	if c, err = strconv.ParseInt(f[2], 10, 64); err != nil {
		return 0, 0, 0, false
	}
	return a, b, c, true
}
