package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// M6c §9.3: the messages that serve, `remote`, `journal`, `undo`, `env init` and
// the phone link give a person, each in Spanish next to its English. English is
// what the tests of each command already read; these look at the Spanish: no
// English of the catalog is left in it, what is data (an id, a command, a
// path) is as it was, and the words around it are in the language.

func TestFormatEntryInSpanish(t *testing.T) {
	undoneBy := "u_1"
	e := journal.Entry{
		ID: "j_1", TS: time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC), Level: "yellow", Status: "ok",
		Description: "set a value", Argv: []string{"shell", "settings", "put", "global", "x", "1"},
		Reason: "was asked", UndoneBy: &undoneBy, Tampered: true,
		Inverse: &journal.Inverse{Argv: []string{"shell", "settings", "put", "global", "x", "0"}},
	}
	en, es := formatEntryIn(i18n.EN, e), formatEntryIn(i18n.ES, e)
	if en != formatEntry(e) {
		t.Fatalf("formatEntry is the English one:\n%s\n%s", formatEntry(e), en)
	}
	for _, want := range []string{", deshecho", ", ⚠ no escrito por ADBuddy"} {
		if !strings.Contains(es, want) {
			t.Errorf("the Spanish line lacks %q:\n%s", want, es)
		}
	}
	// A status that is not ok shows its reason; an entry that is not undone shows its undo.
	e.Status, e.UndoneBy, e.Tampered = "denied", nil, false
	es = formatEntryIn(i18n.ES, e)
	for _, want := range []string{"\n    motivo: was asked", "\n    deshacer: adb shell settings put global x 0", "[j_1] adb shell settings put global x 1", "set a value"} {
		if !strings.Contains(es, want) {
			t.Errorf("the Spanish line lacks %q:\n%s", want, es)
		}
	}
	i18ntest.NoEnglish(t, es)
}

func TestUndoItemsInSpanish(t *testing.T) {
	items := func(app *App) string {
		var out bytes.Buffer
		printUndoItems(app.Say(&out), []daemon.UndoItem{{ID: "j_1", Description: "a"}}, []daemon.UndoItem{{ID: "j_2", Description: "b", Error: "e1"}}, []daemon.UndoItem{{ID: "j_3", Description: "c", Error: "e2"}})
		return out.String()
	}
	getenv := func(string) string { return "" }
	en := items(&App{Getenv: getenv, Lang: i18n.EN})
	es := items(&App{Getenv: getenv, Lang: i18n.ES})
	if en != "undone   j_1  a\nfailed   j_2  b: e1\nskipped  j_3  c (e2)\n" {
		t.Fatalf("English as always: %q", en)
	}
	if es != "deshecho  j_1  a\nfallido   j_2  b: e1\nomitido   j_3  c (e2)\n" {
		t.Fatalf("Spanish: %q", es)
	}
	// The agent reads English whatever the language of the PC.
	agent := &App{Lang: i18n.ES, Getenv: func(k string) string {
		if k == "ADBUDDY_SOURCE" {
			return "agent"
		}
		return ""
	}}
	if got := items(agent); got != en {
		t.Fatalf("the agent's undo list changes with the language: %q", got)
	}
}

func TestSkillLinesInSpanish(t *testing.T) {
	paths := []string{
		"AGENTS.md",
		filepath.Join(".claude", "skills", "debloat", "SKILL.md"),
		filepath.Join(".claude", "skills", "battery", "SKILL.md"),
		filepath.Join(".agents", "skills", "debloat", "SKILL.md"),
	}
	want := map[string][]string{
		"created": {"  creado AGENTS.md", "  creadas 2 skills en .claude/skills", "  creada 1 skill en .agents/skills"},
		"updated": {"  actualizado AGENTS.md", "  actualizadas 2 skills en .claude/skills", "  actualizada 1 skill en .agents/skills"},
	}
	for verb, w := range want {
		if got := strings.Join(groupSkillLines(i18n.ES, verb, paths), "\n"); got != strings.Join(w, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", verb, got, strings.Join(w, "\n"))
		}
		// English keeps its old wording, "1 skills" included (the existing test reads it).
		if got := groupSkillLines(i18n.EN, verb, paths); got[2] != "  "+verb+" 1 skills in .agents/skills" {
			t.Errorf("English changed: %q", got)
		}
	}
}

