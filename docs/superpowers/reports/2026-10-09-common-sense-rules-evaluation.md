# Common-sense rules for the heuristic bots: evaluation

Status: evaluation and proposal. Nothing here is implemented in the engine or the
bots; the prototypes are throwaway test harnesses kept outside the repository.

## Question

Can the hand-written bots share a "common sense" layer that removes actions
that are probably detrimental, without touching the search and trained bots
(`search`, `sb-search-*`, `az-redeal`, `az-c3`), which should learn it?

## Answer

Yes, for one class of mistake that turns out to be large, and it can be done
once for every heuristic bot. The class is **casting or activating a hostile
effect (damage or removal) when every legal target is the bot's own**. Two
default bots do it in 17% of their hostile target decisions. A shared
pre-filter removes 98% of those and, over 3,360 games each, makes the bot win
more, not less.

## What the bots do today

Hostile target decisions made in 420 games per policy (every ordered pair of the
15 constructed repo decks, both seats the same policy; the self-hit is a
target controlled by the deciding seat):

| Policy | Hostile target decisions | Aimed at own permanent or player | …with an opposing target available |
|---|---|---|---|
| `bot` | 1,713 | 291 (17.0%) | **1** |
| `lethal-pressure` | 1,679 | 281 (16.7%) | **1** |
| `sb-tactical` | 1,541 | 175 (11.4%) | **1** |
| `sb-heuristic` | 1,467 | 787 (53.6%) | 488 |
| `sb-uniform` | 2,206 | 1,219 (55.3%) | 834 |
| `sb-first` | never answers a target decision (it only passes), 0 | | |

The decisive column is the last. For the three bots meant to play well, the
target *choice* is fine: in 291 self-hits, an opposing target was available in
exactly one. The mistake is the **cast**: they play the spell when the only
thing it can hit is their own. The cards: Wasteland (172 of 291 for `bot`),
Swords to Plowshares (65), Fatal Push (23), Path to Exile (17), Hero's
Downfall (7), Warping Wail (4). The two baselines (`sb-heuristic`,
`sb-uniform`) also pick own targets when an opposing one exists; they are
supposed to be dumb and are out of scope.

Live demo findings agree: Stone Rain and Ghost Quarter reproduced the same way
(the bot destroys its own land when the opponent has none).

## The rule that matters

> Do not offer an action whose first targeting step is a hostile effect
> (`TargetEffect.Damage`, or `TargetEffect.Removal` of kind destroy, exile,
> graveyard or sacrifice) when no legal target is controlled by an opponent.

