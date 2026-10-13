#!/usr/bin/env bash
# Post-merge full suite in batches, bisect on red (operator, 2026-10-05).
#
# agentctl lands with landing.push = false behind the lean per-ticket gate
# (scripts/gate_affected.sh). This loop runs scripts/postmerge_full.sh once on
# main's head, then again on whatever landed since the last run -- never once
# per commit. A green run pushes exactly the commit it tested.
#
# A red run is bisected, not re-run: the failing packages/tests are replayed
# (only those, never the full suite) over the first-parent commits between
# the last green and the red head, and the first commit that fails them is
# logged as the culprit (CULPRIT lines, also in $CULPRITS). While main stays
# red, a new head first re-checks just those failing tests; the full suite
# runs again only once they pass.
#
# The suite and its bisect steps run GATE class (`gorge_heavy_run -g`): they
# hold a heavy lane but are never parked or deferred by a ticket gate's broker
# bracket. This loop is what pushes main, so parking it stalls every landing's
# push behind the gates of later tickets.
#
# A red run also PAUSES the agentctl pipeline (writes $PAUSE, the file the
# daemon checks every tick) so nothing else lands on a red main while the
# operator bisects and fixes. The pause is ours only if the file was absent:
# it carries a "postmerge_batch" reason, and the next GREEN run removes it.
# A pause anyone else wrote (fleet halt, operator) is never touched.
#
#   scripts/postmerge_batch.sh            # loop forever
#   GORGE_HEAVY_LOCK=/path/to/lock scripts/postmerge_batch.sh
#   scripts/postmerge_batch.sh --parse-fails <go-test-output>   # self-test aid
set -uo pipefail
repo=$(git rev-parse --show-toplevel)
. "$(dirname "$0")/heavy_lock.sh"  # the one HEAVY lock definition
LOCK=${LOCK:-$GORGE_HEAVY_LOCK}
LOG=${LOG:-$repo/.ds4/postmerge-batch.log}
CULPRITS=${CULPRITS:-$repo/.ds4/postmerge-culprits.log}
OUT=$repo/.ds4/postmerge-full.out
PAUSE=${PAUSE:-$repo/.ds4/orchestrator/pause}
wt=$repo/.worktrees/postmerge-full
# postmerge_full runs up to eight test binaries concurrently (-p=8). Keep
# their aggregate soft heap limit below this scope's hard cap, with per-test
# runtime settings matching the operator's 4 GiB / 4 vCPU budget (doubled
# 2026-10-09 from 2 GiB / 2 vCPU with the box's DRAM). The two host tests that
# pinned the 2 GiB inventory fit 4 GiB, so the shrink-only exception inventory
# at internal/testutil/testdata/rss_exceptions.txt is empty; the scope's hard
# cap doubles to 32G so eight binaries at the 4 GiB ceiling still fit it.
SCOPE=(systemd-run --user --scope -q -p MemoryMax=32G -p CPUQuota=1600% env GOMEMLIMIT=3GiB GOMAXPROCS=4 GOGC=200 GOFLAGS="-p=2 -trimpath" GORGE_ORACLEGEN_FULL_TARGET_AUDIT=1)

say() { echo "$(date '+%F %T') $*" | tee -a "$LOG"; }

pause_pipeline() {
  [ -e "$PAUSE" ] && return 0
  echo "postmerge_batch RED $1 $(date -u +%FT%TZ): main->origin full suite failed; bisecting (see $LOG)" >"$PAUSE"
  say "PAUSED pipeline: RED ${1:0:9}"
}
# our_pause: the pause marker exists and this loop wrote it.
our_pause() { [ -e "$PAUSE" ] && /usr/bin/grep -q '^postmerge_batch ' "$PAUSE"; }
# resume_pipeline <sha> [why]: lift our own pause; why defaults to "GREEN <sha>".
resume_pipeline() {
  if [ -e "$PAUSE" ] && /usr/bin/grep -q '^postmerge_batch ' "$PAUSE"; then
    rm -f "$PAUSE"
    say "RESUMED pipeline: ${2:-GREEN ${1:0:9}}"
  fi
}

