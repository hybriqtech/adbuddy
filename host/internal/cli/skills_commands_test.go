package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

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

// uncheckedCmdRe is a line that looks like an adb or adbuddy command outside a
// checked block (prose, an indented block, a block of another language):
// spec §1.1 wants every example annotated and checked.
var uncheckedCmdRe = regexp.MustCompile(`(?i)^(\$|>|PS>)?\s*(\S*[/\\])?adb(uddy)?(\.exe)?(\s|$)`)

// adbProgramRe is the first word of an example that names the adb program.
var adbProgramRe = regexp.MustCompile(`(?i)^(.*[/\\])?adb(\.exe)?$`)

// installGroupRe is a group of short options of install that includes -g.
var installGroupRe = regexp.MustCompile(`^-[a-z]*g[a-z]*$`)

// wipeRe matches "wipe" not preceded by a letter: wipe, --wipe_data, /wipe, pm
// wipe; not swipe (input swipe is the way to drag an icon).
var wipeRe = regexp.MustCompile(`(^|[^a-z])wipe`)

// skillAdbuddySubcommands are the subcommands a skill may use.
var skillAdbuddySubcommands = map[string]bool{
	"screen": true, "tap": true, "type": true, "foreground": true,
	"screenshot": true, "send": true, "journal": true, "undo": true,
}

// allowedRecursiveRm is the one recursive delete a recipe may carry: storage-cleanup
// empties its own trash folder with it (red: the user confirms it). The match is
// the exact command, field by field, in that one skill: no patterns, no prefixes.
const allowedRecursiveRmFile = "storage-cleanup/SKILL.md"

var allowedRecursiveRm = [][]string{
	{"shell", "rm", "-r", "/sdcard/ADBuddy-papelera"},
}

// shellSeparators turn what a shell treats as a command boundary or a
// grouping into a space, so that `x;rm -r y` and `$(rm -r y)` inside one
// quoted argument still show their rm.
var shellSeparators = strings.NewReplacer(";", " ", "|", " ", "&", " ", "(", " ", ")", " ", "$", " ", "`", " ", "<", " ", ">", " ", "{", " ", "}", " ", "\"", " ", "'", " ")

// forbiddenRecipe says whether a command does what no recipe does, however
// well it is annotated (spec §2.3): clear an app's data, delete recursively,
// wipe, reboot, or install granting every permission.
func forbiddenRecipe(file string, fields []string) bool {
	for _, ok := range allowedRecursiveRm {
		if file == allowedRecursiveRmFile && slices.Equal(fields, ok) {
			return false
		}
	}
	var toks []string
	for _, f := range fields {
		toks = append(toks, strings.Fields(shellSeparators.Replace(strings.ToLower(f)))...)
	}
	joined := strings.Join(toks, " ")
	if wipeRe.MatchString(joined) || strings.Contains(joined, "reboot") ||
		strings.Contains(joined, "pm clear") || strings.Contains(joined, "package clear") ||
		strings.Contains(joined, "master_clear") || strings.Contains(joined, "factory_reset") {
		return true
	}
	for i, tk := range toks {
		switch {
		case tk == "find" || strings.HasSuffix(tk, "/find"):
			if slices.Contains(toks[i+1:], "-delete") {
				return true
			}
		case tk == "uninstall" && i > 0 && (toks[i-1] == "pm" || toks[i-1] == "package"):
			// Only for one user, never for the whole phone.
			rest := toks[i+1:]
			if !slices.ContainsFunc(rest, func(o string) bool { return o == "--user" || strings.HasPrefix(o, "--user=") }) {
				return true
			}
		case tk == "rm" || strings.HasSuffix(tk, "/rm"):
			for _, o := range toks[i+1:] {
				if o == "--recursive" || (strings.HasPrefix(o, "-") && !strings.HasPrefix(o, "--") && strings.Contains(o, "r")) {
					return true
				}
			}
		case strings.HasPrefix(tk, "install"):
			for _, o := range toks[i+1:] {
				if installGroupRe.MatchString(o) || strings.HasPrefix(o, "--grant") {
					return true
				}
			}
		}
	}
	return false
}

