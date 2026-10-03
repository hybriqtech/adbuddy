package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
)

func TestHelpUnknownTopicIsAUsageError(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.EN)
	exit, out, errOut := runCLI(t, "help", "nope")
	if exit != 2 || !strings.Contains(errOut, "Error: USAGE — ") || !strings.Contains(errOut, "nope") {
		t.Fatalf("exit %d %q", exit, errOut)
	}
	if strings.Contains(errOut, "adbuddy help --help") || !strings.Contains(errOut, "`adbuddy --help`") {
		t.Fatalf("fix: %q", errOut)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("stdout has the help: %q", out)
	}

	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "help", "nope")
	if exit != 2 || !strings.Contains(es, "tema de ayuda desconocido") {
		t.Fatalf("es: exit %d %q", exit, es)
	}
}

func TestHelpKnownTopicStillWorks(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.EN)
	exit, out, _ := runCLI(t, "help", "tap")
	if exit != 0 || !strings.Contains(out, "adbuddy tap") {
		t.Fatalf("help tap: exit %d %q", exit, out)
	}
	exit, out, _ = runCLI(t, "help")
	if exit != 0 || !strings.Contains(out, "Available Commands:") {
		t.Fatalf("help: exit %d %q", exit, out)
	}
}
