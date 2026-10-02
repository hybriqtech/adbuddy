package classify

import (
	"strings"
	"testing"
)

// pgrep -L SIGNAL kills, ss -D FILE writes a file and ss -K closes sockets,
// and ip accepts abbreviations (ip a a ..., ip l s ...) and -batch: none of
// them may be 🟢 (P1, review of the plan: C1, C2, I4).

func TestPgrepSignalIsRed(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"-L 9 x", "-lL9 x", "-fL KILL x", "-L9 x", "x -L 9", "-l -L 9 x", "--signal 9 x", "--signal=9 x",
		"-L", "-d", "-x --foo x", "-d -L 9 x",
	} {
		classifyBoth(t, c, "pgrep", strings.Fields(args), Red)
	}
	for _, line := range []string{"nice pgrep -L 9 x", "timeout 5 pgrep -lL9 x"} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
		if r := c.Classify(append([]string{"shell"}, strings.Fields(line)...)); r.Level != Red {
			t.Errorf("argv %q: %v, want red", line, r.Level)
		}
	}
}

func TestPgrepReadingFormsStayGreen(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"-f com.x", "-l x", "x", "-lf x", "-n x", "-u shell x", "-d , x", "-d, x", "-fl com.x", "-P 1", "-U 2000 x", "-x system_server",
	} {
		classifyBoth(t, c, "pgrep", strings.Fields(args), Green)
	}
	for _, line := range []string{"nice pgrep -f x", "timeout 5 pgrep -l x"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v (%s), want green", line, r.Level, r.Primary.Reason.EN)
		}
	}
}

func TestSsDiagKillAreRed(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"-D /sdcard/x", "-tanD /sdcard/x", "--diag=/sdcard/x", "--diag /sdcard/x", "-K", "--kill", "-tanK", "-F /sdcard/f",
		"-tanF/sdcard/f", "--filter=/sdcard/f", "--foo", "-N x", "-t -D",
	} {
		classifyBoth(t, c, "ss", strings.Fields(args), Red)
	}
	for _, line := range []string{"nice ss -D /sdcard/x", "timeout 5 ss -K"} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
		if r := c.Classify(append([]string{"shell"}, strings.Fields(line)...)); r.Level != Red {
			t.Errorf("argv %q: %v, want red", line, r.Level)
		}
	}
}

func TestSsReadingFormsStayGreen(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"", "-tanp", "-ltn", "-s", "-t state established", "-a", "--tcp --numeric", "-4 -t", "-A tcp", "-tan sport = :80",
	} {
		classifyBoth(t, c, "ss", strings.Fields(args), Green)
	}
	for _, line := range []string{"nice ss -tanp", "timeout 5 ss -ltn"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v (%s), want green", line, r.Level, r.Primary.Reason.EN)
		}
	}
}

func TestIPAbbreviationsAndBatchAreRed(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"a a 192.0.2.4/24 dev wlan0", "l s wlan0 down", "r d default", "-b -", "-batch /sdcard/x", "-force addr show",
		"-b /sdcard/x addr show", "addr add 192.0.2.4/24 dev wlan0", "link set wlan0 down", "addr flush dev wlan0",
		"route del default", "r a default via 192.0.2.1", "n flush all", "a s", "l s", "netns exec x id", "-n x addr", "-- addr",
		"-s -b - addr", "foo", "addr foo", "route sh", "a dev wlan0", "monitor", "tuntap add dev x mode tap", "-f inet addr",
	} {
		classifyBoth(t, c, "ip", strings.Fields(args), Red)
	}
	for _, line := range []string{"nice ip a a 192.0.2.4/24 dev wlan0", "timeout 5 ip -b -"} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
		if r := c.Classify(append([]string{"shell"}, strings.Fields(line)...)); r.Level != Red {
			t.Errorf("argv %q: %v, want red", line, r.Level)
		}
	}
}

func TestIPReadingFormsStayGreen(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"", "addr show wlan0", "addr", "a", "addr show", "addr list", "-4 addr show", "-br addr", "-o link show", "route get 192.0.2.1",
		"route", "r", "r show", "rule list", "link show wlan0", "-s -d link", "addr show dev wlan0", "route show table all",
		"neigh", "neigh show", "-6 -j -p addr", "-c a", "link", "l", "neighbor show", "neighbour show", "-s -s link show", "route get 192.0.2.1 from 192.0.2.2",
	} {
		classifyBoth(t, c, "ip", strings.Fields(args), Green)
	}
	for _, line := range []string{"nice ip addr show wlan0", "timeout 5 ip route"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v (%s), want green", line, r.Level, r.Primary.Reason.EN)
		}
	}
}
