#!/usr/bin/env python3
"""Contract test of the CI runner-selection rules (docs/ci-self-hosted.md, section 6).

Static mode (default, no network). It does not run GitHub Actions. It walks every
job of every workflow (*.yml, *.yaml) of the repository and requires `runs-on` to
be a closed form: a hosted literal, an expression reading exactly one known
`vars.CI_*` variable, or `matrix.runner` in host.yml. The real `if:`, `runs-on:`
and `matrix.include` expressions are evaluated with a small evaluator of the
subset of the Actions expression language they use (&&, ||, !, ==, !=,
parentheses, string literals, github.*, vars.*, fromJSON, operands returned as in
Actions, case-insensitive string equality), for each event (taken from the real
`on:` of the workflow) x variable case, and compared with the expected table.

--live (read-only, needs `gh`): checks the repository variables and the registered
runners against the label scheme. --strict makes legacy labels a failure.
--self-test: offline tests of this script. --root DIR: repository to check.

Needs PyYAML. Run from anywhere:  python scripts/check-ci-runner-rules.py
Exit code 0 when everything matches, 1 on failures, 2 on a tool error (gh, JSON).
"""
import argparse
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

import yaml

DEFAULT_ROOT = pathlib.Path(__file__).resolve().parent.parent
OWNER, REPO = "owner-account", "owner-account/adbuddy"


class ToolError(Exception):
    """gh or JSON failed: the result is unknown, never a false green."""


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
            if val in ("true", "false", "null"):
                return {"true": True, "false": False, "null": None}[val]
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


# ---- contexts and constants -------------------------------------------------

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


PR_EVENTS = ["pr-owner", "pr-dependabot", "pr-other-bot", "pr-collaborator", "pr-fork"]
TRUSTED = {"pr-owner", "push", "dispatch"}

# Label scheme (docs/ci-self-hosted.md): capability labels, opt-in.
EMU_LABEL, HOSTWIN_LABEL, LINUX_LABEL = "adbuddy-emulator", "adbuddy-host-windows", "adbuddy-linux"
LEGACY_LABELS = ("adbuddy-local", "adbuddy-host")  # forbidden; adbuddy-host-windows is NOT legacy
EMU_SELF = '["self-hosted","windows","adbuddy-emulator"]'
LINUX_SELF = '["self-hosted","linux","adbuddy-linux"]'
HOST_SELF = ('[{"name":"ubuntu-latest","runner":["self-hosted","linux","adbuddy-linux"]},'
             '{"name":"windows-latest","runner":["self-hosted","windows","adbuddy-host-windows"]}]')
HOST_DEFAULT = ["ubuntu-latest", "windows-latest", "macos-latest"]
HOSTED = ("ubuntu-latest", "windows-latest", "macos-latest")
# variable -> (capability label, is the emulator role, self-hosted sample value)
ROLE_VARS = {
    "CI_RUNNER_ANDROID": (LINUX_LABEL, False, LINUX_SELF),
    "CI_RUNNER_RELEASE": (LINUX_LABEL, False, LINUX_SELF),
    "CI_RUNNER_SIGN": (LINUX_LABEL, False, LINUX_SELF),
    "CI_RUNNER_EMULATOR": (EMU_LABEL, True, EMU_SELF),
}
EMU_HEURISTIC = ("emulator.sh", "avdmanager", "ADBUDDY_EMU", "emulator ")
ABSENT_ALLOWED = ()  # CI_* variables allowed to be missing in --live: none today


def workflow_events(doc):
    on = doc.get("on", doc.get(True))  # PyYAML reads the key `on` as True
    if isinstance(on, str):
        names = [on]
    elif isinstance(on, (list, dict)):
        names = list(on)
    else:
        names = []
    evs, unknown = [], []
    for n in names:
        if n == "pull_request":
            evs += PR_EVENTS
        elif n == "push":
            evs.append("push")
        elif n in ("workflow_dispatch", "schedule"):
            if "dispatch" not in evs:
                evs.append("dispatch")
        else:
            unknown.append(n)
    return evs, unknown


def strip_comments(text):
    return re.sub(r"(?m)^\s*#.*$", "", text)


# ---- static mode --------------------------------------------------------------

