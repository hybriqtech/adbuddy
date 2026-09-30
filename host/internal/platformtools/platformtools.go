// Package platformtools installs ADBuddy's own copy of Android platform-tools
// (spec §3.1 step 1). ADBuddy never uses the system adb: it needs adb 37 or
// newer for Wi-Fi adb 2.0.
//
// The archive comes from Google's SDK repository manifest, which lists each
// platform-tools release with its size and SHA-1: the download is checked
// against both before anything is extracted.
package platformtools

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RepositoryURL is Google's SDK repository.
const RepositoryURL = "https://dl.google.com/android/repository/"

// ManifestName is the repository manifest that lists platform-tools.
const ManifestName = "repository2-3.xml"

// MinMajor is the oldest platform-tools major version ADBuddy accepts.
const MinMajor = 37

// Limits on what a platform-tools archive may make ADBuddy download and write
// (variables so tests can lower them).
var (
	maxArchiveSize int64 = 256 << 20 // the zip itself
	maxEntryBytes  int64 = 256 << 20 // one extracted file
	maxTotalBytes  int64 = 1 << 30   // all extracted files together
	maxEntries     int64 = 10000
)

// Archive is one downloadable zip of a release.
type Archive struct {
	URL    string
	Size   int64
	SHA1   string
	HostOS string // windows, macosx, linux
}

// Release is a platform-tools version with its archives.
type Release struct {
	Major, Minor, Micro int
	Archives            []Archive
}

// Version is "major.minor.micro".
func (r Release) Version() string { return fmt.Sprintf("%d.%d.%d", r.Major, r.Minor, r.Micro) }

func (r Release) newer(o Release) bool {
	if r.Major != o.Major {
		return r.Major > o.Major
	}
	if r.Minor != o.Minor {
		return r.Minor > o.Minor
	}
	return r.Micro > o.Micro
}

type manifest struct {
	Channels []struct {
		ID   string `xml:"id,attr"`
		Name string `xml:",chardata"`
	} `xml:"channel"`
	Packages []struct {
		Path     string `xml:"path,attr"`
		Revision struct {
			Major int `xml:"major"`
			Minor int `xml:"minor"`
			Micro int `xml:"micro"`
		} `xml:"revision"`
		Channel struct {
			Ref string `xml:"ref,attr"`
		} `xml:"channelRef"`
		Archives []struct {
			Size     int64  `xml:"complete>size"`
			Checksum string `xml:"complete>checksum"`
			URL      string `xml:"complete>url"`
			HostOS   string `xml:"host-os"`
		} `xml:"archives>archive"`
	} `xml:"remotePackage"`
}

// ParseManifest returns the newest stable platform-tools release.
func ParseManifest(data []byte) (Release, error) {
	var m manifest
	if err := xml.Unmarshal(data, &m); err != nil {
		return Release{}, fmt.Errorf("repository manifest: %w", err)
	}
	stable := ""
	for _, c := range m.Channels {
		if strings.TrimSpace(c.Name) == "stable" {
			stable = c.ID
		}
	}
	if stable == "" {
		return Release{}, errors.New("repository manifest: no stable channel")
	}
	var best Release
	found := false
	for _, p := range m.Packages {
		if p.Path != "platform-tools" || p.Channel.Ref != stable {
			continue
		}
		r := Release{Major: p.Revision.Major, Minor: p.Revision.Minor, Micro: p.Revision.Micro}
		for _, a := range p.Archives {
			r.Archives = append(r.Archives, Archive{URL: a.URL, Size: a.Size, SHA1: strings.ToLower(strings.TrimSpace(a.Checksum)), HostOS: a.HostOS})
		}
		if !found || r.newer(best) {
			best, found = r, true
		}
	}
	if !found {
		return Release{}, errors.New("repository manifest: no stable platform-tools")
	}
	return best, nil
}

// HostOS maps GOOS to the manifest's host-os.
func HostOS(goos string) string {
	switch goos {
	case "windows":
		return "windows"
	case "darwin":
		return "macosx"
	}
	return "linux"
}

// Dir is where platform-tools is installed.
func Dir(dataDir string) string { return filepath.Join(dataDir, "platform-tools") }

// ADBPath is the adb binary inside Dir.
func ADBPath(dataDir, goos string) string {
	name := "adb"
	if goos == "windows" {
		name = "adb.exe"
	}
	return filepath.Join(Dir(dataDir), name)
}

// InstalledVersion reads Pkg.Revision from source.properties ("" if missing).
func InstalledVersion(dataDir string) (string, error) {
	return readRevision(filepath.Join(Dir(dataDir), "source.properties"))
}

func readRevision(file string) (string, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "Pkg.Revision="); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", sc.Err()
}

func majorOf(version string) int {
	n, _ := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	return n
}

