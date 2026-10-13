#!/usr/bin/env bash
# Paired A/B of the paid-cost mana census (flag off vs on): same decks, same seeds, same policies.
# Brief: .ds4/mana-model/D-ab.md. Report: docs/superpowers/reports/2026-10-12-mana-model-ab.md.
#
#   OUT=<dir> DZ=<dzgorge binary> W=<workers> PAIRS=600 bash scripts/manamodel_ab.sh [job ...]
#
# A job is <set>-<run>-<flag>, e.g. fdn-r3-on; with no argument every job runs, one at a time,
# each under the heavy lock. A job whose output exists is skipped (resumable).
# Sets: fdn (FDN eval split of the dzgorge-repro inputs), cons (the 60-card repo decks listed in
# $OUT/cons.only, found by the screen job). Runs: r1 sbtactical vs bot; r2 bot:autopay vs
# sbtactical; r3 the search (no net) vs sbtactical.
set -uo pipefail
OUT=${OUT:?OUT}; DZ=${DZ:?DZ}; W=${W:-6}; PAIRS=${PAIRS:-600}; SEED=${SEED:-4242}
I=${I:-/mnt/sata/gorge-training/dzgorge-repro/inputs}
LOCK=${LOCK:-/home/sadams/projects/gorge/.ds4/heavy.lock}
AZ="az:sims=100:autopay:oppnodes:oppfull:morekinds:mull"
mkdir -p "$OUT"
policy() { case $1 in sb) echo sbtactical;; bot) echo bot;; botap) echo bot:autopay;; az) echo "$AZ";; esac; }
declare -A RUNA=([r1]=sb [r2]=botap [r3]=az) RUNB=([r1]=bot [r2]=sb [r3]=sb)
job() { # set run flag
  local set=$1 run=$2 flag=$3 o="$OUT/$1-$2-$3"
  [ -e "$o.jsonl.summary.json" ] && { echo "skip $o"; return; }
  local src=(-decks "$I/decks" -pool "$I/pool.tsv")
  [ "$set" = cons ] && src=(-decks "$OUT/cdecks" -pool "$OUT/cpool.tsv" -only "$OUT/cons.only")
  local fl=(); [ "$flag" = off ] && fl=(-no-paid-mana-census)
  echo "[$(date -u +%FT%TZ)] $o"
  flock -o "$LOCK" timeout 3h "$DZ" play "${src[@]}" -split eval -a "$(policy "${RUNA[$run]}")" -b "$(policy "${RUNB[$run]}")" \
    -pairs "$PAIRS" -seed "$SEED" -workers "$W" -mulligans 2 -mull-heuristic -mm-stats "${fl[@]}" \
    -out "$o.jsonl" > "$o.log" 2>&1 || echo "FAILED $o (see $o.log)"
}
if [ $# -gt 0 ]; then
  for j in "$@"; do IFS=- read -r s r f <<<"$j"; job "$s" "$r" "$f"; done
else
  for s in fdn cons; do for r in r1 r2 r3; do for f in off on; do job $s $r $f; done; done; done
fi