def static_checks(root):
    """Returns (rows, failures) for the repository at root. No network."""
    wf_dir = pathlib.Path(root) / ".github" / "workflows"
    rows, failures = [], []

    def check(label, got, want):
        ok = got == want
        rows.append((label, got, ok))
        if not ok:
            failures.append("%s: got %r, want %r" % (label, got, want))

    files = sorted(list(wf_dir.glob("*.yml")) + list(wf_dir.glob("*.yaml")))
    if not files:
        failures.append("no workflows found under %s" % wf_dir)
    for path in files:
        wf = path.name
        raw = path.read_text(encoding="utf-8")
        text = strip_comments(raw)
        if "pull_request_target" in text:
            failures.append(wf + ": uses pull_request_target")
        if "dependabot[bot]" in text:
            failures.append(wf + ": a blacklist on dependabot[bot] crept in (the rule is a whitelist by author)")
        for lab in LEGACY_LABELS:  # raw text, comments included
            if re.search(re.escape(lab) + r"(?![\w-])", raw, re.I):
                failures.append("%s: mentions the legacy label %s" % (wf, lab))
        try:
            doc = yaml.safe_load(raw)
            jobs = doc["jobs"]
        except Exception as e:  # noqa: BLE001
            failures.append("%s: cannot parse (%s)" % (wf, e))
            continue
        evs, unknown = workflow_events(doc)
        for jname, job in jobs.items():
            tag = "%s/%s" % (wf, jname)
            ro = job.get("runs-on") if isinstance(job, dict) else None
            blob = json.dumps(job)
            emu_like = any(h in blob for h in EMU_HEURISTIC)
            if not isinstance(ro, str):
                failures.append("%s: runs-on is not a plain string (%r)" % (tag, ro))
                continue
            flat = re.sub(r"\s+", "", ro)
            if ro in HOSTED:
                if emu_like:
                    failures.append("%s: looks like an emulator job but runs on a hosted literal" % tag)
                continue
            if not re.fullmatch(r"\s*\$\{\{.*\}\}\s*", ro, re.S):
                failures.append("%s: runs-on literal %r is not a hosted runner" % (tag, ro))
                continue
            if "self-hosted" in ro:
                failures.append("%s: runs-on has a literal self-hosted label" % tag)
                continue
            if flat == "${{matrix.runner}}":
                if wf != "host.yml":
                    failures.append("%s: matrix.runner is only allowed in host.yml" % tag)
                    continue
                if emu_like:
                    failures.append("%s: looks like an emulator job but does not use CI_RUNNER_EMULATOR" % tag)
                check_host_matrix(tag, job, evs, unknown, check, failures)
                continue
            used = sorted(set(re.findall(r"vars\.(\w+)", ro)))
            if len(used) != 1 or used[0] not in ROLE_VARS:
                failures.append("%s: runs-on must read exactly one known vars.CI_* (found %s)" % (tag, used))
                continue
            var = used[0]
            is_emu = ROLE_VARS[var][1]
            if emu_like and not is_emu:
                failures.append("%s: looks like an emulator job but reads %s, not CI_RUNNER_EMULATOR" % (tag, var))
            if unknown:
                failures.append("%s: events %s are not modelled" % (tag, unknown))
                continue
            check_single(tag, job, var, is_emu, evs, check, failures)
    return rows, failures


def check_single(tag, job, var, is_emu, evs, check, failures):
    selfv = ROLE_VARS[var][2]
    for ev in evs:
        for val, vname in (("", "empty"), (selfv, "self-hosted"), ('"none"', "none")):
            ctx = {"github": event(ev), "vars": {var: val}}
            try:
                run = truthy(evaluate(job.get("if", "${{ true }}"), ctx))
                runs_on = evaluate(job["runs-on"], ctx) if run else None
            except Exception as e:  # noqa: BLE001
                failures.append("%s %s var=%s: cannot evaluate (%s)" % (tag, ev, vname, e))
                continue
            if val == '"none"':
                want_run = False
            elif ev == "pr-fork" and val != "":
                want_run = False  # head-repo guard
            elif is_emu and ev not in TRUSTED:
                want_run = False  # the emulator job is skipped for untrusted PRs
            else:
                want_run = True
            if want_run:
                want_on = json.loads(val) if (val and ev in TRUSTED) else "ubuntu-latest"
            else:
                want_on = None
            check("%s %s var=%s" % (tag, ev, vname), (run, runs_on), (want_run, want_on))


