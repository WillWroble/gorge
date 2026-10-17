#!/usr/bin/env python3
"""Mechanical repo-layout move (docs/decisions/0001-repo-layout.md).

Idempotent on an old-layout tree: run it from the repository root of any
branch that still has the flat layout and it applies the same moves and
rewrites, so a stale branch is "rebased" by re-running it rather than by
resolving rename conflicts by hand.

    python3 ops/restructure/restructure.py [--dry-run]      # Go layout
    python3 ops/restructure/restructure.py --stage=ops      # then ops/ + lab/scripts

Passes, in order:
  0. the seat split: seat/ keeps only the Seat contract; the bot
     implementations become bots/heuristic; BoardSeat moves to botpolicy.
  1. git mv every entry of MOVES (subpackages before their parents).
  2. rewrite Go import paths (exact, segment-aware longest prefix).
  3. rewrite relative paths in Go source ("../x" literals and
     filepath.Join("..", ...) literal runs), recomputed from each file's old
     and new directory.
  4. rewrite repo-relative path tokens in text files, only where the token
     names a path that existed in the old tree.
Afterwards run goimports on the touched Go files and build every tag set.
"""
import os
import re
import subprocess
import sys

MOD = "github.com/adams-shaun/gorge"
DRY = "--dry-run" in sys.argv

# old path -> new path. Longest prefix wins in path mapping; execution order
# is by descending depth so a subpackage leaves before its parent moves.
MOVES = {
    # engine core
    "cards": "engine/cards",
    "state": "engine/state",
    "decision": "engine/decision",
    "events": "engine/events",
    "effects": "engine/effects",
    "rules": "engine/rules",
    "view": "engine/view",
    "replay": "engine/replay",
    "deck": "engine/deck",
    "seat": "engine/seat",  # after pass 0 it holds only the contract
    # reference server
    "host": "server/host",
    "protocol": "server/protocol",
    "internal/manabrew": "server/manabrew",
    "internal/divergence": "server/divergence",
    # bots: policy library and hosted-bot inference runtime
    "botpolicy": "bots/botpolicy",
    "internal/azmcts": "bots/azmcts",
    "internal/gonet": "bots/gonet",
    "internal/policynet": "bots/policynet",
    "internal/traceboard": "bots/traceboard",
    "internal/searchseat": "bots/searchseat",
    "internal/searchprobe": "bots/searchprobe",
    "internal/spellbench/builtins": "bots/spellbench/builtins",
    "internal/spellbench/payexec": "bots/spellbench/payexec",
    "internal/spellbench/sbsearch": "bots/spellbench/sbsearch",
    # lab: draft / limited / events
    "limited": "lab/draft/limited",
    "event": "lab/draft/event",
    "eventhost": "lab/draft/eventhost",
    "draftlab": "lab/draft/draftlab",
    # lab: compliance oracles
    "compliance": "lab/oracle/compliance",
    "validation": "lab/oracle/validation",
    "tools/xmageoracle": "lab/oracle/xmageoracle",
    # lab: training and research
    "internal/policynet/v2front": "lab/training/v2front",
    "internal/spellbench": "lab/training/spellbench",
    "internal/gamepool": "lab/training/gamepool",
    "internal/mzenc": "lab/training/mzenc",
    "internal/inferbroker": "lab/training/inferbroker",
    "internal/searchbench": "lab/training/searchbench",
    "internal/corpuscov": "lab/training/corpuscov",
    "internal/memcap": "lab/training/memcap",
    "experimental": "lab/training/experimental",
    "contracts": "lab/contracts",
}
LAB_CMDS = """policytune policytrain exitloop searchteacher searchprobe searchbench
encablate dzgorge dzbroker cardval corpuscov traindash sbagent sbv1agent
sbv2engine sbv2local sbv2shadow kshadowcheck draftrl draftsim labdemo extagent
compliance oraclediff xmageport oraclepacket botbench""".split()
for c in LAB_CMDS:
    MOVES["cmd/" + c] = "lab/cmd/" + c

DELETE = ["cmd/autopayaudit", "cmd/manamodelstats", "deck.list"]

