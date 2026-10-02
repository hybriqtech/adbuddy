package cli

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// requiredDocs are the guides the M6d plan delivers (spec §1). Each task adds
// its own until the whole list is here.
var requiredDocs = []string{
	"README.md", "README.es.md",
	"docs/index.md", "docs/install.md", "docs/usage.md", "docs/safety.md", "docs/faq.md", "docs/troubleshooting.md",
	"docs/setup/index.md", "docs/setup/es/index.md",
}

var (
	docLinkRe    = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	docRefLinkRe = regexp.MustCompile(`(?m)^ {0,3}\[[^\]\n]+\]:\s+(\S+)`)
	docInlineRe  = regexp.MustCompile("`([^`\n]+)`")
	docNumTokRe  = regexp.MustCompile(`[\d.]+`)
	docIPv4Re    = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
	docPrivateRe = regexp.MustCompile(`C:\\Users|/home/|/Users/|emulator-\d+|ANDROID_SERIAL=`)
	docMarkerRe  = regexp.MustCompile(`<[^<>\n]*>`)
	docVersionRe = regexp.MustCompile(`(?i)(^|[^a-z])(v|version\s*)$`)
	// Documentation ranges (RFC 5737) and the first CGNAT block, which the
	// product's own --help uses as its Tailscale example.
	docIPRanges = []string{"192.0.2.", "198.51.100.", "203.0.113.", "100.64.0."}
	docIPExact  = []string{"0.0.0.0", "127.0.0.1"}
	// Languages whose blocks hold commands a person types.
	docShellLangs = map[string]bool{"": true, "sh": true, "bash": true, "shell": true, "console": true, "powershell": true, "pwsh": true, "ps1": true}
)

// docsRepoRoot walks up from the package folder to the repository root.
func docsRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docs", "_config.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root (a folder with docs/_config.yml) not found")
		}
		dir = parent
	}
}

// docExcludes returns the `exclude` entries of docs/_config.yml, without the
// trailing slash. A missing or broken file excludes nothing.
func docExcludes(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_config.yml"))
	if err != nil {
		return nil
	}
	var cfg struct {
		Exclude []string `yaml:"exclude"`
	}
	if yaml.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	for i, e := range cfg.Exclude {
		cfg.Exclude[i] = strings.TrimSuffix(strings.TrimPrefix(e, "./"), "/")
	}
	return cfg.Exclude
}

// docExcluded reports whether rel (relative to docs/, with forward slashes) is
// left out of the site: Jekyll matches each exclude as a glob against the path
// and against its parent folders; names starting with "_" or "." are left out
// too.
func docExcluded(excludes []string, rel string) bool {
	parts := strings.Split(rel, "/")
	for i := range parts {
		if strings.HasPrefix(parts[i], "_") || strings.HasPrefix(parts[i], ".") {
			return true
		}
		prefix := strings.Join(parts[:i+1], "/")
		for _, e := range excludes {
			if ok, _ := path.Match(e, prefix); ok {
				return true
			}
		}
	}
	return false
}

// publishedDocs lists, relative to root and with forward slashes, the Markdown
// files GitHub Pages publishes plus the READMEs: docs/_config.yml's `exclude`
// says what stays out.
func publishedDocs(t *testing.T, root string) []string {
	t.Helper()
	excludes := docExcludes(root)
	var out []string
	for _, f := range []string{"README.md", "README.es.md"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			out = append(out, f)
		}
	}
	docs := filepath.Join(root, "docs")
	err := filepath.WalkDir(docs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(docs, p)
		rel = filepath.ToSlash(rel)
		if rel != "." && docExcluded(excludes, rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, "docs/"+rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// docBlock is a fenced block with its language.
type docBlock struct {
	lang  string
	lines []string
	line  int // line number of the first line inside the block
}

// splitDoc splits a Markdown text into the fenced blocks, the rest (prose) and
// the line numbers of "~~~" fences, which the guards do not understand.
func splitDoc(text string) (blocks []docBlock, prose string, tildes []int) {
	var cur *docBlock
	var p strings.Builder
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case cur == nil && strings.HasPrefix(trim, "```"):
			lang := ""
			if f := strings.Fields(strings.TrimPrefix(trim, "```")); len(f) > 0 {
				lang = strings.ToLower(f[0])
			}
			cur = &docBlock{lang: lang, line: i + 2}
		case cur != nil && trim == "```":
			blocks = append(blocks, *cur)
			cur = nil
		case cur != nil:
			cur.lines = append(cur.lines, line)
		default:
			if strings.HasPrefix(trim, "~~~") {
				tildes = append(tildes, i+1)
			}
			p.WriteString(line)
			p.WriteByte('\n')
		}
	}
	if cur != nil {
		blocks = append(blocks, *cur)
	}
	return blocks, p.String(), tildes
}

