// Package latbench is the model half of the latency harness
// (docs/superpowers/plans/2026-10-03-latencia.md): it runs the real agent
// (Claude Code, headless) on typical tasks and records, for each run, the
// time, the turns, the tokens, the cost and what the model's share of the wait
// was. The phone behind it is a real emulator (internal/emutest) or the fake
// one in this package (Phone), which needs nothing but costs the same tokens.
package latbench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Task is something a user asks the agent from the chat.
type Task struct {
	Name   string
	Prompt string
}

// Tasks are the typical ones: reading the phone without the screen, opening an
// app, moving through two screens, and searching and typing.
var Tasks = []Task{
	{"1 read: Android version and battery", "What Android version is the phone running and what is its battery level? Answer in one short line."},
	{"2 open an app and read it", "Open the Settings app and tell me the first three entries on its main screen. Answer in one short line."},
	{"3 navigate two screens", "In the Settings app, open Network & internet and list the names of the entries it shows. Answer in one short line."},
	{"4 list installed apps", "How many third-party (user-installed) apps does the phone have? Name them. Answer in one short line."},
	{"5 search and type", "In the Settings app, tap the search bar, type the word wifi and tell me the title of the first result. Answer in one short line."},
}

// ModelIDs turns the aliases of the configurations into model ids.
var ModelIDs = map[string]string{"opus": "claude-opus-5-5", "sonnet": "claude-sonnet-5-5", "fable": "claude-fable-5-1"}

// Config is a model and an effort.
type Config struct{ Alias, Effort string }

// ParseConfigs reads "opus:medium,sonnet:low".
func ParseConfigs(s string) []Config {
	var out []Config
	for _, c := range strings.Split(s, ",") {
		a, e, _ := strings.Cut(strings.TrimSpace(c), ":")
		if a != "" {
			out = append(out, Config{a, e})
		}
	}
	return out
}

func (c Config) Model() string {
	if id := ModelIDs[c.Alias]; id != "" {
		return id
	}
	return c.Alias
}

