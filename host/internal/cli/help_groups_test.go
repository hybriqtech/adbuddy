package cli

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

func TestHelpGroupsRoot(t *testing.T) {
	want := []struct {
		title    map[i18n.Lang]string
		commands []string
	}{
		{map[i18n.Lang]string{i18n.EN: "Getting started:", i18n.ES: "Primeros pasos:"}, []string{"doctor", "serve", "setup"}},
		{map[i18n.Lang]string{i18n.EN: "Day to day:", i18n.ES: "Día a día:"}, []string{"attach", "connect", "env", "journal", "new", "remote", "run", "stop", "undo", "version"}},
		{map[i18n.Lang]string{i18n.EN: "For the agent:", i18n.ES: "Para el agente:"}, []string{"foreground", "notifications", "screen", "screenshot", "send", "tap", "type"}},
	}
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		out := helpText(t, lang, nil)
		pos := 0
		grouped := map[string]bool{}
		for _, g := range want {
			title := g.title[lang]
			i := strings.Index(out[pos:], "\n"+title+"\n")
			if i < 0 {
				t.Fatalf("[%s] group %q missing or out of order:\n%s", lang, title, out)
			}
			start := pos + i + len(title) + 2
			end := len(out)
			if j := strings.Index(out[start:], "\n\n"); j >= 0 {
				end = start + j
			}
			block := out[start:end]
			pos = end
			var got []string
			for _, line := range strings.Split(block, "\n") {
				if f := strings.Fields(line); len(f) > 0 {
					got = append(got, f[0])
				}
			}
			if strings.Join(got, ",") != strings.Join(g.commands, ",") {
				t.Errorf("[%s] %q lists %v, want %v", lang, title, got, g.commands)
			}
			for _, c := range g.commands {
				if !regexp.MustCompile(`(?m)^  ` + c + `\s`).MatchString(block) {
					t.Errorf("[%s] %q: no line for %s", lang, title, c)
				}
				grouped[c] = true
			}
		}
		root := NewRoot(&App{Getenv: func(string) string { return "" }, Lang: lang})
		root.InitDefaultHelpCmd()
		root.InitDefaultCompletionCmd()
		for _, c := range root.Commands() {
			if c.Hidden || strings.HasPrefix(c.Name(), "__complete") || c.Name() == "help" || c.Name() == "completion" {
				continue
			}
			if !grouped[c.Name()] {
				t.Errorf("[%s] visible command %q is in no group", lang, c.Name())
			}
		}
	}
}

func TestHelpGroupsRootExample(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		out := helpText(t, lang, nil)
		for _, s := range []string{"adbuddy setup", "adbuddy serve"} {
			if !strings.Contains(out, s) {
				t.Errorf("[%s] root help has no example %q:\n%s", lang, s, out)
			}
		}
	}
}

// H7: a command's one-line description fits a row of the list in 80 columns.
func TestHelpShortsFit(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		root := NewRoot(&App{Getenv: func(string) string { return "" }, Lang: lang})
		for _, path := range helpPaths(root) {
			name := strings.Join(path, " ")
			// The root's Short is the title of the application, not a row of
			// the list (69 characters in English, 90 in Spanish); help and
			// completion are cobra's, with fixed text.
			if len(path) == 0 || path[0] == "help" || path[0] == "completion" {
				continue
			}
			c, _, err := root.Find(path)
			if err != nil || c == nil {
				t.Fatalf("%v: %v", path, err)
			}
			if n := utf8.RuneCountInString(c.Short); n > 60 {
				t.Errorf("[%s] %q: Short has %d characters (max 60): %q", lang, name, n, c.Short)
			}
		}
	}
}