// skillExamplesIn extracts the examples from every Markdown file of fsys and
// returns the problems found in how the blocks are written.
func skillExamplesIn(fsys fs.FS) ([]skillExample, []string) {
	var out []skillExample
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		kind := ""
		other := false // inside a block of another language (sh, text…)
		fence := 0     // backticks of the fence that opened that block
		for i, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			trim := strings.TrimSpace(line)
			ticks := len(trim) - len(strings.TrimLeft(trim, "`"))
			closing := ticks >= 3 && ticks == len(trim) // only backticks: a closing fence
			switch {
			case kind == "" && !other && strings.HasPrefix(trim, "~~~"):
				bad("%s:%d: use ``` fences, never ~~~: the blocks of ```adb are checked", p, i+1)
			case kind == "" && !other && ticks >= 3:
				info := strings.TrimLeft(trim, "`")
				word := strings.ToLower(strings.Fields(info + " x")[0])
				switch {
				case ticks == 3 && (info == "adb" || info == "adbuddy"):
					kind = info
				case strings.HasPrefix(word, "adb") || strings.Contains(strings.ToLower(info), "{") && strings.Contains(strings.ToLower(info), "adb"):
					// adb{.x}, adbuddy,x, {.adb}: a renderer may still call it adb
					bad("%s:%d: the fence must be exactly ```adb or ```adbuddy (lower case, three backticks, nothing else): %q", p, i+1, trim)
					other, fence = true, ticks
				default:
					other, fence = true, ticks
				}
			case other && closing && ticks >= fence:
				other = false
			case kind != "" && closing:
				kind = ""
			case kind != "" && trim != "":
				ex := skillExample{file: p, line: i + 1, kind: kind, text: trim}
				if kind == "adb" {
					if m := annotationRe.FindStringSubmatch(trim); m != nil {
						ex.text, ex.level = m[1], m[2]
					}
				}
				out = append(out, ex)
			case kind == "" && uncheckedCmdRe.MatchString(trim):
				// prose, an indented block or a block of another language
				bad("%s:%d: a command that starts with adb or adbuddy goes in a ```adb or ```adbuddy block, where it is checked: %q", p, i+1, trim)
			}
		}
		if kind != "" || other {
			bad("%s: a code block is not closed", p)
		}
		return nil
	})
	if err != nil {
		bad("walking the skills: %v", err)
	}
	return out, errs
}

// skillExamples extracts the examples from every skill, failing t on a block
// that is badly written. Reused by the task that checks the skills' text.
func skillExamples(t *testing.T) []skillExample {
	t.Helper()
	out, errs := skillExamplesIn(workspace.Skills())
	for _, e := range errs {
		t.Error(e)
	}
	return out
}

// splitExample splits a command into arguments the way a shell does for
// quotes, without expanding anything. Outside quotes only plain characters are
// accepted, and inside double quotes nothing the shell would expand: in Git
// Bash $(…), a backtick, a pipe or a redirection run on the PC before adb
// starts (spec §2.5). Scripts go in single quotes.
func splitExample(s string) ([]string, error) {
	const plain = "_./:=,+@%-"
	errQuote := errors.New("quote the script in single quotes (spec §2.5)")
	var out []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else if quote == '"' && strings.ContainsRune("$`\\!", r) {
				return nil, errQuote
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
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(plain, r):
			cur.WriteRune(r)
			inWord = true
		default:
			return nil, errQuote
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

// checkSkillExamples checks every example and returns the problems.
func checkSkillExamples(cl *classify.Classifier, examples []skillExample) []string {
	var errs []string
	for _, ex := range examples {
		where := fmt.Sprintf("%s:%d", ex.file, ex.line)
		for _, e := range checkSkillExample(cl, ex) {
			errs = append(errs, where+": "+e)
		}
	}
	return errs
}

func checkSkillExample(cl *classify.Classifier, ex skillExample) (errs []string) {
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	defer func() {
		if r := recover(); r != nil {
			bad("checking %q panicked: %v", ex.text, r)
		}
	}()
	fields, err := splitExample(ex.text)
	if err != nil {
		bad("%v: %q", err, ex.text)
		return
	}
	switch ex.kind {
	case "adb":
		if adbProgramRe.MatchString(fields[0]) {
			bad("write the command without the adb prefix: %q", ex.text)
			return
		}
		if forbiddenRecipe(ex.file, fields) {
			bad("a recipe never does this (spec §2.3): %q", ex.text)
			return
		}
		g, argv := classify.SplitGlobal(fields)
		if g.Serial != "" || len(g.Forbidden) > 0 || len(g.Pass) > 0 {
			bad("skills never use adb's global options: %q", ex.text)
			return
		}
		if len(argv) == 0 {
			bad("empty command: %q", ex.text)
			return
		}
		got := cl.Classify(argv).Level.String()
		switch {
		case ex.level == "":
			bad("no annotation (the classifier says %s): add `# %s` to %q", got, got, ex.text)
		case got != ex.level:
			bad("annotated %s but the classifier says %s: %q", ex.level, got, ex.text)
		}
	case "adbuddy":
		if adbProgramRe.MatchString(fields[0]) || fields[0] == "adbuddy" {
			bad("write the subcommand without the adbuddy prefix: %q", ex.text)
			return
		}
		if !skillAdbuddySubcommands[fields[0]] {
			bad("a skill does not use `adbuddy %s` (allowed: screen, tap, type, foreground, screenshot, send, journal, undo): %q", fields[0], ex.text)
			return
		}
		// A fresh tree per example: ParseFlags leaves values in the flags.
		root := NewRoot(&App{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }})
		cmd, rest, err := root.Find(fields)
		if err != nil || cmd == root {
			bad("adbuddy has no subcommand %q", fields[0])
			return
		}
		// ParseFlags knows the persistent flags (--json), -ofile, --x=y and
		// values that start with a dash; it fails on an unknown flag.
		if err := cmd.ParseFlags(rest); err != nil {
			bad("%q: %v", ex.text, err)
			return
		}
		if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
			bad("%q: %v", ex.text, err)
		}
	default:
		bad("unknown block kind %q", ex.kind)
	}
	return
}