# Stage "ops": agent-pipeline operations under ops/, research scripts under
# lab/scripts/; scripts/ keeps what a contributor or CI runs.
DEV_SCRIPTS = {"agent-worktree.sh", "smoke.sh", "smoke-perf.sh", "ci_shard.sh",
               "docs_sets.py", "enginebench-build.sh", "enginebench-lib.sh",
               "enginebench-pair.sh", "enginebench-profile.sh",
               "enginebench-summarize.py", "enginebench-verify.sh"}
LAB_SCRIPTS = {"compliance-pass.sh", "compliance-summary.py", "forge-oracle-run.sh",
               "forge-oracle-setup.sh", "forge-pass.sh", "sb-gauntlet.sh",
               "spellbench-rate.py", "spellbench-fdn-catalog.py", "m1b-distill.sh",
               "m1b-winrate.py", "mtgjson_trim.py", "searchbench", "encablate",
               "playlines", "spellbench-arena"}
OPS_DELETE = """compliance-printed.py forge-oracle-test-cast.sh forge-oracle-test-setup.sh
xmage-oracle-test-driver-contract.sh xmage-oracle-test-forge-chunks.sh az-stage0.sh
m1b-paired.py manamodel_ab.sh manamodel_ab_report.py manamodel_report.py
encablate/summarize_keep.py searchbench/plot.py searchbench/report_tables.py
tests/test_perf_record.py spellbench-arena/agent_bridge_v1.rs spellbench-arena/h2h.py
spellbench-arena/mkcfg.py spellbench-arena/remix_decks.py spellbench-arena/replay.py
spellbench-arena/view.sh spellbench-arena/wl.py""".split()


def ops_moves():
    m = {"orchestrator": "ops/orchestrator", "deploy": "ops/deploy"}
    for name in sorted({f.split("/")[1] for f in git_files() if f.startswith("scripts/")}):
        if name in DEV_SCRIPTS:
            continue
        if name in LAB_SCRIPTS or name.startswith("xmage-oracle-"):
            m["scripts/" + name] = "lab/scripts/" + name
        else:
            m["scripts/" + name] = "ops/scripts/" + name
    return m


# Exported identifiers that leave package seat for bots/heuristic.
SEAT_IMPL = """Bot NewBot BotSource NewBotOn NewLethalPressureBot NewCombinedLethalBot
NewBlocksBot NewExploreBot NewAttackSimBot NewCastProfileBot
NewCastProfileBotWithWeights BoardFromView PolicyNetBot AdmissionNames
PolicyNetDecision PolicyNetKindNames ParsePolicyNetKinds NewPolicyNetBot
NewPolicyNetBotKinds PriorityInDistribution MarkBotPicks""".split()

# Text never rewritten: historical records (parked out of the tree) and data.
SKIP_TEXT = re.compile(r"^(ops/restructure/|docs/superpowers/|docs/decisions/|docs/agents/heads-history\.md|web/node_modules/|.*\.(png|jpg|gif|ico|gz|gob|zip|woff2?|ttf|wasm|bin|dzgw|npz|pt)$)")


def sh(*a, check=True):
    return subprocess.run(a, check=check, capture_output=True, text=True).stdout


def git_files():
    return [f for f in sh("git", "ls-files", "-z").split("\0") if f]


OLD_FILES = set(git_files())
OLD_DIRS = {""}
for f in OLD_FILES:
    d = os.path.dirname(f)
    while d not in OLD_DIRS:
        OLD_DIRS.add(d)
        d = os.path.dirname(d)


def old_exists(p):
    p = p.rstrip("/")
    return p in OLD_FILES or p in OLD_DIRS


def map_path(p):
    """Repo-relative old path -> new path (segment-aware longest prefix)."""
    best = None
    for old in MOVES:
        if p == old or p.startswith(old + "/"):
            if best is None or len(old) > len(best):
                best = old
    if best is None:
        return p
    return MOVES[best] + p[len(best):]


def already_moved():
    return os.path.isdir("engine/rules")


