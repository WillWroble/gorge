#!/usr/bin/env bash
# PL-8 step 1: measure card values with the az-c5 pilot (random decks, in-hand
# recording), then fit gih/iwd/hlogit/deck with the SAME code that produced the
# az-c4-s25 control values (cardval/analyze.py hand_values, via pl8_fit.py).
#   pl8_values.sh <arm> <sims> <games> [seed]     e.g.  pl8_values.sh c5s25 25 20000 71
# Env: PL8_OUT (default /mnt/sata/gorge-training/playlines/pl-8), WORKERS (16),
#      MEMMAX (24G), GOMEM (16, GiB; cardval needs MEMMAX >= 1.4x GOMEM and GOMEM >= ~1 GiB/worker), CPUQ (1600%), C5NET, CORPUS, TIMEOUT (seconds; 0 = none).
# Full runs: c5s25 25 20000 71 (~35 min), c5s100 100 6000 72 (~40 min).
set -euo pipefail
arm=${1:?arm}; sims=${2:?sims}; games=${3:?games}; seed=${4:-71}
here=$(cd "$(dirname "$0")" && pwd); repo=$(cd "$here/../../.." && pwd)
OUT=${PL8_OUT:-/mnt/sata/gorge-training/playlines/pl-8}
NET=${C5NET:-/mnt/sata/gorge-training/az-c5b/c5.dzgw}
CORPUS=${CORPUS:-$repo/.cards}
PY=${PY:-/mnt/sata/gorge-training/cardval/venv/bin/python}
WORKERS=${WORKERS:-16}; MEMMAX=${MEMMAX:-24G}; GOMEM=${GOMEM:-16}; CPUQ=${CPUQ:-1600%}
mkdir -p "$OUT/bin" "$OUT/values/$arm"
(cd "$repo" && go build -o "$OUT/bin/cardval" ./lab/cmd/cardval)
t=${TIMEOUT:-0}; tm=(); [ "$t" != 0 ] && tm=(timeout "$t")
"${tm[@]}" systemd-run --user --scope -q -p MemoryMax=$MEMMAX -p MemorySwapMax=0 -p CPUQuota=$CPUQ \
  env GOMAXPROCS=$WORKERS GOMEMLIMIT=${GOMEM}GiB \
  "$OUT/bin/cardval" -corpus "$CORPUS" -workers "$WORKERS" -mode random -set FDN -hands \
    -az -az-net "$NET" -az-sims "$sims" -n "$games" -seed "$seed" -max-objs 2000 \
    -out "$OUT/values/$arm" 2>&1 | /usr/bin/grep -v skip | tail -20
"$PY" -I "$here/pl8_fit.py" "$OUT/values/$arm"
