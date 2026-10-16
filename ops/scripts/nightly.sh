#!/usr/bin/env bash
# nightly.sh — the T3 nightly tier (docs/agents/test-throughput-2026-10-12.md §4).
#
# The audit found a set of lanes nothing ran: `make fuzz`, `make conformance`,
# `make test-manabrew`, `make test-spellbench-bridge`, a -race pass, and a
# perf-tracking record. This script runs them once, sequentially, against a
# FRESH DETACHED worktree of origin/main, then removes that worktree.
#
#   ops/scripts/nightly.sh                               # every default step
#   NIGHTLY_STEPS=fuzz,race ops/scripts/nightly.sh       # a subset, in that order
#   NIGHTLY_DRY_RUN=1 ops/scripts/nightly.sh             # print the plan, run nothing
#   make nightly                                     # the same, from the Makefile
#
# Steps (default order): fuzz conformance manabrew spellbench race perf.
# Opt-in only (NIGHTLY_STEPS must name it): cover (`make cover`: the whole
# suite again with coverage, so it is not in the default set).
#
# Every step:
#   - takes a lane of the shared HEAVY pool (ops/scripts/heavy_lock.sh) as ordinary
#     PREEMPTIBLE work: not gate class (-g), so it defers while a broker gate
#     bracket is open and, once running, yields by CPUWeight (the heavy_lock.sh
#     default `weight` mode -- never SIGSTOP, so `go test -timeout` deadlines
#     stay honest). It waits at most NIGHTLY_LANE_WAIT s for a lane; no lane
#     in time is a FAIL ("no heavy lane"), never a silent pass.
#   - first waits (at most NIGHTLY_PM_WAIT s) while the post-merge full suite
#     is mid-run (.ds4/postmerge-batch.log's last FULL start has no GREEN/RED
#     after it), so the nightly does not start a heavy step on top of the
#     suite that pushes main. The lanes already bound concurrency; this keeps
#     the two out of each other's wall clocks where it can.
#   - runs in its OWN `systemd-run --user --scope` in gorge-heavy.slice with a
#     MemoryMax, a CPUQuota, CPUWeight=20, the operator's per-test runtime caps
#     (GOMEMLIMIT=3GiB, GOMAXPROCS=4, GOFLAGS="-p=2 -trimpath") and a coreutils
#     `timeout` wall cap.
#   - logs to .ds4/nightly/<date>/<step>.log in the MAIN checkout, and appends
#     one line to .ds4/nightly.log:
#         <iso time> <sha12> <step> PASS|FAIL|SKIP <wall>s <log> [note]
# A failed step does not stop the run; the script exits 1 if any step FAILed.
#
# The race step runs one package at a time (Makefile:30-32: the race
# detector's shadow memory sits outside GOMEMLIMIT, so -race is never
# parallel across packages) over NIGHTLY_RACE_PKGS, each with
# `-race -p=1 -count=1 -timeout NIGHTLY_RACE_TIMEOUT`. server/host/ shrinks its own
# fixture under -race (server/host/fixture_race_test.go, raceEnabled).
#
# The perf step runs only when PERF_NIGHTLY=1, and only if the tag
# perf-anchor-1 exists (otherwise SKIP with a note): scripts/enginebench-pair.sh
# <anchor> <head> -> ops/scripts/perf-record.py -> ops/scripts/perf-publish.sh, with
# PERF_PUSH (and the enginebench knobs REPS, BUDGET, PIN, ...) passed through.
# The bench scopes itself (enginebench-lib.sh heavy(), MEM_MAX), so this step
# holds a lane but adds no outer scope.
#
# Knobs (environment):
#   NIGHTLY_STEPS        comma list of steps (default: the six above)
#   NIGHTLY_DRY_RUN=1    print the plan and exit 0; touches nothing
#   NIGHTLY_REF          what to test (default origin/main, fetched first)
#   NIGHTLY_KEEP_WT=1    keep the worktree after the run (for debugging)
#   NIGHTLY_LANE_WAIT    seconds to wait for a heavy lane per step (default 7200)
#   NIGHTLY_PM_WAIT      seconds to wait out a running post-merge suite (default 3600)
#   NIGHTLY_SLICE        systemd slice (default gorge-heavy.slice)
#   NIGHTLY_RACE_PKGS    space list (default below)
#   NIGHTLY_RACE_TIMEOUT go test -timeout per race package (default 45m)
#   NIGHTLY_RACE_MEM     race scope MemoryMax (default 10G)
#   PERF_NIGHTLY=1       enable the perf step; PERF_PUSH=1 pushes perf-data
set -uo pipefail