// sameVersion reports whether version ("37.0.1", or "37.0" for "37.0.0")
// is the release r.
func sameVersion(version string, r Release) bool {
	parts := strings.Split(version, ".")
	if len(parts) > 3 {
		return false
	}
	var got [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return false
		}
		got[i] = n
	}
	return got == [3]int{r.Major, r.Minor, r.Micro}
}

// ErrChecksum means the download does not match the manifest.
var ErrChecksum = errors.New("platform-tools download does not match its checksum")

// Installer downloads and installs platform-tools.
type Installer struct {
	DataDir string
	BaseURL string // RepositoryURL (the default), or a mirror or test server
	Client  *http.Client
	GOOS    string    // runtime.GOOS by default
	Log     io.Writer // progress for the person (nil: discarded)
}

// Result describes what Ensure did.
type Result struct {
	ADB       string `json:"adb"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"` // false: an acceptable version was already there
}

func (in Installer) withDefaults() Installer {
	if in.BaseURL == "" {
		in.BaseURL = RepositoryURL
	}
	if !strings.HasSuffix(in.BaseURL, "/") {
		in.BaseURL += "/"
	}
	if in.GOOS == "" {
		in.GOOS = runtime.GOOS
	}
	if in.Log == nil {
		in.Log = io.Discard
	}
	return in
}

// Ensure installs platform-tools unless a version >= MinMajor is present.
func (in Installer) Ensure(ctx context.Context) (Result, error) {
	in = in.withDefaults()
	removeLeftovers(in.DataDir)
	adb := ADBPath(in.DataDir, in.GOOS)
	have, err := InstalledVersion(in.DataDir)
	if err != nil {
		return Result{}, err
	}
	if have != "" && majorOf(have) >= MinMajor && usable(adb) {
		return Result{ADB: adb, Version: have}, nil
	}
	if in.BaseURL != RepositoryURL {
		// The mirror serves both the manifest and the zip: its checksums only
		// catch corruption, not a malicious mirror.
		fmt.Fprintf(in.Log, "Using mirror %s: checksums come from the mirror\n", in.BaseURL)
	}
	data, err := in.get(ctx, in.BaseURL+ManifestName, 32<<20)
	if err != nil {
		return Result{}, err
	}
	rel, err := ParseManifest(data)
	if err != nil {
		return Result{}, err
	}
	if rel.Major < MinMajor {
		return Result{}, fmt.Errorf("the newest platform-tools is %s; ADBuddy needs %d or newer", rel.Version(), MinMajor)
	}
	var arch *Archive
	for i := range rel.Archives {
		if rel.Archives[i].HostOS == HostOS(in.GOOS) {
			arch = &rel.Archives[i]
		}
	}
	if arch == nil {
		return Result{}, fmt.Errorf("platform-tools %s has no archive for %s", rel.Version(), HostOS(in.GOOS))
	}
	if strings.Contains(arch.URL, "/") || strings.Contains(arch.URL, "..") {
		return Result{}, fmt.Errorf("unexpected archive name %q", arch.URL)
	}
	if arch.Size <= 0 || arch.Size > maxArchiveSize {
		return Result{}, fmt.Errorf("platform-tools %s: unexpected archive size %d bytes (the limit is %d)", rel.Version(), arch.Size, maxArchiveSize)
	}
	fmt.Fprintf(in.Log, "Downloading platform-tools %s (%.1f MB)…\n", rel.Version(), float64(arch.Size)/1e6)
	if err := os.MkdirAll(in.DataDir, 0o700); err != nil {
		return Result{}, err
	}
	zipPath, err := in.download(ctx, in.BaseURL+arch.URL, *arch)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(zipPath)
	if err := install(zipPath, in.DataDir, in.GOOS, rel); err != nil {
		return Result{}, err
	}
	got, err := InstalledVersion(in.DataDir)
	if err != nil {
		return Result{}, err
	}
	fmt.Fprintf(in.Log, "platform-tools %s installed in %s\n", got, Dir(in.DataDir))
	return Result{ADB: adb, Version: got, Installed: true}, nil
}

// usable reports whether adb is a regular file ADBuddy can run.
func usable(adb string) bool {
	st, err := os.Stat(adb)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode()&0o100 != 0
}

// removeLeftovers deletes what an interrupted install left in dataDir
// (.platform-tools-*.zip downloads and .platform-tools-new-* folders).
func removeLeftovers(dataDir string) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".platform-tools-") {
			os.RemoveAll(filepath.Join(dataDir, e.Name()))
		}
	}
}

// client follows redirects only within BaseURL's scheme: never from https
// to http.
func (in Installer) client() *http.Client {
	c := http.Client{Timeout: 10 * time.Minute}
	if in.Client != nil {
		c = *in.Client
	}
	scheme := "https"
	if u, err := url.Parse(in.BaseURL); err == nil {
		scheme = u.Scheme
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if req.URL.Scheme != scheme {
			return fmt.Errorf("refusing a redirect from %s to %s", scheme, req.URL.Scheme)
		}
		return nil
	}
	return &c
}