// checkDocFences returns a message for every "~~~" fence.
func checkDocFences(file, text string) []string {
	var bad []string
	_, _, tildes := splitDoc(text)
	for _, n := range tildes {
		bad = append(bad, file+":"+strconv.Itoa(n)+": use ``` fences, not ~~~ (the guards do not read them)")
	}
	return bad
}

// checkDocLinks returns one message per relative link whose target does not
// exist, and, for a page under docs/, per link that leaves docs/ or reaches an
// excluded file (a 404 on GitHub Pages). Jekyll's convention is followed: "x/"
// is x/index.md, "x" is x.md, x/index.md or the file x.
func checkDocLinks(root, file, text string) []string {
	var bad []string
	_, prose, _ := splitDoc(text)
	var targets []string
	for _, m := range docLinkRe.FindAllStringSubmatch(prose, -1) {
		targets = append(targets, m[1])
	}
	for _, m := range docRefLinkRe.FindAllStringSubmatch(prose, -1) {
		targets = append(targets, m[1])
	}
	excludes := docExcludes(root)
	for _, raw := range targets {
		target := raw
		// Links starting with "/" are skipped: they depend on the site's
		// baseurl on Pages, which a file check cannot know.
		if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") {
			continue
		}
		if i := strings.IndexAny(target, "#?"); i >= 0 {
			target = target[:i]
		}
		if target == "" {
			continue
		}
		base := filepath.Join(root, filepath.Dir(filepath.FromSlash(file)), filepath.FromSlash(target))
		var candidates []string
		switch {
		case strings.HasSuffix(target, "/"):
			candidates = []string{filepath.Join(base, "index.md")}
		case filepath.Ext(target) == "":
			candidates = []string{base + ".md", filepath.Join(base, "index.md"), base}
		default:
			candidates = []string{base}
		}
		found := ""
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				found = c
				break
			}
		}
		if found == "" {
			bad = append(bad, file+": the link "+raw+" points to nothing")
			continue
		}
		if !strings.HasPrefix(file, "docs/") {
			continue
		}
		rel, err := filepath.Rel(filepath.Join(root, "docs"), found)
		rel = filepath.ToSlash(rel)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			bad = append(bad, file+": the link "+raw+" leaves docs/: on Pages it would be a 404; use a https://github.com/hybriqtech/adbuddy/blob/main/… link")
		} else if docExcluded(excludes, rel) {
			bad = append(bad, file+": the link "+raw+" points to an excluded file: on Pages it would be a 404; use a https://github.com/hybriqtech/adbuddy/blob/main/… link")
		}
	}
	return bad
}

