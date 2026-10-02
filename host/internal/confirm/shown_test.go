package confirm

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

const rtlNote = "NOTE: right to left"

// hostile is a question whose three texts carry Hebrew and Arabic letters
// and digits; Detail has a real line break on purpose.
func hostile() Request {
	return Request{ID: "j_9", Title: "מחק א", Detail: "שורה א\nسطر ٣", RawCommand: "adb shell mv א ب",
		ExpiresAt: time.Now().Add(time.Minute), Key: "k"}
}

func TestShownCopiesAndEscapes(t *testing.T) {
	r := hostile()
	orig := r
	s := r.Shown(rtlNote)
	if r != orig {
		t.Fatal("Shown changed the original")
	}
	bs := string(rune(92))
	if s.Title != bs+"u05de"+bs+"u05d7"+bs+"u05e7 "+bs+"u05d0" {
		t.Errorf("Title: %q", s.Title)
	}
	if hasRTL(s.Title) || hasRTL(s.Detail) || hasRTL(s.RawCommand) {
		t.Fatalf("RTL left: %+v", s)
	}
	lines := strings.Split(s.Detail, "\n")
	if len(lines) != 3 || lines[2] != rtlNote {
		t.Fatalf("Detail must keep its line break and end with the note: %q", s.Detail)
	}
	if s.ID != r.ID || s.Key != r.Key || !s.ExpiresAt.Equal(r.ExpiresAt) {
		t.Errorf("other fields changed: %+v", s)
	}
}

func TestShownNoteOnlyForRTL(t *testing.T) {
	plain := Request{Title: "Restart", Detail: "one\ntwo", RawCommand: "adb shell reboot"}
	if got := plain.Shown(rtlNote); got != plain {
		t.Fatalf("a plain question changed: %+v", got)
	}
	cf := Request{Title: "a‮b", Detail: "x", RawCommand: "c"} // a control: escaped, but not RTL letters
	got := cf.Shown(rtlNote)
	if strings.Contains(got.Detail, rtlNote) || strings.Contains(got.Title, "‮") {
		t.Fatalf("Cf only: %+v", got)
	}
	noDetail := Request{RawCommand: "adb shell mv א ב"}
	if d := noDetail.Shown(rtlNote).Detail; d != rtlNote {
		t.Fatalf("RTL only in RawCommand, no Detail: %q", d)
	}
}

func TestTerminalShowsNoRTL(t *testing.T) {
	for lang, note := range map[i18n.Lang]string{i18n.EN: "Some characters read right to left", i18n.ES: "Algunos caracteres se leen de derecha a izquierda"} {
		pr, pw := io.Pipe()
		out := &syncBuffer{}
		term := testTerminal(pr, out, "42").WithLang(lang)
		done := make(chan Decision, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		go func() {
			done <- term.Confirm(ctx, hostile())
		}()
		prompt := "Code: "
		if lang == i18n.ES {
			prompt = "Código: "
		}
		waitFor(t, out, prompt, 1)
		io.WriteString(pw, "42\n")
		if d := <-done; d != Allow {
			t.Fatalf("%s: %v", lang, d)
		}
		s := out.String()
		if hasRTL(s) {
			t.Errorf("%s: RTL in the question:\n%s", lang, s)
		}
		if !strings.Contains(s, note) || strings.Count(s, "u05d0") < 3 {
			t.Errorf("%s: note or escapes missing:\n%s", lang, s)
		}
		pw.Close()
	}
}

func TestTerminalQuestionWithoutRTLHasNoNote(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42")
	done := make(chan Decision, 1)
	go func() { done <- term.Confirm(context.Background(), req()) }()
	waitFor(t, out, "Code: ", 1)
	io.WriteString(pw, "42\n")
	<-done
	if strings.Contains(out.String(), "right to left") {
		t.Fatalf("note without RTL:\n%s", out.String())
	}
}

func TestDenyAllShowsNoRTL(t *testing.T) {
	var out syncBuffer
	if d := (DenyAll{Out: &out}).Confirm(context.Background(), hostile()); d != Nobody {
		t.Fatal(d)
	}
	if out.String() == "" || hasRTL(out.String()) {
		t.Fatalf("DenyAll output: %q", out.String())
	}
}
