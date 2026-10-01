package cli

import (
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
)

// cfgStore is the one writer of config.toml while serve runs: the chat port,
// the Tailscale address the app reports and the remote mode's expiry all
// change through it, each on top of what the others saved.
type cfgStore struct {
	mu   sync.Mutex
	path string
	cfg  config.Config // its own copy: nobody else writes the env's config while serve runs
	save func(string, config.Config) error
}

func newCfgStore(path string, cfg config.Config) *cfgStore {
	return &cfgStore{path: path, cfg: cfg, save: config.Save}
}

// Config returns a copy of the current configuration.
func (s *cfgStore) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update applies f and saves; if saving fails the change is undone.
func (s *cfgStore) Update(f func(*config.Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	f(&next)
	if err := s.save(s.path, next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func (s *cfgStore) TailscaleIP() string { return s.Config().TailscaleIP }
func (s *cfgStore) Until() time.Time    { return s.Config().RemoteUntilTime() }

// SetUntil saves the remote mode's expiry; the zero time clears it.
func (s *cfgStore) SetUntil(t time.Time) error {
	return s.Update(func(c *config.Config) {
		if t.IsZero() {
			c.RemoteUntil = ""
		} else {
			c.RemoteUntil = t.UTC().Format(time.RFC3339)
		}
	})
}
