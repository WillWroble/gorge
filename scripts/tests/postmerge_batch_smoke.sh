#!/usr/bin/env bash
# postmerge_batch_smoke.sh -- scripts/postmerge_batch.sh's pause/resume around
# an ALL FLAKY red, against fakes: a temp git repo with a bare origin, a stub
# heavy_lock.sh, and stub systemd-run / go / postmerge_full.sh / sleep on PATH.
# No Go build, no network, no systemd.
#
#   scripts/tests/postmerge_batch_smoke.sh
#
# Each scenario: full run #1 is RED (one package timeout) and the package
# passes alone, so the batch is ALL FLAKY. The pause marker must be gone
# BEFORE the confirming full run #2 reports (the stub records whether the
# marker exists when run #2 starts). Then:
#   green: run #2 GREEN  -> head pushed to origin, marker absent.
#   red:   run #2 RED and the package now fails alone too -> marker present.
# The second `sleep` ends the loop (the stub kills the script).
set -uo pipefail
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
ROOT=$(git rev-parse --show-toplevel)
SCRIPT=${PMB_SCRIPT_UNDER_TEST:-$ROOT/scripts/postmerge_batch.sh} # a mutant, when proving the smoke can fail
TMP=$(mktemp -d "${TMPDIR:-/tmp}/pmb-smoke.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
export GIT_AUTHOR_NAME=smoke GIT_AUTHOR_EMAIL=smoke@example.invalid GIT_COMMITTER_NAME=smoke GIT_COMMITTER_EMAIL=smoke@example.invalid
fails=0
check() { # check <name> <exit status of the assertion> [detail]
	if [ "$2" = 0 ]; then
		printf 'ok   %s\n' "$1"
	else
		printf 'FAIL %s %s\n' "$1" "${3:-}"
		fails=$((fails + 1))
	fi
}
has() { /usr/bin/grep -qF -- "$2" "$1" 2>/dev/null; }

# ---- stubs on PATH --------------------------------------------------------
B=$TMP/bin
mkdir -p "$B"
cat >"$B/systemd-run" <<'EOF'
#!/usr/bin/env bash
# Drop the scope options; run the `env ...` command line as-is.
while [ $# -gt 0 ] && [ "$1" != env ]; do shift; done
exec "$@"
EOF
cat >"$B/go" <<'EOF'
#!/usr/bin/env bash
# `go test` alone: fails while $PMB_STATE/go_fail exists.
[ -e "$PMB_STATE/go_fail" ] && exit 1
exit 0
EOF
cat >"$B/sleep" <<'EOF'
#!/usr/bin/env bash
n=$(( $(cat "$PMB_STATE/sleeps" 2>/dev/null || echo 0) + 1 ))
echo "$n" >"$PMB_STATE/sleeps"
[ "$n" -ge 2 ] && kill -TERM "$PPID"
exit 0
EOF
chmod +x "$B"/*

# run_scenario <name> <green|red>: prints the scenario dir.
run_scenario() {
	local d=$TMP/$1 r
	r=$d/repo
	mkdir -p "$d/state"
	git init -q --bare "$d/origin.git"
	git init -q -b main "$r"
	mkdir -p "$r/scripts" "$r/.ds4/orchestrator"
	cp "$SCRIPT" "$r/scripts/postmerge_batch.sh"
	cat >"$r/scripts/heavy_lock.sh" <<'EOF'
GORGE_HEAVY_LOCK=${GORGE_HEAVY_LOCK:-/nonexistent/heavy.lock}
gorge_heavy_run() { [ "${1:-}" = -g ] && shift; "$@"; }
EOF
	cat >"$r/scripts/postmerge_full.sh" <<'EOF'
#!/usr/bin/env bash
# Full run N: records whether the pause marker exists as it starts.
n=$(( $(cat "$PMB_STATE/fulls" 2>/dev/null || echo 0) + 1 ))
echo "$n" >"$PMB_STATE/fulls"
if [ -e "$PMB_PAUSE" ]; then echo present; else echo absent; fi >"$PMB_STATE/marker_at_full$n"
if { [ "$n" = 1 ] && [ "${PMB_FIRST:-red}" = red ]; } || { [ "$n" != 1 ] && [ "$PMB_RERUN" = red ]; }; then
	[ "$n" = 1 ] || touch "$PMB_STATE/go_fail" # run #2 red is real: fails alone too
	printf 'FAIL\tgithub.com/x/pkg\t600.0s\n'
	exit 1
fi
echo ok
EOF
	chmod +x "$r/scripts/"*.sh
	printf '.ds4/\n.worktrees/\n' >"$r/.gitignore"
	git -C "$r" add -A && git -C "$r" commit -q -m base
	git -C "$r" remote add origin "$d/origin.git"
	git -C "$r" push -q origin main
	echo x >"$r/landed.txt"
	git -C "$r" add landed.txt && git -C "$r" commit -q -m "landed behind the gate"
	git -C "$r" worktree add -q --detach "$r/.worktrees/postmerge-full" main
	if [ "${3:-}" = foreign ]; then
		git -C "$r" push -q origin main
		echo "operator hold" >"$r/.ds4/orchestrator/pause"
	fi
	if [ "${3:-}" = idle ]; then
		# Nothing ahead of origin and no pause at all: the idle fast path.
		git -C "$r" push -q origin main
	fi
	if [ "${3:-}" = handpush ]; then
		# The loop's own pause is up and the fix was pushed by hand: nothing
		# is ahead of origin.
		git -C "$r" push -q origin main
		echo "postmerge_batch RED deadbeef 2026-10-12T21:37:39Z: main->origin full suite failed" \
			>"$r/.ds4/orchestrator/pause"
	fi
	(cd "$r" && PATH="$B:$PATH" PMB_STATE="$d/state" PMB_PAUSE="$r/.ds4/orchestrator/pause" PMB_RERUN="$2" \
		PMB_FIRST="${4:-red}" timeout 60 bash scripts/postmerge_batch.sh >"$d/stdout" 2>&1)
	echo "$d"
}

# ---- green: ALL FLAKY -> resumed early; rerun GREEN -> pushed --------------
d=$(run_scenario green green)
r=$d/repo
head=$(git -C "$r" rev-parse main)
check "green: full run #1 RED paused the pipeline" "$(has "$r/.ds4/postmerge-batch.log" "PAUSED pipeline: RED ${head:0:9}"; echo $?)"
check "green: early resume logged" "$(has "$r/.ds4/postmerge-batch.log" "RESUMED pipeline: ALL FLAKY ${head:0:9} (confirming full rerun running)"; echo $?)"
check "green: marker absent when the confirming rerun started" "$([ "$(cat "$d/state/marker_at_full2" 2>/dev/null)" = absent ]; echo $?)" "$(cat "$d/state/marker_at_full2" 2>/dev/null)"
check "green: confirming rerun pushed head" "$([ "$(git -C "$d/origin.git" rev-parse main)" = "$head" ]; echo $?)"
check "green: GREEN logged" "$(has "$r/.ds4/postmerge-batch.log" "GREEN ${head:0:9} pushed"; echo $?)"
check "green: marker absent at the end" "$([ ! -e "$r/.ds4/orchestrator/pause" ]; echo $?)"

# ---- red: ALL FLAKY -> resumed early; rerun RED real -> re-paused ----------
d=$(run_scenario red red)
r=$d/repo
head=$(git -C "$r" rev-parse main)
check "red: early resume logged" "$(has "$r/.ds4/postmerge-batch.log" "RESUMED pipeline: ALL FLAKY ${head:0:9}"; echo $?)"
check "red: marker absent when the confirming rerun started" "$([ "$(cat "$d/state/marker_at_full2" 2>/dev/null)" = absent ]; echo $?)" "$(cat "$d/state/marker_at_full2" 2>/dev/null)"
check "red: marker present again after the real RED" "$(has "$r/.ds4/orchestrator/pause" "postmerge_batch RED $head"; echo $?)"
check "red: paused twice" "$([ "$(/usr/bin/grep -c 'PAUSED pipeline: RED' "$r/.ds4/postmerge-batch.log")" = 2 ]; echo $?)"
check "red: nothing pushed" "$([ "$(git -C "$d/origin.git" rev-parse main)" != "$head" ]; echo $?)"

# ---- handpush: our pause is up, the fix reached origin by hand ------------
# Nothing is ahead of origin, so the old loop never ran again and its own
# pause never lifted. The loop must run the suite at the new head anyway.
d=$(run_scenario handpush green handpush green)
r=$d/repo
head=$(git -C "$r" rev-parse main)
check "handpush: a full run started with nothing ahead of origin" "$([ "$(cat "$d/state/fulls" 2>/dev/null)" = 1 ]; echo $?)" "fulls=$(cat "$d/state/fulls" 2>/dev/null)"
check "handpush: GREEN lifted our pause" "$([ ! -e "$r/.ds4/orchestrator/pause" ]; echo $?)"
check "handpush: RESUMED logged" "$(has "$r/.ds4/postmerge-batch.log" "RESUMED pipeline: GREEN ${head:0:9}"; echo $?)"

# ---- foreign: an operator's pause with nothing ahead is left alone --------
d=$(run_scenario foreign green foreign green)
r=$d/repo
check "foreign: no full run for someone else's pause" "$([ ! -e "$d/state/fulls" ]; echo $?)" "fulls=$(cat "$d/state/fulls" 2>/dev/null)"
check "foreign: the operator's pause is untouched" "$(has "$r/.ds4/orchestrator/pause" "operator hold"; echo $?)"

# ---- idle: nothing ahead and no pause -> no run ---------------------------
# The guard's new "our pause is up" disjunct must not fire with no pause, and
# the head-equality check must still suppress a run when nothing is ahead.
d=$(run_scenario idle green idle green)
r=$d/repo
check "idle: no full run with nothing ahead and no pause" "$([ ! -e "$d/state/fulls" ]; echo $?)" "fulls=$(cat "$d/state/fulls" 2>/dev/null)"
check "idle: no pause marker created" "$([ ! -e "$r/.ds4/orchestrator/pause" ]; echo $?)"

[ "$fails" = 0 ] && echo PASS || { echo "FAIL ($fails)"; exit 1; }
