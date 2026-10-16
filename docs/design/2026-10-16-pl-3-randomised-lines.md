# PL-3: randomised play lines

## Question
Does injecting randomness at play time (root noise, early-turn move sampling, a temperature, a
"second-best" rate) (a) cost win rate and (b) measurably diversify the lines the pilot takes? Is a
diverse arm a usable training-data diversifier / router arm, or just a weaker player?

## Method
Reference opponent (my choice; the ticket left it open): `sbtactical`. All arms derive from the az-c5
spec at sims=25 (`B = az:sims=25:autopay:oppnodes:oppfull:morekinds:closedlist:topk=6:gonet=.../c5.dzgw`).

| arm | spec suffix |
|---|---|
| base | (none) |
| ex2 / ex4 / ex8 | `:explore:nonoise:exploreturns=N` (N=2,4,8) |
| exn | `:explore` (Dirichlet noise, default 4 turns) |
| t05 / t10 | `:temp=0.5` / `:temp=1.0` (sample by visits^(1/T) on every searched decision) |
| s05 / s15 | `:second=0.05` / `:second=0.15` (play the 2nd most-visited root move with prob p) |
| base2 | baseline again on seed 79 (null control for the selection step) |

New opt-in dzgorge spec keys (az only; defaults reproduce today byte for byte): `exploreturns=N`
(maps to the existing `SeatConfig.ExploreTurns`), `temp=T`, `second=P`, `aseed=N`. `temp`/`second` live in
`bots/azmcts/choose_line.go` (`chooseMove`, replacing `choose` at the one call in `Search`). Their draws
come from PCG streams seeded by `Options.Seed ^ ChoiceSalt`, so a line is a pure function of the game seed
and `:aseed`; `aseed` salts ONLY the noise/choice stream, never world dealing or the environment bots, so
replays differ only by the arm's own randomness. `second` only swaps to a move that has visits.

New `dzgorge play -lines` (lab/cmd/dzgorge/lines.go, read-only): the game record gets `lines.seq[seat]` (the
non-forced choices of turns <= 5, labelled `t<turn>:<kind>|<option kind>:<label>`) and
`lines.timing[seat]` (INSTANT_LOG-equivalent counters: instant/flash casts, casts in the opponent's turn,
windows with an opposing spell on top and an instant castable, and casts among them).

Measures: (1) paired win rate vs the reference (400 games = 200 pairs x 2 legs, seed 78, Wilson 95% +
paired SE over pairs); (2) diversity: 8 replays (`:aseed=1..8`) of `DIVPAIRS`=40 single-leg games per arm:
divergence-before-turn-4 (first differing arm choice of replay k vs replay 1, turn < 4), entropy in bits of
the 8 early-turn sequences, mean distinct (result, turns) outcomes; (3) timing: arm-seat share of instant
casts in the opponent's turn and response rate to an opposing spell; (4) per-archetype (deck colour pair)
gain, chosen on even-indexed pairs and scored on odd-indexed pairs, with the base2-vs-base run as the null
control (selection bias to subtract) and a bootstrap over odd-pair games.

## Pre-registered rule (verbatim from the ticket)
- A "usable diverse arm" = win rate within -1.0 pp of baseline (paired, upper bound of the loss <
  1.5 pp) AND divergence-before-turn-4 up by >= 25 percentage points over the baseline arm's own
  noise-free divergence (baseline with different seeds should be ~0: report it).
- PURSUE (as a training diversifier / router arm) if at least one arm is usable AND its timing
  profile shifts by >= 5 pp on either timing row, OR it wins >= +3 pp on some archetype (split-half).
- ABORT if no arm is usable, or every usable arm leaves timing unchanged and gives no
  archetype win (randomness only adds noise).
- INCONCLUSIVE otherwise.

Interpretive choices fixed in `pl3_analyze.py` before any data: "upper bound of the loss < 1.5 pp" is read
as paired 95% lower bound on (arm - base) > -1.5 pp; INCONCLUSIVE is returned when no arm is PURSUE and
some arm passes the point-estimate tests (delta >= -1.0 pp, divergence >= +25 pp) but fails only the CI bound.