def check_host_matrix(tag, job, evs, unknown, check, failures):
    if unknown:
        failures.append("%s: events %s are not modelled" % (tag, unknown))
        return
    try:
        include = job["strategy"]["matrix"]["include"]
    except Exception:  # noqa: BLE001
        failures.append("%s: matrix.runner without strategy.matrix.include" % tag)
        return
    for ev in evs:
        for val, vname in (("", "empty"), (HOST_SELF, "self-hosted matrix")):
            ctx = {"github": event(ev), "vars": {"CI_HOST_MATRIX": val}}
            try:
                run = truthy(evaluate(job.get("if", "${{ true }}"), ctx))
                cells = evaluate(include, ctx) if run else None
            except Exception as e:  # noqa: BLE001
                failures.append("%s %s var=%s: cannot evaluate (%s)" % (tag, ev, vname, e))
                continue
            runners = None if cells is None else [c["runner"] for c in cells]
            if ev == "pr-fork" and val != "":
                want = (False, None)
            elif ev in TRUSTED:
                want = (True, HOST_DEFAULT if val == "" else [c["runner"] for c in json.loads(val)])
            else:
                want = (True, ["ubuntu-latest"])  # untrusted PR: a single hosted ubuntu cell
            check("%s %s var=%s" % (tag, ev, vname), (run, runners), want)


# ---- live checks (pure) -----------------------------------------------------

def _lower(xs):
    return {str(x).lower() for x in xs}


def _labels(r):
    return _lower(l["name"] if isinstance(l, dict) else l for l in r.get("labels", []))


def _rname(r):
    return "%s(#%s)" % (r.get("name", "?"), r.get("id", "?"))


def live_checks(variables, runners, strict=False):
    """variables: {name: value-string}; runners: list of API runner dicts.
    Returns messages: failures as plain text, warnings prefixed "WARN: "."""
    out = []

    def fail(m):
        out.append(m)

    def warn(m):
        out.append("WARN: " + m)

    for v in list(ROLE_VARS) + ["CI_HOST_MATRIX"]:
        if v not in variables and v not in ABSENT_ALLOWED:
            fail("variable %s is missing" % v)

    legacy_re = re.compile(r"(?<![\w-])(%s)(?![\w-])" % "|".join(map(re.escape, LEGACY_LABELS)), re.I)
    for name, val in variables.items():
        if name.startswith("CI_") and legacy_re.search(str(val)):
            fail("variable %s contains a legacy label" % name)

    labs = [(r, _labels(r)) for r in runners]

    def osname(r):
        return str(r.get("os", "")).lower()

    # capability labels, counting offline runners too
    emu = [r for r, lb in labs if EMU_LABEL in lb]
    if len(emu) != 1:
        fail("%s must be on exactly one runner, found %d (%s)" % (EMU_LABEL, len(emu), ", ".join(map(_rname, emu))))
    for r in emu:
        if osname(r) != "windows":
            fail("%s is on a non-Windows runner %s" % (EMU_LABEL, _rname(r)))
    for r, lb in labs:
        if LINUX_LABEL in lb and osname(r) != "linux":
            fail("%s is on a non-Linux runner %s" % (LINUX_LABEL, _rname(r)))
        if HOSTWIN_LABEL in lb and osname(r) != "windows":
            fail("%s is on a non-Windows runner %s" % (HOSTWIN_LABEL, _rname(r)))
        if "dependabot" in lb:
            fail("runner %s carries the dependabot label" % _rname(r))
        if lb & set(LEGACY_LABELS):
            (fail if strict else warn)("runner %s still carries a legacy label (%s)" % (
                _rname(r), ", ".join(sorted(lb & set(LEGACY_LABELS)))))

    def parse(name):
        raw = variables.get(name)
        if raw is None:
            return None, False
        if str(raw).strip() == "":
            return "", True
        try:
            return json.loads(raw), True
        except ValueError:
            fail("variable %s is not valid JSON: %r" % (name, raw))
            return None, False

    def match(what, req, cap):
        """req: labels asked by a variable; cap: the capability label it needs."""
        if not isinstance(req, list) or not all(isinstance(x, str) for x in req):
            fail("%s: expected a list of labels, got %r" % (what, req))
            return
        rset = _lower(req)
        if cap not in rset:
            fail("%s lacks the capability label %s" % (what, cap))
        matches = [r for r, lb in labs if rset <= lb]
        if not matches:
            fail("%s: no registered runner satisfies %s" % (what, req))
            return
        bad = [r for r in matches if cap not in _labels(r)]
        if bad:
            fail("%s: runners without %s match it: %s" % (what, cap, ", ".join(map(_rname, bad))))
        if not any(str(r.get("status", "")).lower() == "online" for r in matches):
            warn("%s: no online runner satisfies %s" % (what, req))

    for name, (cap, _is_emu, _s) in ROLE_VARS.items():
        val, ok = parse(name)
        if not ok or isinstance(val, str):
            continue  # hosted literal, "none" or empty default: legitimate
        match(name, val, cap)

    val, ok = parse("CI_HOST_MATRIX")
    if ok and not isinstance(val, str):
        if not isinstance(val, list):
            fail("CI_HOST_MATRIX must be a list of cells")
        else:
            for i, cell in enumerate(val):
                run = cell.get("runner") if isinstance(cell, dict) else None
                what = "CI_HOST_MATRIX[%d]" % i
                if isinstance(run, str):
                    if run not in HOSTED:
                        fail("%s: runner %r is not a hosted runner" % (what, run))
                elif isinstance(run, list):
                    low = _lower(run)
                    if "linux" in low:
                        match(what, run, LINUX_LABEL)
                    elif "windows" in low:
                        match(what, run, HOSTWIN_LABEL)
                    else:
                        fail("%s: cannot tell the OS of %r" % (what, run))
                else:
                    fail("%s: unsupported runner %r" % (what, run))
    return out


