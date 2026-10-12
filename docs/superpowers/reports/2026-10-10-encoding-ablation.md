# Encoding ablation: which state symbols carry signal (2026-10-10)

Branch `wt/rs-encablate`. Data and run logs: `/mnt/sata/gorge-training/encablate`.
Code: `cmd/encablate` (label corpus -> family-tagged token dump),
`internal/mzenc` (`Encoder.Hook` / `ProcessStateHook`, research-only, nil on
every production path, ids unchanged), `scripts/encablate/{ablate,summarize,summarize_keep}.py`.

## Setup

- **Corpus (parts 1, 2, 4):** the existing SpellBench honest-teacher label corpora
  in `llm-qlora/data` (`sb1`, `c1r`, `t1`, `sbt`; `c1` dropped as a duplicate of
  `c1r`): **40,062 decisions, 13,974 games**, 34 decks across fdn-limited,
  pauper-kernel and repo-constructed. Kinds: priority/cast 23,283, attackers
  8,627, target 6,318, blockers 1,834. 24% of decisions are teacher overrides
  (teacher pick != bot pick).
- **Encoding:** every record's *redacted seat view* is walked by the mzenc port.
  `Encoder.Hook` attributes each emitted feature to the walker family that
  ORIGINATED it (before `passToParent` propagation), so dropping a family also
  drops its propagated root/player-level copies. Per-card-instance namespaces
  are abstracted out of tokens (the zone-level aggregates are kept); card
  identity is a separate `zone|name` bag (`Ident.Names`) so it can be swapped
  for an IR bag. Three families mzenc does not emit on a seat view were added
  (`X.*`: hand size, graveyard/exile counts).
- **Model:** bag-of-tokens (EmbeddingBag d=64 -> MLP) state trunk; candidate
  scored by MLP([h, c, h*c]) with c a bag of the candidate's option tokens
  (+ option card identity). Three heads: **policy** (CE over candidates,
  target = teacher choice), **cand-Q** (MSE of each candidate's teacher rollout
  value, the dense low-noise version of the policy target), **value** (BCE on
  game outcome). 2 epochs (epoch sweep: value BCE 0.565/0.555/0.571/0.583 at
  1/2/3/4 epochs; it overfits by memorising games after 2).
- **Splits:** held-out = 20% of GAMES by crc32 (8,226 decisions, 1,939
  overrides). CIs are a paired bootstrap over held-out games (300 resamples),
  averaged over seeds.

Baselines (held-out, 5 seeds): policy CE 0.711, top-1 76.4% (= pick-bot rate),
override CE 2.24, cand-Q MSE 0.0637, **value BCE 0.555, value acc 71.5%**.
Empty-state model (no state tokens): value BCE 0.691 / acc 53.2%, Q MSE 0.113,
policy CE 0.725.

## 1. Gap table

Legend: Y emitted, - absent, ~ partial. "Signal" is the part-2 keep-one value
BCE gain (x1e3) where measured.

| MageZero family | MZ (XMage) | policynet `mz` | mzenc (seat view) | Signal / note |
|---|---|---|---|---|
| TurnStep, DecisionType, DecisionsText | Y | ~ (v1 dense step/phase) | Y | +3.9 / +3.8, weak |
| Stack (+targets, X, modes, kicks) | Y | ~ (side, spell API, types) | Y | +7.6, small |
| StackCostTags | Y | - | - (no view field) | unmeasured |
| Exile (per zone) | Y | - | ~ (one flat list) | 0.0: **noise** on this corpus |
| IsActivePlayer / IsDecisionPlayer | Y | Y (v1) | Y | ~+2 / 0 (IsDecisionPlayer constant) |
| LifeTotal | Y | Y (v1 + buckets, lifediff, lethal-unblocked) | Y | Opp +9.1, Me ~0: 68% of labels sit at life diff within ±2 |
| LibraryCount | Y | Y (v1) | Y | ~0 |
| ManaPool | Y | ~ (avail mana) | Y | Me +16 (mostly untapped-mana proxy), Opp 0 |
| CanPlayLand | Y | - | Y | Opp +3.3, Me 0 |
| Player counters / attachments | Y | - | Y | ~0 (rare in corpus) |
| DayNight, InPayManaMode, Activating, MicroDecisions | Y | - | - (no engine state) | not measurable; MicroDecisions moot (atomic casting) |
| Battlefield (per-permanent props) | Y | Y (both sides) | Y | **Opp +61.5, Me +55.5** (top generic families) |
| Battlefield card names | Y | ~ (hashed name rows) | Y | Opp +50.6, Me +41.9 |
| Hand (own) | Y | Y | Y | props +54.2, names +50.6 |
| Hand (opponent) | Y (perfectInfo) | count only (v1) | **- : neither cards NOR count** on a seat view (perfectInfo branch emits no CardsInHand) | X.Opp.HandSize +5.0; belief probe: part 3 |
| Graveyard | Y | count only (v1) | Y | **Opp props +26.6, Me +19.8**, names +19/+13 |
| CommandZone / Emblem | Y | - | ~ / - | not exercised (no commander decks here) |
| GlobalWatchers (spells cast, life gained/lost, tokens this turn) | Y | - | - (no engine state) | **missing; not measurable from labels** |
| Dynamic types/abilities, CardAbilities | Y | ~ (keywords, spell API) | - | partially covered by IR bag (part 4) |
| CanAttack/CanBlock, combat flags | Y | Y | Y (derived) | inside Battlefield props |
| Own library composition | - | `mz-ownlib` | - | not in this run |

