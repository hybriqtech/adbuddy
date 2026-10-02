package session

import (
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// The notice of an attached terminal when the agent ends: AgentEnded in
// English (what the tests and the agent's logs know), the PC's language otherwise.
func TestAgentEndedInTheLanguageOfThePC(t *testing.T) {
	if got := New(Options{}).agentEnded(); got != AgentEnded {
		t.Fatalf("English must be AgentEnded byte for byte: %q", got)
	}
	if got := New(Options{Lang: i18n.EN}).agentEnded(); got != AgentEnded {
		t.Fatalf("en: %q", got)
	}
	want := "\r\n[ADBuddy] El agente terminó. Vuelve a arrancarlo con adbuddy new, o desconéctate con Ctrl+].\r\n"
	if got := New(Options{Lang: i18n.ES}).agentEnded(); got != want {
		t.Fatalf("es: %q", got)
	}
}
