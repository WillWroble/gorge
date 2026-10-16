# Draft-bot reinforcement learning: design

Status: draft for operator review, 2026-10-14. Base: main@5f4d124df.
Builds on the draft-events spec (`2026-10-13-draft-events-design.md`, E4 "better bots") and the
E5 composition API (`draftlab`). Every `path:line` below was read at the base SHA; anything not
read from code is marked **INFERRED**.

## 0. Summary

A four-stage program that turns the cold-start card values from `lab/cmd/cardval` into a learned
pick policy that reads (a) its own pool, (b) card strength with its uncertainty, and (c) the
table: what other seats are taking, inferred only from what this seat legally saw.

| Stage | What | Gate (gauntlet is the authority) |
|---|---|---|
| S0 | Value-table drafter: `rating-colour` scoring with cardval az-c4 values instead of 17Lands (in flight, another agent) | S0 ≥ BR + ⅔·(B17 − BR) |
| S1 | Behaviour-clone a small transformer from S0, plus an auxiliary seat-prediction head | top-1 agreement ≥ 85%; gauntlet non-inferior to S0 |
| S2 | PPO on drafted-deck win rate, az-c4 piloting both seats, pod-mean baseline, league of frozen drafters | S2 − S1 ≥ +3 pp (paired, lower 95% bound > 0) and S2 ≥ B17 − 1 pp |
| S3 | Recompute card values from S2's drafted-deck games and feed them back as priors | S0′ − S0 ≥ +1 pp; S2′ non-inferior to S2 |

B17 is the 17Lands-rated `rating-colour` drafter and BR is the random-from-pool baseline. Both
are measured under the gate pilot (§3.0). 17Lands correlation is reported at every stage but
never gates anything.

### Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | The policy's only input is `draft.PickView` plus a static per-set table. Features are a pure function of the view, recomputed from `Seen` at every pick, with no drafter state. | This is the legality guarantee: the pod hands a seat copies of its own history and nothing else (`lab/draft/limited/draft/draft.go:121-143`). Being stateless also survives the event-restart rule (draft-events Q9), which rebuilds drafters from the log. |
| D2 | Omniscient information appears only in a training-time critic and in auxiliary labels, both in the Python trainer. Neither is ever exported into the weights file the Go drafter loads. | Operator rules: never trust the client, and no bot in an event reads decklists. |
| D3 | Rollout and hosted inference are Go-native (new pure `lab/draft/limited/draftnet`). Python trains and exports weights. Go computes the features and Python consumes them, never recomputing them. | Hosted bots must be in-process `pickbot` drafters (`lab/draft/eventhost/draft.go:141`). One feature implementation means no train/serve skew. This is the same split as `bots/gonet` (`bots/gonet/gonet.go:15-33`). |
| D4 | (Revised by the §11 ruling.) The training reward is each seat's score in the **round robin within its pod**: its built deck against the other three seats' decks. No 17Lands deck enters a rollout; the fdn-limited gauntlet decks are evaluation only, and a card-list-hash guard refuses them. | The reward needs no human data, so it works on a cold set. Intra-pod games are zero-sum, which invites co-adaptation with the league (§9 R1); the past-checkpoint league, the fixed gauntlet gate and the mixed-pod check push back. |
| D5 | The builder is fixed for the whole program: `best23` with the same value table as the policy's prior. | This isolates drafting. Builder learning is a separate program. |
| D6 | Pods hold 2 learners and 2 frozen league seats. The baseline is the leave-one-out mean over all 4 seats' rewards. | The 4 seats share packs, so the pod mean cancels pack quality. Frozen seats keep the table's behaviour non-stationary only slowly. |
| D7 | Persona identity is never a feature. | Prevents learning to exploit a named bot. The policy must read behaviour, not labels. |
| D8 | Seat posteriors are an explicit Bayesian computation in Go, fed to the network as features. The network refines them, and an auxiliary head is trained against true labels. | Interpretable, ablatable, and gives table features signal from S1 on. S0 ignores the table, so pure behaviour cloning would never learn to use it. |

## 1. What exists (verified)

- **Pick interface.** `pickbot.Drafter.Pick(v PickView, r *rand.Rand) string`
  (`lab/draft/limited/pickbot/registry.go:93-95`). `PickView` = `{Set, Round, PickNo, Pack, Picks, Seen}`
  (`lab/draft/limited/draft/draft.go:45-60`).
  - `Seen` is every pack the seat picked from, as it stood before the pick, across all rounds
    (`draft.go:174`).
  - `Picks[i]` is the card taken from `Seen[i]`.
  - CardRef IDs are unique event-wide (draft-events §4.1), so a wheeled pack is recognised by
    ID overlap.
  - Direction: odd rounds pass left (`seat+1`), even rounds right (`draft.go:229-234`).
  - The view carries no pod size and no seat index.
- **Current drafter.** `RatingColour` (`lab/draft/limited/pickbot/drafter.go:126-154`) scores centred
  rating + k·colourFit, with k ramping over pack 1 (`ColourFitMax` 8, `registry.go:80`).
- **Hosting.** `lab/draft/eventhost/persona.go:26-42` accepts only the concrete
  `RatingColour`/`Best23` types. The hosted pick rng is per pick (`lab/draft/eventhost/draft.go:19-22`),
  where draftlab's is per seat (`lab/draft/draftlab/draft.go:176`).
