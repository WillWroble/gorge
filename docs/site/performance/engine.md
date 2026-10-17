# Engine speed

Engine throughput matters twice over: once for hosting games, and again
because search bots clone and step the engine thousands of times per
decision. This page covers what is measured, how to run it, and the current
numbers. *Compiled 2026-10-12 against `main` @ `f1ac002c4` from existing
benchmark results; nothing here was re-run for this page.*

## What is measured

| Tool | Measures | Output |
|---|---|---|
| `cmd/enginebench` (one row) | Fixed workloads: `random` and `bot` play (turns/s, games/s, decisions/s per CPU-second); `clone` and `step` (µs/op with and without GC, bytes and mallocs per op, retained bytes per clone); `az` and `sampler` search rows (sims/s, ms per root); live heap after corpus load | one JSON line per run |
| `make enginebench-pair BASE= CAND=` | A paired, interleaved ABBA comparison of two revisions, each built from `git archive` | JSONL plus a text table: per-row median throughput and the median per-repetition candidate/base ratio |
| `make enginebench-profile [REV=]` | CPU and allocation pprof per row, and GC share | pprof files |
| `make enginebench-verify [REV=]` | A memo-verifying build; fails on errors, stalls, panics or stale memos | pass/fail |
| `make sim` | 20 verified four-seat games | replay OK count |
| `make gc-gate` | GC CPU share of one package's tests (default `./server/host`, budget 0.30) | ratio |

Bot win-rate evaluation and the whole-game smoke run from a private companion
repository, not from gorge.

The `enginebench` workloads use four 17lands Foundations (FDN) draft decks
(CC BY 4.0) and an mtg-kernel Burn deck (MIT), in
`cmd/enginebench/testdata/decks`. Pair A is UG vs WG, pair B is WR vs UR.

### How to run

```sh
make enginebench-pair BASE=main CAND=.          # BUDGET=60s per row, REPS=2 by default
make enginebench-pair BASE=main CAND=. BUILD_ARGS="-tags enginebench_az" ROWS="-row az -sims 100"
make enginebench-profile REV=.
make enginebench-verify REV=.
```

Knobs: `BUDGET`, `ROWS`, `REPS`, `SECS`, `GOGC`, `BENCH_DIR`, `HEAVY_LOCK`.

!!! warning "Noise"
    The reference machine is a shared 32-thread Ryzen 9 9950X (Go 1.26.3)
    whose 1-minute load average is usually 10–80. A single reading moves
    ±15–30% and a per-repetition ratio ±10–15%. **Only paired, interleaved
    A/B ratios are trustworthy**; treat every absolute number below as
    indicative.

## Current numbers

Latest full row set: 2026-10-07, `GOMAXPROCS=1`, 60 s per row, load average
34–80. The binary was built from approximately `49c8224ce` (inferred) with the
`az` tag.

| Row | Value |
|---|---|
| Random play, FDN pair A / B | 3,237 / 3,129 turns/s (75.7 / 77.8 games/s; ~85k decisions/s) |
| Random play, Burn | 5,482 turns/s (154.8 games/s) |
| Heuristic bot, manual mana taps, A / B | 121.1 / 80.8 games/s |
| Heuristic bot, auto-pay, A / B | 57.3 / 56.5 games/s |
| Clone, mid-game | 16.5 µs without GC / 35.7 µs with GC; 144 KB/op |
| Step (clone + submit) | 74.6 µs / 120.8 µs; 535 KB/op |
| MCTS (`azmcts`, honest redeal), 100 / 1000 sims | 595 / 468 sims per CPU-second (172 ms / 2.18 s per root) |
| Old rollout sampler | 62 sims per CPU-second |
| Live heap after corpus load | 41 MB |

A newer 100-sim MCTS measurement with `GOMAXPROCS=32` on a quieter machine
gave 1,389–1,410 sims per CPU-second (2026-10-09); it is not comparable with
the single-core row above.

Whole-game heuristic-bot sweep, 2026-09-27 (`f4b21c036` → `55975c912`), mean
CPU milliseconds per game over 29 decks:

| Format | Manual taps | Auto-pay |
|---|---|---|
| Constructed | 43 → 28 | 105 → 42 |
| Commander | 75 → 53 | 239 → 75 |
| 4 / 6 / 8 seats | 282 / 514 / 930 → 176 / 285 / 495 | 516 / 958 / 1,693 → 218 / 371 / 611 |

## Trajectory

| Date | Commit(s) | Workload | Result |
|---|---|---|---|
| 2026-09-21 | `4744d8a7`, `1c485b7d` | PIMC search teacher | 496 → 171 ms per decision at 4 workers |
| 2026-09-27 | `a60dfcd9e` | bot sweep | −16.6% sweep CPU; `rules` tests 50 s → 15 s |
| 2026-09-27 | `bc14fd36a`, `43a60ba67`, … | bot sweep | manual taps −24…32%, auto-pay −22…47% |
| 2026-09-30 | `a4af596` vs `e0496f062` | `enginebench`, pinned | `main` 7–25% slower than the published baseline; live heap 143 MB; top hotspot an index rebuild at 12.4% |
| 2026-09-30 → 10-01 | `308f7e0b9` → `9226f882e` | SpellBench-style MCTS games | MCTS 3.9× less CPU / 4.9× less wall; other search seats 5–8× |
| 2026-10-05 | `08f248918` | `rules` test gate | −44% CPU, −47% allocation |
| 2026-10-06 | `a6107d0df`, `d55a968a8`, `a3b5123a3`, `b75197f98`, `3f88c20d0` | corpus image, event log, decision arena | live heap 143 → ~41 MB. **Baseline break:** random/bot rows now recycle games through a spare pool |
| 2026-10-07 | ~`49c8224ce` | `enginebench`, `GOMAXPROCS=1` | vs 2026-09-30: random A 4.3×, Burn 2.9×, bot A 3.5×, auto-pay 2.3×, MCTS-100 1.8×, MCTS-1000 2.7× (crosses the baseline break; directional only) |
| 2026-10-07 | heap-attack branch | self-play | +10.8% combined; allocated objects −30% |
| 2026-10-09/10 | quiet-machine series | MCTS-100 | +10.4% median |

The engine-speed method and the first pinned comparison are written up in
[`2026-09-30-gorge-engine-speed.md` (archived)](https://github.com/adams-shaun/gorge/blob/main/docs/design/ARCHIVE.md).

## Tracking over time

A plan to record a nightly anchored series and chart it on this site is
described on [Tracking over time](tracking.md).
