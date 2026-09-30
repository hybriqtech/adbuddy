// Package config reads and writes config.toml (spec §8).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the content of config.toml. Every field is optional.
type Config struct {
	Agent          string   `toml:"agent"`           // default agent for `adbuddy run`
	ADBPath        string   `toml:"adb_path"`        // real adb; empty = ADBuddy's own platform-tools (installed by `adbuddy setup`)
	ADBServerPort  int      `toml:"adb_server_port"` // private adb server port (spec §3.2)
	Serial         string   `toml:"serial"`          // active device; empty = the only connected one
	Address        string   `toml:"address"`         // last IP:port the phone answered on (maybe over Tailscale); empty = none
	Workspace      string   `toml:"workspace"`       // agent environment; empty = ~/ADBuddy/default
	ConfirmTimeout Duration `toml:"confirm_timeout"` // how long a 🔴 confirmation waits
	Rules          string   `toml:"rules"`           // user rules.yaml; empty = <config dir>/rules.yaml
	Language       string   `toml:"language"`        // "en" or "es"; empty = from LANG
}

// Duration is a time.Duration written as text ("2m") in TOML.
type Duration struct{ time.Duration }

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Defaults.
const (
	DefaultADBServerPort  = 5038
	DefaultConfirmTimeout = 2 * time.Minute
)

// Default returns the configuration used when config.toml does not exist.
func Default() Config {
	return Config{ADBServerPort: DefaultADBServerPort, ConfirmTimeout: Duration{DefaultConfirmTimeout}}
}

// Load reads path. A missing file yields Default(). Unknown keys are errors,
// so a typo never silently falls back to a default.
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, err
	}
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return Config{}, fmt.Errorf("%s: unknown keys: %s", path, strings.Join(keys, ", "))
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Validate checks value ranges.
func (c Config) Validate() error {
	if c.ADBServerPort < 1 || c.ADBServerPort > 65535 {
		return fmt.Errorf("adb_server_port must be between 1 and 65535, got %d", c.ADBServerPort)
	}
	if c.ConfirmTimeout.Duration <= 0 || c.ConfirmTimeout.Duration > time.Hour {
		return fmt.Errorf("confirm_timeout must be between 1s and 1h, got %s", c.ConfirmTimeout)
	}
	if c.Address != "" && !validAddress(c.Address) {
		return fmt.Errorf("address must be an IP address and port (e.g. 100.64.0.5:41234), got %q", c.Address)
	}
	switch c.Language {
	case "", "en", "es":
	default:
		return fmt.Errorf("language must be \"en\" or \"es\", got %q", c.Language)
	}
	return nil
}

// Save writes c to path atomically, readable only by the user.
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// WorkspaceDir is the agent environment directory.
func (c Config) WorkspaceDir(home string) string {
	if c.Workspace != "" {
		return c.Workspace
	}
	return filepath.Join(home, "ADBuddy", "default")
}

// Lang is the language for rule descriptions: the configured one, else "es"
// when LC_ALL/LC_MESSAGES/LANG start with "es", else "en".
func (c Config) Lang(getenv func(string) string) string {
	if c.Language != "" {
		return c.Language
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "es") {
				return "es"
			}
			return "en"
		}
	}
	return "en"
}

// validAddress accepts only IP:port with a port in 1-65535.
func validAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if _, err := netip.ParseAddr(host); err != nil {
		return false
	}
	p, err := strconv.ParseUint(port, 10, 16)
	return err == nil && p >= 1
}
