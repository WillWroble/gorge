# PL-8: pilot-matched card values for drafting

Status: experiment BUILT, smoke-verified; full sweep not yet run (orchestrator's, serial, heavy).

## Question
Card strength measured by one pilot does not transfer to another (G0 under the az-c4 pilot:
bot-measured values 0.430, 17Lands 0.448, az-c4-measured 0.471). Is az-c4/sims=25/hlogit
already saturated, or does a better-matched pilot (az-c5 net, more sims) still add strength?
It also builds the exact path S3 (values recomputed from drafted-deck games) needs.

## Method
1. `scripts/playlines/pl8_values.sh` runs `cmd/cardval -az -az-net c5.dzgw -az-sims S -hands
   -mode random -set FDN -n N -max-objs 2000` (random decks, in-hand recording) then
   `pl8_fit.py` fits gih/iwd/hlogit/deck by exec'ing `cardval/analyze.py`'s own `hand_values` /
   `regression_values` unchanged (no re-implementation), so the numbers are comparable to the
   az-c4-s25 control. It also prints split-half reliability.
2. Values feed the drafter through `ovr:PATH#FIELD` (`cmd/draftsim/ratings_override.go` /
   `draftrl gate -drafters`), exactly as G0's `run-az.sh` did.
3. `scripts/playlines/pl8_gauntlet.sh <arm>`: `draftrl gate -pilot az -az-net c5.dzgw -az-sims 25
   -seed 301 -events 24 -games 4` (6144 games per arm). The control is re-run on the new seeds.
4. `scripts/playlines/pl8_compare.py`: Spearman of each set to c4 hlogit / bot gih / 17Lands,
   the 20 cards moving most in rank each way (with Oracle text pulled from `.cards` for the
   human-written reason), gauntlet win rate with Wilson 95%, difference to control, verdict.

Seeds: G0 used `draftrl gate -seed 201 -events 24` = event seeds 201-224 (g0/az-*.log: "seeds
201-224"; run-az.sh). PL-8 uses `-seed 301` = 301-324. No overlap. (Value-measurement
`cardval` seeds: c4-s25 used 61; PL-8 uses 71 and 72.)

Caveat on pairing: `gate.json` stores per-seat tallies, not per-event ones, so the compare
script uses the UNPAIRED difference SE. Events share packs/seeds across arms, so the true
paired SE is smaller or equal: the stated bounds are conservative for PURSUE and a
slightly harder bar for ABORT. Making `draftrl gate` emit per-event tallies is a separate change
(not done; out of scope).

## Pre-registered rule (verbatim from the ticket)
- PURSUE (pilot-matching still matters, adopt c5-measured values as the S0 prior, run S3 with
  them) if some c5-measured value set beats the az-c4 hlogit control by >= +1.5 pp (paired
  seeds, lower 95% bound > 0).
- ABORT (saturated: stop value-measurement work; S3 on the c4-hlogit prior) if every c5 set is
  within +/-1.0 pp of control with upper bound < +1.5 pp.
- INCONCLUSIVE otherwise (report games needed).
- Secondary table: pilot dependence = 1 - Spearman(c5 values, c4 values); if < 0.05 the two
  pilots agree on card order and the remaining gap is not about card strength.

## Run commands (orchestrator, serial, under scripts/heavy.sh on /mnt/sata)
Defaults: PL8_OUT=/mnt/sata/gorge-training/playlines/pl-8, 16 workers. Values runs use
MemoryMax 24G / GOMEMLIMIT 16GiB (cardval's memcap refuses anything under 1.4x); gauntlets
use 12G / 10GiB as G0 did.

| # | arm | command | games | est. wall |
|---|---|---|---|---|
| 1 | values c5 sims25 | `scripts/playlines/pl8_values.sh c5s25 25 20000 71` | 20000 | ~50 min |
| 2 | values c5 sims100 | `scripts/playlines/pl8_values.sh c5s100 100 6000 72` | 6000 | ~60 min |
| 3 | gauntlet c4 hlogit (control) | `scripts/playlines/pl8_gauntlet.sh c4hlogit` | 6144 | ~12 min |
| 4 | gauntlet c5s25 hlogit | `scripts/playlines/pl8_gauntlet.sh c5s25hlogit` | 6144 | ~12 min |
| 5 | gauntlet c5s100 hlogit | `scripts/playlines/pl8_gauntlet.sh c5s100hlogit` | 6144 | ~12 min |
| 6 | gauntlet c5s25 gih | `scripts/playlines/pl8_gauntlet.sh c5s25gih` | 6144 | ~12 min |
| 7 | gauntlet 17Lands + random baseline | `scripts/playlines/pl8_gauntlet.sh 17l` | 6144 (+ random-deck baseline) | ~12-20 min |
| 8 | verdict | `/mnt/sata/gorge-training/cardval/venv/bin/python -I scripts/playlines/pl8_compare.py` | - | seconds |

Total about 3 h (limit 5 h). Steps 1-2 must precede 4-6. Wall estimates: values rate measured in
smoke at 2.4 CPU-s/game at sims 25 (100 games, 2 workers, 129 s wall), scaled to 16 cores
(~6.6 g/s; the ticket's 10 g/s is optimistic) and x4 for sims 100; gauntlet from G0 (721 s).
Caution: 6000 sims-100 games may give a noisy hlogit fit (check the `split-half hlogit` line
printed by step 2 before spending gauntlet time; if < ~0.5, raise N for that arm).

## Smoke (seat, caps: 4G / 200% / GOMAXPROCS 2 / timeout 300, outputs in .ds4/scratch/pl8)
- `PL8_OUT=.ds4/scratch/pl8 WORKERS=2 MEMMAX=4G GOMEM=2 CPUQ=200% TIMEOUT=300 scripts/playlines/pl8_values.sh c5s25 25 100 71`:
  "done: 100 games, 0 errors", az panics 0, values.json written, 129 s wall. split-half rho ~ -0.05
  (expected noise at 100 games; the fit path is what is verified).
- `... EVENTS=1 GAMES=1 MAXHUMAN=2 scripts/playlines/pl8_gauntlet.sh c5s25hlogit` (and c4hlogit):
  8 games each, seeds 301-301, panics 0, 20 s wall.
- `pl8_compare.py` on the smoke outputs printed rho table, rank movers with Oracle text, the
  gauntlet table, differences and `VERDICT: INCONCLUSIVE (~33080 games ...; have 8)`.
- `/mnt/sata` is read-only inside the seat jail, so smokes could not use the production dir.
- `.cards` was present (symlink) in this worktree.

## Results
(empty until the orchestrator fills it)

## Verdict
(pending)

## If PURSUE, the next ticket is
Adopt the winning c5-measured value set as the S0 prior and run S3 (recompute values from
drafted-deck games, DR-10) with it; reuse `pl8_values.sh`/`pl8_fit.py` on drafted-deck games.

## Issues
- `draftrl gate`'s `gate.json` has no per-event tallies, so a truly paired SE is not computable
  (see caveat). Hypothesis: adding per-event tallies to gate.json is a small change in
  `cmd/draftrl/gate.go`; not done here.
- `pl8_fit.py` depends on `/mnt/sata/gorge-training/cardval/analyze.py` (outside the repo) and
  asserts its last line is `main()`.
- pandas is not in the cardval venv (ticket said it is); scripts use numpy/scipy only.