# ---- gh (read-only) -------------------------------------------------------

def run_gh(args):
    exe = os.environ.get("GH", "gh")
    try:
        p = subprocess.run([exe] + args, capture_output=True, text=True, encoding="utf-8", timeout=120)
    except (OSError, subprocess.SubprocessError) as e:
        raise ToolError("cannot run %s: %s" % (exe, e))
    if p.returncode != 0:
        raise ToolError("gh %s failed (%d): %s" % (" ".join(args[:2]), p.returncode, p.stderr.strip()))
    return p.stdout


def parse_json_stream(text):
    """Concatenated JSON values (what `gh api --paginate --jq '.x[]'` prints)."""
    dec, i, out = json.JSONDecoder(), 0, []
    text = text or ""
    while True:
        while i < len(text) and text[i].isspace():
            i += 1
        if i >= len(text):
            return out
        try:
            obj, i = dec.raw_decode(text, i)
        except ValueError as e:
            raise ToolError("malformed JSON from gh: %s" % e)
        out.append(obj)


def derive_repo(root):
    try:
        url = subprocess.run(["git", "-C", str(root), "remote", "get-url", "origin"],
                             capture_output=True, text=True, encoding="utf-8", timeout=30).stdout.strip()
    except (OSError, subprocess.SubprocessError) as e:
        raise ToolError("cannot read git remote: %s" % e)
    m = re.search(r"[:/]([^/:]+/[^/]+?)(?:\.git)?$", url)
    if not m:
        raise ToolError("cannot derive OWNER/REPO from remote %r; pass --repo" % url)
    return m.group(1)


def fetch_live(repo):
    runners = parse_json_stream(run_gh(["api", "repos/%s/actions/runners" % repo, "--paginate", "--jq", ".runners[]"]))
    total = run_gh(["api", "repos/%s/actions/runners?per_page=1" % repo, "--jq", ".total_count"]).strip()
    try:
        total = int(total)
    except ValueError:
        raise ToolError("bad total_count from gh: %r" % total)
    if total != len(runners):
        raise ToolError("pagination mismatch: total_count=%d but %d runners read" % (total, len(runners)))
    raw = run_gh(["variable", "list", "-R", repo, "--json", "name,value"])
    try:
        variables = {v["name"]: v["value"] for v in json.loads(raw)}
    except (ValueError, KeyError, TypeError) as e:
        raise ToolError("malformed variable list from gh: %s" % e)
    return variables, runners


# ---- self-test --------------------------------------------------------------

def _mk(i, name, os_, labels, status="online"):
    base = ["self-hosted", os_, "X64"]
    return {"id": i, "name": name, "os": os_, "status": status,
            "labels": [{"id": n, "name": lb, "type": "custom"} for n, lb in enumerate(base + labels)]}


def _good():
    variables = {"CI_RUNNER_ANDROID": LINUX_SELF, "CI_RUNNER_RELEASE": LINUX_SELF,
                 "CI_RUNNER_SIGN": LINUX_SELF, "CI_RUNNER_EMULATOR": EMU_SELF,
                 "CI_HOST_MATRIX": HOST_SELF}
    runners = [_mk(1, "win1", "Windows", [EMU_LABEL, HOSTWIN_LABEL]),
               _mk(2, "win2", "Windows", [HOSTWIN_LABEL]),
               _mk(3, "lin1", "Linux", [LINUX_LABEL]),
               _mk(4, "lin2", "Linux", [LINUX_LABEL])]
    return variables, runners