SELF_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
COMMON=$(git -C "$SELF_DIR" rev-parse --path-format=absolute --git-common-dir)
ROOT=$(dirname "$COMMON") # the main checkout, even when run from a worktree copy
. "$SELF_DIR/heavy_lock.sh"

DEFAULT_STEPS=fuzz,conformance,manabrew,spellbench,race,perf
STEPS=${NIGHTLY_STEPS:-$DEFAULT_STEPS}
DRY=${NIGHTLY_DRY_RUN:-0}
REF=${NIGHTLY_REF:-origin/main}
LANE_WAIT=${NIGHTLY_LANE_WAIT:-7200}
PM_WAIT=${NIGHTLY_PM_WAIT:-3600}
SLICE=${NIGHTLY_SLICE:-gorge-heavy.slice}
RACE_PKGS=${NIGHTLY_RACE_PKGS:-./state ./engine/decision ./engine/events ./engine/effects ./engine/rules/... ./engine/replay ./engine/view ./engine/seat ./server/host ./engine/cards}
RACE_TIMEOUT=${NIGHTLY_RACE_TIMEOUT:-45m}
RACE_MEM=${NIGHTLY_RACE_MEM:-10G}
SUMMARY=$ROOT/.ds4/nightly.log
PM_LOG=$ROOT/.ds4/postmerge-batch.log
STAMP=$(date +%F)
LOGDIR=$ROOT/.ds4/nightly/$STAMP
[ -e "$LOGDIR" ] && [ "$DRY" != 1 ] && LOGDIR=$ROOT/.ds4/nightly/${STAMP}T$(date +%H%M%S)
WT=$ROOT/.worktrees/nightly-run-$(date +%Y%m%dT%H%M%S)

# The per-test runtime caps (AGENTS.md test budget), inside every step scope.
GOENV=(GOMEMLIMIT=3GiB GOMAXPROCS=4 GOGC=200 "GOFLAGS=-p=2 -trimpath")

say() { printf 'nightly: %s\n' "$*"; }

# step_spec NAME: sets MEM QUOTA CAP and CMD (an array) for one step, or
# returns 1 for an unknown step. CMD runs with cwd = the nightly worktree.
step_spec() {
	MEM=10G QUOTA=800% CAP=60m DESC=""
	case $1 in
	fuzz)
		CAP=90m
		CMD=(make fuzz "GO_TEST_FLAGS=-p=2 -timeout 60m")
		;;
	conformance)
		CAP=30m
		CMD=(make conformance "GO_TEST_FLAGS=-p=2 -timeout 20m")
		;;
	manabrew)
		CAP=60m
		CMD=(make test-manabrew "GO_TEST_FLAGS=-p=2 -timeout 30m")
		;;
	spellbench)
		CAP=45m
		CMD=(make test-spellbench-bridge "GO_TEST_FLAGS=-p=2 -timeout 30m")
		;;
	race)
		MEM=$RACE_MEM QUOTA=400% CAP=240m
		# One package at a time; every package runs even after one fails.
		CMD=(bash -c 'rc=0; for p in "$@"; do
			echo "== go test -race $p"; s=$(date +%s)
			if GOFLAGS=-trimpath go test -race -p=1 -count=1 -timeout "$0" "$p"; then
				echo "== PASS $p ($(( $(date +%s) - s ))s)"
			else
				echo "== FAIL $p ($(( $(date +%s) - s ))s)"; rc=1
			fi
		done; exit $rc' "$RACE_TIMEOUT" $RACE_PKGS)
		DESC="per package, sequentially: go test -race -p=1 -count=1 -timeout $RACE_TIMEOUT <pkg> for $RACE_PKGS"
		;;
	cover)
		MEM=12G QUOTA=800% CAP=120m
		CMD=(make cover "GO_TEST_FLAGS=-p=2 -timeout 30m" "COVER_OUT=$LOGDIR/coverage.out")
		;;
	perf)
		CAP=120m
		CMD=(perf) # handled by run_perf; no outer scope (the bench scopes itself)
		;;
	*) return 1 ;;
	esac
}

