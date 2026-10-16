# Architecture overview

*Measured on `main` @ `6d95c6a54`, 2026-10-16, after the repository
restructure ([ADR 0001](https://github.com/adams-shaun/gorge/blob/main/docs/decisions/0001-repo-layout.md)),
from `go list ./...` and the import graph.*

## The core idea

Card behaviour is not hand-written per card. Forge's card scripts (fetched,
never committed) are compiled by the `engine/cards` package into an IR. The rules
core (`engine/state`, `decision`, `events`, `effects`, `rules`) implements the
primitives the IR references: `api:` effects, `trig:` trigger modes, `kw:`
keywords, `stat:` static abilities, `count:` value expressions and so on. A
card is *playable* when every primitive it uses is implemented.

All state mutation goes through `events.Apply`, which appends to a
SHA-256 hash-chained log. Combined with a seeded RNG and no wall-clock reads,
that makes a match a pure function of `(Config, intents)`:

- **replay** re-executes the log and stops at the first divergent event;
- **resume** after a restart is a replay;
- **undo** and **search clones** reuse the same machinery.

Players never see the engine. Each seat receives a redacted `view.View` and
answers a `decision.Decision` drawn from a closed set of kinds (priority,
targets, attackers, blockers, trigger order, modes, mulligan, and so on).

## Groups

The repository is grouped by audience. Each group imports only the groups
above it in this table, with one exception noted below.

| Group | Contents | Role |
|---|---|---|
| `engine/` | `cards` (+`oracletext`), `deck`, `state`, `decision`, `events`, `effects` (+`params`), `rules` and its sub-packages, `replay`, `view`, `seat` | T0 card pipeline (Forge script → IR) and T1 rules core, the per-seat projection, and the `Seat` contract (`seat` imports only `decision` and `view`) |
| `bots/` | `botpolicy`, `heuristic` (the default `Bot`, `BoardSeat`, `PolicyNetBot`), the registry (`bots`, `bots/all`), hosted bots (`azc3`/`azc4`/`azc5`, `sbtactical`, ...) and their inference runtime (`azmcts`, `gonet`, `policynet`, `searchprobe`, `searchseat`, `traceboard`, `spellbench/{builtins,payexec,sbsearch}`) | every bot a table can seat |
| `server/` | `host` (+`httpapi`, `manabrewhttp`), `protocol`, `manabrew`, `divergence` | tables, persistence, HTTP + SSE, the wire protocols |
| `internal/` | `archtest`, `testutil`, `corpusheap`, `bench`, `paymirror`, `codeshape`, `testbudget`, `tsgen`, `botobs` | architecture ratchets, test support, generators |
| `cmd/` | `gorged`, `forgec`, `repro`, `mtgsim`, `pkgmap`, `codeshape`, `testbudget`, ... | product and developer binaries; nothing imports a command |
| `lab/` | `draft/`, `oracle/`, `training/`, `contracts`, `cmd/`, `scripts/` | fenced extensions: draft events, compliance oracles, training research; slated for their own repository |
| `web/` | the Svelte+Vite client | embedded into `cmd/gorged` |

```mermaid
flowchart TB
  subgraph E["engine/"]
    cards["cards, deck"]
    state["state"] --> cards
    decision["decision"] --> state
    events["events (Apply, log, hash chain)"] --> decision
    effects["effects (+params, rules/cost)"] --> events
    sub["rules/chars, combat, trigmatch, pay, resolve, scriptfacts"] --> effects
    rules["rules (Engine)"] --> sub
    replay["replay"] --> rules
    view["view"] --> events
    seat["seat (contract)"] --> view
  end
  subgraph B["bots/"]
    botpolicy["botpolicy"] --> effects
    heuristic["heuristic"] --> botpolicy
    heuristic --> seat
    searchseat["searchseat, searchprobe, azmcts"] --> rules
    searchseat --> heuristic
    registry["bots registry, bots/all, hosted bots"] --> searchseat
  end
  subgraph S["server/"]
    protocol["protocol"] --> view
    host["host"] --> rules
    host --> registry
    host --> protocol
    httpapi["host/httpapi, manabrew"] --> host
  end
  subgraph C["cmd/"]
    gorged["cmd/gorged"] --> httpapi
  end
  subgraph L["lab/"]
    draft["draft/eventhost, limited"]
    oracle["oracle/compliance, training/*"] --> rules
  end
  gorged -. "-tags lab" .-> draft
```

Arrows point from an importer to the package it imports. The dashed edge exists
only in a `-tags lab` build: `cmd/gorged`'s draft-event wiring (`events.go`,
`art_sets.go`) is the one composition point, and the untagged binary links no
`lab/` package (ADR 0001 decision 5; `internal/archtest/lab_fence_test.go`). The generated tier graph on the [Packages](packages.md) page is the
authority.

## Size

About 517k lines of non-test Go and 733k lines of tests. `engine/` is 281k /
533k of that; the `engine/rules` directory alone is 130k non-test lines.
`lab/` holds 145k / 99k, `bots/` 47k / 43k, `server/` 21k / 35k.

## Observations

1. **The rules core is clean.** `go list -deps ./engine/rules` reaches 15
   in-repo packages, all under `engine/`. The dependency direction is enforced
   by `internal/archtest` (see [Invariants](invariants.md)).
2. **`botpolicy` is the shared board model.** It lives in `bots/` and imports
   only `engine/`; an architecture test forbids it from importing `view`,
   `rules` or `seat`. Every search seat uses it as the opponent and
   environment model.
3. **The engine no longer links bots.** The old `seat → policynet` and
   `seat → botpolicy` edges are gone: `engine/seat` is the contract only, and
   the concrete bots live in `bots/heuristic` and the hosted packages. The
   server reaches bots through the `bots` registry (`bots.Factory`).
4. **`cmd/gorged` links the draft extension.** It is the only import from
   outside `lab/` into `lab/` (see the dashed edge above).

## Further reading

- [Packages](packages.md): every package with its purpose, size and status.
- [Invariants](invariants.md): the rules the engine relies on, and what
  enforces each.
- [ADR 0001: repository layout](https://github.com/adams-shaun/gorge/blob/main/docs/decisions/0001-repo-layout.md).
- The original M0 design spec (2026-09-03) is archived; see
  [`docs/design/ARCHIVE.md`](https://github.com/adams-shaun/gorge/blob/main/docs/design/ARCHIVE.md).
