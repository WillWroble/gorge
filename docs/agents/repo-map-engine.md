# Repo map — the engine chain

Part of the [repo map](repo-map.md) index. See also:
[invariants.md](invariants.md) (the rules every change must keep) and
[do-not.md](do-not.md) (the mistakes that have actually happened here).

## The engine chain

The import direction is one-way. `internal/archtest`'s
`TestDependencyOrderHolds` pins the arrows that must never appear (see
[invariants.md](invariants.md#1-dependency-direction)).

| Package | What it owns |
|---|---|
| `cards` | Compiles Forge card scripts into the card IR; keyword expanders (`cards/kw_*.go`); `cards.Registry`, coverage. `cards.OpenCorpusFor` (`cards/subset.go`) opens only a known card pool's cards from the per-card segment file, falling back to the whole corpus when the pool reads the name universe; botbench's deck and `-spellbench` modes use it. `cards.CompilerFingerprint` hashes every non-test `.go` file here, so **any edit in `cards/` invalidates every IR cache** and forces a recompile. |
| `cards/oracletext` | Printed-card (Oracle) view for the Oracle audit. A subpackage on purpose so it does not move the fingerprint. |
| `state` | Objects, zones, players, `state.Game`, the `ContinuousEffect` vocabulary. Plain data; mutated only by `events.Apply`. |
| `decision` | `Decision` / `Option` / `Intent`, and the closed set of `decision.Kind`s (`decision.Kinds`). The whole engine↔seat contract. |
| `deck` | `{name, count}` deck-list JSON resolved against a `cards.Registry`. |
| `botpolicy` | The one heuristic bot policy (`botpolicy.Decide`), in one copy, shared by `seat` and rules' own fuzz driver. Reads a `Board`, never a view or an engine. Cast profiles in `botpolicy/profiles/`. |
| `bots` | Registry of hosted bot policies (`bots/registry.go`); built-ins register from `bots/bot`, `bots/lethalpressure` and `bots/castprofile`, linked together by `bots/all`. |
| `events` | The event union, `events.Apply` (the only state mutator), the log, the sha256 hash chain. `events.Kind` is append-only. |
| `effects` | `api:` primitive implementations, filter/count/value evaluators. Talks to the engine only through the small `effects.Host` interface; never imports `rules`. |
| `rules` | The engine: turn structure, priority, stack, combat, SBAs, layers, casting and payment, and the `kw:`/`trig:`/`stat:`/`repl:` primitives. `rules.New`, `Pending`, `Submit`, `Advance`, `Clone`. `rules.NewStaged` (`staging.go`) builds a hypothetical engine at a described position. Also holds the coverage ratchets (`rules/*_test.go`) and the golden heads (`rules/testdata/heads/`, pinned by `TestHeads`). |
| `rules/chars` | The read-only CR 613 layer walk (W5 step E4): `Types` (layer 4), `Compute` (layers 3/5/6 with CR 613.6 dependency order, then 7), `PT` (layer 7), `Name`, `BaseKeywords`, the type vocabulary and layer-4 transforms, and the layer-7a CDA evaluators (`CDASetPT`, `CDAPTStatic`). Reads the game only through `chars.Board`, which `rules/chars_board.go` implements, and keeps its buffers in a `chars.Scratch` the Engine embeds (`charsWalk`); never imports `rules`. The effect lifecycle, the static scan, the Derived memo, the printed fast paths, the incremental layer tables and the Affected$ match (`matchesWithCharsPT`) stay in `rules`. A new characteristic rule goes here; a new Board method fails `TestCharsBoardOnlyShrinks`. |
| `rules/combat` | Attack/block legality (W5 step E5): `CanAttack`/`CanAttackPair`/`CanBlock`, the CR 508.1d requirement set (`AttackRequirements`), goad, the CantAttack/CantBlock/CantBlockBy/CanAttackDefender restriction walks, hidden-keyword grants, AttackRestrict and MinMaxBlocker bounds, MustBlock duties. Reads the game only through `combat.Board`, which `rules/combat_board.go` implements; never imports `rules`. The declaration flow (asks, validators, attack/block charges, events) stays in `rules/combat.go` and `rules/attack_cost.go`. A new legality rule goes here; a new Board method fails `TestCombatBoardOnlyShrinks`. |
| `rules/trigmatch` | The trigger-mode matchers (L5, W5 E3): the `Mode$` registry (`Lookup`, `Match`) and one matcher per mode, reading the engine only through the read-only `trigmatch.Board` (implemented by `rules/trigmatch_board.go`). Queueing, latches and Notes stay in `rules` (`checkTriggers`, `checkFaceTriggers`). A new trigger mode registers here. |
| `view` | Projects one seat's redacted `view.View` (`Project`, `ProjectFor`, `RedactEvents`). The only way a client reads state. `view/view.go` is the orchestration (`Project`/`project`, `View`, the shared text helpers); the card/battlefield projection family lives in `view/card.go` (`CardView`, `cardViews`, `cardView`) and the stack family in `view/stack.go` (`StackView`, `stackViews`). A projection change goes in the family file for its seam, not in `view/view.go` — the two families were interleaved in one file and two projection tickets collided on it (split out in `97655ff0b`). |
| `seat` | Who answers decisions: `seat.Seat` (+ `BoardSeat`, `PaymentPlanConsumer`), `NewBot`, `NewAttackSimBot`, `NewPolicyNetBot`, explore bot. Handed a view, never an engine. |
| `replay` | Re-executes `(Config, Log)` and names the first divergent event (`Replay`, `ReplayTo`). |
| `protocol` | Versioned wire types for host↔client. Types only; never imports `rules`. |
| `host` | Tables (one goroutine each), sessions, snapshots, persistence, undo, the hosted bot-policy vocabulary (`host/bot_policy.go`). Its only clock is an injected sleep. |
| `host/httpapi` | `net/http` JSON + SSE + the embedded web client. Every seat route goes through `claimForTable`. |
| `eventhost` | Hosted draft events inside gorged: event store (`event.json` + `log.jsonl`), lobby/join, draft with synchronous bots, per-event SSE frames; build and games in later E1 tickets. |

## The draft stack

E0's headless limited lab: a draft pod's own event stream, below (not
inside) the engine chain. Everything here is pure or nearly so — the only
package that touches the engine is `cmd/draftsim` (see
[repo-map-paths.md](repo-map-paths.md#cmd)). Design:
`docs/superpowers/specs/2026-10-13-draft-events-design.md`.

| Package | What it owns |
|---|---|
| `limited` | The draft stack's shared vocabulary: `CardRef`, `Rarity`, the pool/receipt types every `limited/` package speaks. No behaviour; embeddable in logs and wire payloads. |
| `limited/setdata` | The embedded set specs (`FDN`, `BLB` as go:generate-refreshed JSON) and `Load`/`Codes`/`Support` over them. |
| `limited/booster` | Seeded Play-Booster opening: slot-ordered fills, weighted wildcards, colour balance; `OpenPod` opens a whole pod's packs from one seed. |
| `limited/draft` | The pure four-seat booster-draft state machine (left/right pack flow, FIFO pack queues); interleaving-independent. |
| `limited/build` | Pool → engine-loadable 40-card deck: validation, coloured-pip land split, `deck.File` output (`ToDeckFile`). |
| `limited/pickbot` | The pick decision and pool→deck build policies a drafting bot is made of: `RatingColour` (Drafter), `Best23` (Builder), the shared registry with `Lookup`/`Names`. Registry entries are shared pointers — construct fresh instances with a `CardLookup` injected. |
| `event` | The draft event: a LOBBY→DRAFT→BUILD→ROUNDn→COMPLETE state machine over an append-only, hash-chained log (`Apply`, `ReplayLog`, `MarshalLog`), the per-seat draft view (`Next`/`Pool`/`SavedDeck`), round-robin pairings, standings and the CSV draft log. Owns the replay-from-log contract `cmd/draftsim -verify` asserts. |
