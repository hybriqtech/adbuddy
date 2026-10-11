package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
)

func TestConfirmersNote(t *testing.T) {
	cases := []struct {
		mode        string
		interactive bool
		key         string
	}{
		{"auto", true, "serve.confirmations_auto_terminal"},
		{"auto", false, "serve.confirmations_auto_no_terminal"},
		{"terminal", true, "serve.confirmations_terminal_terminal"},
		{"terminal", false, "serve.confirmations_terminal_pipe"},
		{"deny", true, "serve.confirmations_deny"},
		{"deny", false, "serve.confirmations_deny"},
	}
	for _, c := range cases {
		if got := confirmersNote(c.mode, c.interactive).Key; got != c.key {
			t.Errorf("confirmersNote(%q, %v) = %s, want %s", c.mode, c.interactive, got, c.key)
		}
		// Whatever --confirm says, with the app connected the question goes
		// to the app: no text may claim a denial without saying so.
		en := confirmersNote(c.mode, c.interactive).In(i18n.EN)
		es := confirmersNote(c.mode, c.interactive).In(i18n.ES)
		if !strings.Contains(en, "while it is connected") || !strings.Contains(es, "mientras esté conectada") {
			t.Errorf("%s does not mention the app: %q / %q", c.key, en, es)
		}
	}
}

func TestPhoneWarningsDoNotSayHappenHere(t *testing.T) {
	keys := []string{"phone.confirmations_happen_here_without_the", "phone.could_not_check_whether_the",
		"phone.the_adbuddy_app_is_not", "phone.the_phone_app_disconnected_confirmations", "phone.could_not_reach_the_phone"}
	for _, l := range []i18n.Lang{i18n.EN, i18n.ES} {
		for _, k := range keys {
			s := l.T(k, "x")
			if strings.Contains(s, "happen here") || strings.Contains(s, "se hacen aquí") {
				t.Errorf("%s %s still says where: %q", l, k, s)
			}
		}
	}
}

func TestServeSaysWhereConfirmationsGo(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.EN, i18n.ES} {
		t.Run(string(l), func(t *testing.T) {
			shortWaits(t)
			i18ntest.Lang(t, l)
			cfg := config.Default()
			cfg.Serial, cfg.Address = phoneSerial, tsAddr
			// The first three words of the serving line, in the language under test.
			serving := strings.SplitN(l.T("serve.adbuddy_is_serving_session_journal", "", "", ""), " ", 4)
			marker := serving[0] + " " + serving[1] + " " + serving[2]
			code, errOut, _, _ := serveOnceMarker(t, remoteRules("AAAA1111"), cfg, marker)
			if code != 0 {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			want := confirmersNote("deny", false).In(l)
			i := strings.Index(errOut, want)
			j := strings.Index(errOut, marker)
			if i < 0 || j < 0 || i > j {
				t.Fatalf("note at %d, serving line at %d; stderr %q", i, j, errOut)
			}
		})
	}
}
