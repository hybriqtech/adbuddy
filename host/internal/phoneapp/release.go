package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ReleaseURL is where the latest APK of the app is published (spec §3.1 step 5).
const ReleaseURL = "https://github.com/hybriqtech/adbuddy/releases/latest/download/"

// APKName is the release asset; APKName+".sha256" holds its SHA-256.
const APKName = "adbuddy.apk"

// maxAPK bounds the download.
const maxAPK = 200 << 20

// ErrNoRelease means there is no published APK yet.
var ErrNoRelease = errors.New("no published release of the app")

// ErrChecksum means the downloaded APK does not match its published SHA-256.
var ErrChecksum = errors.New("the APK does not match its SHA-256")

// Download fetches the latest APK into dir, checks it against the published
// SHA-256 and returns its path. base is ReleaseURL (or a test server).
// Without a client or a deadline in ctx it takes at most downloadTimeout.
func Download(ctx context.Context, client *http.Client, base, dir string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, downloadTimeout)
		defer cancel()
	}
	sum, err := get(ctx, client, base+APKName+".sha256", 1024)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", fmt.Errorf("unexpected %s.sha256: %q", APKName, sum)
	}
	want := strings.ToLower(fields[0])
	data, err := get(ctx, client, base+APKName, maxAPK)
	if err != nil {
		return "", err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return "", ErrChecksum
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, APKName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", url, limit)
	}
	return data, nil
}

// HostName is the PC's name as the app shows it: letters, digits, spaces,
// dots, dashes and underscores only, at most 40 characters.
func HostName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if b.Len() >= 40 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		return s
	}
	return "PC"
}
