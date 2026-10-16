# Bot player training and adoption guidelines

*Moved verbatim from the repository README on 2026-10-16; only paths were updated. Measurements are as they were recorded.*

This section is for anyone building, searching with, training or shipping a
bot policy. It covers:

- the interfaces the engine offers a bot;
- what has been tried and measured so far;
- the gate a policy must pass before it plays real users.

Paths marked **(spellbench-prep)** are on the `spellbench-prep` branch, which
is about to land on `main`. Everything else is on `main` today.

## The decision contract

Everything a bot does is answering a `decision.Decision`:

- The engine lists **every** legal `Option`. The bot replies with a
  `decision.Intent` naming option indices.
- `(*Decision).Validate` rejects anything the engine did not offer, so a bot
  can never make an illegal move. It can only choose badly.
- `decision.Kinds` is a closed set: priority, target, attackers, blockers,
  mulligan, modes, trigger order/optional, choose, commander zone,
  replacement, arrange, starting player. A policy must answer all of them,
  usually by delegating the kinds it does not specialise in to the default
  bot.

A bot implements `seat.Seat`:

```go
Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error)
```

It receives the seat's redacted `view.View`, never the engine. There are two
opt-in extensions:

- `heuristic.BoardSeat` (`bots/heuristic`) takes the cheaper `botpolicy.Board` instead of a projected
  view.
- `seat.PaymentPlanConsumer` asks for the engine's mana-payment plans.

External processes can play a seat in three ways:

- over HTTP, through `server/host/httpapi`: `…/pending`, `…/intent`, `…/view`,
  `…/events`, SSE `/api/stream`;
- **(spellbench-prep)** as a SpellBench v2 agent, in NDJSON over stdio
  (`lab/training/spellbench/v2agent`, `lab/cmd/sbagent`; built only with
  `-tags spellbench_bridge`, see `make test-spellbench-bridge`);
- **(manabrew)** as a ManaBrew protocol client, over SSE + POST
  (`server/host/manabrewhttp`, `server/manabrew`). Opt-in and off by default;
  `gorged -manabrew` mounts it (`-manabrew-addr` for a separate listener),
  and it requires a seat gate (`-humans` or `-vsbot`).

## Engine capabilities for search and reinforcement learning

| Capability | API | Notes |
|---|---|---|
| Step environment | `rules.New(Config)`, `Pending()`, `Submit(Intent)`, `Advance()` | Deterministic: same `Config.Seed` plus same intents gives the same events. |
| Legal actions | `Decision.Options`; `decision.PotentialAction` | The action space *is* the option list. Mana taps are options too, unless auto-pay is on. `PotentialAction` lists the plays that would become castable after floating mana. |
| Intent-level mana | bot auto-pay (`EnableAutoPayMana`); **(spellbench-prep)** `rules.(*Engine).PotentialPaymentPlans` plus `bots/spellbench/payexec` | Plan "cast X / activate Y" and lower the payment witness to taps afterwards, so taps never enter a search tree. Naive manual-tap bots lose about 190–230 Elo. |
| Fork / simulate | `(*Engine).Clone()`, `CloneInto(*Spare)` / `Release()` | Clone at an intent boundary only. Recycle spares on hot paths. |
| Chance-honest worlds | `rules.NewHypothetical`, `(*Engine).CloneHypothetical(seed)`, `SubmitHypothetical`, `AdvanceHypothetical` | A search clone gets a fresh chance stream, so it cannot peek at the real game's future shuffles and draws. |
| Hidden-information sampling | `bots/searchprobe`: `NewCollector` / `Capture`, `Sample`, `Redealer`, known-card tracking | Determinization. `Sample` never sees the real engine, log, seed or hidden hash. **(spellbench-prep)** `azmcts.NewRedeal` wraps it for search. |
| Observation | `view.Project` / `ProjectFor` (`Seat` / `Public` / `Omniscient`); `policynet.EncodeState` / `EncodeOption` | Hidden zones are counts. Omniscient never exposes library order. |
| Terminal / reward | `View.Over`, `View.Winner`; `searchprobe.LeafValue` (heuristic leaf); `policynet` value head | No shaped reward is built in. Win/loss is the ground truth. |
| Replay and audit | `events.Log` (sha256 chain, `Head()`), `replay.Replay` / `ReplayTo` | Any game a policy plays can be re-run and checked event for event. |
| Game runner | `internal/bench`: `PlayGame`, `RunPairs` | Watchdogs, livelock recovery, no clock. Shared by `botbench` and `policytune`. |
| Throughput | measured | About 17.5 ms per bot-vs-bot game on one core (`docs/superpowers/specs/2026-09-06-gorge-learned-policy-research.md`). About 20–23 duel games/s/core after the hotspot work (`docs/superpowers/reports/2026-09-17-hotspot-resume.md`). |