Likely to matter, in order: battlefield props (both), own hand, graveyard
(both — missing from policynet `mz`), opponent hand count (missing from mzenc
seat view), per-turn watchers (missing everywhere; storm/prowess/lifegain decks
in the catalog key off them).

## 2. Family ablation

Two readouts. **Keep-one** (train with ONLY that family, gain over the
empty-state model) measures the signal a family carries on its own.
**Drop-one** (trained with 15% per-family input dropout so the eval-time drop
is in-distribution, then each family removed at eval; 5 seeds) measures what
is not redundant with the rest.

### Keep-one (3 seeds; gain x1e3, * = CI excludes 0)

| family alone | value BCE gain [CI] | value acc | cand-Q MSE gain [CI] | policy CE gain |
|---|---|---|---|---|
| all families | +137.0 [+109.6,+163.6]* | 71.7% | +52.3* | +16.2 |
| Ident.Names (zone\|name bag) | +114.1 [+86.5,+139.2]* | 70.1% | +42.0* | +6.4 |
| Opp.Battlefield.props | +61.5* | 64.3% | +26.3* | +6.9 |
| Me.Battlefield.props | +55.5* | 63.1% | +17.6* | +6.3 |
| Me.Hand.props | +54.2* | 63.6% | +18.0* | +4.6 |
| Me.Hand.name | +50.6* | 63.9% | +16.5* | +1.8 |
| Opp.Battlefield.name | +50.6* | 63.3% | +22.0* | +4.3 |
| Me.Battlefield.name | +41.9* | 61.2% | +12.6* | +4.3 |
| Opp.Graveyard.props | +26.6* | 56.2% | +10.0* | +2.4 |
| Me.Graveyard.props | +19.8* | 58.5% | +6.6* | +1.3 |
| Opp.Graveyard.name | +19.1* | 55.7% | +8.4* | +0.3 |
| Me.ManaPool | +16.1* | 57.3% | +4.9* | +1.4 |
| Me.Graveyard.name | +12.8 | 57.8% | +4.0* | -0.8 |
| X.Opp.ZoneCounts | +10.1* | 54.2% | +2.1 | +0.9 |
| Opp.LifeTotal | +9.1* | 54.0% | +2.7* | +1.6 |
| Stack | +7.6* | 55.7% | +1.8* | +1.5 |
| X.Opp.HandSize | +5.0* | 53.5% | +1.1* | +1.8 |
| G.StepOrDecisionType / G.DecisionsText | +3.9* / +3.8* | 55% | ~0 | ~0 |
| Opp.CanPlayLand, X.Me.ZoneCounts, Is*ActivePlayer | +1.8 to +3.3* | 53-54% | small | ~0 |
| Me.LifeTotal, Me/Opp.LibraryCount, Me.CanPlayLand, Exile, Me.IsDecisionPlayer, ZoneMarkers, Opp.ManaPool, PlayerMisc, X.Me.HandSize | within ±1, CI spans 0 | 53% | ~0 | ≤0 |

### Drop-one (5 seeds; loss increase x1e3 when removed, * = CI > 0)

| family removed | Δ value BCE [CI] | Δ cand-Q MSE [CI] | Δ policy CE [CI] |
|---|---|---|---|
| Me.Hand.props | +12.9 [+5.5,+20.6]* | +5.4* | +2.1* |
| Opp.Battlefield.props | +12.6 [+4.4,+21.1]* | +6.9* | +4.1* |
| Me.Battlefield.props | +11.1 [+3.9,+18.5]* | +3.3* | +3.5* |
| Opp.Graveyard.props | +6.2 [+0.2,+12.2]* | +2.6* | +0.3 |
| Opp.Battlefield.name | +5.6 [+1.2,+10.3]* | +2.5* | +0.7 |
| Me.Graveyard.props | +4.7 [+0.5,+8.9]* | +1.5* | +0.9* |
| Me.Hand.name | +4.0 | +2.2* | +1.1 |
| Me.LibraryCount, Me/Opp.LifeTotal | +1.7 to +2.5 (CI touches 0) | +0.5 to +0.7* | ~+1 |
| Ident.Names | +1.6 | +1.7* | +0.2 |
| Opp.CanPlayLand, Stack, Me.IsDecisionPlayer | <1 | +0.2-0.3* | ~0 |
| everything else | CI spans 0 | ~0 | ~0 |

