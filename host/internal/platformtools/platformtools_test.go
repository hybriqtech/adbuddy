package platformtools

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// makeZip builds a platform-tools archive with the given entries.
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, content)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func manifestFor(archive []byte, version [3]int, sum string) string {
	if sum == "" {
		h := sha1.Sum(archive)
		sum = hex.EncodeToString(h[:])
	}
	return fmt.Sprintf(`<?xml version='1.0' encoding='utf-8'?>
<sdk:sdk-repository xmlns:sdk="http://schemas.android.com/sdk/android/repo/repository2/03" xmlns:common="http://schemas.android.com/repository/android/common/02" xmlns:generic="http://schemas.android.com/repository/android/generic/02" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <channel id="channel-0">stable</channel>
  <channel id="channel-2">dev</channel>
  <remotePackage path="platform-tools">
    <type-details xsi:type="generic:genericDetailsType"/>
    <revision><major>99</major><minor>0</minor><micro>0</micro></revision>
    <channelRef ref="channel-2"/>
    <archives><archive><complete><size>1</size><checksum type="sha1">00</checksum><url>preview.zip</url></complete><host-os>%[1]s</host-os></archive></archives>
  </remotePackage>
  <remotePackage path="platform-tools">
    <revision><major>%[2]d</major><minor>%[3]d</minor><micro>%[4]d</micro></revision>
    <channelRef ref="channel-0"/>
    <archives>
      <archive><complete><size>%[5]d</size><checksum type="sha1">%[6]s</checksum><url>platform-tools_test-%[1]s.zip</url></complete><host-os>%[1]s</host-os></archive>
      <archive><complete><size>5</size><checksum type="sha1">ff</checksum><url>other.zip</url></complete><host-os>nowhere</host-os></archive>
    </archives>
  </remotePackage>
  <remotePackage path="emulator"><revision><major>100</major><minor>0</minor><micro>0</micro></revision><channelRef ref="channel-0"/></remotePackage>
</sdk:sdk-repository>`, HostOS(runtime.GOOS), version[0], version[1], version[2], len(archive), sum)
}

type server struct {
	*httptest.Server
	hits atomic.Int32
}

func serve(t *testing.T, manifest string, archive []byte) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+ManifestName):
			io.WriteString(w, manifest)
		case strings.HasSuffix(r.URL.Path, ".zip"):
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func installer(t *testing.T, s *server) (Installer, *bytes.Buffer) {
	var log bytes.Buffer
	return Installer{DataDir: t.TempDir(), BaseURL: s.URL + "/repo/", GOOS: runtime.GOOS, Log: &log}, &log
}

func adbName() string {
	if runtime.GOOS == "windows" {
		return "adb.exe"
	}
	return "adb"
}

func goodArchive(t *testing.T, version string) []byte {
	return makeZip(t, map[string]string{
		"platform-tools/source.properties": "Pkg.UserSrc=false\nPkg.Revision=" + version + "\n",
		"platform-tools/" + adbName():      "fake adb binary",
		"platform-tools/lib64/libc++.so":   "lib",
	})
}

func TestParseManifestPicksStableRelease(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	rel, err := ParseManifest([]byte(manifestFor(archive, [3]int{37, 0, 1}, "")))
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version() != "37.0.1" || len(rel.Archives) != 2 || rel.Archives[0].HostOS != HostOS(runtime.GOOS) {
		t.Fatalf("%+v", rel)
	}
	if _, err := ParseManifest([]byte("<sdk-repository></sdk-repository>")); err == nil {
		t.Fatal("empty manifest accepted")
	}
}

func TestEnsureDownloadsVerifiesAndInstalls(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, log := installer(t, s)
	res, err := in.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Installed || res.Version != "37.0.1" || res.ADB != ADBPath(in.DataDir, runtime.GOOS) {
		t.Fatalf("%+v", res)
	}
	if b, err := os.ReadFile(res.ADB); err != nil || string(b) != "fake adb binary" {
		t.Fatalf("adb: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(Dir(in.DataDir), "lib64", "libc++.so")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "Downloading platform-tools 37.0.1") {
		t.Fatalf("log %q", log.String())
	}
	// Second run: already installed, no network.
	before := s.hits.Load()
	res, err = in.Ensure(context.Background())
	if err != nil || res.Installed || s.hits.Load() != before {
		t.Fatalf("second run: %+v %v (hits %d→%d)", res, err, before, s.hits.Load())
	}
	// No temporary files are left behind.
	entries, _ := os.ReadDir(in.DataDir)
	if len(entries) != 1 || entries[0].Name() != "platform-tools" {
		t.Fatalf("data dir: %v", entries)
	}
}

func TestEnsureReplacesAnOldVersion(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, _ := installer(t, s)
	os.MkdirAll(Dir(in.DataDir), 0o700)
	os.WriteFile(filepath.Join(Dir(in.DataDir), "source.properties"), []byte("Pkg.Revision=35.0.2\n"), 0o600)
	os.WriteFile(filepath.Join(Dir(in.DataDir), "stale"), []byte("x"), 0o600)
	res, err := in.Ensure(context.Background())
	if err != nil || !res.Installed || res.Version != "37.0.1" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(Dir(in.DataDir), "stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old install not replaced")
	}
}

func TestEnsureRejectsBadChecksum(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, strings.Repeat("0", 40)), archive)
	in, _ := installer(t, s)
	if _, err := in.Ensure(context.Background()); !errors.Is(err, ErrChecksum) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(Dir(in.DataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("something was installed")
	}
	entries, _ := os.ReadDir(in.DataDir)
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestEnsureRejectsEntriesOutsidePlatformTools(t *testing.T) {
	for _, bad := range []string{"../evil", "platform-tools/../../evil", "other/file", "/abs"} {
		archive := makeZip(t, map[string]string{
			"platform-tools/source.properties": "Pkg.Revision=37.0.1\n",
			bad:                                "x",
		})
		s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
		in, _ := installer(t, s)
		if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "unexpected entry") {
			t.Errorf("%q: %v", bad, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(in.DataDir), "evil")); err == nil {
			t.Errorf("%q escaped the data dir", bad)
		}
	}
}

func TestEnsureRefusesTooOldRelease(t *testing.T) {
	archive := goodArchive(t, "36.0.0")
	s := serve(t, manifestFor(archive, [3]int{36, 0, 0}, ""), archive)
	in, _ := installer(t, s)
	if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "needs 37") {
		t.Fatalf("%v", err)
	}
}
