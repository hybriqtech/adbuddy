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

const again = "Run `adbuddy setup` again to retry."

// chooseAPK reads the --apk file once (at most MaxAPK: it could be a FIFO)
// and returns a private copy to install, so nobody swaps it between the
// check and adb install. When the release certificate does not sign it, it
// also returns its SHA-256: that exact APK (not its key: the debug key
// lives on this PC) is trusted as the development build once it installs
// (spec M6a §6.1).
func chooseAPK(e *env, apk string) (copyPath, devDigest string, err error) {
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
	warn := func(format string, args ...any) {
		if ctx.Err() == nil {
			say(format, args...)
		}
	}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if err != nil {
		res.note(err.Error())
		warn("⚠ Could not check the ADBuddy app (%v): setup goes on without it; 🔴 confirmations will happen in the terminal. "+again+"\n", err)
		return res
	}
	if version == "" || apk != "" {
		if apk == "" {
			base, err := releaseURL(app)
			if err != nil {
				res.note(err.Error())
				warn("⚠ %v. Setup goes on without the app; 🔴 confirmations will happen in the terminal.\n", err)
				return res
			}
			if releaseCert != "" {
				say("Getting the ADBuddy app…\n")
			}
			apk, err = phoneapp.Download(ctx, nil, base, filepath.Join(e.dirs.Data, "app"), releaseCert)
			if err != nil {
				res.note(err.Error())
				switch {
				case errors.Is(err, phoneapp.ErrUnpinned):
					warn("This build of adbuddy has no pinned signing certificate for the app, so it does not download it: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrNoRelease):
					warn("There is no published version of the ADBuddy app yet: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrChecksum):
					warn("⚠ The downloaded ADBuddy app does not match its checksum, so nothing was installed. Try again later, or pass --apk <file>. 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrSigner):
					warn("⚠ The downloaded ADBuddy app is not signed by hybriqtech, so nothing was installed. Please report it at https://github.com/hybriqtech/adbuddy/issues, or pass --apk <file>. 🔴 confirmations will happen in the terminal.\n")
				default:
					warn("⚠ Could not get the ADBuddy app (%v): setup goes on without it; 🔴 confirmations will happen in the terminal. Try again later, or pass --apk <file>.\n", err)
				}
				return res
			}
		} else if _, err := os.Stat(apk); err != nil {
			res.note(err.Error())
			warn("⚠ %v: skipping the app. Check the path of --apk, then "+again+"\n", err)
			return res
		} else {
			chosen, digest, err := chooseAPK(e, apk)
			if err != nil {
				res.note(err.Error())
				warn("⚠ %v: skipping the app. Pass a signed APK to --apk, then "+again+"\n", err)
				return res
			}
			devFile, devDigest, apk = apk, digest, chosen
		}
		say("Installing the ADBuddy app on the phone…\n")
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.note(err.Error())
			if errors.Is(err, phoneapp.ErrOtherSigner) {
				warn("⚠ The ADBuddy app on the phone was signed with another key (a development build?), so Android will not update it. Uninstall it on the phone (Settings › Apps › ADBuddy › Uninstall; it forgets its pairing with this PC) and run `adbuddy setup` again. Setup goes on without it.\n")
			} else {
				warn("⚠ Could not install the app: %v. Setup goes on without it; try again later or with --apk <file>.\n", err)
			}
			return res
		}
		if devDigest != "" {
			path, err := phoneapp.TrustDevAPK(e.dirs.Data, devDigest)
			if err != nil {
				res.note(err.Error())
				warn("⚠ Could not trust the development build (%v): setup goes on without the app.\n", err)
				return res
			}
			warn("⚠ Trusting this development build of the app (SHA-256 %s of %s): adbuddy serve will hand the phone app's token to exactly this APK. Delete %s to stop.\n", devDigest, devFile, path)
		}
		if version, err = phoneapp.InstalledVersion(ctx, a); err != nil || version == "" {
			res.note("the app does not appear as installed")
			warn("⚠ The app does not appear as installed on the phone. Setup goes on without it. " + again + "\n")
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
			warn("⚠ The ADBuddy app on the phone is too old for this host: run `adbuddy setup` again to update it. 🔴 confirmations will happen in the terminal.\n")
		case errors.Is(err, phoneapp.ErrUnpinned):
			warn("This build of adbuddy trusts no signing certificate for the app yet, so it skips it: pass --apk <file> to install and trust a development build. 🔴 confirmations will happen in the terminal.\n")
		case errors.Is(err, phoneapp.ErrSigner):
			warn("⚠ The ADBuddy app on the phone is not signed by hybriqtech (a development build?), so ADBuddy will not trust it. Uninstall it on the phone and run `adbuddy setup` again, or pass --apk <that APK> to trust a development build. 🔴 confirmations will happen in the terminal.\n")
		default:
			warn("⚠ Could not check the ADBuddy app on the phone (%v), so ADBuddy will not trust it. "+again+" 🔴 confirmations will happen in the terminal.\n", err)
		}
		return res
	}
	if v.Dev() {
		say("Note: the ADBuddy app on the phone is the development build you trusted (certificate %s).\n", v.Cert())
	} else if removed, err := forgetDevBuild(e); err != nil {
		warn("⚠ Could not stop trusting the development build (%v): delete %s by hand.\n", err, phoneapp.DevTrustPath(e.dirs.Data))
	} else if removed {
		say("The app on the phone is a release: ADBuddy no longer trusts the development build.\n")
	}
	store := newCfgStore(e.dirs.ConfigFile(), e.cfg)
	_, err = chatPort(store)
	e.cfg.ChatPort = store.Config().ChatPort // setup saves e.cfg later
	if err != nil {
		res.note(err.Error())
		warn("⚠ %v. Setup goes on; the app's chat needs it. "+again+"\n", err)
	}

	host, _ := os.Hostname()
	say("Open the phone: the ADBuddy app explains what this PC will do. Accept there to continue (waiting up to %s).\n", waitText(consentWait))
	switch err := phoneapp.AskConsent(ctx, a, v, phoneapp.HostName(host), consentWait, time.Second); {
	case errors.Is(err, phoneapp.ErrConsentDenied):
		res.Consent = "denied"
		say("You said no in the app: ADBuddy works without it, and 🔴 confirmations happen in the terminal. Run `adbuddy setup` again to change it.\n")
		return res
	case errors.Is(err, phoneapp.ErrConsentTimeout):
		res.Consent = "timeout"
		say("No answer in the app: skipping it for now. Run `adbuddy setup` again when you have the phone at hand.\n")
		return res
	case err != nil:
		res.Consent = "skipped"
		if ctx.Err() == nil {
			res.note(err.Error())
			say("⚠ Could not ask in the app: %v. Setup goes on without it; 🔴 confirmations will happen in the terminal. "+again+"\n", err)
		}
		return res
	}
	res.Consent = "granted"
	if err := phoneapp.Grant(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("⚠ Could not give the app the permission to cut access: %v. "+again+"\n", err)
	} else {
		res.Permission = true
	}
	if err := phoneapp.AllowBackground(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("⚠ Could not exempt the app from battery optimisation (it may disconnect with the screen off): %v. "+again+"\n", err)
	} else {
		res.Battery = true
	}
	if prev, err := phoneapp.EnableAccessibility(ctx, a, v); err != nil {
		res.note(err.Error())
		warn("⚠ Could not turn on the app's accessibility service: %v. Turn it on by hand: Settings > Accessibility > ADBuddy, or run `adbuddy setup` again to retry.\n", err)
	} else {
		res.Accessibility = true
		if prev != "" && prev != "null" {
			say("Accessibility services already on were kept: %s\n", prev)
		}
	}
	if res.Permission && res.Battery && res.Accessibility {
		say("The ADBuddy app is ready (version %s).\n", version)
	} else {
		warn("The ADBuddy app is only partly ready (version %s); see the warnings above.\n", version)
	}
	return res
}

// waitText says a wait in minutes (or as a duration when it is short).
func waitText(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		n := int(d / time.Minute)
		if n == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", n)
	}
	return d.Round(time.Millisecond).String()
}

// fmtApp is the human summary of the app.
func fmtApp(r appResult) string {
	switch {
	case !r.Installed && r.Note == noAppNote:
		return "app: skipped"
	case !r.Installed:
		return "app: not installed"
	case r.Consent != "granted":
		return fmt.Sprintf("app %s (%s)", r.Version, r.Consent)
	}
	var off []string
	if !r.Permission {
		off = append(off, "permission off")
	}
	if !r.Battery {
		off = append(off, "battery off")
	}
	if !r.Accessibility {
		off = append(off, "accessibility off")
	}
	if len(off) == 0 {
		return fmt.Sprintf("app %s (granted)", r.Version)
	}
	return fmt.Sprintf("app %s (granted, %s)", r.Version, strings.Join(off, ", "))
}
