package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/spf13/pflag"
)

// pflag takes what is between backticks in a flag's text as the name of its
// value ("--no-agent adbuddy run   do not start..."), so no flag text may
// carry a backtick.
func TestHelpTextFlagsHaveNoBackticks(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		app := &App{Getenv: func(string) string { return "" }, Lang: lang}
		root := NewRoot(app)
		for _, path := range helpPaths(root) {
			c, _, err := root.Find(path)
			if err != nil || c == nil {
				t.Fatalf("%v: %v", path, err)
			}
			check := func(f *pflag.Flag) {
				if strings.Contains(f.Usage, "`") {
					t.Errorf("[%s] %q --%s: backtick in %q", lang, strings.Join(path, " "), f.Name, f.Usage)
				}
			}
			c.LocalFlags().VisitAll(check)
			c.InheritedFlags().VisitAll(check)
		}
	}
}

func TestHelpTextServeNoAgentFlag(t *testing.T) {
	out := helpText(t, i18n.EN, []string{"serve"})
	if strings.Contains(out, "--no-agent adbuddy run") {
		t.Errorf("pflag invented a value name:\n%s", out)
	}
	if !strings.Contains(out, "do not start the agent here: run it yourself with 'adbuddy run' (no chat from the phone)") {
		t.Errorf("the whole text of --no-agent is missing:\n%s", out)
	}
}
