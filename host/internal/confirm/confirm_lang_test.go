package confirm

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
)

// M6c §4.1 and §9.2: the question of the terminal in Spanish. What decides is
// the same as in English: the code allows, anything else denies, the end of
// the input is nobody; and the command is always there, whole.

// waitFor waits until out has shown s n times, and a little more (past the
// settle time of the terminal).
func waitFor(t *testing.T, out *syncBuffer, s string, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(out.String(), s) < n {
		if time.Now().After(deadline) {
			t.Fatalf("%q never shown %d times:\n%s", s, n, out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
}

func spanishReq() Request {
	r := req()
	r.Title = "Reiniciar el móvil"
	r.Detail = "puede reiniciar el móvil\nsegunda línea"
	return r
}

func TestSpanishQuestionDecidesLikeTheEnglishOne(t *testing.T) {
	cases := map[string]Decision{"42\n": Allow, " 42 \r\n": Allow, "y\n": Deny, "sí\n": Deny, "yes\n": Deny, "\n": Deny, "24\n": Deny,
		"s\n": Deny, "si\n": Deny, "SÍ\n": Deny, "permitir\n": Deny, "42 extra\n": Deny}
	for input, want := range cases {
		pr, pw := io.Pipe()
		out := &syncBuffer{}
		term := testTerminal(pr, out, "42").WithLang(i18n.ES)
		go func() {
			waitFor(t, out, "Código: ", 1)
			io.WriteString(pw, input)
		}()
		if got := term.Confirm(context.Background(), spanishReq()); got != want {
			t.Errorf("%q: got %v want %v", input, got, want)
		}
		pw.Close()
		for _, s := range []string{"Reiniciar el móvil", "adb shell reboot", "Hace falta tu confirmación", "Motivo: puede reiniciar el móvil", "⚠ Esto puede cortar la conexión con el móvil", "escribe 42"} {
			if !strings.Contains(out.String(), s) {
				t.Errorf("%q: the question lacks %q:\n%s", input, s, out.String())
			}
		}
		for _, s := range []string{"Confirmation needed", "Why:", "To allow it", "This may cut", "Code:"} {
			if strings.Contains(out.String(), s) {
				t.Errorf("%q: the Spanish question has English %q:\n%s", input, s, out.String())
			}
		}
		end := map[Decision]string{Allow: "→ permitido", Deny: "→ denegado"}[want]
		if !strings.Contains(out.String(), end) {
			t.Errorf("%q: no %q at the end:\n%s", input, end, out.String())
		}
	}
}

func TestSpanishDetailLinesAlignUnderTheLabel(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42").WithLang(i18n.ES)
	go func() {
		waitFor(t, out, "Código: ", 1)
		pw.Close()
	}()
	term.Confirm(context.Background(), spanishReq())
	// "   Motivo: " is 11 characters; the second line starts under the first.
	if !strings.Contains(out.String(), "   Motivo: puede reiniciar el móvil\n"+strings.Repeat(" ", 11)+"segunda línea\n") {
		t.Errorf("the detail is not aligned:\n%s", out.String())
	}
}

func TestSpanishNobodyAndExpiry(t *testing.T) {
	out := &syncBuffer{}
	term := testTerminal(strings.NewReader(""), out, "42").WithLang(i18n.ES) // the input ends at once
	if got := term.Confirm(context.Background(), spanishReq()); got != Nobody {
		t.Fatalf("got %v, want Nobody", got)
	}
	if !strings.Contains(out.String(), "→ denegado (no hay más entrada: nadie puede confirmar)") {
		t.Errorf("no Spanish end of input:\n%s", out.String())
	}

	pr, pw := io.Pipe()
	defer pw.Close()
	out = &syncBuffer{}
	term = testTerminal(pr, out, "42").WithLang(i18n.ES)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if got := term.Confirm(ctx, spanishReq()); got != Timeout {
		t.Fatalf("got %v, want Timeout", got)
	}
	if !strings.Contains(out.String(), "→ caducó, denegado") {
		t.Errorf("no Spanish expiry:\n%s", out.String())
	}
}

func TestSpanishWithdrawnAndAbandoned(t *testing.T) {
	run := func(end func(cancel context.CancelCauseFunc)) string {
		pr, pw := io.Pipe()
		defer pw.Close()
		out := &syncBuffer{}
		term := testTerminal(pr, out, "42").WithLang(i18n.ES)
		ctx, cancel := context.WithCancelCause(context.Background())
		go func() {
			waitFor(t, out, "Código: ", 1)
			end(cancel)
		}()
		term.Confirm(ctx, spanishReq())
		return out.String()
	}
	if got := run(func(cancel context.CancelCauseFunc) { cancel(errors.New("paused from the phone")) }); !strings.Contains(got, "→ retirado: paused from the phone") {
		t.Errorf("no Spanish withdrawal:\n%s", got)
	}
	// A fixed reason of ours (the daemon's pause) is said in Spanish too.
	if got := run(func(cancel context.CancelCauseFunc) { cancel(i18n.M("confirm.cause.paused").Err()) }); !strings.Contains(got, "→ retirado: pausado desde el móvil") {
		t.Errorf("a withdrawal reason of ours is not in Spanish:\n%s", got)
	}
	// And the one YOLO withdraws a question with (daemon.errWithdrawnYolo).
	if got := run(func(cancel context.CancelCauseFunc) { cancel(i18n.M("confirm.cause.yolo").Err()) }); !strings.Contains(got, "→ retirado: se activó el modo YOLO") {
		t.Errorf("the YOLO withdrawal reason is not in Spanish:\n%s", got)
	}
	if got := run(func(cancel context.CancelCauseFunc) { cancel(nil) }); !strings.Contains(got, "→ abandonado por el agente") {
		t.Errorf("no Spanish abandonment:\n%s", got)
	}
}

func TestDenyAllInSpanish(t *testing.T) {
	out := &syncBuffer{}
	if got := (DenyAll{Out: out, Lang: i18n.ES}).Confirm(context.Background(), spanishReq()); got != Nobody {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(out.String(), "🔴 Denegado (nadie puede confirmar): Reiniciar el móvil") {
		t.Errorf("got %q", out.String())
	}
	out = &syncBuffer{}
	(DenyAll{Out: out}).Confirm(context.Background(), req())
	if !strings.Contains(out.String(), "🔴 Denied (nobody to confirm): Restart the phone") {
		t.Errorf("English by default: %q", out.String())
	}
}

// Spec §4.1: a key missing in Spanish shows in English, never empty, and the
// question can still be answered.
func TestAMissingSpanishTextFallsBackToEnglishAndCanBeAnswered(t *testing.T) {
	i18ntest.Without(t, i18n.ES, "confirm.howto", "confirm.code", "confirm.special")
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42").WithLang(i18n.ES)
	go func() {
		waitFor(t, out, "Code: ", 1) // the prompt is the English one now
		io.WriteString(pw, "42\n")
	}()
	if got := term.Confirm(context.Background(), spanishReq()); got != Allow {
		t.Fatalf("got %v, want Allow", got)
	}
	pw.Close()
	for _, s := range []string{"To allow it type 42", "This may cut the connection", "Hace falta tu confirmación", "adb shell reboot"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("lacks %q:\n%s", s, out.String())
		}
	}
}
