package emutest

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

var (
	viewerRe   = regexp.MustCompile(`(?m)^\s+packageName=(\S+)`)
	grantRe    = regexp.MustCompile(`content://tech\.hybriq\.adbuddy\.files/downloads/(\S+?) \[user \d+\]\}\s+targetUserId=\d+ sourcePkg=(\S+) targetPkg=(\S+)\s+mode=(0x[0-9a-f]+)`)
	startLineR = regexp.MustCompile(`START u0 \{[^\n]*\}[^\n]*`)
)

// viewerOf returns the package of the app the system picks for VIEW of mime
// ("" when no app opens it: the app then falls to the share sheet).
func (e *emu) viewerOf(t *testing.T, mime string) string {
	t.Helper()
	out := e.shell(t, "cmd package query-activities -a android.intent.action.VIEW -t "+mime)
	i := strings.Index(out, "ActivityInfo:")
	if i < 0 {
		return ""
	}
	m := viewerRe.FindStringSubmatch(out[i:])
	if m == nil {
		t.Fatalf("query-activities for %s has no packageName: %s", mime, out)
	}
	return m[1]
}

// uriGrant is a temporary permission on a file of the app's cache, as
// `dumpsys activity permissions` shows it (mode=0x1 is read only).
type uriGrant struct{ File, Source, Target, Mode string }

// grantsOn returns the grants on the cache file called file (under downloads/).
func grantsOn(dump, file string) []uriGrant {
	var out []uriGrant
	for _, m := range grantRe.FindAllStringSubmatch(dump, -1) {
		if m[1] == file {
			out = append(out, uriGrant{File: m[1], Source: m[2], Target: m[3], Mode: m[4]})
		}
	}
	return out
}

func TestGrantsOn(t *testing.T) {
	dump := `UID 10199 holds:
  UriPermission{4f2a content://tech.hybriq.adbuddy.files/downloads/00ff-big.pdf [user 0]}
    targetUserId=0 sourcePkg=tech.hybriq.adbuddy targetPkg=com.google.android.apps.docs
    mode=0x1 owned=0x1 readOwners: ActivityRecord{x}
  UriPermission{4f2b content://tech.hybriq.adbuddy.files/downloads/00ff-other.pdf [user 0]} targetUserId=0 sourcePkg=tech.hybriq.adbuddy targetPkg=com.android.intentresolver mode=0x3 owned=0x1`
	got := grantsOn(dump, "00ff-big.pdf")
	want := uriGrant{"00ff-big.pdf", "tech.hybriq.adbuddy", "com.google.android.apps.docs", "0x1"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("grantsOn = %+v, want %+v", got, want)
	}
}

// sampleGrants polls `dumpsys activity permissions` (a grant goes when the
// target activity ends) until a grant on file shows, or d passes.
func (e *emu) sampleGrants(t *testing.T, file string, d time.Duration) []uriGrant {
	t.Helper()
	var got []uriGrant
	poll(d, func() bool {
		got = grantsOn(e.shell(t, "dumpsys activity permissions"), file)
		return len(got) > 0
	})
	return got
}

// startLine waits up to d for a START line of the activity manager in logcat
// that has all of the parts, and returns it ("" when none came).
func (e *emu) startLine(t *testing.T, d time.Duration, parts ...string) string {
	t.Helper()
	var found string
	poll(d, func() bool {
		for _, l := range startLineR.FindAllString(e.shell(t, "logcat -d -s ActivityTaskManager:I"), -1) {
			if !slices.ContainsFunc(parts, func(p string) bool { return !strings.Contains(l, p) }) {
				found = l
				return true
			}
		}
		return false
	})
	return found
}

// latest returns the node with that text lowest on screen (the newest message).
func latest(nodes []uiNode, text string) (uiNode, bool) {
	var best uiNode
	ok := false
	for _, n := range nodes {
		if n.Text == text && (!ok || n.Y > best.Y) {
			best, ok = n, true
		}
	}
	return best, ok
}

