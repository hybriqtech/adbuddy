package emutest

import (
	"strings"
	"testing"
	"time"
)

func TestPinBudget(t *testing.T) {
	for _, c := range []struct {
		name        string
		left        time.Duration
		hasDeadline bool
		ci          bool
		want        pinVerdict
	}{
		{"no deadline, local", 0, false, false, pinOK},
		{"no deadline, CI", 0, false, true, pinOK},
		{"plenty left, local", 30 * time.Minute, true, false, pinOK},
		{"plenty left, CI", 30 * time.Minute, true, true, pinOK},
		{"exactly the minimum, local", pinMinBudget, true, false, pinOK},
		{"exactly the minimum, CI", pinMinBudget, true, true, pinOK},
		{"just under the minimum, local", pinMinBudget - time.Second, true, false, pinSkip},
		{"just under the minimum, CI", pinMinBudget - time.Second, true, true, pinFatal},
		{"deadline already passed, local", -time.Second, true, false, pinSkip},
		{"deadline already passed, CI", -time.Second, true, true, pinFatal},
	} {
		if got := pinBudget(c.left, c.hasDeadline, c.ci); got != c.want {
			t.Errorf("%s: pinBudget(%s, %v, %v) = %v, want %v", c.name, c.left, c.hasDeadline, c.ci, got, c.want)
		}
	}
}

func TestPinBudgetMessageSaysWhatToDoInCI(t *testing.T) {
	msg := pinBudgetMsg(90*time.Second + 400*time.Millisecond)
	for _, want := range []string{"1m30s", "4m0s", "-timeout"} {
		if !strings.Contains(msg, want) {
			t.Errorf("pinBudgetMsg = %q, missing %q", msg, want)
		}
	}
}

func TestIsCI(t *testing.T) {
	for _, c := range []struct {
		val  string
		want bool
	}{{"true", true}, {"", false}, {"false", false}, {"1", false}} {
		t.Setenv("CI", c.val)
		if got := isCI(); got != c.want {
			t.Errorf("CI=%q: isCI() = %v, want %v", c.val, got, c.want)
		}
	}
}