# ---- pass 0: seat split --------------------------------------------------
def seat_split():
    os.makedirs("bots/heuristic", exist_ok=True)
    for f in sorted(os.listdir("seat")):
        if f == "seat.go":
            continue
        run_mv(f"seat/{f}", f"bots/heuristic/{f}")
        path = f"bots/heuristic/{f}"
        if path.endswith(".go") and not DRY:
            src = open(path).read()
            src = re.sub(r"^package seat_test\b", "package heuristic_test", src, flags=re.M)
            src = re.sub(r"^package seat\b", "package heuristic", src, flags=re.M)
            open(path, "w").write(src)
    if DRY:
        return
    # BoardSeat moves to botpolicy, which owns Board.
    s = open("seat/seat.go").read()
    m = re.search(r"\n((?://[^\n]*\n)*)type BoardSeat interface \{.*?\n\}\n", s, re.S)
    assert m, "BoardSeat not found in seat/seat.go"
    s = s[: m.start()] + "\n" + s[m.end():]
    open("seat/seat.go", "w").write(s)
    decl = m.group(0).lstrip("\n").replace("botpolicy.Board", "Board")
    open("botpolicy/boardseat.go", "w").write(
        "package botpolicy\n\nimport (\n\t\"context\"\n\n\t\"" + MOD + "/decision\"\n)\n\n" + decl)
    subprocess.run(["git", "add", "botpolicy/boardseat.go"], check=True)
    ident = re.compile(r"\bseat\.(" + "|".join(SEAT_IMPL) + r")\b")
    for f in git_files() + [p for p in walk_go("bots/heuristic")]:
        if not f.endswith(".go") or not os.path.exists(f):
            continue
        src = open(f).read()
        new = ident.sub(r"heuristic.\1", src)
        new = re.sub(r"\bseat\.BoardSeat\b", "botpolicy.BoardSeat", new)
        if f.startswith("botpolicy/"):
            new = new.replace("botpolicy.BoardSeat", "BoardSeat")
        if new != src:
            if "heuristic." in new and not f.startswith("bots/heuristic/"):
                new = add_import(new, MOD + "/bots/heuristic")
            if "botpolicy.BoardSeat" in new:
                new = add_import(new, MOD + "/botpolicy")
            open(f, "w").write(new)


def walk_go(d):
    for root, _, files in os.walk(d):
        for f in files:
            if f.endswith(".go"):
                yield os.path.join(root, f)


def add_import(src, path):
    q = '"' + path + '"'
    if q in src:
        return src
    m = re.search(r"^import \(\n", src, re.M)
    if m:
        return src[: m.end()] + "\t" + q + "\n" + src[m.end():]
    m = re.search(r"^package \w+\n", src, re.M)
    return src[: m.end()] + "\nimport " + q + "\n" + src[m.end():]


# ---- pass 1: moves -------------------------------------------------------
def run_mv(src, dst):
    if DRY:
        print(f"mv {src} -> {dst}")
        return
    os.makedirs(os.path.dirname(dst) or ".", exist_ok=True)
    subprocess.run(["git", "mv", src, dst], check=True)


def do_moves():
    for old in sorted(MOVES, key=lambda p: -p.count("/")):
        if os.path.exists(old):
            run_mv(old, MOVES[old])
    for d in DELETE:
        if os.path.exists(d) and not DRY:
            subprocess.run(["git", "rm", "-rqf", d], check=True)
    # tools/ is empty once xmageoracle leaves.


# ---- pass 2-4: rewrites --------------------------------------------------
IMPORT_RE = re.compile(re.escape(MOD) + r"/([A-Za-z0-9_./-]+)")
FIRST = {p.split("/")[0] for p in MOVES}
TOKEN_RE = re.compile(r"(?<![\w.@/$-])((?:\./)?(?:" + "|".join(sorted(FIRST, key=len, reverse=True)) + r")(?:/[\w.*-]+)*/?(?:\.\.\.)?)")


def rewrite_import(m):
    rest = m.group(1)
    # A linker -X target or a stack frame names pkg.Symbol: map the package.
    sym = ""
    mm = re.match(r"^(.*?)(\.[A-Za-z_]\w*.*)$", rest)
    if mm and "/" not in mm.group(2):
        rest, sym = mm.group(1), mm.group(2)
    return MOD + "/" + map_path(rest) + sym


def norm(p):
    p = os.path.normpath(p)
    return "" if p == "." else p


def relocate(old_dir, new_dir, parts):
    """parts: path components relative to old_dir. Return new components or None."""
    target = norm(os.path.join(old_dir, *parts))
    if target.startswith(".."):
        return None  # outside the repository
    new_target = map_path(target) if target else ""
    rel = os.path.relpath(new_target or ".", new_dir or ".")
    new_parts = [] if rel == "." else rel.split("/")
    return None if new_parts == list(parts) else new_parts


