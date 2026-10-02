package classify

// reason builds the Text of a reason, the "why" that goes into the detail of a
// 🔴 question: the English template, whose {placeholders} are filled by the
// name/value pairs after it, and its Spanish from reasonsES, filled the same
// way. The Spanish of a template that is not in reasonsES is empty, which
// Text.In answers in English; TestReasonsBilingual makes that impossible in a
// build that passes its tests. A value is not translated; it is shortened like
// a title's words (display: escaped, blanks compacted, 80 runes), so the agent
// cannot make a line of the detail endless, and Result.Detail escapes the
// whole line again (Printable, which is idempotent).
func reason(en string, kv ...string) Text {
	t := Text{EN: en, ES: reasonsES[en]}
	if len(kv) == 0 {
		return t
	}
	vars := make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		vars[kv[i]] = display([]string{kv[i+1]}) // A4-3: no endless line in the detail
	}
	return render(t, vars)
}

// Empty reports whether t says nothing: no reason.
func (t Text) Empty() bool { return t.EN == "" && t.ES == "" }

// Join is two reasons in one line, either of which may be empty.
func Join(a, b Text) Text {
	switch {
	case b.Empty():
		return a
	case a.Empty():
		return b
	}
	return Text{EN: a.EN + "; " + b.EN, ES: a.ES + "; " + b.ES}
}

// Same is a Text that says the same in both languages: a path, a name, the
// message of an error of the system.
func Same(s string) Text { return Text{EN: s, ES: s} }

// Cat puts texts one after the other, in each language.
func Cat(parts ...Text) Text {
	var t Text
	for _, p := range parts {
		t.EN += p.EN
		t.ES += p.ES
	}
	return t
}
