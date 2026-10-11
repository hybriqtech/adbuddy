package cli

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// Fixes that legitimately end with an argument (a name, a command).
var argSuffixOK = map[string]bool{
	"err.serve.device_not_found.fix": true,
	"err.run.agent_not_found.fix":    true,
}

// A fix is a sentence: it starts with a capital and ends with a full stop.
// The ones that start or end with an argument (a command, a name) are exempt.
func TestFixStyle(t *testing.T) {
	for _, p := range i18n.Pairs() {
		if !strings.HasPrefix(p.Key, "err.") || !strings.HasSuffix(p.Key, ".fix") {
			continue
		}
		for lang, s := range map[string]string{"en": p.EN, "es": p.ES} {
			first, _ := utf8.DecodeRuneInString(s)
			if !strings.HasPrefix(s, "%[") && !unicode.IsUpper(first) {
				t.Errorf("[%s] %s: should start with a capital: %q", lang, p.Key, s)
			}
			if !strings.HasSuffix(s, ".") && !strings.HasPrefix(s, "%[") && !argSuffixOK[p.Key] && !strings.HasSuffix(s, "]d") {
				t.Errorf("[%s] %s: should end with a full stop: %q", lang, p.Key, s)
			}
		}
	}
}
