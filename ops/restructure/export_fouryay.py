#!/usr/bin/env python3
"""Export the fenced extensions (lab/) and the hosted bots gorge no longer
ships into the four-yay repository (docs/decisions/0001-repo-layout.md).

    python3 ops/restructure/export_fouryay.py <four-yay checkout>

Copies tracked files at their new paths, rewrites Go import paths (gorge/lab/X
-> four-yay/X, moved bots -> four-yay/bots/...), recomputes relative paths
that stay inside the exported set, and reports every relative path that
pointed back into gorge (those need gorge's root at run time). It never
deletes anything in gorge; the removal is a separate gorge commit.
"""
import os
import re
import shutil
import subprocess
import sys

GORGE = "github.com/adams-shaun/gorge"
FY = "github.com/adams-shaun/four-yay"

# gorge path -> four-yay path (longest prefix wins).
EXPORT = {
    "lab": "",
    "bots/azc3": "bots/azc3",
    "bots/azc4": "bots/azc4",
    "bots/azc5": "bots/azc5",
    "bots/castprofile": "bots/castprofile",
    "bots/sbbuiltin": "bots/sbbuiltin",
    "bots/sbsearch": "bots/sbsearch",
    "bots/search": "bots/search",
    "bots/gonet": "bots/gonet",
    "bots/azmcts/clairvoyant": "bots/clairvoyant",
    "bots/spellbench/sbsearch": "bots/spellbench/sbsearch",
    "examples/extagent": "examples/extagent",
    "NOTICE": "NOTICE",
}
for doc in """2026-10-02-xmage-compliance-oracle-design.md 2026-10-05-compliance-level-b.md
2026-10-10-xmage-scenario-port-design.md 2026-10-09-az-c3-hosted.md 2026-10-14-az-c4-hosted.md
2026-10-15-az-c5.md 2026-10-13-draft-events-design.md 2026-10-13-draft-events-e1-plan.md
2026-10-13-draft-events-e3-plan.md 2026-10-14-draft-events-e5-plan.md 2026-10-14-draft-events-e5d-plan.md
2026-10-14-draft-rl-design.md 2026-10-16-pl-3-randomised-lines.md 2026-10-16-pl-4-shaped-value.md
2026-10-16-pl-8-pilot-matched-values.md""".split():
    EXPORT["docs/design/" + doc] = "docs/design/" + doc


def best(p, table):
    b = None
    for o in table:
        if p == o or p.startswith(o + "/"):
            if b is None or len(o) > len(b):
                b = o
    return b


def fy_path(p):
    """gorge path -> four-yay path, or None when it stays in gorge."""
    b = best(p, EXPORT)
    if b is None:
        return None
    rest = p[len(b):].lstrip("/")
    return "/".join(x for x in (EXPORT[b], rest) if x)


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: export_fouryay.py <four-yay checkout>")
    dst = os.path.abspath(sys.argv[1])
    files = subprocess.run(["git", "ls-files", "-z"], capture_output=True, text=True, check=True).stdout.split("\0")
    files = [f for f in files if f]
    moved = {f: fy_path(f) for f in files if fy_path(f) is not None}
    imp = re.compile(re.escape(GORGE) + r"/([A-Za-z0-9_/-]+)")
    rel = re.compile(r'"((?:\.\./)+[^"\n]*)"')
    join = re.compile(r'filepath\.Join\(((?:\s*"[^"\n]*"\s*,?)+)')
    back = []
    for old, new in sorted(moved.items()):
        out = os.path.join(dst, new)
        os.makedirs(os.path.dirname(out), exist_ok=True)
        try:
            src = open(old, encoding="utf-8").read()
        except UnicodeDecodeError:
            shutil.copy2(old, out)
            continue

        def imp_sub(m):
            p = m.group(1)
            q = fy_path(p)
            return FY + ("/" + q if q else "") if q is not None else m.group(0)

        txt = imp.sub(imp_sub, src)
        if old.endswith(".go"):
            od, nd = os.path.dirname(old), os.path.dirname(new)

            def relocate(parts, form):
                target = os.path.normpath(os.path.join(od, *parts))
                if target.startswith(".."):
                    return None
                q = fy_path(target) if target != "." else None
                if q is None:
                    back.append(f"{new}: {form} -> gorge:{target}")
                    return None
                r = os.path.relpath(q or ".", nd or ".")
                return [] if r == "." else r.split("/")

            def rel_sub(m):
                s = m.group(1)
                r = relocate([x for x in s.split("/") if x], f'"{s}"')
                return m.group(0) if r is None else '"' + "/".join(r) + ("/" if s.endswith("/") else "") + '"'

            def join_sub(m):
                lits = re.findall(r'"([^"\n]*)"', m.group(1))
                if not lits or lits[0] != "..":
                    return m.group(0)
                parts = [x for l in lits for x in l.split("/") if x]
                r = relocate(parts, "Join(" + ", ".join(lits) + ")")
                if r is None:
                    return m.group(0)
                tail = "," if m.group(1).rstrip().endswith(",") else ""
                return "filepath.Join(" + ", ".join('"' + x + '"' for x in r) + tail

            txt = join.sub(join_sub, txt)
            txt = rel.sub(rel_sub, txt)
        # repo-relative path tokens: lab/X -> X, moved bots -> new home
        def tok(m):
            p = m.group(2)
            q = fy_path(p.rstrip("/"))
            return m.group(1) + (q + ("/" if p.endswith("/") else "") if q else m.group(2))

        txt = re.sub(r"(^|[\s`'\"(=./])((?:lab/[\w.*-]+|bots/(?:azc3|azc4|azc5|castprofile|sbbuiltin|sbsearch|search|gonet)|bots/azmcts/clairvoyant|bots/spellbench/sbsearch)(?:/[\w.*-]+)*/?)", tok, txt)
        txt = txt.replace("./lab/", "./")
        with open(out, "w", encoding="utf-8") as fh:
            fh.write(txt)
        shutil.copymode(old, out)
    with open(os.path.join(dst, ".export-backrefs.txt"), "w") as fh:
        fh.write("\n".join(back) + "\n")
    print(f"exported {len(moved)} files; {len(back)} relative paths point back into gorge (.export-backrefs.txt)")


if __name__ == "__main__":
    main()
