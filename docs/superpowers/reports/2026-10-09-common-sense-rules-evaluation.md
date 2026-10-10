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
