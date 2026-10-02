# Running CI on self-hosted runners

This page is for maintainers. It explains how to run the repository's CI jobs on your own machine instead of GitHub-hosted runners, and how to switch back.

## 1. Why

- **Billing or outage.** When GitHub Actions does not start jobs (billing problem, outage, quota), a self-hosted runner keeps CI working.
- **Privacy.** The repository can stay private without spending hosted minutes.
- **Speed.** Warm caches and a local Android SDK make runs faster.

It does not replace `release.yml`: releases are built and signed on GitHub-hosted runners (signing secrets, tags only), and that workflow is never pointed at your machine.

## 2. The variables

Each job reads its runner from a repository variable. The value is JSON: a string such as `"ubuntu-latest"` or an array of labels such as `["self-hosted","windows","adbuddy-local"]`. With no variable set, every workflow runs on GitHub-hosted runners exactly as before.

| Variable | Used by | Default | What the runner needs |
| --- | --- | --- | --- |
| `CI_HOST_MATRIX` | `host.yml` | the three hosted cells (`ubuntu-latest`, `windows-latest`, `macos-latest`) | A JSON array of cells `{"name": ..., "runner": ...}`. A Linux cell needs `gcc` (for `go test -race`); a Windows cell needs Git for Windows. Leave a cell out of the array to drop it. |
| `CI_RUNNER_ANDROID` | `android.yml` | `"ubuntu-latest"` | JDK, Android SDK (`ANDROID_HOME`); Windows or Linux. |
| `CI_RUNNER_EMULATOR` | `emulator.yml` | `"ubuntu-latest"` | Android SDK, the AVD `adbuddy-ci-api37`, hardware acceleration; a Linux runner needs `/dev/kvm` (see section 6). |
| `CI_RUNNER_RELEASE` | `release-check.yml` (`scripts` and `goreleaser`) | `"ubuntu-latest"` | Linux with `shellcheck`. |
| `CI_RUNNER_SIGN` | `release-check.yml` (`sign`) | `"ubuntu-latest"` | Linux with an Android SDK: `apksigner` and `zipalign` are Linux scripts here. |

The special value `"none"` skips the job of a single-job workflow.

Example for "only a Windows runner for now":

```sh
gh variable set CI_HOST_MATRIX --body '[{"name":"ubuntu-latest","runner":"ubuntu-latest"},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","windows","adbuddy-local"]'
```

Leave `CI_RUNNER_RELEASE` and `CI_RUNNER_SIGN` unset (they stay on GitHub-hosted runners) or set them to `"none"`. The same goes for `CI_RUNNER_EMULATOR`.

Example with both runners (Windows and the Linux runner of section 4):

```sh
gh variable set CI_HOST_MATRIX --body '[{"name":"ubuntu-latest","runner":["self-hosted","linux","adbuddy-local"]},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","windows","adbuddy-local"]'
gh variable set CI_RUNNER_RELEASE --body '["self-hosted","linux","adbuddy-local"]'
gh variable set CI_RUNNER_SIGN --body '["self-hosted","linux","adbuddy-local"]'
```

The emulator stays on the Windows runner (`CI_RUNNER_EMULATOR`): it has the AVD and the hardware acceleration of Windows, and the WSL2 runner has no KVM.

Never point a Linux cell at a Windows runner. The steps that only run on Linux (gofmt, `-race`) would be skipped and the cell would be green without them.

Every job whose variable is set refuses pull requests from forks. With no variables set, the workflows behave exactly as before, fork pull requests included, on GitHub-hosted runners.

For `CI_HOST_MATRIX`, `"none"` and `[]` are not valid: they break the workflow. Delete the variable, or leave in the array the cells you want.

## 3. Setting up a Windows runner

1. Download the runner (`win-x64`) from the official `actions/runner` releases page and check the SHA-256 listed in the release notes before unpacking it into `%USERPROFILE%\actions-runner`.
2. Get a registration token. It is short-lived: never write it to a file, a log or a commit.

   ```sh
   gh api -X POST repos/OWNER/REPO/actions/runners/registration-token --jq .token
   ```

3. Configure the runner with the labels the workflows expect:

   ```bat
   cd %USERPROFILE%\actions-runner
   config.cmd --url https://github.com/OWNER/REPO --token TOKEN --labels self-hosted,windows,adbuddy-local
   ```

