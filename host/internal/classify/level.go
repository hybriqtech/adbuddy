// Package classify decides how risky an adb command is (spec §4.2). It is a
// pure function of the command: prechecks that need the phone live in
// package precheck.
package classify

import "fmt"

// Level is the risk level of a command.
type Level int

const (
	Green  Level = iota // read-only: runs and is journaled
	Yellow              // reversible or harmless: runs, journaled with its inverse
	Red                 // irreversible, dangerous or unknown: needs confirmation
)

var levelNames = [...]string{"green", "yellow", "red"}

func (l Level) String() string {
	if l < Green || l > Red {
		return fmt.Sprintf("Level(%d)", int(l))
	}
	return levelNames[l]
}

// Emoji is the traffic-light symbol used in human output.
func (l Level) Emoji() string {
	switch l {
	case Green:
		return "🟢"
	case Yellow:
		return "🟡"
	default:
		return "🔴"
	}
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = v
	return nil
}

// ParseLevel parses "green", "yellow" or "red".
func ParseLevel(s string) (Level, error) {
	for i, n := range levelNames {
		if n == s {
			return Level(i), nil
		}
	}
	return Red, fmt.Errorf("unknown level %q (want green, yellow or red)", s)
}

func maxLevel(a, b Level) Level {
	if a > b {
		return a
	}
	return b
}