"Hostile" and "controlled by" are already in the engine's vocabulary: every
target decision carries `Decision.TargetEffect` and each option carries the
seat that controls it (`Option.Player`). A rule on the target *choice* ("never
pick your own") would fix almost nothing, per the table.

## Prototype and results

A wrapper seat dry-runs each offered priority action on a clone of the engine
(`Clone`, `Submit`, `Advance`, `Pending`), drops those whose next pending
decision is a hostile target ask with no opposing option, and asks the wrapped
bot once about what is left. The guarded and plain copy of one policy play each
other, both seat orders, same decks (15 constructed decks, every ordered pair,
8 seeds, 3,360 games per policy, 95% Wilson intervals over decided games):

| Policy | Guarded wins | Win rate | 95% interval | Hostile self-hits (guarded / plain) |
|---|---|---|---|---|
| `bot` | 1,696 - 1,528 | **52.6%** | [50.9, 54.3] | 22 of 6,019 / 1,155 of 6,731 |
| `lethal-pressure` | 1,692 - 1,544 | **52.3%** | [50.6, 54.0] | 25 of 5,993 / 1,131 of 6,623 |
| `sb-tactical` | 1,677 - 1,594 | 51.3% | [49.6, 53.0] | 34 of 5,884 / 778 of 6,410 |

Self-hits fall from 17% to 0.4%. The win-rate gain is small but outside the noise for `bot` and `lethal-pressure`
(+2 to +3 points, intervals above 50%), and not significant for `sb-tactical`,
whose self-hits were mostly Wasteland (153 of 175). A wrapper with the
filtering switched off played even (50.2, 50.7, 49.3%), so the wrapper adds no
bias of its own.

The residual 0.4% are not at a priority stop, so the priority filter cannot
see them (triggered or activated abilities are the likely source; I did not
trace them).

### Pitfalls the prototypes found

1. **Vetoing to a pass** (replace a bad action with a pass) played at 51 to 53%
   with 20 to 38% of non-pass priority actions vetoed, because the bot proposes
   the same bad action again at every priority stop. It works, but wastes the
   bot's turn; removing the action and letting the bot take its next best play
   is the better shape.
2. **Re-asking the bot after a veto is untested.** My first re-ask run lost 4 to
   9 points, but it had the renumbering bug in (3) below, so it says nothing
   about re-asking. The bots do keep state between asks (multi-step plans, mana
   lowering), so asking once, after filtering, is the safe shape and is what the
   prototype does.
3. **Removing options means renumbering them.** Each option carries its own
   `Index` and each payment action a `BaseOptionIndex`; leave them stale and
   the bot's answer maps to the wrong action. My first filtered runs lost 6 to
   9 points to exactly this.
4. **Auto-pay casts are separate from the option list**: they are selected by
   payment-action id, linked to an option. A filter must drop the action and its
   linked option together.

## Cost

Dry-running every offered action costs about 600 clones per game, roughly ten
times the CPU of a plain game in the bench. That is nothing for a live table (a
handful of priority stops a turn) and too much for a training or sweep
workload. The engine already knows each candidate cast's legal targets when it
builds the offered actions (it refuses to offer a cast with none); publishing
"own and opposing legal target counts" with the action would make the rule
nearly free. I did not measure that, and have not checked whether the offer
path computes the counts in a form that is cheap to expose.

## Proposed design

- **One shared pre-filter**, applied by construction to the heuristic policies
  only (`bot`, `lethal-pressure`, `sb-tactical`, and `sb-heuristic` if kept).
  Not applied to `sb-uniform` and `sb-first` (baselines meant to be dumb), nor
  to any search or trained bot.
- **Applied at policy construction, not inside shared bot code.** The search and
  trained bots play the default bot for everything they do not search and as the
  opponent model inside their simulations. A guard inside the shared code would
  reach them through that route and shift what `az-c3` trained against; a guard
  added when the *hosted heuristic policy* is built does not.
- **Same layer for both registries.** SpellBench has decorators
  (`bot+passguard+lethal`), the hosted registry has none. The filter is a
  decorator of the same shape, available to both, so a sanity layer is one
  implementation, not one per registry.
- **First implementation:** the clone dry-run above, behind a small interface so
  the engine-published hint can replace it later without touching any bot.

## What to decide

1. Scope of "heuristic": is `sb-heuristic` in (I lean out, it is a deliberately
   simple baseline) and are the `sb-search-*` variants "AI" (I treat them as
   AI).
2. Whether to honour Forge's `AITgts` hints where a card has one (about 98
   cards; for example Ghost Quarter hits nonbasic lands only). That adds real
   behaviour beyond "never self-target" and changes play on those cards.
3. Whether the engine should publish the counts (cheap, touches the offer path
   and the golden games) or the first version should dry-run (no engine change,
   ten times the CPU in a bench).

## Caveats

- Self-play of one policy against its own guarded copy on 15 constructed repo
  decks. Commander and four-seat tables were not measured.
- The classification is by the engine's own `TargetEffect`. Card-specific
  exceptions where hitting your own permanent is correct (a sacrifice outlet, a
  blink, thinning with Ghost Quarter) would be removed by this rule; the A/B
  shows no net loss, but a per-card allow-list is the safe form.
- Other probably-detrimental actions (discarding good cards, paying life,
  attacking into a lethal block) were not measured and are not part of this rule.

## Reproduction

The harness is two Go test files (`census_selfhit_test.go.txt`,
`prefilter_ab_test.go.txt`) and the raw output (`ab-big.log`) in
`/mnt/sata/gorge-training/bot-sanity/`. They are not committed because each runs
over the per-test budget (about 25 s and 330 s at 16 vCPU). Drop either into
`internal/bench/` and run with `-run TestCensusSelfHits` or
`PF_SEEDS=8 go test -run TestPrefilterABv3`.

