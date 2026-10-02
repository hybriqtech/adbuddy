# ADBuddy host: developer notes

The `adbuddy` program: the host that sits between a coding agent and an Android phone. This page is for people who work on its code. To use ADBuddy, see the [project README](../README.md), [Install ADBuddy](../docs/install.md) and [Use ADBuddy](../docs/usage.md).

Licensed under Apache-2.0 (`LICENSE`, at the repository root).

## Build

You need [Go](https://go.dev/dl/) 1.27 or newer. From this folder (`host/`):

```sh
go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
./adbuddy --version                    # a build from source says "adbuddy dev"
```

A build from source pins no signing certificate for the phone app, so `adbuddy setup` does not download it: install your own build with `adbuddy setup --apk <file>`. Releases are built by the pipeline described in `docs/releasing.md`.

## Tests

```sh
go test ./...
```

The tests never touch a phone: they use a fake adb (`internal/testutil/fakeadb`). The Windows PC does not run `go test -race`; the CI does.

### The documentation guard

`TestDocs*` in `internal/cli/docs_test.go` checks the user guides (the READMEs and the pages of `docs/` that GitHub Pages publishes):

- every `adbuddy` command in a code block or in inline code is valid against the real command tree (command, flags and arguments);
- relative links point to files that exist, and a published page does not link outside `docs/`;
- no loose `<…>` marker (it must be inside backticks), no real IP addresses and no private paths;
- the required guides exist.

Run it after you touch a guide or a command:

```sh
go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m
```

### The classifier's oracle

What the phone's shell (mksh) really does is not taken from memory: `internal/classify/testdata/oracle.jsonl` records, for every case of `differential.jsonl` and every generated one, whether a real mksh ran the sentinel `@S@`. `go test` fails if a case is not recorded, or if mksh ran the sentinel and the classifier did not answer red special. `mksh-builtins.tsv` lists the builtins of that mksh, and each must be admitted with a grammar or never analysed.

To record them again after adding a case, use the Android emulator (SDK packages `emulator` and `system-images;android-37.0;google_apis;x86_64`). It has its own adb server, and the recorder refuses any device that is not an emulator:

```sh
eval "$(scripts/emulator.sh start)"     # from the repository root; about a minute
cd host && go test -timeout 60m -count=1 -run TestRecordOracle ./internal/classify   # about 10 minutes
go test ./internal/classify
../scripts/emulator.sh stop
```

The `emulator` workflow records it again on every pull request that touches the classifier and fails if the result differs from the files in the repository.

### The skills' read-only examples on the emulator

The read-only examples of the skills (`# green`, `shell ...`) also run on the emulator, with the same variables as the recorder:

```sh
go test -timeout 20m -count=1 -run TestSkillsReadOnlyOnEmulator ./internal/cli
```

It fails if the phone says a program or an option does not exist, and it refuses any serial that is not `emulator-*`. The `emulator` workflow runs it too.

### Claude Code with a real agent

This test runs the real Claude Code. It spends a few cents and never touches the phone or your `~/.claude` settings:

```sh
ADBUDDY_REAL_CLAUDE=1 go test ./internal/cli -run TestRealClaudeCode -v -timeout 600s
```

## The agent's environment

`adbuddy env init [--force]` creates or refreshes the agent's environment (`~/ADBuddy/<phone model>/`) without starting anything; `adbuddy setup` and `adbuddy serve` do the same. With Claude Code as the agent, it also puts six hooks in the environment's `.claude/settings.json` (`SessionStart`, `UserPromptSubmit`, `Stop`, `Notification`, `PermissionRequest`, `SessionEnd`), each running the environment's own `adbuddy hook <event>`; your own hooks in that file are kept. `adbuddy hook` fails open: if the host is down or anything is wrong, it says nothing and exits 0, so Claude Code asks in the terminal as usual. The design is in `docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md`, and the recordings the tests use are in `internal/claudecode/testdata`.

## Skills

Skills are recipes the agent follows for common phone tasks: `device-profile`, `permissions-audit`, `storage-cleanup`, `debloat`, `battery`, `organize-files`, `backup-to-pc`, `home-screen` and `build-utility-apk`. They live in `internal/workspace/skills/<name>/SKILL.md` and are embedded in the binary. `adbuddy env init` copies them to `.claude/skills/` and `.agents/skills/` of the environment (and writes `.gemini/settings.json` so Gemini CLI reads `AGENTS.md`); `adbuddy env init --force` refreshes them. A skill file that contains `<!-- adbuddy:user -->` is the user's and is never overwritten (checked per file: marking `SKILL.md` does not protect the other files of that skill). `adbuddy setup` rebuilds the environment with Force, so it rewrites every skill that lacks that mark. `adbuddy env init` prints the skill paths grouped by folder (with `/` also on Windows); `--json` lists them all.

To add a skill: create its folder, add its name to `requiredSkills` in `internal/workspace/skills_test.go`, and write its examples in checked blocks: ```` ```adb ```` (a command without the `adb` prefix, annotated `# green`, `# yellow` or `# red`), ```` ```adbuddy ```` (a subcommand) and, for commands that run on the PC, ```` ```text ```` (not checked). The guard `internal/cli/skills_commands_test.go` enforces:

- the annotation is the level the real classifier gives (at run time the level can rise through the precheck);
- recipes never clear app data, wipe, reboot, install granting permissions or delete recursively; the one exception is the exact `shell rm -r /sdcard/ADBuddy-papelera` in `storage-cleanup`;
- scripts go in single quotes, and a line that starts with `adb` or `adbuddy` outside a checked block is an error.

## Manual check with a real phone

Only with a phone you can afford to experiment on. With the phone paired and Wireless debugging on:

```sh
adbuddy serve --confirm deny     # deny: every red command is refused without asking
# in another terminal, using the environment's shim:
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell getprop ro.product.model
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell settings get global window_animation_scale
adbuddy journal
```