Readings:

- **Signal:** per-permanent properties on both battlefields, own hand
  properties, and both graveyards. Card identity is the single strongest bag
  on its own (70.1% value acc alone vs 71.7% with everything) but is almost
  fully redundant with the props families (drop-one +1.6, not significant):
  the value head is largely a **deck/matchup prior** read off the names.
- **Noise (≈0 both ways, prune candidates):** Exile, LibraryCount (both),
  Me.CanPlayLand, Me.IsDecisionPlayer (constant on a seat view), ZoneMarkers,
  Opp.ManaPool (always empty when we decide), PlayerMisc/counters, X.Me.HandSize
  (redundant with hand props), DecisionsText/StepOrDecisionType for value.
- **Policy is not learnable from state on this corpus.** No family moves top-1
  off the pick-bot rate (76.4%); override CE stays ~2.2 nats (worse than
  uniform over ~3.4 candidates). Families do move policy CE and, much more,
  the per-candidate Q — the dense target is where state signal shows. Matches
  pn12's finding that override labels are unlearnable; recommend training
  policy heads on candidate values / visits, not argmax labels.

## 3. Opponent-hand belief probe

No existing corpus carried the diagnostic opponent hand, so a fresh one was
generated: `searchteacher -label-extras` (honest teacher, same three catalogs,
15 games/pair, seed 72000000; `data/ex2.labels.jsonl.gz`): **4,524 decisions**,
all with the true opponent hand (mean 5.1 cards, 428 distinct names). Held-out
20% of games = 1,034 decisions (248 overrides). 3 epochs, 5 seeds. Arms:
`base` (redacted view only); `aux` (+ auxiliary multi-label head predicting the
opponent's hand over card names from the state trunk, BCE summed over names,
weight w; trained from the redacted view, playable); `input` (the true
opponent hand fed as name + IR tokens -- the diagnostic `mz-opphand` ceiling,
not playable).

| arm | value BCE | value acc | cand-Q MSE | policy CE | override CE | opp-hand BCE (nats/rec) |
|---|---|---|---|---|---|---|
| base | 0.639±.010 | 66.1% | 0.0903 | 0.739 | 2.22 | (untrained) |
| aux w=0.002 | 0.657±.017 | 64.6% | 0.0924 | +0.006 [-0.001,+0.013] | 2.17 | 23.3 |
| aux w=0.01 | 0.655±.012 | 62.6% | 0.1105 | +0.009 [+0.001,+0.018] | 2.19 | 23.8 |
| aux w=0.05 | 0.686±.006 | 53.3% | 0.1239 | +0.011 [+0.003,+0.020] | 2.17 | 24.3 |
| aux w=0.5 | 0.701±.008 | 49.8% | 0.1243 | +0.012 [+0.003,+0.021] | 2.17 | 24.3 |
| **input (ceiling)** | **0.623±.012** | **68.3%** | **0.0860** | -0.000 [-0.013,+0.011] | 2.19 | -- |

Reference belief baselines on the same split (no network): the marginal
name-frequency prior scores **22.3** nats/record; a prior conditioned on the
opponent's DECK LIST scores **9.5**.

- The **ceiling is small**: knowing the opponent's actual hand improves value
  BCE by 0.016 (CI [-0.052,+0.022] spans 0), value acc +2.2pp, cand-Q MSE -5%,
  and policy CE not at all. Hidden-hand information is not where the
  policy's missing signal is on this corpus (consistent with pn12).
- The **auxiliary head hurts** at every weight (value BCE +0.016 to +0.06,
  policy CE +0.006 to +0.012, CI excludes 0 for w >= 0.01) and its belief is
  no better than the marginal prior (23.3 vs 22.3 nats): with 3.6k training
  decisions the trunk cannot even identify the opponent's deck, which alone
  gets 9.5 nats. A learned belief head is not worth it at this scale; the
  cheap belief is a **deck-conditional prior** (opponent deck list minus
  seen cards), which is what the ceiling would be approached with first.
- Caveat: 4.5k decisions is small (the first, 2x larger extras run was killed
  at 7,170/7,830 games by the session cut and published nothing).