// Usage are the tokens of a run.
type Usage struct {
	In         int64 `json:"input_tokens"`
	Out        int64 `json:"output_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
}

// Result is the part of claude's final event that is flat.
type Result struct {
	IsError    bool    `json:"is_error"`
	Result     string  `json:"result"`
	NumTurns   int     `json:"num_turns"`
	DurationMs int64   `json:"duration_ms"`
	APIMs      int64   `json:"duration_api_ms"`
	Cost       float64 `json:"total_cost_usd"`
}

// Run is one task done by one configuration.
type Run struct {
	Task, Alias, Effort, Model string
	WallMs                     int64
	Result
	Usage Usage
	Err   string

	// From the stream (Follow): when each thing arrived, so the model's time
	// can be told from the commands'.
	StartupMs     int64    // until the init event: Claude Code's own start
	FirstActionMs int64    // until the first message that uses a tool, from the start of the process
	ThinkMs       int64    // the sum of the waits for each assistant message (the model)
	ToolMs        int64    // the sum of the waits for each tool result (the commands, the tool's own overhead included)
	Tools         []string // what each tool call did
	Commands      []string // what the host served, with how long it took
	CommandMs     []int64
	CommandsSum   int64
}

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Result
	Usage Usage `json:"usage"`
}

// Clip squeezes spaces and cuts s to n bytes.
func Clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Follow reads claude's stream-json output until it ends and fills in the
// run's timings.
func (r *Run) Follow(rd io.Reader, t0 time.Time) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	prev := t0
	for sc.Scan() {
		now := time.Now()
		var ev streamEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "system":
			if r.StartupMs == 0 {
				r.StartupMs = now.Sub(t0).Milliseconds()
			}
		case "assistant":
			r.ThinkMs += now.Sub(prev).Milliseconds()
			for _, c := range ev.Message.Content {
				if c.Type != "tool_use" {
					continue
				}
				if r.FirstActionMs == 0 {
					r.FirstActionMs = now.Sub(t0).Milliseconds()
				}
				r.Tools = append(r.Tools, c.Name+" "+Clip(string(c.Input), 90))
			}
		case "user":
			r.ToolMs += now.Sub(prev).Milliseconds()
		case "result":
			r.Result, r.Usage = ev.Result, ev.Usage
		}
		prev = now
	}
}

// CallLog records each request of the agent to the host and how long it took.
type CallLog struct {
	mu    sync.Mutex
	names []string
	ms    []int64
}

func (c *CallLog) Add(name string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
	c.ms = append(c.ms, d.Milliseconds())
}

func (c *CallLog) Take() ([]string, []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, m := c.names, c.ms
	c.names, c.ms = nil, nil
	return n, m
}

// Env is where the agent runs.
type Env struct {
	Claude string   // the claude executable
	Dir    string   // the ADBuddy environment
	Env    []string // as agent.Env builds it, with the data folder of the host
	Budget string   // dollars a run may spend (--max-budget-usd)
	Calls  *CallLog
}

// Do runs the task with the configuration and records it.
func (e Env) Do(ctx context.Context, task Task, cfg Config) Run {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	e.Calls.Take()
	cmd := exec.CommandContext(ctx, e.Claude, "-p", task.Prompt, "--model", cfg.Model(), "--effort", cfg.Effort,
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "bypassPermissions", "--max-budget-usd", e.Budget, "--no-session-persistence",
		"--setting-sources", "project,local", "--strict-mcp-config")
	cmd.Dir, cmd.Env = e.Dir, e.Env
	var errb bytes.Buffer
	cmd.Stderr = &errb
	r := Run{Task: task.Name, Alias: cfg.Alias, Effort: cfg.Effort, Model: cfg.Model()}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		r.Err = err.Error()
		return r
	}
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		r.Err = err.Error()
		return r
	}
	r.Follow(pipe, t0)
	runErr := cmd.Wait()
	r.WallMs = time.Since(t0).Milliseconds()
	r.Commands, r.CommandMs = e.Calls.Take()
	for _, ms := range r.CommandMs {
		r.CommandsSum += ms
	}
	if runErr != nil {
		r.Err = Clip(runErr.Error()+" "+errb.String(), 200)
	}
	if r.IsError && r.Err == "" {
		r.Err = Clip(r.Result.Result, 200)
	}
	if r.Err == "" && r.Result.Result == "" {
		r.Err = "no result event"
	}
	return r
}

// Header is the table's head.
const Header = "| task | model:effort | wall s | start s | first action s | model s | tools s | host cmds s | turns | tool calls | in tok | out tok | cache read | cache write | $ | error |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n"

// Row is r as a line of the table.
func (r Run) Row() string {
	return fmt.Sprintf("| %s | %s:%s | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %d | %d | %d | %d | %d | %d | %.3f | %s |\n", r.Task, r.Alias, r.Effort,
		float64(r.WallMs)/1000, float64(r.StartupMs)/1000, float64(r.FirstActionMs)/1000, float64(r.ThinkMs)/1000, float64(r.ToolMs)/1000, float64(r.CommandsSum)/1000,
		r.NumTurns, len(r.Tools), r.Usage.In, r.Usage.Out, r.Usage.CacheRead, r.Usage.CacheWrite, r.Cost, r.Err)
}

// Describe is r for a test's log: its tools and its answer.
func (r Run) Describe() string {
	return fmt.Sprintf("%s %s:%s wall %.1fs first action %.1fs model %.1fs tools %.1fs turns %d $%.3f err=%q\n  tools: %q\n  commands: %q %v\n  answer: %s",
		r.Task, r.Alias, r.Effort, float64(r.WallMs)/1000, float64(r.FirstActionMs)/1000, float64(r.ThinkMs)/1000, float64(r.ToolMs)/1000, r.NumTurns, r.Cost, r.Err,
		r.Tools, r.Commands, r.CommandMs, Clip(r.Result.Result, 240))
}

// Write appends the runs to path as a Markdown table, and as JSON lines to
// path+".agent.jsonl" (an empty path writes nothing).
func Write(path, title string, runs []Run) {
	if path == "" || len(runs) == 0 {
		return
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "\n### %s\n\n%s", title, Header)
		for _, r := range runs {
			fmt.Fprint(f, r.Row())
		}
		f.Close()
	}
	if f, err := os.OpenFile(path+".agent.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		for _, r := range runs {
			b, _ := json.Marshal(r)
			fmt.Fprintln(f, string(b))
		}
		f.Close()
	}
}

// EnvOr is the environment variable k, or def when it is empty.
func EnvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
