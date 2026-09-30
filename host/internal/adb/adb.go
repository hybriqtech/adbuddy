// Package adb runs the real adb binary, always against ADBuddy's private adb
// server (ANDROID_ADB_SERVER_PORT) and the active device (-s).
package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Runner runs the real adb.
type Runner struct {
	Path       string // adb executable
	ServerPort int    // private adb server port; 0 = adb's default
	Serial     string // active device; empty for host-only commands
}

// Command builds the adb process for args in dir (the shim's working
// directory, so relative paths in push/pull/install resolve like the agent expects).
func (r Runner) Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := args
	if r.Serial != "" {
		full = append([]string{"-s", r.Serial}, args...)
	}
	cmd := exec.CommandContext(ctx, r.Path, full...)
	cmd.Dir = dir
	cmd.Env = r.env()
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

func (r Runner) env() []string {
	var env []string
	for _, kv := range os.Environ() {
		k := kv[:max(strings.IndexByte(kv, '='), 0)]
		if isolated(k) {
			continue
		}
		env = append(env, kv)
	}
	if r.ServerPort > 0 {
		env = append(env, "ANDROID_ADB_SERVER_PORT="+strconv.Itoa(r.ServerPort))
	}
	return env
}

// Run runs adb with the given streams and returns its exit code. err is only
// set when adb could not be started or was killed.
//
// stdin is copied through a pipe of Run's own: os/exec would otherwise wait
// for its copying goroutine, and a caller's stdin that never reaches EOF (a
// pipe nobody closes) would keep Run waiting after adb has exited. The copy
// is abandoned when adb exits; it ends by itself when the read returns.
func (r Runner) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := r.Command(ctx, dir, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	var in, feed *os.File
	if stdin != nil {
		var err error
		if in, feed, err = os.Pipe(); err != nil {
			return -1, err
		}
		cmd.Stdin = in
	}
	if err := cmd.Start(); err != nil {
		if in != nil {
			in.Close()
			feed.Close()
		}
		return -1, err
	}
	if in != nil {
		in.Close() // adb has its own copy
		go func() {
			_, _ = io.Copy(feed, stdin)
			feed.Close()
		}()
	}
	err := cmd.Wait()
	if feed != nil {
		feed.Close() // a copy still waiting on stdin fails its next write
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case ctx.Err() != nil:
		return -1, err
	case errors.As(err, &ee):
		return ee.ExitCode(), nil
	case cmd.ProcessState != nil && cmd.ProcessState.Exited():
		// exec.ErrWaitDelay: adb exited but something it started kept
		// stdout or stderr open; adb's own code is still the answer.
		return cmd.ProcessState.ExitCode(), nil
	default:
		return -1, err
	}
}

// Output runs adb and returns stdout, stderr and the exit code.
func (r Runner) Output(ctx context.Context, args ...string) (string, string, int, error) {
	var out, errb bytes.Buffer
	code, err := r.Run(ctx, "", args, nil, &out, &errb)
	return out.String(), errb.String(), code, err
}

// Shell implements precheck.Device.
func (r Runner) Shell(ctx context.Context, command string) (string, int, error) {
	out, _, code, err := r.Output(ctx, "shell", command)
	return out, code, err
}

// Pull implements precheck.Device.
func (r Runner) Pull(ctx context.Context, remote, local string) error {
	_, errOut, code, err := r.Output(ctx, "pull", remote, local)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("adb pull: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Device is one line of `adb devices`.
type Device struct {
	Serial string
	State  string // device, offline, unauthorized…
}

// ParseDevices parses the output of `adb devices` ("serial\tstate" lines).
// It splits on the tab, since an mDNS serial may have a space
// ("adb-<guid> (2)._adb-tls-connect._tcp").
func ParseDevices(out string) []Device {
	var ds []Device
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		serial, rest, ok := strings.Cut(line, "\t")
		state := strings.Fields(rest)
		if ok && serial != "" && len(state) > 0 {
			ds = append(ds, Device{Serial: serial, State: state[0]})
		}
	}
	return ds
}

// Online lists the devices in state "device".
func (r Runner) Online(ctx context.Context) ([]Device, error) {
	host := r
	host.Serial = ""
	out, errOut, code, err := host.Output(ctx, "devices")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("adb devices: %s", strings.TrimSpace(errOut))
	}
	var online []Device
	for _, d := range ParseDevices(out) {
		if d.State == "device" {
			online = append(online, d)
		}
	}
	return online, nil
}

// ErrNoDevice and ErrManyDevices explain why the active device is unknown.
var (
	ErrNoDevice    = errors.New("no phone connected")
	ErrManyDevices = errors.New("more than one phone connected")
)

// WaitSerial returns want if it is online, or the only online device when
// want is empty, polling until timeout (the private adb server needs a few
// seconds to reconnect paired phones found by mDNS).
func (r Runner) WaitSerial(ctx context.Context, want string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		online, err := r.Online(ctx)
		if err != nil {
			return "", err
		}
		for _, d := range online {
			if want != "" && d.Serial == want {
				return want, nil
			}
		}
		if want == "" && len(online) == 1 {
			return online[0].Serial, nil
		}
		if want == "" && len(online) > 1 {
			return "", ErrManyDevices
		}
		if time.Now().After(deadline) {
			return "", ErrNoDevice
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// isolated reports whether the variable k could send adb to another server or
// device than ADBuddy's private one: the device (ANDROID_SERIAL) and the
// server (port, address, socket). All of them are dropped from the inherited
// environment; only the configured port is set again.
func isolated(k string) bool {
	for _, name := range []string{"ANDROID_SERIAL", "ANDROID_ADB_SERVER_PORT", "ANDROID_ADB_SERVER_ADDRESS", "ADB_SERVER_SOCKET"} {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}
