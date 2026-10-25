# CI runs on GitHub-hosted runners only

This page is for maintainers. ADBuddy's CI does **not** use self-hosted runners.

Every workflow in `.github/workflows/` runs on runners hosted by GitHub (a fresh virtual machine per job). The repository has no registered self-hosted runners and no repository variables that choose a runner (the old `CI_RUNNER_*` and `CI_HOST_MATRIX`).

## Why not self-hosted

The repository is public. Anyone can open a pull request from a fork, and a pull request can change the code that CI runs: tests, the Gradle build, a workflow file. On a self-hosted runner that code would run on a maintainer's own machine, as the user that runs the runner, with whatever that user can reach: the adb key authorized on their phone, the adb server, SSH keys, the `gh` token, the credentials of any coding agent. A hosted runner is thrown away after the job and holds none of that.

So the rule is simple and has no exceptions: no self-hosted runner is registered for this repository, and none is added "just for the emulator" or "just for Dependabot".

## What guards the workflows instead

- "Approval for running fork pull request workflows" requires a maintainer's approval for pull requests from outside collaborators (Settings > Actions > General). Read the diff of the workflow files and of the build scripts before approving.
- Dependabot pull requests also run on hosted runners. Do not turn on "Dependabot on self-hosted runners".
- Releases are built and signed on hosted runners, on tag pushes only; the rules for the signing key and secrets are in [releasing.md](releasing.md).

## Running CI on your own machine

Not supported here. If you fork the project and want your own runners, that is your fork and your risk; keep such a fork private, and never register a runner on a public repository.

## History

Until 2026-10-12 the project ran part of its CI on the maintainer's own machines, while the repository was private. That setup, with its variables, runner labels and security notes, was removed when the repository became public. The old text of this page is in the git history (for example `git show aa7f3698:docs/ci-self-hosted.md`).
