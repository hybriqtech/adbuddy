package cli

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

func TestPluralsOne(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		app := &App{Getenv: func(string) string { return "" }, Lang: lang}
		if got := app.TN("doctor.server.ok", 1, 5038); strings.Contains(got, "phone(s)") || strings.Contains(got, "phones") {
			t.Errorf("[%s] doctor.server.ok 1: %q", lang, got)
		}
		if lang == i18n.EN {
			if got := app.TN("doctor.server.ok", 2, 5038); !strings.Contains(got, "2 phones") {
				t.Errorf("doctor.server.ok 2: %q", got)
			}
			if got := app.TN("journal.damaged_journal_lines_were_skipped", 1); strings.Contains(got, "lines") {
				t.Errorf("damaged 1: %q", got)
			}
			if got := app.TN("journal.damaged_journal_lines_were_skipped", 2); !strings.Contains(got, "lines") {
				t.Errorf("damaged 2: %q", got)
			}
			for _, k := range []string{"envcmd.skills_created", "envcmd.skills_updated"} {
				if got := app.TN(k, 1, "x"); strings.Contains(got, "skills") || strings.Contains(got, "%!") {
					t.Errorf("%s 1: %q", k, got)
				}
				if got := app.TN(k, 3, "x"); !strings.Contains(got, "3 skills") {
					t.Errorf("%s 3: %q", k, got)
				}
			}
		} else {
			// Spanish: a .one that counts must differ from its .other.
			for _, k := range []string{"doctor.server.ok", "journal.damaged_journal_lines_were_skipped", "envcmd.skills_created", "envcmd.skills_updated"} {
				if app.TN(k, 1, "x") == app.TN(k, 2, "x") {
					t.Errorf("[es] %s: .one equals .other", k)
				}
			}
		}
	}
}

// A fix is a sentence: it starts with a capital and ends with a full stop.
// The ones that start or end with an argument (a command, a name) are exempt.
func TestFixStyle(t *testing.T) {
	for _, p := range i18n.Pairs() {
		if !strings.HasPrefix(p.Key, "err.") || !strings.HasSuffix(p.Key, ".fix") {
			continue
		}
		for lang, s := range map[string]string{"en": p.EN, "es": p.ES} {
			first, _ := utf8.DecodeRuneInString(s)
			if !strings.HasPrefix(s, "%[") && !unicode.IsUpper(first) {
				t.Errorf("[%s] %s: should start with a capital: %q", lang, p.Key, s)
			}
			if !strings.HasSuffix(s, ".") && !strings.HasPrefix(s, "%[") && !strings.HasSuffix(s, "]s") && !strings.HasSuffix(s, "]d") {
				t.Errorf("[%s] %s: should end with a full stop: %q", lang, p.Key, s)
			}
		}
	}
}
