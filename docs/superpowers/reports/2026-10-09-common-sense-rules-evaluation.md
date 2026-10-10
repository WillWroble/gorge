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

**Ready to wire.** One line per factory (`sanity.Wrap(seat)`) in `bots/bot`,
`bots/lethalpressure`, `bots/sbtactical`, `bots/sbheuristic`. Two things to
handle then: `host/bot_policy.go`'s `newCaretakerSeat` type-asserts `*seat.Bot`
(the wrapper exposes `UnwrapSeat()` for it), and an `Options` switch to build
the plain policy for A/B arms. SpellBench registry parity (a `sanity` decorator
beside `passguard`) is not done.
