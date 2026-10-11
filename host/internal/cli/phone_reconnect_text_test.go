package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// Only the warning about a disconnection promises a reconnection: the app
// never comes back after the others until serve restarts.
func TestOnlyTheDisconnectionWarningPromisesReconnection(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.EN, i18n.ES} {
		app := &App{Lang: l, Getenv: func(string) string { return "" }}
		promises := func(s string) bool {
			return strings.Contains(s, "reconnect") || strings.Contains(s, "vuelva a conectar")
		}
		if got := app.T("phone.the_phone_app_disconnected_confirmations"); !promises(got) {
			t.Errorf("[%s] disconnection warning must promise it: %q", l, got)
		}
		for name, got := range map[string]string{
			"without_the":   app.T("phone.confirmations_happen_here_without_the", "x"),
			"could_not_chk": app.T("phone.could_not_check_whether_the", "x"),
			"not_installed": app.T("phone.the_adbuddy_app_is_not"),
		} {
			if promises(got) {
				t.Errorf("[%s] %s must not promise a reconnection: %q", l, name, got)
			}
		}
	}
}