- **Composition API.**
  - `Lab.Draft` takes per-seat `NamedDrafter`s (`lab/draft/draftlab/draft.go:47`). A caller-built
    `NamedDrafter{Ref, Drafter}` is accepted (`lab/draft/draftlab/agents.go:99`), the way extern does.
  - `Lab.Gauntlet` (`lab/draft/draftlab/gauntlet.go:52`) and `Lab.Games` (`lab/draft/draftlab/games.go:14`) play
    games.
  - `GameResult.OpeningHand`/`Drawn` give the 17Lands in-hand sets per player
    (`lab/draft/draftlab/play.go:49-50`).
  - `Lab.GameDataCSV` writes 17Lands `game_data` shape (`lab/draft/draftlab/csv.go:87`).
  - Env seats run only with `Options.AllowDecklistBots` (`lab/draft/draftlab/lab.go:28`), driven through
    the bench Env driver (`lab/draft/draftlab/play.go:110-136`).
- **az-c4.**
  - Registered as `Env: true, ClosedLists: true` (`bots/azc4/azc4.go:98`), so
    `ReadsDecklists()` is false (`bots/registry.go:169`).
  - It needs `Enable(net, belief)` (`azc4.go:48`). gorged builds the belief from a deck dir
    plus a `pool.tsv` train split (`cmd/gorged/azc4.go:76`).
  - The split on `/mnt/sata/gorge-training/dzgorge-repro/inputs/pool.tsv` is 28,366 train /
    3,150 eval decks (counted).
- **cardval.**
  - `registerAZ` (`lab/cmd/cardval/az.go:66-114`) registers a closed-list az pilot whose belief is
    random archetype decks (`:74-91`), so no 17Lands deck corpus is needed. Defaults are
    `-az-sims 25` and `-az-net ""` (`lab/cmd/cardval/main.go:112-114`).
  - `handLog` (`lab/cmd/cardval/hands.go:21-69`) is the in-hand fold that draftlab's
    `OpeningHand`/`Drawn` now provide natively.
- **Gauntlet.**
  - `lab/cmd/draftsim -gauntlet fdn-limited` accepts only `-set FDN` (`lab/cmd/draftsim/gauntlet.go:53-58`).
  - It refuses decklist readers (`:75`).
  - It takes one `-drafter` for all seats (`lab/cmd/draftsim/main.go:43`).
  - It drives games **without** `EnvSeats` (`lab/cmd/draftsim/game.go:168-178`), so it cannot seat
    az-c4 as is.
  - The gate decks are 16 wr60 17Lands FDN decks (`lab/training/spellbench/fdn.go:10-23`).
- **S0 in flight** (worktree `wt/draftrl-s0`, uncommitted when this was written):
  - `pickbot.RatingsOverride`/`ParseRatingsOverride` (`lab/draft/limited/pickbot/override.go`) plus
    draftsim `-ratings-file/-ratings-field`.
  - It rescales cardval values onto the 17Lands `gih_wr` scale and **requires an embedded
    17Lands file for the scale**, so a truly cold set fails there (§9 R6). **INFERRED** that it
    lands in this shape.
- **Measured rates.**
  - cardval az-c4 at sims=25, both seats: 20,000 games in ~39 min on 16 workers ≈ 8.5 games/s
    (`/mnt/sata/gorge-training/cardval/az-c4-s25`, `run.json` and `az.json` mtimes). The
    brief says ~10.
  - Scripted bot: 160–200 games/s on 16 workers (`cardval/runs.log`).
  - `BenchmarkGames` (`lab/draft/draftlab/extern/bench_test.go:33`, operator numbers, 4 workers, 40-turn
    cap): builtin 1.25 M games/h, ext-stdio-lean ~327 k games/h. Not re-measured here.
- **Baselines** (operator): `rating-colour` 48.1%, random-from-pool 31.5% under the `bot` pilot (an earlier draft said 25.7%; 31.5% is the measured figure).
  Both are draftsim homogeneous pods, `play-arena`, seeds 201–224, fdn-limited gauntlet.
  **INFERRED**: the pilot was draftsim's default `-gauntlet-player bot`, since az-c4 cannot be
  seated there.
- **Precedent.** In-repo PPO with a KL anchor (`bots/policynet/ppo.go:1-40`). Python trains
  and Go evaluates (`bots/gonet`).

## 2. Hard constraints

1. **Legal view only.** `limited/draftfeat.Extract(v draft.PickView, t *SetTable) Features` is
   the only feature entry point.
   - Its package imports `limited`, `lab/draft/limited/draft` and `cards` only. The import test forbids
     `event`, `eventhost`, `draftlab` and everything `lab/draft/limited/pickbot/imports_test.go:18` already
     forbids.
   - `SetTable` is public, static set data: card facts, prior values, opponent-model constants.
     It never holds per-event data.
2. **Training-only omniscience.**
   - The critic and the seat-prediction labels live in the Python trainer.
   - `draftnet.Load` refuses a weights file that carries any tensor outside the policy graph.
     A test feeds it a file with a `critic.*` tensor.
