# Repo map — `lab/`: draft events, oracles and training research

Part of the [repo map](repo-map.md) index. `lab/` holds the fenced
extensions ([ADR 0001](../decisions/0001-repo-layout.md) decision 2): nothing
outside `lab/` may import it (today `cmd/gorged` still links
`lab/draft/eventhost` and `lab/draft/limited/setdata`; the registration seam
removes that), and it is slated to move to its own repository. The bot
inference runtime the hosted bots need lives in `bots/`, not here.

## `lab/draft/`

E0's headless limited lab and the hosted draft events. Everything here is
pure or nearly so. Design:
[`docs/design/2026-10-13-draft-events-design.md`](../design/2026-10-13-draft-events-design.md),
with the E-series plans beside it in `docs/design/`.

| Package | What it owns |
|---|---|
| `lab/draft/limited` | The draft stack's shared vocabulary: `CardRef`, `Rarity`, the pool/receipt types every `lab/draft/limited/` package speaks. |
| `lab/draft/limited/setdata` | The embedded set specs (go:generate-refreshed JSON) and `Load`/`Codes`/`Support` over them. |
| `lab/draft/limited/booster` | Seeded Play-Booster opening; `OpenPod` opens a whole pod's packs from one seed. |
| `lab/draft/limited/draft` | The pure N-seat (2..8; hosted 4) booster-draft state machine. |
| `lab/draft/limited/build` | Pool → engine-loadable 40-card deck (`ToDeckFile`). |
| `lab/draft/limited/pickbot` | Pick and build policies a drafting bot is made of (`RatingColour`, `Best23`), registry with `Lookup`/`Names`. |
| `lab/draft/limited/draftbelief`, `draftfeat`, `draftnet` | Draft-RL pieces: opponent-pool inference from one seat's observations, pick features, the pick policy network ([`docs/design/2026-10-14-draft-rl-design.md`](../design/2026-10-14-draft-rl-design.md)). |
| `lab/draft/event` | The draft event state machine over an append-only, hash-chained log (`Apply`, `ReplayLog`), pairings, standings, the CSV draft log. Owns the replay-from-log contract `lab/cmd/draftsim -verify` asserts. |
| `lab/draft/eventhost` | Hosted draft events inside gorged: event store, lobby/join, draft with synchronous bots, per-event SSE frames. |
| `lab/draft/draftlab` | The headless lab's Go composition API: sealed pools, builders and players by ref, games and gauntlets as pure functions of (entrants, seed), whole events (`RunEvent`), 17Lands-shaped exports. |
| `lab/draft/draftlab/extern` (+`wire`, `refagent`) | Out-of-process agents over the `gorge-agent/1` JSON-lines wire; the reference agent. The protocol of record is the E5-d plan §4. |

## `lab/oracle/`

| Path | What it owns |
|---|---|
| `lab/oracle/compliance` (+`adopt`, `gate`, `levelb`, `oraclediff`, `oraclegen`, `shape`) | Declared-set compliance data, the gate, level-B requirements, scenario generation and XMage diff triage ([`docs/design/2026-10-02-xmage-compliance-oracle-design.md`](../design/2026-10-02-xmage-compliance-oracle-design.md), [`docs/design/2026-10-05-compliance-level-b.md`](../design/2026-10-05-compliance-level-b.md)). |
| `lab/oracle/xmageoracle/` | The Java XMage replay driver. |
| `lab/oracle/validation/oracle/` | The verdict-refresh runbook. |

## `lab/training/`

| Package | What it owns |
|---|---|
| `lab/training/searchbench` (+`statespec`) | Native search-benchmark replication: the sealed item manifest and scoring, `Materialize`, canonical options, 17lands label matching; a strict mirror of Draft Zero's StateSpec v1. |
| `lab/training/spellbench` (+`kshadow`, `registry`, `v1agent`, `v2agent`, `v2engine`, `v2shadow`) | gorge's side of the SpellBench arena; the v1/v2 bridge builds only with `-tags spellbench_bridge`. |
| `lab/training/v2front` | Front end V, kept out of `bots/policynet` so only the bridge tools link the v2 wire. |
| `lab/training/corpuscov` | Coverage census of a bot-played training corpus. |
| `lab/training/mzenc` | Byte-identical Go port of MageZero's state encoder (ablations only). |
| `lab/training/gamepool`, `inferbroker`, `memcap` | Tag-gated experimental runners and a cgroup memory-limit reader. |
| `lab/training/experimental/gamepool/` | A nested Go module (GPU scorer); its `purego` dependency stays out of the root `go.mod`. |

`lab/contracts` is the registry of external contracts (`lab/contracts/registry.json`; rules in `docs/contracts.md`).

## `lab/cmd/` and `lab/scripts/`

| Command | Purpose |
|---|---|
| `botbench` | Head-to-head policy evaluation: deck-pair matrices, CIs, decision traces, search/az front doors. |
| `searchteacher` / `searchprobe` / `searchbench` | PIMC label-corpus generator / search calibration / native search-study front door. |
| `policytrain` / `policytune` / `exitloop` / `encablate` | Train `policynet` / SPSA-fit cast weights / expert-iteration loop / encoding ablation dump. |
| `traindash` | Read-only training dashboard over `/mnt/sata/gorge-training` (`make traindash`). |
| `draftsim` / `labdemo` / `extagent` / `draftrl` / `cardval` | Headless draft pods (`-verify` replays every event) / the E5-c4 example workflow / the reference `gorge-agent/1` agent process (the Python example is `examples/extagent/`) / draft-RL driver / cold-start card values. |
| `compliance` / `oraclediff` / `oraclepacket` / `xmageport` | Compliance data maintenance / the XMage pipeline's gorge side / printed-card text for audit authors / XMage test → oracle-scenario translation. |
| `dzgorge` / `dzbroker` / `corpuscov` / `kshadowcheck` / `sbagent` / `sbv1agent` / `sbv2engine` / `sbv2local` / `sbv2shadow` | DraftZero self-play driver and the SpellBench bridge tools. |

`lab/scripts/` holds the oracle drivers (`xmage-oracle-*.sh`, `forge-oracle-*.sh`, `compliance-pass.sh`), training and arena drivers (`sb-gauntlet.sh`, `m1b-*`, `spellbench-arena/`, `searchbench/`, `encablate/`) and the PlayLines experiments (`playlines/`).
