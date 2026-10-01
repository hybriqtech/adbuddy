package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

const (
	phoneSerial = "adb-AAAA1111-bbb._adb-tls-connect._tcp"
	tcpPortRule = `^shell getprop service\.adb\.tcp\.port$`
	phoneOnline = "List of devices attached\n" + phoneSerial + "\tdevice\n"
	phoneGone   = "List of devices attached\n"
)

// invariantRunner points the fake adb at rules and returns a runner for the
// phone and the fake adb's log.
func invariantRunner(t *testing.T, rules ...testutil.FakeRule) (adb.Runner, string) {
	t.Helper()
	bin := fakeADB(t)
	dir := t.TempDir()
	script, err := testutil.WriteScript(dir, rules...)
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "adb.log")
	t.Setenv("FAKEADB_SCRIPT", script)
	t.Setenv("FAKEADB_LOG", log)
	return adb.Runner{Path: bin, ServerPort: 5038, Serial: phoneSerial}, log
}

func loggedCalls(t *testing.T, log string) []string {
	t.Helper()
	calls, err := testutil.ReadLog(log)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range calls {
		out = append(out, c.Joined())
	}
	return out
}

func errorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// cancelWhen cancels once buf contains s: the test stops the command at a
// known step instead of after a guessed time.
func cancelWhen(t *testing.T, buf *syncBuffer, s string, cancel context.CancelFunc) {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			if strings.Contains(buf.String(), s) {
				cancel()
				return
			}
			select {
			case <-done:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
}

// After `adb usb` the host waits for adbd to drop, then for the phone to come
// back, and checks again that the port is closed.
func TestApplyInvariant(t *testing.T) {
	setVar(t, &usbDropWait, 300*time.Millisecond)
	setVar(t, &invariantPoll, 20*time.Millisecond)
	getprop := "shell getprop service.adb.tcp.port"

	t.Run("closed", func(t *testing.T) {
		r, log := invariantRunner(t, testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"})
		var errb strings.Builder
		if err := applyInvariant(context.Background(), r, time.Second, &errb, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		if got := loggedCalls(t, log); strings.Join(got, "|") != getprop || errb.Len() != 0 {
			t.Fatalf("calls %q, stderr %q", got, errb.String())
		}
	})

	t.Run("keeps the remote port", func(t *testing.T) {
		r, log := invariantRunner(t, testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n"})
		if err := applyInvariant(context.Background(), r, time.Second, &strings.Builder{}, "adbuddy serve", "5555"); err != nil {
			t.Fatal(err)
		}
		if got := loggedCalls(t, log); strings.Join(got, "|") != getprop {
			t.Fatalf("calls %q: `adb usb` must not run", got)
		}
	})

	t.Run("reconnects", func(t *testing.T) {
		r, log := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n", Times: 1},
			testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			testutil.FakeRule{Match: "^usb$", Stdout: "restarting in USB mode\n"},
			// Still listed right after `adb usb`, then gone, then back.
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		var errb strings.Builder
		if err := applyInvariant(context.Background(), r, time.Second, &errb, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		want := []string{getprop, "usb", "devices", "devices", "devices", getprop}
		if got := loggedCalls(t, log); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("calls %q, want %q", got, want)
		}
		if !strings.Contains(errb.String(), "adb usb") {
			t.Fatalf("stderr %q", errb.String())
		}
	})

	t.Run("never drops", func(t *testing.T) {
		r, _ := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n", Times: 1},
			testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		start := time.Now()
		if err := applyInvariant(context.Background(), r, time.Second, &strings.Builder{}, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d < usbDropWait {
			t.Fatalf("did not give adbd %s to drop (took %s)", usbDropWait, d)
		}
	})

	t.Run("port stays open", func(t *testing.T) {
		r, log := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n"},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		err := applyInvariant(context.Background(), r, time.Second, &strings.Builder{}, "adbuddy serve", "")
		if errorCode(err) != "DEVICE_PORT_OPEN" || !strings.Contains(AsError(err).Fix, "adbuddy serve") {
			t.Fatalf("%v", err)
		}
		// The second check only reads the port: adbd is not restarted again.
		got := loggedCalls(t, log)
		usb := 0
		for _, c := range got {
			if c == "usb" {
				usb++
			}
		}
		if got[len(got)-1] != getprop || usb != 1 {
			t.Fatalf("calls %q: want one `adb usb` and the port checked again", got)
		}
	})

	// An `adb devices` that hangs while adbd drops is cut at usbDropWait.
	t.Run("adb hangs", func(t *testing.T) {
		r, _ := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n", Times: 1},
			testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone, SleepMs: 20000, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		start := time.Now()
		if err := applyInvariant(context.Background(), r, time.Second, &strings.Builder{}, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Fatalf("waited %s for a hanging adb", d)
		}
	})

	t.Run("never comes back", func(t *testing.T) {
		r, _ := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n", Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone},
		)
		err := applyInvariant(context.Background(), r, 100*time.Millisecond, &strings.Builder{}, "adbuddy setup", "")
		if errorCode(err) != "DEVICE_NOT_FOUND" || ExitCode(err) != ExitTransient {
			t.Fatalf("%v", err)
		}
	})
}

// Ctrl+C while serve waits for the phone after `adb usb` exits with 130.
func TestServeInterruptedDuringTheInvariant(t *testing.T) {
	isolate(t)
	invariantRunner(t,
		testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline, Times: 1},
		testutil.FakeRule{Match: "^devices$", Stdout: phoneGone},
		testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n"},
		testutil.FakeRule{Match: "^usb$"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb syncBuffer
	cancelWhen(t, &errb, "Closed adb's network port", cancel)
	done := make(chan int, 1)
	go func() {
		done <- mainContext(ctx, []string{"serve", "--adb", fakeADB(t), "--workspace", filepath.Join(t.TempDir(), "ws"), "--confirm", "deny"}, strings.NewReader(""), &out, &errb)
	}()
	select {
	case code := <-done:
		if code != ExitInterrupt || !strings.Contains(errb.String(), "INTERRUPTED") || strings.Contains(errb.String(), "DEVICE_") {
			t.Fatalf("exit %d, stderr %q", code, errb.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
	}
}

// serialnoRule answers ro.serialno as the phone of phoneSerial does.
var serialnoRule = testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"}

// guidCases are a phone saved by its mDNS name that adb lists under another
// serial: its address, after a fallback `adb connect`, or its name renamed
// by mDNS after a clash.
var guidCases = map[string]struct {
	rules  []testutil.FakeRule
	active string
}{
	"address": {rules: []testutil.FakeRule{
		{Match: "^devices$", Stdout: "List of devices attached\n192.0.2.10:41000\tdevice\n"},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\nadb-AAAA1111-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41000\n"},
		{Match: `^connect 192\.0\.2\.10:41000$`, Stdout: "already connected to 192.0.2.10:41000\n"},
		serialnoRule,
	}, active: "192.0.2.10:41000"},
	"renamed": {rules: []testutil.FakeRule{
		{Match: "^devices$", Stdout: "List of devices attached\nadb-AAAA1111-bbb (2)._adb-tls-connect._tcp\tdevice\n"},
		serialnoRule,
	}, active: "adb-AAAA1111-bbb (2)._adb-tls-connect._tcp"},
}

// phoneCallsUse reports whether every call to the phone (shell, usb) went
// to serial.
func phoneCallsUse(t *testing.T, log, serial string) bool {
	t.Helper()
	calls, err := testutil.ReadLog(log)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range calls {
		if len(c.Args) > 0 && (c.Args[0] == "shell" || c.Args[0] == "usb") {
			return false // no -s
		}
		if len(c.Args) >= 3 && c.Args[0] == "-s" {
			if c.Args[1] != serial {
				return false
			}
			n++
		}
	}
	return n > 0
}

// serve finds the phone saved by its mDNS name under the serial adb lists
// it with, and uses that serial for the session. The journal and the agent
// keep knowing it by its mDNS name (final review of M1b, I1).
func TestServeFindsThePhoneByItsGuid(t *testing.T) {
	setVar(t, &deviceWait, 3*time.Second)
	setVar(t, &autoConnectWait, 0)
	for name, c := range guidCases {
		t.Run(name, func(t *testing.T) {
			data := isolate(t)
			cfg := config.Default()
			cfg.Serial = phoneSerial
			if err := config.Save(filepath.Join(os.Getenv("ADBUDDY_CONFIG_DIR"), "config.toml"), cfg); err != nil {
				t.Fatal(err)
			}
			_, log := invariantRunner(t, append(c.rules, testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"})...)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out, errb syncBuffer
			var ping ipc.Ping
			var pingErr error
			pinged := make(chan struct{})
			serving := make(chan struct{})
			cancelWhen(t, &errb, "ADBuddy is serving", func() { close(serving) })
			go func() {
				defer close(pinged)
				defer cancel()
				select {
				case <-serving:
					ping, pingErr = ipc.PingDaemon(context.Background(), data)
				case <-ctx.Done():
				}
			}()
			done := make(chan int, 1)
			go func() {
				done <- mainContext(ctx, []string{"serve", "--adb", fakeADB(t), "--workspace", filepath.Join(t.TempDir(), "ws"), "--confirm", "deny"}, strings.NewReader(""), &out, &errb)
			}()
			select {
			case code := <-done:
				if code != 0 || !strings.Contains(errb.String(), "ADBuddy is serving "+c.active+" ") {
					t.Fatalf("exit %d, stderr %q", code, errb.String())
				}
			case <-time.After(15 * time.Second):
				t.Fatal("serve did not stop")
			}
			<-pinged
			if pingErr != nil || ping.Serial != c.active || ping.Identity != phoneSerial {
				t.Fatalf("ping %+v (%v): want serial %s and identity %s", ping, pingErr, c.active, phoneSerial)
			}
			if !phoneCallsUse(t, log, c.active) {
				t.Fatalf("calls to the phone not sent to %s: %q", c.active, loggedCalls(t, log))
			}
		})
	}
}
