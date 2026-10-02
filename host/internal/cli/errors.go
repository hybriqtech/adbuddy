package cli

import (
	"errors"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// Exit codes. They are part of the CLI contract and are listed in `adbuddy --help`.
const (
	ExitOK        = 0
	ExitFailure   = 1
	ExitUsage     = 2
	ExitTransient = 75
	ExitConfig    = 78
	ExitInterrupt = 130 // stopped with Ctrl+C (128 + SIGINT)
)

// Error is the structured error every command returns. Human mode prints it on
// stderr; --json mode prints it inside the {"ok":false,"error":{…}} envelope.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Fix       string `json:"fix"`
	Transient bool   `json:"transient"`
	exit      int

	// key, fixKey and args are the catalog keys of Message and Fix and their
	// arguments, when the error was built with NewErrorT and its relatives.
	// Message and Fix are then the English of those keys, computed when the
	// error is built: --json, the agent and the tests read them, and they never
	// change with the language. Only the human message of WriteFailure is
	// written again in the language of the person (Error.in).
	key, fixKey string
	args        []any
}

// errorT builds an error from catalog keys. fixKey may be "". The arguments
// are shared: the message and the fix pick what they need by index
// (%[1]s, %[2]s). A cause of the system or of another package, which is
// English and is not translated, goes in as the argument of the key
// "error.cause" (the text is "%[1]s").
func errorT(exit int, transient bool, code, key, fixKey string, args []any) *Error {
	e := &Error{Code: code, Message: i18n.EN.T(key, args...), Transient: transient, exit: exit, key: key, fixKey: fixKey, args: args}
	if fixKey != "" {
		e.Fix = i18n.EN.T(fixKey, args...)
	}
	return e
}

// NewErrorT is NewError with catalog keys (exit code 1).
func NewErrorT(code, key, fixKey string, args ...any) *Error {
	return errorT(ExitFailure, false, code, key, fixKey, args)
}

// TransientErrorT is TransientError with catalog keys (exit code 75).
func TransientErrorT(code, key, fixKey string, args ...any) *Error {
	return errorT(ExitTransient, true, code, key, fixKey, args)
}

// ConfigErrorT is ConfigError with catalog keys (exit code 78).
func ConfigErrorT(code, key, fixKey string, args ...any) *Error {
	return errorT(ExitConfig, false, code, key, fixKey, args)
}

// UsageErrorT is UsageError with a catalog key for the message; the fix is
// always "Run `adbuddy --help` to see the usage." (error.usage_fix).
func UsageErrorT(key string, args ...any) *Error {
	return errorT(ExitUsage, false, "USAGE", key, "error.usage_fix", args)
}

// in is the message and the fix in lang: the catalog's when the error has
// keys, else its English fields (an error that is not migrated, a cause).
func (e *Error) in(lang i18n.Lang) (message, fix string) {
	if e.key == "" {
		return e.Message, e.Fix
	}
	message = lang.T(e.key, e.args...)
	if e.fixKey != "" {
		fix = lang.T(e.fixKey, e.args...)
	}
	return message, fix
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

// Interrupted is returned when the user stops a command with Ctrl+C.
func Interrupted() *Error {
	return &Error{Code: "INTERRUPTED", Message: "stopped with Ctrl+C", exit: ExitInterrupt}
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

// msgs is the message and the fix as messages of their own, to be written in
// another language later (doctor does it with the error of a bad config). An
// error without keys gives its English, as it is.
func (e *Error) msgs() (message, fix i18n.Msg) {
	if e.key == "" {
		return i18n.M("error.cause", e.Message), i18n.M("error.cause", e.Fix)
	}
	message = i18n.M(e.key, e.args...)
	if e.fixKey != "" {
		fix = i18n.M(e.fixKey, e.args...)
	}
	return message, fix
}