GO_REL_RE = re.compile(r'(filepath\.Join\([^()\n]*\))|"((?:\.\./)+[^"\n]*)"')
ARG_RE = re.compile(r'\s*("(?:[^"\\\n]|\\.)*"|[^,]+)\s*(?:,|$)')


def is_lit(a):
    return len(a) >= 2 and a[0] == '"' and a[-1] == '"'


def rewrite_go_rel(src, old_dir, new_dir, path, report):
    def join_call(text):
        inner = text[len("filepath.Join("):-1]
        args = [a.strip() for a in ARG_RE.findall(inner) if a.strip()]
        out, i, changed = [], 0, False
        while i < len(args):
            if not is_lit(args[i]):
                out.append(args[i]); i += 1; continue
            j = i
            while j < len(args) and is_lit(args[j]):
                j += 1
            lits = [a[1:-1] for a in args[i:j]]
            parts = [seg for l in lits for seg in l.split("/") if seg]
            new = None
            if i == 0 and parts and parts[0] == "..":
                new = relocate(old_dir, new_dir, parts)
            elif (parts and parts[0] in FIRST and old_exists("/".join(parts))
                  and (len(parts) > 1 or (i > 0 and re.search(r"(?i)root|repo", args[i - 1])))):
                m = map_path("/".join(parts))
                if m != "/".join(parts):
                    new = m.split("/")
            if new is None:
                out += args[i:j]
            else:
                out += ['"' + p + '"' for p in new]
                changed = True
            i = j
        if not changed:
            return text
        res = "filepath.Join(" + ", ".join(out) + ")"
        report.append(f"{path}: {text} -> {res}")
        return res

    def sub(m):
        if m.group(1):
            return join_call(m.group(1))
        s = m.group(2)
        new = relocate(old_dir, new_dir, [x for x in s.split("/") if x])
        if new is None:
            return m.group(0)
        out = "/".join(new) + ("/" if s.endswith("/") else "")
        report.append(f'{path}: "{s}" -> "{out}"')
        return '"' + out + '"'

    return GO_REL_RE.sub(sub, src)


SHELLVAR_RE = re.compile(r"((?:\$\{?\w+\}?|\$\([^()\n]*\))/)([\w.-]+(?:/[\w.*-]+)*/?)")


def map_token(tok, path, report):
    lead = "./" if tok.startswith("./") else ""
    body = tok[len(lead):]
    suffix = ""
    for s in ("/...", "/"):
        if body.endswith(s):
            body, suffix = body[: -len(s)], s
            break
    if "/" not in body and not lead and not suffix:
        return tok
    probe = body.split("*")[0].rstrip("/") if "*" in body else body
    if "*" in body and "/" in probe:
        probe = probe.rsplit("/", 1)[0]
    if not old_exists(probe):
        return tok
    new = map_path(body)
    if new == body:
        return tok
    report.append(f"{path}: {tok} -> {lead}{new}{suffix}")
    return lead + new + suffix


def rewrite_tokens(src, path, report):
    src = TOKEN_RE.sub(lambda m: map_token(m.group(1), path, report), src)
    src = SHELLVAR_RE.sub(lambda m: m.group(1) + (map_token(m.group(2), path, report)
                          if m.group(2).split("/")[0] in FIRST else m.group(2)), src)
    if os.path.basename(path) == ".gitignore":
        src = re.sub(r"(?m)^(!?/)([\w.-]+(?:/[\w.*-]+)*/?)",
                     lambda m: m.group(1) + map_token(m.group(2), path, report), src)
    return src


# module + "/rules" style fragments (archtest builds package paths this way).
FRAG_RE = re.compile(r'(\bmodule\s*\+\s*)"/([\w.-]+(?:/[\w.-]+)*)(/?)"')  # only module + "/pkg": URL paths share the shape


def frag(m, f, report):
    p = m.group(2)
    if p not in OLD_DIRS:
        return m.group(0)
    q = map_path(p)
    if q == p:
        return m.group(0)
    report.append(f"{f}: /{p} -> /{q}")
    return m.group(1) + '"/' + q + m.group(3) + '"'