## Engine speed

Measured with `cmd/enginebench` on the rows of draft-zero's
`docs/015-rules-engine-comparison.md` §1 (method in §6): random play is
turns/s, the heuristic bot is games/s, and the rest are per-operation costs
and per-thread search rates. *subset corpus* is an `-tags enginebench_subset`
build that opens only the workload decks' cards rather than the whole registry
— the measuring vehicle for the pointer-free corpus work; every other column
opens the shared imaged registry.

| docs/015 row | a4af596 (docs/015 pin) | old main (e0496f062) | today's main | today, subset corpus | today vs old main |
|---|---|---|---|---|---|
| Random play FDN A, turns/s | 933 | 807 | 3,956 | 3,922 | 4.9x |
| Random play FDN B, turns/s | 1,722 | 1,266 | 3,653 | 3,867 | 2.9x |
| Random play Burn, turns/s | 3,093 | 2,389 | 6,224 | 6,754 | 2.6x |
| Built-in bot FDN A / B, games/s | 39.6 / 68.8 | 38.4 / 50.5 | 148.6 / 150.1 | 148.8 / 163.3 | 3.9x / 3.0x |
| Auto-pay bot FDN A / B, games/s | 31.5 / 45.0 | 30.9 / 32.7 | 83.0 / 92.2 | 104.8 / 101.3 | 2.7x / 2.8x |
| Copy a mid-game state, GC excluded | 14.3 µs | 12.0 µs | 10.7 µs | 7.8 µs | 1.1x |
| Same copies with GC paid | 50.3 µs | 51.3 µs | 53.9 µs | 24.4 µs | 0.95x |
| One search step (copy + submit) | 59.9 µs | 61.4 µs | 31.7 µs | 30.0 µs | 1.9x |
| Pass-priority step only | 39.8 µs | 42.1 µs | 25.9 µs | 26.9 µs | 1.6x |
| Old rollout sampler, rollouts/s | 40.0 | 28.3 | 94.3 | 103.3 | 3.3x |
| azmcts + honest redeal, 100 sims, sims/s | n/a | 336 | 1,099 | 1,294 | 3.3x |
| azmcts + honest redeal, 1,000 sims, sims/s | n/a | 203 | 954 | 1,012 | 4.7x |

## Search

- **PIMC teacher and seat (main).** `bots/searchseat` and
  `bots/searchprobe`, used by `lab/cmd/searchteacher` and `botbench -a
  search`. The method is ensemble determinization:
  - sample K hidden-information worlds;
  - roll every candidate out with the default policy;
  - pick the best mean.

  Candidate 0 is always the bot's own answer and wins ties. The L10 seat
  measured **53.7% [52.2, 55.2] vs a 50.4% control (+3.3pp)** on the held-out
  gate. It costs **664 ms mean / 2.2 s p95 per asked decision**, and its
  coverage falls to 4.8% after turn 13
  (`docs/superpowers/reports/2026-09-24-training-approaches-summary.md`). It
  is merged opt-in and has not been promoted.
