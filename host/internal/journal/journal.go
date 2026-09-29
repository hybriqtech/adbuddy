// Package journal stores one JSON line per command in
// <workspace>/journal/YYYY-MM.jsonl (spec §4.4). Records are only appended:
// updating an entry (its final status, or undoneBy) appends the whole entry
// again, and readers keep the last record of each id.
package journal

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	MAC         string    `json:"mac,omitempty"` // HMAC-SHA256 of the record, with the installation key

	// Tampered is set by Load when a signing store finds a record whose MAC
	// does not match: someone else wrote it, so undo must not trust it.
	Tampered bool `json:"-"`
}

// Store appends to and reads a journal directory. It is safe for concurrent use.
type Store struct {
	dir   string
	key   []byte
	scope string // absolute journal folder, part of the signature
	now   func() time.Time
	mu    sync.Mutex
}

// Open returns the store for dir (created on first write). It neither signs
// nor verifies records.
func Open(dir string) *Store { return &Store{dir: dir, now: time.Now} }

// OpenSigned returns a store that signs every record it writes with key and
// marks as Tampered the records whose signature does not match. The journal
// lives in the agent's folder; the key lives in ADBuddy's data folder.
//
// The signature covers the folder's path as filepath.Abs writes it, without
// resolving links or case: the same folder written another way (C:\x and c:\x
// on Windows, or through a symlink) makes authentic records show as "not
// written by ADBuddy". That is a false alarm, never a forged record accepted.
func OpenSigned(dir string, key []byte) *Store {
	scope, err := filepath.Abs(dir)
	if err != nil {
		scope = dir
	}
	return &Store{dir: dir, key: append([]byte{}, key...), scope: filepath.Clean(scope), now: time.Now}
}

// sign returns the record's MAC (computed with MAC empty).
func (s *Store) sign(e Entry) (string, error) {
	e.MAC = ""
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(s.scope + "\n")) // a record copied from another environment does not verify
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil)), nil
}

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
	return os.OpenFile(s.file(), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
}

// Append writes e as one line and syncs it to disk.
func (s *Store) Append(e Entry) error {
	if e.ID == "" {
		return errors.New("journal entry without id")
	}
	if e.TS.IsZero() {
		e.TS = s.now().UTC()
	}
	e.Tampered = false
	if s.key != nil {
		mac, err := s.sign(e)
		if err != nil {
			return err
		}
		e.MAC = mac
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
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err == nil && last[0] != '\n' {
			line = append([]byte{'\n'}, line...) // close a line torn by a crash
		}
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
			if s.key != nil {
				want, err := s.sign(e)
				e.Tampered = err != nil || !hmac.Equal([]byte(want), []byte(e.MAC))
			}
			if i, ok := byID[e.ID]; ok {
				if e.UndoneBy == nil {
					e.UndoneBy = entries[i].UndoneBy // set once, never cleared
				}
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
