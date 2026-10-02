//go:build unix

package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// A FIFO planted as this month's file hangs neither Load (adbuddy journal,
// undo) nor the next Append: both are errors that name it.
func TestFifoMonthFileDoesNotHang(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(s.Dir(), "2026-10.jsonl"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRegular) || !strings.Contains(err.Error(), "2026-10.jsonl") {
			done <- fmt.Errorf("Load: %v, want ErrNotRegular naming the file", err)
			return
		}
		if err := s.Append(Entry{ID: "j_1"}); !errors.Is(err, safeio.ErrNotRegular) {
			done <- fmt.Errorf("Append: %v, want ErrNotRegular", err)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the journal hangs on a FIFO")
	}
}
