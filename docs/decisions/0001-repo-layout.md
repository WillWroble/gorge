# 0001 — Repository layout: engine / server / bots / lab / ops

- **Status:** accepted 2026-10-16 (operator). Execution: phases below.
- **Supersedes:** the flat-root layout and README's "Package layout" chain.

## Why

The root holds 43 tracked entries (31 Go package dirs) plus ~15 untracked
ones. Five audiences share one flat namespace — the rules engine, the
reference server, bot/AI research, compliance oracles and the agent
pipeline — with near-collisions (`event`/`events`, `bots`/`botpolicy`,
`host`/`eventhost`, `tools`/`scripts`, `compliance`/`validation`). An outside
contributor fixing a card (PR #3, `rules/mayplay.go`) has no way to tell the
platform from the research, and the docs are written for agent seats.

## Decisions

1. **Audience.** The repo is laid out for an outside contributor first.
   Agent-seat operating rules move under `ops/`; AGENTS.md becomes thin.
2. **Identity.** gorge is an extensible platform: engine, reference server,
   web client, baseline and hosted bots, developer tooling. Draft/limited
   events, compliance oracles and scripted train/retrain/iterate research are
   *extensions*: fenced under `lab/` now, moved to another repo later.
3. **Fence.** Nothing outside `lab/` imports `lab/`. `internal/archtest`
   enforces it; `scripts/ci_shard.sh`'s research exclude regex seeds the list.
4. **Hosted bots: inference in the platform.** The runtime half of the hosted
   bots (MCTS search, net forward pass, belief, sb builtins, searchseat)
   moves into `bots/`. Trainers, corpus builders and calibration benches go to
   `lab/training/`. `policynet` and `searchprobe` are split along that line.
5. **Draft in gorged: seam + lab composition.** The seam is the `lab` build
   tag: `cmd/gorged/events.go` and `art_sets.go` (the draft runtime and set
   art prewarm) build only with `-tags lab`; the untagged binary links no
   `lab/` package and refuses `-events` (`TestEventsNeedAPlugin`).
   `make deploy-demo`, the per-ticket gate, the full suite and CI build with
   `-tags lab`. `server/protocol/draftevent.go` stays: it is wire types the
   web client generates from and imports nothing from `lab/`.
6. **`seat` is a contract, bots are injected.** `seat` keeps only `Seat`,
   `PaymentPlanConsumer`, `TargetHintConsumer` and imports only `decision` +
   `view`; it lives at `engine/seat`. `BoardSeat` (names `botpolicy.Board`)
   moves to `bots`. `seat.Bot` → `bots/heuristic`, `PolicyNetBot` →
   `bots/policynet`. `host` receives a `seat.Factory` (the `bots` registry)
   injected by its command instead of importing concrete bots. This also
   breaks today's `seat → botpolicy`, `bots → seat` cycle.
   *As built:* the contract is `engine/seat` (`Seat`, `PaymentPlanConsumer`,
   `TargetHintConsumer`); `Bot`, `PolicyNetBot` and `BoardSeat` live in
   `bots/heuristic` (botpolicy may not import view, so it cannot own an
   interface that embeds `seat.Seat`). `server/host` already resolves bots
   through the `bots` registry; a narrower factory interface is follow-up.
7. **Engine prefix now.** The core moves under `engine/` in the same window as
   the other Go moves, before any external consumer pins import paths.
8. **Dead material is deleted**, not archived; git history keeps it.
9. **Historical design records leave the tree.** `docs/superpowers/` is parked
   in a gitignored local directory, linked into worktrees like `.superpowers`;
   living content is rewritten into `docs/architecture/` and this
   `docs/decisions/` series.

## Target root

```
README.md CONTRIBUTING.md AGENTS.md LICENSE NOTICE Makefile go.mod
engine/    cards state decision events effects rules view replay deck seat
server/    host (+httpapi) protocol manabrew divergence feedback
bots/      policy (was botpolicy), registry, heuristic, policynet, hosted inference
web/       TS client
cmd/       product + developer tools only
internal/  archtest testutil corpusheap
lab/       draft/ oracle/ training/ cmd/ scripts/
ops/       orchestrator deploy scripts docs     (.agentctl stays at root)
docs/      contributing/ architecture/ decisions/ site/
```

## As built (2026-10-16)

All phases landed in one sitting on branch `wt/restructure`, driven by
`ops/restructure/restructure.py` (stage `go`, then `--stage=ops`) plus hand
fixes for what a path rewrite cannot see: archtest/codeshape short package
names, gate trigger regexes, smoke fixtures, Python path joins. The fence is
`internal/archtest/lab_fence_test.go`. `packages(t)`-style checks that build
`module + "/" + name` from short names would have gone silently inert; they
were found by reading archtest, not by a failing test -- re-check that class
when moving packages again.

## Phases (as planned)

Every move phase ships a committed, idempotent move script (`git mv`, import
rewrite, `goimports`, path rewrite in docs / AGENTS.md / seat prompt files),
lands with the queue drained, and runs the full suite. A stale branch is
rebased by re-running the script on it.

- **P0 — hygiene and onboarding (no moves, no pause).**
  - Delete: `cmd/autopayaudit`, `cmd/manamodelstats`, `deck.list` (+ its
    `.gitattributes` rule), unreferenced scripts (`compliance-printed.py`,
    `forge-oracle-test-{cast,setup}.sh`,
    `xmage-oracle-test-{driver-contract,forge-chunks}.sh`, `az-stage0.sh`,
    `m1b-paired.py`, `manamodel_ab_report.py`, `manamodel_report.py`,
    `encablate/summarize_keep.py`, `searchbench/{plot,report_tables}.py`,
    `spellbench-arena/*` except `rate.py`, `tests/test_perf_record.py`) —
    each re-checked for references before deletion. Untracked: both
    `.cards.old-*`, `.agentctl/config.toml.bak-*`, root `gorged`.
    `.gitignore` gains `feedback/`.
  - README: fix the stale claims (test budget, orchestrator, dependency
    chain, hosted-bot vocabulary, spellbench-prep markers, fork source,
    `make test`, ratchet location, missing `core.hooksPath`); move the bot
    training section to `docs/research/`.
  - New: `CONTRIBUTING.md`; `docs/contributing/{first-card-fix,testing,
    glossary,card-pipeline}.md`. first-card-fix follows PR #3 and also the
    no-Go oracle-scenario route (`rules/testdata/oracle/`, `make oracle-audit`).
  - `doc.go` for `rules/` and `effects/`; consolidate the 42 duplicate
    corpus-lookup test helpers into one shared file.
- **P1 — cut fence edges (code, no moves).** Decisions 4–6: split
  `policynet`/`searchprobe`; move `Bot`/`PolicyNetBot` out of `seat`; host
  `seat.Factory` injection; gorged registration seam; land the archtest lab
  rule as a shrink-only allowlist.
- **P2 — Go moves, one drain window.** `lab/`, `bots/`, `server/`,
  `engine/`, `internal/` trim (`tsgen`/`codeshape`/`testbudget` beside their
  commands). Rename `event` → `draftevent`, `botpolicy` → `bots/policy`.
- **P3 — `ops/` move.** Hazards: the gates fetch `scripts/ratchet_gate.py`
  and `scripts/gate_affected.sh` from `{base}` (config lands in the move
  commit, then daemon restart); `orchestrator.hooks:*` renamed in
  `.agentctl/config.toml` and agentctl's `consumers/gorge/config.toml`;
  30 scripts locate the root by `dirname/..`; re-run `seed-install.sh` and
  relaunch the four transient units; `core.hooksPath` is absolute;
  `.worktrees` (12 GB) moves out of the repo via `worktrees_dir`.

## Not in scope

Splitting the flat `engine/rules/` directory (the lasagna work owns it). Moving
`lab/` to its own repository.

## Amendment (2026-10-16): the lab tree moves to four-yay

- **Status:** accepted 2026-10-16 (operator), the same day as the layout
  above. Supersedes decision 2's "moved to another repo later", decisions 3
  and 5, and the *Not in scope* line about moving `lab/`. Everything above
  this section is the record as first written.

**What moved.** The fenced `lab/` tree left gorge for a new **private**
module and repository, four-yay (`github.com/adams-shaun/four-yay`, checked
out beside gorge as `../four-yay`). Its `go.mod` requires gorge and replaces
it with the sibling checkout. Paths drop the prefix: gorge's `lab/X` is
four-yay's `X` (`draft/`, `oracle/`, `training/`, `contracts/`, `cmd/` with
the 27 research binaries such as `botbench`, `draftsim`, `oraclediff`,
`compliance` and `dzgorge`, and `scripts/`). With it went the hosted
research bots: `az-c3`/`az-c4`/`az-c5` (`bots/azc3..5`), `cast-profile`
(`bots/castprofile`), `sb-heuristic`/`sb-uniform`/`sb-first`
(`bots/sbbuiltin`), `sb-search-lite-atk` (`bots/sbsearch`,
`bots/spellbench/sbsearch`), `search` (`bots/search`), the clairvoyant world
source (`bots/azmcts/clairvoyant`, now four-yay's `bots/clairvoyant`),
`examples/extagent`, `NOTICE` (it covered only the XMage-derived compliance
manifests), the 15 lab design docs (four-yay's `docs/design/`), and the
agent docs that described only lab content (`docs/agents/repo-map-research.md`
and `docs/contracts.md`, now under four-yay's `docs/`).

**What gorge keeps.** The hosted bots `bot`, `lethal-pressure`,
`sb-tactical` and `az-redeal` (with the `azmcts` search core) and their
support packages: `botpolicy`, `heuristic`, `sanity`, `policynet`, `gonet`,
`traceboard`, `searchseat`, `searchprobe`, `spellbench` (the SpellBench deck
catalogs, moved from `lab/training/spellbench` to `bots/spellbench`, with
their generator `bots/spellbench/fdn-catalog.py`), `spellbench/builtins`,
`spellbench/payexec` and `bots/all`.

**The seams that replace the fence.** gorge never imports four-yay; four-yay
composes gorge from outside:

- `server/gorged` is the importable server (`Main`, `Serve`). Its
  `Extensions{Events EventsPlugin}` carries the draft-event runtime
  (four-yay's `draft/gorgedevents`); with no plugin, `-events` is refused at
  startup (`TestEventsNeedAPlugin`). `RegisterNetBot` links the az-c3/c4/c5
  implementations in; gorged keeps their names, flags and closed deck-list
  machinery. `cmd/gorged` is a thin main over it that links gorge's
  `bots/all`; four-yay's `cmd/gorged` is the full demo server.
- `bots.Register` is unchanged; four-yay's `bots/all` links gorge's
  `bots/all` plus its own bots.
- `testkit/` (`testutil`, `bench`, `paymirror`) and `cli/` moved out of
  `internal/` so an outside module can import them.

**What replaced the enforcement.** The fence is now structural: the root
`go.mod` has no `require` lines (`TestRootModuleHasNoRequires`), so gorge
cannot import four-yay. `internal/archtest/lab_fence_test.go` and the check
that engine tests never import the compliance packages were deleted as
vacuous, and the `lab` build tag is gone from gorge, its CI and its gates.

**Process changes.**

- The Makefile targets for the moved tools went with them (`botbench`,
  `draftsim`, `dzbroker`, `test-broker`, `gamepool`, `traindash`,
  `compliance-manifests`/`-pass`/`-status`/`-sets`, `xmage-oracle-setup`,
  `forge-oracle-setup`, `test-spellbench-bridge`, `smoke-perf`;
  `scripts/smoke-perf.sh` too, which now compares two gorge revisions through
  four-yay's `botbench`). gorge's `make deploy-demo` builds four-yay's
  `gorged` (`FOURYAY_ROOT`, default the `../four-yay` sibling of gorge's main
  checkout).
- gorge drops its compliance gate (operator decision); four-yay's CI checks
  gorge for compliance.
- The docs workflow no longer measures set support (`oraclediff` is
  private); the public sets page shows its hand snapshot, and
  `scripts/docs_sets.py` stays in gorge as the renderer.
- Some contract goldens are owned by gorge packages (`bots/policynet`'s
  azvisits, `bots/gonet`'s dzgw header, `server/protocol`'s frames) while
  four-yay's contract registry pins their hashes, so changing one is a
  coordinated change across both repositories.
