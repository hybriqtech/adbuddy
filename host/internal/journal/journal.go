// Package journal stores one JSON line per command in
// <workspace>/journal/YYYY-MM.jsonl (spec §4.4). Records are only appended:
// updating an entry (its final status, or undoneBy) appends the whole entry
// again, and readers keep the last record of each id.
package journal

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status values. Running is written before a 🟡/🔴 command starts so the
// inverse is on disk even if the host dies mid-command.
const (
	StatusRunning = "running"
	StatusOK      = "ok"
	StatusError   = "error"
	StatusDenied  = "denied"
	StatusBlocked = "blocked"
	StatusTimeout = "timeout"
)

// Sources.
const (
	SourceAgent = "agent"
	SourceUser  = "user"
	SourceUndo  = "undo"
)

// Inverse is the adb command that undoes an entry.
type Inverse struct {
	Argv []string `json:"argv"`
}

// Entry is one journaled command.
type Entry struct {
	ID          string    `json:"id"`
	TS          time.Time `json:"ts"`
	Session     string    `json:"session"`
	Source      string    `json:"source"`
	Argv        []string  `json:"argv"`
	Level       string    `json:"level"`
	Rule        string    `json:"rule"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	ExitCode    *int      `json:"exitCode"`
	Inverse     *Inverse  `json:"inverse"`
	Backups     []string  `json:"backups"`
	UndoneBy    *string   `json:"undoneBy"`
	Undoes      string    `json:"undoes,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

// Store appends to and reads a journal directory. It is safe for concurrent use.
type Store struct {
	dir string
	now func() time.Time
	mu  sync.Mutex
}

// Open returns the store for dir (created on first write).
func Open(dir string) *Store { return &Store{dir: dir, now: time.Now} }

// Dir is the journal directory.
func (s *Store) Dir() string { return s.dir }

// BackupDir is where the backups of entry id go.
func (s *Store) BackupDir(id string) string { return filepath.Join(s.dir, "backups", id) }

func (s *Store) file() string {
	return filepath.Join(s.dir, s.now().UTC().Format("2006-01")+".jsonl")
}

// Writable checks that a record could be appended right now.
func (s *Store) Writable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.openAppend()
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *Store) openAppend() (*os.File, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(s.file(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
}

// Append writes e as one line and syncs it to disk.
func (s *Store) Append(e Entry) error {
	if e.ID == "" {
		return errors.New("journal entry without id")
	}
	if e.TS.IsZero() {
		e.TS = s.now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.openAppend()
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Load reads every month file and returns the latest record of each entry,
// oldest first. Lines that cannot be parsed (a crash mid-write) are skipped
// and counted.
func (s *Store) Load() (entries []Entry, skipped int, err error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	byID := map[string]int{}
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			return nil, 0, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var e Entry
			if json.Unmarshal([]byte(line), &e) != nil || e.ID == "" {
				skipped++
				continue
			}
			if i, ok := byID[e.ID]; ok {
				entries[i] = e
				continue
			}
			byID[e.ID] = len(entries)
			entries = append(entries, e)
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, 0, err
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].TS.Before(entries[j].TS) })
	return entries, skipped, nil
}

// Get returns the latest record of id.
func (s *Store) Get(id string) (Entry, bool, error) {
	entries, _, err := s.Load()
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID returns prefix + a ULID: sortable by time, 26 characters.
func NewID(prefix string, now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	n := new(big.Int).SetBytes(b[:])
	out := make([]byte, 26)
	mod := new(big.Int)
	base := big.NewInt(32)
	for i := 25; i >= 0; i-- {
		n.DivMod(n, base, mod)
		out[i] = crockford[mod.Int64()]
	}
	return prefix + string(out)
}