3. **Purity and determinism** (draft-events §3).
   - `lab/draft/limited/draftfeat` and `lab/draft/limited/draftnet` are pure: no `os`, `time` or legacy
     `math/rand`.
   - Weights arrive as `[]byte`.
   - Float reductions run in fixed order.
   - Sampling uses only the `r` the pod hands in.
   - A rollout is reproducible from (iteration seed, weights hashes).
4. **Test budget.** Every test fits 4 GB RSS, 4 CPU and 1 min. Game-playing tests use the
   scripted bot or a **no-net** az pilot at sims ≤ 2, with one pod and ≤ 2 games per seat.
5. **Compute discipline.**
   - One heavy job at a time, via `ops/scripts/heavy.sh heavy --mem 24G`.
   - `heavy.sh` sets no `MemorySwapMax` (`ops/scripts/heavy.sh:177-178`), so the launcher adds
     `-p MemorySwapMax=0`.
   - The job honours `GORGE_PAUSE_FILE` at iteration boundaries.
   - All data goes under `/mnt/sata/gorge-training/draftrl/`.

## 3. Staged plan

### 3.0 The gate (authority)

**Protocol.**
- Homogeneous 4-seat pods, `play-arena` packs, event seeds 201–224 (24 events, 96 seats).
- Each seat's `best23` deck plays the 16 fdn-limited decks, 4 games each with sides alternated:
  6,144 games per arm, 95% CI half-width ≈ 1.25 pp.
- Arms compare **paired**: the same seeds give the same packs.
- Seeds 201–248 never appear in training. Training seeds start at 1,000,000.

**Pilot.** The authority pilot is the az-c4 closed-list search at sims=25 on both seats, with
the C4 net and the belief built from the train split. The training pilot is the same az search
(the S2 loop takes the net as config: C5 once it exists).

A secondary pilot, the scripted `bot`, runs at every gate as a pilot-exploit guard (§9 R2).
A final S2 confirmation runs at sims=100, the hosted config.

**First action (G0).** Re-baseline BR, B17 and S0 under the az pilot. All thresholds are
relative to those numbers.

**Where it runs.** The gate is `lab/cmd/draftrl gate` over `draftlab`, not draftsim. draftsim
cannot seat an Env pilot (§1) or mix drafters per seat. DR-4's acceptance requires the
`bot`-pilot arms to reproduce 48.1% and 31.5% within their CIs. Pack and game seed streams
differ between draftsim and draftlab, so agreement is statistical, not byte-identical
(**INFERRED**).

**Mixed-pod check** (S2 and later): pods of 2 candidate seats and 2 B17 seats, the same seeds.
The candidate seats' gauntlet must be ≥ the B17 seats' at the same tables (point estimate).

**Ablation verdicts** use seeds 201–248: 6,144 games, paired SE of a difference ≈ 0.9 pp.

### 3.1 S0: value-table drafter (other agent)

`RatingColour` with `Ratings` set to cardval values (`gih` field of az-c4-s25 `values.json`),
and `Best23` reading the same values.

Gate: S0 ≥ BR + ⅔·(B17 − BR). Under the `bot` pilot that is ≥ 40.6%.

The bar shows the sim values carry most of what 17Lands carries. Missing it does not block S1,
but S2's bar then rests on S1 alone.

### 3.2 S1: behaviour clone

**Data.**
- 30k draft-only pods (no games). Each pod seats 2 S0 seats and 2 variants drawn from
  {S0 with `ColourFitMax` 4 or 12, B17}, so the table is diverse.
- Records come from every seat.
- BC targets come only from S0 seats, as a soft target `softmax(score/τ_T)`. τ_T is set so the
  teacher's mean top-1 probability ≈ 0.7.

**Loss.** `CE(π_θ, teacher) + 0.3·CE(seat head_j, y_j)`, where y_j is seat j's final
main-deck pair (two most-played colours of its built deck, a training label).

**Gate.**
- Top-1 agreement with S0 ≥ 85% on held-out pods.
- Gauntlet S1 − S0: point estimate ≥ −1 pp and lower 95% bound ≥ −3 pp.
- Seat-head top-1 accuracy at the last pick of round 2 beats the naive baseline. The naive
  baseline is the colour mass of the cards the seat saw removed, assigned to the nearest
  downstream seat. This is a diagnostic, not a gate.

S1 does not get stronger by construction. It exists to give S2 a stochastic starting
distribution and seat tokens already shaped by the auxiliary loss.

### 3.3 S2: PPO on drafted-deck win rate

**Iteration.**
1. 256 pods. Each pod seats 2 learner seats at random positions, sampling at temperature 1,
   and 2 league seats.
2. League draw: S0 20%, B17 20%, S1 20%, past checkpoints 40% (uniform over the last 10,
   added every 5 iterations).
3. Every seat builds with `best23` and the current value table.
4. The pod plays a round robin (§11): every pair of seats plays K games (K = 4 default),
   sides alternated, under the az pilot, so each seat plays 3K games. Draws and stalls score
   0.5. No 17Lands deck is an opponent.
5. R_i = seat i's mean score over its round-robin games (`seats.jsonl` `score`).

