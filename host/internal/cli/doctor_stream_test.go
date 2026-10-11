package cli

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// stampWriter notes the instant of every Write.
type stampWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	writes []stampedWrite
}

type stampedWrite struct {
	at   time.Time
	text string
}

func (w *stampWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, stampedWrite{time.Now(), string(p)})
	return w.buf.Write(p)
}

func (w *stampWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// slowLast is a healthy phone whose last adb check (the phone app's) is slow.
func slowLast() []testutil.FakeRule {
	return doctorRules(testutil.FakeRule{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, SleepMs: 1500,
		Stdout: "Packages:\n    versionName=0.2.0\n      android.permission.WRITE_SECURE_SETTINGS: granted=true\n"})
}

func TestDoctorStreamsEachCheckAsItEnds(t *testing.T) {
	doctorEnv(t, slowLast())
	w := &stampWriter{}
	var errb bytes.Buffer
	code := mainContext(context.Background(), []string{"doctor"}, strings.NewReader(""), w, &errb)
	if code != 0 {
		t.Fatalf("exit %d %q", code, w.String())
	}
	var configAt, lastAt time.Time
	for _, wr := range w.writes {
		if configAt.IsZero() && strings.Contains(wr.text, "config:") {
			configAt = wr.at
		}
		lastAt = wr.at
	}
	if configAt.IsZero() || lastAt.Sub(configAt) < time.Second {
		t.Fatalf("config line not written before the slow check ended (%v)", lastAt.Sub(configAt))
	}
	// The same text as the batch path (--fix prints it whole at the end).
	var batch bytes.Buffer
	if c := mainContext(context.Background(), []string{"doctor", "--fix"}, strings.NewReader(""), &batch, &errb); c != code {
		t.Fatalf("exit %d vs %d", c, code)
	}
	if batch.String() != w.String() {
		t.Fatalf("streamed:\n%s\nbatch:\n%s", w.String(), batch.String())
	}
}

func TestDoctorWithoutStreamingWritesOnceAtTheEnd(t *testing.T) {
	cases := map[string]struct {
		args  []string
		agent bool
	}{
		"json":  {[]string{"doctor", "--json"}, false},
		"fix":   {[]string{"doctor", "--fix"}, false},
		"agent": {[]string{"doctor"}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, slowLast())
			if tc.agent {
				t.Setenv("ADBUDDY_SOURCE", "agent")
			}
			w := &stampWriter{}
			var errb bytes.Buffer
			mainContext(context.Background(), tc.args, strings.NewReader(""), w, &errb)
			if len(w.writes) == 0 {
				t.Fatal("no output")
			}
			first, last := w.writes[0].at, w.writes[len(w.writes)-1].at
			if last.Sub(first) > 500*time.Millisecond {
				t.Fatalf("output spread over %v", last.Sub(first))
			}
			if name == "json" && (strings.Count(w.String(), `{"ok"`) != 1 || len(w.writes) != 1) {
				t.Fatalf("not a single envelope: %q", w.String())
			}
		})
	}
}

func TestDoctorCtrlCStreamsNoCancellation(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, SleepMs: 5000}))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	out, _, code := runCtx(t, ctx, "doctor")
	if code != ExitInterrupt || strings.Contains(out, "Verdict:") || strings.Contains(out, "canceled") || strings.Contains(out, "✗") {
		t.Fatalf("exit %d %q", code, out)
	}
}

func TestDoctorStreamedVerdictAndExit(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}))
	out, _, code := runWithInput(t, "", "doctor")
	if code != 1 || !strings.Contains(out, "\nVerdict: broken") || strings.Count(out, "Verdict:") != 1 {
		t.Fatalf("exit %d %q", code, out)
	}
}

func TestDoctorStreamedVerdictOfAHealthyPhone(t *testing.T) {
	doctorEnv(t, doctorRules())
	out, _, code := runWithInput(t, "", "doctor")
	if code != 0 || strings.Count(out, "Verdict:") != 1 {
		t.Fatalf("exit %d %q", code, out)
	}
}
