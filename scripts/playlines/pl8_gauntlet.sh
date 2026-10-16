#!/usr/bin/env bash
# PL-8 step 3: gauntlet one value set (or the 17Lands/random control) with the
# az-c5 pilot, sims=25, FRESH seeds 301-324 (G0 used 201-224: draftrl gate -seed
# 201 -events 24, event n = seed+n; verified in g0/*.log "seeds 201-224").
#   pl8_gauntlet.sh <arm>   arm in: c4hlogit c5s25hlogit c5s100hlogit c5s25gih 17l
# Env: PL8_OUT, SEED (301), EVENTS (24), GAMES (4), MAXHUMAN (0=all), WORKERS,
#      MEMMAX, CPUQ, C5NET, CORPUS, TIMEOUT. Writes $OUT/gauntlet/<arm>/{gate.json,log}.
set -euo pipefail
arm=${1:?arm}
here=$(cd "$(dirname "$0")" && pwd); repo=$(cd "$here/../.." && pwd)
OUT=${PL8_OUT:-/mnt/sata/gorge-training/playlines/pl-8}
NET=${C5NET:-/mnt/sata/gorge-training/az-c5b/c5.dzgw}
CORPUS=${CORPUS:-$repo/.cards}
CV=/mnt/sata/gorge-training/cardval
WORKERS=${WORKERS:-16}; MEMMAX=${MEMMAX:-12G}; GOMEM=${GOMEM:-10}; CPUQ=${CPUQ:-1600%}
case $arm in
  c4hlogit)     ds=(-baseline none -drafters "ovr:$CV/az-c4-s25/values.json#hlogit") ;;
  c5s25hlogit)  ds=(-baseline none -drafters "ovr:$OUT/values/c5s25/values.json#hlogit") ;;
  c5s100hlogit) ds=(-baseline none -drafters "ovr:$OUT/values/c5s100/values.json#hlogit") ;;
  c5s25gih)     ds=(-baseline none -drafters "ovr:$OUT/values/c5s25/values.json#gih") ;;
  17l)          ds=(-baseline random) ;;  # 17Lands drafter + the random-deck baseline in one run
  *) echo "unknown arm $arm" >&2; exit 2 ;;
esac
mkdir -p "$OUT/bin" "$OUT/gauntlet/$arm"
(cd "$repo" && go build -o "$OUT/bin/draftrl" ./cmd/draftrl)
t=${TIMEOUT:-0}; tm=(); [ "$t" != 0 ] && tm=(timeout "$t")
extra=(); [ "${MAXHUMAN:-0}" != 0 ] && extra=(-max-human "$MAXHUMAN")
"${tm[@]}" systemd-run --user --scope -q -p MemoryMax=$MEMMAX -p MemorySwapMax=0 -p CPUQuota=$CPUQ \
  env GOMAXPROCS=$WORKERS GOMEMLIMIT=${GOMEM}GiB \
  "$OUT/bin/draftrl" gate -corpus "$CORPUS" -workers "$WORKERS" -seed "${SEED:-301}" \
    -events "${EVENTS:-24}" -games "${GAMES:-4}" -pilot az -az-net "$NET" -az-sims 25 \
    -out "$OUT/gauntlet/$arm" "${extra[@]}" "${ds[@]}" > "$OUT/gauntlet/$arm/log" 2>&1
tail -8 "$OUT/gauntlet/$arm/log"
