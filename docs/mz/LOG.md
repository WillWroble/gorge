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
