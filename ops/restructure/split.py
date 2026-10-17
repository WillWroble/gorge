#!/usr/bin/env python3
"""Stage "kit" of the repo layout move (docs/decisions/0001-repo-layout.md):
make the helpers an outside module needs public, so four-yay (which imports
gorge) can use them. Reuses restructure.py's move and rewrite passes.

    python3 ops/restructure/split.py
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import restructure as R  # noqa: E402

KIT_MOVES = {
    "internal/testutil": "testkit/testutil",
    "internal/bench": "testkit/bench",
    "internal/paymirror": "testkit/paymirror",
    "internal/cli": "cli",
}


def main():
    if os.path.isdir("testkit/testutil"):
        print("kit stage already applied; nothing to do")
        return
    if R.sh("git", "status", "--porcelain", "--untracked-files=no").strip():
        sys.exit("refusing: tracked files are modified; commit first")
    R.MOVES.clear()
    R.MOVES.update(KIT_MOVES)
    R.FIRST = {p.split("/")[0] for p in R.MOVES}
    R.TOKEN_RE = re.compile(r"(?<![\w.@/$-])((?:\./)?(?:" + "|".join(sorted(R.FIRST, key=len, reverse=True)) + r")(?:/[\w.*-]+)*/?(?:\.\.\.)?)")
    new_to_old = {R.map_path(f): f for f in R.git_files()}
    R.do_moves()
    report, touched = R.rewrite_all(new_to_old)
    os.makedirs(".ds4", exist_ok=True)
    with open(".ds4/split-kit-report.txt", "w") as fh:
        fh.write("\n".join(report) + "\n")
    print(f"kit rewrites: {len(report)}; go files touched: {len(touched)}")


if __name__ == "__main__":
    main()
