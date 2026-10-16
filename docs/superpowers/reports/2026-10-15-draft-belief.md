# Draft-informed opponent belief for the az search bots

Date: 2026-10-15. Branch: `wt/draft-belief`. Data:
`/mnt/sata/gorge-training/draft-belief/` (run scripts `run_eval.sh` and
`run_play.sh`, one directory and one log per run).

## Summary

- A seat that only looks at the packs it was passed can say who has most of
  every other seat's deck. In the hosted 4-seat × 3-round pod, a seat sees
  about 80% of each opponent's main-deck nonland cards pass through its own
  hands. The question is only which of the three downstream seats took each
  one. Each opponent also makes 5 to 7 picks (out of 42) that the seat never
  sees.
- A belief sampled from that card flow gets 6× more of the redeal right than
  today's 17Lands colour-filtered belief. At game start, 60.5% of the
  nonland cards a world deals the opponent are actually in its deck, against
  9.8% for (a), on hosted-bot pods. The colour pair is right 58% of the time
  against 11%. After the opponent has shown 3 nonland cards the figures are
  68.5% against 16.1%.
- Bo3 game 1's reveals sharpen game 2 to 78.6% world precision at the start
  of game 2, against 60.5% with the draft alone and 19% for (a) with game
  1's colours remembered.
- Cross-round exclusion helps without any spectating. The cards the seat saw
  its *earlier* opponents play are not this opponent's, and that lifts
  game-start precision from 60.5% to 67.5%.
- **The pick model is what makes it work.** Opponents are modelled as
  rating-plus-pool-colour drafters, so a pick depends on the pool so far,
  and the model is sampled with a particle filter. A fixed latent colour
  pair per seat, sampled exactly, reaches only 29% (mixed pods). The
  particle filter is better than that same sampler *given the true pairs*
  (55%).
- **For play it does not matter today.** In a paired gate (az-c5 pilot both
  sides, beliefs swapped on the same seed), the draft belief scores 0.497 ±
  0.004 against the 17Lands belief over 3,000 games. Even the opponent's
  *real list* scores only 0.498 ± 0.004 at 25 sims and 0.494 ± 0.006 at 100
  sims. The az search does not turn opponent-list knowledge into wins, so no
  legal belief can move its win rate.
- **Recommendation:** do not wire this into hosted play for win rate yet.
  - Keep `limited/draftbelief` and the eventhost recording design below
    ready.
  - Gate any future adoption on the cheap oracle arm (`beliefplay -treat
    oracle`). Only when a search or net change makes the real list worth
    something does the draft belief have something to collect. It would
    collect most of it: 61% of a redeal right against 10% today, and 79% in
    Bo3 game 2.
  - It is cheap to adopt when that day comes: one belief per pod mate at
    deckbuilding (64 decks, about 0.17 s of CPU), and nothing extra per
    decision.

## Information boundary

The belief builder receives exactly one seat's legal observation log, and
nothing else that changes per event:

| Input | Legal because | Where it comes from |
|---|---|---|
| Every pack the seat was shown, in pick order, as it stood (card IDs and names), with the round and pick number | the seat saw it | `draft.PickView.Pack/Round/PickNo`, recorded by `draftbelief.Obs.Add` at each pick |
| The card the seat took from each pack | its own action | the pick |
| Pod size and pass direction (L-R-L) | public table facts | `PickView.Seats`, `draft` package doc |
| Cards it has seen a given pod mate own (revealed in any game of the event) | public plays in the seat's own games | the seat's game logs (`Constraints.Known`) |
| Set table: card values, colours, collation frequencies | public set data (17Lands ratings, MTGJSON collation); a sim-trained value table is public model data, not event state | `draftfeat.SetTable`, `setdata` |
| Pick-model and value-type parameters | fitted on *simulated* pods, never on the event being played | `SMCOptions`, `-belief-types` |

The builder never receives:

- another seat's picks, pool, deck, drafter or persona name;
- the event log, server-side pod state, other seats' PickViews, or anything
  under `eventhost`;
- the opponent's real list. The redeal still never reads it
  (`PublicGame.Closed`).

This is enforced as follows:

- `limited/draftbelief` is pure. `imports_test.go` admits only `limited`,
  `limited/draft` and `limited/draftfeat`, and bans the clock, I/O,
  `draftlab`, `event`, `eventhost`, `internal/*` and the engine tiers.
