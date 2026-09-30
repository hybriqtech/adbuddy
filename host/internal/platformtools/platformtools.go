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
	"os"
	"path"
	"path/filepath"
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
	f, err := os.Open(filepath.Join(Dir(dataDir), "source.properties"))
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

// ErrChecksum means the download does not match the manifest.
var ErrChecksum = errors.New("platform-tools download does not match its checksum")

// Installer downloads and installs platform-tools.
type Installer struct {
	DataDir string
	BaseURL string // RepositoryURL, or a test server
	Client  *http.Client
	GOOS    string
	Log     io.Writer // progress for the person
}

// Result describes what Ensure did.
type Result struct {
	ADB       string `json:"adb"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"` // false: an acceptable version was already there
}

// Ensure installs platform-tools unless a version >= MinMajor is present.
func (in Installer) Ensure(ctx context.Context) (Result, error) {
	adb := ADBPath(in.DataDir, in.GOOS)
	have, err := InstalledVersion(in.DataDir)
	if err != nil {
		return Result{}, err
	}
	if have != "" && majorOf(have) >= MinMajor {
		if _, err := os.Stat(adb); err == nil {
			return Result{ADB: adb, Version: have}, nil
		}
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
	fmt.Fprintf(in.Log, "Downloading platform-tools %s (%.1f MB)…\n", rel.Version(), float64(arch.Size)/1e6)
	if err := os.MkdirAll(in.DataDir, 0o700); err != nil {
		return Result{}, err
	}
	zipPath, err := in.download(ctx, in.BaseURL+arch.URL, *arch)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(zipPath)
	if err := install(zipPath, in.DataDir); err != nil {
		return Result{}, err
	}
	got, err := InstalledVersion(in.DataDir)
	if err != nil {
		return Result{}, err
	}
	fmt.Fprintf(in.Log, "platform-tools %s installed in %s\n", got, Dir(in.DataDir))
	return Result{ADB: adb, Version: got, Installed: true}, nil
}

func (in Installer) client() *http.Client {
	if in.Client != nil {
		return in.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (in Installer) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := in.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response too large", url)
	}
	return data, nil
}

// download saves the archive to a temporary file and checks size and SHA-1.
func (in Installer) download(ctx context.Context, url string, a Archive) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := in.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
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
	if err == nil && (n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA1) {
		err = ErrChecksum
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// install extracts platform-tools/ from the zip into a fresh folder and swaps
// it in place of the old one. Entries outside platform-tools/ or escaping
// the folder make the whole install fail.
func install(zipPath, dataDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	staging, err := os.MkdirTemp(dataDir, ".platform-tools-new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, f := range zr.File {
		name := path.Clean(f.Name)
		if !strings.HasPrefix(name, "platform-tools/") && name != "platform-tools" || strings.Contains(name, "..") || path.IsAbs(name) {
			return fmt.Errorf("unexpected entry %q in the platform-tools archive", f.Name)
		}
		dst := filepath.Join(staging, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := extract(f, dst); err != nil {
			return err
		}
	}
	src := filepath.Join(staging, "platform-tools")
	if _, err := os.Stat(filepath.Join(src, "source.properties")); err != nil {
		return errors.New("the platform-tools archive has no source.properties")
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

func extract(f *zip.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := os.FileMode(0o600)
	if f.Mode()&0o111 != 0 {
		mode = 0o700
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(rc, 512<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
