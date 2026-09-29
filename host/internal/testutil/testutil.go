// Package testutil builds the helper binaries used by integration tests.
package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Exe adds ".exe" on Windows.
func Exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Build compiles the package pkg (an import path of this module) into
// dir/name and returns the binary path.
func Build(pkg, dir, name string) (string, error) {
	out := filepath.Join(dir, Exe(name))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %v\n%s", pkg, err, b)
	}
	return out, nil
}

// FakeRule is one answer of the fake adb.
type FakeRule struct {
	Match     string `json:"match"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Exit      int    `json:"exit,omitempty"`
	EchoStdin bool   `json:"echoStdin,omitempty"`
}

// WriteScript writes the fake adb script and returns its path.
func WriteScript(dir string, rules ...FakeRule) (string, error) {
	b, err := json.Marshal(map[string]any{"rules": rules})
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "fakeadb.json")
	return p, os.WriteFile(p, b, 0o600)
}

// FakeCall is one logged call of the fake adb.
type FakeCall struct {
	Args  []string `json:"args"`
	Port  string   `json:"port"`
	Cwd   string   `json:"cwd"`
	Stdin string   `json:"stdin"`
}

// Joined returns the arguments without "-s SERIAL", joined by spaces.
func (c FakeCall) Joined() string {
	a := c.Args
	if len(a) >= 2 && a[0] == "-s" {
		a = a[2:]
	}
	return strings.Join(a, " ")
}

// ReadLog returns the calls logged by the fake adb (none if the log is missing).
func ReadLog(path string) ([]FakeCall, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var calls []FakeCall
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c FakeCall
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, sc.Err()
}
