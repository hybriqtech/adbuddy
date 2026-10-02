---
title: Install ADBuddy
layout: default
lang: en
---

# Install ADBuddy

ADBuddy has two parts: the `adbuddy` program on your PC (the **host**) and a small app on your Android phone. This page installs the host. `adbuddy setup` installs the app on the phone over the connection it pairs. Until there is a release, you build the APK and pass it with `--apk`.

**There is no public release yet.** Today the way in is [Build from source](#build-from-source). The release downloads below describe what will work once a release is published; the [Releases page](https://github.com/hybriqtech/adbuddy/releases) is where it will appear.

## Requirements

- **Phone:** Android 11 or newer (the app's `minSdk` is 30), with Wireless debugging. [Set up your phone](setup/) shows how.
- **PC:** Windows, macOS or Linux, on the same Wi-Fi network as the phone (or Tailscale: see [Use ADBuddy](usage.md)).
- **A coding agent** already installed on the PC: Claude Code, Codex, Gemini CLI or OpenCode. ADBuddy does not install agents. If the one you picked is missing, `adbuddy serve` and `adbuddy run` print the command that installs it.
- **adb** is not a requirement: `adbuddy setup` downloads Google's Android platform-tools and checks them against Google's checksum.

## Build from source

This works today, on any of the three systems. You need [Git](https://git-scm.com/) and [Go](https://go.dev/dl/) 1.27 or newer.

1. Get the code and build the host.

   ```sh
   git clone https://github.com/hybriqtech/adbuddy.git
   cd adbuddy/host
   go build -o adbuddy ./cmd/adbuddy
   ```

   On Windows, build `adbuddy.exe` instead: `go build -o adbuddy.exe ./cmd/adbuddy`.

2. Put the program in a folder that is in your `PATH`. On Linux and macOS, `~/.local/bin` is the usual one:

   ```sh
   mkdir -p ~/.local/bin
   cp adbuddy ~/.local/bin/
   ```

   On Windows, copy `adbuddy.exe` to a folder of your own and add it to your `PATH` as in the Windows steps further down. After each `git pull` and new build, copy the program again.

3. Check it. A build from source has no version number, so it says `dev`.

   ```sh
   adbuddy --version
   ```

   ```text
   adbuddy dev
   ```

4. Build the phone app. This needs JDK 17, the Android SDK platform 37 and Gradle 9.8; the repository has no Gradle wrapper yet. Gradle must find the SDK: set `ANDROID_HOME` (as the CI does) or put `sdk.dir=<path to the SDK>` in `android/local.properties`, or it stops with "SDK location not found".

   ```sh
   cd ../android
   gradle :app:assembleDebug
   ```

   The APK is `app/build/outputs/apk/debug/app-debug.apk`. Keep its path for setup's `--apk` option.

Today the source pins no release certificate for the app, so `adbuddy setup` does not download it. You give it the APK yourself (from the `android` folder, or with the full path):

```sh
adbuddy setup --apk app/build/outputs/apk/debug/app-debug.apk
```

Because that APK is not signed with the project's release key, **that exact file** (its SHA-256) becomes the one development build the host trusts. `adbuddy serve` says so every time it starts. See [Safety](safety.md) for what that trust means.

## When there is a public release

### Linux and macOS

These steps apply once a release exists. One command installs the latest release in `~/.local/bin`, without `sudo`:

```sh
curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh
```

The script needs `curl` or `wget`, `tar`, `awk` and `sha256sum` or `shasum`. It picks the archive for your system (Linux on x86-64 or arm64, macOS on Intel or Apple silicon), downloads `checksums.txt` and the archive, checks the archive's SHA-256 and refuses to install on a mismatch. If `~/.local/bin` is not in your `PATH`, it says so.

The checksum file comes from the same release as the archive. It catches a damaged download; the checksum is not a signature.

Change what it does with two variables:

- `ADBUDDY_VERSION` installs another release instead of the latest one. It must be a release tag, `vX.Y.Z` or `vX.Y.Z-rc.N`. Prereleases are never "latest", so a release candidate needs this variable.
- `ADBUDDY_INSTALL_DIR` installs in another folder.

```sh
curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | ADBUDDY_INSTALL_DIR="$HOME/bin" sh
```

**Read the script before you run it** if you prefer. It is short: [`scripts/install.sh` on GitHub](https://github.com/hybriqtech/adbuddy/blob/main/scripts/install.sh). Download it, read it, then run it:

```sh
curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh -o install.sh
less install.sh
sh install.sh
```

### Windows

1. Open the [Releases page](https://github.com/hybriqtech/adbuddy/releases) and download two files: the archive for your PC and `checksums.txt`. Their names are:

   - `adbuddy_<version>_windows_amd64.zip` for most PCs;
   - `adbuddy_<version>_windows_arm64.zip` for ARM PCs.

   `<version>` has no leading `v`. To see which one you have, run `$env:PROCESSOR_ARCHITECTURE` in PowerShell: `AMD64` or `ARM64`.

2. Check the archive. This prints `True` when the archive's SHA-256 is the one in `checksums.txt`. Run it in the folder with both files and change the file name to yours. On an ARM PC, also change `windows_amd64` to `windows_arm64` in `-Pattern`. The checksum is not a signature: it catches a damaged download, not a tampered release.

   ```powershell
   $want = (Select-String -Path .\checksums.txt -Pattern 'windows_amd64\.zip').Line.Split(' ')[0]
   (Get-FileHash .\adbuddy_<version>_windows_amd64.zip -Algorithm SHA256).Hash -eq $want
   ```

   If it prints `False`, delete the download and fetch it again.

3. Unzip it in a folder of your own and add that folder to your `PATH`. The archive holds `adbuddy.exe`, `LICENSE` and `README.md`.

   ```powershell
   $dir = "$env:LOCALAPPDATA\Programs\adbuddy"
   New-Item -ItemType Directory -Force $dir
   Expand-Archive .\adbuddy_<version>_windows_amd64.zip -DestinationPath $dir -Force
   $path = [Environment]::GetEnvironmentVariable("Path", "User")
   if (($path -split ';') -notcontains $dir) {
     [Environment]::SetEnvironmentVariable("Path", "$path;$dir", "User")
   }
   ```

   This edits your user `PATH`. If it holds entries with `%VAR%` in them, check them afterwards in the Windows settings: this call can store them already expanded.

4. Open a new terminal and check it:

   ```powershell
   adbuddy --version
   ```

**Windows may warn you.** The Windows program is not code-signed yet, so SmartScreen can say "Windows protected your PC" the first time you run it. Run it only if you downloaded it from the project's Releases page and the checksum matched. Choose **More info**, then **Run anyway**. `Unblock-File "$dir\adbuddy.exe"` removes the mark that triggers the warning.

### The phone app

Once a release is published and the host pins its signing certificate, `adbuddy setup` downloads `adbuddy.apk` from the release, checks it against `adbuddy.apk.sha256` and against the release certificate, and only then installs it on the phone. It does nothing without that certificate. Today, a build says this and goes on without the app:

```text
This build of adbuddy has no pinned signing certificate for the app, so it does not download it: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.
```

Without the app the host still works, but you confirm in the terminal and there is no chat on the phone. `adbuddy setup --no-app` skips the app on purpose.

## Run setup

With the host in your `PATH`, no cable is needed. Put the phone on the same Wi-Fi, turn on Wireless debugging (see [Set up your phone](setup/)) and run:

```sh
adbuddy setup
```

It goes step by step. It skips pairing and the download when they are already done, but it asks for consent again and rewrites the environment, so you can run it again. It refuses to run while `adbuddy serve` is running (`ALREADY_RUNNING`): stop serve first.

1. Downloads Android platform-tools, checked against Google's checksum.
2. Pairs the phone with a QR code (or with `--code`, a pairing code) and connects.
3. Reads the phone's profile.
4. Installs the app, then waits up to 5 minutes for you to accept in it. If you accept, the PC gives the app the permission to cut access, exempts it from battery optimisation and turns on its accessibility service, keeping the ones you already use.
5. Picks the agent (it asks when you have several installed; `--agent` and `--yes` skip the question).
6. Creates the agent's environment folder, `~/ADBuddy/<phone name>/`, and saves your choices.

Then see [Use ADBuddy](usage.md).

## Update

1. Stop `adbuddy serve` (Ctrl+C in its terminal).
2. Replace the program:
   - **Linux and macOS, once there is a release:** run the install command again. It replaces the old program.
   - **Windows, once there is a release:** download the new archive, check it, and unzip it over the old folder.
   - **Built from source:** run `git pull`, build again and copy the program again.
3. Refresh the files in the agent's environment. `adbuddy serve` already refreshes the `adb` and `adbuddy` programs the agent uses. The instructions and skills change when you run setup again (with serve stopped), which also rewrites the phone's profile in `AGENTS.md`:

   ```sh
   adbuddy setup
   ```

   `NOTES.md` is never touched, and a skill file that contains `<!-- adbuddy:user -->` is kept. `adbuddy env init --force` also rewrites them, but it leaves the phone's profile in `AGENTS.md` as "Not collected yet." until the next setup.

4. Update the app by giving setup the newer APK: `adbuddy setup --apk <file>`. Plain `adbuddy setup` installs the app only when the phone has none, so it does not replace an older one by itself.

## Uninstall

Do the phone and the PC in this order.

1. Stop `adbuddy serve`. If you used remote mode, close it: `adbuddy remote off`.
2. **On the phone:** open **Settings → Apps → ADBuddy** and uninstall it. In **Developer options → Wireless debugging**, forget this PC under **Paired devices** (the wording varies by phone). Turn Wireless debugging off if you no longer need it.
3. **On the PC:** delete the program, the settings, the data and the environment folder.

The settings hold `config.toml` and `rules.yaml`. The data holds the downloaded platform-tools, the secret that signs the journal and the copy of the app. The environment folder holds `NOTES.md` and the **journal**, so deleting it also deletes the history and the ability to undo.

| System | Settings | Data |
|---|---|---|
| Linux | `~/.config/adbuddy/` | `~/.local/share/adbuddy/` |
| macOS | `~/Library/Application Support/ADBuddy/` | the same folder |
| Windows | `%APPDATA%\ADBuddy\` | `%LOCALAPPDATA%\ADBuddy\` |

Linux uses `$XDG_CONFIG_HOME` and `$XDG_DATA_HOME` when you set them. `ADBUDDY_CONFIG_DIR` and `ADBUDDY_DATA_DIR` override all of this.

On Linux and macOS:

```sh
rm ~/.local/bin/adbuddy
rm -r ~/.config/adbuddy ~/.local/share/adbuddy
rm -r ~/ADBuddy/<phone name>
```

On macOS the settings and the data are one folder: `rm -r ~/Library/Application\ Support/ADBuddy`.

On Windows, in PowerShell:

```powershell
Remove-Item -Recurse "$env:APPDATA\ADBuddy", "$env:LOCALAPPDATA\ADBuddy"
Remove-Item -Recurse "$HOME\ADBuddy\<phone name>"
Remove-Item -Recurse "$env:LOCALAPPDATA\Programs\adbuddy"
```

Then remove that folder from your `PATH` (Settings, search for "environment variables").

## Next

- [Set up your phone](setup/): Developer options, Wireless debugging and pairing (screenshots coming).
- [Use ADBuddy](usage.md): start the host, talk to the agent, undo, remote mode.
- [Safety](safety.md): what ADBuddy protects and what it does not.
- [Troubleshooting](troubleshooting.md): when something fails.