## Implementation (2026-10-09, engine hints and guard package; guard not wired)

Operator rulings applied: the engine filters nothing; the guard drops options
only inside a seat decorator; a per-card allow-list exempts deliberate
self-targets; consolidation of the bots comes before wiring, so no factory or
registry entry uses the guard yet.

**Engine hints.** `decision.Option.TargetHints` (and the same field on
`PaymentAction`, for floating-mana plans that have no base option) is a packed
`uint32`, omitted when zero: present and hostile bits plus the count of legal
targets under the deciding seat's control (a player target being that seat) and
under opponents. It is set only for a cast with the card's own cost or a
printed activated ability whose root `SpellAbility` is targeted and hostile
(direct damage; destroy, exile, graveyard or sacrifice). Modal, alternative,
adventure, granted and chained-sub-target shapes carry no hint (fail open).

The offer path could not supply the counts: `targetSAAvailable` runs a LIMITED
census that stops at the mandatory minimum (an any-target spell stops at the
first player), so the counts need the full census (`legalTargetCandidates`, the
census the target ask itself uses). It is therefore a separate, opt-in pass,
`Engine.EnsureTargetHints()` (the `EnsurePaymentActions` pattern): a seat
implements `seat.TargetHintConsumer`; `host.projectNextData` and
`internal/bench` call it for such seats only. The offered option list, every
event and every chain head are untouched; hints are built after the walk on
the posed decision, so the prio memo and quiet-serve caches never see them.

Cost (`go test ./rules -bench 'PriorityOffer|EnsureTargetHints' -benchtime 2000x`,
a mid-game position with the most hinted options in 6 repo-deck games):

| | ns/op | allocs/op |
|---|---|---|
| priority offer walk (unchanged; hints are not on this path) | 220,144 | 72 |
| `EnsureTargetHints`, seat opted in, position with hinted options | 15,681 | 10 |
| `EnsureTargetHints`, a priority decision with no hostile target step | 10.7 | 0 |

A seat that does not opt in pays nothing (no code on its path). I did not build
a second binary to time a before/after of the walk; it is unchanged by
construction, not measured.

**Verification.** `TestTargetHintsStoneRain` (own only, opposing only, both,
each checked against the KTarget decision it poses), the Any-target, pump and
ability cases, and `TestTargetHintsMatchTargetDecisionInGames` (6 repo-deck
games, 75 hinted options, every one equal to the split of the target decision
it poses on a clone) pass. `TestHeads`, `TestRepoDeckGamesReplayExactly`, the
clone audits, `./decision`, `./seat`, `./view`, `./internal/archtest` and the
host bot tests pass; no head moved and no fixture JSON changes (the field is
omitted unless a seat opts in). `web/src/protocol.ts` was regenerated.
Leak check: the counts are over legal targets that are public (battlefield
permanents, players, graveyard and exile cards, stack objects), the same set
the seat's own KTarget decision would list.

**Guard package.** `bots/sanity`: `Filter(decision)` drops cast/ability
options with `hostile && opp == 0 && own > 0` unless the card name is in a
sorted allow-list, renumbers `Option.Index`, remaps each payment action's
`BaseOptionIndex`, drops actions linked to a dropped option and floating-mana
actions whose own hint says drop, never touches pass, and `MapBack` restores the
original numbering. `Wrap(seat)` is the decorator (keeps BoardSeat, EnvSeat and
RefusalAnswerer surfaces, delegates the payment opt-in, requests hints, asks the
wrapped seat once). Allow-list seed: Ghost Quarter (land thinning); the other
census hits (Wasteland, Swords, Path, Fatal Push, Hero's Downfall, Doom Blade,
Karn) stay guarded. The card name is read from the option label (`Cast X`,
`X: ...`), which is the only name the decision carries.