func TestRemoteTextInSpanish(t *testing.T) {
	at := time.Date(2026, 10, 2, 21, 30, 0, 0, time.Local)
	old := remoteNow
	remoteNow = func() time.Time { return at.Add(-3 * time.Hour) }
	defer func() { remoteNow = old }()
	infos := map[string]remoteInfo{
		"off":               {},
		"on":                {Enabled: true, ExpiresAt: &at, Serve: true, Address: "100.101.102.103:5555"},
		"on without serve":  {Enabled: true, ExpiresAt: &at},
		"closing":           {Enabled: true, Closing: true, ExpiresAt: &at, Serve: true},
		"closing no serve":  {Enabled: true, Closing: true, ExpiresAt: &at},
		"off without serve": {Serve: false},
	}
	for name, info := range infos {
		for _, op := range []string{"on", "off", "status"} {
			en, es := remoteTextIn(i18n.EN, op, info), remoteTextIn(i18n.ES, op, info)
			if en != remoteText(op, info) {
				t.Errorf("%s %s: remoteText is the English one", name, op)
			}
			i18ntest.NoEnglish(t, es)
			if es == en || es == "" {
				t.Errorf("%s %s: Spanish %q, English %q", name, op, es, en)
			}
			// A sentence alone starts with a capital; after "serve no está en marcha; " it does not.
			if info.Serve && es[:1] != strings.ToUpper(es[:1]) {
				t.Errorf("%s %s: no capital: %q", name, op, es)
			}
			// The commands and the port are what they were.
			for _, keep := range []string{"adbuddy remote off", "5555"} {
				if strings.Contains(en, keep) != strings.Contains(es, keep) {
					t.Errorf("%s %s: %q is in one language only:\n%s\n%s", name, op, keep, en, es)
				}
			}
		}
	}
	if got := remoteTextIn(i18n.ES, "status", infos["on without serve"]); !strings.HasPrefix(got, "serve no está en marcha; modo remoto: activado hasta las 21:30 (unas 3 h).") {
		t.Errorf("status without serve: %q", got)
	}
	if got := remoteTextIn(i18n.ES, "off", infos["off"]); got != "El modo remoto está desactivado: el puerto 5555 está cerrado." {
		t.Errorf("off: %q", got)
	}
	// What serve says when it starts and when it stops.
	s := remote.State{Enabled: true, ExpiresAt: at}
	if got := remoteOnNoticeIn(i18n.ES, s); got != "El modo remoto está activado hasta las 21:30 (puerto 5555 abierto)." {
		t.Errorf("start: %q", got)
	}
	if got := stillOnNoticeIn(i18n.ES, s); !strings.Contains(got, "sigue activado") || !strings.Contains(got, "`adbuddy remote off`") {
		t.Errorf("stop: %q", got)
	}
	if remoteOnNotice(s) != remoteOnNoticeIn(i18n.EN, s) || stillOnNotice(s) != stillOnNoticeIn(i18n.EN, s) {
		t.Error("the plain ones are the English ones")
	}
}

// A message that a part of the phone link writes to its log comes out in the
// language of the log (the language of the run: app.Log()), and in English when
// the log is a plain writer (every test that reads it).
func TestPhoneLinkLogsInTheLanguageOfTheLog(t *testing.T) {
	var plain, es bytes.Buffer
	newUILink("", &plain).Protected("x")
	newUILink("", langWriter{Writer: &es, Lang: i18n.ES}).Protected("x")
	if !strings.Contains(plain.String(), "The phone app has not sent its list of protected apps yet") {
		t.Errorf("a plain log is English: %q", plain.String())
	}
	if !strings.Contains(es.String(), "La app del móvil todavía no ha enviado su lista de apps protegidas") {
		t.Errorf("a Spanish log is Spanish: %q", es.String())
	}
	i18ntest.NoEnglish(t, es.String())
}

// `adbuddy new` and `adbuddy stop` answer in the language of the PC; an error
// of the session is the daemon's and stays English.
func TestSessionAnswersInTheLanguageOfThePC(t *testing.T) {
	sess := testSession(t)
	data := shortDir(t)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		return handleSessionIn(i18n.ES, ctx, sess, "claude", req, c)
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	var out, errOut bytes.Buffer
	exit, err := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindSession, Argv: []string{"stop"}}, nil, &out, &errOut, func([]byte) error { return nil })
	if err != nil || exit.Code != 0 || out.String() != "El agente se detuvo; adbuddy serve sigue en marcha.\n" {
		t.Fatalf("exit %+v, err %v, out %q", exit, err, out.String())
	}
}

// The errors that have no catalog message of their own still say their fix in
// the language of the person, and Ctrl+C has its words.
func TestInterruptedAndUsageInSpanish(t *testing.T) {
	var out, errOut bytes.Buffer
	WriteFailureIn(&out, &errOut, false, i18n.ES, Interrupted())
	if got := errOut.String(); got != "Error: INTERRUPTED — detenido con Ctrl+C\n" {
		t.Errorf("interrupted: %q", got)
	}
	errOut.Reset()
	WriteFailureIn(&out, &errOut, false, i18n.ES, UsageError("unknown command \"x\" for \"adbuddy\""))
	if got := errOut.String(); !strings.Contains(got, "unknown command \"x\"") || !strings.Contains(got, "Arreglo: Ejecuta `adbuddy --help` para ver cómo se usa.") {
		t.Errorf("usage: %q", got)
	}
	// The JSON and the plain error are what they were.
	if e := Interrupted(); e.Message != "stopped with Ctrl+C" || e.Code != "INTERRUPTED" || ExitCode(e) != ExitInterrupt {
		t.Errorf("%+v", e)
	}
	if e := UsageError("m"); e.Message != "m" || e.Fix != "Run `adbuddy --help` to see the usage." || ExitCode(e) != ExitUsage {
		t.Errorf("%+v", e)
	}
}
