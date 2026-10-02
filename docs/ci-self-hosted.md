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
| `CI_RUNNER_EMULATOR` | `emulator.yml` | `"ubuntu-latest"` | Android SDK, the AVD `adbuddy-ci-api37`, hardware acceleration; a Linux runner needs passwordless `sudo` or an accessible `/dev/kvm`. |
| `CI_RUNNER_RELEASE` | `release-check.yml` (`scripts` and `goreleaser`) | `"ubuntu-latest"` | Linux with `shellcheck`. |
| `CI_RUNNER_SIGN` | `release-check.yml` (`sign`) | `"ubuntu-latest"` | Linux with an Android SDK: `apksigner` and `zipalign` are Linux scripts here. |

The special value `"none"` skips the job of a single-job workflow.

Example for "only a Windows runner for now":

```sh
gh variable set CI_HOST_MATRIX --body '[{"name":"ubuntu-latest","runner":"ubuntu-latest"},{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-local"]}]'
gh variable set CI_RUNNER_ANDROID --body '["self-hosted","windows","adbuddy-local"]'
```

Leave `CI_RUNNER_RELEASE` and `CI_RUNNER_SIGN` unset (they stay on GitHub-hosted runners) or set them to `"none"`. The same goes for `CI_RUNNER_EMULATOR`.

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

## 4. Setting up a Linux runner (WSL2 Ubuntu)

Install the tools the jobs need (`gcc` comes with `build-essential` and is required by `go test -race`):

```sh
sudo apt install -y build-essential git curl unzip zip shellcheck
```

Then download and configure the runner as above, with the `linux-x64` package, in `~/actions-runner`, and the labels `self-hosted,linux,adbuddy-local`:

```sh
cd ~/actions-runner
./config.sh --url https://github.com/OWNER/REPO --token TOKEN --labels self-hosted,linux,adbuddy-local
./run.sh
```

For `CI_RUNNER_SIGN` (and the Android jobs) also install an Android SDK and set `ANDROID_HOME` in the runner's `.env`. In the same `.env`, set `GRADLE_USER_HOME`, `GOMODCACHE` and `GOCACHE` to folders inside the runner folder, so CI never touches your real caches.

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

Also:

- Take the runners down (remove them in the repository settings and delete the runner folders) before making the repository public.
- The workflows use `pull_request` and `push`, never `pull_request_target`, and these jobs have no secrets.
- One job runs at a time per runner. Register a second runner in another folder to run jobs in parallel.
- The emulator job uses and stops the AVD `adbuddy-ci-api37` (emulator port 5590) and the adb server on port 5049 of the runner's machine: do not use them by hand meanwhile. It no longer uses the script's default AVD and ports, which can be used by hand at the same time.
- Two emulators at once need about 5 GB of RAM each.
- A Linux runner needs passwordless `sudo` or an already accessible `/dev/kvm` for the emulator job.

## 7. What is not covered locally

- macOS: the macOS cell is left out of `CI_HOST_MATRIX`, so macOS is not tested while the variable is set that way.
- `release.yml` always runs on GitHub-hosted runners.
