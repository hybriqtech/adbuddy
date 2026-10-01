package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// skillExample is one command from a ```adb or ```adbuddy block of a skill.
type skillExample struct {
	file  string // path inside the skills folder
	line  int
	kind  string // "adb" or "adbuddy"
	text  string // the command, without the annotation
	level string // the annotation of an adb example: green, yellow or red
}

var annotationRe = regexp.MustCompile(`^(.*\S)\s+#\s+(green|yellow|red)\s*$`)

// forbiddenRecipeRe is what no recipe does, however well it is annotated
// (spec §2.3): clear an app's data, delete recursively, wipe, reboot, or
// install granting every permission.
var forbiddenRecipeRe = regexp.MustCompile(`\bpm clear\b|\brm -rf\b|\bwipe\b|\breboot\b|\binstall\b.*\s-g\b`)

// skillExamples extracts the examples from every Markdown file of every skill.
func skillExamples(t *testing.T) []skillExample {
	t.Helper()
	skills := workspace.Skills()
	var out []skillExample
	err := fs.WalkDir(skills, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		data, err := fs.ReadFile(skills, p)
		if err != nil {
			return err
		}
		kind := ""
		other := false // inside a block of another language (sh, text…)
		for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			trim := strings.TrimSpace(line)
			switch {
			case kind == "" && !other && (trim == "```adb" || trim == "```adbuddy"):
				kind = strings.TrimPrefix(trim, "```")
			case kind == "" && !other && strings.HasPrefix(trim, "```"):
				other = true
			case other && trim == "```":
				other = false
			case other && (strings.HasPrefix(trim, "adb ") || strings.HasPrefix(trim, "adbuddy ")):
				// An unchecked command: spec §1.1 wants every example annotated.
				t.Errorf("%s:%d: a command that starts with adb or adbuddy goes in a ```adb or ```adbuddy block, where it is checked: %q", p, i+1, trim)
			case kind != "" && trim == "```":
				kind = ""
			case kind != "" && trim != "":
				ex := skillExample{file: p, line: i + 1, kind: kind, text: trim}
				if kind == "adb" {
					if m := annotationRe.FindStringSubmatch(trim); m != nil {
						ex.text, ex.level = m[1], m[2]
					}
				}
				out = append(out, ex)
			}
		}
		if kind != "" || other {
			t.Errorf("%s: a code block is not closed", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// splitExample splits a command into arguments the way a shell does for
// quotes, without expanding anything. Examples use single quotes for scripts:
// a $ or a backtick inside double quotes would expand on the PC.
func splitExample(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else if quote == '"' && (r == '$' || r == '`') {
				return nil, errors.New("use single quotes for a script with $ or a backtick")
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inWord {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, errors.New("empty command")
	}
	return out, nil
}

func TestSkillExamples(t *testing.T) {
	cl, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, ex := range skillExamples(t) {
		where := fmt.Sprintf("%s:%d", ex.file, ex.line)
		fields, err := splitExample(ex.text)
		if err != nil {
			t.Errorf("%s: %v: %q", where, err, ex.text)
			continue
		}
		checked++
		switch ex.kind {
		case "adb":
			if fields[0] == "adb" {
				t.Errorf("%s: write the command without the adb prefix: %q", where, ex.text)
				continue
			}
			if forbiddenRecipeRe.MatchString(ex.text) {
				t.Errorf("%s: a recipe never does this (spec §2.3): %q", where, ex.text)
				continue
			}
			g, argv := classify.SplitGlobal(fields)
			if g.Serial != "" || len(g.Forbidden) > 0 || len(g.Pass) > 0 {
				t.Errorf("%s: skills never use adb's global options: %q", where, ex.text)
				continue
			}
			got := cl.Classify(argv).Level.String()
			switch {
			case ex.level == "":
				t.Errorf("%s: no annotation (the classifier says %s): add `# %s` to %q", where, got, got, ex.text)
			case got != ex.level:
				t.Errorf("%s: annotated %s but the classifier says %s: %q", where, ex.level, got, ex.text)
			}
		case "adbuddy":
			if fields[0] == "adbuddy" {
				t.Errorf("%s: write the subcommand without the adbuddy prefix: %q", where, ex.text)
				continue
			}
			// A fresh tree per example: ParseFlags leaves values in the flags.
			root := NewRoot(&App{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }})
			cmd, rest, err := root.Find(fields)
			if err != nil || cmd == root {
				t.Errorf("%s: adbuddy has no subcommand %q", where, fields[0])
				continue
			}
			// ParseFlags knows the persistent flags (--json), -ofile, --x=y and
			// values that start with a dash; it fails on an unknown flag.
			if err := cmd.ParseFlags(rest); err != nil {
				t.Errorf("%s: %q: %v", where, ex.text, err)
				continue
			}
			if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
				t.Errorf("%s: %q: %v", where, ex.text, err)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no skill example was found: the blocks must start with ```adb or ```adbuddy")
	}
}
