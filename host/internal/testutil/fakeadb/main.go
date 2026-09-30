// Command fakeadb stands in for adb in tests. It never talks to a phone.
//
// Every call appends one JSON line {"args","port","cwd","stdin"} to
// $FAKEADB_LOG. The answer comes from the first rule in $FAKEADB_SCRIPT (a
// JSON file {"rules":[{"match","stdout","stderr","exit","echoStdin","times"}]})
// whose regular expression matches the arguments joined by spaces, without
// the leading "-s SERIAL". A rule with "times" > 0 answers only that many
// calls, then it is skipped (successive answers: list the rules in order).
// Its count lives in a file next to the script. Without a matching rule it
// prints nothing and exits 0.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type rule struct {
	Match     string `json:"match"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Exit      int    `json:"exit"`
	EchoStdin bool   `json:"echoStdin"`
	Times     int    `json:"times"`
}

func main() {
	args := os.Args[1:]
	key := args
	if len(key) >= 2 && key[0] == "-s" {
		key = key[2:]
	}
	joined := strings.Join(key, " ")

	var r rule
	if path := os.Getenv("FAKEADB_SCRIPT"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakeadb:", err)
			os.Exit(99)
		}
		var script struct{ Rules []rule }
		if err := json.Unmarshal(data, &script); err != nil {
			fmt.Fprintln(os.Stderr, "fakeadb:", err)
			os.Exit(99)
		}
		for i, candidate := range script.Rules {
			if !regexp.MustCompile(candidate.Match).MatchString(joined) {
				continue
			}
			if candidate.Times > 0 && !use(path, i, candidate.Times) {
				continue
			}
			r = candidate
			break
		}
	}

	var stdin []byte
	if r.EchoStdin {
		stdin, _ = io.ReadAll(os.Stdin)
	}
	if path := os.Getenv("FAKEADB_LOG"); path != "" {
		cwd, _ := os.Getwd()
		line, _ := json.Marshal(map[string]any{"args": args, "port": os.Getenv("ANDROID_ADB_SERVER_PORT"), "cwd": cwd, "stdin": string(stdin), "serial": os.Getenv("ANDROID_SERIAL"), "socket": os.Getenv("ADB_SERVER_SOCKET"), "address": os.Getenv("ANDROID_ADB_SERVER_ADDRESS")})
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			f.Write(append(line, '\n'))
			f.Close()
		}
	}
	os.Stdout.WriteString(r.Stdout)
	if r.EchoStdin {
		os.Stdout.Write(stdin)
	}
	os.Stderr.WriteString(r.Stderr)
	os.Exit(r.Exit)
}

// use counts one more answer of rule i and reports whether it still had
// answers left (fewer than times used).
func use(script string, i, times int) bool {
	counter := script + ".count" + strconv.Itoa(i)
	b, _ := os.ReadFile(counter)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if n >= times {
		return false
	}
	if err := os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "fakeadb:", err)
		os.Exit(99)
	}
	return true
}
