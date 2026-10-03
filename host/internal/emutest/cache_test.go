package emutest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The cache of the downloads (validation 3 of M3b, e). The app keeps
// cache/downloads under 500 MiB = 524 288 000 bytes (FileCache.kt): after a
// download completes it trims in the background, oldest modification time
// first, and a .part (a download in progress or one to resume) is neither
// counted nor ever deleted. There is no "file in use" (T1).
const cacheCap = 500 << 20

// freeKB is the free space of the emulator's /data in KB.
func (e *emu) freeKB(t *testing.T) int64 {
	t.Helper()
	lines := strings.Split(e.shell(t, "df -k /data"), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if len(f) < 4 {
		t.Fatalf("df -k /data: %q", lines)
	}
	n, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		t.Fatalf("df -k /data: %q: %v", lines, err)
	}
	return n
}

// fillCache creates cache/downloads/name of mib MiB of zeros dated date (as
// the app, so the app can delete it).
func (e *emu) fillCache(t *testing.T, name string, mib int, date string) {
	t.Helper()
	e.runAs(t, fmt.Sprintf(`sh -c 'dd if=/dev/zero of=cache/downloads/%s bs=1M count=%d 2>/dev/null && touch -d "%s" cache/downloads/%s'`, name, mib, date, name))
	if got := e.cacheLs(t)[name]; got != int64(mib)<<20 {
		t.Fatalf("cache/downloads/%s has %d bytes, want %d", name, got, int64(mib)<<20)
	}
}

// cacheBytes adds the sizes in files; a .part counts only when withPart.
func cacheBytes(files map[string]int64, withPart bool) int64 {
	var n int64
	for name, size := range files {
		if strings.HasSuffix(name, ".part") && !withPart {
			continue
		}
		n += size
	}
	return n
}

func TestCacheBytes(t *testing.T) {
	files := map[string]int64{"a-x.bin": 10, "b.part": 100}
	if got := cacheBytes(files, false); got != 10 {
		t.Errorf("without the .part: %d, want 10", got)
	}
	if got := cacheBytes(files, true); got != 110 {
		t.Errorf("with the .part: %d, want 110", got)
	}
}

// The cache is filled with four files of 125 MiB (exactly the cap) dated 2020
// and a .part of 150 MiB dated 2019 (the oldest of all, which a trim that
// counted .part files would delete first); a small PDF comes by the channel
// and the app trims by itself, asynchronously, so it is polled. Checked: the
// oldest finished file went, the others and the new one stayed, what is left
// (the .part not counted) is within the cap, and the .part is untouched.
//
// Mutations (ADBUDDY_EMUTEST_MUTATE, see files_test.go):
//
//	count-part  the test measures the cache with the .part included (as `du`
//	            of the whole folder does): it is over the cap
//	no-trigger  no file comes by the channel, so nothing trims
//	reverse-age the oldest file is made the newest after the fill, so the app
//	            deletes another one first
func TestCacheTrimsTheOldestAndKeepsTheParts(t *testing.T) {
	mutate := mutation(t, "count-part", "no-trigger", "reverse-age")
	l := openLink(t)
	e := l.e
	l.openChat(t)
	if free := e.freeKB(t); free < 2<<20 {
		t.Fatalf("/data has %d KB free: the fill (650 MiB) and the rest do not fit", free)
	}
	e.runAs(t, "mkdir -p cache/downloads")

	// In order of modification time, oldest first. The names, the order of
	// creation and the dates all differ, so only the mtime says which goes
	// first: the oldest sorts LAST by name and is created last; the next
	// oldest sorts FIRST by name and is created first. (The ids are 32 hex
	// digits as the app's own.)
	id := func(c string) string { return strings.Repeat(c, 32) }
	olds := []string{
		id("f") + "-oldest.bin", // 2020-01: the one that must go
		id("a") + "-second.bin", // 2020-02
		id("b") + "-third.bin",  // 2020-03
		id("c") + "-fourth.bin", // 2020-04
	}
	for _, i := range []int{1, 2, 3, 0} { // creation order = name order
		e.fillCache(t, olds[i], 125, fmt.Sprintf("2020-0%d-01 00:00:00", i+1))
	}
	part := strings.Repeat("b", 32) + ".part"
	e.fillCache(t, part, 150, "2019-01-01 00:00:00")
	if mutate == "reverse-age" {
		e.runAs(t, `touch -d "2025-01-01 00:00:00" cache/downloads/`+olds[0])
	}
	before := e.cacheLs(t)
	if got := cacheBytes(before, false); got != cacheCap {
		t.Fatalf("the fill is %d bytes without the .part, want exactly the cap, %d", got, cacheCap)
	}
	if got := cacheBytes(before, true); got <= cacheCap {
		t.Fatalf("the fill is %d bytes with the .part: it must be over the cap for the .part to matter", got)
	}

	var newName string
	if mutate != "no-trigger" {
		pdf := pdfFile(t, "new.pdf", 1<<20, 6)
		l.download(t, "k1", pdf, 1)
		newName = pdf.att.ID + "-" + pdf.att.Name
		// The trim is asynchronous: wait until the oldest is gone (it took
		// under a second in T1).
		poll(20*time.Second, func() bool {
			_, there := e.cacheLs(t)[olds[0]]
			return !there
		})
		// ... and a little more, for it to have finished: what follows is a
		// negative claim (nothing else was deleted), so it is made after a wait
		// that was preceded by polling for the positive event.
		time.Sleep(2 * time.Second)
	}
	after := e.cacheLs(t)
	if _, there := after[olds[0]]; there {
		t.Errorf("%s, the oldest file, is still in the cache: %v", olds[0], mapKeys(after))
	}
	for _, name := range olds[1:] {
		if _, ok := after[name]; !ok {
			t.Errorf("%s, newer than the one that had to go, was deleted: %v", name, mapKeys(after))
		}
	}
	if newName != "" {
		if _, ok := after[newName]; !ok {
			t.Errorf("the new download %s is not in the cache: %v", newName, mapKeys(after))
		}
	}
	if got, ok := after[part]; !ok || got != 150<<20 {
		t.Errorf("the .part is %d bytes (present %v), want %d: the trim must not touch it", got, ok, int64(150)<<20)
	}
	counted := mutate == "count-part"
	if total := cacheBytes(after, counted); total > cacheCap {
		t.Errorf("the cache holds %d bytes, the cap is %d (.part files counted: %v)", total, cacheCap, counted)
	}
}
