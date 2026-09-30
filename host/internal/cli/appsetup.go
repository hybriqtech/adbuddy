package cli

import (
	"context"
	"errors"
	"fmt"
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

func releaseURL(app *App) string {
	u := app.Getenv("ADBUDDY_RELEASE_URL")
	if u == "" {
		return phoneapp.ReleaseURL
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u
}

const again = "Run `adbuddy setup` again to retry."

// setupApp runs steps 5-6 of spec §3.1: install the app (from apk, or the
// latest release) unless it is there, ask for consent in the app, then grant
// WRITE_SECURE_SETTINGS, exempt it from battery optimisation and enable its
// accessibility service. The app is optional for the host: every problem here
// is reported and setup goes on (🔴 confirmations then happen in the
// terminal). On Ctrl+C it returns quietly and the caller stops.
func setupApp(ctx context.Context, app *App, e *env, a phoneapp.ADB, apk string, say func(string, ...any)) appResult {
	res := appResult{Consent: "skipped"}
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
			say("Getting the ADBuddy app…\n")
			apk, err = phoneapp.Download(ctx, nil, releaseURL(app), filepath.Join(e.dirs.Data, "app"))
			if err != nil {
				res.note(err.Error())
				switch {
				case errors.Is(err, phoneapp.ErrNoRelease):
					warn("There is no published version of the ADBuddy app yet: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrChecksum):
					warn("⚠ The downloaded ADBuddy app does not match its checksum, so nothing was installed. Try again later, or pass --apk <file>. 🔴 confirmations will happen in the terminal.\n")
				default:
					warn("⚠ Could not get the ADBuddy app (%v): setup goes on without it; 🔴 confirmations will happen in the terminal. Try again later, or pass --apk <file>.\n", err)
				}
				return res
			}
		} else if _, err := os.Stat(apk); err != nil {
			res.note(err.Error())
			warn("⚠ %v: skipping the app. Check the path of --apk, then "+again+"\n", err)
			return res
		}
		say("Installing the ADBuddy app on the phone…\n")
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.note(err.Error())
			warn("⚠ Could not install the app: %v. Setup goes on without it; try again later or with --apk <file>.\n", err)
			return res
		}
		if version, err = phoneapp.InstalledVersion(ctx, a); err != nil || version == "" {
			res.note("the app does not appear as installed")
			warn("⚠ The app does not appear as installed on the phone. Setup goes on without it. " + again + "\n")
			return res
		}
	}
	res.Installed, res.Version = true, version
	if _, err := chatPort(e); err != nil {
		res.note(err.Error())
		warn("⚠ %v. Setup goes on; the app's chat needs it. "+again+"\n", err)
	}

	host, _ := os.Hostname()
	say("Open the phone: the ADBuddy app explains what this PC will do. Accept there to continue (waiting up to %s).\n", waitText(consentWait))
	switch err := phoneapp.AskConsent(ctx, a, phoneapp.HostName(host), consentWait, time.Second); {
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
	if err := phoneapp.Grant(ctx, a); err != nil {
		res.note(err.Error())
		warn("⚠ Could not give the app the permission to cut access: %v. "+again+"\n", err)
	} else {
		res.Permission = true
	}
	if err := phoneapp.AllowBackground(ctx, a); err != nil {
		res.note(err.Error())
		warn("⚠ Could not exempt the app from battery optimisation (it may disconnect with the screen off): %v. "+again+"\n", err)
	} else {
		res.Battery = true
	}
	if prev, err := phoneapp.EnableAccessibility(ctx, a); err != nil {
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
