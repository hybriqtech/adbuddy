package classify

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus lives in testdata as data, one JSON object per line (a command
// may span several lines):
//
//	habitual.jsonl      commands of everyday use and the level they must keep
//	m2-levels.jsonl     the level of every 🟢 or 🟡 shell line of the tests at M2
//	raised.jsonl        the lines of m2-levels.jsonl that M2.5 raised, and why
//	differential.jsonl  tricks with the sentinel @S@, run on a real mksh
//	oracle.jsonl        what the real mksh did with each differential case
//	mksh-builtins.tsv   the builtins, reserved words and aliases of that mksh

// corpusCase is one line of a corpus file. A shell line is {"sh": …}; any
// other adb command is {"argv": […]}.
type corpusCase struct {
	Sh      string   `json:"sh,omitempty"`
	Argv    []string `json:"argv,omitempty"`
	Level   string   `json:"level,omitempty"`
	Special bool     `json:"special,omitempty"`
	Why     string   `json:"why,omitempty"`
	Ran     bool     `json:"ran,omitempty"`
}

func (k corpusCase) argv() []string {
	if k.Argv != nil {
		return k.Argv
	}
	return []string{"shell", k.Sh}
}

func (k corpusCase) level(t *testing.T) Level {
	t.Helper()
	l, err := ParseLevel(k.Level)
	if err != nil {
		t.Fatalf("%q: %v", k.argv(), err)
	}
	return l
}

// readJSONL reads a corpus file; lines starting with # are comments.
func readJSONL[T any](t *testing.T, name string) []T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []T
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var v T
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%s:%d: %v", name, n, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func loadCorpus(t *testing.T, name string) []corpusCase {
	t.Helper()
	return readJSONL[corpusCase](t, name)
}

// Spec M2.5 §2.1, criterion 2: nothing of everyday use changes level.
func TestHabitualKeepsItsLevels(t *testing.T) {
	c := mustNew(t, "")
	cases := loadCorpus(t, "habitual.jsonl")
	if len(cases) < 100 {
		t.Fatalf("habitual.jsonl has %d cases, want at least 100", len(cases))
	}
	for _, k := range cases {
		r, ok := safeClassify(t, c, k.argv())
		if want := k.level(t); ok && (r.Level != want || r.Special != k.Special) {
			t.Errorf("%q: %v special=%v (%s: %s), want %v special=%v", k.argv(), r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, want, k.Special)
		}
	}
}

// Spec M2.5 §2.1, criterion 6: every line that was 🟢 or 🟡 at M2 and is
// higher now is in raised.jsonl with its reason, and none of them is of
// everyday use.
func TestWhatM2Point5RaisedIsListed(t *testing.T) {
	c := mustNew(t, "")
	raised := map[string]bool{}
	for _, k := range loadCorpus(t, "raised.jsonl") {
		if k.Why == "" {
			t.Errorf("raised.jsonl: %q has no reason", k.Sh)
		}
		raised[k.Sh] = true
	}
	for _, k := range loadCorpus(t, "habitual.jsonl") {
		if k.Argv == nil && raised[k.Sh] {
			t.Errorf("%q is in habitual.jsonl and in raised.jsonl", k.Sh)
		}
	}
	seen := map[string]bool{}
	for _, k := range loadCorpus(t, "m2-levels.jsonl") {
		seen[k.Sh] = true
		r, ok := safeClassify(t, c, k.argv())
		if !ok {
			continue
		}
		switch was := k.level(t); {
		case r.Level > was && !raised[k.Sh]:
			t.Errorf("%q was %v at M2 and is %v (%s: %s): fix it or list it in raised.jsonl", k.Sh, was, r.Level, r.Primary.Rule, r.Primary.Reason)
		case r.Level <= was && raised[k.Sh]:
			t.Errorf("%q is in raised.jsonl but is still %v", k.Sh, r.Level)
		}
	}
	for sh := range raised {
		if !seen[sh] {
			t.Errorf("raised.jsonl: %q is not in m2-levels.jsonl", sh)
		}
	}
}

// Spec M2.5 §2.1, criterion 5: no command of any corpus makes Classify
// panic. `dumpsys` alone did at M2.
func TestNoCorpusCommandPanics(t *testing.T) {
	c := mustNew(t, "")
	var all [][]string
	for _, name := range []string{"habitual.jsonl", "m2-levels.jsonl", "raised.jsonl"} {
		for _, k := range loadCorpus(t, name) {
			all = append(all, k.argv())
		}
	}
	for _, k := range differentialCases(t) {
		all = append(all, sh(withSentinel(k.Sh)))
	}
	// Every command on its own and cut after each word: a command with
	// fewer words than its check expects must not panic either.
	for _, argv := range all {
		safeClassify(t, c, argv)
		if len(argv) == 2 && argv[0] == "shell" {
			words := strings.Fields(argv[1])
			for i := 1; i < len(words); i++ {
				safeClassify(t, c, sh(strings.Join(words[:i], " ")))
			}
		}
	}
	// 🔴 since T2 of the manual validations: it includes the notification
	// service's dump (TestDumpsysOfTheNotificationsIsRed).
	if r, ok := safeClassify(t, c, sh("dumpsys")); ok && (r.Level != Red || r.Primary.Rule != "android.dumpsys") {
		t.Errorf("dumpsys: %v (%s: %s), want red by android.dumpsys", r.Level, r.Primary.Rule, r.Primary.Reason)
	}
}

// safeClassify classifies a command and reports a panic as a test error.
func safeClassify(t *testing.T, c *Classifier, argv []string) (r Result, ok bool) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("%q: panic: %v", argv, p)
			ok = false
		}
	}()
	return c.Classify(argv), true
}

// mkshNames reads testdata/mksh-builtins.tsv: name, tab, kind.
func mkshNames(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "mksh-builtins.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		name, kind, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if !ok {
			t.Fatalf("mksh-builtins.tsv: bad line %q", line)
		}
		out[name] = kind
	}
	return out
}

// Spec M2.5 §4.7: every builtin and predefined alias of the real mksh is
// admitted with a grammar or never analysed; nothing in between. Reserved
// words are the parser's: a construct the evaluator admits or not.
func TestEveryMkshBuiltinIsAdmittedOrSpecial(t *testing.T) {
	c := mustNew(t, "")
	names := mkshNames(t)
	if len(names) < 40 {
		t.Fatalf("mksh-builtins.tsv has %d names, want at least 40", len(names))
	}
	for name, kind := range names {
		if kind == "reserved" {
			continue
		}
		_, admitted := admittedBuiltins[name]
		switch {
		case admitted, scriptBuiltins[name]:
		case otherBuiltins[name]:
			for _, line := range []string{name, name + " x", "command " + name + " x", "builtin " + name + " x"} {
				if r := c.Classify(sh(line)); r.Level != Red || !r.Special {
					t.Errorf("%q: %v special=%v, want red special (%s is a %s of mksh)", line, r.Level, r.Special, name, kind)
				}
			}
		default:
			t.Errorf("%s is a %s of mksh: admit it in admittedBuiltins with a grammar or list it in otherBuiltins", name, kind)
		}
	}
	for name := range otherBuiltins {
		if _, ok := names[name]; !ok {
			t.Errorf("otherBuiltins lists %s, which the recorded mksh does not have", name)
		}
	}
}
