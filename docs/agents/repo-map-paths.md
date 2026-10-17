# Repo map — every other path

Part of the [repo map](repo-map.md) index. The platform chain is in
[repo-map-engine.md](repo-map-engine.md), `internal/` in
[repo-map-internal.md](repo-map-internal.md), and `lab/` in
[repo-map-research.md](repo-map-research.md).

## `cmd/`

| Command | Purpose |
|---|---|
| `forgec` | Fetch and compile the Forge corpus; `report`, `coverage`. |
| `gorged` | The server: perpetual bot tables, vs-bot games, web client. |
| `mtgsim` | Headless self-play over the repo decks with replay verification (`make sim`). |
| `repro` | Replays a player feedback snapshot; `-emit-test` writes a failing test. |
| `headdiff` | Plays the `TestHeads` acceptance games; `-dump` records their event streams, `-against` names the first divergent event between two builds. |
| `cardfuzz` | Random mono-colour decks over the supported corpus; hunts panics, livelocks, divergences. |
| `enginebench` | The draft-zero docs/015 engine-speed rows (random/bot play, Clone, one step, search rates). `make enginebench-pair BASE=main CAND=.`, `make enginebench-profile`, `make enginebench-verify` (scripts: `scripts/enginebench-*.sh`). |
| `testbudget` / `gcgate` | Per-test wall and peak-RSS budget check (post-merge, `ops/scripts/postmerge_full.sh`); by-hand GC-share check. |
| `gentypes` | Regenerates `web/src/protocol.ts` (`make gentypes`; `-check` in lint). |
| `pkgmap` | The package map for the docs site; `-check` (a pre-commit hook) fails on an unclassified package. |
| `codeshape` | Prints `internal/codeshape`'s metrics. |
| `deckimport` | Plain-text decklist → repo deck JSON. |
| `ledger` | Rebuilds the derived issue ledger (`.ds4/ledger.json`). |
| `keywordbench` / `paymirror` | Keyword presence stats / payment A/B CLI. |

Research and draft commands are in `lab/cmd/` ([repo-map-research.md](repo-map-research.md#labcmd-and-labscripts)).

## `ops/`

The agent pipeline. Read [`ops/README.md`](../../ops/README.md) before
changing anything here.

| Path | What it is |
|---|---|
| `ops/orchestrator/hooks.py` | agentctl pipeline hooks, resolved as `ops.orchestrator.hooks:*`. Stdlib-only Python. |
| `ops/deploy/` | systemd units (seed timer, nightly timer, heavy/probe slices), installed as copies by `ops/scripts/seed-install.sh`. |
| `ops/scripts/` | Pipeline scripts: `postmerge_batch.sh`, `ratchet_gate.py`, `gate_affected.sh`, `heavy.sh`/`heavy_lock.sh`, the reward/seed agent, `fleet.sh` (port allocation), `cleanup.sh`, `deploy-demo.sh` (operator only), `reward_collect.py` (hot-file table) with `hotfiles-notes.json`. Smoke tests in `ops/scripts/tests/`. |
| `ops/restructure/restructure.py` | The ADR 0001 move script; re-run it on a stale branch to rebase across the restructure. |
| `.agentctl/config.toml` | agentctl pipeline config: tiers, gates, landing. Mirrored by agentctl's `consumers/gorge/config.toml`. |

## Everything else

| Path | What it is |
|---|---|
| `web/` | Svelte + Vite client. `web/src/protocol.ts` is **generated** — never hand-edit. |
| `docs/design/` | Living designs and contracts, flat, `YYYY-MM-DD-<slug>[-design].md`, plus `cast-payment-plans/` and `seat-deck-manifest/` brief sets. Deliberate contracts (not debt) are in `docs/design/2026-09-22-engine-contracts.md`. |
| `docs/design/ARCHIVE.md` | Index of the parked historical design records; their text is in git history and the operator's local `.superpowers/archive/`. |
| `docs/decisions/` | Numbered architecture decisions (`0001-repo-layout.md`). |
| `docs/site/` | The public MkDocs site (`mkdocs.yml`); `docs/site/architecture/packages.tiers.json` classifies every package. |
| `docs/agents/` | This guide. |
| `docs/contracts.md` | Rules for external contracts (`lab/contracts`). |
| `.githooks/` | pre-commit (runs `pkgmap-check`), commit-msg, pre-merge-commit, pre-push (`core.hooksPath=.githooks`). |
| `scripts/` | Contributor/CI only: `agent-worktree.sh` (the only sanctioned worktree creator), `ci_shard.sh`, `smoke.sh`, `smoke-perf.sh`, `enginebench-*.sh`, `docs_sets.py`. |
| `examples/extagent/` | A Python `gorge-agent/1` example agent. |
| `.github/workflows/` | `ci.yml`; `docs.yml` (builds the site, regenerates the package map); `coverage.yml` (on push to main: `make coverage`, publishes `.coverage/` as the `card-coverage` artifact; commits nothing). |
| `testkit/testutil/testdata/wall_exceptions.txt`, `rss_exceptions.txt` | Shrink-only inventories of tests over the per-test wall / RSS budget. |

## Untracked but load-bearing

| Path | Notes |
|---|---|
| `.cards/` | Forge corpus (`cardsfolder/`, `tokenscripts/`) and the IR cache `ir-<fingerprint>.gob.gz` with its per-card segment file `ir-<fingerprint>.seg` (written by `Registry.Save`, read by the subset loader). GPL-3.0 — never tracked. Without it, corpus tests **skip and read green**. |
| `.ds4/` | Pipeline scratch (issues, briefs, `ledger.json`). `TestNothingUnderDS4IsTracked` keeps it out of git. |
| `.superpowers/` | Orchestration records, seat prompt context and `archive/` (the parked design records). Linked into worktrees by agentctl. |
| `.worktrees/` | Sibling task checkouts. Exclude from searches or you get duplicate hits. |
| `gorged-data/` | Default server persistence. Never share one directory between servers. |
| `feedback/` | Player bug reports (agentctl intake). Excluded via `.git/info/exclude`, not `.gitignore`. |
| `cmd/repro/testdata/.tokens/` | GPL token text stripped from committed feedback fixtures. |
| `/mnt/sata/gorge-training` | Training corpora, checkpoints, `GOTMPDIR`. Never `/tmp` (RAM-backed). |
