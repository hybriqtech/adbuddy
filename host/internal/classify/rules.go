package classify

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Text is a human description in English and Spanish.
type Text struct {
	EN string `yaml:"en" json:"en"`
	ES string `yaml:"es" json:"es"`
}

// In returns the text in lang ("es" or anything else for English).
func (t Text) In(lang string) string {
	if lang == "es" && t.ES != "" {
		return t.ES
	}
	return t.EN
}

// Rule maps a command (and optionally its subcommand) to a level.
type Rule struct {
	ID        string   `yaml:"id"`
	Scope     string   `yaml:"scope"` // "shell" (default): a command run by the phone's shell; "adb": an adb command
	Cmd       string   `yaml:"cmd"`
	Sub       []string `yaml:"sub"`     // matches when the first non-option argument is one of these
	LevelName string   `yaml:"level"`   // green, yellow or red (required)
	AnyArgs   bool     `yaml:"anyArgs"` // arguments ADBuddy cannot read in advance ($x, globs…) are fine
	Check     string   `yaml:"check"`   // built-in argument check that may raise the level or define the change
	UI        bool     `yaml:"ui"`      // interface action without inverse
	Title     Text     `yaml:"title"`   // description; placeholders: {cmd} {args} {pkg} {key} {value} {ns} {path} {src} {dst} {apk} {op} {mode}

	level Level
	user  bool
}

//go:embed rules/*.yaml
var builtinFS embed.FS

// builtinRules parses every embedded rules file, in file name order.
func builtinRules() ([]Rule, error) {
	names, err := fs.Glob(builtinFS, "rules/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var all []Rule
	for _, name := range names {
		data, err := builtinFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		rules, err := parseRules(data, false)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		all = append(all, rules...)
	}
	return all, nil
}

// opChecks produce an Op (a change with an inverse). Attached by a user rule
// to another command they would record a wrong inverse.
var opChecks = map[string]bool{
	"settings-put": true, "pm-disable": true, "pm-uninstall-user": true, "appops-set": true, "deviceidle": true,
	"mv": true, "mkdir": true, "install": true, "push": true, "screencap": true, "uiautomator-dump": true,
}

type ruleFile struct {
	Rules []Rule `yaml:"rules"`
}

// parseRules decodes a rules file. Unknown fields, unknown checks and missing
// levels are errors: a typo must never turn a rule green by default.
func parseRules(data []byte, user bool) ([]Rule, error) {
	var f ruleFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("rules must be a single YAML document (found a second one after ---)")
	}
	for i := range f.Rules {
		r := &f.Rules[i]
		if r.ID == "" || r.Cmd == "" {
			return nil, fmt.Errorf("rule #%d: id and cmd are required", i+1)
		}
		for _, s := range r.Sub {
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("rule %s: empty sub", r.ID)
			}
		}
		if r.Scope == "" {
			r.Scope = "shell"
		}
		if r.Scope != "shell" && r.Scope != "adb" {
			return nil, fmt.Errorf("rule %s: scope must be shell or adb", r.ID)
		}
		lvl, err := ParseLevel(r.LevelName)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", r.ID, err)
		}
		r.level = lvl
		if r.Check != "" {
			if _, ok := checks[r.Check]; !ok {
				return nil, fmt.Errorf("rule %s: unknown check %q", r.ID, r.Check)
			}
			if user && opChecks[r.Check] {
				return nil, fmt.Errorf("rule %s: check %q describes a change with an undo and only built-in rules can use it", r.ID, r.Check)
			}
		}
		if r.Title.EN == "" {
			r.Title = Text{EN: r.ID + ": {cmd}", ES: r.ID + ": {cmd}"}
		}
		r.user = user
	}
	return f.Rules, nil
}

type ruleSet struct{ user, builtin []Rule }

// find returns the rule for cmd/sub. User rules win over built-in ones; inside
// each set a rule naming the subcommand wins over a generic one.
func (rs ruleSet) find(scope, cmd, sub string) (Rule, bool) {
	for _, list := range [][]Rule{rs.user, rs.builtin} {
		var generic *Rule
		for i := range list {
			r := &list[i]
			if r.Scope != scope || r.Cmd != cmd {
				continue
			}
			if len(r.Sub) == 0 {
				if generic == nil {
					generic = r
				}
				continue
			}
			for _, s := range r.Sub {
				if s == sub {
					return *r, true
				}
			}
		}
		if generic != nil {
			return *generic, true
		}
	}
	return Rule{}, false
}

// render substitutes {name} placeholders.
func render(t Text, vars map[string]string) Text {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	r := strings.NewReplacer(pairs...)
	return Text{EN: r.Replace(t.EN), ES: r.Replace(t.ES)}
}
