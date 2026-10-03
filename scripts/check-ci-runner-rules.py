#!/usr/bin/env python3
"""Contract test of the CI runner-selection rules (docs/ci-self-hosted.md, section 6).

It does not run GitHub Actions. It takes the real `if:`, `runs-on:` and
`matrix.include` expressions out of the workflows and evaluates them with a
small evaluator of the subset of the Actions expression language they use
(&&, ||, !, ==, !=, parentheses, string literals, github.*, vars.*, fromJSON,
operands returned as in Actions, case-insensitive string equality), for each
event x variable case, and compares the result with the expected table below.

Needs PyYAML. Run from anywhere:  python scripts/check-ci-runner-rules.py
Exit code 0 when every cell matches, 1 otherwise.
"""
import json
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
WF = ROOT / ".github" / "workflows"
OWNER, REPO = "owner-account", "owner-account/adbuddy"

# ---- expression evaluator -------------------------------------------------

TOKEN = re.compile(r"\s*(?:(\|\||&&|==|!=|[()!,])|'((?:[^']|'')*)'|([A-Za-z_][\w.]*))")


def tokenize(src):
    pos, out = 0, []
    while pos < len(src):
        if src[pos:].strip() == "":
            break
        m = TOKEN.match(src, pos)
        if not m:
            raise ValueError("cannot tokenize at: " + src[pos:pos + 30])
        pos = m.end()
        if m.group(1):
            out.append(("op", m.group(1)))
        elif m.group(2) is not None:
            out.append(("str", m.group(2).replace("''", "'")))
        else:
            out.append(("id", m.group(3)))
    return out


def truthy(v):
    return not (v is None or v is False or v == "" or v == 0)


def eq(a, b):
    if isinstance(a, str) and isinstance(b, str):
        return a.lower() == b.lower()
    return a == b


class Parser:
    def __init__(self, src, ctx):
        self.t, self.i, self.ctx = tokenize(src), 0, ctx

    def peek(self):
        return self.t[self.i] if self.i < len(self.t) else (None, None)

    def take(self):
        tok = self.peek()
        self.i += 1
        return tok

    def parse(self):
        v = self.or_()
        if self.i != len(self.t):
            raise ValueError("trailing tokens")
        return v

    def or_(self):
        v = self.and_()
        while self.peek() == ("op", "||"):
            self.take()
            r = self.and_()  # both sides are evaluated: no side effects here
            v = v if truthy(v) else r
        return v

    def and_(self):
        v = self.cmp_()
        while self.peek() == ("op", "&&"):
            self.take()
            r = self.cmp_()
            v = r if truthy(v) else v
        return v

    def cmp_(self):
        v = self.unary()
        while self.peek() in (("op", "=="), ("op", "!=")):
            op = self.take()[1]
            r = self.unary()
            v = eq(v, r) if op == "==" else not eq(v, r)
        return v

    def unary(self):
        if self.peek() == ("op", "!"):
            self.take()
            return not truthy(self.unary())
        return self.atom()

    def atom(self):
        kind, val = self.take()
        if (kind, val) == ("op", "("):
            v = self.or_()
            assert self.take() == ("op", ")")
            return v
        if kind == "str":
            return val
        if kind == "id":
            if val == "fromJSON":
                assert self.take() == ("op", "(")
                arg = self.or_()
                assert self.take() == ("op", ")")
                return json.loads(arg)
            return self.lookup(val)
        raise ValueError("unexpected token %r" % (val,))

    def lookup(self, path):
        cur = self.ctx
        for part in path.split("."):
            if not isinstance(cur, dict) or part not in cur:
                return None
            cur = cur[part]
        return cur


def evaluate(expr, ctx):
    """expr is a whole `${{ ... }}` scalar, or a plain value (returned as is)."""
    if not isinstance(expr, str):
        return expr
    m = re.fullmatch(r"\s*\$\{\{(.*)\}\}\s*", expr, re.S)
    if not m:
        return expr
    return Parser(m.group(1), ctx).parse()


# ---- contexts -------------------------------------------------------------

def event(kind):
    base = {"repository": REPO, "repository_owner": OWNER}
    if kind == "dispatch":
        return {**base, "event_name": "workflow_dispatch", "event": {}}
    if kind == "push":
        return {**base, "event_name": "push", "event": {}}
    login, head = {
        "pr-owner": (OWNER, REPO),
        "pr-dependabot": ("dependabot[bot]", REPO),
        "pr-other-bot": ("some-other-bot[bot]", REPO),
        "pr-collaborator": ("a-collaborator", REPO),
        "pr-fork": ("a-stranger", "a-stranger/adbuddy"),
    }[kind]
    return {**base, "event_name": "pull_request",
            "event": {"pull_request": {"user": {"login": login},
                                       "head": {"repo": {"full_name": head}}}}}


