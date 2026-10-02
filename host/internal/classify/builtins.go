package classify

import "strings"

// A builtin is code of the shell itself: it can change what the rest of the
// script means (trap, alias, eval, typeset, set -A…) in ways a rule for its
// words cannot see. testdata/mksh-builtins.tsv lists every builtin and
// predefined alias of the phone's mksh, recorded from a real one; each of
// them is either admitted here with a grammar for its arguments, or 🔴
// special. This runs before the rules, so a user rule cannot lower one.

// admittedBuiltins are the builtins and aliases a script may use. A nil
// grammar takes any word the evaluator admits; the wrappers (command,
// builtin, exec, nohup) are read by unwrap.
var admittedBuiltins = map[string]func(e *evaluator, ws []Word) Text{
	"echo": nil, "true": nil, "false": nil, ":": nil, "cd": nil, "pwd": nil,
	"command": nil, "builtin": nil, "exec": nil, "nohup": nil,
	"[":        nil, // checked by testFindings
	"test":     nil,
	"read":     readGrammar,
	"exit":     countGrammar,
	"break":    countGrammar,
	"continue": countGrammar,
	"type":     staticGrammar,
	"whence":   staticGrammar,
	"kill":     staticGrammar,
	"set":      setGrammar,
	"printf":   printfGrammar,
}

// scriptBuiltins run a file or a string as shell code: evalWords gives them
// their own finding.
var scriptBuiltins = map[string]bool{".": true, "source": true, "eval": true}

// otherBuiltins are the rest of mksh's builtins and predefined aliases:
// never analysed.
var otherBuiltins = map[string]bool{
	"alias": true, "autoload": true, "bg": true, "bind": true, "chdir": true, "export": true,
	"fc": true, "fg": true, "functions": true, "getopts": true, "hash": true,
	"history": true, "integer": true, "jobs": true, "let": true, "local": true, "login": true,
	"nameref": true, "print": true, "r": true, "readonly": true, "rename": true, "return": true,
	"shift": true, "suspend": true, "times": true, "trap": true, "typeset": true,
	"ulimit": true, "umask": true, "unalias": true, "unset": true, "wait": true,
}

// builtin returns the findings for a command that is a shell builtin which
// is not admitted or whose arguments do not fit its grammar; nil if the
// command may go on to its rule.
func (e *evaluator) builtin(ws []Word) []Finding {
	name := ws[0].Value
	vals := values(ws)
	if name == "[" || name == "test" {
		return e.testFindings(ws)
	}
	if grammar, ok := admittedBuiltins[name]; ok {
		if grammar == nil {
			return nil
		}
		if why := grammar(e, ws); !why.Empty() {
			return []Finding{unanalysable("shell.builtin", why, vals, textUnsupported)}
		}
		return nil
	}
	if otherBuiltins[name] {
		return []Finding{unanalysable("shell.builtin", reason("{name} is a shell builtin ADBuddy does not analyse", "name", name), vals, textUnsupported)}
	}
	return nil
}

// readGrammar is `read [-r] name…`, with names of the script's own. Without
// a name read sets REPLY.
func readGrammar(_ *evaluator, ws []Word) Text {
	args := ws[1:]
	if len(args) > 0 && !args[0].Dynamic && args[0].Value == "-r" {
		args = args[1:]
	}
	if len(args) == 0 {
		return reason("read without a variable name")
	}
	for _, a := range args {
		if a.Dynamic || !ownName(a.Value) {
			return reason("read into a variable that is not the script's own (lower-case names only)")
		}
	}
	return Text{}
}

// countGrammar is exit, break and continue: no argument, a number or $?.
func countGrammar(e *evaluator, ws []Word) Text {
	switch {
	case len(ws) == 1:
		return Text{}
	case len(ws) == 2 && !ws[1].Dynamic && isDigits(ws[1].Value):
		return Text{}
	case len(ws) == 2 && testKind(ws[1], e.numeric) == testNumber:
		return Text{}
	}
	return reason("{value} with an argument that is not a number", "value", ws[0].Value)
}

// printfGrammar refuses -v, which stores the output in a shell variable, and
// an option only known on the phone, which may be -v. On the phone printf is
// a program, not a builtin; a build of mksh with its own printf would take it.
func printfGrammar(_ *evaluator, ws []Word) Text {
	// Only the options before the format count: the first word that is
	// not one ends them.
	for _, a := range ws[1:] {
		if a.Dynamic || !strings.HasPrefix(a.Value, "-") || a.Value == "--" {
			break
		}
		if a.Value == "-v" {
			return reason("printf -v writes to a shell variable")
		}
	}
	return Text{}
}

// staticGrammar takes only static words.
func staticGrammar(_ *evaluator, ws []Word) Text {
	for _, a := range ws[1:] {
		if a.Dynamic {
			return reason("{value} with an argument only known on the phone", "value", ws[0].Value)
		}
	}
	return Text{}
}

// setOptions are the options of set a script may turn on or off: they
// change how the shell reports errors and traces, not what a word means.
var (
	setLetters = "efuvxC"
	setNames   = map[string]bool{
		"errexit": true, "noglob": true, "nounset": true, "verbose": true, "xtrace": true,
		"noclobber": true, "pipefail": true,
	}
)

// setGrammar is `set` with those options only: no positional parameters, no
// -A (an array) and nothing that changes how words expand.
func setGrammar(_ *evaluator, ws []Word) Text {
	for i := 1; i < len(ws); i++ {
		a := ws[i]
		if a.Dynamic || len(a.Value) < 2 || (a.Value[0] != '-' && a.Value[0] != '+') {
			return reason("set with positional parameters or an argument only known on the phone")
		}
		flags := a.Value[1:]
		if flags == "o" {
			if i+1 >= len(ws) || ws[i+1].Dynamic || !setNames[ws[i+1].Value] {
				return reason("set -o with an option ADBuddy does not admit")
			}
			i++
			continue
		}
		if strings.Trim(flags, setLetters) != "" {
			return reason("set {value} is not an option ADBuddy admits", "value", a.Value)
		}
	}
	return Text{}
}
