package cli

import (
	"errors"
	"strings"
)

// Exit codes. They are part of the CLI contract and are listed in `adbuddy --help`.
const (
	ExitOK        = 0
	ExitFailure   = 1
	ExitUsage     = 2
	ExitTransient = 75
	ExitConfig    = 78
)

// Error is the structured error every command returns. Human mode prints it on
// stderr; --json mode prints it inside the {"ok":false,"error":{…}} envelope.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Fix       string `json:"fix,omitempty"`
	Transient bool   `json:"transient"`
	exit      int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// NewError builds a domain failure (exit code 1).
func NewError(code, message, fix string) *Error {
	return &Error{Code: code, Message: message, Fix: fix, exit: ExitFailure}
}

// TransientError builds a failure that may resolve by retrying (exit code 75).
func TransientError(code, message, fix string) *Error {
	return &Error{Code: code, Message: message, Fix: fix, Transient: true, exit: ExitTransient}
}

// ConfigError builds a configuration failure (exit code 78).
func ConfigError(code, message, fix string) *Error {
	return &Error{Code: code, Message: message, Fix: fix, exit: ExitConfig}
}

// UsageError builds an invalid-usage failure (exit code 2).
func UsageError(message string) *Error {
	return &Error{Code: "USAGE", Message: message, Fix: "Run `adbuddy --help` to see the usage.", exit: ExitUsage}
}

// AsError converts any error into an *Error. Cobra's own parsing errors
// become USAGE errors; anything else becomes INTERNAL.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand flag", "flag needs an argument", "invalid argument", "accepts ", "requires at least", "requires at most"} {
		if strings.HasPrefix(msg, p) {
			return UsageError(msg)
		}
	}
	return &Error{Code: "INTERNAL", Message: msg, exit: ExitFailure}
}

// ExitCode maps an error returned by a command to the process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var code *exitCodeError
	if errors.As(err, &code) {
		return code.code
	}
	e := AsError(err)
	if e.exit != 0 {
		return e.exit
	}
	if e.Transient {
		return ExitTransient
	}
	return ExitFailure
}

// exitCodeError carries a child process exit code (adbuddy run) without printing anything.
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return "exit status" }

// ExitWith makes the command end with the given exit code and no error message.
func ExitWith(code int) error {
	if code == 0 {
		return nil
	}
	return &exitCodeError{code: code}
}
