package classify

import (
	"reflect"
	"strings"
	"testing"
)

// getoptSpec.parse is the helper under the checks of sort, uniq, pgrep, ss and
// logcat: its reading of --, values, unknown letters and the two modes is
// tested here directly.
func TestGetoptSpecParse(t *testing.T) {
	spec := func(strict bool) getoptSpec {
		return getoptSpec{
			flags: "ab", values: "tk",
			longFlags:  map[string]bool{"--all": true},
			longValues: map[string]bool{"--name": true},
			strict:     strict,
		}
	}
	for _, tc := range []struct {
		name   string
		strict bool
		args   string
		seen   map[byte]string
		long   []string
		pos    []string
		bad    string
	}{
		{name: "flags in a cluster", strict: true, args: "-ab x", seen: map[byte]string{'a': "", 'b': ""}, pos: []string{"x"}},
		{name: "value glued", strict: true, args: "-t, x", seen: map[byte]string{'t': ","}, pos: []string{"x"}},
		{name: "value in the next word", strict: true, args: "-t , x", seen: map[byte]string{'t': ","}, pos: []string{"x"}},
		{name: "value takes the rest of the cluster", strict: true, args: "-atb x", seen: map[byte]string{'a': "", 't': "b"}, pos: []string{"x"}},
		{name: "-- ends the options", strict: true, args: "-a -- -b x", seen: map[byte]string{'a': ""}, pos: []string{"-b", "x"}},
		{name: "-- as the value of an option", strict: true, args: "-t -- -b", bad: "-t"},
		{name: "value that starts with a dash", strict: true, args: "-k -b x", bad: "-k"},
		{name: "value missing", strict: true, args: "-ab -t", seen: map[byte]string{'a': "", 'b': ""}, bad: "-t"},
		{name: "unknown letter", strict: true, args: "-az x", seen: map[byte]string{'a': ""}, bad: "-az"},
		{name: "lone dash is a positional", strict: true, args: "- -a", seen: map[byte]string{'a': ""}, pos: []string{"-"}},
		{name: "options after a positional", strict: true, args: "x -a y", seen: map[byte]string{'a': ""}, pos: []string{"x", "y"}},
		{name: "long flag", strict: true, args: "--all x", long: []string{"--all"}, pos: []string{"x"}},
		{name: "long flag with a value", strict: true, args: "--all=1", bad: "--all=1"},
		{name: "long value glued", strict: true, args: "--name=x y", long: []string{"--name"}, pos: []string{"y"}},
		{name: "long value in the next word", strict: true, args: "--name x y", long: []string{"--name"}, pos: []string{"y"}},
		{name: "long value missing", strict: true, args: "--name", bad: "--name"},
		{name: "abbreviated long", strict: true, args: "--al", bad: "--al"},
		{name: "unknown long", strict: true, args: "--foo", bad: "--foo"},
		// Not strict: -- is an unknown long option, a value is taken from the
		// next word whatever it looks like and may be missing.
		{name: "loose: --", args: "--", bad: "--"},
		{name: "loose: value that starts with a dash", args: "-t -b x", seen: map[byte]string{'t': "-b"}, pos: []string{"x"}},
		{name: "loose: value missing", args: "-a -t", seen: map[byte]string{'a': "", 't': ""}},
		{name: "loose: unknown letter", args: "-z", bad: "-z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ws []Word
			for _, a := range strings.Fields(tc.args) {
				ws = append(ws, Word{Value: a})
			}
			r := spec(tc.strict).parse(ws)
			if r.bad != tc.bad {
				t.Fatalf("bad = %q, want %q", r.bad, tc.bad)
			}
			if tc.bad != "" {
				return
			}
			if len(r.seen) != len(tc.seen) || (len(tc.seen) > 0 && !reflect.DeepEqual(r.seen, tc.seen)) {
				t.Errorf("seen = %v, want %v", r.seen, tc.seen)
			}
			for _, l := range tc.long {
				if !r.long[l] {
					t.Errorf("long %s not seen: %v", l, r.long)
				}
			}
			if len(r.pos) != len(tc.pos) || (len(tc.pos) > 0 && !reflect.DeepEqual(r.pos, tc.pos)) {
				t.Errorf("pos = %v, want %v", r.pos, tc.pos)
			}
		})
	}
}