## 4. Held-out decks: name-hash vs IR bag

Hold-out decks (8): fdn-limited FDN13-BG, FDN06-RG, FDN08-WBR; pauper-kernel
Elves, Burn; repo-constructed mono-blue-tempo, death-n-taxes, tron. Train =
decisions where NEITHER deck is held out (minus a 20% game fold, the seen-deck
test, n=6,190); unseen test = decisions whose own deck is held out (n=5,533).
Identity arms: `none` (all name families dropped), `name` (zone|name bag),
`ir` (zone|IR-bag: types, cmc, colours, P/T, keywords, SA kind+API incl. subs,
trigger modes + effect API, static modes, replacement events; from the
compiled IR, 430 cards, 18 unresolved names), `name+ir`. 3 seeds, mean ± sd.

| split | identity | value BCE | value acc | cand-Q MSE | policy CE |
|---|---|---|---|---|---|
| seen decks | none | 0.549±.005 | 71.8% | 0.0655 | 0.706 |
| seen decks | name | **0.538±.002** | 72.9% | 0.0610 | 0.702 |
| seen decks | ir | 0.543±.008 | 72.6% | 0.0624 | 0.707 |
| seen decks | name+ir | 0.540±.005 | **73.1%** | **0.0606** | 0.707 |
| unseen decks | none | 0.688±.010 | 60.4% | 0.0908 | 0.795 |
| unseen decks | name | 0.677±.007 | 61.0% | 0.0889 | 0.802 |
| unseen decks | ir | 0.678±.017 | 61.0% | **0.0865**±.004 | 0.820 |
| unseen decks | name+ir | 0.694±.031 | 60.5% | 0.0905 | 0.816 |

- The **generalisation gap dominates everything**: value acc falls ~12pp
  (72.9% -> 61.0%) and value BCE goes to ~0.68 (empty-state is 0.69) on unseen
  decks. What the net learns is mostly deck-specific.
- On seen decks identity helps (name -0.011 BCE vs none). On unseen decks
  neither identity encoding transfers: `ir` is best on cand-Q (-2.7% MSE vs
  name, inside ~1 sd), equal on value, worse on policy CE. A crude IR bag does
  not buy deck-generic play at this data scale (~30k train decisions, 26 decks).
- `name+ir` overfits most (largest unseen variance).

## Conclusions

1. Keep: battlefield props (both), own hand, graveyards (both). Add to
   policynet `mz`: graveyard contents (absent today, carry +20-27 x1e3 value
   BCE alone).
2. Fix the mzenc seat-view hole: with the opponent's hand hidden it emits
   neither cards nor `CardsInHand`; emit the count.
3. Prune/ignore: Exile, LibraryCount, Me.CanPlayLand, IsDecisionPlayer,
   zone markers, Opp mana pool, counters/misc.
4. Per-turn watchers cannot be measured until the engine tracks them; a cheap
   engine-side counter set (spells cast / life gained / life lost / tokens
   this turn) is the one missing family worth building.
5. Value heads on this corpus are matchup priors; deck-held-out evaluation is
   mandatory for any encoder claim, and neither name hashing nor an IR bag
   generalises at 26 training decks.

## Next tickets

- mzenc: emit `CardsInHand` for hidden hands on a seat view.
- policynet `mz`: add graveyard per-card props (both seats).
- engine: per-turn watcher counters in `state` + view (spells cast, life
  gained/lost, tokens created), then re-run this harness with them.
- training: policy target = candidate values/visits rather than argmax
  teacher; report on held-out DECKS.
- data: a label corpus with far more decks (fdn-limited sealed pools) before
  revisiting IR embeddings; try a learned per-card encoder over IR tokens
  (entity-style) rather than a bag.

## Reproduce

```sh
cd /mnt/sata/gorge-training/encablate
bin/encablate -cards <repo>/.cards -out data/base.gz ../llm-qlora/data/{sb1,c1,c1r,t1,sbt}.labels.jsonl.gz
PY=../llm-qlora/venv/bin/python S=<repo>/scripts/encablate
$PY $S/ablate.py --data data/base.gz --mode ablate  --seeds 1,2,3,4,5 --epochs 2 --results runs/ablate.jsonl --tag base
$PY $S/ablate.py --data data/base.gz --mode keepone --seeds 1,2,3     --epochs 2 --results runs/keepone.jsonl
$PY $S/ablate.py --data data/base.gz --mode decks --identity none,name,ir,name+ir --holdout <8 decks> --seeds 1,2,3 --epochs 2 --results runs/decks.jsonl
$PY $S/summarize.py runs/ablate.jsonl base; $PY $S/summarize_keep.py runs/keepone.jsonl
```
