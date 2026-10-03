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
	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
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
	ChatPort       int      `toml:"chat_port"`       // port of the app channel on 127.0.0.1 (PC and phone); 0 = chosen on first use
	TailscaleIP    string   `toml:"tailscale_ip"`    // the phone's Tailscale IPv4, as its app reported it; empty = unknown
	TailscalePort  int      `toml:"tailscale_port"`  // wireless debugging port, from the host's connection to the phone (its ip:port or its mDNS service); 0 = unknown
	RemoteUntil    string   `toml:"remote_until"`    // RFC 3339: remote mode (adb tcpip 5555) is on until then; empty = off
	AgentModel     string   `toml:"agent_model"`     // model passed to Claude Code (--model) by `adbuddy serve` only (`adbuddy run` takes it after --); empty = the one the user configured in it
	AgentEffort    string   `toml:"agent_effort"`    // effort passed to Claude Code (--effort) by `adbuddy serve` only; empty = the user's own
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
	if c.ChatPort != 0 && (c.ChatPort < 1024 || c.ChatPort > 65535) {
		return fmt.Errorf("chat_port must be between 1024 and 65535, got %d", c.ChatPort)
	}
	if c.TailscaleIP != "" && !ValidTailscaleIP(c.TailscaleIP) {
		return fmt.Errorf("tailscale_ip must be an IPv4 address in 100.64.0.0/10, got %q", c.TailscaleIP)
	}
	if c.TailscalePort != 0 && (c.TailscaleIP == "" || c.TailscalePort < 1 || c.TailscalePort > 65535) {
		return fmt.Errorf("tailscale_port needs tailscale_ip and must be between 1 and 65535, got %d", c.TailscalePort)
	}
	if c.RemoteUntil != "" {
		if _, err := time.Parse(time.RFC3339, c.RemoteUntil); err != nil {
			return fmt.Errorf("remote_until must be an RFC 3339 time (2026-10-02T09:00:00Z), got %q", c.RemoteUntil)
		}
	}
	if c.AgentModel != "" && !agent.ValidModel(c.AgentModel) {
		return fmt.Errorf("agent_model is not a model name: %q", c.AgentModel)
	}
	if c.AgentEffort != "" && !agent.ValidEffort(c.AgentEffort) {
		return fmt.Errorf("agent_effort must be one of low, medium, high, xhigh, max, got %q", c.AgentEffort)
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

// Lang is the language of the host's texts as the environment and this
// config give it (i18n.Resolve): ADBUDDY_LANG, then language, then
// LC_ALL, LC_MESSAGES and LANG, else "en". It does not ask the system; the
// command line's entry point does (i18n.Detect), so that serve and its
// daemon never disagree with the help.
func (c Config) Lang(getenv func(string) string) string {
	return string(i18n.Resolve(getenv, c.Language))
}

// ReadLanguage is the "language" of the config.toml at path, "" when the file
// is missing, unreadable or does not say. It ignores every other key and
// every error: a broken configuration must not keep --help or an error
// message from appearing in the right language.
func ReadLanguage(path string) string {
	var c struct {
		Language string `toml:"language"`
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return ""
	}
	switch c.Language {
	case "en", "es":
		return c.Language
	}
	return ""
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

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// ValidTailscaleIP accepts only an IPv4 address in Tailscale's CGNAT range.
func ValidTailscaleIP(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4() && cgnat.Contains(a)
}

// RemoteUntilTime is remote_until as a time; zero when it is empty or unreadable.
func (c Config) RemoteUntilTime() time.Time {
	t, err := time.Parse(time.RFC3339, c.RemoteUntil)
	if err != nil {
		return time.Time{}
	}
	return t
}
