package session

import (
	"sync"
	"testing"
	"time"
)

// alive is the fakes whose process has not ended.
func (r *rig) alive() []*fakePTY {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*fakePTY
	for _, f := range r.fakes {
		select {
		case <-f.done:
		default:
			out = append(out, f)
		}
	}
	return out
}

// settle waits until the number of live agents stops changing (a late
// restart from pump would show up here) and returns them.
func (r *rig) settle() []*fakePTY {
	last, since := -1, time.Now()
	for time.Since(since) < 60*time.Millisecond {
		if n := len(r.alive()); n != last {
			last, since = n, time.Now()
		}
		time.Sleep(5 * time.Millisecond)
	}
	return r.alive()
}

// check: at most maxAlive agents alive, and the Manager's process is the one
// alive; its info agrees, also the last one published.
func (r *rig) check(t *testing.T, round int, maxAlive int) {
	t.Helper()
	alive := r.settle()
	if len(alive) > maxAlive {
		t.Fatalf("round %d: %d agents alive, want at most %d", round, len(alive), maxAlive)
	}
	cur := r.m.current()
	switch {
	case len(alive) == 1 && (cur == nil || cur.p != alive[0]):
		t.Fatalf("round %d: the agent alive is not the session's", round)
	case len(alive) == 0 && cur != nil:
		t.Fatalf("round %d: the session's agent is dead", round)
	}
	info := r.m.Info()
	if (cur == nil) != (info.State == StateStopped) {
		t.Fatalf("round %d: info %+v with process %v", round, info, cur != nil)
	}
	r.mu.Lock()
	last := r.infos[len(r.infos)-1]
	r.mu.Unlock()
	if last.State != info.State || last.Epoch != info.Epoch {
		t.Fatalf("round %d: last info published %+v, current %+v", round, last, info)
	}
}

// rounds is how many times each race runs.
const rounds = 30

func both(a, b func()) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a() }()
	go func() { defer wg.Done(); b() }()
	wg.Wait()
}

// Two Start at once leave one agent, the session's; the other ends.
func TestConcurrentStartsLeaveOneAgent(t *testing.T) {
	for i := 0; i < rounds; i++ {
		r := newRig(t)
		both(func() { r.m.Start("codex", false) }, func() { r.m.Start("gemini", false) })
		r.check(t, i, 1)
		if len(r.alive()) != 1 {
			t.Fatalf("round %d: no agent alive", i)
		}
		r.m.Stop()
	}
}

// Start and Stop at once: no agent is left behind the session's back.
func TestStartAndStopAtOnce(t *testing.T) {
	for i := 0; i < rounds; i++ {
		r := newRig(t)
		r.m.Start("codex", false)
		both(func() { r.m.Start("gemini", false) }, r.m.Stop)
		r.check(t, i, 1)
		r.m.Stop()
	}
}

// The retry of a resumed claude that failed races with a Start: one agent.
func TestRetryRacesWithStart(t *testing.T) {
	for i := 0; i < rounds; i++ {
		r := newRig(t)
		r.m.Start("claude", true)
		both(func() { r.fake(0).exit(1) }, func() { r.m.Start("codex", false) })
		r.check(t, i, 1)
		if len(r.alive()) != 1 {
			t.Fatalf("round %d: no agent alive", i)
		}
		r.m.Stop()
	}
}

// The retry races with a Stop: no agent at the end.
func TestRetryRacesWithStop(t *testing.T) {
	for i := 0; i < rounds; i++ {
		r := newRig(t)
		r.m.Start("claude", true)
		both(func() { r.fake(0).exit(1) }, r.m.Stop)
		r.check(t, i, 0)
	}
}

// holdRetry makes the retry of the resumed claude wait in Look until the
// returned release is called; started tells when it is waiting there.
func (r *rig) holdRetry() (started <-chan struct{}, release func()) {
	in, out := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r.mu.Lock()
	r.onLook = func(name string) {
		if name == "claude" {
			once.Do(func() { close(in); <-out })
		}
	}
	r.mu.Unlock()
	return in, func() { close(out) }
}

// A Start that comes while the retry is starting claude: the retry's agent
// does not outlive it.
func TestStartDuringTheRetry(t *testing.T) {
	r := newRig(t)
	r.m.Start("claude", true)
	waiting, release := r.holdRetry()
	r.fake(0).exit(1)
	<-waiting
	done := make(chan struct{})
	go func() { r.m.Start("codex", false); close(done) }()
	time.Sleep(50 * time.Millisecond) // the Start waits for the retry, or not
	release()
	<-done
	r.check(t, 0, 1)
	if i := r.m.Info(); i.Agent != "codex" {
		t.Fatalf("info %+v, want codex", i)
	}
}

// A Stop that comes while the retry is starting claude: no agent at the end.
func TestStopDuringTheRetry(t *testing.T) {
	r := newRig(t)
	r.m.Start("claude", true)
	waiting, release := r.holdRetry()
	r.fake(0).exit(1)
	<-waiting
	done := make(chan struct{})
	go func() { r.m.Stop(); close(done) }()
	time.Sleep(50 * time.Millisecond)
	release()
	<-done
	r.check(t, 0, 0)
}

// Stop hangs up first; an agent that ends then is not killed (its exit code
// is the hangup's; pump's Kill afterwards only releases the terminal).
func TestStopHangsUpFirst(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	f := r.fake(0)
	r.m.Stop()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hangups != 1 || f.code != 129 {
		t.Fatalf("hangups %d kills %d code %d", f.hangups, f.kills, f.code)
	}
}

// An agent that ignores the hangup is killed after stopWait.
func TestStopKillsADeafAgent(t *testing.T) {
	old := stopWait
	stopWait = 100 * time.Millisecond
	defer func() { stopWait = old }()
	r := newRig(t)
	r.m.Start("codex", false)
	f := r.fake(0)
	f.mu.Lock()
	f.deaf = true
	f.mu.Unlock()
	start := time.Now()
	r.m.Stop()
	if d := time.Since(start); d < stopWait {
		t.Fatalf("killed after %v, before stopWait", d)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hangups != 1 || f.kills == 0 || f.code != 137 {
		t.Fatalf("hangups %d kills %d code %d", f.hangups, f.kills, f.code)
	}
}
