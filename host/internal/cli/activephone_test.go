package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
)

// loggedSerials reads the -s of every call the fake adb logged.
func loggedSerials(t *testing.T, log string) []string {
	t.Helper()
	f, err := os.Open(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c struct{ Args []string }
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		s := ""
		if len(c.Args) >= 2 && c.Args[0] == "-s" {
			s = c.Args[1]
		}
		out = append(out, s)
	}
	return out
}

// phoneWith builds an activePhone over the fake adb; script is the JSON rules
// ("" for none). The globals of the test are read here, before any goroutine.
func phoneWith(t *testing.T, serial, script string) (*activePhone, string) {
	t.Helper()
	bin := fakeADB(t)
	tmp := t.TempDir()
	sp := ""
	if script != "" {
		sp = filepath.Join(tmp, "script.json")
		if err := os.WriteFile(sp, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(tmp, "adb.log")
	t.Setenv("FAKEADB_SCRIPT", sp)
	t.Setenv("FAKEADB_LOG", log)
	return newActivePhone(adb.Runner{Path: bin, Serial: serial}), log
}

func TestActivePhoneSwitchChangesNextCommand(t *testing.T) {
	p, log := phoneWith(t, "old", "")
	run := func() {
		if _, err := p.Run(context.Background(), "", []string{"shell", "ls"}, nil, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	run()
	p.Switch("new")
	run()
	if got := loggedSerials(t, log); !reflect.DeepEqual(got, []string{"old", "new"}) {
		t.Fatalf("serials %q", got)
	}
	if p.Serial() != "new" || p.Runner().Serial != "new" {
		t.Fatalf("serial %q", p.Serial())
	}
}

func TestActivePhoneSerialsNoDuplicatesCurrentFirst(t *testing.T) {
	p := newActivePhone(adb.Runner{Serial: "a"})
	p.Switch("b")
	p.Switch("b")
	p.Switch("a")
	p.Switch("c")
	p.Switch("b")
	if got, want := p.Serials(), []string{"b", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("serials %q, want %q", got, want)
	}
	got := p.Serials()
	got[0] = "mutated"
	if p.Serials()[0] != "b" {
		t.Fatal("Serials returns the internal slice")
	}
}

func TestActivePhoneConcurrentRunAndSwitch(t *testing.T) {
	p, log := phoneWith(t, "s0", "")
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Run(ctx, "", []string{"shell", "ls"}, nil, io.Discard, io.Discard); err != nil {
				t.Error(err)
			}
			_ = p.Serial()
			_ = p.Serials()
		}()
	}
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p.Switch("s" + string(rune('0'+n%10)))
		}(i)
	}
	wg.Wait()
	got := loggedSerials(t, log)
	if len(got) != 50 {
		t.Fatalf("%d calls logged, want 50", len(got))
	}
	for _, s := range got {
		if s == "" {
			t.Fatal("a command ran with an empty -s")
		}
	}
}

// A command already started finishes with the serial it started with.
func TestActivePhoneRunningCommandKeepsItsSerial(t *testing.T) {
	p, log := phoneWith(t, "old", `{"rules":[{"match":"^shell slow$","sleepMs":300}]}`)
	done := make(chan error, 1)
	go func() {
		_, err := p.Run(context.Background(), "", []string{"shell", "slow"}, nil, io.Discard, io.Discard)
		done <- err
	}()
	// The fake logs the call before it waits: once it is logged, the command
	// has started with its serial.
	deadline := time.Now().Add(30 * time.Second)
	for len(loggedSerials(t, log)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the command never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.Switch("new")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := loggedSerials(t, log); !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatalf("serials %q", got)
	}
	if p.Serial() != "new" {
		t.Fatalf("serial %q", p.Serial())
	}
}
