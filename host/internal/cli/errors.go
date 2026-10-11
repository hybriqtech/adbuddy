package cli

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	// fixArgs, when not nil, are the arguments of the fix instead of args.
	fixArgs []any
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
// "Run `adbuddy --help` to see the usage." (error.usage_fix), which forCommand
// replaces with the one of the command that failed.
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
		fix = lang.T(e.fixKey, e.fixArguments()...)
	}
	return message, fix
}

func (e *Error) fixArguments() []any {
	if e.fixArgs != nil {
		return e.fixArgs
	}
	return e.args
}

// forCommand points a usage error at the help of the command that failed, not at the root's.
func (e *Error) forCommand(path string) *Error {
	if e.fixKey != "error.usage_fix" || path == "" {
		return e
	}
	c := *e
	c.fixKey = "error.usage_fix_cmd"
	c.fixArgs = []any{path}
	c.Fix = i18n.EN.T("error.usage_fix_cmd", path)
	return &c
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
	return errorT(ExitInterrupt, false, "INTERRUPTED", "error.interrupted", "", nil)
}

// UsageError builds an invalid-usage failure (exit code 2).
func UsageError(message string) *Error {
	// The message is cobra's (English, a cause); the fix is ours, in the language of the person.
	return errorT(ExitUsage, false, "USAGE", "error.cause", "error.usage_fix", []any{message})
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
		fix = i18n.M(e.fixKey, e.fixArguments()...)
	}
	return message, fix
}

var (
	unknownCommandRe = regexp.MustCompile(`(?s)^unknown command "(.*?)" for "([^"]*)"`)
	argsExactRe      = regexp.MustCompile(`^accepts (\d+) arg\(s\), received (\d+)$`)
	argsAtMostRe     = regexp.MustCompile(`^accepts at most (\d+) arg\(s\), received (\d+)$`)
)

// translateFlagError turns the typed errors of pflag into a usage error with
// our own text, or nil when err is none of them.
func translateFlagError(err error) *Error {
	var notExist *pflag.NotExistError
	var needsValue *pflag.ValueRequiredError
	var badValue *pflag.InvalidValueError
	var badSyntax *pflag.InvalidSyntaxError
	switch {
	case errors.As(err, &notExist):
		name := "--" + notExist.GetSpecifiedName()
		if notExist.GetSpecifiedShortnames() != "" {
			name = "-" + notExist.GetSpecifiedName()[:1]
		}
		return UsageErrorT("err.usage.unknown_flag", name)
	case errors.As(err, &needsValue):
		name := "--" + needsValue.GetSpecifiedName()
		if needsValue.GetSpecifiedShortnames() != "" {
			name = "-" + needsValue.GetSpecifiedName()[:1]
		}
		return UsageErrorT("err.usage.flag_needs_value", name)
	case errors.As(err, &badValue):
		return UsageErrorT("err.usage.bad_value", badValue.GetValue(), "--"+badValue.GetFlag().Name)
	case errors.As(err, &badSyntax):
		return UsageErrorT("err.usage.bad_syntax", badSyntax.GetSpecifiedFlag())
	}
	return nil
}

// translateArgsError translates what cobra reports about commands and
// arguments, which does not go through SetFlagErrorFunc. cmd is the command
// that failed (its name is not in cobra's message for argument counts).
// It returns nil for any other error.
func translateArgsError(root, cmd *cobra.Command, err error) *Error {
	msg := err.Error()
	short := func(path string) string { return strings.TrimPrefix(path, root.CommandPath()+" ") }
	if m := unknownCommandRe.FindStringSubmatch(msg); m != nil {
		// cobra wrote the argument with %q; UsageErrorT quotes it again.
		if u, err := strconv.Unquote(`"` + m[1] + `"`); err == nil {
			m[1] = u
		}
		if m[2] == root.CommandPath() {
			if s := suggestCommand(root, m[1]); s != "" {
				return UsageErrorT("err.usage.unknown_command_suggest", m[1], s)
			}
			return UsageErrorT("err.usage.unknown_command", m[1])
		}
		return UsageErrorT("err.usage.no_args", short(m[2]), m[1])
	}
	name := root.Name()
	if cmd != nil {
		name = short(cmd.CommandPath())
	}
	if m := argsExactRe.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		got, _ := strconv.Atoi(m[2])
		if n == 1 {
			return UsageErrorT("err.usage.args_exact.one", n, name, got)
		}
		return UsageErrorT("err.usage.args_exact.other", n, name, got)
	}
	if m := argsAtMostRe.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		got, _ := strconv.Atoi(m[2])
		if n == 1 {
			return UsageErrorT("err.usage.args_at_most.one", n, name, got)
		}
		return UsageErrorT("err.usage.args_at_most.other", n, name, got)
	}
	return nil
}

// suggestCommand is the command of the root that name most likely meant, or "".
// A prefix of two letters or more wins (the shortest, then the first
// alphabetically); otherwise the closest by edit distance, at most 2.
func suggestCommand(root *cobra.Command, name string) string {
	name = strings.ToLower(name)
	var names []string
	for _, c := range root.Commands() {
		if c.IsAvailableCommand() && c.Name() != "help" && c.Name() != "completion" {
			names = append(names, strings.ToLower(c.Name()))
		}
	}
	sort.Strings(names)
	best := ""
	if len([]rune(name)) >= 2 {
		for _, n := range names {
			if strings.HasPrefix(n, name) && (best == "" || len(n) < len(best)) {
				best = n
			}
		}
		if best != "" {
			return best
		}
	}
	bestDist := 3
	for _, n := range names {
		if d := editDistance(name, n); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
