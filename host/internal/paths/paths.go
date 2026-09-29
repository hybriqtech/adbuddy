// Package paths resolves where ADBuddy keeps its configuration and its data
// (private adb, IPC socket and secret) on each operating system (spec §8).
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Dirs are the two per-user directories ADBuddy owns.
type Dirs struct {
	Config string // config.toml, rules.yaml
	Data   string // platform-tools, IPC socket and secret
}

// ConfigFile is the path of config.toml.
func (d Dirs) ConfigFile() string { return filepath.Join(d.Config, "config.toml") }

// RulesFile is the default path of the user's rules.yaml.
func (d Dirs) RulesFile() string { return filepath.Join(d.Config, "rules.yaml") }

// Resolve computes the directories for goos. ADBUDDY_CONFIG_DIR and
// ADBUDDY_DATA_DIR override the defaults (tests and portable installs).
func Resolve(getenv func(string) string, goos, home string) (Dirs, error) {
	var d Dirs
	switch goos {
	case "windows":
		appData, localAppData := getenv("APPDATA"), getenv("LOCALAPPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		d = Dirs{Config: filepath.Join(appData, "ADBuddy"), Data: filepath.Join(localAppData, "ADBuddy")}
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support", "ADBuddy")
		d = Dirs{Config: base, Data: base}
	default:
		cfg, data := getenv("XDG_CONFIG_HOME"), getenv("XDG_DATA_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		d = Dirs{Config: filepath.Join(cfg, "adbuddy"), Data: filepath.Join(data, "adbuddy")}
	}
	if v := getenv("ADBUDDY_CONFIG_DIR"); v != "" {
		d.Config = v
	}
	if v := getenv("ADBUDDY_DATA_DIR"); v != "" {
		d.Data = v
	}
	if d.Config == "" || d.Data == "" {
		return Dirs{}, errors.New("cannot determine the ADBuddy directories")
	}
	return d, nil
}

// Default resolves the directories for the running process.
func Default(getenv func(string) string) (Dirs, error) {
	home, err := os.UserHomeDir()
	if err != nil && getenv("ADBUDDY_CONFIG_DIR") == "" {
		return Dirs{}, err
	}
	return Resolve(getenv, runtime.GOOS, home)
}