func (in Installer) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := in.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response too large", rawURL)
	}
	return data, nil
}

// download saves the archive to a temporary file and checks size and SHA-1.
func (in Installer) download(ctx context.Context, rawURL string, a Archive) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := in.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	f, err := os.CreateTemp(in.DataDir, ".platform-tools-*.zip")
	if err != nil {
		return "", err
	}
	h := sha1.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, a.Size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		if n != a.Size {
			err = fmt.Errorf("%w: got %d bytes, want %d", ErrChecksum, n, a.Size)
		} else if sum := hex.EncodeToString(h.Sum(nil)); sum != a.SHA1 {
			err = fmt.Errorf("%w: got sha1 %s, want %q", ErrChecksum, sum, a.SHA1)
		}
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// install extracts platform-tools/ from the zip into a fresh folder, checks
// it is the release rel with an adb, and swaps it in place of the old one.
// Entries outside platform-tools/, escaping the folder or other than files
// and folders make the whole install fail and leave the old one untouched.
func install(zipPath, dataDir, goos string, rel Release) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if int64(len(zr.File)) > maxEntries {
		return fmt.Errorf("the platform-tools archive has too many entries (%d, the limit is %d)", len(zr.File), maxEntries)
	}
	staging, err := os.MkdirTemp(dataDir, ".platform-tools-new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	src := filepath.Join(staging, "platform-tools")
	budget := maxTotalBytes
	for _, f := range zr.File {
		rest, ok := strings.CutPrefix(f.Name, "platform-tools/")
		if !ok && f.Name == "platform-tools" {
			rest, ok = "", true
		}
		mode := f.Mode()
		if !ok || strings.Contains(rest, `\`) || rest != "" && !filepath.IsLocal(filepath.FromSlash(rest)) ||
			!mode.IsDir() && (!mode.IsRegular() || rest == "") {
			return fmt.Errorf("unexpected entry %q in the platform-tools archive", f.Name)
		}
		dst := filepath.Join(src, filepath.FromSlash(rest))
		if mode.IsDir() {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return err
			}
			continue
		}
		n, err := extract(f, dst, budget)
		if err != nil {
			return err
		}
		budget -= n
	}
	if err := checkStaged(src, goos, rel); err != nil {
		return err
	}
	final := Dir(dataDir)
	old := final + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, old); err != nil {
			return fmt.Errorf("cannot replace %s (is adb still running? run `adb kill-server` with ADBuddy's adb): %w", final, err)
		}
	}
	if err := os.Rename(src, final); err != nil {
		os.Rename(old, final)
		return err
	}
	os.RemoveAll(old)
	return nil
}

// checkStaged checks the extracted folder before it replaces anything: an adb,
// and a source.properties with the manifest's version (>= MinMajor). Outside
// Windows, adb and fastboot are made executable whatever the zip said.
func checkStaged(src, goos string, rel Release) error {
	adb := filepath.Base(ADBPath("", goos))
	if st, err := os.Stat(filepath.Join(src, adb)); err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("the platform-tools archive has no %s", adb)
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{"adb", "fastboot"} {
			p := filepath.Join(src, name)
			if _, err := os.Stat(p); err == nil {
				if err := os.Chmod(p, 0o700); err != nil {
					return err
				}
			}
		}
	}
	v, err := readRevision(filepath.Join(src, "source.properties"))
	if err != nil {
		return err
	}
	switch {
	case v == "":
		return errors.New("the platform-tools archive has no Pkg.Revision in source.properties")
	case majorOf(v) < MinMajor:
		return fmt.Errorf("the platform-tools archive is version %q; ADBuddy needs %d or newer", v, MinMajor)
	case !sameVersion(v, rel):
		return fmt.Errorf("the platform-tools archive is version %q, but the manifest says %s", v, rel.Version())
	}
	return nil
}

// extract writes one file, refusing more than maxEntryBytes or the remaining
// budget of the whole archive. It returns the bytes written.
func extract(f *zip.File, dst string, budget int64) (int64, error) {
	limit := min(maxEntryBytes, budget)
	tooLarge := func() error {
		if limit < maxEntryBytes {
			return fmt.Errorf("the platform-tools archive is too large when extracted (limit %d bytes)", maxTotalBytes)
		}
		return fmt.Errorf("%q in the platform-tools archive is too large (limit %d bytes)", f.Name, maxEntryBytes)
	}
	if f.UncompressedSize64 > uint64(limit) {
		return 0, tooLarge()
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return 0, err
	}
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	mode := os.FileMode(0o600)
	if f.Mode()&0o111 != 0 {
		mode = 0o700
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if err == nil && n > limit {
		err = tooLarge()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return n, err
}
