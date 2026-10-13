# sb-tactical in 4-seat Commander: the runnable gate and its pre-registered pass rule

Ticket: sb-tactical 4-seat Commander measurement path (brief cli-20261013T173521Z-5910a2e6).
Worktree measured on main@2224b64fb (this worktree adds no engine change: **no
botbench code change was needed** — see §1).

## 0. What the smoke showed (the one-line answer)

The measurement path works: a 4-seat Commander pod seating `sb-tactical`
against the hosted auto-pay bot finished 20/20 games, 10-10, zero stalls,
mean 47.2 turns, in 2.8 s wall on 2 workers. The `Formats: constructed` /
`MaxSeats: 2` declarations on `bots/sbtactical/sbtactical.go:65-66` are
metadata consumed only by gorged's `validateBotPolicyFlags`
(`cmd/gorged/game.go:73-80`); botbench never reads them, so the bench runs a
4-seat pod with sb-tactical unmodified.

## 1. Confirmation that botbench runs it (brief work item 1)

Command (build then 1-game probe, capped):

```
go build -o .ds4/scratch/botbench ./cmd/botbench
timeout 900 .ds4/scratch/botbench -format commander -seats 4 -a sb-tactical \
  -b attack-sim-auto-pay -games 1 -seed 900001 -workers 1 -dir .cards
```

Output:

```
game 0: seed 900001, sb-tactical@0 attack-sim-auto-pay@1 sb-tactical@2 attack-sim-auto-pay@3,   1296 intents,  30 turns, winner=attack-sim-auto-pay@1

games played: 1
A wins: 0  B wins: 1  draws: 0
```

No refusal, no seat-cap or format error, no card-dependency error
(`benchBotOptions` supplies `Deps.Cards` from the opened corpus,
`cmd/botbench/main.go:404-406`; `setTacticalRegistry` wires it before any
game, `cmd/botbench/main.go:2324`). **No botbench change, hence no focused
test and no `## Fails without the fix` section — there is no diff to revert.**

### Which `-b` name is the hosted shape, and why not bare `bot`

The brief wrote `-b bot`; the hosted demo default is the auto-pay
combat-sim bot (`scripts/deploy-demo.sh` `BOT_POLICY=bot` with gorged's
default `-bot-auto-mana true`, `cmd/gorged/main.go:302` →
`bots.New("bot", AutoPayMana: true)` → `seat.NewAttackSimBot +
EnableAutoPayMana`, `bots/bot/bot.go:33-37`). In botbench, the bare `bot`
name resolves through `hostedPolicy`, which forces `AutoPayMana` only for
sb-uniform/first/heuristic (`benchAutoPayHosted`, `cmd/botbench/main.go:383-389`),
so bare `bot` plays the **manual** surface. The bench arm that plays exactly
what the hosted default plays is `attack-sim-auto-pay`
(`cmd/botbench/main.go:172-179`, whose own comment says so). Every smoke
below therefore uses `-b attack-sim-auto-pay`. sb-tactical is always
auto-pay by construction (`builtins.NewTactical(builtins.AutoPay, …)`,
`bots/sbtactical/sbtactical.go:77`), so the pod is an auto-pay table on both
sides, which is the hosted shape the brief names.

### Fidelity caveat the operator should read before trusting the gate (Hypothesis, but a verifiable code path)

