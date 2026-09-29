package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Main(args, strings.NewReader(""), &out, &errb)
	return out.String(), errb.String(), code
}

func TestVersionJSONEnvelope(t *testing.T) {
	out, errOut, code := run(t, "version", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not JSON: %q (%v)", out, err)
	}
	if !env.OK || env.Data.Version != Version {
		t.Fatalf("unexpected envelope: %s", out)
	}
	if errOut != "" {
		t.Fatalf("stderr must be empty in --json mode, got %q", errOut)
	}
}

func TestVersionHuman(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 || !strings.HasPrefix(out, "adbuddy "+Version) {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	out, errOut, code := run(t, "frobnicate")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
	if out != "" {
		t.Fatalf("stdout must stay empty on errors in human mode, got %q", out)
	}
	if !strings.Contains(errOut, "Error: USAGE") || !strings.Contains(errOut, "Fix:") {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestUnknownFlagJSONError(t *testing.T) {
	out, _, code := run(t, "version", "--json", "--nope")
	if code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.OK || env.Error == nil || env.Error.Code != "USAGE" {
		t.Fatalf("stdout %q (%v)", out, err)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{NewError("X", "m", ""), 1},
		{TransientError("X", "m", ""), 75},
		{ConfigError("X", "m", ""), 78},
		{UsageError("m"), 2},
		{errors.New("boom"), 1},
		{ExitWith(3), 3},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestHelpListsExitCodes(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Exit codes:") || !strings.Contains(out, "78") {
		t.Fatalf("exit %d, help %q", code, out)
	}
}
