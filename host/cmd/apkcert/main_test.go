package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
)

func TestPrintsEachCertificateOnce(t *testing.T) {
	a, b := apktest.Cert("a"), apktest.Cert("b")
	p := filepath.Join(t.TempDir(), "x.apk")
	apk := apktest.APK(
		apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: a}}},
		apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: a}, {Cert: b}}})
	if err := os.WriteFile(p, apk, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{p}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	want := []string{apktest.SHA256(a), apktest.SHA256(b)}
	if want[0] > want[1] {
		want[0], want[1] = want[1], want[0]
	}
	if got := strings.Fields(out.String()); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFailsOnABadAPK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.apk")
	os.WriteFile(p, apktest.APK(), 0o600)
	var out, errb bytes.Buffer
	if code := run([]string{p}, &out, &errb); code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "no APK Signing Block") {
		t.Fatalf("exit %d out %q err %q", code, out.String(), errb.String())
	}
	if code := run([]string{filepath.Join(t.TempDir(), "none.apk")}, &out, &errb); code != 1 {
		t.Fatalf("missing file: exit %d", code)
	}
}

func TestPinned(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-pinned"}, &out, &errb)
	if phoneapp.SigningCertSHA256 == "" {
		if code != 1 || !strings.Contains(errb.String(), "pins no signing certificate") {
			t.Fatalf("exit %d err %q", code, errb.String())
		}
		return
	}
	if code != 0 || strings.TrimSpace(out.String()) != phoneapp.SigningCertSHA256 {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage") {
			t.Fatalf("%q: exit %d", args, code)
		}
	}
}