def _fails(msgs):
    return [m for m in msgs if not m.startswith("WARN: ")]


def _warns(msgs):
    return [m for m in msgs if m.startswith("WARN: ")]


def self_test(root):
    results = []

    def t(name, cond):
        results.append((name, bool(cond)))

    v, r = _good()
    t("live: correct scheme passes clean", live_checks(v, r) == [])
    t("live: strict also passes", live_checks(v, r, True) == [])
    t("live: adbuddy-local in a variable fails",
      _fails(live_checks(dict(v, CI_RUNNER_ANDROID='["self-hosted","adbuddy-local"]'), r)))
    t("live: emulator label missing on the runner fails",
      _fails(live_checks(v, [_mk(1, "win1", "Windows", [HOSTWIN_LABEL])] + r[1:])))
    t("live: two runners with adbuddy-emulator fails",
      _fails(live_checks(v, r[:2] + [_mk(3, "lin1", "Linux", [LINUX_LABEL, EMU_LABEL])] + r[3:])))
    t("live: host2 with an upper-case adbuddy-emulator fails",
      _fails(live_checks(v, [r[0], _mk(2, "win2", "Windows", [HOSTWIN_LABEL, "ADBuddy-Emulator"])] + r[2:])))
    m = live_checks(v, [_mk(1, "win1", "Windows", [EMU_LABEL, HOSTWIN_LABEL], "offline")] + r[1:])
    t("live: offline emulator runner still counted: no failure, a warning", not _fails(m) and _warns(m))
    t("live: offline emulator runner plus another emulator runner fails",
      _fails(live_checks(v, r + [_mk(9, "w3", "Windows", [EMU_LABEL], "offline")])))
    off = [r[0], r[1], _mk(3, "lin1", "Linux", [LINUX_LABEL], "offline"), _mk(4, "lin2", "Linux", [LINUX_LABEL], "offline")]
    m = live_checks(v, off)
    t("live: variable without an online runner is a warning, not a failure", not _fails(m) and _warns(m))
    t("live: variable with no runner at all fails", _fails(live_checks(v, r[:2])))
    v3 = dict(v)
    del v3["CI_RUNNER_SIGN"]
    t("live: absent variable fails", any("CI_RUNNER_SIGN" in x for x in _fails(live_checks(v3, r))))
    try:
        m = live_checks(dict(v, CI_HOST_MATRIX="[{not json"), r)
        t("live: malformed JSON is a failure, no exception", any("not valid JSON" in x for x in m))
    except Exception:  # noqa: BLE001
        t("live: malformed JSON is a failure, no exception", False)
    t('live: "none" and a hosted literal are legitimate',
      not _fails(live_checks(dict(v, CI_RUNNER_ANDROID='"none"', CI_RUNNER_RELEASE='"ubuntu-latest"'), r)))
    t("live: dependabot label fails",
      _fails(live_checks(v, r[:3] + [_mk(4, "lin2", "Linux", [LINUX_LABEL, "dependabot"])])))
    t("live: adbuddy-host-windows is not legacy", not any("legacy" in x for x in live_checks(v, r, True)))
    r9 = [r[0], _mk(2, "win2", "Windows", [HOSTWIN_LABEL, "adbuddy-host"])] + r[2:]
    t("live: legacy runner label is a warning in transition", not _fails(live_checks(v, r9)) and _warns(live_checks(v, r9)))
    t("live: legacy runner label fails in strict", _fails(live_checks(v, r9, True)))
    t("live: host-windows on Linux fails",
      _fails(live_checks(v, r[:2] + [_mk(3, "lin1", "Linux", [LINUX_LABEL, HOSTWIN_LABEL])] + r[3:])))
    t("live: emulator variable without the capability fails",
      _fails(live_checks(dict(v, CI_RUNNER_EMULATOR='["self-hosted","windows"]'), r)))
    page1 = json.dumps(r[0]) + "\n" + json.dumps(r[1])
    page2 = json.dumps(r[2]) + "\n" + json.dumps(r[3])
    t("gh: two pages parse into all runners",
      [x["id"] for x in parse_json_stream(page1 + "\n" + page2)] == [1, 2, 3, 4])
    try:
        parse_json_stream('{"id": 1}\n{"id"')
        t("gh: a truncated page is a tool error", False)
    except ToolError:
        t("gh: a truncated page is a tool error", True)

    # static mode: baseline and mutations on a scratch copy of the workflows
    base_rows, base_fail = static_checks(root)
    t("static: baseline evaluates cells", len(base_rows) > 0)
    src = pathlib.Path(root) / ".github" / "workflows"
    emu_files = [p.name for p in sorted(src.glob("*.y*ml")) if "emulator.sh" in p.read_text(encoding="utf-8")]
    t("static: an emulator workflow is found", bool(emu_files))

    def mutated(name, f):
        with tempfile.TemporaryDirectory() as d:
            dst = pathlib.Path(d) / ".github" / "workflows"
            shutil.copytree(src, dst)
            p = dst / name
            s = p.read_text(encoding="utf-8")
            n = f(s)
            if n == s:
                return None
            p.write_text(n, encoding="utf-8")
            return static_checks(d)[1]

    def worse(name, f):
        got = mutated(name, f)
        return got is not None and len(got) > len(base_fail)

    def set_runs_on(value):
        return lambda s: re.sub(r"(?m)^(\s*runs-on:).*$", lambda m_: m_.group(1) + " " + value, s, count=1)

    t("static: adbuddy-local in a workflow fails", worse("android.yml", lambda s: s + "\n# adbuddy-local\n"))
    t("static: adbuddy-host-windows is not legacy",
      mutated("android.yml", lambda s: s + "\n# adbuddy-host-windows\n") == base_fail)
    t("static: legacy adbuddy-host fails", worse("android.yml", lambda s: s + "\n# adbuddy-host\n"))
    t("static: literal self-hosted runs-on fails", worse("android.yml", set_runs_on("[self-hosted, linux]")))
    t("static: literal adbuddy-linux runs-on fails", worse("android.yml", set_runs_on("adbuddy-linux")))
    if emu_files:
        t("static: emulator author guard removed fails",
          worse(emu_files[0], lambda s: s.replace("github.event.pull_request.user.login == github.repository_owner", "true")))
        t("static: emulator job on the wrong variable fails",
          worse(emu_files[0], lambda s: s.replace("CI_RUNNER_EMULATOR", "CI_RUNNER_ANDROID")))
        t("static: emulator job on a hosted literal fails", worse(emu_files[0], set_runs_on("ubuntu-latest")))

    for name, ok in results:
        print(("ok   " if ok else "FAIL ") + name)
    bad = [n for n, ok in results if not ok]
    print("\n%d self-tests, %d failures" % (len(results), len(bad)))
    return 1 if bad else 0


