package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// M6c §3: what serve tells the person is in the language of the PC.
func TestWaitingNoticeInSpanish(t *testing.T) {
	e := newEnv(t)
	e.d.o.Lang = "es"
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "input tap")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	n := &notices{}
	e.d.o.OnNotice = n.add
	yellow := e.execAsync("shell", "input tap 1 1")
	g.waitStarted(t)
	red := e.execAsync("shell", "reboot")
	if !n.waitFor("Esperando a que termine") {
		t.Fatalf("not said in Spanish: %q", n.get())
	}
	for _, how := range []string{"pausa desde la app del móvil", "detén el comando del agente", "Ctrl+C"} {
		if n.count(how) != 1 {
			t.Fatalf("the notice does not say %q: %q", how, n.get())
		}
	}
	if n.count("Waiting for") != 0 {
		t.Fatalf("English in the Spanish notice: %q", n.get())
	}
	close(g.open)
	waitResult(t, yellow)
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, red)
}

func TestUndoPrefixAndWarningsFollowTheLanguage(t *testing.T) {
	if got := undoPrefix("es"); got != "Deshacer: " {
		t.Errorf("es: %q", got)
	}
	if got := undoPrefix("en"); got != "Undo: " {
		t.Errorf("en: %q", got)
	}
	if got := undoPrefix(""); got != "Undo: " {
		t.Errorf("no language: %q", got)
	}
	e := newEnv(t)
	e.d.o.Lang = "es"
	if got := e.d.lang().T("daemon.journal_failed", "j_1", "disco lleno"); !strings.HasPrefix(got, "falló la escritura en el diario para j_1: ") {
		t.Errorf("warning: %q", got)
	}
}