# postmerge_busy: the post-merge full suite is mid-run -- its log's last
# FULL start/GREEN/RED line is a FULL start, and the log moved within 2 h (a
# killed loop must not hold the nightly forever).
postmerge_busy() {
	[ -f "$PM_LOG" ] || return 1
	[ $(($(date +%s) - $(stat -c %Y "$PM_LOG"))) -le 7200 ] || return 1
	/usr/bin/grep -E ' (FULL start|GREEN|RED|STILL RED) ' "$PM_LOG" | tail -n1 | /usr/bin/grep -q ' FULL start '
}

wait_postmerge() {
	local deadline=$(($(date +%s) + PM_WAIT))
	while postmerge_busy; do
		[ "$(date +%s)" -lt "$deadline" ] || {
			say "post-merge suite still running after ${PM_WAIT}s; starting anyway (lanes + CPUWeight bound the overlap)"
			return 0
		}
		sleep 30
	done
}

record() { # record STEP STATUS WALL LOG [NOTE]
	local line
	line="$(date +%FT%T%z) ${SHA:0:12} $1 $2 $3s $4${5:+ $5}"
	printf '%s\n' "$line" >>"$SUMMARY"
	say "$line"
}

# run_scoped NAME LOG: run CMD in its own capped scope holding a heavy lane.
run_scoped() {
	local name=$1 log=$2
	(
		cd "$WT" || exit 2
		gorge_heavy_run -w "$LANE_WAIT" -- \
			systemd-run --user --scope -q --slice="$SLICE" --unit="gorge-nightly-$name-$$" \
			-p "MemoryMax=$MEM" -p "CPUQuota=$QUOTA" -p CPUWeight=20 \
			timeout -k 60 "$CAP" env "${GOENV[@]}" "${CMD[@]}"
	) >"$log" 2>&1
}

# run_perf LOG: the perf-tracking record (PERF_NIGHTLY=1 only).
run_perf() {
	local log=$1 anchor
	anchor=$(git -C "$WT" rev-parse -q --verify 'perf-anchor-1^{commit}') || return 99
	(
		cd "$WT" || exit 2
		gorge_heavy_run -w "$LANE_WAIT" -- timeout -k 60 "$CAP" bash -c '
			set -euo pipefail
			anchor=$1 head=$2 dir=$3
			scripts/enginebench-pair.sh "$anchor" "$head" "$dir/perf-pair.jsonl"
			python3 ops/scripts/perf-record.py --anchor "$anchor" --commit "$head" --host "$(hostname)" \
				"$dir/perf-pair.jsonl" >"$dir/perf-rows.jsonl"
			ops/scripts/perf-publish.sh "$dir/perf-rows.jsonl"' _ "$anchor" "$SHA" "$LOGDIR"
	) >"$log" 2>&1
}

IFS=',' read -r -a step_list <<<"$STEPS"
for s in "${step_list[@]}"; do
	step_spec "$s" || {
		printf 'nightly: unknown step %q (known: fuzz conformance manabrew spellbench race perf cover)\n' "$s" >&2
		exit 2
	}
done

# ---- dry run: the plan, nothing touched ------------------------------------
if [ "$DRY" = 1 ]; then
	SHA=$(git -C "$ROOT" rev-parse -q --verify "$REF^{commit}" 2>/dev/null || echo "<unresolved $REF>")
	say "DRY RUN -- nothing is fetched, created or run"
	say "ref       $REF -> $SHA (fetched fresh at run time)"
	say "worktree  $WT (detached, .cards -> $ROOT/.cards; removed after)"
	say "logs      $LOGDIR/<step>.log; summary -> $SUMMARY"
	say "lane      heavy pool $GORGE_HEAVY_LOCK x$GORGE_HEAVY_LANES, wait <= ${LANE_WAIT}s, yield=$GORGE_HEAVY_YIELD (not gate class)"
	say "postmerge wait <= ${PM_WAIT}s while a full suite is mid-run (busy now: $(postmerge_busy && echo yes || echo no))"
	for s in "${step_list[@]}"; do
		step_spec "$s"
		if [ "$s" = perf ]; then
			if [ "${PERF_NIGHTLY:-}" != 1 ]; then
				say "step perf: SKIP (PERF_NIGHTLY!=1)"
			elif a=$(git -C "$ROOT" rev-parse -q --verify 'perf-anchor-1^{commit}'); then
				say "step perf: cap $CAP; enginebench-pair.sh ${a:0:12} <head> -> perf-record.py -> perf-publish.sh (PERF_PUSH=${PERF_PUSH:-unset})"
			else
				say "step perf: SKIP (tag perf-anchor-1 does not exist)"
			fi
			continue
		fi
		say "step $s: scope slice=$SLICE MemoryMax=$MEM CPUQuota=$QUOTA CPUWeight=20 timeout=$CAP"
		say "           env $(printf '%q ' "${GOENV[@]}")${DESC:-$(printf '%q ' "${CMD[@]}")}"
	done
	exit 0
