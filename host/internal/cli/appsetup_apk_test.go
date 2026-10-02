package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/paths"
)

// A directory is not an APK: --apk takes a regular file only (V16).
func TestChooseAPKRejectsADirectory(t *testing.T) {
	e := &env{dirs: paths.Dirs{Data: t.TempDir()}}
	_, _, err := chooseAPK(e, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want one saying it is not a regular file", err)
	}
}
