package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The help of every command, recorded in English before M6c touched it:
// what a person (and a script that reads --help) sees must not change when
// the texts move to the catalog. `go test ./internal/cli -run TestHelp
// -update-help` writes testdata/help; the diff is for a person to read.
var updateHelp = flag.Bool("update-help", false, "rewrite testdata/help")

// helpPaths is every command that has help, the root first: its path of names.
func helpPaths(root *cobra.Command) [][]string {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	paths := [][]string{{}}
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "__complete" || sub.Name() == "__completeNoDesc" {
				continue
			}
			p := append(append([]string{}, path...), sub.Name())
			paths = append(paths, p)
			walk(sub, p)
		}
	}
	walk(root, nil)
	return paths
}

// helpText is what `adbuddy <path> --help` writes, in lang, for a person.
func helpText(t *testing.T, lang i18n.Lang, path []string) string {
	t.Helper()
	var out bytes.Buffer
	app := &App{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out, Getenv: func(string) string { return "" }, Lang: lang}
	root := NewRoot(app)
	root.SetArgs(append(append([]string{}, path...), "--help"))
	if err := root.Execute(); err != nil {
		t.Fatalf("%v --help: %v", path, err)
	}
	return out.String()
}

func helpFile(lang i18n.Lang, path []string) string {
	name := strings.Join(path, "_")
	if name == "" {
		name = "root"
	}
	return filepath.Join("testdata", "help", string(lang), name+".txt")
}

func TestHelpEnglishIsAsRecorded(t *testing.T) {
	paths := helpPaths(NewRoot(&App{Getenv: func(string) string { return "" }}))
	if len(paths) < 25 {
		t.Fatalf("only %d commands with help: did the command tree change?", len(paths))
	}
	for _, path := range paths {
		got := helpText(t, i18n.EN, path)
		file := helpFile(i18n.EN, path)
		if *updateHelp {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%v: no recorded help (%v): run with -update-help on the English code", path, err)
			continue
		}
		if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Errorf("%v: the English help changed:\n--- recorded\n%s\n--- now\n%s", path, want, got)
		}
	}
}

var englishHeaders = regexp.MustCompile(`(?m)^(Usage|Aliases|Examples|Available Commands|Additional Commands|Flags|Global Flags|Additional help topics):$`)
var aKey = regexp.MustCompile(`(?m)^\s*[a-z]+(\.[a-z_]+)+\s*$`)

// Spanish help: no header, no catalog key and no command text of ours in
// English; the recorded Spanish of the root and of a few commands is for a
// person to read in the diff.
func TestHelpInSpanishIsComplete(t *testing.T) {
	// What stays English on purpose (M6c ruling 19): the text of `completion <shell>`, instructions for a shell.
	skip := func(path []string) bool { return len(path) >= 2 && path[0] == "completion" }
	root := NewRoot(&App{Getenv: func(string) string { return "" }})
	for _, path := range helpPaths(root) {
		if skip(path) {
			continue
		}
		en, es := helpText(t, i18n.EN, path), helpText(t, i18n.ES, path)
		if en == es {
			t.Errorf("%v: the Spanish help is the English one", path)
		}
		if h := englishHeaders.FindString(es); h != "" {
			t.Errorf("%v: an English header in the Spanish help: %q\n%s", path, h, es)
		}
		if k := aKey.FindString(es); k != "" {
			t.Errorf("%v: a catalog key shows in the Spanish help: %q\n%s", path, k, es)
		}
		// Nothing of the command's own text in English: not its Short, its
		// Long or the help of a flag. (Its examples are command lines and
		// are not translated.)
		c, _, err := root.Find(path)
		if err != nil || c == nil {
			t.Fatalf("%v: %v", path, err)
		}
		own := []string{c.Short, strings.SplitN(c.Long, "\n", 2)[0]}
		c.Flags().VisitAll(func(f *pflag.Flag) { own = append(own, f.Usage) })
		c.InheritedFlags().VisitAll(func(f *pflag.Flag) { own = append(own, f.Usage) })
		for _, s := range own {
			if s != "" && !strings.HasPrefix(s, "help for ") && !strings.HasPrefix(s, "version for ") && strings.Contains(es, s) {
				t.Errorf("%v: English text in the Spanish help: %q", path, s)
			}
		}
	}
	for _, path := range [][]string{{}, {"doctor"}} {
		got := helpText(t, i18n.ES, path)
		file := helpFile(i18n.ES, path)
		if *updateHelp {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(file)
		if err != nil || got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Errorf("%v: the Spanish help is not the recorded one (%v):\n%s", path, err, got)
		}
	}
}

// With ADBUDDY_LANG=es, --json and ADBUDDY_SOURCE=agent keep the help in
// English (what a program or a model reads), and a person gets Spanish.
func TestHelpStaysEnglishForPrograms(t *testing.T) {
	want := helpText(t, i18n.EN, []string{"doctor"})
	es := helpText(t, i18n.ES, []string{"doctor"})
	cases := []struct {
		name  string
		agent bool
		args  []string
		want  string
	}{
		{"json", false, []string{"doctor", "--help", "--json"}, ""},
		{"agent", true, []string{"doctor", "--help"}, want},
		{"person", false, []string{"doctor", "--help"}, es},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ADBUDDY_LANG", "es")
			if c.agent {
				t.Setenv("ADBUDDY_SOURCE", "agent")
			}
			out, errb, code := runCtx(t, t.Context(), c.args...)
			if code != 0 {
				t.Fatalf("exit %d: %s%s", code, out, errb)
			}
			if c.name == "json" {
				// The --json flag itself is listed in the global flags; compare the rest.
				out = strings.ReplaceAll(out, "\r\n", "\n")
				if strings.Contains(out, "Uso:") || !strings.Contains(out, "Usage:") {
					t.Errorf("--json help is not English:\n%s", out)
				}
				return
			}
			if out != c.want {
				t.Errorf("help differs:\n--- want\n%s\n--- got\n%s", c.want, out)
			}
		})
	}
}