# ---- main ---------------------------------------------------------------

def main(argv=None):
    ap = argparse.ArgumentParser(description="Check the CI runner-selection rules.")
    ap.add_argument("--root", help="repository to check (default: the one holding this file)")
    ap.add_argument("--self-test", action="store_true", help="run the offline tests of this script")
    ap.add_argument("--live", action="store_true",
                    help="also check repo variables and runners through gh (read-only)")
    ap.add_argument("--strict", action="store_true", help="with --live: legacy labels on runners are failures")
    ap.add_argument("--repo", help="OWNER/REPO for --live (default: from git remote origin)")
    args = ap.parse_args(argv)
    if args.strict and not args.live:
        ap.error("--strict needs --live")
    root = pathlib.Path(args.root).resolve() if args.root else DEFAULT_ROOT

    if args.self_test:
        return self_test(root)

    rows, failures = static_checks(root)
    for label, got, ok in rows:
        print(("ok   " if ok else "FAIL ") + label + " -> " + json.dumps(got))
    print("\n%d cells checked, %d failures" % (len(rows), len(failures)))
    for f in failures:
        print("FAIL", f)

    if args.live:
        try:
            repo = args.repo or derive_repo(root)
            print("\n--live%s on %s" % (" --strict" if args.strict else "", repo))
            variables, runners = fetch_live(repo)
        except ToolError as e:
            print("ERROR:", e, file=sys.stderr)
            return 2
        msgs = live_checks(variables, runners, args.strict)
        lf = _fails(msgs)
        for m in msgs:
            print(m if m.startswith("WARN: ") else "FAIL " + m)
        print("%d runners, %d variables, %d live failures, %d warnings" % (
            len(runners), len(variables), len(lf), len(msgs) - len(lf)))
        failures += lf
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
