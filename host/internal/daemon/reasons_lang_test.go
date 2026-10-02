package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// M6c §4.1 and §9.2: the question a person answers says the same in English
// and in Spanish: the reasons are in the language of the PC, whoever raised
// them (a rule, the agent's folder, the state of the phone), and everything
// that decides (the raw command, the level, special, the key of "allow
// during the session") is the same in both.

// askedIn runs argv from cwd in the given language, the user saying no, and
// returns the question that was asked.
func askedIn(t *testing.T, lang string, prepare func(*env), cwd string, argv ...string) confirm.Request {
	t.Helper()
	e := newEnv(t)
	e.d.o.Lang = lang
	if prepare != nil {
		prepare(e)
	}
	e.conf.decision = confirm.Deny
	if cwd == "" {
		cwd = t.TempDir()
	}
	_, exit := e.execIn(t, cwd, argv...)
	if exit.Code != ExitDenied || len(e.conf.asked) != 1 {
		t.Fatalf("%s %q: exit %+v asked %d", lang, argv, exit, len(e.conf.asked))
	}
	return e.conf.asked[0]
}

// withoutCwd is a key (rule, cwd and argv, NUL apart) without the cwd, which
// is a folder of its own in each run.
func withoutCwd(key string) string {
	parts := strings.Split(key, "\x00")
	if len(parts) < 3 {
		return key
	}
	return parts[0] + "\x00" + strings.Join(parts[2:], "\x00")
}

func TestTheQuestionDecidesTheSameInEnglishAndSpanish(t *testing.T) {
	outside := t.TempDir()
	for _, c := range []struct {
		name    string
		prepare func(*env)
		cwd     string
		argv    []string
		en, es  string // what the detail says in each language
		notInES string // English that must not be in the Spanish detail
		keyed   bool   // a question of a rule has a key; one that depends on state has none
	}{
		{name: "a reason of a rule", argv: []string{"shell", "sed", "-i", "s/a/b/", "/sdcard/x"},
			en: "sed -i edits files in place", es: "sed -i modifica los ficheros directamente", notInES: "edits files in place", keyed: true},
		{name: "a reason with a value", argv: []string{"shell", "sed", "-Q", "s/a/b/", "/sdcard/x"},
			en: "unknown sed option -Q", es: "opción de sed desconocida: -Q", notInES: "unknown sed option", keyed: true},
		{name: "the PC folder of adb pull", cwd: outside, argv: []string{"pull", "/sdcard/Download/x", "run.txt"},
			en: "writes outside the agent environment: " + outside, es: "escribe fuera del entorno del agente: " + outside, notInES: "writes outside"},
		{name: "the state of the phone", argv: []string{"shell", "pm", "uninstall", "--user", "0", "com.user"},
			prepare: func(e *env) { e.adb.shell["pm list packages -s com.user"] = "" },
			en:      "not a system app", es: "no es una app del sistema", notInES: "not a system app"},
	} {
		t.Run(c.name, func(t *testing.T) {
			en := askedIn(t, "en", c.prepare, c.cwd, c.argv...)
			es := askedIn(t, "es", c.prepare, c.cwd, c.argv...)
			if !strings.Contains(en.Detail, c.en) {
				t.Errorf("English detail %q lacks %q", en.Detail, c.en)
			}
			if !strings.Contains(es.Detail, c.es) {
				t.Errorf("Spanish detail %q lacks %q", es.Detail, c.es)
			}
			if strings.Contains(es.Detail, c.notInES) {
				t.Errorf("Spanish detail %q has English: %q", es.Detail, c.notInES)
			}
			if en.Title == es.Title {
				t.Errorf("the title is the same in both languages: %q", en.Title)
			}
			if en.RawCommand != es.RawCommand || en.Special != es.Special || (en.Key != "") != c.keyed || (es.Key != "") != c.keyed {
				t.Errorf("what decides changed with the language: en %+v / es %+v", en, es)
			}
			if withoutCwd(en.Key) != withoutCwd(es.Key) {
				t.Errorf("the key of allow-during-the-session changed: %q / %q", en.Key, es.Key)
			}
		})
	}
}

// A reason that comes with a message of the system keeps it as it is in both
// languages, and the text around it is in each.
func TestPCWriteReasonWithASystemErrorIsInBothLanguages(t *testing.T) {
	e := newEnv(t)
	e.d.o.Workspace = "/no/such/folder/for/this/test"
	got := e.d.checkPCWrites(t.TempDir(), []classify.PCWrite{{Cmd: "pull"}})
	if got.EN == "" || got.ES == "" || got.EN == got.ES || !strings.HasPrefix(got.ES, "pull escribe en el PC") {
		t.Fatalf("both languages must say their own thing: %+v", got)
	}
	if i := strings.Index(got.EN, ": "); i < 0 || !strings.HasSuffix(got.ES, got.EN[i+2:]) {
		t.Fatalf("the system's message must be the same in both: %+v", got)
	}
}
