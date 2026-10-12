# Packages

*Measured on `main` @ `f1ac002c4`, 2026-10-12: 120 packages in the default
build plus 2 tag-gated commands.*

Columns:

- **NT / T**: non-test / test lines of Go in that directory (not recursive).
- **Imp**: in-repo packages importing it from non-test code, plus test-only importers.
- **Last**: date of the last commit touching the directory. A large repo-wide
  sweep landed on 2026-10-06, so "10-06" often means "swept", not "worked on".
- **30d**: commits touching it (including subdirectories) since 2026-09-12.
- **Status**: **CORE** (the engine), **SUPP** (supports the product),
  **EXT** (optional extension: bots, search, ML, alternate wires),
  **TOOL** (tooling, tests, ratchets), **SUSP** (suspected obsolete) or
  **RED** (redundant). SUSP and RED entries are discussed on the
  [cleanup roadmap](cleanup.md).

For where a given kind of change belongs, the contributor-facing repo map is
[`docs/agents/repo-map.md`](https://github.com/adams-shaun/gorge/blob/main/docs/agents/repo-map.md).

## T0 card pipeline and T1 rules core

| Path | Purpose | NT / T | Imp | Last | 30d | Status |
|---|---|---|---|---|---|---|
| `cards` | Forge→IR compiler, keyword expanders, Registry, coverage | 14908/9825 | 61+13 | 10-12 | 474 | CORE |
| `cards/oracletext` | Printed-card view for Oracle audit | 66/49 | 1+1 | 10-06 | 2 | TOOL |
| `deck` | deck JSON → cards | 1102/1498 | 15+2 | 10-12 | 53 | CORE |
| `state` | objects, zones, players, Game, continuous-effect vocab | 5533/1341 | 51+5 | 10-10 | 485 | CORE |
| `decision` | Decision/Option/Intent; closed Kinds | 3467/3121 | 51+6 | 10-12 | 211 | CORE |
| `events` | event union, Apply, log, sha256 chain | 9541/4458 | 27+7 | 10-12 | 561 | CORE |
| `effects` | `api:` primitives, evaluators, effects.Host | 78619/64227 | 19+2 | 10-12 | 2259 | CORE |
| `effects/params` | compiled ability-param records | 1306/302 | 2+0 | 10-06 | 7 | CORE |
| `rules` | engine: turns, priority, stack, SBAs, layers, casting; ratchets; heads | 124514/417681 | 37+13 | 10-12 | 4907 | CORE |
| `rules/chars` | CR 613 layer walk | 2061/178 | 2 | 10-12 | 28 | CORE |
| `rules/combat` | attack/block legality | 1809/241 | 1 | 10-10 | 17 | CORE |
| `rules/cost` | cost vocabulary leaf | 3549/555 | 6 | 10-10 | 17 | CORE |
| `rules/pay` | payment planner | 11915/351 | 2 | 10-09 | 53 | CORE |
| `rules/resolve` | resolution kernel | 693/0 | 3+1 | 10-03 | 15 | CORE (no own tests) |
| `rules/scriptfacts` | printed/script facts | 393/122 | 1 | 10-06 | 2 | CORE |
| `rules/trigmatch` | trigger Mode$ matchers | 5624/400 | 1 | 10-09 | 47 | CORE |
| `replay` | re-execute (Config, Log); first divergence | 353/919 | 8+5 | 10-06 | 10 | CORE |
| `botpolicy` | heuristic bot (`Decide`), cast profiles | 9753/11523 | 28+6 | 10-12 | 251 | CORE |

## T2 projection, host and wire

| Path | Purpose | NT / T | Imp | Last | 30d | Status |
|---|---|---|---|---|---|---|
| `view` | redacted per-seat projection | 4546/7314 | 41+5 | 10-08 | 238 | CORE |
| `seat` | decision answerers (Bot, policynet, explore) | 1778/4745 | 32+6 | 10-07 | 78 | CORE |
| `protocol` | versioned wire types | 390/287 | 6 | 10-06 | 45 | SUPP |
| `host` | tables, sessions, snapshots, persistence, undo | 6095/15217 | 4+2 | 10-12 | 175 | SUPP |
| `host/httpapi` | JSON + SSE + embedded web | 949/3071 | 2 | 10-10 | 28 | SUPP |
| `host/manabrewhttp` | ManaBrew transport (`-manabrew`) | 937/2237 | 1 | 10-06 | 14 | EXT |
| `internal/manabrew` | view ↔ ManaBrew DTO | 4325/4935 | 3 | 10-06 | 45 | EXT |
| `internal/manabrew/mbtest` | mock MB client (used by cardfuzz) | 2366/2486 | 1 | 10-06 | 19 | EXT |
| `protocol/manabrew` | MB wire types | 3154/691 | 3+1 | 10-06 | 10 | EXT |
| `protocol/manabrew/internal/schemagen` | schema generator | 296/33 | 0 | 10-06 | 2 | TOOL |

## T3 bots, ML and search

| Path | Purpose | NT / T | Imp | Last | 30d | Status |
|---|---|---|---|---|---|---|
| `bots` | closed registry of hosted policies | 276/296 | 16 | 10-06 | 23 | SUPP |
| `bots/all` | links every built-in | 15/0 | 3+2 | 09-29 | 6 | SUPP |
| `bots/bot`, `castprofile`, `lethalpressure` | thin registrations | ~25 each | 1–2 | 09-28 | 1 | SUPP |
| `bots/search` | L10 search-teacher adapter | 133/302 | 2 | 10-06 | 6 | EXT |
| `bots/azredeal` | AZ-MCTS adapter | 198/366 | 2 | 10-06 | 7 | EXT |
| `bots/sbtactical` | SpellBench tactical adapter | 169/365 | 3+1 | 10-06 | 6 | EXT |
| `bots/sbsearch` | sb-search adapter | 287/634 | 2 | 10-06 | 8 | EXT |
| `bots/sbfirst`, `sbuniform`, `sbheuristic` | identical 65-LOC wrappers | 65 each | 3 | 09-29/10-06 | 1–2 | RED |
| `internal/policynet` | pure-Go learned policy | 7981/4454 | 10 | 10-12 | 48 | EXT (linked into seat) |
| `internal/traceboard` | decision-trace schema | 139/0 | 3 | 09-30 | 2 | EXT |
| `internal/searchprobe` | world sampler + PIMC | 8817/8434 | 13+5 | 10-08 | 99 | EXT (linked into host) |
| `internal/searchseat` | PIMC decision fn + Feed | 1197/1723 | 15+1 | 10-06 | 37 | EXT (linked into host) |
| `internal/azmcts` | PUCT search | 4338/4062 | 10+2 | 10-08 | 48 | EXT |
| `internal/azmcts/clairvoyant` | cheating world source | 78/44 | 3 | 10-06 | 3 | EXT |
| `internal/bench` | policy-agnostic game runner | 991/455 | 7+6 | 10-08 | 27 | TOOL/SUPP |
| `internal/hindsight` | hindsight branching | 375/113 | 1 | 10-06 | 3 | SUSP |
| `internal/mzenc` | MageZero encoder port | 1465/1226 | 1 | 10-12 | 27 | EXT |
| `internal/corpuscov` | bot-corpus coverage census | 908/56 | 2 | 10-10 | 1 | TOOL |
| `internal/paymirror` | planned vs manual payment A/B | 4581/1865 | 3 | 10-12 | 133 | TOOL |
| `internal/searchbench` (+`statespec`) | Draft Zero search-bench replication | 6099/2586, 718/46 | 1 | 10-06 | 54 | EXT |
| `internal/spellbench` | deck catalogs | 184/346 | 7+3 | 10-06 | 98 | EXT |
| `internal/spellbench/builtins` | SpellBench bots port | 6071/1374 | 11+2 | 10-06 | 29 | EXT |
| `internal/spellbench/sbsearch` | determinized search | 931/329 | 5+1 | 10-06 | 14 | EXT |
| `internal/spellbench/payexec` | plan → manual mana | 706/499 | 3 | 10-06 | 4 | EXT |
| `internal/spellbench/registry` | botbench SpellBench names | 3010/2894 | 1 | 10-06 | 13 | EXT (2nd registry) |
| `internal/spellbench/v1agent` | protocol v1 agent | 5687/1403 | 4 | 10-06 | 15 | SUSP |
| `internal/spellbench/kshadow` | v1 shadow engine | 3607/217 | 2 | 10-06 | 15 | SUSP |
| `internal/spellbench/v2agent` | v2 agent (imported by policynet) | 2191/1285 | 7 | 10-06 | 7 | SUSP |
| `internal/spellbench/v2engine` | v2 env role | 4362/741 | 3+1 | 10-06 | 12 | SUSP |
| `internal/spellbench/v2shadow` | v2 shadow engine | 4043/223 | 2 | 10-06 | 12 | SUSP |
| `internal/gamepool` | GPU game pool (`-tags gamepool`; pulls `purego`) | 1219/133 | 0 | 10-08 | 4 | EXT |
| `internal/broker` | decision broker (`-tags broker`) | 395/171 | 0 | 10-08 | 2 | EXT |

## T4 tooling

| Path | Purpose | NT / T | Imp | Last | 30d | Status |
|---|---|---|---|---|---|---|
| `internal/archtest` | import-graph + determinism ratchets | 5/1920 | 0 | 10-08 | 76 | TOOL (load-bearing) |
| `internal/codeshape` | function-length / interface-width ratchet | 1471/1198 | 1 | 10-12 | 125 | TOOL |
| `internal/testbudget` | per-test RSS/wall recorder | 94/118 | 1+98 | 10-06 | 2 | TOOL |
| `internal/testutil` | corpus registry, repo decks, AGENTS ratchet | 692/832 | 15+41 | 10-12 | 266 | TOOL |
| `internal/testutil/feedback` | snapshot loader (`EngineAt`) | 503/210 | 1+2 | 10-06 | 33 | TOOL |
| `internal/corpusheap` | heap-footprint ratchet | 0/248 | 0 | 10-12 | 8 | TOOL |
| `internal/botobs` | reward-loop observability ratchet | 60/398 | 0 | 10-06 | 7 | TOOL |
| `internal/divergence` | human-divergence capture | 513/600 | 1 | 10-12 | 2 | TOOL |
| `internal/tsgen` | Go → TS for web protocol | 580/119 | 1 | 10-06 | 4 | TOOL |
| `compliance` | declared-set manifests from XMage | 818/664 | 7+2 | 10-12 | 1102 | TOOL |
| `compliance/oraclegen` | scenario generator | 7197/7941 | 4+1 | 10-12 | 804 | TOOL |
| `compliance/oraclegen/templates` | Level-B templates | 29966/29661 | 2+2 | 10-12 | 676 | TOOL |
| `compliance/levelb` | IR → level-B requirements | 3234/2680 | 3+2 | 10-10 | 102 | TOOL |
| `compliance/oraclediff` | gorge vs XMage diff | 1089/1207 | 3 | 10-09 | 31 | TOOL |
| `compliance/gate` | claim check | 629/668 | 2+1 | 10-12 | 27 | TOOL |
| `compliance/adopt` | set-adoption queue | 1639/756 | 1+1 | 10-09 | 14 | TOOL |
| `compliance/shape` | disagreement signatures | 679/331 | 2 | 10-10 | 7 | TOOL |

## Commands (`cmd/`)

No package imports a command.

| Group | Commands (NT LOC, last, 30d) | Status |
|---|---|---|
| Product & acceptance | `gorged` (3407), `forgec` (463), `mtgsim` (276), `repro` (633), `headdiff` (310), `gentypes` (88), `deckimport` (867) | CORE/TOOL |
| Engine QA | `cardfuzz` (2597), `keywordbench` (506), `paymirror` (245), `autopayaudit` (1056, tag), `enginebench` (1784), `gcgate` (246), `testbudget` (510), `codeshape` (118) | TOOL |
| Compliance | `oraclediff` (2801), `compliance` (139), `xmageport` (3572), `oraclepacket` (95) | TOOL |
| Pipeline | `ledger` (561) | TOOL |
| Bot bench & training | `botbench` (7310), `policytrain` (2972), `policytune` (853), `searchteacher` (1419), `searchprobe` (225), `traindash` (1429), `corpuscov` (296), `encablate` (419) | EXT |
| Research, possibly dead | `exitloop` (1121, 09-29), `hindsight` (727, KILL 09-24), `searchbench` (1453), `dzgorge` (3094, 0 tests) | SUSP/EXT |
| SpellBench | `sbagent`, `sbv1agent`, `kshadowcheck`, `sbv2engine`, `sbv2local` ("wip"), `sbv2shadow` | SUSP (no tests except sbagent; no Makefile refs) |
| Tag-gated | `dzbroker` (`-tags broker`), `gamepool` (`-tags gamepool`) | EXT |

## Non-Go top-level paths

| Path | What | Status |
|---|---|---|
| `web/` (462 files) | Svelte+Vite client (~68k LOC) | SUPP |
| `scripts/` (101) | worktree/fleet/postmerge/ratchet/broker/heavy/reward/oracle/enginebench/spellbench-arena | TOOL (spellbench-arena EXT) |
| `tools/xmageoracle/` | Java XMage replay driver | TOOL |
| `validation/oracle/` | verdict refresh runbook | TOOL |
| `orchestrator/` | `hooks.py` merge-gate hooks for the CI-style landing pipeline | TOOL (live) |
| `deploy/` | systemd units | TOOL |
| `patches/` | DraftZero patches "@ gorge c9a422b7" | RED |
| `docs/` (167) | specs, plans, reports, agent guide | SUPP |
| `deck.list` (root, 260 KB) | census scratch decklists | SUSP |
| `benchmark.md` (root) | `dzgorge` benchmark notes | SUSP |

