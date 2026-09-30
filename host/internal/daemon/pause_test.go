package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

func TestPausedRefusesEverything(t *testing.T) {
	e := newEnv(t)
	paused := true
	e.d.o.Paused = func() bool { return paused }
	s, exit := e.exec(t, "shell", "getprop", "ro.product.model")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") {
		t.Fatalf("green while paused: %+v %q", exit, s.errOut.String())
	}
	if len(e.adb.runs) != 0 {
		t.Fatalf("adb ran while paused: %v", e.adb.runs)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "paused") {
		t.Fatalf("journal %+v", j)
	}
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if u.Code != ExitDenied || u.ErrorCode != "PAUSED" {
		t.Fatalf("undo while paused: %+v", u)
	}
	paused = false
	e.adb.out["shell getprop ro.product.model"] = "Pixel\n"
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("after resume: %+v", exit)
	}
}

func TestConfirmationKeyIsRuleAndExactArguments(t *testing.T) {
	e := newEnv(t)
	e.exec(t, "shell", "reboot")
	e.exec(t, "shell", "reboot", "bootloader")
	if len(e.conf.asked) != 2 {
		t.Fatalf("asked %d", len(e.conf.asked))
	}
	a, b := e.conf.asked[0].Key, e.conf.asked[1].Key
	if a == "" || a == b || !strings.HasPrefix(a, e.last(t).Rule) {
		t.Fatalf("keys %q %q", a, b)
	}
}
