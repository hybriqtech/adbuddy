package pty

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// V08: a Write stuck in the console's input (a program that does not read,
// and a conhost that stopped taking it) does not hold Kill: closing the
// input ends that Write instead of waiting for it.
func TestClosingTheInputEndsAStuckWrite(t *testing.T) {
	var r, w windows.Handle
	if err := windows.CreatePipe(&r, &w, nil, 0); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(r) // nobody reads it
	in := os.NewFile(uintptr(w), "conpty-in")
	res := make(chan error, 1)
	go func() {
		_, err := in.Write(make([]byte, 1<<20))
		res <- err
	}()
	select {
	case err := <-res:
		t.Fatalf("the Write did not get stuck: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	start := time.Now()
	if !closeNoWait(in, 5*time.Second) {
		t.Fatal("Close waited for the stuck Write")
	}
	select {
	case err := <-res:
		if err == nil {
			t.Fatal("the stuck Write ended without an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stuck Write did not end")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("closing took %v", d)
	}
}