// backToChat brings the app's chat back after another app was on top.
func (e *emu) backToChat(t *testing.T) {
	t.Helper()
	e.shell(t, "am start -W -n "+phoneapp.Activity)
	e.waitText(t, "Message the agent", 15*time.Second)
}

// download publishes f as an assistant message, taps its card, serves it and
// waits until openCards cards say they can be opened.
func (l *link) download(t *testing.T, msgID string, f hostFile, openCards int) {
	t.Helper()
	l.drain()
	l.say(t, msgID, f)
	e := l.e
	g, req := l.tapForGet(t, "Tap to download")
	if g.ID != f.att.ID || g.Offset != 0 {
		t.Fatalf("the tap asked %+v, want %s from 0", g, f.att.ID)
	}
	l.sendFile(t, req, f.data)
	e.waitCount(t, "Tap to open", openCards, 15*time.Second)
}

// checkGrants checks the grants on file: there is one to target, and every
// one is from the app with the mode wantMode (0x1: read only).
func checkGrants(t *testing.T, grants []uriGrant, file, target, wantMode string) {
	t.Helper()
	found := false
	for _, g := range grants {
		if g.Source != phoneapp.Package {
			t.Errorf("grant on %s from %s, not from the app", file, g.Source)
		}
		if g.Mode != wantMode {
			t.Errorf("grant on %s to %s has mode %s, want %s (0x1 is read only)", file, g.Target, g.Mode, wantMode)
		}
		if g.Target == target {
			found = true
		}
	}
	if !found {
		t.Errorf("no grant on %s to %s (grants: %+v)", file, target, grants)
	}
}