4. Create `.env` in the runner folder. Use forward slashes in `ANDROID_HOME`, and keep the caches inside the runner folder so CI never touches your real ones:

   ```text
   ANDROID_HOME=C:/path/to/android-sdk
   JAVA_HOME=C:/path/to/jdk
   GRADLE_USER_HOME=C:/path/to/actions-runner/gradle
   GOMODCACHE=C:/path/to/actions-runner/gomod
   GOCACHE=C:/path/to/actions-runner/gocache
   ```

5. Create `.path`. It REPLACES the PATH of the jobs, so generate it from your current PATH, putting the `bin` folder of Git for Windows (the one that holds `bash.exe`) before `C:\Windows\System32` (WSL installs a `bash.exe` there that would shadow Git Bash). Run this in PowerShell from the runner folder; it checks that `bash.exe` exists, so adjust `$bin` if Git is installed elsewhere:

   ```powershell
   $bin = 'C:\Program Files\Git\bin'
   if (-not (Test-Path "$bin\bash.exe")) { throw "bash.exe not found in $bin" }
   $parts = @($bin) + ($env:PATH -split ';' | Where-Object { $_ -and $_ -ne $bin })
   [System.IO.File]::WriteAllText("$PWD\.path", ($parts -join ';'), (New-Object System.Text.UTF8Encoding($false)))
   ```

6. Run it as a foreground process, not as a service, so it only exists while you want it:

   ```bat
   run.cmd
   ```

   To start it with a clean environment, relaunch it from Git Bash with `env -i` and pass only what Windows tools need. Pass `OS=Windows_NT` too: some tools detect Windows from `$OS` (`scripts/emulator.sh` also detects it with `uname`, but others do not). Pass `SYSTEMROOT`, `USERPROFILE`, `LOCALAPPDATA`, `APPDATA`, `TEMP`, `TMP`, `HOME` and `COMSPEC` as well, copying their values from your session:

   ```sh
   env -i OS=Windows_NT SYSTEMROOT="$SYSTEMROOT" USERPROFILE="$USERPROFILE" LOCALAPPDATA="$LOCALAPPDATA" APPDATA="$APPDATA" TEMP="$TEMP" TMP="$TMP" HOME="$HOME" COMSPEC="$COMSPEC" PATH="$PATH" ./run.cmd
   ```

## 4. Setting up a Linux runner (WSL2 Ubuntu)

This is the procedure that was tested. The Linux runner lives in a WSL2 Ubuntu distribution that cannot see your Windows files (check it with step 4).

1. In PowerShell as administrator, install the distribution. The first time it may ask for a reboot (Virtual Machine Platform):

   ```powershell
   wsl --install -d Ubuntu --no-launch
   ```

2. Limit and isolate the VM. RAM is the scarce resource of the PC (one emulator uses about 5 GB). Create `%USERPROFILE%\.wslconfig`:

   ```ini
   [wsl2]
   memory=8GB
   processors=6
   swap=2GB
   networkingMode=nat
   localhostForwarding=false
   guiApplications=false
   ```

   - `networkingMode=nat`: the loopback of WSL is not the loopback of Windows. `mirrored` would break this and is critical to avoid.
   - `localhostForwarding=false`: a job cannot take loopback ports of Windows such as 5037.
   - `guiApplications=false`: no WSLg, so no clipboard, microphone or windows on your desktop.

   `.wslconfig` is global to your Windows user and is applied with `wsl --shutdown` (`wsl -t` is not enough).

3. Isolate the runner. As root in Ubuntu (`wsl -d Ubuntu -u root`), create the user and write `/etc/wsl.conf`:

   ```sh
   adduser --disabled-password --gecos '' runner
   groupadd -f kvm
   usermod -aG kvm runner
   cat > /etc/wsl.conf <<'EOF'
   [automount]
   enabled=false
   mountFsTab=false

   [interop]
   enabled=false
   appendWindowsPath=false

   [user]
   default=runner

   [boot]
   command="echo -1 > /proc/sys/fs/binfmt_misc/WSLInterop 2>/dev/null; echo -1 > /proc/sys/fs/binfmt_misc/WSLInterop-late 2>/dev/null; chmod 600 /dev/dxg 2>/dev/null; chgrp kvm /dev/kvm 2>/dev/null; chmod 660 /dev/kvm 2>/dev/null"
   EOF
   ```

   Then apply both files from PowerShell with `wsl --shutdown`. The runner user has no root and no sudo. Do not rely on this alone: check it with step 4.

