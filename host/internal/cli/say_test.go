package cli

import (
	"bytes"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

func TestTellWritesInTheLanguageOfTheWriter(t *testing.T) {
	var plain bytes.Buffer
	tell(&plain, "error.fix_line", "x")
	if got := plain.String(); got != "Fix: x\n" {
		t.Fatalf("a plain writer is English: %q", got)
	}
	var es bytes.Buffer
	tell(langWriter{Writer: &es, Lang: i18n.ES}, "error.fix_line", "x")
	if got := es.String(); got != "Arreglo: x\n" {
		t.Fatalf("a langWriter speaks its language: %q", got)
	}
}

func TestLogFollowsTheRunLanguageAndTheAgent(t *testing.T) {
	var out bytes.Buffer
	app := &App{Stderr: &out, Getenv: func(string) string { return "" }, Lang: i18n.ES}
	tell(app.Log(), "error.fix_line", "x")
	if got := out.String(); got != "Arreglo: x\n" {
		t.Fatalf("got %q", got)
	}
	out.Reset()
	app.Getenv = func(k string) string {
		if k == "ADBUDDY_SOURCE" {
			return "agent"
		}
		return ""
	}
	tell(app.Log(), "error.fix_line", "x")
	if got := out.String(); got != "Fix: x\n" {
		t.Fatalf("the agent reads English: %q", got)
	}
}