// Validation 3 of M3b (d): tapping a downloaded file opens it with the app
// the system has for its type, or shares it when none does. What is checked
// is what the app asks of the system, not how it looks (no visual judgement):
// the activity manager's START line (action VIEW, a content://
// tech.hybriq.adbuddy.files/ URI, the host's MIME type, the read-grant flag,
// the target app) and the temporary permission `dumpsys activity permissions`
// shows (from the app, read only, to that target).
//
// A PDF opens in the viewer the image has (Drive on google_apis; with none
// the test expects the share sheet) and a type nothing opens
// (application/x-adbuddy-test) ends in the share sheet, "Sharing 1 file",
// whose file name (the cache's: the id and the name) uiautomator reads. That
// the PDF opens in the viewer's activity is checked by the window focus; what
// the viewer then paints is not looked at (not automatable without judging an
// image of another app, and not needed: the app's part ends at the intent).
func TestOpenAndShare(t *testing.T) {
	mutate := mutation(t, "pdf-as-octet", "expect-write")
	l := openLink(t)
	e := l.e
	l.openChat(t)
	wantMode := "0x1"
	if mutate == "expect-write" {
		wantMode = "0x3"
	}
	const pdfMIME, oddMIME = "application/pdf", "application/x-adbuddy-test"
	if v := e.viewerOf(t, oddMIME); v != "" {
		t.Fatalf("%s opens %s on this image: the share branch cannot be tested", v, oddMIME)
	}
	viewer := e.viewerOf(t, pdfMIME)
	if viewer != "" {
		e.atCleanup(t, "shell", "am force-stop "+viewer)
	}

	// Open: the PDF in its viewer.
	const tiny = "%PDF-1.1\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"
	pdf := newHostFile(t, "doc.pdf", pdfMIME, []byte(tiny))
	if mutate == "pdf-as-octet" {
		pdf.att.MIME = "application/octet-stream"
	}
	l.download(t, "o1", pdf, 1)
	e.shell(t, "logcat -c")
	e.tap(t, e.waitText(t, "Tap to open", 5*time.Second))
	file := pdf.att.ID + "-doc.pdf"
	commonView := []string{"act=android.intent.action.VIEW", "dat=content://tech.hybriq.adbuddy.files/"}
	if viewer != "" {
		parts := append(slices.Clone(commonView), "typ="+pdfMIME, "flg=0x10000001", "cmp="+viewer+"/", "result code=0")
		if e.startLine(t, 15*time.Second, parts...) == "" {
			t.Fatalf("no START of VIEW %s into %s in logcat:\n%s", pdfMIME, viewer, e.shell(t, "logcat -d -s ActivityTaskManager:I"))
		}
		// The viewer is the resumed activity (the window focus alone is null while
		// the window animates in).
		var top string
		if !poll(10*time.Second, func() bool {
			top = e.shell(t, "dumpsys activity activities | grep -E \"topResumedActivity|ResumedActivity\"; dumpsys window | grep mCurrentFocus; true")
			return strings.Contains(top, viewer+"/")
		}) {
			t.Errorf("%s is not the resumed activity after opening the PDF: %s\nlogcat: %s", viewer, top, e.shell(t, "logcat -d -t 60 AndroidRuntime:E ActivityTaskManager:I *:S"))
		}
		checkGrants(t, e.sampleGrants(t, file, 10*time.Second), file, viewer, wantMode)
	} else if e.startLine(t, 15*time.Second, append(slices.Clone(commonView), "typ="+pdfMIME, "result code=-91")...) == "" {
		t.Fatalf("the image has no viewer for %s and no VIEW START with result code=-91 came", pdfMIME)
	}
	e.backToChat(t)

	// Share: a type nothing opens.
	odd := newHostFile(t, "odd.xyz", oddMIME, noise(100<<10, 7))
	l.download(t, "o2", odd, 2)
	e.shell(t, "logcat -c")
	card, ok := latest(e.screen(t), "Tap to open")
	if !ok {
		t.Fatal("no card to open")
	}
	e.atCleanup(t, "shell", "input keyevent KEYCODE_BACK")
	e.tap(t, card)
	oddFile := odd.att.ID + "-odd.xyz"
	if e.startLine(t, 15*time.Second, append(slices.Clone(commonView), "typ="+oddMIME, "flg=0x10000001", "result code=-91")...) == "" {
		t.Fatalf("no START of VIEW %s with result code=-91 (no app opens it):\n%s", oddMIME, e.shell(t, "logcat -d -s ActivityTaskManager:I"))
	}
	if e.startLine(t, 15*time.Second, "act=android.intent.action.CHOOSER", "cmp=com.android.intentresolver/") == "" {
		t.Fatalf("no CHOOSER START after the VIEW with no target:\n%s", e.shell(t, "logcat -d -s ActivityTaskManager:I"))
	}
	// The VIEW with no target comes first and the share sheet after it.
	if log := e.shell(t, "logcat -d -s ActivityTaskManager:I"); strings.Index(log, "result code=-91") > strings.Index(log, "act=android.intent.action.CHOOSER") {
		t.Errorf("the CHOOSER START is before the VIEW that found no target:\n%s", log)
	}
	e.waitNode(t, "Sharing 1 file", 15*time.Second, func(n uiNode) bool {
		return n.ID == "com.android.intentresolver:id/headline" && n.Text == "Sharing 1 file"
	})
	e.waitNode(t, "the shared file's name", 5*time.Second, func(n uiNode) bool {
		// The cache's name is the id plus the name: that the sheet shows the id of
		// 32 hex digits is a finding of T1 about the product, not a contract, so
		// only the end is required.
		return n.ID == "com.android.intentresolver:id/content_preview_filename" && strings.HasSuffix(n.Text, "-odd.xyz")
	})
	checkGrants(t, e.sampleGrants(t, oddFile, 10*time.Second), oddFile, "com.android.intentresolver", wantMode)
	e.shell(t, "input keyevent KEYCODE_BACK")
	e.backToChat(t)
}
