# mz-gorge log

Every number goes here with the command, the machine and the commit.

## 2026-10-20 — Baseline (before any change)

- **Commit:** `52590c85` (branch `mz-gorge`, upstream gorge `311e9a0a` + CLAUDE.md and design doc)
- **Machine:** cloud container, 4 vCPU Intel Xeon @ 2.10GHz, 15 GiB RAM, Linux 6.18, Go 1.25.8
- **Corpus:** adams-shaun/forge `098b0553` (`FORGE_REF`): 34074 cards and 854 tokens, with 2 compile diagnostics
  (`imoen_trickster_friend`, `karlach_raging_tiefling`: unsupported `K:Specialize` riders)

### Setup

```
make fetch-cards compile-cards     # 32 s wall
```

### Test suite

```
make test GO_TEST_FLAGS="-p=2 -json"   # = GOGC=200 go test -p=2 -json ./...  (GOMEMLIMIT=5GiB, GOFLAGS=-p=2 -trimpath)
```

`-json` was added only to record per-test results. `-p=2` is the Makefile default.

| | count |
|---|---|
| Wall / user / sys | 9m59.7s / 28m50.3s / 3m1.1s |
| Packages pass / no tests | 63 / 13 |
| Packages fail | 0 |
| Top-level tests pass | 11012 |
| Top-level tests skip | 29 |
| Top-level tests fail | 0 |

The per-test result (`pass|skip <package> <Test>`) is in [`baseline-tests.txt`](baseline-tests.txt).
Every one of the 29 skips is an opt-in measurement, golden-generation or network test that skips by design.

