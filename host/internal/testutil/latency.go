package testutil

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// LatencyOn says whether the latency harness (ADBUDDY_LATENCY=1) was asked
// for: its tests measure, they do not check anything, and are slow.
func LatencyOn() bool { return os.Getenv("ADBUDDY_LATENCY") == "1" }

// Samples are durations of one thing measured several times.
type Samples []time.Duration

// Median, P90 and Min of s (0 when empty).
func (s Samples) Median() time.Duration { return s.pct(50) }
func (s Samples) P90() time.Duration    { return s.pct(90) }
func (s Samples) Min() time.Duration    { return s.pct(0) }

func (s Samples) pct(p int) time.Duration {
	if len(s) == 0 {
		return 0
	}
	c := slices.Clone(s)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	i := (len(c) - 1) * p / 100
	return c[i]
}

// Ms formats d in milliseconds with one decimal.
func Ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d.Microseconds())/1000) }

// Measure runs f n times after warmup runs that are dropped, and returns the
// durations of the n.
func Measure(warmup, n int, f func()) Samples {
	for range warmup {
		f()
	}
	out := make(Samples, 0, n)
	for range n {
		t0 := time.Now()
		f()
		out = append(out, time.Since(t0))
	}
	return out
}

// Report collects rows (name, samples) and writes them as a Markdown table
// to $ADBUDDY_LATENCY_OUT (appending) and to the test's log function.
type Report struct {
	mu    sync.Mutex
	title string
	rows  []string
}

// NewReport starts a report with a title.
func NewReport(title string) *Report { return &Report{title: title} }

// Add a row of samples.
func (r *Report) Add(name string, s Samples) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, fmt.Sprintf("| %s | %d | %s | %s | %s |", name, len(s), Ms(s.Median()), Ms(s.P90()), Ms(s.Min())))
}

// Note adds a free line under the table.
func (r *Report) Note(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, "\n"+fmt.Sprintf(format, a...)+"\n")
}

// Flush writes the report.
func (r *Report) Flush(logf func(string, ...any)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "\n### %s\n\n| what | n | median ms | p90 ms | min ms |\n|---|---|---|---|---|\n", r.title)
	for _, row := range r.rows {
		b.WriteString(row + "\n")
	}
	logf("%s", b.String())
	if p := os.Getenv("ADBUDDY_LATENCY_OUT"); p != "" {
		if f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.WriteString(b.String())
			_ = f.Close()
		}
	}
}
