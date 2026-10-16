#!/usr/bin/env bash
# PL-3 runner: randomised play lines. Builds nothing; BIN must be a dzgorge built from this tree.
#
#   BIN=... OUT=/mnt/sata/gorge-training/playlines/pl-3/full scripts/playlines/pl3_run.sh
#
# Per arm: one WIN run (PAIRS pairs x 2 legs, seed 78, -lines) and REPLAYS diversity replays
# (DIVPAIRS pairs x 1 leg each, same seed, only the arm's :aseed=k differs). The null control
# "base2" is the baseline on SEED2. Output: $OUT/<arm>.jsonl, $OUT/<arm>.r<k>.jsonl, $OUT/arms.tsv.
# Run it under scripts/heavy.sh (orchestrator); every dzgorge call is already wrapped in `timeout`.
set -uo pipefail
BIN=${BIN:?set BIN to a dzgorge binary}
OUT=${OUT:?set OUT}
I=${INPUTS:-/mnt/sata/gorge-training/dzgorge-repro/inputs}
NET=${NET:-/mnt/sata/gorge-training/az-c5b/c5.dzgw}
SIMS=${SIMS:-25}
PAIRS=${PAIRS:-200}        # x2 legs = 400 paired games per arm
DIVPAIRS=${DIVPAIRS:-40}   # single-leg games per replay set
REPLAYS=${REPLAYS:-8}
SEED=${SEED:-78}
SEED2=${SEED2:-79}
WORKERS=${WORKERS:-8}
ONLY=${ONLY:-}             # optional -only file
TMO=${TMO:-7200}
REF=${REF:-sbtactical}
ARMS_FILTER=${ARMS_FILTER:-}   # space-separated subset of arm names (smoke)
mkdir -p "$OUT"

B="az:sims=$SIMS:autopay:oppnodes:oppfull:morekinds:closedlist:topk=6:gonet=$NET"
declare -A SPEC=(
  [base]="$B"
  [ex2]="$B:explore:nonoise:exploreturns=2"
  [ex4]="$B:explore:nonoise:exploreturns=4"
  [ex8]="$B:explore:nonoise:exploreturns=8"
  [exn]="$B:explore"
  [t05]="$B:temp=0.5"
  [t10]="$B:temp=1.0"
  [s05]="$B:second=0.05"
  [s15]="$B:second=0.15"
)
ORDER=(base ex2 ex4 ex8 exn t05 t10 s05 s15)
: > "$OUT/arms.tsv"
for a in "${ORDER[@]}"; do printf '%s\t%s\n' "$a" "${SPEC[$a]}" >> "$OUT/arms.tsv"; done
printf 'base2\t%s\n' "${SPEC[base]}" >> "$OUT/arms.tsv"

only=(); [ -n "$ONLY" ] && only=(-only "$ONLY")
play() { # name arm-spec seed pairs single(0/1) outfile
  local single=(); [ "$5" = 1 ] && single=(-single)
  timeout "$TMO" "$BIN" play -decks "$I/decks" -pool "$I/pool.tsv" -split eval "${only[@]}" \
    -a "$2" -b "$REF" -pairs "$4" "${single[@]}" -seed "$3" -workers "$WORKERS" -mulligans 2 -mull-heuristic \
    -lines -out "$6" > "${6%.jsonl}.log" 2>&1 || echo "FAILED $1 (exit $?)" >&2
}
want() { [ -z "$ARMS_FILTER" ] || [[ " $ARMS_FILTER " == *" $1 "* ]]; }

for a in "${ORDER[@]}"; do
  want "$a" || continue
  play "$a" "${SPEC[$a]}" "$SEED" "$PAIRS" 0 "$OUT/$a.jsonl"
  for ((k = 1; k <= REPLAYS; k++)); do
    play "$a.r$k" "${SPEC[$a]}:aseed=$k" "$SEED" "$DIVPAIRS" 1 "$OUT/$a.r$k.jsonl"
  done
done
want base2 && play base2 "${SPEC[base]}" "$SEED2" "$PAIRS" 0 "$OUT/base2.jsonl"
echo done > "$OUT/DONE"