- **AlphaZero-style PUCT (spellbench-prep).** `bots/azmcts`, used by
  `botbench -a az` and `-a az-redeal`:
  - The search is single-perspective: tree nodes are only the seat's own
    searched decisions, and `botpolicy.Decide` plays everything else.
  - There are no rollouts. A leaf is scored by a value function on the
    *redacted* view.
  - The environment seam is `azmcts.Env` (`Root`, `Play(Key)`, `Leaf`). The
    entry point is `azmcts.Search(root, worldSource, net, opts)`.
  - Defaults: 100 sims, c_puct 1.5, first-play urgency 0.1. Exploration noise
    (Dirichlet α 0.3, ε 0.25) applies only during generation.
  - Clairvoyant worlds are refused unless `AllowClairvoyant()` is called,
    which only botbench does. An archtest rule keeps `azmcts` out of `host`
    and `cmd/gorged`.
  - Measured with honest redeal worlds at generation 0 (uniform prior,
    heuristic leaf): az25 **64.7% [63.2, 66.2] vs `bot`, +14.7pp, 45 ms mean
    per searched decision**, and az100 +20.5pp at about 250 ms
    (`docs/superpowers/specs/2026-09-28-spellbench-agent-design.md` §12).
  - Caveat: `bot` is both the opponent and the search's environment model in
    those runs, which flatters every search arm.

## Learned policies

`bots/policynet` is a pure-Go learned policy:

- hashed FNV features, an MLP scorer and an optional value head;
- hand-derived gradients, no autodiff;
- binary `.gpol` checkpoints that refuse to load on an encoder-hash mismatch.

The tools around it:

- `lab/cmd/policytrain` trains it with single-threaded, deterministic SGD. The
  same seed and corpus produce a byte-identical checkpoint, which is why it
  has no `-workers` flag. It supports CE, hybrid, PPO and value-only losses.
- `lab/cmd/searchteacher` produces label corpora.
- `lab/cmd/exitloop` runs the teacher → train → bench generations.
- `lab/cmd/policytune` SPSA-fits the small JSON cast profiles in
  `bots/botpolicy/profiles/`.
- `lab/cmd/traindash` is a read-only dashboard.
- `heuristic.NewPolicyNetBot` (`bots/heuristic`) plays a checkpoint. Any kind the net does not answer
  goes to the default bot.

What has been measured (all against `bot`; details in
`docs/superpowers/reports/2026-09-24-training-approaches-summary.md`):

| Approach | Result |
|---|---|
| AR7 heuristic (lethal pressure) | +1.65pp held-out. **Promoted.** |
| Auto-pay mana for the hosted bot | +3.17pp constructed, +3.69pp Commander. **Promoted** (66aa546dd). |
| SPSA cast profile | 50.92% [49.38, 52.47]. Failed the +3pp gate. |
| PIMC search seat (L10) | +3.3pp held-out, slow. Opt-in. |
| Distilled net / ExIt / PPO / VDWM | 44.9–51.5%. Reproduces the bot, no better. |
| Value head | Log loss 0.244 vs 0.460 base rate, but no win-rate gain on its own. |

The durable lesson: **decision-time search beats the bot, and compressing
search into a cheap policy has so far only reproduced the bot.** The intended
direction on `spellbench-prep` follows from that. The network *serves search*:

- a value head first, then an entity encoder;
- visit-count targets (the planned `cmd/azgen`, `policytrain -loss visits`,
  `exitloop -mode az`).

This is staged: clairvoyant AZ loop, then honest sampled worlds, with kill
criteria at each stage (`docs/superpowers/specs/2026-09-27-alphazero-mcts-design.md`,
`docs/superpowers/reports/2026-09-28-spellbench-policy-networks.md`).

## Evaluating a policy

`lab/cmd/botbench` is the only evaluation harness.

- Game *i* is seeded `base + i`, seats trade policies every game, and
  `-pairs all` runs the full deck-pair matrix with per-pair and pooled 95% CIs.
- Always run `-a bot -b bot` on the same seeds as the control.
- `-decision-trace` writes redacted per-decision JSONL for diagnosis.

Sample size decides what a result can show:

| Games | Smallest effect detectable |
|---:|---|
| 95 | about +100 Elo |
| 379 | about +50 Elo |
| 1,792 | about +23 Elo (3.3pp) |

A 200-game run is about ±7pp: a smoke test, not a verdict
(`docs/superpowers/reports/2026-09-28-spellbench-theory-and-references.md` §7, on
spellbench-prep).