func TestSkillExamples(t *testing.T) {
	cl, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	examples := skillExamples(t)
	for _, e := range checkSkillExamples(cl, examples) {
		t.Error(e)
	}
	kinds := map[string]int{}
	for _, ex := range examples {
		kinds[ex.kind]++
	}
	if kinds["adb"] == 0 || kinds["adbuddy"] == 0 {
		t.Fatalf("the skills need at least one ```adb and one ```adbuddy example, found %v", kinds)
	}
}

// skillProblems runs the whole check over one Markdown file.
func skillProblems(t *testing.T, cl *classify.Classifier, md string) []string {
	t.Helper()
	fsys := fstest.MapFS{"x/SKILL.md": {Data: []byte(md)}}
	examples, errs := skillExamplesIn(fsys)
	return append(errs, checkSkillExamples(cl, examples)...)
}

// skillProblemsAt is skillProblems for a file at the given path of the skills folder.
func skillProblemsAt(t *testing.T, cl *classify.Classifier, path, md string) []string {
	t.Helper()
	examples, errs := skillExamplesIn(fstest.MapFS{path: {Data: []byte(md)}})
	return append(errs, checkSkillExamples(cl, examples)...)
}

func adbBlock(lines ...string) string {
	return "```adb\n" + strings.Join(lines, "\n") + "\n```\n"
}

func adbuddyBlock(lines ...string) string {
	return "```adbuddy\n" + strings.Join(lines, "\n") + "\n```\n"
}

