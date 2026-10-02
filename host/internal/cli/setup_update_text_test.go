package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// V36: plain `adbuddy setup` does not replace an installed app, so no text may
// send the person to it to update; `--apk <file>` is what does.
func TestNoTextPromisesThatSetupUpdatesTheApp(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		for _, key := range []string{"phone.the_adbuddy_app_on_the", "appsetup.the_adbuddy_app_on_the_2"} {
			s := lang.T(key, "1")
			for _, bad := range []string{"to update it", "setup` again", "para actualizarla", "de nuevo"} {
				if strings.Contains(s, bad) {
					t.Errorf("%s %s: %q still says %q", lang, key, s, bad)
				}
			}
			if !strings.Contains(s, "--apk") {
				t.Errorf("%s %s: %q does not mention --apk", lang, key, s)
			}
		}
	}
}
