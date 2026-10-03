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
| `CI_RUNNER_ANDROID` | `android.yml` | `"ubuntu-latest"` | JDK, Android SDK (`ANDROID_HOME`); Windows or Linux. Linux runner recommended (the Windows runner is the bottleneck: it runs one job at a time and the emulator oracle takes ~16 min and the app's tests about 15 min more (estimated, not measured)). |
| `CI_RUNNER_EMULATOR` | `emulator.yml` | `"ubuntu-latest"` | Android SDK, hardware acceleration (the AVDs `adbuddy-ci-api37` for the oracle and `adbuddy-ci-api34` for the app's tests are created by `scripts/emulator.sh` when missing), and JDK 17 for the app's tests (Gradle 9.8 is downloaded by `setup-gradle`); a Linux runner needs `/dev/kvm` (see section 6). |
| `CI_RUNNER_RELEASE` | `release-check.yml` (`scripts` and `goreleaser`) | `"ubuntu-latest"` | Linux with `shellcheck`. |
| `CI_RUNNER_SIGN` | `release-check.yml` (`sign`) | `"ubuntu-latest"` | Linux with an Android SDK: `apksigner` and `zipalign` are Linux scripts here. |

`emulator.yml` runs on `pull_request` (same path filters) and by hand with `workflow_dispatch`. It does not run on every push to `main`: merges are fast-forwards of a pull request that already passed it.

The special value `"none"` skips the job of a single-job workflow.

Example for "only a Windows runner for now". **This is not the recommended configuration**: every variable that points at the Windows runner makes it run the code of any branch as your Windows user, with your adb server and phone in reach (section 6, "Requirement"). Until the dedicated Windows account exists (user decision pending; the user said "later"), the risk stays bounded only because the repository is private and only our own agents push branches; do not make the repository public with the runner registered.

```sh
gh variable set CI_HOST_MATRIX --body '[{"name":"ubuntu-latest","runner":"ubuntu-latest"},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'
gh variable set CI_RUNNER_EMULATOR --body '["self-hosted","windows","adbuddy-local"]'
```

`CI_RUNNER_ANDROID` is left out on purpose: it runs Gradle and the unit tests of the branch with no isolated home, so on Windows it carries the same risk as the other two (it only belongs on the Windows runner under the same bounds or with a dedicated account). The Ubuntu cell of the matrix stays on a GitHub-hosted runner here; no label `linux` appears because there is no Linux runner. Leave `CI_RUNNER_RELEASE` and `CI_RUNNER_SIGN` unset (they stay on GitHub-hosted runners) or set them to `"none"`.

Example with both runners (Windows and the Linux runner of section 4):

```sh
gh variable set CI_HOST_MATRIX --body '[{"name":"ubuntu-latest","runner":["self-hosted","linux","adbuddy-local"]},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","linux","adbuddy-local"]'
gh variable set CI_RUNNER_RELEASE --body '["self-hosted","linux","adbuddy-local"]'
gh variable set CI_RUNNER_SIGN --body '["self-hosted","linux","adbuddy-local"]'
gh variable set CI_RUNNER_EMULATOR --body '["self-hosted","windows","adbuddy-local"]'
```

The emulator stays on the Windows runner (`CI_RUNNER_EMULATOR`): it has the AVD and the hardware acceleration of Windows, and the WSL2 runner has no KVM.

Never point a Linux cell at a Windows runner. The steps that only run on Linux (gofmt, `-race`) would be skipped and the cell would be green without them.

Every job whose variable is set refuses pull requests from forks. With no variables set, the workflows behave exactly as before, fork pull requests included, on GitHub-hosted runners.

For `CI_HOST_MATRIX`, `"none"` and `[]` are not valid: they break the workflow. Delete the variable, or leave in the array the cells you want.

### Emulator environment variables

`scripts/emulator.sh` (used by the emulator job and by the local oracle tests) reads these variables:

| Variable | Meaning |
| --- | --- |
| `ADBUDDY_EMU_AVD` | AVD name. The CI uses `adbuddy-ci-api37`. |
| `ADBUDDY_EMU_PORT` | Emulator port. The CI uses `5590`. |
| `ADBUDDY_EMU_ADB_PORT` | Port of the script's own adb server. The CI uses `5049`. It must be a number in 1024-65535 and not `5037` (the user's adb server) nor `5038` (ADBuddy's); otherwise the script exits with 2. |
| `ADBUDDY_EMU_GPU` | GPU mode (default `swiftshader_indirect`). |
| `ADBUDDY_EMU_IMAGE` | System image used when the AVD is created (the AVD is created with it the first time only: to change the image, use another AVD name). |
| `ADBUDDY_EMU_STABLE_SECS` | After boot, watch this many seconds that `system_server` and `surfaceflinger` do not restart. Default `0` (off); on a restart the script exits with 4. |

The CI values differ from the defaults so that a job does not collide with an emulator started by hand.

The script also runs `unset ADB_SERVER_SOCKET ANDROID_ADB_SERVER_ADDRESS`: adb gives them priority over `ANDROID_ADB_SERVER_PORT`, so they would point it at another server (the user's).

The script exports `ADB_MDNS=0` and `ADB_MDNS_AUTO_CONNECT=0` so that its adb server never auto-connects a real phone found over mDNS, and it aborts (exit 3) if that server sees any other device.

After boot, `start` also checks that the emulator on the port runs the AVD of `ADBUDDY_EMU_AVD` (`adb emu avd name`); if not, it stops it and exits with 3. `stop` waits, for up to 60 s, until adb no longer lists the emulator and its console port is closed, and only then stops the adb server: the emulator job stops the android-37 emulator and starts the android-34 one on the same port, and the start must not take the dying one for the new one. `stop` always exits 0 (after 60 s it warns on stderr).

Exit codes: `1` SDK or packages missing; `2` incorrect usage or an invalid `ADBUDDY_EMU_ADB_PORT`; `3` the adb server sees another device (or none), or the emulator on the port runs another AVD; `4` the emulator is not stable; `124` it did not boot in 10 minutes.

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

4. Create `.env` in the runner folder. Use forward slashes in `ANDROID_HOME`, and keep the caches inside the runner folder so CI never touches your real ones. **Target:** the runner has its own Android SDK, JDK and Go (inside the runner folder or the profile of the dedicated account), not the `ANDROID_HOME` or the `adb` you use every day: the `sdkmanager` steps of the workflows write into that SDK, and a branch could leave a trojan `adb.exe` or `emulator.exe` there that you or `adbuddy serve` would run later against your phone. This is not met today (see section 6):

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
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","linux","adbuddy-local"]'

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

- **Requirement** to keep `CI_HOST_MATRIX`, `CI_RUNNER_ANDROID` or `CI_RUNNER_EMULATOR` on Windows while agents push branches to the repository: run the runner under a dedicated local Windows account that has none of those. For adb it is the only barrier (see below).
- **Open risk today (user decision pending: dedicated Windows account, the user said "later"):** while the Windows runner runs under the user's own account, the requirement is NOT met. Meanwhile the risk stays bounded because the repository is private and only our own agents push branches; do not make the repository public with the runner registered. The attack: a test or Gradle task in a branch calls the `adb` on the runner's PATH, which reaches the user's adb server (port 5037) with the phone already connected (or loads `\Users\<user>\.android\adbkey` and pairs its own server), and from there it runs anything on the phone with no classifier, journal or confirmation; it also reads SSH keys, the `gh` token and the credentials of the coding agents. The closing is the dedicated Windows account, and it is the user's decision. Until then, do not leave any `CI_*` variable pointing at the Windows runner while nobody watches (or stop `run.cmd`).
- **Target, not met today:** the runner uses its own Android SDK, JDK and Go (section 3, step 4), never the user's `ANDROID_HOME` or `adb`. The `sdkmanager` steps of `android.yml` and `emulator.yml` write into the SDK, so a branch could replace `platform-tools/adb.exe` there and the change would persist for the user and `adbuddy serve`. Having a dedicated account does not fix it if that account is given write access to the user's SDK. Check that the `ANDROID_HOME` of the runner's `.env` is not the SDK you use with ADBuddy.
- **Minimum:** start `run.cmd` from a clean console, without `ADBUDDY_*`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT`, `ADB_SERVER_SOCKET` or `ANDROID_ADB_SERVER_ADDRESS` in the environment (the last two would make the diagnostic `adb devices -l` of `emulator.yml` ask the user's server and write the phone's serial in the Actions log).

The real barriers are:

- the repository is private;
- only branches of the repository itself are built;
- "Run workflows from fork pull requests" is off in the repository settings.

The fork guard in the workflows is only an extra, and it acts only when the job's variable is defined: a pull request runs the workflow files of its own merge commit, so it can remove the guard. The real barrier is the repository configuration.

Bots and agents that open pull requests from branches of the repository itself pass the guard and run their code on the runner. There is no `dependabot.yml` today.

The practical route to the key-reading risk is the branches that local coding agents push. An agent that is tricked (a poisoned web page, a malicious dependency, a prompt injection in an issue) can add a test or a script to a branch of this very repository, and the runner will run it as the PC's user, with no fork involved and past every guard above. Reviewing the diff before the runner picks it up is not realistic for agent branches.

adb on Windows loads the real key from \Users\<user>\.android\adbkey whatever `HOME`, `USERPROFILE` and `ANDROID_USER_HOME` say (measured with adb 37 and `ADB_TRACE=all`: `auth.cpp loaded new key from 'C:\Users\...\.android\adbkey'`). Isolating the environment of the job therefore does not protect the adb key. For adb the only barrier is a dedicated Windows account for the runner.

What `host.yml` does (and `emulator.yml` does not, because its adb and emulator must share the real key or adb reports `unauthorized`): each self-hosted Windows job gets its own `HOME`, `USERPROFILE`, `ANDROID_USER_HOME`, `APPDATA` and `LOCALAPPDATA` (folders inside `RUNNER_TEMP`) and an empty `ADB_VENDOR_KEYS`. That covers only what depends on the environment: ssh, gh, git, the configuration of coding agents and of ADBuddy. It covers `gh` and git only in part: on Windows `gh` keeps its configuration in `%AppData%\GitHub CLI` and the git token is in the Credential Manager, and neither depends on `HOME`; the isolated `APPDATA`/`LOCALAPPDATA` cover the first only partly. `GOPATH` moves to the isolated home, with no effect because `GOMODCACHE` and `GOCACHE` are explicit. A malicious process can still read the real profile by absolute path, so this stops accidents and naive code, not an attacker.

### Linux runner

Barriers:

- The `runner` user has no root and no sudo.
- `/etc/wsl.conf` disables the Windows drives (`automount`), interop and the Windows PATH, and locks `/dev/dxg`.
- `.wslconfig` uses NAT networking, no localhost forwarding and no WSLg.
- Step 4 of section 4 checks all of it. Run it again after every `wsl --update` or change of configuration.

Residual risk: a bug in the guest kernel gives root, and root can mount C: and read your adb key. The structural barrier is a dedicated Windows account that installs the distribution and launches `wsl.exe`. Keep `wsl --update` current.

Phone: do not use `adb tcpip`, and reject any unexpected authorization dialog on the phone (the NAT reaches the LAN).

Persistence between jobs: the runner is not ephemeral, so its jobs share the runner folder and its caches. The `runner` user can read the runner's credentials and poison `GOMODCACHE`, `GOCACHE` and Gradle in the runner folder, which affects later jobs. This is the cost of a non-ephemeral runner; `--ephemeral` with a new registration per job would avoid it.

### Windows runner: persistence between jobs

The Windows runner is not ephemeral either, and nothing cleans it automatically: no workflow removes `_work/_tool` (the Go and JDK that `setup-go` and `setup-java` install), `GOMODCACHE`, `GOCACHE` or `GRADLE_USER_HOME` (checked in `.github/workflows/`: no cleaning step). A branch can poison them, or the runner's `.env` and `.path` (the same user writes them), and every later job, including the `push` to `main`, then runs with those tools: a green on `main` could be fabricated or a failure hidden. What to clean by hand when you suspect a branch: stop the runner, delete `_work/_tool`, `gradle`, `gomod` and `gocache` of the runner folder, and re-check `.env` and `.path` against section 3. With a dedicated account, `.env`, `.path` and the runner's `bin/` should belong to an administrator and not be writable by that account. `--ephemeral` with a new registration per job would avoid all of it.

### Also

- Take the runners down (remove them in the repository settings and delete the runner folders) before making the repository public.
- The workflows use `pull_request` and `push`, never `pull_request_target`, and these jobs have no secrets.
- One job runs at a time per runner. Register a second runner in another folder to run jobs in parallel.
- The emulator job uses and stops the AVD `adbuddy-ci-api37` (emulator port 5590) and the adb server on port 5049 of the runner's machine: do not use them by hand meanwhile. It no longer uses the script's default AVD and ports, which can be used by hand at the same time.
- Two emulators at once need about 5 GB of RAM each.
- A Linux runner needs /dev/kvm accessible through the kvm group for the emulator job (WSL2 here has no /dev/kvm without nested virtualization, so the emulator job stays on the Windows runner). Never give the runner user sudo: root in the distribution can mount C: and read your adb key.

### Repository settings

What is applied today (replace `OWNER/REPO`):

- **Actions permissions.** Actions enabled, and pinning to a full commit SHA required (`sha_pinning_required=true`): a workflow that uses an action by tag or branch fails. Apply (together with the allowed actions below):

  ```sh
  gh api -X PUT repos/OWNER/REPO/actions/permissions -F enabled=true -f allowed_actions=selected -F sha_pinning_required=true
  ```

- **Allowed actions.** `allowed_actions=selected`: GitHub-owned actions (`github_owned_allowed=true`), no actions from verified creators (`verified_allowed=false`) and only these patterns: `gradle/actions/setup-gradle@*` and `goreleaser/goreleaser-action@*`. Warning: a workflow that uses a new action from another owner fails at once with `startup_failure` until you add it:

  ```sh
  gh api -X PUT repos/OWNER/REPO/actions/permissions/selected-actions -F github_owned_allowed=true -F verified_allowed=false -f 'patterns_allowed[]=gradle/actions/setup-gradle@*' -f 'patterns_allowed[]=goreleaser/goreleaser-action@*'
  ```

  The PUT replaces the whole list: to add an owner, repeat the command with every pattern plus `-f 'patterns_allowed[]=NEW-OWNER/NEW-ACTION@*'`.

  Revert (any action allowed, pinning kept): `gh api -X PUT repos/OWNER/REPO/actions/permissions -F enabled=true -f allowed_actions=all -F sha_pinning_required=true`. To drop the pinning too, repeat it with `-F sha_pinning_required=false`.

- **Default workflow permissions.** `default_workflow_permissions=read` was already the default (not changed by us): a workflow without its own `permissions:` block gets a read-only token. To set it explicitly:

  ```sh
  gh api -X PUT repos/OWNER/REPO/actions/permissions/workflow -f default_workflow_permissions=read -F can_approve_pull_request_reviews=false
  ```

- **Run workflows from fork pull requests:** already off by default in private repositories (not changed by us). Check: `gh api repos/OWNER/REPO/actions/permissions/fork-pr-workflows-private-repos` returns `run_workflows_from_fork_pull_requests: false`.

What is not applied, and why:

- **The guard in the workflows and the private repository** are what stop forks, besides the setting above.
- **The `release` environment does not exist yet.** Create it with required reviewers before releases depend on it, and do not create the signing secrets before those reviewers are effective (see `docs/releasing.md`, "Before creating the secrets"). On a tag push GitHub uses the `release.yml` of the tagged commit, so a branch can edit that file and drop its "tag is on main" check; the environment rule `v*` only looks at the tag name. Without a required reviewer that really stops the job, the secrets are readable from any branch.
- **Rulesets and branch protection are not available** in the free plan for a private repository. When the repository goes public, create the `release` environment with a reviewer and protect `main` in the same step.

## 7. What is not covered locally

- macOS: the macOS cell is left out of `CI_HOST_MATRIX`, so macOS is not tested locally while the variable is set that way.
- `release.yml` always runs on GitHub-hosted runners.
