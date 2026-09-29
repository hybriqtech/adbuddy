// Command fakeadb stands in for adb in tests. It never talks to a phone.
//
// Every call appends one JSON line {"args","port","cwd","stdin"} to
// $FAKEADB_LOG. The answer comes from the first rule in $FAKEADB_SCRIPT (a
// JSON file {"rules":[{"match","stdout","stderr","exit","echoStdin"}]}) whose
// regular expression matches the arguments joined by spaces, without the
// leading "-s SERIAL". Without a matching rule it prints nothing and exits 0.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type rule struct {
	Match     string `json:"match"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Exit      int    `json:"exit"`
	EchoStdin bool   `json:"echoStdin"`
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
		for _, candidate := range script.Rules {
			if regexp.MustCompile(candidate.Match).MatchString(joined) {
				r = candidate
				break
			}
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