4. Verify the isolation. Open a shell with the environment of the runner:

   ```powershell
   wsl.exe -d Ubuntu -u runner --exec env -i HOME=$HOME PATH=/usr/bin:/bin bash
   ```

   and check all of this:

   - `/proc/sys/fs/binfmt_misc/WSLInterop` does not exist.
   - Copy a real Windows executable into the distribution and run it. From PowerShell:

     ```powershell
     cmd /c "wsl.exe -d Ubuntu -u runner --exec sh -c 'cat > /tmp/w.exe && chmod 755 /tmp/w.exe' < C:\Windows\System32\whoami.exe"
     ```

     Neither `/tmp/w.exe` nor `/init /tmp/w.exe` may ever print your Windows `computer\user`.
   - Ports 5037, 5038, 5039 and 5049 are closed on `127.0.0.1` and on the gateway (`ip route show default`): `timeout 3 bash -c '</dev/tcp/HOST/PORT'` must fail for each.
   - `python3 -m http.server 5999 --bind 127.0.0.1` in WSL is not reachable from Windows: `Test-NetConnection 127.0.0.1 -Port 5999` must fail.
   - `/mnt/wslg` and `/tmp/.X11-unix` are empty or do not exist.
   - `ls -l /dev/dxg` shows mode 600, owner root.
   - `mkdir /tmp/x; mount -t drvfs C: /tmp/x` fails ("must be superuser").
   - `sudo -n true` is denied.

   If anything prints your Windows user, stop the runner at once.

5. Install the packages as root (`gcc` comes with `build-essential` and is required by `go test -race`):

   ```sh
   apt update
   apt install -y build-essential git curl unzip zip shellcheck openjdk-17-jdk-headless
   ```

6. As `runner`, install Go and the Android SDK, always checking the checksum before unpacking:

   - Go: the version in `host/go.mod`, from go.dev. Get the SHA-256 of the `linux-amd64` archive from `https://go.dev/dl/?mode=json`, check it with `sha256sum -c`, and unpack it so that `~/tools/go` holds the toolchain.
   - Android SDK: download the command-line tools listed in `https://dl.google.com/android/repository/repository2-3.xml`, check the SHA-1 listed there with `sha1sum -c`, and unpack them as `~/android-sdk/cmdline-tools/latest`. Then install the packages (accepting the SDK licenses is part of it):

     ```sh
     export ANDROID_HOME=~/android-sdk
     yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" --licenses
     "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0" "build-tools;37.0.0" "platform-tools"
     ```

7. Install the runner as `runner`: download the `linux-x64` package of `actions/runner` into `~/actions-runner`, check the SHA-256 of the release notes, and unpack it. As root, run `./bin/installdependencies.sh` in that folder.

8. Register it. The registration token must go only through the stdin of the script: never in argv and never in a file. From PowerShell, send a first line that sets the variable, followed by the script, to `bash -s` (with `-c` the stdin does not arrive). If the token comes from `gh.exe`, trim the trailing CR and LF:

   ```sh
   TOKEN=$(gh api -X POST repos/OWNER/REPO/actions/runners/registration-token --jq .token | tr -d '\r\n')
   {
     printf "RT='%s'\n" "$TOKEN"
     cat <<'EOF'
   cd ~/actions-runner
   ./config.sh --unattended --url https://github.com/OWNER/REPO --token "$RT" --labels self-hosted,linux,adbuddy-local
   EOF
   } | wsl.exe -d Ubuntu -u runner -- bash -s
   ```

9. Configure the jobs' environment. In `~/actions-runner/.env`, with the caches inside the runner folder (the runner does not expand `~`, so generate the file with the real paths):

   ```sh
   cd ~/actions-runner
   cat > .env <<EOF
   ANDROID_HOME=$HOME/android-sdk
   ANDROID_SDK_ROOT=$HOME/android-sdk
   JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64
   GOTOOLCHAIN=local
   GRADLE_USER_HOME=$PWD/gradle
   GOMODCACHE=$PWD/gomod
   GOCACHE=$PWD/gocache
   EOF
   printf '%s' "$HOME/tools/go/bin" > .path
   ```

   `.path` REPLACES the PATH of the jobs, so keep in it everything the jobs need: add the system folders (`/usr/local/bin:/usr/bin:/bin`) after the Go `bin` folder.

