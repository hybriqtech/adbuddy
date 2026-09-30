package classify

import "testing"

func TestTheAppChannelIsOffLimits(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"content call --uri content://tech.hybriq.adbuddy.bridge --method handshake --extra token:s:x",
		"content call --uri content://tech.hybriq.adbuddy.bridge --method getSetupConsent",
		"content write --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content read --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content query --uri content://tech.hybriq.adbuddy.bridge",
		"content query --uri content://tech.hybriq.adbuddy%2ebridge",
		"content query --uri content://TECH.HYBRIQ.ADBUDDY.bridge",
		"run-as tech.hybriq.adbuddy cat shared_prefs/bridge.xml",
		"run-as tech.hybriq.adbuddy ls",
		"am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP",
		"am force-stop tech.hybriq.adbuddy",
		"pm revoke tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS",
		"pm disable-user tech.hybriq.adbuddy",
		"appops set tech.hybriq.adbuddy RUN_IN_BACKGROUND ignore",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	for _, line := range []string{
		"pm list packages tech.hybriq.adbuddy",
		"dumpsys package tech.hybriq.adbuddy",
		"content query --uri content://settings/global",
	} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", line, r.Level, r.Primary.Reason)
		}
	}
}
