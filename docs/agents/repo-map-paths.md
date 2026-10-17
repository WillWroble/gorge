# Repo map — every other path

Part of the [repo map](repo-map.md) index. The platform chain is in
[repo-map-engine.md](repo-map-engine.md), `internal/` in
[repo-map-internal.md](repo-map-internal.md). The former `lab/` tree is in
four-yay (private); its map is four-yay's `docs/agents/repo-map-research.md`.

## `cmd/`

| Command | Purpose |
|---|---|
| `forgec` | Fetch and compile the Forge corpus; `report`, `coverage`. |
| `gorged` | The server's thin main over `server/gorged` (perpetual bot tables, vs-bot games, web client), linking gorge's `bots/all`. four-yay's `cmd/gorged` is the full demo server (draft events, research bots), deployed from four-yay. |
| `mtgsim` | Headless self-play over the repo decks with replay verification (`make sim`). |
| `repro` | Replays a player feedback snapshot; `-emit-test` writes a failing test. |
| `headdiff` | Plays the `TestHeads` acceptance games; `-dump` records their event streams, `-against` names the first divergent event between two builds. |
| `cardfuzz` | Random mono-colour decks over the supported corpus; hunts panics, livelocks, divergences. |
| `enginebench` | The draft-zero docs/015 engine-speed rows (random/bot play, Clone, one step, search rates). `make enginebench-pair BASE=main CAND=.`, `make enginebench-profile`, `make enginebench-verify` (scripts: `scripts/enginebench-*.sh`). |
| `testbudget` / `gcgate` | Per-test wall and peak-RSS budget check (the post-merge full suite); by-hand GC-share check. |
| `gentypes` | Regenerates `web/src/protocol.ts` (`make gentypes`; `-check` in lint). |
| `pkgmap` | The package map for the docs site; `-check` (a pre-commit hook) fails on an unclassified package. |
| `codeshape` | Prints `internal/codeshape`'s metrics. |
| `deckimport` | Plain-text decklist → repo deck JSON. |
| `keywordbench` / `paymirror` | Keyword presence stats / payment A/B CLI. |

Research and draft commands (`botbench`, `draftsim`, `dzgorge`, `policytrain`, …) and the pipeline's issue `ledger` moved to four-yay's `cmd/` (private); the compliance commands (`oraclediff`, `compliance`, `xmageport`, `oraclepacket`) to gorge-compliance's `cmd/` (public).

## The agent pipeline (not in this repo)

gorge carries no agent-pipeline or demo-deploy machinery. The maintainers'
agentctl config, gate hooks, systemd units, post-merge batch, heavy-job lock,
reward loop and seed agent, worktree tooling (`agent-worktree.sh`), the
policy git hooks, the demo deploy and the ADR 0001 move scripts moved to the
private four-yay repository's `ops/` on 2026-10-16 (its `ops/README.md`).

## Everything else

| Path | What it is |
|---|---|
| `web/` | Svelte + Vite client. `web/src/protocol.ts` is **generated** — never hand-edit. |
| `docs/design/` | Living designs and contracts, flat, `YYYY-MM-DD-<slug>[-design].md`, plus `cast-payment-plans/` and `seat-deck-manifest/` brief sets. Deliberate contracts (not debt) are in `docs/design/2026-09-22-engine-contracts.md`. |
| `docs/design/ARCHIVE.md` | Index of the parked historical design records; their text is in git history and the operator's local `.superpowers/archive/`. |
| `docs/decisions/` | Numbered architecture decisions (`0001-repo-layout.md`). |
| `docs/site/` | The public MkDocs site (`mkdocs.yml`); `docs/site/architecture/packages.tiers.json` classifies every package. |
| `docs/agents/` | This guide. |
| `.githooks/` | pre-commit and pre-merge-commit (run `pkgmap-check`), pre-push (the opt-in `RACE=1` race gate on a push of main) (`core.hooksPath=.githooks`). |
| `scripts/` | Contributor/CI only: `ci_shard.sh`, `smoke.sh`, `enginebench-*.sh`, `docs_sets.py`. (`smoke-perf.sh` and `agent-worktree.sh` moved to four-yay.) |
| `.github/workflows/` | `ci.yml`; `docs.yml` (builds the site, regenerates the package map); `coverage.yml` (on push to main: `make coverage`, publishes `.coverage/` as the `card-coverage` artifact; commits nothing). |
| `testkit/testutil/testdata/wall_exceptions.txt`, `rss_exceptions.txt` | Shrink-only inventories of tests over the per-test wall / RSS budget. |

## Untracked but load-bearing

| Path | Notes |
|---|---|
| `.cards/` | Forge corpus (`cardsfolder/`, `tokenscripts/`) and the IR cache `ir-<fingerprint>.gob.gz` with its per-card segment file `ir-<fingerprint>.seg` (written by `Registry.Save`, read by the subset loader). GPL-3.0 — never tracked. Without it, corpus tests **skip and read green**. |
| `.ds4/` | Agent scratch (issues, briefs, `ledger.json`), written by the maintainers' tooling. `TestNothingUnderDS4IsTracked` keeps it out of git. |
| `.superpowers/` | Orchestration records, seat prompt context and `archive/` (the parked design records). |
| `.worktrees/` | Sibling task checkouts. Exclude from searches or you get duplicate hits. |
| `gorged-data/` | Default server persistence. Never share one directory between servers. |
| `feedback/` | Player bug reports. Excluded via `.git/info/exclude`, not `.gitignore`. |
| `cmd/repro/testdata/.tokens/` | GPL token text stripped from committed feedback fixtures. |
| `/mnt/sata/gorge-training` | Training corpora, checkpoints, `GOTMPDIR`. Never `/tmp` (RAM-backed). |