fi

# ---- the run ----------------------------------------------------------------
mkdir -p "$ROOT/.ds4" "$LOGDIR"
exec 8>"$ROOT/.ds4/nightly.lock"
flock -n 8 || {
	say "another nightly holds $ROOT/.ds4/nightly.lock; exiting"
	exit 75
}

# Under the run lock no other nightly is live, so any leftover nightly-run-*
# worktree is from a killed run: remove it.
for old in "$ROOT"/.worktrees/nightly-run-*; do
	[ -d "$old" ] || continue
	say "removing stale worktree $old"
	git -C "$ROOT" worktree remove --force "$old" 2>/dev/null || rm -rf "$old"
done
git -C "$ROOT" worktree prune

git -C "$ROOT" fetch -q origin main || say "WARNING: fetch origin main failed; testing the local $REF"
SHA=$(git -C "$ROOT" rev-parse --verify "$REF^{commit}") || {
	say "cannot resolve $REF"
	exit 2
}
[ -d "$ROOT/.cards" ] && [ -n "$(ls -A "$ROOT/.cards" 2>/dev/null)" ] || {
	# Without the corpus every corpus-dependent test SKIPS and reads green
	# (scripts/agent-worktree.sh). Refuse rather than report a hollow pass.
	say "no corpus at $ROOT/.cards -- run 'make fetch-cards compile-cards' first"
	record setup FAIL 0 - "no .cards corpus"
	exit 1
}
git -C "$ROOT" worktree add -q --detach "$WT" "$SHA" || {
	record setup FAIL 0 - "worktree add failed"
	exit 1
}
cleanup() {
	if [ "${NIGHTLY_KEEP_WT:-0}" = 1 ]; then
		say "keeping $WT (NIGHTLY_KEEP_WT=1)"
	else
		git -C "$ROOT" worktree remove --force "$WT" 2>/dev/null || rm -rf "$WT"
		git -C "$ROOT" worktree prune
	fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP
ln -sfn "$ROOT/.cards" "$WT/.cards"
mkdir -p "$WT/.ds4"
say "testing ${SHA:0:12} ($REF) in $WT; logs in $LOGDIR"

failed=0
RUN_START=$(date +%s)
for s in "${step_list[@]}"; do
	step_spec "$s"
	log=$LOGDIR/$s.log
	if [ "$s" = perf ] && [ "${PERF_NIGHTLY:-}" != 1 ]; then
		record perf SKIP 0 - "PERF_NIGHTLY!=1"
		continue
	fi
	wait_postmerge
	start=$(date +%s)
	if [ "$s" = perf ]; then
		run_perf "$log"
	else
		run_scoped "$s" "$log"
	fi
	rc=$?
	wall=$(($(date +%s) - start))
	case $rc in
	0) record "$s" PASS "$wall" "$log" ;;
	99) record "$s" SKIP "$wall" "$log" "tag perf-anchor-1 does not exist" ;;
	75)
		record "$s" FAIL "$wall" "$log" "no heavy lane in ${LANE_WAIT}s (or the step exited 75)"
		failed=1
		;;
	124 | 137)
		record "$s" FAIL "$wall" "$log" "wall cap $CAP hit (rc=$rc)"
		failed=1
		;;
	*)
		record "$s" FAIL "$wall" "$log" "rc=$rc"
		failed=1
		;;
	esac
done

run_wall=$(($(date +%s) - RUN_START))
if [ "$failed" = 0 ]; then
	record run PASS "$run_wall" "$LOGDIR" "steps=$STEPS"
else
	record run FAIL "$run_wall" "$LOGDIR" "steps=$STEPS"
fi
exit "$failed"
