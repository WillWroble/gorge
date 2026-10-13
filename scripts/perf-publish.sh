#!/usr/bin/env bash
# perf-publish.sh ROWS.jsonl: append schema-1 perf rows (scripts/perf-record.py
# output) to history.jsonl on the orphan branch perf-data, and commit them.
#
# The branch lives in its OWN worktree, so the main checkout's HEAD is never
# touched. If the worktree is missing it is created: from the local perf-data
# branch, else from $PERF_REMOTE/perf-data, else as a fresh orphan branch.
#
#   PERF_DATA_WT  worktree path (default /mnt/sata/gorge-training/perf-data-wt)
#   PERF_REPO     repository (default: the one this script lives in)
#   PERF_REMOTE   remote to fetch from / push to (default origin)
#   PERF_PUSH     1 = push perf-data to PERF_REMOTE after committing; anything
#                 else (the default) commits locally only
#
# history.jsonl is append-only. A row whose (commit, anchor, host.id, scenario,
# metric, run_date) is already present is skipped, so re-publishing the same
# rows is a no-op; every row must parse as JSON with schema 1 or nothing is
# written. The commit message is "perf: <short sha of the rows' commit>".
#
# Nightly example (NOT installed; a systemd user timer):
#
#   # ~/.config/systemd/user/gorge-perf.service
#   [Unit]
#   Description=gorge nightly perf record
#   [Service]
#   Type=oneshot
#   WorkingDirectory=/home/sadams/projects/gorge
#   Environment=PIN=9 REPS=4 BUDGET=600 PERF_PUSH=1
#   ExecStart=/usr/bin/bash -c 'set -e; \
#     anchor=$(cat /mnt/sata/gorge-training/perf-anchor); \
#     head=$(git rev-parse origin/main); out=/mnt/sata/gorge-training/perf-nightly-$head.jsonl; \
#     scripts/enginebench-pair.sh "$anchor" "$head" "$out"; \
#     scripts/perf-record.py --anchor "$anchor" --commit "$head" --host "$(hostname)" "$out" > "$out.rows"; \
#     scripts/perf-publish.sh "$out.rows"'
#
#   # ~/.config/systemd/user/gorge-perf.timer
#   [Timer]
#   OnCalendar=*-*-* 03:30:00
#   Persistent=true
#   [Install]
#   WantedBy=timers.target
#
#   systemctl --user daemon-reload && systemctl --user enable --now gorge-perf.timer
set -euo pipefail
rows=${1:?usage: perf-publish.sh ROWS.jsonl}
rows=$(cd "$(dirname "$rows")" && pwd -P)/$(basename "$rows")
REPO=${PERF_REPO:-$(git -C "$(dirname "$0")" rev-parse --show-toplevel)}
WT=${PERF_DATA_WT:-/mnt/sata/gorge-training/perf-data-wt}
REMOTE=${PERF_REMOTE:-origin}
BRANCH=perf-data

if [ ! -e "$WT/.git" ]; then
	mkdir -p "$(dirname "$WT")"
	if git -C "$REPO" show-ref --verify --quiet "refs/heads/$BRANCH"; then
		git -C "$REPO" worktree add "$WT" "$BRANCH"
	elif git -C "$REPO" fetch --quiet "$REMOTE" "$BRANCH" 2>/dev/null; then
		git -C "$REPO" worktree add -b "$BRANCH" "$WT" FETCH_HEAD
	else
		git -C "$REPO" worktree add --orphan -b "$BRANCH" "$WT"
	fi
fi
cur=$(git -C "$WT" symbolic-ref --short HEAD 2>/dev/null || true)
if [ "$cur" != "$BRANCH" ]; then
	echo "perf-publish: $WT is on '$cur', not $BRANCH; refusing" >&2
	exit 1
fi

added=$(python3 - "$rows" "$WT/history.jsonl" <<'EOF'
import json, os, sys
src, hist = sys.argv[1], sys.argv[2]
def key(r):
    return (r.get("commit"), r.get("anchor"), (r.get("host") or {}).get("id"),
            r.get("scenario"), r.get("metric"), r.get("run_date"))
seen = set()
if os.path.exists(hist):
    with open(hist) as f:
        for line in f:
            if line.strip():
                seen.add(key(json.loads(line)))
new = []
with open(src) as f:
    for n, line in enumerate(f, 1):
        if not line.strip():
            continue
        r = json.loads(line)
        if r.get("schema") != 1:
            sys.exit(f"perf-publish: {src}:{n}: schema {r.get('schema')!r}, want 1")
        if key(r) in seen:
            continue
        seen.add(key(r))
        new.append(json.dumps(r, sort_keys=True))
if new:
    with open(hist, "a") as f:
        f.write("".join(l + "\n" for l in new))
commits = sorted({json.loads(l)["commit"] for l in new})
print(len(new), commits[0][:12] if len(commits) == 1 else ",".join(c[:12] for c in commits))
EOF
)
n=${added%% *}
short=${added#* }
if [ "$n" = 0 ]; then
	echo "perf-publish: nothing new in $rows"
else
	git -C "$WT" add history.jsonl
	git -C "$WT" commit -q -m "perf: $short"
	echo "perf-publish: $n rows -> $WT ($(git -C "$WT" rev-parse --short HEAD))"
fi
# Push even when nothing was new, so an earlier local-only publish catches up.
if [ "${PERF_PUSH:-}" = 1 ]; then
	git -C "$WT" push "$REMOTE" "$BRANCH"
fi
