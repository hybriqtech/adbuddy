package classify

import (
	"regexp"

	"mvdan.cc/sh/v3/syntax"
)

// A script only sets variables of its own: lower-case names. Every variable
// mksh or the phone's programs treat in a special way (PS4, ENV, IFS, PATH,
// LD_PRELOAD, the integer ones such as SECONDS…) is upper-case, so none of
// them has to be listed: `typeset +` and `env` in adb's shell show no
// lower-case name (recorded in testdata/oracle.jsonl). A script never
// exports one either, so no program and no nested shell sees them.

var ownNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ownName reports a variable name a script may assign.
func ownName(name string) bool { return ownNameRE.MatchString(name) }

// nameFinding is for an assignment, in any form, to a name that is not the
// script's own.
func nameFinding(name string) Finding {
	return unanalysable("shell.assign", "sets "+name+", a variable that is not the script's own (lower-case names only)", []string{name}, textAssign)
}

// commandEnv are the only variables a command may get in its environment
// (NAME=value command, env NAME=value command): they pick a language or a
// time zone. Anything else could change which program or library runs.
var commandEnv = map[string]bool{
	"LANG": true, "LC_ALL": true, "LC_COLLATE": true, "LC_CTYPE": true,
	"LC_MESSAGES": true, "LC_NUMERIC": true, "LC_TIME": true, "TZ": true,
}

// assign checks `name=word` as a statement of its own.
func (e *evaluator) assign(a *syntax.Assign) {
	if a.Name == nil || a.Index != nil || a.Array != nil || a.Append || a.Naked {
		// a[i]=…, a=(…), a+=… and whatever else the parser reads here.
		e.unsupported("assignment to an array or with +=", a)
		return
	}
	if !ownName(a.Name.Value) {
		e.out = append(e.out, nameFinding(a.Name.Value))
	}
	if a.Value != nil {
		e.word(a.Value)
	}
}

// envAssign checks `NAME=value command`.
func (e *evaluator) envAssign(a *syntax.Assign) {
	if a.Name == nil || a.Index != nil || a.Array != nil || a.Append || a.Naked {
		e.unsupported("assignment to an array or with +=", a)
		return
	}
	var v Word
	if a.Value != nil {
		v = e.word(a.Value)
	}
	if !commandEnv[a.Name.Value] || v.Dynamic {
		e.out = append(e.out, unanalysable("shell.assign", "sets "+a.Name.Value+" in the environment of a command", []string{a.Name.Value}, textAssign))
	}
}

// numericVars returns the variables of a script proven to hold a number:
// every assignment to them, anywhere in the script and whatever the order,
// is a number literal or admitted arithmetic. mksh evaluates the value of a
// variable used in arithmetic as an expression, $(…) included, so one
// assignment that is anything else (a word only known on the phone, read, a
// loop over words that are not numbers) makes the variable not numeric.
// unproven are the variables the script sets that are not numeric.
func numericVars(f *syntax.File) (numeric, unproven map[string]bool) {
	type assigned struct {
		values []*syntax.Word
		other  bool // set by read or by a loop over something that is not a number
	}
	vars := map[string]*assigned{}
	get := func(name string) *assigned {
		v := vars[name]
		if v == nil {
			v = &assigned{}
			vars[name] = v
		}
		return v
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Assign:
			if n.Name == nil {
				break
			}
			v := get(n.Name.Value)
			if n.Index != nil || n.Array != nil || n.Append || n.Value == nil {
				v.other = true
				break
			}
			v.values = append(v.values, n.Value)
		case *syntax.WordIter:
			v := get(n.Name.Value)
			if !n.InPos.IsValid() {
				v.other = true
			}
			for _, w := range n.Items {
				if lit := wordOf(w); lit.Dynamic || !numberRE.MatchString(lit.Value) {
					v.other = true
				}
			}
		case *syntax.CallExpr:
			// read NAME…, alone or behind a wrapper (command read x): every
			// static word of the command may be a name.
			var static []string
			reads := false
			for _, w := range n.Args {
				if lit := wordOf(w); !lit.Dynamic {
					static = append(static, lit.Value)
					reads = reads || lit.Value == "read"
				}
			}
			if reads {
				for _, name := range static {
					get(name).other = true
				}
			}
		}
		return true
	})
	// The largest set in which every assignment of every variable is a
	// number: start from all of them and drop until nothing changes
	// (i=$((i+1)) is a number as long as i is).
	numeric = map[string]bool{}
	for name, v := range vars {
		if !v.other && ownName(name) {
			numeric[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name := range numeric {
			for _, w := range vars[name].values {
				if !numericValue(w, numeric) {
					delete(numeric, name)
					changed = true
					break
				}
			}
		}
	}
	unproven = map[string]bool{}
	for name := range vars {
		if !numeric[name] {
			unproven[name] = true
		}
	}
	return numeric, unproven
}

// numericValue reports an assigned value that is a number literal or only
// one admitted arithmetic expansion.
func numericValue(w *syntax.Word, numeric map[string]bool) bool {
	if lit := wordOf(w); !lit.Dynamic {
		return numberRE.MatchString(lit.Value)
	}
	if len(w.Parts) != 1 {
		return false
	}
	a, ok := w.Parts[0].(*syntax.ArithmExp)
	return ok && arithOK(a, numeric)
}