# parse_fails prints one "<pkg> <regex>" line per failing package from
# `go test` output (non -v): the top-level --- FAIL names seen before each
# "FAIL<TAB><pkg>" line. A package that failed with no named test (timeout,
# panic, build failure) gets regex "." (run the whole package).
parse_fails() {
  awk '
    /^--- FAIL: / { n = $3; if (!(n in seen)) { seen[n] = 1; t = t (t ? "|" : "") n } next }
    /^FAIL\t[^ \t]+/ {
      split($0, f, "\t"); p = f[2]
      if (p ~ /^github\.com\//) print p, (t ? "^(" t ")$" : ".")
      t = ""; delete seen
    }' "$1"
}

for _a in "$@"; do [ "$_a" = --print-heavy-lock ] && { printf '%s\n' "$LOCK"; exit 0; }; done
unset _a
if [ "${1:-}" = "--parse-fails" ]; then parse_fails "$2"; exit 0; fi

# fails_at <sha> <pkg> <regex>: 0 if the tests FAIL at sha, 1 if they pass.
fails_at() {
  git -C "$wt" switch -q --detach "$1" || return 2
  ! (cd "$wt" && gorge_heavy_run -g "${SCOPE[@]}" go test -p=4 -run "$3" "$2" >/dev/null 2>&1)
}

# bisect <good> <bad> <pkg> <regex>: first first-parent commit in good..bad
# whose tests fail. Assumes they pass at good and fail at bad.
bisect() {
  local commits lo hi mid
  mapfile -t commits < <(git rev-list --first-parent --reverse "$1..$2")
  lo=0; hi=$((${#commits[@]} - 1))
  while [ "$lo" -lt "$hi" ]; do
    mid=$(( (lo + hi) / 2 ))
    if fails_at "${commits[$mid]}" "$3" "$4"; then hi=$mid; else lo=$((mid + 1)); fi
  done
  echo "${commits[$lo]}"
}

[ -d "$wt" ] || scripts/agent-worktree.sh postmerge-full main >/dev/null

last_tested=""   # head of the last completed full run (green or red)
last_green=$(git rev-parse origin/main)
red_fails=""     # "<pkg> <regex>" lines still failing on main
flaky_reruns=0   # consecutive all-flaky full reruns at one head
FLAKY_RERUNS_MAX=2
# Cadence (2026-10-12): a full run is ~8-12 min on up to 16 cores, so on a
# GREEN pipeline it starts only once BATCH_MIN_AHEAD commits are unpushed or
# the oldest unpushed commit is BATCH_MAX_AGE_S old. Any non-green state (red
# failures outstanding, our pause marker, a pending confirming rerun) runs on
# every head change, as before.
BATCH_MIN_AHEAD=${BATCH_MIN_AHEAD:-4}
BATCH_MAX_AGE_S=${BATCH_MAX_AGE_S:-1800}
need_run=0       # 1 from a RED/ALL FLAKY until the next GREEN: never defer
last_deferred="" # head whose DEFER line was already logged
while true; do
  git fetch -q origin main 2>/dev/null || true
  head=$(git rev-parse main)
  # Normally only an unpushed head is worth a run. While OUR pause is up,
  # any new head is: a hand fix pushed straight to origin leaves nothing
  # ahead, and main cannot move ahead while the pipeline is paused, so
  # requiring an unpushed commit here kept the pause up forever (2026-10-12:
  # 6025ab05c, the caretaker-flake fix, was pushed by hand at 15:4x with this
  # loop's pause in place; the promised confirming rerun never started).
  go_run=0
  if [ "$head" != "$last_tested" ]; then
    ahead=$(git rev-list --count origin/main..main)
    if [ -n "$red_fails" ] || [ "$need_run" = 1 ] || our_pause; then
      { [ "$ahead" -gt 0 ] || our_pause; } && go_run=1
    elif [ "$ahead" -gt 0 ]; then
      oldest=$(git log --reverse --format=%ct origin/main..main | head -n1)
      age=$(( $(date +%s) - ${oldest:-$(date +%s)} ))
      if [ "$ahead" -ge "$BATCH_MIN_AHEAD" ] || [ "$age" -ge "$BATCH_MAX_AGE_S" ]; then
        go_run=1
      elif [ "$head" != "$last_deferred" ]; then
        last_deferred=$head
        say "DEFER ahead=$ahead oldest=${age}s (min $BATCH_MIN_AHEAD / max ${BATCH_MAX_AGE_S}s)"
      fi
    fi
  fi
  if [ "$go_run" = 1 ]; then
    # Still red? Re-check only the failing tests before paying for a full run.
    still=""
    while read -r pkg re; do
      [ -n "$pkg" ] || continue
      if fails_at "$head" "$pkg" "$re"; then still+="$pkg $re"$'\n'; fi
    done <<<"$red_fails"
    if [ -n "$still" ]; then
      last_tested=$head
      say "STILL RED ${head:0:9}: $(echo "$still" | awk '{print $1}' | sort -u | tr '\n' ' ')"
    else
      red_fails=""
      say "FULL start ${head:0:9} ($(git rev-list --first-parent --count "$last_green..$head") since last green ${last_green:0:9})"
      s=$(date +%s)
      if gorge_heavy_run -g "${SCOPE[@]}" scripts/postmerge_full.sh "$wt" "$head" >"$OUT" 2>&1; then
        last_tested=$head
        if git push -q origin "$head:refs/heads/main"; then
          last_green=$head; flaky_reruns=0; need_run=0
          say "GREEN ${head:0:9} pushed ($(( $(date +%s) - s ))s)"
          resume_pipeline "$head"
        elif git fetch -q origin main && git merge-base --is-ancestor "$head" origin/main; then
          # Someone pushed main past the green head first (a direct push):
          # the green commit is already on origin, so the pause must lift --
          # with nothing left to push this loop would never run again.
          last_green=$head; flaky_reruns=0; need_run=0
          say "GREEN ${head:0:9} already on origin ($(( $(date +%s) - s ))s)"
          resume_pipeline "$head"
        else
          say "GREEN ${head:0:9} push FAILED"
        fi
      else
        last_tested=$head; need_run=1
        pause_pipeline "$head"
        say "RED ${head:0:9} ($(( $(date +%s) - s ))s): $(/usr/bin/grep -E '^(--- FAIL|FAIL|panic)' "$OUT" | head -n5 | tr '\n' ' ')"
        red_fails=$(parse_fails "$OUT")
        if [ -z "$red_fails" ]; then
          say "RED ${head:0:9}: no failing package parsed (vet/testbudget/TestHeads/sim step?) -- see $OUT; not bisected"
        fi
        real_fails=""
        while read -r pkg re; do
          [ -n "$pkg" ] || continue
          if ! fails_at "$head" "$pkg" "$re"; then
            say "FLAKY ${head:0:9} $pkg $re: passes on rerun"
            continue
          fi
          real_fails+="$pkg $re"$'\n'
          if fails_at "$last_green" "$pkg" "$re"; then
            say "CULPRIT unknown for $pkg $re: already fails at last green ${last_green:0:9}"
            continue
          fi
          c=$(bisect "$last_green" "$head" "$pkg" "$re")
          msg="CULPRIT ${c:0:9} \"$(git log -1 --format=%s "$c")\" breaks $pkg $re (batch ${last_green:0:9}..${head:0:9})"
          say "$msg"; echo "$(date '+%F %T') $msg" >>"$CULPRITS"
        done <<<"$red_fails"
        # Every parsed failure passed on rerun: a load flake, not a red main.
        # Re-run the full suite at this head (clearing last_tested) instead of
        # waiting for main to move -- main cannot move while paused, so the
        # old wait deadlocked (2026-10-10 11:18 -> 10-12, 47h, 8 FLAKY).
        # Bounded: after FLAKY_RERUNS_MAX flaky reruns it stays paused.
        # Dispatch resumes now rather than after the confirming rerun (18
        # idle minutes on 2026-10-12, 3c23860d2); a RED rerun re-pauses via
        # pause_pipeline, which writes the marker whenever it is absent.
        if [ -n "$red_fails" ] && [ -z "$real_fails" ]; then
          if [ "$flaky_reruns" -lt "$FLAKY_RERUNS_MAX" ]; then
            flaky_reruns=$((flaky_reruns + 1))
            last_tested=""
            say "ALL FLAKY ${head:0:9}: full rerun $flaky_reruns/$FLAKY_RERUNS_MAX"
            resume_pipeline "$head" "ALL FLAKY ${head:0:9} (confirming full rerun running)"
          else
            say "ALL FLAKY ${head:0:9} again after $FLAKY_RERUNS_MAX reruns: staying paused; operator needed"
          fi
        fi
        red_fails=$real_fails
        git -C "$wt" switch -q --detach "$head"
      fi
    fi
  fi
  # The fleet's worktrees each key their own build-cache entries, so
  # GOCACHE grows ~1 TB/day here and Go's own 5-day trim never catches up
  # (2026-10-05: / hit 100% and git could not create worktrees). Drop
  # entries unused for 24h whenever / passes 90%. Not 6h/80%: / sits at
  # ~79%, so the trim ran nearly every loop and deleted entries a live
  # build was opening (2026-10-06 false RED b75197f98: vet "could not
  # import internal/godebug ... no such file").
  if [ "$(df --output=pcent / | tail -n1 | tr -dc 0-9)" -ge 90 ]; then
    gc=$(go env GOCACHE)
    nice find "$gc" -type f -mmin +1440 -delete 2>/dev/null
    say "GOCACHE trimmed: / now $(df --output=pcent / | tail -n1 | tr -d ' ')"
  fi
  sleep 60
done