def rewrite_all(new_to_old):
    report = []
    touched_go = []
    for f in git_files():
        if not os.path.isfile(f) or SKIP_TEXT.match(f):
            continue
        try:
            src = open(f, encoding="utf-8").read()
        except (UnicodeDecodeError, IsADirectoryError):
            continue
        new = IMPORT_RE.sub(rewrite_import, src)
        old = new_to_old.get(f, f)
        if f.endswith(".go"):
            new = rewrite_go_rel(new, os.path.dirname(old), os.path.dirname(f), f, report)
            new = FRAG_RE.sub(lambda m: frag(m, f, report), new)
        if os.path.basename(f) == "go.mod":
            new = re.sub(r"(=>\s*)(\.\.[./]*)", lambda m: m.group(1) + (
                "/".join(relocate(os.path.dirname(old), os.path.dirname(f), m.group(2).split("/")) or m.group(2).split("/"))), new)
        if not f.startswith("web/"):
            new = rewrite_tokens(new, f, report)
        if new != src:
            if not DRY:
                open(f, "w", encoding="utf-8").write(new)
            if f.endswith(".go"):
                touched_go.append(f)
    return report, touched_go


def deepen_roots(path):
    """A script that moved one directory deeper finds the root one level up."""
    src = open(path).read()
    new = re.sub(r'(dirname "(?:\$0|\$\{BASH_SOURCE\[0\]\})"\)(?:/\.\.)+)(?=["/)])', r"\1/..", src)
    new = re.sub(r'(\$here(?:/\.\.)+)(?=["/)])', r"\1/..", new)
    new = new.replace("os.path.dirname(os.path.dirname(os.path.abspath(__file__)))",
                      "os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))")
    if new != src:
        open(path, "w").write(new)
        return True
    return False


def main_ops():
    if os.path.isdir("ops/orchestrator"):
        print("ops stage already applied; nothing to do")
        return
    for d in OPS_DELETE:
        if os.path.exists("scripts/" + d):
            subprocess.run(["git", "rm", "-rqf", "scripts/" + d], check=True)
    MOVES.clear()
    MOVES.update(ops_moves())
    global FIRST, TOKEN_RE
    FIRST = {p.split("/")[0] for p in MOVES}
    TOKEN_RE = re.compile(r"(?<![\w.@/$-])((?:\./)?(?:" + "|".join(sorted(FIRST, key=len, reverse=True)) + r")(?:/[\w.*-]+)*/?(?:\.\.\.)?)")
    new_to_old = {map_path(f): f for f in git_files()}
    do_moves()
    report, touched = rewrite_all(new_to_old)
    deepened = [f for f, o in new_to_old.items()
                if os.path.isfile(f) and f.count("/") > o.count("/") and f.endswith((".sh", ".py")) and deepen_roots(f)]
    os.makedirs(".ds4", exist_ok=True)
    with open(".ds4/restructure-ops-report.txt", "w") as fh:
        fh.write("\n".join(report) + "\n" + "\n".join("deepened " + d for d in deepened) + "\n")
    print(f"ops rewrites: {len(report)}, root-depth fixes: {len(deepened)} (.ds4/restructure-ops-report.txt)")


def main():
    if sh("git", "status", "--porcelain", "--untracked-files=no").strip():
        sys.exit("refusing: tracked files are modified; commit first")
    if "--stage=ops" in sys.argv:
        return main_ops()
    if already_moved():
        print("tree already has the new layout; nothing to do")
        return
    # Record each surviving file's pre-move path before anything moves.
    seat_split()
    pre = git_files() if not DRY else sorted(OLD_FILES)
    new_to_old = {}
    for f in pre:
        # bots/heuristic files came from seat/
        origin = "seat/" + f[len("bots/heuristic/"):] if f.startswith("bots/heuristic/") else f
        new_to_old[map_path(f)] = origin
    do_moves()
    report, touched = rewrite_all(new_to_old)
    os.makedirs(".ds4", exist_ok=True)
    with open(".ds4/restructure-report.txt", "w") as fh:
        fh.write("\n".join(report) + "\n")
    with open(".ds4/restructure-touched-go.txt", "w") as fh:
        fh.write("\n".join(touched) + "\n")
    print(f"rewrites: {len(report)} (see .ds4/restructure-report.txt); go files touched: {len(touched)}")


if __name__ == "__main__":
    main()