// docSegments returns the `adbuddy …` commands of one line as lists of words:
// the prompt and a trailing " #" comment are dropped, quotes group words
// (nothing is expanded) and unquoted pipes, && , || and ; split it. <markers>
// become ARG and unquoted [optional groups] disappear (a bracket inside quotes
// is kept). A redirection (>, >>, 2>, &>, 2>&1, <) ends the command's
// arguments, as do the separators.
func docSegments(line string) [][]string {
	line = strings.TrimSpace(line)
	for _, p := range []string{"$ ", "PS> ", "> "} {
		line = strings.TrimPrefix(line, p)
	}
	line = docMarkerRe.ReplaceAllString(line, "ARG")
	if strings.HasSuffix(line, ` \`) || strings.HasSuffix(line, " `") {
		line = line[:len(line)-1] // line continuation
	}
	var segs [][]string
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	stop := false // after a redirection the words are not arguments
	endWord := func() {
		if inWord && !stop {
			words = append(words, word.String())
		}
		word.Reset()
		inWord = false
	}
	endSeg := func() {
		endWord()
		if len(words) > 0 {
			segs = append(segs, words)
		}
		words = nil
		stop = false
	}
	rs := []rune(line)
scan:
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			endWord()
		case r == '#' && !inWord:
			break scan
		case r == '>' || r == '<':
			// "2>" and "&>" carry the descriptor in the word before it.
			if w := word.String(); w == "&" || (w != "" && strings.Trim(w, "0123456789") == "") {
				word.Reset()
				inWord = false
			}
			endWord()
			stop = true
		case r == '[':
			if j := slices.IndexFunc(rs[i+1:], func(c rune) bool { return c == '[' || c == ']' }); j >= 0 && rs[i+1+j] == ']' {
				endWord()
				i += j + 1
			} else {
				word.WriteRune(r)
				inWord = true
			}
		case r == '|' || r == ';':
			endSeg()
		case r == '&' && i+1 < len(rs) && rs[i+1] == '&':
			endSeg()
			i++
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endSeg()
	var out [][]string
	for _, s := range segs {
		// Only ".exe" is case-insensitive: "ADBuddy setup guide" is prose.
		name := s[0]
		if strings.HasSuffix(strings.ToLower(name), ".exe") {
			name = name[:len(name)-4]
		}
		name = strings.TrimPrefix(strings.TrimPrefix(name, "./"), `.\`)
		if name == "adbuddy" {
			s[0] = "adbuddy"
			out = append(out, s)
		}
	}
	return out
}

// checkDocCommands validates every `adbuddy …` command of a guide against the
// real command tree: the subcommand exists, the flags are valid and the
// positional arguments are accepted. A command in running text whose first
// word is not a subcommand ("adbuddy is free") is not a command. In a
// ```console block only the lines with a prompt are commands; the rest is
// program output. A fresh tree is built per command because cobra keeps the
// parsed flag state in it.
func checkDocCommands(file, text string) []string {
	var bad []string
	check := func(where string, fields []string, inBlock bool) {
		seg := strings.Join(fields, " ")
		if len(fields) < 2 {
			return
		}
		tree := NewRoot(&App{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }})
		tree.InitDefaultHelpCmd()
		tree.InitDefaultHelpFlag()
		tree.InitDefaultVersionFlag() // cobra adds these two only when it runs
		tree.InitDefaultCompletionCmd()
		cmd, rest, err := tree.Find(fields[1:])
		if err != nil || cmd == tree {
			switch {
			case strings.HasPrefix(fields[1], "-"):
				if err := tree.ParseFlags(fields[1:]); err != nil {
					bad = append(bad, where+": "+seg+": "+err.Error())
				}
			case inBlock:
				bad = append(bad, where+": adbuddy has no subcommand "+fields[1]+": "+seg)
			}
			return
		}
		cmd.InitDefaultHelpFlag()
		if err := cmd.ParseFlags(rest); err != nil {
			bad = append(bad, where+": "+seg+": "+err.Error())
			return
		}
		args := cmd.Flags().Args()
		if cmd.Name() == "help" && cmd.Parent() == tree {
			if target, _, err := tree.Find(args); err != nil || (len(args) > 0 && target == tree) {
				bad = append(bad, where+": "+seg+": no such command to get help for")
			}
			return
		}
		if cmd.HasSubCommands() && !cmd.Runnable() && len(args) > 0 {
			bad = append(bad, where+": "+seg+": unknown subcommand "+args[0])
			return
		}
		if err := cmd.ValidateArgs(args); err != nil {
			bad = append(bad, where+": "+seg+": "+err.Error())
		}
	}
	blocks, prose, _ := splitDoc(text)
	for _, b := range blocks {
		if !docShellLangs[b.lang] {
			continue
		}
		for i, line := range b.lines {
			if b.lang == "console" {
				if tl := strings.TrimSpace(line); !strings.HasPrefix(tl, "$ ") && !strings.HasPrefix(tl, "PS> ") {
					continue
				}
			}
			for _, seg := range docSegments(line) {
				check(file+":"+strconv.Itoa(b.line+i), seg, true)
			}
		}
	}
	for _, m := range docInlineRe.FindAllStringSubmatch(prose, -1) {
		for _, seg := range docSegments(m[1]) {
			check(file, seg, false)
		}
	}
	return bad
}

// docIPPrivate reports whether an address is in a private or link-local range.
func docIPPrivate(ip string) bool {
	q := strings.Split(ip, ".")
	a, _ := strconv.Atoi(q[0])
	b, _ := strconv.Atoi(q[1])
	switch {
	case a == 10, a == 172 && b >= 16 && b <= 31, a == 192 && b == 168, a == 100 && b >= 64 && b <= 127, a == 169 && b == 254:
		return true
	}
	return false
}

// docIPAllowed reports whether an address is one the guides may show.
func docIPAllowed(ip string) bool {
	if slices.Contains(docIPExact, ip) {
		return true
	}
	for _, p := range docIPRanges {
		if strings.HasPrefix(ip, p) {
			return true
		}
	}
	return false
}

// checkDocPrivacy returns a message for every private-looking datum. A run of
// digits and dots counts as an address when it has exactly four numbers of
// 0-255 (so "1.0.410.0" and "1.2.3.4.5" are not) and no "v" or "version"
// before it (a four-component version such as "version 1.0.41.0"); that
// exemption does not apply to private ranges.
func checkDocPrivacy(file, text string) []string {
	var bad []string
	if m := docPrivateRe.FindString(text); m != "" {
		bad = append(bad, file+": contains "+m)
	}
	for _, loc := range docNumTokRe.FindAllStringIndex(text, -1) {
		raw := text[loc[0]:loc[1]]
		tok := strings.Trim(raw, ".") // a sentence's final period is not part of it
		start := loc[0] + len(raw) - len(strings.TrimLeft(raw, "."))
		if !docIPv4Re.MatchString(tok) {
			continue
		}
		valid := true
		for _, q := range strings.Split(tok, ".") {
			if n, _ := strconv.Atoi(q); n > 255 {
				valid = false
			}
		}
		if !valid || (docVersionRe.MatchString(text[:start]) && !docIPPrivate(tok)) || docIPAllowed(tok) {
			continue
		}
		bad = append(bad, file+": contains the IP address "+tok+" (use 192.0.2.x or <ip:port>)")
	}
	return bad
}

func docProblems(root, file, text string) []string {
	var bad []string
	bad = append(bad, checkDocLinks(root, file, text)...)
	bad = append(bad, checkDocCommands(file, text)...)
	bad = append(bad, checkDocPrivacy(file, text)...)
	bad = append(bad, checkDocFences(file, text)...)
	return bad
}

func TestDocsAreGuarded(t *testing.T) {
	root := docsRepoRoot(t)
	files := publishedDocs(t, root)
	for _, f := range requiredDocs {
		if !slices.Contains(files, f) {
			t.Errorf("%s is missing: the plan delivers it", f)
		}
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range docProblems(root, f, string(raw)) {
			t.Error(b)
		}
	}
}

func TestDocsGuardCatches(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/_config.yml", "exclude:\n  - releasing.md\n  - superpowers/\n")
	write("docs/x.md", "x")
	write("docs/releasing.md", "r")
	write("docs/xdir/other.md", "o")
	write("README.md", "r")
	write("LICENSE", "l")
	write("docs/page.md", "p")

	block := func(lang string, lines ...string) string {
		return "```" + lang + "\n" + strings.Join(lines, "\n") + "\n```\n"
	}
	sh := func(lines ...string) string { return block("sh", lines...) }
	inline := func(s string) string { return "Run `" + s + "` now.\n" }

	cases := []struct {
		name, file, text string
		want             bool // true: at least one message; false: none
	}{
		// Must be caught.
		{"link to a missing file", "docs/page.md", "[a](missing.md)", true},
		{"link to a folder without index", "docs/page.md", "[a](xdir/)", true},
		{"unknown subcommand in a block", "docs/page.md", sh("adbuddy bogus"), true},
		{"unknown flag in a block", "docs/page.md", sh("adbuddy setup --nope"), true},
		{"extra arguments in a block", "docs/page.md", block("console", "$ adbuddy screen extra extra"), true},
		{"unknown flag inline", "docs/page.md", inline("adbuddy setup --nope"), true},
		{"private IPv4", "docs/page.md", "Phone at 10.0.0." + "5.", true},
		{"user path", "docs/page.md", `Look in C:\Users\x.`, true},
		{"emulator serial", "docs/page.md", "Device emulator-5554.", true},
		{"windows unknown flag", "docs/page.md", block("powershell", `.\adbuddy.exe setup --nope`), true},
		{"unknown root flag", "docs/page.md", sh("adbuddy --nope"), true},
		{"link leaving docs", "docs/page.md", "[a](../README.md)", true},
		{"link to an excluded file", "docs/page.md", "[a](releasing.md)", true},
		{"tilde fence", "docs/page.md", "~~~sh\nadbuddy setup\n~~~\n", true},
		{"broken titled link", "docs/page.md", `[a](nope.md "t")`, true},
		{"broken reference link", "docs/page.md", "[a]: nope.md\n", true},
		{"help for a missing command", "docs/page.md", sh("adbuddy help bogus"), true},
		{"bogus subcommand of a group", "docs/page.md", sh("adbuddy env bogus"), true},
		{"unversioned four numbers", "docs/page.md", "Build 1.0.41.0 now.", true},
		{"version prefix on a private address", "docs/page.md", "Server version 192.168.1." + "5 up.", true},
		{"v prefix on a private address", "docs/page.md", "Seen v10.0.0." + "5 there.", true},
		{"version prefix on 10/8", "docs/page.md", "version 10.0.0." + "5", true},
		{"prose with capitals is not a command", "docs/page.md", inline("ADBuddy bogus"), false},
		{"version flag", "docs/page.md", sh("adbuddy --version"), false},
		{"completion command", "docs/page.md", sh("adbuddy completion bash"), false},
		{"redirect to a file", "docs/page.md", sh("adbuddy journal --json > j.json"), false},
		{"append redirect", "docs/page.md", sh("adbuddy journal >> j.txt"), false},
		{"stderr to stdout then tee", "docs/page.md", sh("adbuddy doctor 2>&1 | tee x"), false},
		{"both streams", "docs/page.md", sh("adbuddy doctor &> out.txt"), false},
		{"line continuation", "docs/page.md", sh(`adbuddy send x.md \`), false},
		{"powershell continuation", "docs/page.md", block("powershell", "adbuddy send x.md `"), false},
		{"bracket in quotes is kept", "docs/page.md", sh(`adbuddy send x.md --caption "[draft]"`), false},
		{"redirect does not hide a bad flag", "docs/page.md", sh("adbuddy setup --nope > x"), true},
		// Must pass.
		{"run with dash then run (one tree each, no panic)", "docs/page.md", sh("adbuddy run codex -- --model o4", "adbuddy run"), false},
		{"setup code connect", "docs/page.md", sh("adbuddy setup --code --connect 100.64.0.5:41234"), false},
		{"connect marker", "docs/page.md", sh("adbuddy connect <new ip:port>"), false},
		{"setup apk marker", "docs/page.md", sh("adbuddy setup --apk <your build>"), false},
		{"run optional", "docs/page.md", sh("adbuddy run [agent]"), false},
		{"marker with a non-ASCII character inline (no panic)", "docs/page.md", inline("adbuddy screen [ … ]"), false},
		{"marker with an accent inline", "docs/page.md", inline("adbuddy screen [é]"), false},
		{"marker with a non-ASCII character in a block", "docs/page.md", sh("adbuddy screen [ … ]", "adbuddy screen [é]"), false},
		{"undo optional inline", "docs/page.md", inline("adbuddy undo [last|session|<id>]"), false},
		{"pipe to jq", "docs/page.md", sh("adbuddy doctor --json | jq ."), false},
		{"root help", "docs/page.md", sh("adbuddy --help"), false},
		{"short help", "docs/page.md", sh("adbuddy setup -h"), false},
		{"help command", "docs/page.md", sh("adbuddy help setup"), false},
		{"double quotes", "docs/page.md", sh(`adbuddy tap "Sign in"`), false},
		{"single quotes", "docs/page.md", sh("adbuddy type 'hello world'"), false},
		{"send with caption", "docs/page.md", sh(`adbuddy send outbox/x.md --caption "Device profile"`), false},
		{"trailing comment", "docs/page.md", sh("adbuddy setup   # pair the phone"), false},
		{"windows exe", "docs/page.md", block("powershell", `.\adbuddy.exe setup --code`), false},
		{"console output line", "docs/page.md", block("console", "adbuddy 0.3.0"), false},
		{"console prompt and output", "docs/page.md", block("console", "$ adbuddy setup", "adbuddy 0.3.0"), false},
		{"prose that is not a command", "docs/page.md", inline("adbuddy is free"), false},
		{"bare adbuddy", "docs/page.md", inline("adbuddy"), false},
		{"https link", "docs/page.md", "[a](https://example.com/x)", false},
		{"link with anchor", "docs/page.md", "[a](x.md#y)", false},
		{"license file from the README", "README.md", "[License](LICENSE)", false},
		{"text block is not shell", "docs/page.md", block("text", "adbuddy bogus"), false},
		{"documentation IP", "docs/page.md", "Phone at 192.0.2.7.", false},
		{"four-component version", "docs/page.md", "Version 1.0.41.0 of the app.", false},
		{"octet over 255 is not an address", "docs/page.md", "Build 1.0.410.0.", false},
		{"link to a page in docs", "docs/page.md", "[a](x.md)", false},
		{"link from the README into docs", "README.md", "[a](docs/releasing.md)", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := docProblems(root, c.file, c.text)
			if c.want && len(got) == 0 {
				t.Errorf("no message for %q", c.text)
			}
			if !c.want && len(got) > 0 {
				t.Errorf("unexpected messages for %q: %v", c.text, got)
			}
		})
	}

	// A bracket inside quotes must stay an argument.
	if got := docSegments(`adbuddy tap "[ok]" extra`); len(got) != 1 || len(got[0]) != 4 {
		t.Errorf("quoted bracket lost: %v", got)
	}

	// Each guard on its own, as the plan's interface says.
	if got := checkDocLinks(root, "docs/page.md", "[a](missing.md)"); len(got) == 0 {
		t.Error("checkDocLinks missed a broken link")
	}
	if got := checkDocCommands("f.md", sh("adbuddy bogus")); len(got) == 0 {
		t.Error("checkDocCommands missed an unknown subcommand")
	}
	if got := checkDocPrivacy("f.md", "10.0.0." + "5"); len(got) == 0 {
		t.Error("checkDocPrivacy missed an address")
	}
}

func TestDocsPublishedExcludes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/_config.yml", "exclude:\n  - releasing.md\n  - private/\n  - \"draft-*.md\"\n")
	for _, f := range []string{"README.md", "docs/index.md", "docs/releasing.md", "docs/private/a.md", "docs/draft-1.md", "docs/.hidden.md", "docs/_site/a.md", "docs/sub/ok.md"} {
		write(f, "x")
	}
	got := publishedDocs(t, root)
	slices.Sort(got)
	want := []string{"README.md", "docs/index.md", "docs/sub/ok.md"}
	if !slices.Equal(got, want) {
		t.Errorf("published = %v, want %v", got, want)
	}
	if !docExcluded([]string{"private"}, "private/a.md") || docExcluded([]string{"private"}, "privacy.md") {
		t.Error("docExcluded: a folder entry must cover its content and only it")
	}
}
