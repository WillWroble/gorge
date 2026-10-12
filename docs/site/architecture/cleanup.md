# Cleanup roadmap

gorge grew quickly, and alongside the engine the repository carries several
research programmes (search, learned policies, benchmark replications), some of
which have concluded. This page lists code that is redundant, parked or
optional, with the evidence and the recommended direction. It is a roadmap,
not a commitment: each item is decided on its own.

*Survey of `main` @ `f1ac002c4`, 2026-10-12. Confidence: **H** high,
**M** medium, **L** low.*

## Priorities

1. **Restore a zero-dependency `go.mod`** (H). Move the tag-gated GPU game pool
   and decision broker into a nested module, and add an architecture test that
   fails on any `require` line.
2. **Remove dead weight** (H). Delete `patches/`, `internal/hindsight` with
   `cmd/hindsight`, and merge the three identical `bots/sb*` wrappers.
3. **Decouple the parked SpellBench stacks** (M). Cut the
   `internal/policynet` → `spellbench/v2agent` import, then move SpellBench v1
   and v2 (~21k lines) behind a build tag or to their branch. This also stops
   `cmd/gorged` linking research code.
4. **Regenerate the package map** (H) from `go list` doc strings, and correct the
   invariants document where it has drifted.
5. **Choose the surviving search harnesses** (L) on benchmark evidence.

## Documentation drift

| Item | Evidence | Direction |
|---|---|---|
| A third-party dependency exists | `go.mod` requires `github.com/ebitengine/purego v0.8.4` (added 2026-10-08), used only by `internal/gamepool/gpu_cuda.go` under `gamepool && gamepool_gpu`. The invariants say "no require lines", and nothing enforces it. | Nested module for gamepool, broker and `cmd/dzbroker`; archtest on `require`. (H) |
| Repo map is incomplete | About 57 of 122 packages are missing, including all of `compliance/*`, `azmcts`, `spellbench/*`, the ManaBrew packages and the `rules` sub-packages. | Generate it from `go list` doc strings. (H) |
| Stale limits in docs | `cmd/testbudget`'s doc still says 2 GB / 2 vCPU (now 4 / 4); the invariants say only the host and CLIs import `time`, but the archtest allowlist has 22 entries. | Correct the text. (H) |

## Parked or concluded programmes

| Candidate | Evidence | Direction |
|---|---|---|
| `internal/hindsight` + `cmd/hindsight` (~1.1k lines) | Last substantive change 2026-09-24 recorded a negative result; no Makefile or script references. | **Delete.** (H) |
| SpellBench v2 (`spellbench/{v2agent,v2engine,v2shadow}`, `cmd/{sbagent,sbv2engine,sbv2local,sbv2shadow}`, ~11.5k) | Development continues on a separate branch; the commands on `main` are last touched 2026-09-28. But `policynet/v2front_v.go` imports `v2agent`, so it is linked into `gorged`. | Cut the `policynet` import, then move the v2 stack to its branch or a module. (M) |
| SpellBench v1 (`v1agent`, `kshadow`, `cmd/sbv1agent`, `cmd/kshadowcheck`, ~9.8k) | Last touched 2026-09-28; self-contained; the commands have no tests. | Build tag or separate module. (M) |
| ManaBrew wire stack (~11k non-test) | Deprioritised, but both wires are kept as permanent; compiled into `gorged`; `cardfuzz` uses its mock client. | Keep; optionally tag the non-test code. (L) |
| `cmd/exitloop` (1.1k) | Last touched 2026-09-29; the expert-iteration line it drives did not beat the heuristic bot. | Delete unless retraining is planned. (L) |
| `cmd/dzgorge` + root `benchmark.md` | DraftZero-style self-play driver, still used by side experiments. | Keep the command; move the notes into `docs/`. (M) |

## Redundancy

| Overlap | Evidence | Direction |
|---|---|---|
| `patches/0001`–`0006` | 0001, 0002 and 0006 reverse-apply cleanly (already applied); 0005's file exists; 0003/0004 are obsolete; nothing references them. | **Delete.** (H) |
| `bots/sbfirst`, `bots/sbuniform`, `bots/sbheuristic` | Identical 65-line wrappers from one commit. | **Merge** into one `bots/sbbuiltin`. (H) |
| `bots` vs `internal/spellbench/registry` | Two policy registries. | Fold together, or make the second a thin decorator. (M) |
| `botpolicy` vs `spellbench/builtins` heuristics | An intentional replication of an external benchmark's bots. | Keep; document why. (M) |
| Four hidden-information search stacks (PIMC `searchprobe`+`searchseat`, PUCT `azmcts`, `spellbench/sbsearch`, `searchbench`; ~22k lines) | Overlapping functionality. | Decide survivors from [benchmark evidence](../performance/bots.md). (L) |
| `kshadow` vs `v2shadow` | Both rebuild engine state from observations. | Keep at most one. (M) |
| `policynet` features vs `mzenc` | Two state encoders; `mzenc` is used only for ablations and the GPU pool. | Decide after the ablation study. (L) |
| Two "brokers" | `scripts/broker.sh` (a cgroup CPU arbiter) and `internal/broker` (inference batching). | Rename one. (M) |

## Legitimately unimported

These have no importers by design and are **not** cleanup candidates:
`internal/archtest`, `internal/corpusheap` and `internal/botobs` (test-only
ratchets), `protocol/manabrew/internal/schemagen` (a generator), and
`internal/gamepool` / `internal/broker` (build-tag stubs).

## Commands with no Makefile or script reference

`corpuscov`, `deckimport`, `dzgorge`, `exitloop`, `hindsight`,
`keywordbench`, `kshadowcheck`, `oraclepacket`, `paymirror`, `policytune`,
`sbagent`, `sbv1agent`, `sbv2engine`, `sbv2shadow`, `searchbench`,
`searchprobe`, `searchteacher`, `xmageport`. Several are run by hand and are
fine; `sbv2local` and `gamepool` are referenced nowhere, and `sbv2local`,
`sbv2shadow`, `sbv2engine`, `hindsight` and `kshadowcheck` have not changed in
two weeks or more.

## Stray tracked files

`deck.list` (a 260 KB census scratch decklist at the repo root) and
`benchmark.md` should move under `docs/` or be removed. (M)