**Smoke A/B (plumbing only, not a verdict).** `bot`, guarded vs plain, 420
games (every ordered pair of the 15 constructed decks, one seed, both seat
orders), allow-list on: guarded 205 - plain 200 (15 draws or stalls), 50.6%
[45.8, 55.5] of decided games, which is indistinguishable from even at this
size. Hostile target decisions aimed at the seat's own permanent or player:
guarded 5 of 740 (0.7%), plain 152 of 832 (18.3%). The first run missed the
floating-mana payment actions (self-hits 53 of 778); the payment-action hint
fixed that. The full 3,360-game A/B per policy is deferred until after
consolidation. Harness: `/mnt/sata/gorge-training/bot-sanity/sanityab_hints_test.go.txt`
(drop into `internal/bench/`; `SAN_MODE=ab|census SAN_BOT=bot SAN_SEEDS=n`).

## Implementation, part 2 (2026-10-09): optional guard, measured

Operator decision: the guard is OPTIONAL, default OFF everywhere, for the
bootstrap-trainer use (the default bot is the fallback and the opponent model
for search/AZ training). Nothing existing changes unless a caller opts in.

**How to opt in.**

- Library: `bots.Options{CommonSense: true}` through `bots.New`. Only the `bot`
  factory and `sbtactical.New` (so also the registry entry) read it; they return
  `sanity.Wrap(seat)`. `lethal-pressure`, `sb-heuristic`, `sb-uniform/first`
  and every search/AZ policy ignore it. With the field false the returned seat
  is the same concrete type as before (`*seat.Bot`, `*hostedSeat`;
  `TestCommonSenseOff...`). The wrapper keeps `BoardSeat`, `EnvSeat`,
  `RefusalAnswerer`, `PaymentPlanConsumer` (delegated) and
  `TargetHintConsumer` (tests in `bots/bot`, `bots/sbtactical`).
- `host`: `newCaretakerSeat` now arms `SkipLifePlans` through `UnwrapSeat()`
  (`skipLifePlans`, tested). Host paths do not set `CommonSense` yet, so a
  hosted table is unchanged.
- `cmd/botbench`: `-common-sense` rebinds `bot`, `bot-auto-pay` and
  `sb-tactical` to the guarded form on both sides; per-side policy names
  `bot+sanity`, `bot-auto-pay+sanity`, `sb-tactical+sanity` give an A/B
  (`-a bot+sanity -b bot`). No other name has the suffix.
- Not wired, by design: `internal/azmcts` builds its fallback/rollout bot with
  `seat.NewBot` / `seat.NewBotOn(...).EnableAutoPayMana()` (`seat.go:157`,
  `env.go:236`, `nodecache.go:418`), and `cmd/dzgorge/policy.go:234` uses
  `seat.NewBot`, all outside `bots.New`, so `CommonSense` does not reach them.
  A trainer opts in by wrapping where it builds that bot (`sanity.Wrap`) and
  letting its driver call `EnsureTargetHints` for the wrapper
  (`seat.TargetHintConsumer`); for an AZ rollout that has no hint-pass this
  needs a separate change. `aztrain` as an opponent via `bots.New`/botbench
  can use the flag or the `+sanity` name today.

**Gain** (harness: `/mnt/sata/gorge-training/bot-sanity/sanityab2_test.go.txt`,
copied into `internal/bench/` only to run; 15 constructed decks, every ordered
pair, both seat orders, 24 seeds = 10,080 games per row, auto-pay mana unless
stated, allow-list on; win rate is over decided games, Wilson 95%; "self-hit"
= a hostile target decision whose chosen target is the deciding seat's own
permanent or player).

| arm A vs arm B | A wins - B wins (draws/stalls) | A win rate | self-hit A | self-hit B |
|---|---|---|---|---|
| bot guarded vs bot plain | 5012 - 4636 (432) | 51.95% [50.95, 52.94] | 0.65% (116/17854) | 16.75% (3400/20304) |
| bot guarded vs bot plain, MANUAL mana | 5000 - 4747 (333) | 51.30% [50.31, 52.29] | 0.66% | 15.02% |
| sb-tactical guarded vs sb-tactical plain | 4922 - 4924 (234) | 49.99% [49.00, 50.98] | 0.87% | 12.02% |
| sb-tactical plain vs bot plain | 5672 - 4088 (320) | 58.11% [57.13, 59.09] | 12.17% | 17.25% |
| sb-tactical plain vs bot guarded | 5460 - 4319 (301) | 55.83% [54.85, 56.82] | 11.94% | 0.64% |
| sb-tactical guarded vs bot plain | 5518 - 4264 (298) | 56.41% [55.42, 57.39] | 0.91% | 17.58% |
| sb-tactical guarded vs bot guarded | 5324 - 4485 (271) | 54.28% [53.29, 55.26] | 0.89% | 0.63% |