**Power warning (read before spending the compute).** With 200 pairs the paired SE of the win-rate
difference is about 2-3 pp, so the 95% lower bound is ~5 pp below the point estimate: the literal CI
criterion can only be met by an arm that is better than baseline by several points. Expect ABORT/INCONCLUSIVE
from the win-rate gate alone; the INCONCLUSIVE branch above exists for that. To settle it, `PAIRS` would
have to rise to ~3000+ (SE ~ 0.7 pp) -- the orchestrator's call; the thresholds are not changed here.

## Run commands
Build and run (orchestrator, serially, under ops/scripts/heavy.sh on /mnt/sata):

    go build -o /mnt/sata/gorge-training/playlines/pl-3/dzgorge ./lab/cmd/dzgorge
    ops/scripts/heavy.sh heavy --name pl3 -- env BIN=/mnt/sata/gorge-training/playlines/pl-3/dzgorge \
      OUT=/mnt/sata/gorge-training/playlines/pl-3/full WORKERS=8 lab/scripts/playlines/pl3_run.sh
    /mnt/sata/gorge-training/cardval/venv/bin/python -I lab/scripts/playlines/pl3_analyze.py \
      /mnt/sata/gorge-training/playlines/pl-3/full | tee /mnt/sata/gorge-training/playlines/pl-3/full/verdict.txt

(`pl3_analyze.py` needs only numpy; the venv has no pandas.) Knobs: `SIMS PAIRS DIVPAIRS REPLAYS SEED SEED2
WORKERS ONLY REF ARMS_FILTER TMO`.

Games: per arm 400 win + 8x40 = 320 replay = 720; 9 arms + base2 (400) = 6,880 games.
Wall estimate: the 2-worker smoke (2-deck pool, sims=25, short WU mirrors) ran 4 games in ~2.5 s, i.e. ~0.8
games/s/worker -> ~6 games/s on 8 workers -> ~20 min; real FDN-eval games run longer, budget 30-60 min at
8 workers. Not measured at scale. `pl3_run.sh` is resumable only by `ARMS_FILTER`.

## Results
(to be filled by the orchestrator)

## Verdict
(pending)

## If PURSUE, the next ticket is
Wire the winning arm as a self-play data diversifier (`dzgorge play -a <arm> ...` into the az-c6 loop with
`record` on) and A/B its trained net against the c5 recipe; or, if the archetype gain drove it, a PL-1
router entry for that archetype.

## Smoke (seat; /mnt/sata is read-only in the jail, so outputs are in .ds4/scratch/pl3smoke)
`ONLY` = 2 decks, `SIMS=25 PAIRS=2 DIVPAIRS=2 REPLAYS=2 WORKERS=2`, arms base ex2 t05 s15 base2: 51 s
wall, no errors; `pl3_analyze.py ... --replays 2` printed the full table, null-control and verdict
(ABORT on n=4, meaningless). Two separate runs of `temp=1.0:second=0.15:aseed=3` (3 games) produced identical
`lines`, turns and results. Baseline divergence 0.000.

## Fails without the fix
`chooseMove` replaced by plain `choose` (Temp/Second ignored):

    --- FAIL: TestChooseMoveTempAndSecondArePureInSeed  ... salt changed nothing
    --- FAIL: TestChooseMoveSecond  Second=1 must always play the second best (2): [1 1 1 ...]
    --- FAIL: TestChooseMoveTempSharpens  T=0.5 best share 4000 not above T=1 4000

## Gates
`go test -run TestChooseMove ./bots/azmcts`, `-run 'TestParsePL3Keys|TestParseSBTactical' ./lab/cmd/dzgorge`: ok.
`./internal/archtest` ok, `./internal/codeshape` ok, `lab/cmd/botbench TestConstructedDefaultIsByteIdentical` ok
(unmoved). gofmt clean on changed dirs (cmd/enginebench/walkstats.go is pre-existing unformatted); `gentypes -check` clean.
The diff touches none of effects/rules/state/events/cards/compliance, so the compliance trio was not run.

## Issues
- The win-rate CI gate of the pre-registered rule is underpowered at 400 games (see warning).
- `-lines` calls `EnsurePaymentActions` per priority decision (read-only but extra cost); opt-in only.
- Baseline null control differs by seed (unpaired), so base2's paired delta is a spread, not a paired SE.
