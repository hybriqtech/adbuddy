package procinfo

import "testing"

// A name with spaces and ")" does not shift the fields.
func TestParseStat(t *testing.T) {
	line := "4242 (a) b (c)) S 77 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 98765 1000 100\n"
	info, err := parseStat(line)
	if err != nil || info != (Info{PPID: 77, Start: "98765"}) {
		t.Fatalf("%+v %v", info, err)
	}
	if info, err := parseStat("4242 (z) Z 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 5\n"); err != nil || !info.Zombie {
		t.Fatalf("zombie: %+v %v", info, err)
	}
	if _, err := parseStat("4242 (short) S 1"); err == nil {
		t.Fatal("a short line was read")
	}
}
