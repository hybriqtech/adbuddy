package procinfo

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

// This process: its parent and a start time that stays the same.
func TestGetThisProcess(t *testing.T) {
	info, err := Get(os.Getpid())
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("%+v %v, want ErrUnsupported", info, err)
		}
		return
	}
	if err != nil || info.PPID != os.Getppid() || info.Start == "" || info.Zombie {
		t.Fatalf("%+v %v; want parent %d", info, err, os.Getppid())
	}
	if again, err := Get(os.Getpid()); err != nil || again.Start != info.Start {
		t.Fatalf("start %q then %q (%v)", info.Start, again.Start, err)
	}
}
