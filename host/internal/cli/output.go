package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// WriteData writes the success envelope {"ok":true,"data":…} as one JSON line.
func WriteData(w io.Writer, data any) error {
	return writeJSON(w, envelope{OK: true, Data: data})
}

// WriteFailure reports err: the JSON envelope on stdout in --json mode, or a
// human message on stderr otherwise (most important line last).
func WriteFailure(stdout, stderr io.Writer, jsonMode bool, err error) {
	var code *exitCodeError
	if errors.As(err, &code) {
		return // a child process already explained itself
	}
	e := AsError(err)
	if jsonMode {
		_ = writeJSON(stdout, envelope{OK: false, Error: e})
		return
	}
	fmt.Fprintf(stderr, "Error: %s — %s\n", e.Code, e.Message)
	if e.Fix != "" {
		fmt.Fprintf(stderr, "Fix: %s\n", e.Fix)
	}
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
