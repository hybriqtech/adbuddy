# Releasing ADBuddy

A release is a git tag. `.github/workflows/release.yml` builds a **draft** release: the host binaries (goreleaser) and the app (`adbuddy.apk` and `adbuddy.apk.sha256`). The app is built without secrets, signed in an isolated job of the `release` environment and checked against the certificate the host pins. Nothing is public until the maintainer approves the signing job and publishes the draft. Design: `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md`.

## User actions (once)

Only the repository owner can do these, in their own terminal and on github.com. **Never run them through a coding agent:** the keystore must not end up in a transcript. Nothing else waits for them, but there is no app release until they are done.

1. **Create the signing key**, outside the repo. It needs `keytool`: install a JDK (`winget install EclipseAdoptium.Temurin.21.JDK`) or use Android Studio's `jbr/bin/keytool`.

   ```sh
   sh scripts/release-key.sh
   ```

   It writes `~/.adbuddy-release/release.p12` (private folder), lets `keytool` ask for the password and prints the certificate's SHA-256 and the `gh secret set` commands. It never prints the keystore.
2. **Back up** `release.p12` and its password (a password manager, an encrypted drive). If they are lost, no future release can update the app already on phones: Android only updates an app signed with the same key.
3. **Create the `release` environment** (Settings › Environments › New environment):
   - Deployment branches and tags: *Selected branches and tags*, add the tag rule `v*`.
   - Required reviewers: yourself.
   - Turn off *Allow administrators to bypass configured protection rules*.
4. **Add its two secrets** with the commands the script printed (`--env release`): `ADBUDDY_KEYSTORE_B64` and `ADBUDDY_KEYSTORE_PASSWORD`. Run them in **Git Bash**, not PowerShell: they use `|` and `<` the POSIX way. They are environment secrets, not repository secrets, so a workflow on another branch cannot read them. The key alias is always `adbuddy`.
5. **Rulesets** (Settings › Rules › Rulesets):
   - a tag ruleset on `v*`: restrict creations, updates and deletions; only repository admins bypass;
   - a branch ruleset on `main`: no direct pushes and no force pushes, changes only through pull requests, and **no bypass list** (not even GitHub Actions). The release jobs that may write (`host` and `publish-apk`) then cannot touch `main`.

   With the `main` ruleset on, branches no longer reach `main` with a local fast-forward and `git push`: open a pull request and merge it with **Rebase and merge**. That keeps the history linear, as today.
6. **Pin the certificate:** put the SHA-256 the script printed (lowercase hex, no colons) in `SigningCertSHA256` in `host/internal/phoneapp/release.go`, in its own commit and PR. Until then `adbuddy setup` downloads no app (it says so), and `publish-apk` fails.
7. **Your phone** has a development build signed with another key. Before the first release, uninstall ADBuddy on the phone; it forgets its pairing, and `adbuddy setup` pairs again. Until then, run `adbuddy setup --apk <your build>` once (and again after each new build) so the host trusts that exact APK. `adbuddy serve` says so every time it starts.

## Cutting a release

0. **Before the first public release only:** the emulator checks of validation 3 in `docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md` have passed on API 30, 33 and the latest. Do not tag v0.x until they have.
1. `main` is green and holds what you want to ship.
2. If this release fixes a security bug in the app, raise `MinAppVersionCode` in `host/internal/phoneapp/trust.go` to this release's versionCode (`sh scripts/release-version.sh vX.Y.Z`), in the same release.
3. Tag a commit of `main` and push the tag: `git tag v0.3.0 && git push origin v0.3.0`. For a prerelease, use `v0.3.0-rc.1`. Tags must be `vX.Y.Z` or `vX.Y.Z-rc.N`; `scripts/release-version.sh` explains the limits. A tag on a commit that is not on `main` releases nothing.
4. The `release` workflow runs `version`, then `test`, then `host` and `build-apk`, then `sign` (waits for your approval), then `publish-apk`. Approve `sign` when it asks.
5. Open the draft on GitHub and check its assets: 5 archives, `checksums.txt`, `adbuddy.apk` and `adbuddy.apk.sha256`. Check the notes, and press **Publish release**. Prereleases are never `latest`, so `adbuddy setup` and `install.sh` skip them.

If a job fails, delete the draft and the tag (`git push origin :v0.3.0`), fix the problem and tag again.

## What protects the app

- `adbuddy setup` checks a downloaded APK against `adbuddy.apk.sha256`. Then it checks that **every** signer in its v2, v3 and v3.1 blocks has the pinned certificate, and that Android would find the same blocks.
- Before it hands the token to the app, and before it asks for consent or grants a permission, the host reads the **installed** app back from the phone. It checks the same things there, and that its version is not below `MinAppVersionCode`. An app with our name signed by anyone else gets nothing, wherever it came from.
- The installed app must be in `/data/app` (not on adopted storage) and the phone must not be rooted: with root the phone's shell could lie.
- Every app that connects with the token must still be that installation (same path and `stat` of `base.apk`). An update in place keeps the app's data, the token included, so it is refused and the app is checked again before a new token goes out.
- What ADBuddy does not protect against: an agent that can run tools on the PC outside `adbuddy` (it could edit the host's files or run the real adb). The agent environment only pre-approves `adbuddy` commands, and `adbuddy setup` refuses to run from the agent.
- `ADBUDDY_RELEASE_URL` (the e2e tests) only takes `https://` or a loopback `http://`. Another origin could only serve old or current APKs that hybriqtech signed, and `MinAppVersionCode` stops the old ones.
- `adbuddy setup --apk <file>` installs a private copy of a file you choose. If the release key does not sign it, **that exact APK** (its SHA-256) becomes the trusted development build: not its key, because the debug key lives on your PC. `adbuddy serve` says so every time it starts, and a setup that finds a release on the phone stops trusting it.
- Key rotation is not supported: rotating the key needs a host release that reads the v3 lineage first.

## If the key leaks

1. Delete the two secrets of the `release` environment and the leaked keystore.
2. Create a new key (`sh scripts/release-key.sh` with a new `ADBUDDY_RELEASE_KEY_DIR`) and its secrets.
3. Release a host with the new certificate in `SigningCertSHA256` and `MinAppVersionCode` above every release signed with the old key. From then on, new hosts trust nothing signed with the old key.
4. Every user uninstalls the app on the phone and runs `adbuddy setup` again: Android cannot update an app to another key.
5. Say what happened in the README and in the release notes.

## Later (M6c)

Homebrew tap, winget, IzzyOnDroid, SignPath and signed build provenance need accounts, repositories or a public release first: see the M6 section of `docs/superpowers/plans/pendientes-m3-m5.md`.
