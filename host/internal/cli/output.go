package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

type envelope struct {
	OK    bool   `json:"ok"`
	Error *Error `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// WriteData writes the success envelope {"ok":true,"data":…} as one JSON line.
func WriteData(w io.Writer, data any) error {
	return writeJSON(w, envelope{OK: true, Data: data})
}

// WriteFailure reports err in English: the JSON envelope on stdout in --json
// mode, or a human message on stderr otherwise (most important line last).
func WriteFailure(stdout, stderr io.Writer, jsonMode bool, err error) {
	WriteFailureIn(stdout, stderr, jsonMode, i18n.EN, err)
}

// WriteFailureIn is WriteFailure with the human message in lang. The JSON
// envelope is always English. The caller passes English for an agent
// (App.speaks).
func WriteFailureIn(stdout, stderr io.Writer, jsonMode bool, lang i18n.Lang, err error) {
	var code *exitCodeError
	if errors.As(err, &code) {
		return // a child process already explained itself
	}
	e := AsError(err)
	if jsonMode {
		_ = writeJSON(stdout, envelope{OK: false, Error: e})
		return
	}
	message, fix := e.in(lang)
	fmt.Fprintln(stderr, lang.T("error.line", e.Code, message))
	if fix != "" {
		fmt.Fprintln(stderr, lang.T("error.fix_line", fix))
	}
}

// writeFailureData writes a failure that still carries a result as one
// envelope {"ok":false,"error":…,"data":…} and returns the error's exit code
// without printing anything else.
func writeFailureData(w io.Writer, e *Error, data any) error {
	if err := writeJSON(w, envelope{OK: false, Error: e, Data: data}); err != nil {
		return err
	}
	return &exitCodeError{code: ExitCode(e)}
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