10. Start it as a foreground process with a clean environment, from PowerShell:

   ```powershell
   wsl.exe -d Ubuntu -u runner --exec sh -c 'cd ~/actions-runner && exec env -i HOME="$HOME" USER=runner LOGNAME=runner LANG=C.UTF-8 PATH=/usr/local/bin:/usr/bin:/bin ./run.sh'
   ```

   That `wsl.exe` process keeps the VM alive. It is not a service and it does not survive a reboot: start it again after one.

## 5. Switching

```sh
# Use a self-hosted runner for a job group
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","windows","adbuddy-local"]'

# Skip a single-job workflow
gh variable set CI_RUNNER_EMULATOR --body '"none"'

# Back to GitHub-hosted runners: delete all five
gh variable delete CI_HOST_MATRIX
gh variable delete CI_RUNNER_ANDROID
gh variable delete CI_RUNNER_EMULATOR
gh variable delete CI_RUNNER_RELEASE
gh variable delete CI_RUNNER_SIGN
```

## 6. Security

A runner executes the code of the branches it builds, as the user that runs it. That user can reach its adb key (authorised on the phone), the adb server, SSH keys, the `gh` token and the credentials of any coding agent.

- **Recommended:** run the runner under a dedicated local Windows account that has none of those.
- **Minimum:** start `run.cmd` from a clean console, without `ADBUDDY_*`, `ANDROID_SERIAL` or `ANDROID_ADB_SERVER_PORT` in the environment.

The real barriers are:

- the repository is private;
- only branches of the repository itself are built;
- "Run workflows from fork pull requests" is off in the repository settings.

The fork guard in the workflows is only an extra, and it acts only when the job's variable is defined: a pull request runs the workflow files of its own merge commit, so it can remove the guard. The real barrier is the repository configuration.

Bots and agents that open pull requests from branches of the repository itself pass the guard and run their code on the runner. There is no `dependabot.yml` today.

### Linux runner

Barriers:

- The `runner` user has no root and no sudo.
- `/etc/wsl.conf` disables the Windows drives (`automount`), interop and the Windows PATH, and locks `/dev/dxg`.
- `.wslconfig` uses NAT networking, no localhost forwarding and no WSLg.
- Step 4 of section 4 checks all of it. Run it again after every `wsl --update` or change of configuration.

Residual risk: a bug in the guest kernel gives root, and root can mount C: and read your adb key. The structural barrier is a dedicated Windows account that installs the distribution and launches `wsl.exe`. Keep `wsl --update` current.

Phone: do not use `adb tcpip`, and reject any unexpected authorization dialog on the phone (the NAT reaches the LAN).

Persistence between jobs: the `runner` user can read the runner's credentials and poison `GOMODCACHE`, `GOCACHE` and Gradle in the runner folder, which affects later jobs. This is the cost of a non-ephemeral runner; `--ephemeral` with a new registration per job would avoid it.

### Also

- Take the runners down (remove them in the repository settings and delete the runner folders) before making the repository public.
- The workflows use `pull_request` and `push`, never `pull_request_target`, and these jobs have no secrets.
- One job runs at a time per runner. Register a second runner in another folder to run jobs in parallel.
- The emulator job uses and stops the AVD `adbuddy-ci-api37` (emulator port 5590) and the adb server on port 5049 of the runner's machine: do not use them by hand meanwhile. It no longer uses the script's default AVD and ports, which can be used by hand at the same time.
- Two emulators at once need about 5 GB of RAM each.
- A Linux runner needs /dev/kvm accessible through the kvm group for the emulator job (WSL2 here has no /dev/kvm without nested virtualization, so the emulator job stays on the Windows runner). Never give the runner user sudo: root in the distribution can mount C: and read your adb key.

## 7. What is not covered locally

- macOS: the macOS cell is left out of `CI_HOST_MATRIX`, so macOS is not tested locally while the variable is set that way.
- `release.yml` always runs on GitHub-hosted runners.