**Implementation (DR-9).** `lab/cmd/draftrl rollout -learner net:W~1 -learners 2` seats the learner
at 2 random positions per pod and draws the other seats from `-league` with `-league-weights`
(the league draw never returns the learner). The manifest records `learner`/`learner_idx`;
PPO trains only on that drafter's picks, using the picks.bin v2 behaviour log-prob as the old
log-prob. The trainer (`py/ppo.py`) and the resumable loop (`s2/s2loop.sh`) live out of repo
under `/mnt/sata/gorge-training/draftrl/`. Because the round robin is zero-sum with equal games
per seat, the pod mean is exactly 0.5 and the leave-one-out advantage is (4R_i − 2)/3; the
baseline still matters once seats have unequal game counts and for the ablations.

**Advantage** for a pick at step t taking card c:

```
A_t = (R_i − mean_{j≠i} R_j) + η · C_i(c)
C_i(c) = Σ_g 1[c ∈ hand_g] · (w_g − R_i) / (n_c + 2)   if c is in i's main deck, else 0
```

- `hand_g` is `OpeningHand ∪ Drawn` of game g, by name, so copies share credit.
- A_t is normalised per batch.
- η starts at 0 (S2a). η = 0.5 is an ablation (S2b). The in-hand term is GIH-style shaping,
  not an unbiased gradient, so it earns its place only through the gate.

**Update.**
- PPO-clip with ε 0.2, 4 epochs, minibatch 2,048 picks, Adam 3e-4.
- KL anchor to S1 with β 0.01, decayed to 0 by iteration 40. This follows the
  `policynet/ppo.go` recipe.
- Entropy bonus 0.01.
- The seat auxiliary loss continues at 0.3, with labels from the rollout's real seats.

**Critic.** An omniscient-at-t value net (Python only) is ablation S2c. Its inputs at pick t:
- every seat's pool,
- every pack in flight before the action,
- the unopened packs.

It must never see later picks by other seats, which depend on the learner's action and would
bias the baseline.

**Run.** 100 iterations, with a gate every 20. Keep the best checkpoint.

**Gate.**
- S2 − S1 ≥ +3 pp (paired, lower 95% bound > 0).
- S2 ≥ B17 − 1 pp.
- The mixed-pod check passes.
- `bot`-pilot S2 − S1 ≥ −2 pp.
- The sims=100 confirmation keeps the gain's sign.

### 3.4 S3: values from drafted-deck games

1. Feed every S2 rollout game (both sides are drafted pod decks, tagged by drafter) through cardval's offline estimators (`gih`, `deck`, `hlogit`) via `GameDataCSV`.
   Archetype-conditional values and a pair-lift table come from the same data.
2. **S0′** = S0 with the S3 values. Gate S0′ − S0 ≥ +1 pp paired, which tests the random-deck
   bias claim directly and cheaply.
3. If S0′ passes:
   - the new values replace the prior-value features and the builder's ratings;
   - S2 resumes from its best checkpoint for 20 iterations (KL anchor to itself, β 0.01);
   - **S2′** must be non-inferior to S2 (≥ −1 pp).

At most two S3 rounds unless each round gains.

Selection bias: a card's games exist only where the policy took it. This is the same bias
17Lands GIH carries, and it is accepted.

## 4. Features

Notation:
- seat s, pod size n, round r, direction d_r = +1 for odd r and −1 for even r, pick k, pack
  size P (14 for FDN);
- the 10 FDN colour pairs a ∈ A;
- `v_i` = prior value of card i (centred z-score);
- `fit(i, a) = |col(i) ∩ a| / |col(i)|`, with colourless = 0.5.

**Pod size.** n is needed before the first wheel. DR-1 adds `Seats int` to `PickView`, filled
by `Pod.Next` (`draft.go:132`). It is a public table fact.

### 4.1 Static per-set table (`SetTable`, built offline, hash-versioned)

