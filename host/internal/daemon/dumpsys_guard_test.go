package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// Task 10b: dumpsys that reads what is on screen (activity top and every
// argument that is not known to be safe, or a full dump) goes through the
// guard like screencap.

var dumpsysReading = [][]string{
	{"shell", "dumpsys", "activity", "top"},
	{"shell", "dumpsys", "activity", "com.bank/.Main"},
	{"shell", "dumpsys", "activity", "future-view-dump"},
	{"shell", "dumpsys", "activity"},
	{"shell", "dumpsys", "-t", "10", "activity", "top"},
	{"shell", "dumpsys", "--proto", "activity", "top"},
	{"shell", "dumpsys", "--pid", "activity", "top"},
	{"shell", "dumpsys", "--pri", "HIGH", "activity", "top"},
	{"shell", "dumpsys", "--priority", "HIGH", "activity", "top"},
	{"shell", "dumpsys", "--future-option", "battery"},
	{"shell", "dumpsys"},
	{"shell", "dumpsys", "--skip", "meminfo"},
	{"shell", "dumpsys", "-t", "10"},
	{"shell", "dumpsys", "--proto"},
}

var dumpsysAlways = [][]string{
	{"shell", "dumpsys", "activity", "activities"},
	{"shell", "dumpsys", "activity", "a"},
	{"shell", "dumpsys", "activity", "services"},
	{"shell", "dumpsys", "activity", "providers"},
	{"shell", "dumpsys", "activity", "broadcasts"},
	{"shell", "dumpsys", "activity", "processes"},
	{"shell", "dumpsys", "-t", "10", "activity", "services"},
	{"shell", "dumpsys", "window"},
	{"shell", "dumpsys", "window", "windows"},
	{"shell", "dumpsys", "battery"},
	{"shell", "dumpsys", "-l"},
	{"shell", "dumpsys", "--help"},
	{"shell", "dumpsys", "--skip", "meminfo", "battery"},
	{"shell", "dumpsys", "notification"},
}

func TestScreenActionOfDumpsys(t *testing.T) {
	for _, argv := range dumpsysReading {
		if _, reads := screenAction(classify.Finding{Words: argv[1:]}); !reads {
			t.Errorf("%q: not guarded", argv)
		}
	}
	for _, argv := range dumpsysAlways {
		if acts, reads := screenAction(classify.Finding{Words: argv[1:]}); acts || reads {
			t.Errorf("%q: guarded", argv)
		}
	}
}

func TestGuardBlocksDumpsysThatReadsTheScreenInAProtectedApp(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank"}
	for _, argv := range dumpsysReading {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "is a protected app") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	for _, argv := range dumpsysAlways {
		s, exit := e.exec(t, argv...)
		if exit.Code == 126 {
			t.Errorf("%q: blocked: %q", argv, s.errOut.String())
		}
	}
}

func TestGuardLetsDumpsysThroughWithANormalApp(t *testing.T) {
	e := newEnv(t)
	e.withApp("com.example.bank")
	for _, argv := range append(append([][]string{}, dumpsysReading...), dumpsysAlways...) {
		s, exit := e.exec(t, argv...)
		if exit.Code == 126 {
			t.Errorf("%q: blocked: %q", argv, s.errOut.String())
		}
	}
}

func TestGuardBlocksDumpsysWhenTheForegroundIsUnknownOrTheShadeIsOpen(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpForeground] = ErrNoApp
	delete(e.adb.shell, "dumpsys activity activities")
	for _, argv := range [][]string{{"shell", "dumpsys", "activity", "top"}, {"shell", "dumpsys"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell which app is in the foreground") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	if _, exit := e.exec(t, "shell", "dumpsys", "battery"); exit.Code == 126 {
		t.Errorf("dumpsys battery blocked: %+v", exit)
	}

	e = newEnv(t)
	e.withApp()
	e.adb.shell[fixedFocusCmd] = focusShade
	e.adb.shell["dumpsys power"] = powerAwake
	for _, argv := range [][]string{{"shell", "dumpsys", "activity", "top"}, {"shell", "dumpsys", "--skip", "meminfo"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "the notification shade is open") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
}

func TestDumpsysThatReadsTheScreenMustRunAloneAndUnwrapped(t *testing.T) {
	e := newEnv(t)
	e.withApp()
	for _, argv := range [][]string{
		{"shell", "dumpsys activity top | head"},
		{"shell", "dumpsys activity top; id"},
		{"shell", "sh -c 'dumpsys activity top'"},
		{"shell", "nohup dumpsys activity top"},
		{"shell", "busybox dumpsys activity top"},
		{"shell", "dumpsys activity top &"},
	} {
		if s, exit := e.exec(t, argv...); exit.Code != 126 {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
}
