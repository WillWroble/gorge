# Repo map — what lives where

A lookup table for agents and new contributors. One line per package or
directory, plus where to go when you need to change a particular kind of
thing. Verified against the tree on 2026-10-16 (after the
[ADR 0001](../decisions/0001-repo-layout.md) restructure, `go list ./...` and
`ls`); where this file and the code disagree, the code wins — fix this file in
the same commit. `go run ./cmd/pkgmap` prints every package with its purpose
and tier, generated from the source.

See also: [invariants.md](invariants.md) (the rules every change must keep)
and [do-not.md](do-not.md) (the mistakes that have actually happened here).

This is the index. The package and path rows live in the per-domain files
below; this file changes only when a GROUP or DOMAIN appears, not when a
package does.

## Groups

| Group | What it holds | Detail |
|---|---|---|
| `engine/` | The rules core: `cards`, `deck`, `state`, `decision`, `events`, `effects`, `rules` (+ sub-packages), `replay`, `view`, and `seat` (the `Seat` contract only). Imports nothing outside `engine/`. | [repo-map-engine.md](repo-map-engine.md#engine) |
| `bots/` | `botpolicy`, the default `heuristic` bot, the bots registry and the hosted bots with their inference runtime. Imports `engine/` only. | [repo-map-engine.md](repo-map-engine.md#bots) |
| `server/` | `host` (+`httpapi`, `manabrewhttp`), `protocol`, `manabrew`, `divergence`. | [repo-map-engine.md](repo-map-engine.md#server) |
| `internal/` | Architecture ratchets, test support, shared bench runner, generators. | [repo-map-internal.md](repo-map-internal.md) |
| `cmd/` | Product and developer binaries. | [repo-map-paths.md](repo-map-paths.md#cmd) |
| `lab/` | Fenced extensions: draft events, compliance oracles, training research, their commands and scripts. Nothing outside `lab/` may import it (ADR 0001 decision 3; enforced by `internal/archtest/lab_fence_test.go`; `cmd/gorged` composes `lab/draft` only under `-tags lab`). | [repo-map-research.md](repo-map-research.md) |
| `ops/` | The agent pipeline: agentctl hooks, systemd units, pipeline and gate scripts. See [`ops/README.md`](../../ops/README.md). | [repo-map-paths.md](repo-map-paths.md#ops) |
| `scripts/` | Contributor and CI helpers only. | [repo-map-paths.md](repo-map-paths.md#everything-else) |
| `web/` | The Svelte + Vite client. | [repo-map-paths.md](repo-map-paths.md#everything-else) |
| `docs/` | This guide, living designs (`docs/design/`), decisions (`docs/decisions/`), the public site (`docs/site/`). | [repo-map-paths.md](repo-map-paths.md#everything-else) |

## Where do I change…

| I want to… | Go to |
|---|---|
| Implement an `api:` effect | `engine/effects/`, `effects.Register("Name", fn)` in an `init()`. If it needs more than `effects.Host` offers, it is rules work. |
| Implement a keyword/trigger/static/replacement | `engine/rules/`, `effects.RegisterNonAPI("kw:X", …)`. A keyword that expands to script lines is a `engine/cards/kw_*.go` expander instead (moves the fingerprint). A trigger mode's matcher goes in `engine/rules/trigmatch/`. |
| Add a `count:` value head | The evaluator arm in `engine/effects/` **and** `effects.modelledValueHeads` (`engine/effects/value_heads.go`; `TestValueHeadRegistryMatchesEvaluator`). |
| Add an event kind | Append to `engine/events/event.go` after the last constant and add its one `kindInfo` entry in `engine/events/kindinfo.go` (name, trigger class, optional Describe template). Behaviour (`events.Apply`, a custom `view.Describe` case) is separate. |
| Add a decision kind | Don't, without an operator decision — the set is closed; every seat, bot, client and the protocol must answer it. |
| Change the default bot's decisions | `bots/botpolicy/` (the one policy, reading a `Board`); its seat adapter is `bots/heuristic/bot.go`. |
| Add a bot policy | Bench-only: `lab/cmd/botbench`. Hosted: register it in the `bots/` registry (`bots/registry.go`, linked by `bots/all`) and the host's closed vocabulary `server/host/bot_policy.go`. See the README's *Bot player training and adoption guidelines* and [`docs/design/2026-09-28-hosted-bot-packages.md`](../design/2026-09-28-hosted-bot-packages.md). |
| Add a repo deck | `internal/testutil/decks/*.json` via `cmd/deckimport`; it must be fully supported (the acceptance ratchet). |
| Turn a player report into a test | `go run ./cmd/repro -emit-test <pkg> <feedback-dir>`. |
| Change the wire | `server/protocol/`, then `make gentypes`. |
| Change an external file, wire or grammar (dzgorge outputs, `.dzgw`, visit corpus) | Its owner package, then `lab/contracts/registry.json`; the rules are in `docs/contracts.md`. |
| Change a seat's projected view | The family file for the seam: a card/battlefield projection in `engine/view/card.go`, a stack projection in `engine/view/stack.go`. Only an assembler change (`Project`/`project`) belongs in `engine/view/view.go`, which two projection tickets once collided on. |
| Move or add a package | Classify it in `docs/site/architecture/packages.tiers.json` in the same commit (`go run ./cmd/pkgmap -check`, a pre-commit hook). |
| Change the pipeline (gates, hooks, units, postmerge) | `ops/` — read [`ops/README.md`](../../ops/README.md) first; the gates fetch two of its scripts from the base commit. |
| Add or change a living design | `docs/design/`. Historical records are parked; see [`docs/design/ARCHIVE.md`](../design/ARCHIVE.md). |