Reading: for `bot` the guard is a real but small gain, about +2.0 points on
auto-pay (about 3.8 standard errors, outside noise) and +1.3 on manual (about
2.6 standard errors). For `sb-tactical` the guard buys NOTHING in win rate
(49.99%, dead even) although it removes 12% to 0.9% of hostile self-hits; the
tactical heuristic's self-hits are evidently not costly. The gap sb-tactical
over bot shrinks from 58.1% to 54.3% when both are guarded, i.e. the guard
helps the bot fallback more than it helps the stronger heuristic; a guarded bot
is a modestly harder, more sensible opponent. The residual 0.6-0.9% self-hits
are cases the hint does not cover (modal, alternative, chained shapes, and
Ghost Quarter allow-list picks).

**Cost** (same machine, sequential single-goroutine loop, `bench.PlayGame`,
GOMAXPROCS 4 on 4 cores, 420 games x 3 rounds per arm (210 for sb-tactical),
the same seeds and decks per arm after a discarded warm-up, wall and process
CPU via getrusage; the box is shared, so treat differences under about 3% as
noise. "hintonly" requests the target hints but never filters, and plays
exactly the plain games, so it isolates `EnsureTargetHints`):

| policy | arm | wall us/game | cpu us/game | decisions/game | vs plain (wall) |
|---|---|---|---|---|---|
| bot, auto-pay | plain | 10,291 | 12,658 | 434.0 | |
| | hintonly | 10,729 | 13,245 | 434.0 | +4.3% |
| | guarded | 10,850 | 13,339 | 421.6 | +5.4% |
| bot, manual | plain | 7,564 | 8,946 | 462.4 | |
| | hintonly | 7,514 | 8,966 | 462.4 | -0.7% |
| | guarded | 7,449 | 8,937 | 453.5 | -1.5% |
| sb-tactical | plain | 25,173 | 36,401 | 395.9 | |
| | hintonly | 25,224 | 35,842 | 395.9 | +0.2% |
| | guarded | 24,369 | 34,760 | 390.0 | -3.2% |

`Filter` + `MapBack` alone over 8,029 real decisions: 57 ns per decision, 30
us per game (0.3% of a bot game). So the cost is about +4-5% on auto-pay bot
(the census pass runs when the seat asks for payment actions), indistinguishable
from zero on manual mana and on sb-tactical. The guarded arms play slightly
fewer decisions per game (they skip removed options), which can make the
guarded arm cheaper than the hint-only arm; that is trajectory, not savings.
Timing came from a clean timed loop in the harness, not `cmd/enginebench`.

**Not covered.** Commander and 4-seat/multiplayer tables (only 2-seat
constructed measured; the hostile-vs-own counts are per deciding seat, so
multiplayer semantics are unverified); SpellBench registry parity (no `sanity`
decorator beside `passguard` in `internal/spellbench/registry`); `sb-heuristic`
and `lethal-pressure` (deliberately not wired); search/AZ policies and the
az/dzgorge internal fallback bots (built outside `bots.New`); hosted tables
(no host option sets `CommonSense`); a persisted policy name (the suffix
exists only in botbench).

**Wiring notes (superseded by part 2 above).** One line per factory (`sanity.Wrap(seat)`) in `bots/bot`,
`bots/lethalpressure`, `bots/sbtactical`, `bots/sbheuristic`. Two things to
handle then: `host/bot_policy.go`'s `newCaretakerSeat` type-asserts `*seat.Bot`
(the wrapper exposes `UnwrapSeat()` for it), and an `Options` switch to build
the plain policy for A/B arms. SpellBench registry parity (a `sanity` decorator
beside `passguard`) is not done.

## Mulligan (2026-10-10)

Operator request: add a mulligan strategy to the optional `Options.CommonSense`
layer (default OFF; the engine filters nothing; AI/search/trained bots get no
hand-coded guard).

### Today's behaviour, per policy

