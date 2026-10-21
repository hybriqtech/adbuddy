package shq

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"com.facebook.katana":  "com.facebook.katana",
		"0.5":                  "0.5",
		"/sdcard/Download/a.b": "/sdcard/Download/a.b",
		"":                     "''",
		"a b":                  "'a b'",
		"$HOME":                "'$HOME'",
		"x;reboot":             "'x;reboot'",
		"it's":                 `'it'\''s'`,
		"`id`":                 "'`id`'",
		"line1\nline2":         "'line1\nline2'",
		"-n":                   "-n",
		"*":                    "'*'",
		"~":                    "'~'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every quoted value must come back byte for byte when a POSIX shell parses it.
func TestQuoteRoundTripsThroughShellParser(t *testing.T) {
	values := []string{"", "plain", "a b", "$(reboot)", "x'; rm -rf /sdcard; echo '", "\\", "\"", "tab\there", "ñandú", "a\nb", "$'x'", "*?[a]", "{a,b}", "#c", "!x"}
	p := syntax.NewParser(syntax.Variant(syntax.LangMirBSDKorn))
	for _, v := range values {
		src := "echo " + Quote(v)
		f, err := p.Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		call := f.Stmts[0].Cmd.(*syntax.CallExpr)
		if len(call.Args) != 2 {
			t.Fatalf("%q split into %d words", src, len(call.Args))
		}
		var got strings.Builder
		for _, part := range call.Args[1].Parts {
			switch p := part.(type) {
			case *syntax.Lit:
				got.WriteString(p.Value)
			case *syntax.SglQuoted:
				if p.Dollar {
					t.Fatalf("%q: $'' quoting", src)
				}
				got.WriteString(p.Value)
			default:
				t.Fatalf("%q: unexpected part %T", src, part)
			}
		}
		// Lit parts outside quotes only appear for safe words and for the
		// backslash-escaped quote in '\''.
		if strings.ReplaceAll(got.String(), `\'`, "'") != v {
			t.Errorf("round trip of %q gave %q", v, got.String())
		}
	}
}

func TestJoinAndArgs(t *testing.T) {
	if got := Join("settings", "put", "global", "k", "a b"); got != "settings put global k 'a b'" {
		t.Fatal(got)
	}
	if got := Args("mv", "/sdcard/a b", "/sdcard/c"); strings.Join(got, "|") != "mv|'/sdcard/a b'|/sdcard/c" {
		t.Fatal(got)
	}
}

// FuzzQuoteUnquote: whatever Quote writes, Unquote reads back unchanged.
// (The converse is false by design: Unquote also accepts words Quote would
// never write, like 'a' for a.)
func FuzzQuoteUnquote(f *testing.F) {
	for _, s := range []string{"", "a", "a b", "it's", "'", "''", `\'`, "$(x);y", "é", "a\x00b", "/sdcard/x y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, ok := Unquote(Quote(s))
		if !ok || got != s {
			t.Fatalf("Unquote(Quote(%q)) = %q, %v", s, got, ok)
		}
	})
}