- Its only per-event input type is `Obs`, which is built from what the seat
  was shown: `{Seats, Packs: [{Round, PickNo, Cards, Took}]}`.
- The harness records `Obs` through `obsDrafter`. It wraps each seat's
  drafter and sees exactly the `PickView` the pod hands that seat. The true
  pools and decks are read only to score, and for the oracle ablation, which
  is labelled as such.

One possible leak to keep an eye on is card IDs. The model relies on
`CardRef.ID` being stable across sightings of the same physical card, which
the seat legitimately sees. IDs are assigned in pack-open order
(`booster.IDs`, seat-major). An ID therefore carries the pack's origin seat,
which a human cannot see. Today nothing in the model reads ID values beyond
equality. eventhost should still either keep that property tested or mint
per-seat opaque IDs (see [Hosted implementation](#what-a-hosted-implementation-needs)).

## The model

**Card flow.** Take a pack the seat passes after its pick at pick k. Its
remainder S0 goes to the downstream seats, and each takes one card in pass
order before the pack returns at pick k+n or runs out. If it returns,
removed set R = S0 minus what came back, and |R| = n−1. If it never comes
back, R is all of S0. Either way R's cards were taken by downstream offsets
1, 2, …, |R|, in that order. Only the *assignment* of R's cards to those
seats is unknown, which is at most 3! orders in a 4-seat pod. A pack the
seat first sees at pick k > 1 had k−1 cards taken by upstream seats, which
the seat never sees. Per opponent that is 5, 6 or 7 picks out of 42, by
offset (unit-tested:
`TestCardFlowBookkeeping`). Everything is accounted for: 42 own picks,
108 cards in removed sets and 18 unseen picks make up all 168 cards.

**Pick model (pool commitment).** A seat scores a card
v_c + κ·fit(c, pool), where fit is the mean over the card's colours of each
colour's share of the seat's picks so far. A colourless card scores 0.5,
and κ ramps up over pack 1. This is pickbot's `RatingColour`, and
approximately what a person does. Picks are softmax(β·score). A **value
type** per seat says whose ratings v it drafts by. The measured runs used
two public tables: 17Lands GIH and the sim-trained `hlogit` values. The
seat's picks select the type. On a seat's *own* picks, with the pool known,
this model ranks the real pick first 94% of the time, for both 17l and the
s1 net drafter when given its matching type (β=8, κ=2 in z units). Measured
by `-calib-mf`, which uses `OwnPickFit`.

**Inference (particle filter).** Each particle is one complete history of
the other seats' picks. The events are every removed set and every unseen
pick, processed in pick-time order:

1. For each removed set, the filter draws an order from the pool model given
   the particle's pools at that moment. The particle's weight is multiplied
   by the set's likelihood, which is the sum over orders.
2. For each unseen pick, it draws a card from collation frequency × the
   pick model under the seat's current pool, given what that seat passed.
3. It resamples when the effective sample size halves.
4. It runs 8 independent filters of 256 particles and pools them, weighting
   each by its evidence estimate.

Step 4 is needed because the first picks are almost symmetric between
seats. A single filter can lock into a wrong labelling, and the evidence of
a filter that found the right one dominates.

The result is a posterior sample of each pod mate's 42-card pool. The
assumed builder (best23, 17Lands ratings) turns 64 sampled pools into 64
deck lists. Those lists are a `searchprobe.Belief` corpus, so the closed
redeal uses them unchanged.

**Conditioning on reveals.** A card the seat saw opponent o play is
*claimed* for o. That forces the instance to o in its removed set, or puts
it into one of o's unseen slots. Each sampled deck is then weighted by
0.05^(number of revealed cards the unforced build left out), and 64 decks
are resampled from 256. The revealed cards are then forced into those decks
by swapping out the lowest-rated nonland. A card seen played by a
*different* pod mate is claimed for that seat, which excludes it from o.

**Ablations kept in the package** for comparison: per-set independent
sampling under draftfeat's pair marginals (`Sample`), a Gibbs sampler and
an exact enumeration over a fixed latent pair per seat
(`SampleGibbs`/`SampleExact`), and a mean field over the pool model
(`MeanField`). All four lose clearly; see the ablation table.

## Measurements

### Setup

- **Pods:** FDN play-arena 4-seat pods with pod seeds 3,000,000+. They
  share no seed with the gate (201+), training (1,000,000+) or calibration
  (9,000,000+) ranges.
- **Population P1, hosted bots:** every seat is pickbot `rating-colour`
  (17l), which is what eventhost bot personas draft with. 100 pods give
  1200 (seat, opponent) pairs.
- **Population P2, mixed:** each seat draws from 17l, `net:s1.dpkw`
  (argmax) or `net:s1.dpkw~1` (sampled, T=1). The noisy sampler stands in
  for an imperfect human. 100 pods, 1200 pairs.
- **Decks:** best23 with 17Lands ratings for every seat. The belief assumes
  the same builder, except in E3.
- **Reveals:** game g's reveal stream is the opponent's 40-card list in a
  seeded random order. Level N is the prefix up to its N-th nonland card,
  lands before it included, which is roughly what N casts and the lands
  played around them show. The colour filter reads every revealed card,
  lands included.
- **Beliefs compared:**
  - **(a)** the 28,366-deck 17Lands train split (`LoadBeliefDir`). Metrics
    read up to 3,000 decks per colour mask, evenly strided.
  - **(b)** registerAZ's 500 random sealed-pool decks.
  - **(c)** the draft belief as a drop-in replacement: 64 decks from the
    seat's log, with the same colour filter `Belief.draw` applies.
  - **(c+)** (c) re-sampled conditioned on the current game's revealed
    cards.
  - **(d)** Bo3 game 2: (c+) also conditioned on game 1's 8 revealed nonland
    cards.
  - **(c+x)** cross-round: (c+) with the two *other* pod mates' 8 revealed
    cards each from the seat's earlier rounds, used as exclusions.
- **Metrics,** for each (seat, opponent, belief, reveal level), averaged
  over pairs with ± one standard error:
  - **pair acc:** share of belief decks whose main colour pair equals the
    true deck's.
  - **world precision:** the share of a world's dealt *unrevealed* nonland
    cards that really are in the opponent's unrevealed deck (multiset). This
    is what the redeal gets right per world.
  - **world recall:** the share of the opponent's unrevealed nonland cards
    that a world contains.
  - **LL/card:** mean log-probability of each true unrevealed nonland card
    under the belief's card marginal, with 1% smoothing over the set's 267
    nonland names. Uniform is about −5.6.
  - **top-k:** share of the true deck's distinct unrevealed nonland names
    that are among the belief's k most likely names, with k = that count.

### Q1: how much does the pack-pass log narrow it? (game start, N = 0)

| belief | P1 pair acc | P1 world prec | P1 LL/card | P1 top-k | P2 pair acc | P2 world prec | P2 LL/card | P2 top-k |
|---|---|---|---|---|---|---|---|---|
| (a) 17Lands, colour-filtered | 0.106 | 0.098 | −5.27 | 0.180 | 0.105 | 0.094 | −5.35 | 0.163 |
| (b) random sealed pools | 0.103 | 0.097 | −5.30 | 0.153 | 0.103 | 0.097 | −5.31 | 0.149 |
| **(c) draft belief** | **0.577** | **0.605** | **−3.66** | **0.736** | **0.553** | **0.568** | **−3.82** | **0.682** |

Standard errors are ≤ 0.009 on pair acc and ≤ 0.005 on precision; the full
tables are in `e1-17l.log` and `e2-mixed.log`. Precision and recall are
equal here because both sides are 23 nonland cards.

The card-flow facts behind it, per opponent offset, averaged over P1 (P2 in
brackets):

| offset | unseen picks (of 42) | true main nonlands the seat saw | share the posterior gives the right seat with P ≥ 0.5 | mean P(right seat) over seen main cards | sampled-deck pair MAP acc |
|---|---|---|---|---|---|
| 1 (left in packs 1 and 3) | 5 | 0.83 (0.84) | 0.76 (0.71) | 0.83 (0.80) | 0.73 (0.65) |
| 2 (opposite) | 6 | 0.80 (0.82) | 0.68 (0.63) | 0.76 (0.72) | 0.68 (0.63) |
| 3 (right in packs 1 and 3) | 7 | 0.77 (0.78) | 0.70 (0.66) | 0.83 (0.79) | 0.74 (0.69) |

The opposite seat is the hardest, because it is always the middle picker of
a removed set. Pure colour-signal reasoning does much worse: draftfeat's
pair posterior (Carry 0.9, fixed-pair model) reads the true pair only 21–37%
of the time.

### Q2: belief quality against ground truth after N revealed nonland cards (game 1)

World precision, with pair acc in brackets:

| N | (a) P1 | (c) P1 | (c+) P1 | (a) P2 | (c) P2 | (c+) P2 |
|---|---|---|---|---|---|---|
| 0 | 0.098 (0.11) | 0.605 (0.58) | 0.605 (0.58) | 0.094 (0.11) | 0.568 (0.55) | 0.568 (0.55) |
| 1 | 0.155 (0.31) | 0.651 (0.73) | 0.663 (0.73) | 0.142 (0.30) | 0.617 (0.69) | 0.641 (0.72) |
| 3 | 0.161 (0.51) | 0.656 (0.85) | 0.685 (0.86) | 0.151 (0.51) | 0.629 (0.81) | 0.673 (0.87) |
| 5 | 0.148 (0.56) | 0.638 (0.90) | 0.683 (0.91) | 0.139 (0.56) | 0.612 (0.84) | 0.678 (0.93) |
| 8 | 0.125 (0.58) | 0.597 (0.93) | 0.672 (0.96) | 0.116 (0.56) | 0.571 (0.85) | 0.669 (0.97) |
| 12 | 0.092 (0.60) | 0.521 (0.94) | 0.649 (0.98) | 0.085 (0.56) | 0.499 (0.86) | 0.642 (0.99) |

LL/card over the same levels: (a) runs from −5.27 to −4.95 and (c+) from
−3.66 to −3.16 (P1). (b) is no better than uniform at any level and gets
worse as cards are revealed (precision 0.043 at N=12). It does not even
respect colours: its random 23 have no colour coherence.

Precision falls slightly at high N for every belief. This is a property of
the metric, not of the belief. The unrevealed remainder shrinks while
recall stays flat, and the right answer becomes "the few cards left".
(c+) holds up because the revealed cards no longer occupy belief slots.

### Q3: within-match and cross-round updating

**Bo3 game 2.** Game 1 revealed 8 nonland cards, plus the lands around
them. Sideboarding is not modelled.

| belief at game 2 | P1 world prec, N=0 | P1 world prec, N=3 | P2 world prec, N=0 | P2 world prec, N=3 |
|---|---|---|---|---|
| (a) as today (no memory) | 0.098 | 0.162 | 0.094 | 0.151 |
| (a) + game-1 colours remembered | 0.189 | 0.166 | 0.177 | 0.154 |
| (c+) draft only | 0.605 | 0.682 | 0.568 | 0.677 |
| **(d) draft + game-1 cards** | **0.786** | **0.774** | **0.784** | **0.774** |

(d) gets the pair right 96–98% of the time. Game 1's reveals are worth
about +18 points of world precision at the start of game 2, and the
remaining error is mostly the 5–7 unseen picks and builder disagreement.
The comparison for (a) shows that game-1 memory without the draft is worth
only about 9 points.

**Cross-round.** The hosted event is a 4-seat round robin, so the seat never
meets an opponent twice. What it does know before round 3 is what its round
1 and 2 opponents played.

- Those cards are not this opponent's. Used as exclusions they lift (c+)
  from 0.605 to 0.675 at N=0 and from 0.685 to 0.697 at N=3 (P1). P2 goes
  from 0.568 to 0.657.
- This is legal with no spectating: they are the seat's own games.
- If the event ever exposes other tables' public plays, an opponent's
  round-1 reveals against someone else are the same kind of evidence as
  Bo3's game 1, a +18-point lift. Spectating is a product decision, and the
  belief would take those reveals through the same `Constraints.Known`
  input.

### Robustness and ablations (P2 mixed pods unless noted; 50 pods, 600 pairs)

World precision at N=0 and N=3 with (c+); Bo3 game 2 uses (d):

| variant | N=0 | N=3 | Bo3 g2 N=0 | pair acc N=0 |
|---|---|---|---|---|
| **SMC, two value types (as measured above, P2)** | 0.568 | 0.673 | 0.784 | 0.55 |
| SMC, true decks built on `hlogit` values, belief assumes the 17Lands builder (E3) | 0.535 | 0.619 | 0.742 | 0.55 |
| SMC, single 17Lands value type | 0.314 | 0.492 | 0.678 | 0.27 |
| SMC, single 17Lands type, P1 pods (all 17l drafters) | 0.611 | 0.685 | 0.787 | 0.58 |
| fixed latent pair, exact enumeration (`SampleExact`) | 0.289 | 0.455 | 0.651 | 0.23 |
| fixed latent pair given the **true** pairs (oracle, not legal) | 0.552 | 0.533 | 0.661 | 0.94 |
| per-set independent, draftfeat pair marginals (`Sample`) | 0.273 | 0.398 | 0.603 | 0.21 |

Readings:

- **Value types matter when drafters disagree with 17Lands.** The s1 net
  drafter agrees with 17Lands' top card only 25% of the time but with
  `hlogit` 94% of the time. A belief that knows only 17Lands values halves
  its precision on such seats (0.31 against 0.57). For hosted bot seats a
  single 17Lands type is the exact model (0.611). For people, 17Lands GIH is
  the natural public prior, but how well it fits humans is **unmeasured**:
  we have no human pod logs.
- **Builder mismatch costs 3–5 points.** Opponents building on different
  values than the belief assumes (E3) is still 5.7× (a).
- **The pool model plus particle filter is the main win.** Every
  fixed-pair sampler, even with oracle pairs, is worse than SMC.

### Q4: does it matter for play?

**Design.** Pods are hosted-bot (P1) pods with seeds 4,000,000+, decks
built by best23 on 17Lands ratings. Pod mates play each other with the
az-c5 pilot on both sides (`c5.dzgw`, `azc3.ClosedConfig`). One side's
belief about its opponent is the treatment and the other side's is (a), the
17Lands train split. The same seed and the same sides are then replayed
with the beliefs swapped. The treatment's score for a seed pair is its mean
result over the two games, so the score is 0.5 under no effect. Per pod
that is 6 pairings × 2 games (sides alternate) × 2 swaps.

| treatment vs (a) | sims | games | treatment score | 95% CI |
|---|---|---|---|---|
| (c) draft belief | 25 | 3,000 | 0.497 ± 0.004 | [0.489, 0.505] |
| **oracle: the opponent's real list** (upper bound, never legal) | 25 | 3,000 | 0.498 ± 0.004 | [0.490, 0.506] |
| **oracle** | 100 | 1,512 | 0.494 ± 0.006 | [0.482, 0.506] |

None of these is distinguishable from 0.5, and the CIs are ±0.8 to 1.2
points, tighter than the ±2-point target. The swap barely matters: in 90%
of seed pairs the same *side* won both games, whichever belief it had. In
the 149 seed pairs where the swap changed the result (25 sims, draft
belief), the treatment won 70 and lost 79.

**So for az-c5 at 25 or 100 simulations, belief quality is not what limits
play.** Not even knowing the opponent's exact list measurably beats the
17Lands corpus, so no better *legal* belief can either. The 6× gain in
redeal accuracy is real, but this search does not cash it. Likely reasons,
all hypotheses that were not measured:

- 83% of decisions are not searched (searched 207k of 1.19M).
- At the decisions that are searched, the opponent's hidden hand matters
  less than the board.
- The value net was trained against 17Lands-belief worlds and is
  calibrated to them.

**What would change it:** a search that does exploit opponent information,
such as combat tricks, removal-aware attacks, and holding up answers, at
higher budgets or with a value net trained on draft-belief worlds. Re-run
the oracle arm first. It is the cheap upper bound (`beliefplay -treat
oracle`, about 6 minutes per 3,000 games at 25 sims on 12 workers), and only
an oracle gain greater than 0 justifies wiring the draft belief into hosted
play.

## Cost

- **Belief build:** one SMC sample of 64 pools plus 64 best23 builds is
  about 0.17 s of CPU per (seat, opponent) at 8 × 256 particles (beliefplay drafted 125 pods and built 1,500 beliefs serially in 262 s). Scoring
  everything above, at 14 sampler calls per pair, took 219 s for 1,200
  pairs on 16 cores, about 2.9 CPU-s per pair.
- **Per decision:** nothing extra. The belief is a 64-list `Belief` corpus
  instead of 28k lists. `draw` is the same O(1) cover lookup, and
  `closedDeal` is unchanged.
- **Reveal conditioning (c+/d):** one re-sample, about 0.5 s of CPU at 256
  pools. It belongs off the decision path: between games for Bo3, or in a
  background refresh when a new card is revealed (at most about 12 per
  game). Without it, (c) is still 4× (a) at every reveal level.

## What a hosted implementation needs

1. **Record each seat's observation log in eventhost.**
   - Store the `PickView` sent to the seat (round, pick number, pack card IDs
     and names) and the ID it took.
   - Store it per seat, in the seat's own private record, and include it in
     replay/resume. eventhost already logs `Picked` and the packs; the
     per-seat view must be derivable *from that seat's entries only*.
   - Expose it to the seat's bot as the seat's own data. It must not be read
     from the pod.
2. **Build the belief once per seat at BUILD.**
   - `draftbelief.Analyze(obs, table, {Freq, Types})`, then
     `SampleSMC(off, 64, nil, DefaultSMC, rng)`, then best23 builds, then
     `searchprobe.NewBelief`. Give the seat one belief per pod mate, keyed
     by relative offset.
   - The az seat needs a per-opponent belief. Today `azc3.ClosedConfig(b)`
     takes one global belief per process (`azc3.SetBelief`). The hosted
     factory would pass the belief for the opponent of the game being
     started, as `beliefplay` does with a per-game `NamedPlayer`.
   - Seed the rng from the event seed and the seat, so a replay rebuilds the
     same belief.
3. **Optional: the seat's own reveals.**
   - Fold the opponent's public cards from earlier games of the series (Bo3)
     and from the seat's earlier rounds (other opponents, as exclusions)
     into `Constraints.Known`.
   - Re-sample between games. It reads only the seat's own game logs.
4. **Privacy tests to add.**
   - (i) Metamorphic: two pods that are identical in seat s's PickViews but
     differ in what other seats picked from packs s never saw must give
     bit-identical beliefs for s.
   - (ii) The builder's inputs are typed so that it cannot take a pod or an
     event (the import purity test already covers the package).
   - (iii) Card IDs: either assert the model reads IDs only for equality, or
     mint per-seat opaque IDs. Pack-open-order IDs reveal a pack's origin
     seat.
   - (iv) Extend `eventhost/privacy_test.go`: a seat's observation log
     contains no other seat's pick or pool.
   - (v) The az leak audit's existing rule still holds: the redeal never
     reads the opponent's real list.
5. **Model data shipped with the server:**
   - the set table (17Lands values) and collation frequencies, which are
     already embedded;
   - optionally a second value type;
   - the SMC parameters, which were fitted on simulated pods (seeds
     9,000,000+): β = 8, κ = 1.5 (z units), 8 × 256 particles.

## Unfinished and caveats

- There is no human data. P2's sampled net drafter stands in for noisy
  drafters, and how well the 17Lands value type fits people is untested.
- The value types in P2 include the tables P2's drafters actually use. That
  is in-distribution: a real opponent's ratings are never exactly a public
  table.
- The reveal model is a random prefix of the list. It ignores that cheap
  cards are cast earlier and that some drawn cards are never shown.
- Sideboarding between Bo3 games is not modelled.
- Only 4-seat pods were measured. 8-seat pods see fewer wheels (two
  sightings per pack) and fall back to sequential sampling for 7-card sets.
- The play gate covers Bo1 game 1 with belief (c) only, on hosted-bot pods.
  (c+)/(d) need a reveal-refresh hook in the az seat, which does not exist
  yet. Since the oracle arm shows no gain, they were not built.
- Mixed-pod (P2) play gates were not run, for the same reason.

## Reproduce

```sh
cd .worktrees/draft-belief && go build -o /mnt/sata/gorge-training/draft-belief/bin/draftrl ./cmd/draftrl
/mnt/sata/gorge-training/draft-belief/run_eval.sh     # E1-E4 (each: draftrl belief ...)
/mnt/sata/gorge-training/draft-belief/run_play.sh     # Q4 paired play gate (draftrl beliefplay ...)
# calibration (seeds 9,000,000+): draftrl belief -pods 16 -seed 9000000 -league ... -sampler smc -smc-beta B -smc-kappa K -levels 0
# own-pick fit of the pick model:  draftrl belief -debug -calib-mf "2,4,8;0,1,2,3" -league ...
```

Tests: `go test ./limited/draftbelief/` covers card-flow bookkeeping
(every removed set is one card per named downstream seat, and the 168
cards are all accounted for), full 42-card pools and determinism for every
sampler, known cards landing in every sampled pool, SMC recovering the
assignment on a pool-model pod, and import purity.
