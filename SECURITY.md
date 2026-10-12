# Security policy

## Reporting a vulnerability

Please report security problems **privately**, not in a public issue or pull request.

Use GitHub's private vulnerability reporting: open a [new security advisory](https://github.com/hybriqtech/adbuddy/security/advisories/new) on this repository. Only the maintainers can see it.

Please include:

- what you found and which part of ADBuddy it affects (host, phone app, classifier, shim, journal, confirmations, release pipeline);
- the version or commit, and the Android version if the phone is involved;
- how to reproduce it, with the smallest example you can;
- what an attacker gains, and what they need first.

Please do not post working exploits, your phone's serial number or your IP addresses in public.

## Supported versions

Until the first release, the `main` branch is the supported version. After that, only the latest release is supported. Fixes land in `main` and ship in the next release; older releases are not patched.

## What to expect

Reports are read by the maintainers and answered as time allows. There is no committed response time or disclosure schedule.

What ADBuddy does and does not guarantee is described in [docs/safety.md](docs/safety.md).