EVENTS = ["pr-owner", "pr-dependabot", "pr-other-bot", "pr-collaborator", "pr-fork", "push", "dispatch"]
TRUSTED = {"pr-owner", "push", "dispatch"}
SELF = '["self-hosted","adbuddy-local"]'
HOST_SELF = ('[{"name":"ubuntu-latest","runner":["self-hosted","linux"]},'
             '{"name":"windows-latest","runner":["self-hosted","windows"]}]')
HOST_DEFAULT = ["ubuntu-latest", "windows-latest", "macos-latest"]


def jobs(name):
    return yaml.safe_load((WF / name).read_text(encoding="utf-8"))["jobs"]


failures, rows = [], []


def check(label, got, want):
    ok = got == want
    rows.append((label, got, ok))
    if not ok:
        failures.append("%s: got %r, want %r" % (label, got, want))


# Single-runner jobs: (file, job, variable). Each cell gives (job runs?, runs-on).
SINGLE = [("android.yml", "unit", "CI_RUNNER_ANDROID"),
          ("release-check.yml", "scripts", "CI_RUNNER_RELEASE"),
          ("release-check.yml", "goreleaser", "CI_RUNNER_RELEASE"),
          ("release-check.yml", "sign", "CI_RUNNER_SIGN"),
          ("emulator.yml", "mksh", "CI_RUNNER_EMULATOR")]

for wf, jname, var in SINGLE:
    job = jobs(wf)[jname]
    for ev in EVENTS:
        if wf == "emulator.yml" and ev == "push":
            continue  # emulator.yml has no push trigger
        if wf != "emulator.yml" and ev == "dispatch":
            continue  # only emulator.yml has workflow_dispatch
        for val, vname in (("", "empty"), (SELF, "self-hosted"), ('"none"', "none")):
            ctx = {"github": event(ev), "vars": {var: val}}
            run = truthy(evaluate(job.get("if", "${{ true }}"), ctx))
            runs_on = evaluate(job["runs-on"], ctx) if run else None
            if val == '"none"':
                want_run = False
            elif ev == "pr-fork" and val != "":
                want_run = False  # head-repo guard, unchanged
            elif wf == "emulator.yml" and ev not in TRUSTED:
                want_run = False  # the emulator job is skipped for untrusted PRs
            else:
                want_run = True
            if want_run:
                want_on = json.loads(val) if (val and ev in TRUSTED) else "ubuntu-latest"
            else:
                want_on = None
            check("%s/%s %s var=%s" % (wf, jname, ev, vname), (run, runs_on), (want_run, want_on))

# host.yml: the matrix ("none" is not valid for CI_HOST_MATRIX, so it is not a case)
hjob = jobs("host.yml")["test"]
for ev in [e for e in EVENTS if e != "dispatch"]:
    for val, vname in (("", "empty"), (HOST_SELF, "self-hosted matrix")):
        ctx = {"github": event(ev), "vars": {"CI_HOST_MATRIX": val}}
        run = truthy(evaluate(hjob["if"], ctx))
        cells = evaluate(hjob["strategy"]["matrix"]["include"], ctx) if run else None
        runners = None if cells is None else [c["runner"] for c in cells]
        if ev == "pr-fork" and val != "":
            want = (False, None)
        elif ev in TRUSTED:
            want = (True, HOST_DEFAULT if val == "" else [c["runner"] for c in json.loads(val)])
        else:
            want = (True, ["ubuntu-latest"])  # untrusted PR: a single hosted ubuntu cell
        check("host.yml/test %s var=%s" % (ev, vname), (run, runners), want)
check("host.yml runs-on is the matrix cell", hjob["runs-on"].replace(" ", ""), "${{matrix.runner}}")

# Structure guards: the rule stays a whitelist by author and never uses pull_request_target.
for wf in ("android.yml", "emulator.yml", "host.yml", "release-check.yml"):
    text = re.sub(r"(?m)^\s*#.*$", "", (WF / wf).read_text(encoding="utf-8"))
    if "pull_request_target" in text:
        failures.append(wf + ": uses pull_request_target")
    if "dependabot[bot]" in text:
        failures.append(wf + ": a blacklist on dependabot[bot] crept in (the rule is a whitelist by author)")

for label, got, ok in rows:
    print(("ok   " if ok else "FAIL ") + label + " -> " + json.dumps(got))
print("\n%d cells checked, %d failures" % (len(rows), len(failures)))
for f in failures:
    print("FAIL", f)
sys.exit(1 if failures else 0)