The London round only exists when `rules.Config.Mulligans > 0`
(`rules/mulligan.go`: it is the permitted count per seat, not the multiplayer
free mulligan; a seat that has used its allowance is offered `keep` only).
`gorged` serves 1 (`cmd/gorged/main.go:276`), `host.TableConfig` defaults to 0
(`host/table.go:163`), and `cmd/botbench` runs the round only in `-spellbench`
mode (`-spellbench-mulligans`, `cmd/botbench/spellbench.go:325`); its ordinary
matrix mode and the earlier sanity A/B harness (`rules.Config` without
`Mulligans`) never ask a mulligan, so no earlier measurement here says anything
about mulligans.

| policy | keep/mulligan rule | where | rng |
|---|---|---|---|
| `bot`, `bot-auto-pay`, `lethal-pressure` (the `seat.Bot` family) | `MulliganCoin`: mulligan with probability 1/3 whatever the hand holds; bottoming is `chooseDiscard` | `botpolicy/policy.go:1110-1135` (`r.IntN(3)`, line 1128); the zero value of `Board.Mulligan` (`botpolicy/mulligan.go`) | one `IntN(3)` per keep/mulligan ask that offers a mulligan |
| `sb-tactical` | the same coin: the tactical seat sends every kind it does not score to `botpolicy.Decide` with its own rng (`tactical.fallback`, `internal/spellbench/builtins/tactical.go:328`); NOT separately configurable | as above | its own PCG, one `IntN(3)` per ask |
| `sb-heuristic`, `sb-first` | always keep (`builtins.go:567-572`; `sequential` takes option 0) | `internal/spellbench/builtins/builtins.go` | none |
| `sb-uniform` | uniform over the offered options | `selectSeq` | one draw per ask |
| search / AZ (`az-c3`, `az-redeal`) | `botpolicy.Board.Mulligan` from their config (`MulliganLands` for az-c3) | `internal/azmcts/seat.go:135` | none under a rule |
| SpellBench decorator | keep if 2..5 lands and a turn-3 castable spell and a needed colour | `internal/spellbench/registry/mull.go` | none |

Opt-in rules that existed: `MulliganLands` (2..5 lands among the seven, tuned
for 40-card limited), `MulliganNever`.

### Candidate rules

All are deterministic functions of the deciding seat's own hand (and, for the
new one, its own deck list), consume no rng, and are answered only under an
opt-in.

| arm | rule |
|---|---|
| coin | baseline above |
| never | `MulliganNever`: always keep |
| lands | `MulliganLands` as shipped (K = kept cards: K>=6 keep 2..5 lands, K==5 keep unless no land or no spell, K<=4 keep) |
| constructed | NEW `botpolicy.MulliganConstructed`: for K>=6 keep iff lands in 2..hi AND a spell castable by turn 3 with the hand's lands (mana value <= min(lands,3), every coloured pip covered by a producing land) AND the lands produce a colour some spell needs; K==5 and K<=4 as `lands`. `hi = clamp(ceil(7*landShare)+2, 4, 5)` from `Board.OwnDeck` (the seat's own manifest; 24/60 gives 5, 16/60 gives 4), fixed 2..5 without a manifest. Bottoming is `landBottoming`. |

`MulliganConstructed` is a new enum value after `MulliganNever`; `Board.Mulligan`'s
zero value, `MulliganLands` and `MulliganNever` are untouched, so az-c3,
az-redeal and dzgorge behave byte-for-byte as before (their tests,
`./internal/azmcts`, `./bots/azredeal` and `TestHeads$` pass). The registry
decorator's logic was not moved; the turn-3 test is re-implemented in
`botpolicy/mulligan_constructed.go` on `Board.Cards`.

### Measurement

Harness `/mnt/sata/gorge-training/bot-sanity/mull_ab_test.go.txt` (copied into
`internal/bench/` only to run, deleted after; `MUL_MODE=ab MUL_POL=bot|tact
MUL_ARMS=... MUL_B=... MUL_LIMIT=n MUL_SEEDS=n [MUL_DECKDIR=...]`). 2 seats,
auto-pay mana, same policy on both seats, only the mulligan rule differs;
seat A plays the arm, seat B the baseline (the coin unless stated); both seat
orders; the 15 constructed repo decks, every ordered pair; `Config.Mulligans =
2` (a seat may take two mulligans; London draws seven each time and bottoms one
card per mulligan taken, so it is the usual rule truncated at 5 cards);
24 seeds = 10,080 games per arm (>= 3,360 required). Win rate over decided
games, Wilson 95%. "mull rate" = share of seat-games with at least one
mulligan; "kept lands" = lands in the final kept hand after bottoming
(per seat-game that kept).