// TestSkillExamplesCatches keeps the guard honest: each of these must produce
// at least one problem, and the good ones none.
func TestSkillExamplesCatches(t *testing.T) {
	cl, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"level differs":          adbBlock("shell getprop ro.product.model   # red"),
		"no annotation":          adbBlock("shell getprop ro.product.model"),
		"bogus subcommand":       adbuddyBlock("bogus"),
		"unknown flag":           adbuddyBlock("screen --nope"),
		"extra argument":         adbuddyBlock("foreground extra"),
		"adb in sh block":        "```sh\nadb shell ls\n```\n",
		"pm clear":               adbBlock("shell pm clear com.example.app   # red"),
		"serial":                 adbBlock("-s ABC123 shell getprop ro.product.model   # green"),
		"unclosed block":         "```adb\nshell getprop ro.product.model   # green\n",
		"backtick":               adbBlock("shell echo `whoami`   # green"),
		"pipe":                   adbBlock("shell pm list packages -3 | wc -l   # green"),
		"command substitution":   adbBlock("shell echo $(rm -r ~/Documents)   # red"),
		"semicolon":              adbBlock("shell ls ; ls   # green"),
		"redirection":            adbBlock("shell ls > out.txt   # green"),
		"dollar in double quote": adbBlock(`shell "echo $HOME"   # green`),
		"backslash in dq":        adbBlock(`shell "echo \x"   # green`),
		"fence with a title":     "```adb title\nshell pm clear com.example.app\n```\n",
		"upper case fence":       "```ADB\nshell pm clear com.example.app\n```\n",
		"tilde fence sh":         "~~~sh\nadb shell ls\n~~~\n",
		"tilde fence adb":        "~~~adb\nshell getprop ro.product.model   # green\n~~~\n",
		"dollar prompt in sh":    "```sh\n$ adb shell rm -r /sdcard/Download\n```\n",
		"adb in prose":           "Run this:\n\nadb shell pm clear com.example.app\n",
		"adb indented block":     "Run this:\n\n    adb shell pm clear com.example.app\n",
		"adb.exe in prose":       "PS> adb.exe shell ls\n",
		"adb.exe prefix":         adbBlock("adb.exe shell getprop ro.product.model   # green"),
		"adb path prefix":        adbBlock("/usr/bin/adb shell getprop ro.product.model   # green"),
		"adb prefix":             adbBlock("adb shell getprop ro.product.model   # green"),
		"pm clear two spaces":    adbBlock("shell 'pm  clear com.example.app'   # red"),
		"cmd package clear":      adbBlock("shell cmd package clear com.example.app   # red"),
		"rm -fr":                 adbBlock("shell rm -fr /sdcard/x   # red"),
		"rm -r -f":               adbBlock("shell rm -r -f /sdcard/x   # red"),
		"rm long options":        adbBlock("shell rm --recursive --force /sdcard/x   # red"),
		"wipe_data":              adbBlock("shell recovery --wipe_data   # red"),
		"reboot broadcast":       adbBlock("shell am broadcast -a android.intent.action.REBOOT   # red"),
		"install -rg":            adbBlock("install -rg a.apk   # red"),
		"install --grant":        adbBlock("install --grant-runtime-permissions a.apk   # red"),
		"adbuddy serve":          adbuddyBlock("serve"),
		"adbuddy setup":          adbuddyBlock("setup"),
		"adbuddy annotation":     adbuddyBlock("screen   # green"),
		"adbuddy prefix":         adbuddyBlock("adbuddy screen"),
		"empty fence unclosed":   "```\nadb shell ls\n",
		// V33: fences that used to hide an adb block from the guard.
		"adb after a 4 backtick fence": "````text\nsome output\n````\n```adb\nshell getprop ro.product.model\n```\n",
		"3 ticks inside a 4 fence":     "````text\n```\n````\n```adb\nshell getprop ro.product.model\n```\n",
		"attribute info string":        "```adb{.x}\nshell getprop ro.product.model   # green\n```\n",
		"braces info string":           "``` {.adb}\nshell getprop ro.product.model   # green\n```\n",
		"adbuddy attribute":            "```adbuddy{.x}\nscreen\n```\n",
	}
	for name, md := range bad {
		t.Run("bad/"+name, func(t *testing.T) {
			if errs := skillProblems(t, cl, md); len(errs) == 0 {
				t.Errorf("the guard let this through:\n%s", md)
			}
		})
	}
	for _, cmd := range []string{
		"shell recovery --wipe_data",
		"shell wipe data",
		"shell pm wipe com.example.app",
		"shell x_wipe",
		"shell 1wipe",
	} {
		t.Run("wipe/"+cmd, func(t *testing.T) {
			errs := skillProblems(t, cl, adbBlock(cmd+"   # red"))
			if len(errs) != 1 || !strings.Contains(errs[0], "a recipe never does this") {
				t.Errorf("want only the recipe filter to reject it, got %v", errs)
			}
		})
	}
	good := map[string]string{
		"getprop":                     adbBlock("shell getprop ro.product.model   # green"),
		"input swipe":                 adbBlock("shell input swipe 540 1200 540 400 600   # yellow"),
		"packages":                    adbBlock("shell pm list packages -3   # green"),
		"settings":                    adbBlock("shell settings get global window_animation_scale   # green"),
		"screenshot -o":               adbuddyBlock("screenshot -oshot.png"),
		"screen":                      adbuddyBlock("screen"),
		"send":                        adbuddyBlock(`send outbox/device-profile.md --caption "Device profile"`),
		"foreground":                  adbuddyBlock("foreground"),
		"other language":              "```text\nsome output\n```\n\nPlain prose about the phone.\n",
		"4 fence with a nested block": "````text\n```adb\nshell pm clear com.example.app\n```\n````\n\n" + adbBlock("shell getprop ro.product.model   # green"),
		"find without delete":         adbBlock("shell find /sdcard/DCIM -type d   # green"),
		"uninstall for one user":      adbBlock("shell pm uninstall --user 0 com.example.app   # yellow"),
	}
	for name, md := range good {
		t.Run("good/"+name, func(t *testing.T) {
			if errs := skillProblems(t, cl, md); len(errs) != 0 {
				t.Errorf("a good example failed: %v", errs)
			}
		})
	}
}

