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
