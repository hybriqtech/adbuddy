package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// consentWait is how long setup waits for the answer in the app.
var consentWait = 5 * time.Minute

// appResult is the app part of `adbuddy setup --json`.
type appResult struct {
	Version       string `json:"version,omitempty"`
	Installed     bool   `json:"installed"`
	Consent       string `json:"consent"` // granted, denied, timeout or skipped
	Permission    bool   `json:"permission"`
	Battery       bool   `json:"battery"`
	Accessibility bool   `json:"accessibility"`
	Note          string `json:"note,omitempty"` // every problem, joined with "; "
}

// noAppNote is the Note of --no-app.
const noAppNote = "--no-app"

func (r *appResult) note(s string) {
	if r.Note != "" {
		r.Note += "; "
	}
	r.Note += s
}

// releaseCert is the signing certificate a downloaded APK must have, and
// the release one the installed app may have (a variable so that tests can
// pin theirs).
var releaseCert = phoneapp.SigningCertSHA256

// releaseURL is where setup downloads the app: ReleaseURL, or
// ADBUDDY_RELEASE_URL for the e2e tests, which must be https:// or a
// loopback http://. Even another origin can only serve APKs signed with
// releaseCert and not older than MinAppVersionCode (spec M6a §6.4).
func releaseURL(app *App) (string, error) {
	u := app.Getenv("ADBUDDY_RELEASE_URL")
	if u == "" {
		return phoneapp.ReleaseURL, nil
	}
	p, err := url.Parse(u)
	loopback := err == nil && (p.Hostname() == "127.0.0.1" || p.Hostname() == "::1" || p.Hostname() == "localhost")
	if err != nil || !(p.Scheme == "https" && p.Host != "" || p.Scheme == "http" && loopback) {
		return "", fmt.Errorf("ADBUDDY_RELEASE_URL must be https:// or http:// to 127.0.0.1, ::1 or localhost, not %q", u)
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u, nil
}

// chooseAPK reads the --apk file once (at most MaxAPK: it could be a FIFO)
// and returns a private copy to install, so nobody swaps it between the
// check and adb install. When the release certificate does not sign it, it
// also returns its SHA-256: that exact APK (not its key: the debug key
// lives on this PC) is trusted as the development build once it installs
// (spec M6a §6.1).
func chooseAPK(e *env, apk string) (copyPath, devDigest string, err error) {
	// Stat follows links, so a link to an APK is fine; a FIFO, a device or a
	// directory is not, and is never opened (a FIFO with no writer would
	// block os.Open for ever).
	info, err := os.Stat(apk)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("%s is not a regular file", apk)
	}
	f, err := os.Open(apk)
	if err != nil {
		return "", "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, phoneapp.MaxAPK+1))
	f.Close()
	if err != nil {
		return "", "", err
	}
	if len(data) > phoneapp.MaxAPK {
		return "", "", fmt.Errorf("%s is larger than %d MiB", apk, phoneapp.MaxAPK>>20)
	}
	if _, err := phoneapp.Signers(data); err != nil {
		return "", "", fmt.Errorf("%s is not a signed APK (%v)", apk, err)
	}
	dir := filepath.Join(e.dirs.Data, "app")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	copyPath = filepath.Join(dir, "chosen.apk")
	if err := phoneapp.WritePrivate(copyPath, data); err != nil {
		return "", "", err
	}
	if releaseCert != "" && phoneapp.CheckSigner(data, releaseCert) == nil {
		return copyPath, "", nil
	}
	sum := sha256.Sum256(data)
	return copyPath, hex.EncodeToString(sum[:]), nil
}

// forgetDevBuild stops trusting the development build and removes its copy.
func forgetDevBuild(e *env) (bool, error) {
	os.Remove(filepath.Join(e.dirs.Data, "app", "chosen.apk"))
	return phoneapp.ForgetDevAPK(e.dirs.Data)
}

// setupApp runs steps 5-6 of spec §3.1: install the app (from apk, or the
// latest release) unless it is there, ask for consent in the app, then grant
// WRITE_SECURE_SETTINGS, exempt it from battery optimisation and enable its
// accessibility service. The app is optional for the host: every problem here
// is reported and setup goes on (🔴 confirmations then happen in the
// terminal). On Ctrl+C it returns quietly and the caller stops.
func setupApp(ctx context.Context, app *App, e *env, a phoneapp.ADB, apk string, say func(string, ...any)) appResult {
	res := appResult{Consent: "skipped"}
	devDigest, devFile := "", "" // the --apk that becomes the trusted development build, and its path
	// warn reports a problem, unless it is just Ctrl+C.
	warn := func(key string, args ...any) {
		if ctx.Err() == nil {
			say(key, args...)
		}
	}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if err != nil {
		res.note(err.Error())
		warn("appsetup.could_not_check_the_adbuddy", err)
		return res
	}
	if version == "" || apk != "" {
		if apk == "" {
			base, err := releaseURL(app)
			if err != nil {
				res.note(err.Error())
				warn("appsetup.setup_goes_on_without_the", err)
				return res
			}
			if releaseCert != "" {
				say("appsetup.getting_the_adbuddy_app")
			}
			apk, err = phoneapp.Download(ctx, nil, base, filepath.Join(e.dirs.Data, "app"), releaseCert)
			if err != nil {
				res.note(err.Error())
				switch {
				case errors.Is(err, phoneapp.ErrUnpinned):
					warn("appsetup.this_build_of_adbuddy_has")
				case errors.Is(err, phoneapp.ErrNoRelease):
					warn("appsetup.there_is_no_published_version")
				case errors.Is(err, phoneapp.ErrChecksum):
					warn("appsetup.the_downloaded_adbuddy_app_does")
				case errors.Is(err, phoneapp.ErrSigner):
					warn("appsetup.the_downloaded_adbuddy_app_is")
				default:
					warn("appsetup.could_not_get_the_adbuddy", err)
				}
				return res
			}
		} else if _, err := os.Stat(apk); err != nil {
			res.note(err.Error())
			warn("appsetup.skipping_the_app_check_the", err)
			return res
		} else {
			chosen, digest, err := chooseAPK(e, apk)
			if err != nil {
				res.note(err.Error())
				warn("appsetup.skipping_the_app_pass_a", err)
				return res
			}
			devFile, devDigest, apk = apk, digest, chosen
		}
		say("appsetup.installing_the_adbuddy_app_on")
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.note(err.Error())
			if errors.Is(err, phoneapp.ErrOtherSigner) {
				warn("appsetup.the_adbuddy_app_on_the")
			} else {
				warn("appsetup.could_not_install_the_app", err)
			}
			return res
		}
		if devDigest != "" {
			path, err := phoneapp.TrustDevAPK(e.dirs.Data, devDigest)
			if err != nil {
				res.note(err.Error())
				warn("appsetup.could_not_trust_the_development", err)
				return res
			}
			warn("appsetup.trusting_this_development_build_of", devDigest, devFile, path)
		}
		if version, err = phoneapp.InstalledVersion(ctx, a); err != nil || version == "" {
			res.note("the app does not appear as installed")
			warn("appsetup.the_app_does_not_appear")
			return res
		}
	}
	res.Installed, res.Version = true, version
	load := func() phoneapp.Trust { return phoneapp.LoadTrust(releaseCert, e.dirs.Data) }
	v, err := phoneapp.NewVerifier(load, phoneapp.MinAppVersionCode).Verify(ctx, a)
	if err != nil {
		res.note(err.Error())
		switch {
		case errors.Is(err, phoneapp.ErrAppTooOld):
			warn("appsetup.the_adbuddy_app_on_the_2")
		case errors.Is(err, phoneapp.ErrUnpinned):
			warn("appsetup.this_build_of_adbuddy_trusts")
		case errors.Is(err, phoneapp.ErrSigner):
			warn("appsetup.the_adbuddy_app_on_the_3")
		default:
			warn("appsetup.could_not_check_the_adbuddy_2", err)
		}
		return res
	}
	if v.Dev() {
		say("appsetup.note_the_adbuddy_app_on", v.Cert())
	} else if removed, err := forgetDevBuild(e); err != nil {
		warn("appsetup.could_not_stop_trusting_the", err, phoneapp.DevTrustPath(e.dirs.Data))
	} else if removed {
		say("appsetup.the_app_on_the_phone")
	}
	store := newCfgStore(e.dirs.ConfigFile(), e.cfg)
	_, err = chatPort(store)
	e.cfg.ChatPort = store.Config().ChatPort // setup saves e.cfg later
	if err != nil {
		res.note(err.Error())
		warn("appsetup.setup_goes_on_the_app", err)
	}

	host, _ := os.Hostname()
	say("appsetup.open_the_phone_the_adbuddy", waitText(consentWait))
	switch err := phoneapp.AskConsent(ctx, a, v, phoneapp.HostName(host), consentWait, time.Second); {
	case errors.Is(err, phoneapp.ErrConsentDenied):
		res.Consent = "denied"
		say("appsetup.you_said_no_in_the")
		return res
	case errors.Is(err, phoneapp.ErrConsentTimeout):
		res.Consent = "timeout"
		say("appsetup.no_answer_in_the_app")
		return res
	case err != nil:
		res.Consent = "skipped"
		if ctx.Err() == nil {
			res.note(err.Error())
			say("appsetup.could_not_ask_in_the", err)
		}
		return res
	}
	res.Consent = "granted"
	if err := phoneapp.Grant(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("appsetup.could_not_give_the_app", err)
	} else {
		res.Permission = true
	}
	if err := phoneapp.AllowBackground(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("appsetup.could_not_exempt_the_app", err)
	} else {
		res.Battery = true
	}
	if prev, err := phoneapp.EnableAccessibility(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("appsetup.could_not_turn_on_the", err)
	} else {
		res.Accessibility = true
		if prev != "" && prev != "null" {
			say("appsetup.accessibility_services_already_on_were", prev)
		}
	}
	if res.Permission && res.Battery && res.Accessibility {
		say("appsetup.the_adbuddy_app_is_ready", version)
	} else {
		warn("appsetup.the_adbuddy_app_is_only", version)
	}
	return res
}