`bot` (arm vs coin), Mulligans = 2:

| arm A (vs coin) | A - B (draws/stalls) | A win rate | A mull rate (per game) | A kept lands | B (coin) mull rate | B kept lands |
|---|---|---|---|---|---|---|
| coin (control) | 4799 - 4771 (510) | 50.15% [49.14, 51.15] | 32.3% (0.428) | 2.413 | 32.3% | 2.381 |
| never | 5000 - 4572 (508) | 52.24% [51.23, 53.24] | 0.0% | 2.399 | 32.3% | 2.386 |
| lands | 4940 - 4619 (521) | 51.68% [50.68, 52.68] | 25.4% (0.319) | 2.778 | 32.3% | 2.375 |
| constructed | 4937 - 4626 (517) | 51.63% [50.62, 52.63] | 26.4% (0.334) | 2.773 | 32.3% | 2.375 |

`sb-tactical` (arm vs its own coin; the arm is a harness decorator calling
`botpolicy.MulliganAnswer`, exactly the call the guard makes), Mulligans = 2:

| arm A (vs coin) | A - B (draws/stalls) | A win rate | A mull rate | A kept lands |
|---|---|---|---|---|
| coin (control) | 4967 - 4848 (265) | 50.61% [49.62, 51.59] | 34.2% | 2.417 |
| never | 5218 - 4587 (275) | 53.22% [52.23, 54.20] | 0.0% | 2.399 |
| lands | 5209 - 4591 (280) | 53.15% [52.16, 54.14] | 25.4% | 2.787 |
| constructed | 5229 - 4579 (272) | 53.31% [52.33, 54.30] | 26.4% | 2.782 |

Do the rules beat never? Head to head (A = rule, B = never), `Mulligans = 2`:

| policy | A | games | A - B (draws/stalls) | A win rate |
|---|---|---|---|---|
| bot | constructed | 40,320 (96 seeds) | 19069 - 19340 (1911) | 49.65% [49.15, 50.15] |
| bot | lands | 40,320 | 19062 - 19355 (1903) | 49.62% [49.12, 50.12] |
| bot | constructed vs lands | 10,080 | 4750 - 4860 (470) | 49.43% [48.43, 50.43] |
| sb-tactical | constructed | 40,320 (96 seeds) | 19702 - 19581 (1037) | 50.15% [49.66, 50.65] |
| sb-tactical | constructed vs lands | 10,080 | 4958 - 4863 (259) | 50.48% [49.49, 51.47] |

Controls and variants:

- `Mulligans = 0` (no round is posed, so the rule cannot act): constructed and
  never both give 4775 - 4837 (468), 49.68% [48.68, 50.68], i.e. no effect and a
  harness asymmetry of about -0.3 points (the A seat's seed differs from B's),
  which is inside the interval. Read all rows above with that in mind.
- `Mulligans = 6` vs coin (bot): never 52.70% [51.70, 53.70], lands 52.02%
  [51.02, 53.02], constructed 51.96% [50.96, 52.96]; the coin seat then mulligans
  0.49 times per game.
- 40-card Limited pool (16 of the 36 decks in
  `/mnt/sata/gorge-training/az-c3/decks`, 17lands FDN decks resolved through
  `deck.Parse`/`Resolve`; 11,520 games per arm, 24 seeds, bot vs bot,
  Mulligans = 2): coin 50.18% [49.27, 51.10]; never 55.30% [54.39, 56.21];
  lands 55.65% [54.74, 56.56] (mull rate 13.0%, kept lands 3.09); constructed
  55.28% [54.36, 56.18] (mull rate 19.9%, kept lands 3.07). Coin seat there
  mulligans 32.2% of games.

