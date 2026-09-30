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
	"io/fs"
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
	return manifestSized(archive, version, sum, int64(len(archive)))
}

func manifestSized(archive []byte, version [3]int, sum string, size int64) string {
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
</sdk:sdk-repository>`, HostOS(runtime.GOOS), version[0], version[1], version[2], size, sum)
}

type server struct {
	*httptest.Server
	hits    atomic.Int32
	zipHits atomic.Int32
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
			s.zipHits.Add(1)
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

// makeZipModes is makeZip with explicit modes (entries not in modes get none).
func makeZipModes(t *testing.T, entries map[string]string, modes map[string]fs.FileMode) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if m, ok := modes[name]; ok {
			h.SetMode(m)
		}
		w, err := zw.CreateHeader(h)
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

// setLimit changes a package limit for one test.
func setLimit(t *testing.T, p *int64, v int64) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func TestEnsureRefusesRedirectToAnotherScheme(t *testing.T) {
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		io.WriteString(w, "<sdk-repository/>")
	}))
	t.Cleanup(plain.Close)
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(tlsSrv.Close)
	in := Installer{DataDir: t.TempDir(), BaseURL: tlsSrv.URL + "/repo/", Client: tlsSrv.Client(), GOOS: runtime.GOOS, Log: io.Discard}
	if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("https→http redirect: %v", err)
	}
	if plainHits.Load() != 0 {
		t.Fatal("the http server was contacted")
	}
}

// fakeTransport answers every request with h, whatever the host.
type fakeTransport struct{ h http.Handler }

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func TestEnsureLogsMirrorOnlyForAnotherRepository(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, log := installer(t, s)
	if _, err := in.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "Using mirror " + s.URL + "/repo/: checksums come from the mirror"
	if !strings.Contains(log.String(), want) {
		t.Fatalf("log %q lacks %q", log.String(), want)
	}
	// Google's repository (served in memory, no network): no mirror notice.
	var glog bytes.Buffer
	g := Installer{DataDir: t.TempDir(), BaseURL: RepositoryURL, Client: &http.Client{Transport: fakeTransport{s.Config.Handler}}, GOOS: runtime.GOOS, Log: &glog}
	if _, err := g.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(glog.String(), "mirror") {
		t.Fatalf("log %q", glog.String())
	}
}

func TestEnsureRefusesUnreasonableSize(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	for _, size := range []int64{0, -1, 256<<20 + 1} {
		s := serve(t, manifestSized(archive, [3]int{37, 0, 1}, "", size), archive)
		in, _ := installer(t, s)
		if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "size") {
			t.Errorf("size %d: %v", size, err)
		}
		if s.zipHits.Load() != 0 {
			t.Errorf("size %d: archive downloaded", size)
		}
	}
}

func TestEnsureChecksumErrorsSayWhy(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestSized(archive, [3]int{37, 0, 1}, "", int64(len(archive))+10), archive)
	in, _ := installer(t, s)
	_, err := in.Ensure(context.Background())
	if !errors.Is(err, ErrChecksum) || !strings.Contains(err.Error(), fmt.Sprintf("got %d bytes", len(archive))) {
		t.Fatalf("short body: %v", err)
	}
	s = serve(t, manifestFor(archive, [3]int{37, 0, 1}, strings.Repeat("0", 40)), archive)
	in, _ = installer(t, s)
	_, err = in.Ensure(context.Background())
	if !errors.Is(err, ErrChecksum) || !strings.Contains(err.Error(), "sha1") {
		t.Fatalf("bad sha: %v", err)
	}
}

func TestEnsureEnforcesExtractionLimits(t *testing.T) {
	big := makeZip(t, map[string]string{
		"platform-tools/source.properties": "Pkg.Revision=37.0.1\n",
		"platform-tools/" + adbName():      strings.Repeat("a", 100),
	})
	t.Run("entry", func(t *testing.T) {
		setLimit(t, &maxEntryBytes, 50)
		s := serve(t, manifestFor(big, [3]int{37, 0, 1}, ""), big)
		in, _ := installer(t, s)
		if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("total", func(t *testing.T) {
		two := makeZip(t, map[string]string{
			"platform-tools/source.properties": "Pkg.Revision=37.0.1\n",
			"platform-tools/" + adbName():      strings.Repeat("a", 60),
			"platform-tools/fastboot":          strings.Repeat("b", 60),
		})
		setLimit(t, &maxTotalBytes, 100)
		s := serve(t, manifestFor(two, [3]int{37, 0, 1}, ""), two)
		in, _ := installer(t, s)
		if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("entries", func(t *testing.T) {
		setLimit(t, &maxEntries, 1)
		s := serve(t, manifestFor(big, [3]int{37, 0, 1}, ""), big)
		in, _ := installer(t, s)
		if _, err := in.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "entries") {
			t.Fatalf("%v", err)
		}
	})
}

func TestEnsureMakesADBExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no execute bit on Windows")
	}
	archive := makeZipModes(t, map[string]string{
		"platform-tools/source.properties": "Pkg.Revision=37.0.1\n",
		"platform-tools/adb":               "fake adb",
		"platform-tools/fastboot":          "fake fastboot",
	}, map[string]fs.FileMode{"platform-tools/fastboot": 0o755})
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, _ := installer(t, s)
	res, err := in.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"adb", "fastboot"} {
		st, err := os.Stat(filepath.Join(Dir(in.DataDir), name))
		if err != nil || st.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", name, st, err)
		}
	}
	// An adb without the execute bit is not a usable install.
	os.Chmod(res.ADB, 0o600)
	before := s.zipHits.Load()
	if res, err := in.Ensure(context.Background()); err != nil || !res.Installed || s.zipHits.Load() != before+1 {
		t.Fatalf("reinstall: %+v %v", res, err)
	}
}

// oldInstall leaves a working (older) platform-tools in dataDir.
func oldInstall(t *testing.T, dataDir string) {
	t.Helper()
	os.MkdirAll(Dir(dataDir), 0o700)
	os.WriteFile(filepath.Join(Dir(dataDir), "source.properties"), []byte("Pkg.Revision=35.0.2\n"), 0o600)
	os.WriteFile(filepath.Join(Dir(dataDir), adbName()), []byte("old adb"), 0o700)
}

func TestEnsureKeepsPreviousInstallWhenTheNewOneIsBad(t *testing.T) {
	good := goodArchive(t, "37.0.1")
	adb := "platform-tools/" + adbName()
	props := "platform-tools/source.properties"
	cases := map[string]struct {
		archive []byte
		sum     string
	}{
		"checksum":    {good, strings.Repeat("0", 40)},
		"zip slip":    {makeZip(t, map[string]string{props: "Pkg.Revision=37.0.1\n", adb: "x", "platform-tools/../../evil": "x"}), ""},
		"no adb":      {makeZip(t, map[string]string{props: "Pkg.Revision=37.0.1\n"}), ""},
		"too old":     {goodArchive(t, "36.0.0"), ""},
		"other":       {goodArchive(t, "37.0.2"), ""},
		"no revision": {makeZip(t, map[string]string{props: "Pkg.UserSrc=false\n", adb: "x"}), ""},
		"symlink":     {makeZipModes(t, map[string]string{props: "Pkg.Revision=37.0.1\n", adb: "/bin/sh"}, map[string]fs.FileMode{adb: fs.ModeSymlink | 0o777}), ""},
		"backslash":   {makeZip(t, map[string]string{props: "Pkg.Revision=37.0.1\n", adb: "x", `platform-tools/..\..\evil`: "x"}), ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := serve(t, manifestFor(c.archive, [3]int{37, 0, 1}, c.sum), c.archive)
			in, _ := installer(t, s)
			oldInstall(t, in.DataDir)
			if _, err := in.Ensure(context.Background()); err == nil {
				t.Fatal("bad archive accepted")
			}
			if v, _ := InstalledVersion(in.DataDir); v != "35.0.2" {
				t.Fatalf("version now %q", v)
			}
			if b, _ := os.ReadFile(ADBPath(in.DataDir, runtime.GOOS)); string(b) != "old adb" {
				t.Fatalf("adb now %q", b)
			}
		})
	}
}

func TestEnsureAcceptsDoubleDotsInsideNames(t *testing.T) {
	archive := makeZip(t, map[string]string{
		"platform-tools/source.properties": "Pkg.Revision=37.0.1\n",
		"platform-tools/" + adbName():      "fake adb",
		"platform-tools/lib64/libfoo..so":  "lib",
	})
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, _ := installer(t, s)
	if _, err := in.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRemovesLeftovers(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in, _ := installer(t, s)
	os.WriteFile(filepath.Join(in.DataDir, ".platform-tools-123.zip"), []byte("x"), 0o600)
	os.MkdirAll(filepath.Join(in.DataDir, ".platform-tools-new-456", "platform-tools"), 0o700)
	if _, err := in.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(in.DataDir)
	if len(entries) != 1 || entries[0].Name() != "platform-tools" {
		t.Fatalf("data dir: %v", entries)
	}
}

func TestEnsureDefaults(t *testing.T) {
	archive := goodArchive(t, "37.0.1")
	s := serve(t, manifestFor(archive, [3]int{37, 0, 1}, ""), archive)
	in := Installer{DataDir: t.TempDir(), BaseURL: s.URL + "/repo", GOOS: runtime.GOOS} // nil Log, no trailing slash
	if res, err := in.Ensure(context.Background()); err != nil || !res.Installed {
		t.Fatalf("%+v %v", res, err)
	}
}