**(spellbench-prep)** `lab/scripts/sb-gauntlet.sh <spec>` rates candidates on the
SpellBench scale. It plays them against sb-uniform (the 1000-Elo anchor),
sb-heuristic, `bot` and the promoted champions, and reports anchored
Bradley–Terry Elo with bootstrap CIs. Policies are registered by name in
`lab/training/spellbench/registry`, with the `base+decorator+decorator` spec
grammar; `passguard` is the worked example of a decorator.

## Adoption ladder

A policy moves up one rung at a time. Each rung has a gate.

1. **Bench-only.** Wire it into `lab/cmd/botbench` (`-a <name>`) or, after
   spellbench-prep lands, register it in `lab/training/spellbench/registry`. No
   gate beyond the rules below. Research modes (clairvoyant, oracle, `search`,
   `az`, `policynet`) stay here.
2. **Hosted, opt-in.** Add it to `server/host/bot_policy.go`'s closed vocabulary
   (today: `bot`, `lethal-pressure`, `cast-profile`), so that `POST /api/games`
   can select it with `bot_policy`. Requirements:
   - It must be seat-honest (no clairvoyance, nothing the view does not
     show).
   - It must be deterministic from `(match seed, seat)`.
   - It must fit the hosted latency budget: L10's 664 ms mean / 2.2 s p95 is
     the reference for "slow".
   - A host test must pin it.
   - An operator decision is required.
3. **Default `bot`.** Requirements:
   - It passes the **held-out gate**: seed 1,000,000, 400 games per deck
     pair, seats traded.
   - It gains **≥ +3pp pooled** over the current `bot`.
   - **No pair falls below −5pp.**
   - **Zero new stalls or errors.**
   - The same merge regenerates the `TestHeads` goldens, naming the policy
     change as the cause.
   - Development iterations are gated at +1.96σ against the current best
     before they spend a held-out run.
   - Promotion is the operator's decision
     (`docs/superpowers/plans/2026-09-19-learned-cast-profile.md`,
     "Operator decisions").

**(spellbench-prep)** An agent that enters SpellBench-rated play must also
pass the agent spec's kill criteria (§12.3):

- K1: honest search beats `bot` by ≥ 3pp at ≥ 2,000 games, with the CI clear
  of 0. This has passed.
- K2: ≥ 95% of decisions pass the fidelity gates.
- K3: ≥ 90% candidate agreement with the engine.
- K4: zero timeout forfeits.
- Specialist checkpoints are admitted only if they beat the generic agent in
  their own matchup.

## Rules for training code

- **Determinism.** Seeded `math/rand/v2` only, and no map order in anything
  that reaches a decision or a checkpoint. No `time` in library code: inject a
  `Millis` hook the way `searchseat` and `azmcts` do. Only the allowlisted
  CLIs (`botbench`, `searchteacher`, `exitloop`, …) may read a clock, and
  only for reporting.
- **Pure Go, in-repo.** No cgo, no dependencies, and no PyTorch or other
  external runtime for anything that ships.
- **Honesty.** A policy or value model that will play must read the redacted
  view only. The `mz-oracle` / `mz-opphand` feature sets and clairvoyant
  worlds are diagnostic ceilings. `policynet.WriteCheckpoint` refuses the
  oracle feature sets. The one exception, `WriteOracleCheckpoint`, exists only
  for a search leaf scored inside an already-sampled world.
- **Artifacts.**
  - Corpora, checkpoints and `GOTMPDIR` live under `/mnt/sata/gorge-training`,
    never `/tmp`.
  - Small JSON weight profiles may be committed.
  - `.gpol` neural checkpoints may not be committed without an operator
    decision.
  - Never commit anything derived from Forge script text.
- **Resources.** Run one heavy job at a time, under `systemd-run --user
  --scope -p MemoryMax=…` with `GOMEMLIMIT` set. This is a shared box.
- **Report what you measured.** Record results in
  `docs/superpowers/reports/`: the command, the seeds, the game count, and the
  CI against a same-seed control.

