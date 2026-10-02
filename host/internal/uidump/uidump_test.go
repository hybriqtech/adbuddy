package uidump

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func settings(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/settings.xml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParse(t *testing.T) {
	d, err := Parse(settings(t), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Package != "com.android.settings" || d.Width != 1344 || d.Height != 2992 || d.Rotation != 0 || d.Truncated {
		t.Fatalf("dump %+v", d)
	}
	var got []string
	for _, e := range d.Elements {
		got = append(got, e.Class+":"+e.Text+":"+e.Desc)
		if strings.Contains(e.ID, "adbuddy") || e.Text == "Allow" {
			t.Fatalf("ADBuddy's node shown: %+v", e)
		}
		if e.Text == "Hidden" {
			t.Fatalf("a node with no area shown: %+v", e)
		}
	}
	want := []string{
		"Toolbar::Search settings", "TextView:Search settings:", "RecyclerView::",
		"LinearLayout::", "TextView:Network & internet:", "Switch::Wi-Fi",
		"LinearLayout::", "TextView:OK:", "LinearLayout::", "TextView:ok:", "EditText::",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("elements\n got %q\nwant %q", got, want)
	}
	pin := d.Elements[len(d.Elements)-1]
	if !pin.Password || pin.Text != "" || !pin.Edit || !pin.Focused {
		t.Fatalf("password field %+v", pin)
	}
	sw := d.Elements[5]
	if sw.Checked == nil || *sw.Checked || sw.Bounds != [4]int{980, 610, 1302, 680} || !sw.Click {
		t.Fatalf("switch %+v", sw)
	}
	if d.Elements[4].Checked != nil {
		t.Fatalf("a text is not checkable: %+v", d.Elements[4])
	}
}

func TestParseAll(t *testing.T) {
	some, _ := Parse(settings(t), false, nil)
	all, err := Parse(settings(t), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The root and the empty FrameLayout; never the node with no area.
	if len(all.Elements) != len(some.Elements)+2 {
		t.Fatalf("all: %d elements, some: %d", len(all.Elements), len(some.Elements))
	}
}

func TestParseLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<hierarchy rotation="1"><node package="p" class="a.B" bounds="[0,0][100,100]">`)
	for range MaxElements + 10 {
		b.WriteString(`<node text="x" package="p" class="a.T" bounds="[0,0][10,10]"/>`)
	}
	b.WriteString(`</node></hierarchy>`)
	d, err := Parse([]byte(b.String()), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != MaxElements || !d.Truncated || d.Rotation != 1 {
		t.Fatalf("%d elements, truncated %v", len(d.Elements), d.Truncated)
	}
	deep := strings.Repeat(`<node text="x" package="p" class="a.T" bounds="[0,0][10,10]">`, MaxDepth+5) + strings.Repeat(`</node>`, MaxDepth+5)
	d, err = Parse([]byte(`<hierarchy>`+deep+`</hierarchy>`), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != MaxDepth+1 || !d.Truncated {
		t.Fatalf("deep: %d elements, truncated %v", len(d.Elements), d.Truncated)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, out := range []string{"", "ERROR: could not get idle state.", "<hierarchy><node"} {
		if _, err := Parse([]byte(out), false, nil); err == nil {
			t.Errorf("%q: no error", out)
		}
	}
}

func TestFind(t *testing.T) {
	x := settings(t)
	cases := []struct {
		id, text string
		want     [4]int
		err      error
	}{
		// The text's clickable ancestor, the row.
		{text: "Network & internet", want: [4]int{0, 600, 1344, 690}},
		// The description of a clickable node is the node itself.
		{text: "Wi-Fi", want: [4]int{980, 610, 1302, 680}},
		// Exact text first: "OK" is one row, even if "ok" is another.
		{text: "OK", want: [4]int{0, 700, 1344, 790}},
		{id: "com.android.settings:id/search_action_bar", want: [4]int{42, 180, 1302, 300}},
		{text: "Nothing like this", err: ErrNotFound},
		{text: "Allow", err: ErrProtected},
		{id: "tech.hybriq.adbuddy:id/allow", err: ErrProtected},
		// A password field's text is never matched.
		{text: "hunter2", err: ErrNotFound},
	}
	for _, c := range cases {
		got, err := Find(x, c.id, c.text, nil)
		if !errors.Is(err, c.err) || got != c.want {
			t.Errorf("Find(%q, %q) = %v, %v; want %v, %v", c.id, c.text, got, err, c.want, c.err)
		}
	}
	// "Ok" matches "OK" and "ok" only ignoring case: two rows.
	_, err := Find(x, "", "Ok", nil)
	var amb *AmbiguousError
	if !errors.As(err, &amb) || amb.N != 2 || !strings.Contains(err.Error(), "(0,700)-(1344,790)") {
		t.Fatalf("ambiguous: %v", err)
	}
	// The same id on several rows.
	_, err = Find(x, "android:id/title", "", nil)
	if !errors.As(err, &amb) || amb.N != 3 {
		t.Fatalf("ambiguous id: %v", err)
	}
}

// A clickable ancestor without area gives no touch point: an error, never a
// tap at (0,0) (V10).
func TestFindEmptyClickableBounds(t *testing.T) {
	for _, bounds := range []string{`bounds="[0,0][0,0]"`, `bounds="[10,10][10,50]"`, ``, `bounds="garbage"`} {
		x := `<hierarchy><node package="p" class="a.B" bounds="[0,0][100,100]">` +
			`<node clickable="true" package="p" class="a.R" ` + bounds + `>` +
			`<node text="Go" package="p" class="a.T" bounds="[5,5][50,50]"/></node></node></hierarchy>`
		for _, q := range [][2]string{{"", "Go"}} {
			got, err := Find([]byte(x), q[0], q[1], nil)
			if !errors.Is(err, ErrNoTouchPoint) || got != ([4]int{}) {
				t.Errorf("%s: Find = %v, %v; want ErrNoTouchPoint", bounds, got, err)
			}
		}
	}
	// With area the same layout still finds the row (nothing else changed).
	x := `<hierarchy><node package="p" class="a.B" bounds="[0,0][100,100]"><node clickable="true" package="p" class="a.R" bounds="[0,10][100,60]">` +
		`<node text="Go" package="p" class="a.T" bounds="[5,5][50,50]"/></node></node></hierarchy>`
	if got, err := Find([]byte(x), "", "Go", nil); err != nil || got != [4]int{0, 10, 100, 60} {
		t.Fatalf("Find = %v, %v", got, err)
	}
}

// A protected match still says protected even when another match has no area.
func TestFindProtectedBeatsNoTouchPoint(t *testing.T) {
	x := `<hierarchy><node package="p" class="a.B" bounds="[0,0][100,100]">` +
		`<node clickable="true" package="p" class="a.R" bounds="[0,0][0,0]"><node text="Go" package="p" class="a.T" bounds="[5,5][50,50]"/></node>` +
		`<node text="Go" package="bank" class="a.T" bounds="[5,5][50,50]"/></node></hierarchy>`
	_, err := Find([]byte(x), "", "Go", func(p string) bool { return p == "bank" })
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("Find = %v", err)
	}
}

// Cut at MaxDepth with no match: not "not found" (V10).
func TestFindTooDeep(t *testing.T) {
	deep := strings.Repeat(`<node package="p" class="a.T" bounds="[0,0][10,10]">`, MaxDepth+2) +
		`<node text="Go" package="p" class="a.T" bounds="[0,0][10,10]"/>` + strings.Repeat(`</node>`, MaxDepth+2)
	if _, err := Find([]byte(`<hierarchy>`+deep+`</hierarchy>`), "", "Go", nil); !errors.Is(err, ErrTooDeep) {
		t.Fatalf("deep: %v", err)
	}
	// A shallow layout without the text is still not found.
	if _, err := Find([]byte(`<hierarchy><node package="p" bounds="[0,0][10,10]"/></hierarchy>`), "", "Go", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("shallow: %v", err)
	}
}

func TestResumed(t *testing.T) {
	cases := []struct {
		name, out string
		want      []Activity
	}{
		{"android 14", "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\nDisplay #0 (activities from top to bottom):\n  * Task{abc #28 type=standard A=1000:com.android.settings}\n  topResumedActivity=ActivityRecord{c1a2b3 u0 com.android.settings/.Settings t28}\n",
			[]Activity{{"com.android.settings", "com.android.settings.Settings"}}},
		{"android 11", "  Stack #1: type=standard mode=fullscreen\n    mResumedActivity: ActivityRecord{2b4b1e3 u0 com.whatsapp/com.whatsapp.HomeActivity t31}\n",
			[]Activity{{"com.whatsapp", "com.whatsapp.HomeActivity"}}},
		{"maker", " ResumedActivity: ActivityRecord{9f u0 com.sec.android.app.launcher/.activities.LauncherActivity t1}\n",
			[]Activity{{"com.sec.android.app.launcher", "com.sec.android.app.launcher.activities.LauncherActivity"}}},
		// A split screen: the bank in the other half counts too, and the top
		// one, listed twice, once.
		{"split", "  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{2 u0 com.example.bank/.Main t2}\n",
			[]Activity{{"com.android.settings", "com.android.settings.Settings"}, {"com.example.bank", "com.example.bank.Main"}}},
	}
	for _, c := range cases {
		if got := Resumed(c.out); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
	if got := Resumed("  mLastPausedActivity: ActivityRecord{1 u0 a.b/.C t1}\n"); len(got) != 0 {
		t.Errorf("a paused activity taken as resumed: %+v", got)
	}
}

func TestAsleep(t *testing.T) {
	for out, want := range map[string]bool{
		"POWER MANAGER\n  mWakefulness=Asleep\n":   true,
		"POWER MANAGER\n  mWakefulness=Dozing\n":   true,
		"POWER MANAGER\n  mWakefulness=Awake\n":    false,
		"POWER MANAGER\n  mWakefulness=Dreaming\n": false,
		"":                           false,
		"mWakefulnessChanging=false": false,
	} {
		if Asleep(out) != want {
			t.Errorf("%q: %v", out, !want)
		}
	}
}

// Hidden apps (the protected ones) are left out and never tapped.
func TestHiddenApps(t *testing.T) {
	hidden := func(p string) bool { return p == "com.android.settings" }
	d, err := Parse(settings(t), true, hidden)
	if err != nil || len(d.Elements) != 0 || d.Package != "" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := Find(settings(t), "", "Network & internet", hidden); !errors.Is(err, ErrProtected) {
		t.Fatalf("find: %v", err)
	}
}

func TestServiceOn(t *testing.T) {
	on := "User state[\n  Enabled services:{{tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService}}\n"
	if !ServiceOn(on) || ServiceOn("User state[\n  Enabled services:{}\n") {
		t.Fatal("ServiceOn")
	}
}

// An injected line (a wakelock label with a line break, say) cannot hide the
// real Awake.
func TestAsleepNeedsEveryWakefulnessLineToSayIt(t *testing.T) {
	for out, want := range map[string]bool{
		"mWakefulness=Awake\n  mWakefulness=Asleep\n":  false,
		"mWakefulness=Asleep\n  mWakefulness=Awake\n":  false,
		"mWakefulness=Asleep\n  mWakefulness=Dozing\n": true,
		"mWakefulness=Asleep\r\n":                      true,
		"mWakefulness=Asleep extra\n":                  false,
	} {
		if Asleep(out) != want {
			t.Errorf("%q: %v", out, !want)
		}
	}
}