Win rate split by whether a mulligan happened, `bot` arm A, from the table
above's runs: coin 44.6% when it mulliganed (3,089 games) vs 52.8% when not;
lands 45.6% (2,440) vs 53.8%; constructed 45.1% (2,529) vs 54.0%. This is
selection (a mulliganed seat has a worse hand and one card fewer), not the
effect of the decision, and is reported only because it was asked for.

### Reading

- The coin is the loser. Any hand-agnostic or hand-aware rule beats it by
  about 1.6 to 2.2 points on `bot` (about 3 to 4 standard errors), 2.5 to 2.7 on
  `sb-tactical`, and about 5 points on 40-card Limited. The coin mulligans a
  third of all hands including good ones.
- Almost all of that gain is "stop the random mulligans". The land-window
  rules are NOT distinguishable from never mulliganing: against `never` they
  are -0.35 points on `bot` (constructed 49.65% [49.15, 50.15], lands 49.62%) and
  +0.15 on `sb-tactical` (50.15% [49.66, 50.65]), all intervals touching 50%,
  and `constructed` vs `lands` is within noise (49.43%, 50.48%). Said plainly:
  in this engine, with these bots, a mulligan on a 0, 1, 6 or 7 land hand costs
  a card and buys back about what it costs. The land-aware rules do change what
  is kept (kept lands 2.77 vs 2.40, and fewer extreme hands), which is what a
  human reader would call sane, but not who wins.
- `constructed` and `lands` are equal within noise; `constructed` mulligans
  slightly more (26.4% vs 25.4% on 60-card, 19.9% vs 13.0% on Limited, because
  of the early-play test) with the same win rate.

### Wired

Chosen: `MulliganConstructed`, behind `Options.CommonSense` only, for `bot` and
`sb-tactical`. Rationale (a tie-break, not a measured win): it ties `never`
and `lands` on win rate, and it is the one whose behaviour matches the
"common sense" the option exists for (no keeping a hand with no lands, no
keeping seven lands), derived from the deck's own land share. Both `never`
and `lands` were within noise of it and would have been equally defensible.

- `bots/sanity.Mulligan = botpolicy.MulliganConstructed`; the wrapper answers a
  `KMulligan` ask itself (board path for `seat.BoardSeat`, `seat.BoardFromView`
  for the view and env paths) through the exported
  `botpolicy.MulliganAnswer(board, rule, decision)` and does not consult the
  wrapped seat, so `bot` draws no coin and `sb-tactical` neither. This is the
  same call the measured `sb-tactical` arm used, and the same `Board.Mulligan`
  rule the measured `bot` arm used (`WithMulligan`), so the wired behaviour
  is the measured behaviour; it was not itself A/B'd as a full wrapped game
  because the wrapper also carries the target guard (confounded).
- Off (`CommonSense` false): the factories return the same concrete types and
  the mulligan answer is the coin (`TestCommonSenseOffMulliganIsTheCoin`).
- `cmd/botbench -common-sense` and the `+sanity` names already select the
  wrapper, so they pick this up; botbench only runs a mulligan round in
  `-spellbench` mode. `gorged` tables (Mulligans = 1) do not set
  `CommonSense`, so hosted play is unchanged.

### Cost

Not timed separately. The rule is O(7) per ask with no allocation beyond two
small slices in the early-play test, and at most about three asks per seat per
game (Mulligans = 2). No timing was taken for it; the cost is inferred from
that shape, not measured. The guard otherwise changes nothing about the
priority path.

### Not covered

Commander (the free-mulligan rule in `rules/mulligan.go` shifts the bottoming
count; `keepBottomCount` reads the penalty from the prompt, so a free first
mulligan reads as 0 bottom and the rule treats it as an opening seven) and
4-seat tables were not measured; the harness is 2-seat. Limited was measured
only for `bot`, 16 decks, as a sanity check, not for `sb-tactical`. The
`Mulligans = 2` allowance is the only value run in full (6 and 0 as variants on
`bot`); a hosted table's allowance of 1 truncates the round further and was
not run. The bottoming half was not measured in isolation (all arms bottom with
`landBottoming` or `chooseDiscard` as noted). Win-rate gains are small against
the harness's own asymmetry (about -0.3 points at Mulligans = 0).