The test suite has no autotap/manual-tap switch, so it runs once and covers both modes. A per-mode determinism
baseline (event-chain heads per seed, on the mage fork's decks) is part of stage 1.

## 2026-10-20 — Stage 1 item 0: decks, regression heads, cost baseline

- **Commit:** working tree on `e6f3a85e`, plus `bots/mz/decks` and `cmd/mzgames` (this commit). No engine change.
- **Machine:** the same 4 vCPU Xeon @ 2.10GHz container. The load was the measurement itself (nothing else running).

### Decks

All 48 decks in the mage fork's `Mage.MageZero/decks` are in `bots/mz/decks`, converted with `cmd/deckimport` after dropping XMage's
`[SET:num]` tags and sideboard lines. Three card names needed their diacritics restored to match the corpus: Lórien Revealed
(pauper-cawgates, highnooncontrol) and Troll of Khazad-dûm (pauper-spy). `go test ./bots/mz/decks` checks that every deck resolves card for card.

### Regression heads

```
go test ./cmd/mzgames -run TestGoldenHeads      # -update rewrites testdata/heads.txt
```

The two pairings from the fork's configs: pauper-faeries vs pauper-cawgates (`Mage.MageZero/config`) and uwtempo vs
standard-monob (`Mage.Tests/config`). Seeds 1–8, gorge's `bot` in both seats, autotap and manual tap: 32 games pinned
by event-chain head, in `cmd/mzgames/testdata/heads.txt`.

### Cost per search-node operation (baseline, no search yet)

```
go build -o bin/mzgames ./cmd/mzgames
bin/mzgames -pair <a>,<b> -games 16 -cost 300
```

At the first real choice of turns 5–8 in each game (cmd/enginebench's root rule): `clone` is `CloneInto`+`Release`; `step` is a clone plus
`Submit` of the bot's own answer; `view` is the omniscient `view.ProjectFor`. ns per op, median/mean over the roots.

| tap | pair | roots | clone | step | view |
|---|---|---|---|---|---|
| auto | pauper-faeries vs pauper-cawgates | 63 | 14896 / 15989 | 50672 / 60021 | 16093 / 17773 |
| manual | pauper-faeries vs pauper-cawgates | 63 | 14195 / 15662 | 38490 / 44603 | 15144 / 17489 |
| auto | uwtempo vs standard-monob | 64 | 14985 / 15662 | 46621 / 49817 | 12158 / 12849 |
| manual | uwtempo vs standard-monob | 64 | 14129 / 14466 | 40439 / 39198 | 12087 / 12618 |

## 2026-10-20 — Stage 1 items 1–3: measurements

- **Commit:** `dcad75a5` (stage 1 items 0–3 on `mz-gorge`)
- **Machine:** the same 4 vCPU Xeon @ 2.10GHz container. Timings were taken with nothing else running; the sweeps ran single-threaded, two at a time.

### Test suite

```
make test GO_TEST_FLAGS="-p=2 -json"
```

66 packages pass and 13 have no tests; 11022 top-level tests pass, 29 skip, none fail. Every test that passed at the baseline
still passes. 10 tests are new (`bots/mz/...`, `cmd/mzgames`, `TestEngineStateSkipIsCurrent`, `TestViewCoversState`). 39
packages were served from Go's test cache, which is valid for unchanged inputs, so the wall time is not comparable to the baseline.

### Item 1 — view sufficiency (principle 1)

```
bin/mzgames -pair <a>,<b> -games 2 -suff
```

At every priority decision, up to 3 pairs of offered actions are played in both orders on hypothetical clones (with
deterministic follow-ups). Where both orders reach the same dynamic omniscient view (`view.ProjectDynamic`), both clones get
the same future chance and fresh identical seats and are played 80 decisions forward. Any later difference in the view means
some state was missing from it.

| sweep | games | pairs | transpositions | divergent |
|---|---|---|---|---|
| 23 pairings over 46 of the fork's decks, both tap modes | 92 | 47952 | 41254 | 0 |

Before the fixes in `a60749e8` and `714b6691` the probe found three representation gaps: an embedded game, engine and log
snapshot (unless-cost context, legend-rule batch), event-log positions (`Seq`, `SourceZoneSeq`) and payment-action IDs.

### Item 2 — autotap action set (principle 3)

```
bin/mzgames -pair <a>,<b> -games 4 -gap [-offer]
```

At autotap priority windows (gorge's `bot`, auto-pay), these are the castable casts (`Decision.PotentialCasts`) offered by
neither a payment plan nor a pool-payable cast option.

| sweep | priority windows | windows with an unoffered cast | unoffered casts |
|---|---|---|---|
| 23 pairings, 4 games each, without `OfferPotentialCasts` | 46106 | 1225 (2.7%) | 1824 |
| with `OfferPotentialCasts` (the four worst pairings) | 9083 | 0 | 0 |

With the opt-in, each extra cast was played on a clone, with gorge's bot answering what followed: ubartifacts vs uw-control
997 offered, 698 cast, 299 reversed, 0 failed. uwtempo vs wills-tempo 317 / 124 / 193 / 0. standard-monow vs standard16-5c
257 / 117 / 140 / 0. pauper-rally vs pauper-spy 274 / 229 / 45 / 0. The reversals come from gorge's bot answering the CR 601.2g
mana window, not from the engine; mz-gorge's own tapper is stage 2.

### Item 3 — atomic decisions (principle 2)

```
bin/mzgames -pair <a>,<b> -games 4 -atomic
```

Both seats pick at random, one atomic pick at a time, through `Stage`/`Decline`/`Commit`. In autotap they make no mana taps,
and the 601.2g window is answered by gorge's bot.

| sweep | games | aborted | decisions | atomic picks | rejected composites |
|---|---|---|---|---|---|
| 23 pairings, both tap modes | 184 | 0 | 191063 | 191756 | 38 (0.02%) |

ns per atomic step (it includes the engine's advance to the next decision): 13.6k–160k across pairings, median about 37k.

### Cost per search-node operation

```
bin/mzgames -pair <a>,<b> -games 16 -cost 300
```

The baseline's columns are unchanged within noise (the engine changes are opt-in). `dynamic_view` is the new
`view.ProjectDynamic`. All values are ns per op, median / mean.

| tap | pair | clone | step | view | dynamic_view |
|---|---|---|---|---|---|
| auto | pauper-faeries vs pauper-cawgates | 15449 / 16080 | 51279 / 63808 | 16417 / 18935 | 97913 / 99952 |
| manual | pauper-faeries vs pauper-cawgates | 13737 / 14631 | 35706 / 42726 | 16734 / 17864 | 93863 / 97611 |
| auto | uwtempo vs standard-monob | 15832 / 16734 | 47119 / 50361 | 12925 / 13330 | 93984 / 96713 |
| manual | uwtempo vs standard-monob | 14424 / 15485 | 40353 / 42008 | 13018 / 13741 | 99926 / 102487 |

The dynamic view is about 6× the plain one: a fresh projection (no `Into` reuse), plus the reflected engine state and fact maps.
It is the first candidate if stage 2's sims/s needs it.

## Stage 2: first search measurements (partial)

Commit `0c2ede69` (local branch `mz-gorge-stage2`), cloud container: 4 vCPU, 15 GB, linux/amd64, Go 1.25.8. Offline only.

```
go build -o mzgames ./cmd/mzgames
mzgames -mz -pair pauper-faeries,pauper-cawgates -games 4 -seed 1 -tap both -budget 1000   # prune on, timeout 1h
```

mz is seat 0 (pauper-faeries) against gorge's `bot`. The run was stopped by the 30-minute background limit after 6 of 8
games; the uwtempo/standard-monob pairing did not run.

| seed | tap | turns | winner | searches | sims | sims/s | µs/sim | reused | illegal | dup | replaced |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | auto | 14 | mz | 99 | 39404 | 1071 | 934 | 92 | 685 | 2650 | 129 |
| 2 | auto | 34 | bot | 229 | 80840 | 852 | 1174 | 213 | 2651 | 3500 | 163 |
| 3 | auto | 44 | bot | 305 | 115485 | 475 | 2107 | 285 | 2114 | 34714 | 618 |
| 4 | auto | 14 | mz | 92 | 35752 | 1052 | 951 | 88 | 1120 | 3267 | 85 |
| 1 | manual | 12 | mz | 194 | 73862 | 1057 | 946 | 190 | 0 | 11877 | 1964 |
| 2 | manual | 32 | bot | 473 | 120560 | 873 | 1146 | 459 | 0 | 18494 | 2483 |

No game stalled and no search needed a rebuild. Win rate so far 3/6. A search step (one iteration of the loop) costs about
0.9–2.1 ms. A budget-100 profile puts about 60% of it in `encode.Of` (the dynamic view and the hash walk) and most of the rest
in engine steps. The slow game (seed 3) has a large tree: the loop recomputes tree size, depth and legality on every
iteration, as `applyMCTS` does.

## Stage 2: search speed and budget-1000 games

Commit `4537d290` (`mz-gorge-stage2`), same container (4 vCPU, 15 GB, linux/amd64, Go 1.25.8). Offline only. `make test`
(`GO_TEST_FLAGS="-p=2 -json"`): 68 packages pass, 13 have no tests; 11025 top-level tests pass, 29 skip, none fail.

```
go build -o mzgames ./cmd/mzgames
for p in pauper-faeries,pauper-cawgates uwtempo,standard-monob; do for t in auto manual; do
  mzgames -mz -pair $p -games 4 -seed 1 -tap $t -budget 1000     # prune on, timeout 1h: the budget ends every search
done; done
```

mz is seat 0 (the first deck) against gorge's `bot`. `engine` is the share of search time spent copying a node's engine and
stepping it to the next node (gorge's own work); `hash` is the share spent hashing nodes; `cost` is search time over engine
time, the factor mz adds over the engine work a sim needs.

| pair | seed | tap | turns | winner | searches | sims | sims/s | µs/sim | engine | hash | cost | illegal | dup | replaced |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| pauper-faeries-vs-pauper-cawgates | 1 | auto | 14 | mz | 99 | 39404 | 1589 | 630 | 0.53 | 0.37 | 1.9× | 685 | 2650 | 129 |
| pauper-faeries-vs-pauper-cawgates | 2 | auto | 34 | bot | 229 | 80840 | 1140 | 877 | 0.56 | 0.35 | 1.8× | 2651 | 3500 | 163 |
| pauper-faeries-vs-pauper-cawgates | 3 | auto | 44 | bot | 305 | 115485 | 659 | 1517 | 0.55 | 0.36 | 1.8× | 2114 | 34714 | 618 |
| pauper-faeries-vs-pauper-cawgates | 4 | auto | 14 | mz | 92 | 35752 | 1337 | 748 | 0.52 | 0.36 | 1.9× | 1120 | 3267 | 85 |
| pauper-faeries-vs-pauper-cawgates | 1 | manual | 12 | mz | 194 | 73862 | 1663 | 601 | 0.48 | 0.39 | 2.1× | 0 | 11877 | 1964 |
| pauper-faeries-vs-pauper-cawgates | 2 | manual | 32 | bot | 473 | 120560 | 1316 | 760 | 0.48 | 0.40 | 2.1× | 0 | 18494 | 2483 |
| pauper-faeries-vs-pauper-cawgates | 3 | manual | 44 | bot | 715 | 1477581 | 823 | 1215 | 0.47 | 0.41 | 2.1× | 0 | 506715 | 11271 |
| pauper-faeries-vs-pauper-cawgates | 4 | manual | 18 | mz | 296 | 151634 | 1649 | 606 | 0.47 | 0.39 | 2.1× | 0 | 33183 | 6534 |
| uwtempo-vs-standard-monob | 1 | auto | 11 | bot | 70 | 24824 | 1633 | 613 | 0.59 | 0.32 | 1.7× | 701 | 752 | 24 |
| uwtempo-vs-standard-monob | 2 | auto | 18 | bot | 146 | 40875 | 1368 | 731 | 0.62 | 0.30 | 1.6× | 962 | 1410 | 139 |
| uwtempo-vs-standard-monob | 3 | auto | 10 | bot | 114 | 39099 | 1411 | 709 | 0.57 | 0.33 | 1.8× | 137 | 1709 | 433 |
| uwtempo-vs-standard-monob | 4 | auto | 13 | bot | 72 | 23879 | 1983 | 504 | 0.61 | 0.30 | 1.6× | 164 | 639 | 99 |
| uwtempo-vs-standard-monob | 1 | manual | 9 | bot | 121 | 27797 | 1934 | 517 | 0.49 | 0.38 | 2.0× | 0 | 4020 | 915 |
| uwtempo-vs-standard-monob | 2 | manual | 10 | bot | 137 | 35537 | 2509 | 399 | 0.50 | 0.37 | 2.0× | 0 | 3254 | 781 |
| uwtempo-vs-standard-monob | 3 | manual | 10 | bot | 150 | 35280 | 1954 | 512 | 0.49 | 0.39 | 2.0× | 0 | 6237 | 819 |
| uwtempo-vs-standard-monob | 4 | manual | 11 | bot | 145 | 33118 | 1966 | 509 | 0.50 | 0.36 | 2.0× | 0 | 6903 | 1048 |

- No game stalled and no search needed a rebuild. mz won 4 of 8 with pauper-faeries and 0 of 8 with uwtempo.
- A search costs 1.6–2.1× the engine work its sims need; tree bookkeeping is about 1%. The rest is the hash (the dynamic
  view and the walk), which stage 3's encoder replaces.
- The speed work (hash plans and buffers, `EngineState`'s fields worked out once, facts built only when set, a reused
  view, recycled engines) changed no game: the seed 1 and 2 autotap chains match the partial run above. It took the
  budget-100 run from about 1,000 to 1,700 sims/s.
- Slow sims (seed 3) are long games with big boards: the engine share is unchanged, so the engine step itself costs more.
- Faeries seed 3 under manual tap ran 1.48M sims over 715 searches: with half a million duplicates pruned, a pruned old
  node takes its visits off the root, so the budget takes more iterations to reach, as in `applyMCTS`.