// TestRecursiveRmException keeps the one exception to the rm -r ban exact: only
// that command, only in storage-cleanup, and every negative case annotated red
// so that the recipe filter is the only reason it can fail.
func TestRecursiveRmException(t *testing.T) {
	cl, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	const own = allowedRecursiveRmFile
	if errs := skillProblemsAt(t, cl, own, adbBlock("shell rm -r /sdcard/ADBuddy-papelera   # red")); len(errs) != 0 {
		t.Errorf("the exact command must pass in %s: %v", own, errs)
	}
	for _, cmd := range []string{
		"shell rm -r /sdcard/ADBuddy-papelera/..",
		"shell rm -r /sdcard/ADBuddy-papelera/",
		"shell rm -r /sdcard/ADBuddy-papelera/x",
		"shell rm -r //sdcard/ADBuddy-papelera",
		"shell rm -r /sdcard/adbuddy-papelera",
		"shell rm -r /sdcard/ADBuddy-papelera2",
		"shell rm -rf /sdcard/ADBuddy-papelera",
		"shell rm -r -f /sdcard/ADBuddy-papelera",
		"shell 'rm -r /sdcard/ADBuddy-papelera'",
		"shell rm -r /sdcard",
	} {
		t.Run("bad/"+cmd, func(t *testing.T) {
			errs := skillProblemsAt(t, cl, own, adbBlock(cmd+"   # red"))
			if len(errs) != 1 || !strings.Contains(errs[0], "a recipe never does this") {
				t.Errorf("want only the recipe filter to reject it, got %v", errs)
			}
		})
	}
	for _, other := range []string{"organize-files/SKILL.md", "x/SKILL.md"} {
		errs := skillProblemsAt(t, cl, other, adbBlock("shell rm -r /sdcard/ADBuddy-papelera   # red"))
		if len(errs) != 1 || !strings.Contains(errs[0], "a recipe never does this") {
			t.Errorf("the exact command must fail outside %s (%s): %v", own, other, errs)
		}
	}
}

// V33: what no recipe does, however it is hidden in a quoted argument. Every
// case is annotated red so that the recipe filter is the only reason it fails.
func TestForbiddenRecipesAreCaught(t *testing.T) {
	cl, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{
		"shell 'echo $(rm -r /sdcard/x)'",
		"shell 'echo `rm -r /sdcard/x`'",
		"shell 'ls;rm -r /sdcard/x'",
		"shell 'ls ; rm -rf /sdcard/x'",
		"shell 'ls && rm --recursive /sdcard/x'",
		"shell find /sdcard/x -delete",
		"shell find /sdcard/x -type f -delete",
		"shell /system/bin/find /sdcard/x -delete",
		"shell 'cd /sdcard;find . -delete'",
		"shell pm uninstall com.example.app",
		"shell pm uninstall -k com.example.app",
		"shell cmd package uninstall com.example.app",
		"shell am broadcast -a android.intent.action.MASTER_CLEAR",
		"shell am broadcast -a android.intent.action.FACTORY_RESET",
		"shell am broadcast -a android.intent.action.FACTORY_RESET --es android.intent.extra.REASON x",
	} {
		t.Run(cmd, func(t *testing.T) {
			errs := skillProblems(t, cl, adbBlock(cmd+"   # red"))
			if len(errs) != 1 || !strings.Contains(errs[0], "a recipe never does this") {
				t.Errorf("want only the recipe filter to reject it, got %v", errs)
			}
		})
	}
	for _, cmd := range []string{
		"shell pm uninstall --user 0 com.example.app",
		"shell pm uninstall --user=0 com.example.app",
		"shell find /sdcard/x -type f",
	} {
		if forbiddenRecipe("x/SKILL.md", mustSplit(t, cmd)) {
			t.Errorf("%q is not a forbidden recipe", cmd)
		}
	}
}

func mustSplit(t *testing.T, cmd string) []string {
	t.Helper()
	f, err := splitExample(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