// waitText says a wait in minutes (or as a duration when it is short), as a
// message of its own: it is a piece of a sentence in the language of the run.
func waitText(d time.Duration) i18n.Msg {
	if d >= time.Minute && d%time.Minute == 0 {
		n := int(d / time.Minute)
		if n == 1 {
			return i18n.M("appsetup.wait.one_minute")
		}
		return i18n.M("appsetup.wait.minutes", n)
	}
	return i18n.M("appsetup.wait.duration", d.Round(time.Millisecond).String())
}

// consentWord is the state of the consent as a person reads it; the value of
// the JSON (appResult.Consent) stays as it is.
func consentWord(c string) any {
	switch c {
	case "denied":
		return i18n.M("appsetup.consent.denied")
	case "timeout":
		return i18n.M("appsetup.consent.timeout")
	case "skipped":
		return i18n.M("appsetup.consent.skipped")
	}
	return c
}

// fmtAppMsg is the human summary of the app as a message of its own.
func fmtAppMsg(r appResult) i18n.Msg {
	switch {
	case !r.Installed && r.Note == noAppNote:
		return i18n.M("appsetup.summary.skipped")
	case !r.Installed:
		return i18n.M("appsetup.summary.not_installed")
	case r.Consent != "granted":
		return i18n.M("appsetup.summary.consent", r.Version, consentWord(r.Consent))
	}
	var off []any
	if !r.Permission {
		off = append(off, i18n.M("appsetup.summary.permission_off"))
	}
	if !r.Battery {
		off = append(off, i18n.M("appsetup.summary.battery_off"))
	}
	if !r.Accessibility {
		off = append(off, i18n.M("appsetup.summary.accessibility_off"))
	}
	switch len(off) {
	case 0:
		return i18n.M("appsetup.summary.granted", r.Version)
	case 1:
		return i18n.M("appsetup.summary.granted_off_1", r.Version, off[0])
	case 2:
		return i18n.M("appsetup.summary.granted_off_2", r.Version, off[0], off[1])
	}
	return i18n.M("appsetup.summary.granted_off_3", r.Version, off[0], off[1], off[2])
}
