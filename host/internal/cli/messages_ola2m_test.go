package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// V35: `serve` starts the agent by itself; the other way is `serve --no-agent`
// plus `adbuddy run` in another terminal. No text may say that `serve` and
// `run` always go in different terminals.
func TestServeRunTextsKnowServeStartsTheAgent(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		serve := helpText(t, lang, []string{"serve"})
		starts := map[i18n.Lang]string{i18n.EN: "starts the agent", i18n.ES: "arranca el agente"}[lang]
		if !strings.Contains(serve, starts) || !strings.Contains(serve, "adbuddy run") {
			t.Errorf("%s: serve help does not say %q and adbuddy run:\n%s", lang, starts, serve)
		}
		for _, old := range []string{"Keep it open and start the agent in another terminal", "Déjalo abierto y arranca el agente en otra terminal"} {
			if strings.Contains(serve, old) {
				t.Errorf("%s: serve help still says %q", lang, old)
			}
		}
		if run := helpText(t, lang, []string{"run"}); !strings.Contains(run, "--no-agent") {
			t.Errorf("%s: run help must name --no-agent:\n%s", lang, run)
		}
		for _, line := range strings.Split(lang.T("help.run.long"), "\n") {
			if n := len([]rune(line)); n > 80 {
				t.Errorf("%s: help.run.long has a line of %d characters: %q", lang, n, line)
			}
		}
		for _, key := range []string{"envcmd.environment_ready_next_adbuddy_serve", "setup.next_run_adbuddy_serve_and"} {
			if s := lang.T(key); !strings.Contains(s, "adbuddy serve --no-agent") || !strings.Contains(s, "adbuddy run") {
				t.Errorf("%s %s: %q lacks `adbuddy serve --no-agent` and `adbuddy run`", lang, key, s)
			}
		}
		// A failed workspace.Init is not always another agent: links and permissions too.
		if s := lang.T("err.run.workspace_failed.fix", "x", "DIR"); strings.Contains(s, "Close other agents") || strings.Contains(s, "Cierra los otros agentes") || !strings.Contains(s, "DIR") {
			t.Errorf("%s: err.run.workspace_failed.fix = %q", lang, s)
		}
	}
}

// ErrAppTooOld is shown (in the JSON note) with the same advice as the catalog.
func TestErrAppTooOldDoesNotPromiseThatSetupUpdates(t *testing.T) {
	s := phoneapp.ErrAppTooOld.Error()
	if strings.Contains(s, "to update it") || !strings.Contains(s, "--apk") {
		t.Errorf("ErrAppTooOld = %q", s)
	}
}
