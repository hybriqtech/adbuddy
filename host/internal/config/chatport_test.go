package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChatPort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	os.WriteFile(p, []byte("chat_port = 47123\n"), 0o600)
	c, err := Load(p)
	if err != nil || c.ChatPort != 47123 {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{"chat_port = 80\n", "chat_port = 70000\n"} {
		os.WriteFile(p, []byte(bad), 0o600)
		if _, err := Load(p); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