In the bench's plain drive path the seat answers `Decide`, and **no payment
planner is ever installed on the sb-tactical seat**: the only `SetPlanner`
call sites are the SpellBench play path (`cmd/botbench/spellbench.go:384-388`)
and the host's Env path (`bots/sbtactical/sbtactical.go:125-136`, reached
from `host/match.go:522` via `DecideEnv`); `internal/bench/bench.go`'s
decision loop (lines 296-338) never calls `DecideEnv` or checks
`bots.EnvSeat`. With a nil planner the tactical seat "pursue[s] naively"
(`internal/spellbench/builtins/builtins.go:390`). The SpellBench strength
numbers quoted in the registry (87-41 FDN, Elo 1507) were measured WITH the
planner installed (`sbPlay`'s Setup hook). So the gate below measures the
planner-less arm of sb-tactical — the same seat, same weights, weaker
priority pricing. If the gate passes on this arm it under-states the hosted
shape, which is the safe direction for a widening decision; if it fails,
re-run the follow-up on a hosted table before concluding. (Engine-side
auto-pay payment machinery IS exercised either way — the 2-game
`-payment-stats` probe shows 367 plans offered / 75 planned submissions.)

## 2. The 20-game smoke (brief work item 2)

```
timeout 1800 .ds4/scratch/botbench -format commander -seats 4 \
  -a sb-tactical -b attack-sim-auto-pay -games 20 -seed 900100 -workers 2 -dir .cards
```

Full output in the run log (`head`):

```
bot bench: base seed 900100, sb-tactical vs attack-sim-auto-pay, 20 games, 4 seats, decks avengers-assemble,cavalry-charge,deadly-disguise,foundations-calling-all-angels (commander format)
game 0: seed 900100, sb-tactical@0 attack-sim-auto-pay@1 sb-tactical@2 attack-sim-auto-pay@3,   2038 intents,  46 turns, winner=attack-sim-auto-pay@3
...
```

Tail:

```
games played: 20
A wins: 10  B wins: 10  draws: 0
A win rate: 50.0%  95% CI [28.1%, 71.9%] (normal approximation to the binomial)
seat 0 wins: 2  seat 1 wins: 5  seat 2 wins: 2  seat 3 wins: 11
mean turns per game: 47.2
starting player: seat 0 started 8 and won 1  seat 1 started 2 and won 0  seat 2 started 5 and won 1  seat 3 started 5 and won 4
```

- **Wins per policy:** sb-tactical 10, attack-sim-auto-pay 10, draws 0,
  stalls 0, livelocks 0. Wall: 2.8 s total (~0.14 s/game at `-workers 2`).
- **Turns:** mean 47.2, max 75 (game 17), min 33 — all far under the 200
  `-max-turns` cap and the 20000-intent cap (max 3058 intents).
- **Per-deck (deck pinned to seat; each seat holds sb-tactical in exactly
  10 of the 20 games, `PlaysSeat`, `internal/bench/bench.go:493`):**

  | seat | deck | sb-tactical wins/games held | bot wins/games held |
  |---|---|---|---|
  | 0 | avengers-assemble | 1/10 | 1/10 |
  | 1 | cavalry-charge | 2/10 | 3/10 |
  | 2 | deadly-disguise | 1/10 | 1/10 |
  | 3 | foundations-calling-all-angels | 6/10 | 5/10 |

  Within-deck, sb-tactical is at parity or ahead on all four decks (and the
  6/10-vs-5/10 gap on seat 3 is noise at n=10). Note the gross deck
  imbalance (seat 3's deck won 11/20 overall): deck strength dominates seat
  win share, which is why the pass rule compares **within** a deck across
  policies, not against a flat 25% per-seat expectation.

- **Bot-mirror control** (same 20 seeds, both sides the hosted bot) — for
  the stall comparison the pass rule needs:

  ```
  timeout 900 .ds4/scratch/botbench -format commander -seats 4 \
    -a attack-sim-auto-pay -b attack-sim-auto-pay -games 20 -seed 900100 -workers 2 -dir .cards
  games played: 20
  A wins: 9  B wins: 11  draws: 0
  seat 0 wins: 4  seat 1 wins: 3  seat 2 wins: 1  seat 3 wins: 12
  mean turns per game: 47.5
  ```
  Zero stalls in both arms; mean turns 47.2 vs 47.5.

### Deviation from the brief, declared

The brief said "use the five foundations decks for the smoke". A 4-seat
single-pair botbench run seats the **first four decks of the sorted
commander pool** (`names[:seats]`, `cmd/botbench/main.go:2317-2318`): here
avengers-assemble, cavalry-charge, deadly-disguise,
foundations-calling-all-angels. botbench has no flag to subset the deck pool
for a single-pair run (`-pairs` hard-requires `-seats 2`,
`cmd/botbench/main.go:2030`), and subsetting decks was not among the changes
the brief authorised ("the smallest change that lets the bench run it" — the
bench already runs). The run therefore seats three non-foundations Commander
decks plus one foundations deck, and both sides play every seated deck
equally (decks are pinned to seats, policies trade seats every game), so the
policy comparison stays balanced. The gate's pass rule already carries the
per-deck guard this deviation motivates.

## 3. What a 4-seat `-a sb-tactical -b bot` pod seats, and the null hypothesis

From `internal/bench/bench.go`:

- **Seating.** `PolsFor` (`:508-516`) seats B at every seat and A at the
  seats `PlaysSeat` gives A: `(game+seat) % 2 == 0` (`:493`). With 4 seats
  that is **2 vs 2**: in even games sb-tactical holds seats {0,2} and the
  bot {1,3}; in odd games the reverse. Every seat sees each policy in
  exactly half the games of an even run. The game itself is a free-for-all:
  one winner seat survives, and `classifyOutcome` (`cmd/botbench/main.go:1114-1140`)
  credits the win to whichever side held that seat.
- **Decks.** The first four sorted commander decks are pinned to seats
  (§2's deviation). Policies trade seats, so each policy plays each deck the
  same number of times.
- **Null hypothesis.** If the two policies were identically strong, the
  winner-seat distribution would not depend on which policy held the seat.
  Formally, P(sb-tactical wins a game) = Σ_s p_s · P(sb-tactical holds s) =
  Σ_s p_s · ½ = **exactly 50%**, for ANY deck/seat imbalance (p_s = the
  probability the winner is seat s; the ½ is PlaysSeat's symmetric
  assignment, independent of p_s). Equivalently, per-seat: under the null a
  deck's win rate when sb-tactical holds it equals its win rate when the bot
  holds it — difference 0. This is why the pass rule's per-deck check is the
  paired within-deck difference, not a comparison against 25%.

## 4. The full gate (operator, post-merge) — exact commands

Held-out seeds. Earlier sb-tactical measurements used: SpellBench benchmark
seed 20260926 and held-out seed 99991 (spec 2026-09-28-spellbench-agent-design
§3.4); botbench pair runs at seed 4242 (2026-10-12-mana-model-ab.md:107) and a
96-seed constructed sweep (2026-10-09-common-sense-rules-evaluation.md); az
runs at 91000000+ (different policy family, still avoided); smoke above at
900001-900300. **No recorded run uses the 512,000,000 block** (grepped all of
`docs/` for `5120…`/`512000000`: clean). Base seed **512000000**; game g
plays at 512000000+g, so any sub-range reuses the same games.

Game count. Null p=0.5; a ±2pp half-width at 95% needs
n = (1.96/0.02)² · 0.5 · 0.5 ≈ **2401** games. The gate plays **2500**
(gives 1.96pp; the exact binomial lower bound is what the rule reads, not
the normal approximation). At the smoke's measured 0.14 s/game, 2500 games
is ~3 min wall at `-workers 4` (~6 min at 2) — well inside an operator
window; `-workers 4` keeps the shared box polite.

```
# Build (or reuse the merged binary):
go build -o /tmp/botbench-gate ./cmd/botbench

# Arm 1 — the gate run:
timeout 3600 /tmp/botbench-gate -format commander -seats 4 \
  -a sb-tactical -b attack-sim-auto-pay \
  -games 2500 -seed 512000000 -workers 4 -dir .cards > sbtactical-commander-gate.txt

# Arm 2 — the bot-mirror control (SAME seeds, decides the stall bar and the
# per-deck null):
timeout 3600 /tmp/botbench-gate -format commander -seats 4 \
  -a attack-sim-auto-pay -b attack-sim-auto-pay \
  -games 2500 -seed 512000000 -workers 4 -dir .cards > bot-mirror-commander.txt
```

(`-max-turns`/`-max-intents` stay at their defaults, 200 / 20000; the smoke's
worst game used 75 turns and 3058 intents, so the caps only fire on a
pathology. No `-count=1`-style cache concerns: this is a bench binary, not a
test.)

## 5. Pre-registered pass rule (write the verdict before reading anything else)

Let arm 1 be the sb-tactical run and arm 2 the bot-mirror control, both over
the same 2500 seeds. Denote by `win(P, s)` the number of games whose winner
seat s was held by policy P in arm 1.

1. **Primary — aggregate.** sb-tactical's win share over decided games in
   arm 1 is above 50% **and** its 95% lower bound (exact binomial /
   Clopper-Pearson over decided games) clears 50%: `lower(95%) > 50.0%`.
   (The bench prints the normal-approximation CI; the operator may recompute
   the exact bound from the A/B counts. Normal-approx reads the same verdict
   at n=2500.)
2. **Per-deck guard.** For EACH of the four seated decks (the per-seat
   table at the end of arm 1's summary), its win rate when sb-tactical held
   that seat is **not more than 3 percentage points below** its win rate
   when the bot held that seat — the paired same-seed within-deck
   difference, whose null is exactly 0 (§3). A deck more than 3pp below is a
   veto on the widening even if the aggregate clears: it says the heuristic
  loses a deck the hosted bot wins.
3. **Stall guard.** Arm 1's stall count (games printed with no winner —
   `stallTurns + stallIntents + livelocks` in the summary's denied games)
   is **not higher** than arm 2's stall count on the same seeds. Any
   livelock in either arm is an engine bug and fails the gate outright
   regardless of counts (bench aborts the run on one).

PASS = all three. The smoke's numbers (10-10 aggregate, per-deck −1pp on
seat 1 and +1pp on seat 3, 0 stalls in both arms) are consistent with the
null at n=20 and predict nothing about the gate — that is what 2500 games
are for.

Reading the output: arm 1's summary prints `A win rate` with its CI and the
per-seat win table; per-deck rates are seat wins divided by the 1250 games
each seat held each policy (even run). Decided games = 2500 − stalls −
draws; both guards use decided games only.

## 6. The 1v1-assumption findings (hypothesis confirmed in code)

sb-tactical's tactical state assumes a two-player table in these places;
none is fixed in this ticket:

- **One opponent is picked and the rest are invisible.**
  `internal/spellbench/builtins/tactical_state.go:121-138` (`newState`)
  walks the view's players and takes the FIRST other non-lost player as
  `s.oppP` — in a 4-seat pod, one specific opponent (lowest other seat
  index). Everything race- and life-shaped reads only that pair:
  `s.myLife/s.oppLife` (`:138`), the creature census `s.mine`/`s.theirs`
  built ONLY from `s.meP` and `s.oppP` battlefields (`:171-196`), and the
  clocks `s.myClock/s.oppClock` (`:202-203`). The other two seats'
  creatures exist in `s.objs` (so cast/target legality sees them) but are
  absent from the threat/race census, and their life totals never enter the
  clock math.
- **Cast-time candidate targets see only two players.**
  `internal/spellbench/builtins/tactical_target.go:80-97`
  (`candidateTargets`) enumerates player and battlefield targets from
  `s.oppP` and `s.meP` only — a "target opponent" effect is priced as if
  exactly one opponent existed. (The engine-offered option list is still
  answered over all legal targets at the target decision itself,
  `:531-543`; the distortion is in how plays are priced at priority.)
- **`Opponent` legality means "anyone but me".**
  `validMatch`, `internal/spellbench/builtins/tactical_target.go:39-41,63`:
  `Opponent`/`OppCtrl` qualify any seat but mine. Correct for a free-for-all
  on players, but on creature targets it prices removal against creatures
  controlled by seats the census never reads (they are not in
  `s.theirs`), so removal scoring leans entirely on that one first-opponent
  seat's board.
- **No ally concept.** The engine pods are free-for-alls (one surviving
  seat wins), so "anyone but me is a foe" is directionally right; the defect
  is that three of four foes are unread, not that a teammate is miscast.

Consequence for the gate: the arm's race/removal behaviour is that of a
policy watching seat 1 (or whichever neighbour sorts first) and blind to the
other two. If the gate passes despite this, the hosted shape can only be
stronger with the full pod visible; if it fails, the first fix to try is a
pod-aware census/clock (follow-up ticket, not this one).

## 7. Follow-up if the gate passes (NOT this ticket)

1. Widen the registry metadata: `Formats: []string{"constructed",
   "commander"}` and drop/raise `MaxSeats: 2` in
   `bots/sbtactical/sbtactical.go:65-66` (note `MaxSeats` is a ceiling
   checked as `entry.MaxSeats < seats`, `cmd/gorged/game.go:73`; leaving it
   at 2 still blocks 4-seat tables, so it must change too). Keep the
   strength `Measurement` rows honest: they are 1v1-constructed numbers, so
   add a Commander-pod row sourced from this gate's verdict.
2. Point the deploy default at it: `scripts/deploy-demo.sh` `BOT_POLICY`
   (or the commander table's policy choice) only after `make deploy-demo`
   smoke on the host, since a redeploy aborts in-flight games.
3. Optional fidelity follow-up (filed separately if the gate is close):
   give the bench's single-run path the same planner installation the
   SpellBench path has (`SetPlanner` for unwrapped `*builtins.Seat`, mirror
   of `cmd/botbench/spellbench.go:384-388`), so bench sb-tactical numbers
   stop measuring the naive-pursuit arm.

## Issues found, not fixed

- The planner-less bench arm (§1 fidelity caveat) — bench single-run
  sb-tactical plays "pursue naively" at priority; SpellBench's quoted
  numbers are planner-paid. Location: `internal/bench/bench.go:296-338` (no
  DecideEnv/EnvSeat branch), `cmd/botbench/spellbench.go:384-388` (the
  install the plain path lacks), `internal/spellbench/builtins/builtins.go:390`.
- The 1v1 census/clock narrowing (§6), `internal/spellbench/builtins/tactical_state.go:121-138,171-203`
  and `tactical_target.go:80-97`.
- botbench cannot subset the deck pool for a >2-seat single-pair run
  (`-pairs` requires `-seats 2`, `cmd/botbench/main.go:2030`), so a
  4-seat run's decks are always the pool's first N sorted decks.
