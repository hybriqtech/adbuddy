package procinfo

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

var errBadStat = errors.New("unexpected /proc/<pid>/stat")

// Get reads /proc/<pid>/stat.
func Get(pid int) (Info, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Info{}, err
	}
	return parseStat(string(b))
}

// parseStat reads the fields of a /proc/<pid>/stat line that matter here.
// The name (field 2) is in parentheses and may hold spaces and ")": the
// fields are counted from the last ")".
func parseStat(s string) (Info, error) {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return Info{}, errBadStat
	}
	f := strings.Fields(s[i+1:])
	// f[0] is field 3 (state), f[1] field 4 (ppid), f[19] field 22 (starttime).
	if len(f) < 20 {
		return Info{}, errBadStat
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return Info{}, errBadStat
	}
	return Info{PPID: ppid, Start: f[19], Zombie: f[0] == "Z" || f[0] == "X"}, nil
}
