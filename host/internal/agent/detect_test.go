package agent

import (
	"errors"
	"testing"
)

func TestDetectKeepsRecommendedOrder(t *testing.T) {
	installed := map[string]string{"gemini": "/bin/gemini", "claude": "/bin/claude"}
	look := func(name string) (string, error) {
		if p, ok := installed[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
	got := Detect(look)
	if len(got) != 2 || got[0] != (Found{"claude", "/bin/claude"}) || got[1].Name != "gemini" {
		t.Fatalf("%+v", got)
	}
	if len(Detect(func(string) (string, error) { return "", errors.New("no") })) != 0 {
		t.Fatal("nothing installed")
	}
}