Per card:
- colour mask and pips;
- MV;
- type bits;
- rarity;
- role bits: removal (`pickbot.Heuristic`'s oracle rule, `lab/draft/limited/pickbot/heuristic.go:63-69`),
  evasive creature, fixing land;
- `v_i`;
- `se_i`;
- `v_{i,a}` for the 10 pairs.

**Uncertainty `se_i`.** Split the cardval games by seed parity and compute `v` on each half.
Fit k in `se_i = k / sqrt(n_i)` (n_i = in-hand games) by pooling `(v_even − v_odd)²/4` over all
cards. A single card's split-half difference is too noisy to use alone.

**Pair-conditional value.** `v_{i,a}` comes from games whose deck pair is a (cardval's `pa`/`pb`
columns). It is shrunk to `v_i` as `(n_{i,a}·v̂_{i,a} + 50·v_i)/(n_{i,a}+50)`.

**Opponent-model constants.** β, κ, the κ ramp and the per-pick rejection threshold m_k. They
are fitted by maximum likelihood on simulated league pods (DR-3).

### 4.2 Pool signals (group P)

- **P1 commitment.**
  - Colour shares `c[col] = Σ_{p∈Picks} max(0, v_p − v_repl)·pipshare_p[col]`, normalised,
    plus their total mass. `v_repl` is the 25th-percentile playable value.
  - Per pair a: the sum of the top-23 on-pair playable values (best23's pair scan), the
    on-pair playable count and the creature count.
- **P2 holes** for the top-2 pairs: deficits `max(0, target − count)` for creatures (15), 2-drops
  (4), 3-drops (4), removal (3) and MV 6+ excess over 3. The targets are a starting heuristic;
  the network learns weights on them.
- **P3 synergy.** Implicit until S3, through candidate→pool attention. From S3 on, candidate
  feature `Σ_{p∈Picks} L(cand, p)` over a shrunk pair-lift table L from drafted-deck games.

### 4.3 Card strength (group S, per candidate)

- `v_i` and `se_i`.
- `v_{i,a}` for the top-2 pairs, and `max_a v_{i,a}`.
- Rank of `v_i` in the pack.
- Marginal gain to each top-2 pair's top-23 sum if taken.

### 4.4 Table signals (group T), from `Seen` + `Pack` only

**T1 wheels and missing cards.**
- A pack seen at pick k returns at pick k+n (same IDs).
- The removed set `R = Seen[k] − Pack_{k+n} − {own pick}` has n−1 cards, taken by seats
  s+d_r, s+2d_r, … in that order. Order of the cards within the set is unknown.
- Raw feature: the colour mass of the last wheel's R (5 floats).

**T2 lateness.** At a first sighting (k ≤ n), per colour: the max and sum of `v − E_k[v]` over
visible cards. `E_k` is the expected value at pick k, taken from the S0 simulations. This is
what upstream seats passed (10 floats).

**T3 seat posteriors.** `q_j ∈ Δ(A)` for each other seat j.

- Opponent pick model:
  `π_j(c | pack, a) ∝ exp(β·(v_c + κ_k·fit(c,a)))`.
- Wheel update: enumerate the (n−1)! assignments of R to the downstream seats in order.
  - Each assignment's likelihood is `Π_m π_{s+m·d}(c_{σ(m)} | pack state, a_m)`. The pack state
    is replayed in order.
  - The update is exact for n = 4 (6 assignments). For n = 8 it is 5,040 assignments × 10
    pairs per wheel; budget ≤ 1 ms per pick, measured in DR-3.
- First-sighting update: every upstream seat on the pack's path rejected the visible set V.
  `L_j(a) = e^{β m_k} / (e^{β m_k} + Σ_{v∈V} e^{β(v_v + κ_k fit(v,a))})`, where m_k is the
  score of a phantom "card not in V".
- Round carry-over: `q ← 0.9·q + 0.1·uniform`.
- Everything is recomputed from `Seen` at each pick (D1).

**T4 availability.** For direction δ ∈ {current, next round's}, with upstream seats
`u_m = s − m·δ`:

`avail_δ[col] = mean over the remaining picks' path lengths L of Π_{m≤L} (1 − τ·P_{u_m}(col))`

Here `P_u(col) = Σ_a q_u(a)·1[col ∈ a]` and τ is a calibrated take rate. Also computed per pair
(2 × 15 floats).

**T5 wheel probability per candidate.** `Π_{m=1}^{n−1} (1 − π_{s+m·d}(c | Pack−{c}, q))`,
holding the pack fixed. It is defined only when |Pack| > n.

Persona identity, other seats' picks beyond what wheels reveal, other seats' decks and every
log field are absent by construction.

## 5. Model and losses

**Tokens.**
- **CTX** (1): round, k/P, n, d_r, P1, P2, T1, T2, T4.
- **SEAT** (n−1): relative-offset embedding, an upstream-this-round flag, `q_j`, colour
  marginals, evidence counts.
- **POOL** (≤ 41): card token + pick index.
- **CAND** (≤ 14): card token + groups S and P3 + T5.
- **Card token** = 32-d learned set-specific embedding (zero-initialised, so a new set starts
  from facts alone) ‖ linear(facts, `v`, `se`, `v_{·,a}`).

**Network.** Pre-LN transformer, 2 layers, d 64, 4 heads, FF 128, token-type embeddings, full
attention over ≤ 64 tokens. About 80k parameters, with Go forward ≈ 4 M multiply-adds per
pick (**INFERRED** ≲ 2 ms).

**Heads.**
- CAND → logit (the policy).
- SEAT → 10-way pair (auxiliary).
- CTX → scalar value (diagnostic, non-omniscient).

The omniscient critic is a separate Python MLP and is never exported (constraint 2).

**Hosted play.** Argmax with seeded tie-break, like `RatingColour`. Training samples at
temperature 1 using the seat rng.

**Drafter spec.** `lab/cmd/draftrl`'s `-league`/`-drafters` take
`net:WEIGHTS.dpkw[@TABLE.json][~T]`. With no `~T` the seat is argmax; `~T` (a decimal
T >= 0, e.g. `net:s1.dpkw~1`) samples softmax(logits/T) from the seat's pick rng, which
`draftlab.Draft` derives from (pod seed, seat) only, so picks are identical at any
`-workers`. The temperature is the text after the last `~` only when it parses as a
number; a `~` inside a path is left alone. A sampling seat's drafter ref is
`net@<hash>~T`. `NetDrafter.Pick` stays allocation-free at T > 0.

**Behaviour log-prob in `picks.bin` (version 2).** The record's former zero pad byte is
`flags`; bit 0 means a behaviour block follows the logits: `f32 T, f32 log
softmax(logits/T)[chosenIdx]`. Only a sampling net seat sets it. Version 1 files have the
byte 0 and read unchanged under a v2 reader; a v1 reader must skip the 8 bytes when bit 0
is set. `picks.jsonl` carries the same as `has_behaviour`/`temp`/`logp`.

## 6. Compute budget

| Item | Cost | Source |
|---|---|---|
| az pilot, sims=25, both seats | ≈ 8.5 games/s on 16 workers (≈ 30k/h) | cardval az-c4-s25 |
| scripted bot pilot | 160–200 games/s on 16 workers | cardval runs.log |
| S2 iteration: 256 pods × 6 pairs × K = 4 games (round robin, 12 games/seat) | 6,144 games ≈ 10–12 min at ~10 games/s | rollout-smoke (az-c4, 16 workers) |
| Drafting + Go inference per iteration | 256 × 168 picks × ≲ 2 ms ≈ 86 CPU-s ≈ 6 s wall | **INFERRED** |
| PPO update (21.5k learner picks × 4 epochs, CPU torch) | < 1 min | **INFERRED** |
| S2, 100 iterations | ≈ 28 h, ≈ 820k games | derived |
| Gate arm at sims=25 (6,144 games) | ≈ 12 min measured (G0); ≈ 50 min at sims=100 (**INFERRED** 4×) | measured |
| S1: 30k draft-only pods | minutes to draft; ~5 M picks, ~6 GB of features | **INFERRED** |
| S1 training (5 M picks × 3 epochs, CPU) | 30–60 min | **INFERRED** |
| Ablation, 30 iterations each | ≈ 9 h each | derived |

**Memory and process model.**
- One heavy scope: `heavy.sh heavy --mem 24G` plus `MemorySwapMax=0`, `GOMAXPROCS=16`,
  `GOMEMLIMIT=16GiB`. These are cardval's proven caps (`cardval/runs.sh`).
- The Go rollout phase and the Python training phase alternate inside the scope and never
  overlap.
- No gate runs while the loop runs: the loop pauses at an iteration boundary, gates, and
  resumes.

**Disk** (168 GB free on `/mnt/sata`, measured).
- Per iteration ≈ 30 MB of pick features plus game CSVs.
- Features are written as card indices plus compact float32 dynamic features, with static card
  features looked up from `SetTable`.
- Layout under `/mnt/sata/gorge-training/draftrl/`: `stage0/`, `s1/`, `s2/iter-NNNN/`,
  `league/`, `gate/`, `values/`, and `py/` (the trainer, out of repo like `dzg`).

**Where inference runs (D3).**
- The extern path's 3.8× game-throughput cost (1.25 M vs 327k/h) applies to *player* seats.
  A drafter makes 168 calls per pod, so its extern cost would be small too.
- Extern was rejected anyway because:
  - hosting needs an in-process `pickbot` drafter;
  - a 10-checkpoint league would mean 10 agent processes;
  - every pick ships `Seen` as JSON (`lab/draft/draftlab/extern/drafter.go:60-71`), O(k²) per draft.
- Extern stays allowed for architecture spikes before DR-5 freezes the graph.

## 7. Ablations

Each run is 30 S2 iterations from a BC model retrained with the group zeroed in both BC and RL.
The verdict uses seeds 201–248 (§3.0). Order follows the operator's hypothesis first:

1. **T-all off.** The headline test of "table signals".
2. **T3/T4/T5 off, T1/T2 kept.** Do posteriors beat raw signals?
3. **P1/P2 off.**
4. **S `se` and `v_{·,a}` off.**
5. **P3** (S3 only).
6. **Credit and baseline:** η ∈ {0, 0.5}, the critic baseline (S2c), and K ∈ {4, 8, 16} at
   fixed games per iteration.
7. **Reward:** withdrawn by §11 (17Lands decks never enter rollouts; the round robin is the reward).

A group is kept unless its removal *improves* the gauntlet with a 95% CI excluding 0.

## 8. Tickets

Every test runs under `systemd-run --user --scope -q -p MemoryMax=4G -p CPUQuota=400% env
GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -timeout 2m -run X ./pkg` and finishes in < 1 min. Hot
files: only DR-1 (`lab/draft/limited/draft/draft.go`, one field) and DR-6 (`lab/draft/eventhost/persona.go`, one
case) touch existing hot paths. `docs/agents/repo-map*.md` rows land in DR-10 alone.

| ID | Scope (files) | Tests / acceptance | Depends |
|---|---|---|---|
| DR-1 | `lab/draft/limited/draft/draft.go`: `PickView.Seats`, set in `Next`. Drafter fingerprints must not move. | `draft_test.go`: Seats = pod size for 4 and 8. `draftlab` `TestBuiltinFingerprints` unchanged. | — |
| DR-2 | Extract `cmd/gorged/azc4.go:76 loadBelief` into `bots/searchprobe/beliefdir.go` (`LoadBeliefDir(reg, dir, pool)`); gorged calls it. | Tiny `testdata` pool.tsv (3 decks, 1 eval) loads only the train decks. Corpus-gated. | — |
| DR-3 | New pure `lab/draft/limited/draftfeat`: `SetTable` (+ JSON codec), `Extract`, groups P/S/T. Wheel detection, exact-permutation posteriors, rejection likelihood, availability, wheel probability. `imports_test.go`. | Golden features on a hand-built 4-seat history. Posterior recovers a scripted opponent's pair (≥ 80% top-1 by round 2 pick 5 over 200 draft-only pods, < 20 s). n = 8 wheel update ≤ 1 ms (benchmark, not a gate). Determinism (twice → equal). | DR-1 |
| DR-4 | New `lab/cmd/draftrl`. `gate` subcommand: pods from seeds, per-seat drafters/builders as `NamedDrafter`s, `Lab.Gauntlet` vs `CatalogDecks("fdn-limited")`. Pilot `bot` or the closed-list az (registered like `lab/cmd/cardval/az.go:66-114`, belief via DR-2). Prints draftsim's `built:`/`random:` contract. `calibrate` subcommand fits the opponent-model constants into `SetTable`. | Smoke: 1 event, `maxHuman` 1, 1 game, `bot` pilot; and the no-net az at sims 2. **Operator-run acceptance (not a test):** the `bot` pilot reproduces 48.1% / 31.5% within CI. | DR-2, DR-3 |
| DR-5 | New pure `lab/draft/limited/draftnet`: `.dpkw` format (header + named float32 tensors), `Load([]byte)` (refuses unknown tensors), `Forward`. `NetDrafter` in `lab/draft/limited/pickbot/netdrafter.go` (argmax or temperature, optional `Recorder` hook for logits and features). | Parity: committed fixture (tiny random weights + inputs + expected logits from the Python trainer) within 1e-4. `critic.*` tensor refused. imports test. | DR-3 |
| DR-6 | Hosting: `lab/draft/eventhost/persona.go` accepts a registered `NetDrafter` persona; gorged `-draft-net`/`-draft-table` flags register it (mirrors `azc4.Enable`). | `eventhost` persona test with a fixture net. A privacy test asserts the persona is constructed from the `PickView` path only. | DR-5, operator ruling Q4 |
| DR-7 | `lab/cmd/draftrl rollout`: league pods, `best23` with value-table ratings (`RatingsOverride`), the round robin within the pod, K games per seat pair (§11; gate decks excluded by card-list hash), az pilot. Writes per-iteration `picks.bin` + `picks.jsonl` + `seats.jsonl` + `GameDataCSV` + event logs + manifest. Honours `GORGE_PAUSE_FILE`. | Smoke: 1 pod, `bot` pilot, 2 games/seat; records reproduce from the seed. Gate-deck exclusion test. | DR-4, DR-5, S0 merged |
| DR-8 | `lab/cmd/draftrl bc`: draft-only S0/variant pods writing teacher scores + seat labels. | Smoke: 2 pods; the label equals the built deck's top pair. | DR-7 |
| DR-9 | Trainer (out of repo, `/mnt/sata/.../draftrl/py`): BC, PPO, critic, `.dpkw` export, parity-fixture generator; loop script alternating rollout and train under heavy.sh. | Operator-run S1 and S2 gates. | DR-5, DR-7, DR-8 |
| DR-10 | `lab/cmd/draftrl values`: rollout `GameDataCSV` → split-half `se`, pair-conditional values, pair-lift table → new `SetTable`. Repo-map rows. | Fixed 50-game CSV → golden table. | DR-7 |
| DR-11 | Cold-start fix for S0: `ParseRatingsOverride` falls back to a fixed scale when no embedded 17Lands file exists. | BLB (unrated) override parses. | S0 merged |

Run order: DR-1, DR-2, DR-11 → DR-3 → DR-4 (G0 re-baseline) → DR-5 → DR-7, DR-8 → DR-9 (S1, S2)
→ DR-10 (S3) → DR-6 at the operator's call.

## 9. Risks

- **R1 Bot-exploit drift.** Learners can learn to read the league bots' predictable lanes.
  - D4 and D7, a league that includes past checkpoints, and the gate against fixed human decks
    all push back.
  - The mixed-pod check catches a policy that only wins at its own table.
  - The leave-one-out baseline also rewards hurting neighbours (it is not action-independent).
    The weight is 1/3, and that incentive matches real pod play. Ablation S2c measures it.
- **R2 Pilot exploit.** The policy can learn decks az-c4 pilots well rather than good decks
  (for example, avoiding cards the search misplays). The `bot`-pilot guard at every gate
  reports the divergence.
- **R3 Credit noise.** At 12 round-robin games per seat (K = 4), R has SE ≈ 0.14 per seat. The in-hand term is noisier still
  and biased toward late-game cards (GIH's known bias). Mitigations: the pod baseline, batch
  normalisation, η starting at 0, and the K ablation.
- **R4 az-c4 is FDN-specific.**
  - The net and the belief are 17Lands FDN.
  - A new set has a no-net pilot (cardval's ~0.50 Spearman tier) and a random-deck belief
    (`lab/cmd/cardval/az.go:74-91`), so it needs a set net or accepts the weaker reward.
  - Bot-drafted decks may sit off the belief's distribution (three-colour piles), which
    weakens the closed-list guess.
- **R5 Search panics.** 4% of sims panicked in the az-c4-s25 run (`az.json`: 1,140,650 of
  27,992,475). If panics cluster on specific cards, rewards are biased. The rollout records the
  per-iteration panic count and the cards in hand at each panic.
- **R6 Cold-start scale.** `ParseRatingsOverride` needs an embedded 17Lands file (in-flight
  code). DR-11 removes that.
- **R7 Train/serve skew.** Avoided by D3. The DR-5 parity fixture guards the export.
- **R8 Non-stationarity** from the league. The KL anchor, a 10-checkpoint window, and the
  gate's fixed opponents mitigate it.

## 10. Open questions for the operator

1. **Pilot.** Was the 48.1% / 25.7% baseline run with `-gauntlet-player bot` (**INFERRED**)?
   Do you accept az-c4 sims=25 as the authority pilot, with `bot` as a guard and sims=100 as
   the final confirmation?
2. **Reward opponents.** 17Lands train-split decks (D4) bring 17Lands data into training for
   FDN. Acceptable? For a new set the fallback is cardval random archetype decks or intra-pod
   play.
3. **Reward shape.** Gauntlet (D4) or the pod round robin (what events actually play)? The spec
   ships the gauntlet with the round robin as an ablation.
4. **Hosting.** Is DR-6 pre-authorised on an S2 gate pass, as the learned cast profiles were?
   Weights by flag from `/mnt/sata` (default) or committed?
5. **League.** May B17, which uses 17Lands ratings, sit in the league? It is the bot humans
   meet today, but it is unavailable for cold sets.
6. **Pod size.** 4 seats only (the default), or also train on 8-seat pods (n = 8 wheels at pick
   9; exact posteriors cost 5,040 assignments)?
7. **Search panics.** Do the 4% az search panics (R5) block S2, or are they tracked only?
8. **S0 bar.** Is the S0 bar (⅔ of the random→17Lands gap) the number the S0 agent is working
   to?

## 11. Operator rulings (2026-10-14)

- **Q2/Q3 reward:** the S2 reward is the **pod round robin** among the league's own drafted decks, so it works on a set with no human data. 17Lands decks (the fdn-limited gauntlet) are evaluation only and never enter rollouts. DR-7's "K games/seat vs train-split decks" becomes "the round robin within the pod". The gate-deck exclusion stays as a guard.
- **Q5 league:** B17 (the 17Lands-rated drafter) may sit in the training league as an FDN opponent. A cold-start claim on BLB or a new set must be re-measured without it.
- **S0 result (seeds 201-224, `bot` pilot, 6144 games each):**
  - 17Lands control: 48.0%.
  - Sim values: hands-bot gih 51.2%, az-c4 hlogit 50.6%, az-c4 deck 48.4%, az-nonet gih 47.1%, az-c4 gih 45.6%.
  - The S0 gate passes.
  - Caveat: the gauntlet pilot is the same `bot` policy that produced the hands-bot values. G0 (DR-4) re-measures with the az pilot.
- **Other questions (defaults):**
  - 4-seat pods only.
  - az panics are tracked, not blocking: DR-7 logs panics per card.
  - Weights load by flag from /mnt/sata.
  - DR-6 hosting waits for the operator.

## 12. Results (2026-10-15)

### S1: behaviour clone

- Top-1 agreement with the S0 teacher: **98.3%** on held-out pods (gate ≥ 85%: passes).
- Gauntlet, S1 against its teacher:

  | Pilot | S1 | S0 teacher |
  |---|---|---|
  | `bot` | 50.1% | 50.5% |
  | az-c4 | 46.3% | 47.1% |

  S1 is within 1 pp of the teacher under both pilots, which is what the non-inferiority bar asks.

### S2: PPO on drafted-deck win rate

- Pilot: the C5 net (az-c5, `docs/design/2026-10-15-az-c5.md`), as §3.0 planned.
- 40 iterations. Stopped on a plateau, against the planned 100.
- Gates, 1024 games each:

  | Checkpoint | s1 | i10 | i20 | i30 | i40 |
  |---|---|---|---|---|---|
  | Score | .450 | .465 | .445 | .467 | .452 |

- Paired check, 32 pods and 4096 games per side (seeds 201-232):

  | Checkpoint | Score | vs S1 |
  |---|---|---|
  | S1 | .439 | n/a |
  | iter-0029 | .458 | +1.9 pp, z 1.76 |
  | iter-0039 | .457 | +1.8 pp, z 1.67 |

  The S2 bar is +3 pp with a lower 95% bound above 0, so **S2 does not pass**.
- League reward rose from .49 to .54 and then stayed flat from about iteration 30.
- KL to S1 was still rising at the stop (.33).
- Per seat: S1 falls from .460 to .410 in the last seat, while S2 stays flat at .45-.47.
- Best checkpoint: `/mnt/sata/gorge-training/draftrl/s2/run/iter-0029/ppo/net.dpkw`. It is as
  strong as iter-0039 and has drifted less from S1.
- Run notes: `/mnt/sata/gorge-training/draftrl/s2/run/NOTES.md`. The run can be resumed with
  `s2loop.sh` and the same environment.

**Diagnosis.** A league-only reward saturates. Once the learner beats its own league, the pod
round robin (§11) has little left to teach it. The open options:

- add 17Lands opponents to the reward (this reverses the §11 ruling for FDN only);
- turn on η in-hand credit (S2b);
- reweight the league;
- go to S3 and recompute values from the S2 games.
