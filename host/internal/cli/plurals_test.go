package cli

import (
	"strings"
	"testing"

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
			// Spanish: assert the singular by hand, and that it is not the plural.
			singular := map[string]string{
				"doctor.server.ok":                           "1 móvil conectado",
				"journal.damaged_journal_lines_were_skipped": "Se saltó 1 línea",
				"envcmd.skills_created":                      "creada 1 skill",
				"envcmd.skills_updated":                      "actualizada 1 skill",
			}
			for k, want := range singular {
				got := app.TN(k, 1, 5038)
				if k != "doctor.server.ok" {
					got = app.TN(k, 1, "x")
				}
				if !strings.Contains(got, want) {
					t.Errorf("[es] %s 1: want %q in %q", k, want, got)
				}
				if strings.Contains(got, "%!") || strings.Contains(got, "móviles") || strings.Contains(got, "líneas") || strings.Contains(got, "skills") {
					t.Errorf("[es] %s 1 looks plural or malformed: %q", k, got)
				}
			}
		}
	}
}
