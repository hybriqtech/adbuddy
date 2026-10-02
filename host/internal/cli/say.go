package cli

import (
	"fmt"
	"io"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// langWriter is a writer that knows the language of what is written to it: the
// lines that serve and the pairing say to the person. tell writes them in it,
// so that the functions that only have an io.Writer (a log) do not each have to
// carry the language. A plain writer, a test's buffer, is English.
type langWriter struct {
	io.Writer
	Lang i18n.Lang
}

// tell writes the message key, with its arguments, and a line break, in the
// language of w (English when w is not a langWriter).
func tell(w io.Writer, key string, args ...any) {
	lang := i18n.EN
	if lw, ok := w.(langWriter); ok {
		lang = lw.Lang
	}
	fmt.Fprintln(w, lang.T(key, args...))
}

// Log is where serve and setup write what a person reads as it happens: the
// terminal's error output, in the language of this run.
func (a *App) Log() io.Writer { return langWriter{Writer: a.Stderr, Lang: a.speaks()} }

// Say wraps w, one of the outputs of this run (the standard output), with the
// language of the run: what tell writes to it is in that language.
func (a *App) Say(w io.Writer) io.Writer { return langWriter{Writer: w, Lang: a.speaks()} }
