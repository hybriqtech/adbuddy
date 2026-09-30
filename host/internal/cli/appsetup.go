package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Accessibility bool   `json:"accessibility"`
	Note          string `json:"note,omitempty"`
}

func releaseURL(app *App) string {
	if u := app.Getenv("ADBUDDY_RELEASE_URL"); u != "" {
		return u
	}
	return phoneapp.ReleaseURL
}

// setupApp runs steps 5-6 of spec §3.1: install the app (from apk, or the
// latest release) unless it is there, ask for consent in the app, then grant
// WRITE_SECURE_SETTINGS and enable its accessibility service. The app is
// optional for the host: every problem here is reported and setup goes on
// (🔴 confirmations then happen in the terminal).
func setupApp(ctx context.Context, app *App, e *env, a phoneapp.ADB, apk string, say func(string, ...any)) appResult {
	res := appResult{Consent: "skipped"}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if err != nil {
		res.Note = err.Error()
		say("⚠ Could not check the ADBuddy app: %v\n", err)
		return res
	}
	if version == "" || apk != "" {
		if apk == "" {
			say("Getting the ADBuddy app…\n")
			apk, err = phoneapp.Download(ctx, nil, releaseURL(app), filepath.Join(e.dirs.Data, "app"))
			if err != nil {
				res.Note = err.Error()
				if errors.Is(err, phoneapp.ErrNoRelease) {
					say("There is no published version of the ADBuddy app yet: skipping it (pass --apk <file> to install one).\n")
				} else {
					say("⚠ Could not get the ADBuddy app (%v): skipping it; 🔴 confirmations will happen in the terminal.\n", err)
				}
				return res
			}
		} else if _, err := os.Stat(apk); err != nil {
			res.Note = err.Error()
			say("⚠ %v: skipping the app.\n", err)
			return res
		}
		say("Installing the ADBuddy app on the phone…\n")
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.Note = err.Error()
			say("⚠ Could not install the app: %v\n", err)
			return res
		}
		if version, err = phoneapp.InstalledVersion(ctx, a); err != nil || version == "" {
			res.Note = "the app does not appear as installed"
			say("⚠ The app does not appear as installed.\n")
			return res
		}
	}
	res.Installed, res.Version = true, version
	if _, err := chatPort(e); err != nil {
		say("⚠ %v\n", err)
	}

	host, _ := os.Hostname()
	say("Open the phone: the ADBuddy app explains what this PC will do. Accept there to continue.\n")
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
		res.Consent, res.Note = "skipped", err.Error()
		say("⚠ Could not ask in the app: %v\n", err)
		return res
	}
	res.Consent = "granted"
	if err := phoneapp.Grant(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not give the app the permission to cut access: %v\n", err)
	}
	if err := phoneapp.AllowBackground(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not exempt the app from battery optimisation (it may disconnect with the screen off): %v\n", err)
	}
	if prev, err := phoneapp.EnableAccessibility(ctx, a); err != nil {
		res.Note = err.Error()
		say("⚠ Could not turn on the app's accessibility service: %v\n", err)
	} else {
		res.Accessibility = true
		if prev != "" && prev != "null" {
			say("Accessibility services already on were kept: %s\n", prev)
		}
	}
	say("The ADBuddy app is ready (version %s).\n", version)
	return res
}

// fmtApp is the human summary line of the app.
func fmtApp(r appResult) string {
	if !r.Installed {
		return "app: not installed"
	}
	return fmt.Sprintf("app %s (%s)", r.Version, r.Consent)
}
