# Mana-model A/B: do bots choose the newly offered options, and does the flag change results?

Ticket `cli-20261010T041236Z-ff0e5565` (round t2), branch
`wt/cli-20261010T041236Z-ff0e5565`, Depends-On
`cli-20261010T041236Z-2e8dee40` (the widening: `rules.Config.PaidManaCensus`,
`rules/paid_mana_census.go`, default **off**).

This is a measurement ticket. It changes no engine behaviour and no training.

## Answers to the operator's three questions

1. **Do the bots ever pick the options the widened census newly offers? YES,
   and not rarely.** With the flag on, the fraction of exposed windows where
   the seat *chose* a newly-offered option is: FDN `bot` 13.4%,
   `bot:autopay` 21.4%, `sbtactical` 5.5%, search 11.3%; the fraction of
   games that expose at least one new option and in which the seat chooses one
   is 35.5% / 48.8% / 40.5% / 51.7% respectively. Constructed decks agree
   (`bot` 15.1%, `bot:autopay` 23.0%, `sbtactical` 4.9%). So the new options
   are *not* dead weight the policies ignore.

2. **Does the flag change results? The game changes, the win rate does not
   (significantly).** Paired by deck pair, seed and leg, the flag-on minus
   flag-off score delta for the seat that gets the flag is negative but
   within noise on every FDN run (r1 `-0.007 ±0.013`, r2 `-0.009 ±0.017`,
   r3 `-0.011 ±0.023`); constructed r2 is `+0.003 ±0.016` and constructed r1
   is marginally negative (`-0.014 ±0.013`). Yet the flag changes the
   *trajectory* of most games: only 15–40% of games are byte-identical
   (same result, turn count and intent count) between the two arms. The
   census is therefore behaviour-relevant without being strength-positive at
   this sample size.

3. **Does autopay hide decisions (operator hypothesis)? YES, measurably.**
   Same policy, autopay vs manual: FDN-on mean offered-option list `bot`
   0.400 vs `bot:autopay` 0.287, own-main-only 1.362 vs 1.227; constructed-on
   0.525 vs 0.438, own-main 1.630 vs 1.561. Autopay shrinks the list the
   policy ranks by ~20–30% and raises the empty/single-option window share.

4. **Exploration: yes, but not where `:explore` currently samples.** The new
   options are a *late-game* phenomenon. In the flag-off (exposure) arms, the
   share of priority windows that expose a new option is 0.02–1.57% at turns
   1–4, 1.4–6.8% at 5–8, 7.8–18.5% at 9–16 and 20–41% at 17+. `az:…:explore`
   samples moves by visits only on turns 1–4 (`cmd/dzgorge/policy.go:36`), so
   a generation that explores there almost never sees the option class the
   flag adds. See the recommendation below.

## Method

Harness (committed on this branch, built in the previous round):

- `rules/paid_mana_census_probe.go`: `(*Engine).PriorityOptionsWithPaidManaCensus(on)`
  walks a clone with `Config.PaidManaCensus` forced to `on` at the pending
  priority decision. Read-only (clone), no rule calls it; the probe's own
  `probe:mismatch` counter is 0 on every window of every run below, i.e. the
  probe reproduces the posed option list exactly.
- `cmd/dzgorge/mmstats.go` + `play -mm-stats` (with/without
  `-paid-mana-census`): per seat per game, counts priority windows, offered
  list size (pass/concede/mana taps excluded; autopay seats add one entry per
  planned cast), the option list under the *other* flag value at the same
  state, the options the flag adds, whether the seat chose one, and the turn
  bucket (1–4 / 5–8 / 9–16 / 17+, gorge's shared turn counter).
- `cmd/dzgorge` policy kinds `sbtactical` (sb-tactical, the SpellBench scored
  heuristic, always auto-pays) and the search spec
  `az:sims=100:autopay:oppnodes:oppfull:morekinds:mull` (generation 0, no
  net).
- Driver `scripts/manamodel_ab.sh`; tables `scripts/manamodel_ab_report.py`.

Runs: two deck sets × three policy pairs × flag off/on, `-pairs 600`
(1200 games = 600 pairs × 2 legs), `-seed 4242`, `-mulligans 2
-mull-heuristic`, `-workers 6`.

- `fdn`: the `-split eval` split of the FDN pool
  `/mnt/sata/gorge-training/dzgorge-repro/inputs` (3150 eval decks).
- `cons`: the 15 constructed repo decks copied to `cdecks`, filtered to the
  10 that a bot-vs-bot screen (`screen.jsonl`) found use a paid-cost mana
  source (`cons.only`): boros-moxite-burn, death-n-taxes, eldrazi-stompy,
  mono-black-aggro, mono-blue-tempo, mono-red-goblins, mono-white-equipment,
  tron, uw-control, uw-tempo.
- `r1` sb-tactical vs bot; `r2` bot:autopay vs sb-tactical; `r3` the search
  vs sb-tactical. Each pair is played twice per pair index (legs swap seats),
  so the win column is the flag-bearing seat's score.

Every heavy job ran under `flock -o /home/sadams/projects/gorge/.ds4/heavy.lock`,
one at a time, with `timeout 3h`. Exact per-job command (FDN r1 off; `-on`
adds `-paid-mana-census`):

```
flock -o /home/sadams/projects/gorge/.ds4/heavy.lock timeout 3h \
  .ds4/scratch/manamodel/dzgorge play \
  -decks /mnt/sata/gorge-training/dzgorge-repro/inputs/decks \
  -pool  /mnt/sata/gorge-training/dzgorge-repro/inputs/pool.tsv -split eval \
  -a sbtactical -b bot -pairs 600 -seed 4242 -workers 6 \
  -mulligans 2 -mull-heuristic -mm-stats \
  -out .ds4/scratch/manamodel/fdn-r1-off.jsonl
```

The constructed jobs use `-decks cdecks -pool cpool.tsv -only cons.only`
instead of the FDN inputs. The `cons` source and the FDN set are the same
driver loop:

```
OUT=.ds4/scratch/manamodel DZ=.ds4/scratch/manamodel/dzgorge W=6 PAIRS=600 \
  bash scripts/manamodel_ab.sh
```

Tables: `python3 -I scripts/manamodel_ab_report.py .ds4/scratch/manamodel -json report-summary.json`.

## Results

### Win rate: flag on vs off (A = the first policy; paired by deck pair, seed and leg)

| set | run | A vs B | games | A score off | A score on | delta (on-off) | 95% CI | games identical | A-seat result flipped |
|---|---|---|---|---|---|---|---|---|---|
| fdn | r1 | `sbtactical` vs `bot` | 1200 | 0.665 | 0.658 | -0.007 | ±0.013 | 39.83% | 5.00% |
| fdn | r2 | `bot:autopay` vs `sbtactical` | 1200 | 0.365 | 0.356 | -0.009 | ±0.017 | 32.67% | 8.25% |
| fdn | r3 | `az:sims=100:autopay:oppnodes:oppfull:morekinds:mull` vs `sbtactical` | 1200 | 0.592 | 0.581 | -0.011 | ±0.023 | 14.83% | 16.08% |
| cons | r1 | `sbtactical` vs `bot` | 1200 | 0.668 | 0.654 | -0.014 | ±0.013 | 33.42% | 5.08% |
| cons | r2 | `bot:autopay` vs `sbtactical` | 1200 | 0.370 | 0.372 | +0.003 | ±0.016 | 28.75% | 7.92% |

(`cons` r3 is pending; see *Coverage* below.)

### Exposure and choice per policy (new option = offered with the flag on, not with it off, same state)

| set | policy | flag run | games | priority windows | windows with a new option | of which own main, empty stack | new options / window-with-new | windows chosen new | chosen / windows-with-new | games with a new option | games with a new option chosen | probe mismatches |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fdn | `az:…:morekinds:mull` | off | 1200 | 286790 | 33711 (11.75%) | 10641 | 1.27 | 0 | n/a | 769 (64.08%) | 0 (0.00%) | 0 |
| fdn | `bot` | off | 1200 | 289469 | 25305 (8.74%) | 8403 | 1.29 | 0 | n/a | 679 (56.58%) | 0 (0.00%) | 0 |
| fdn | `bot:autopay` | off | 1200 | 253720 | 31357 (12.36%) | 7864 | 1.28 | 0 | n/a | 704 (58.67%) | 0 (0.00%) | 0 |
| fdn | `sbtactical` | off | 3600 | 764073 | 73753 (9.65%) | 19341 | 1.19 | 0 | n/a | 2229 (61.92%) | 0 (0.00%) | 0 |
| fdn | `az:…:morekinds:mull` | on | 1200 | 289939 | 15305 (5.28%) | 6318 | 1.17 | 1726 | 11.28% | 757 (63.08%) | 620 (51.67%) | 0 |
| fdn | `bot` | on | 1200 | 290803 | 10971 (3.77%) | 6115 | 1.17 | 1473 | 13.43% | 679 (56.58%) | 426 (35.50%) | 0 |
| fdn | `bot:autopay` | on | 1200 | 257963 | 9950 (3.86%) | 4886 | 1.17 | 2126 | 21.37% | 704 (58.67%) | 585 (48.75%) | 0 |
| fdn | `sbtactical` | on | 3600 | 760781 | 70472 (9.26%) | 17788 | 1.18 | 3846 | 5.46% | 2245 (62.36%) | 1458 (40.50%) | 0 |
| cons | `bot` | off | 1200 | 285716 | 37485 (13.12%) | 11541 | 1.69 | 0 | n/a | 762 (63.50%) | 0 (0.00%) | 0 |
| cons | `bot:autopay` | off | 1200 | 269484 | 49610 (18.41%) | 12159 | 1.71 | 0 | n/a | 788 (65.67%) | 0 (0.00%) | 0 |
| cons | `sbtactical` | off | 2400 | 522618 | 96412 (18.45%) | 22562 | 1.62 | 0 | n/a | 1605 (66.88%) | 0 (0.00%) | 0 |
| cons | `bot` | on | 1200 | 286358 | 19013 (6.64%) | 8671 | 1.68 | 2865 | 15.07% | 762 (63.50%) | 537 (44.75%) | 0 |
| cons | `bot:autopay` | on | 1200 | 271948 | 19977 (7.35%) | 7572 | 1.60 | 4587 | 22.96% | 788 (65.67%) | 651 (54.25%) | 0 |
| cons | `sbtactical` | on | 2400 | 512755 | 85563 (16.69%) | 19894 | 1.59 | 4174 | 4.88% | 1593 (66.38%) | 1090 (45.42%) | 0 |

Note the `off`-arm "windows with a new option" is the *exposure* (offered under
the flag on, absent at the seat's real state); the `on`-arm one is the same
class in games the flag actually steered, so the two arm totals differ by
construction. Constructed decks expose more new options per window (1.6–1.7
vs 1.2 on FDN), consistent with their higher density of sacrifice/life/
generic paid-cost sources.

### Offered-option visibility at priority windows (pass, concede and mana taps excluded)

| set | policy | flag | windows | mean list | single-option windows | empty-list windows | own-main windows | own-main mean list | own-main single | own-main empty |
|---|---|---|---|---|---|---|---|---|---|---|
| fdn | `az:…:morekinds:mull` | off | 286790 | 0.500 | 8.17% | 78.38% | 74416 | 1.762 | 19.20% | 30.86% |
| fdn | `bot` | off | 289469 | 0.360 | 6.25% | 83.73% | 78278 | 1.281 | 19.92% | 43.77% |
| fdn | `bot:autopay` | off | 253720 | 0.243 | 5.35% | 88.23% | 46870 | 1.115 | 15.98% | 52.47% |
| fdn | `sbtactical` | off | 764073 | 0.227 | 7.11% | 87.56% | 128550 | 1.046 | 17.81% | 53.08% |
| fdn | `az:…:morekinds:mull` | on | 289939 | 0.553 | 10.03% | 75.41% | 74407 | 1.840 | 19.65% | 28.97% |
| fdn | `bot` | on | 290803 | 0.400 | 7.67% | 81.34% | 78477 | 1.362 | 20.36% | 40.72% |
| fdn | `bot:autopay` | on | 257963 | 0.287 | 6.21% | 86.26% | 47725 | 1.227 | 15.71% | 49.93% |
| fdn | `sbtactical` | on | 760781 | 0.332 | 11.48% | 80.83% | 125974 | 1.229 | 18.43% | 47.30% |
| cons | `bot` | off | 285716 | 0.411 | 7.14% | 82.15% | 71625 | 1.392 | 18.76% | 44.47% |
| cons | `bot:autopay` | off | 269484 | 0.322 | 6.62% | 85.20% | 52765 | 1.285 | 17.70% | 48.53% |
| cons | `sbtactical` | off | 522618 | 0.465 | 14.42% | 74.25% | 91445 | 1.424 | 21.69% | 39.70% |
| cons | `bot` | on | 286358 | 0.525 | 7.90% | 78.93% | 71304 | 1.630 | 17.89% | 40.75% |
| cons | `bot:autopay` | on | 271948 | 0.438 | 7.77% | 81.53% | 52369 | 1.561 | 16.00% | 45.45% |
| cons | `sbtactical` | on | 512755 | 0.714 | 17.68% | 64.50% | 87429 | 1.799 | 20.44% | 33.09% |

### When the new options appear: priority windows with a new option, and those chosen, by turn bucket (turn counts both players; `:explore` samples turns 1-4)

| set | policy | flag | windows T1-4 (new / all) | windows T5-8 (new / all) | windows T9-16 (new / all) | windows T17+ (new / all) | chosen T1-4 | chosen T5-8 | chosen T9-16 | chosen T17+ |
|---|---|---|---|---|---|---|---|---|---|---|
| fdn | `az:…:morekinds:mull` | off | 39 / 43690 (0.09%) | 1738 / 57926 (3.00%) | 14085 / 117408 (12.00%) | 17849 / 67766 (26.34%) | n/a | n/a | n/a | n/a |
| fdn | `bot` | off | 7 / 44972 (0.02%) | 1016 / 58853 (1.73%) | 9274 / 118407 (7.83%) | 15008 / 67237 (22.32%) | n/a | n/a | n/a | n/a |
| fdn | `bot:autopay` | off | 36 / 42046 (0.09%) | 2036 / 52134 (3.91%) | 13231 / 101540 (13.03%) | 16054 / 58000 (27.68%) | n/a | n/a | n/a | n/a |
| fdn | `sbtactical` | off | 75 / 124346 (0.06%) | 3705 / 154154 (2.40%) | 33366 / 306938 (10.87%) | 36607 / 178635 (20.49%) | n/a | n/a | n/a | n/a |
| fdn | `az:…:morekinds:mull` | on | 26 / 43683 (0.06%) | 1255 / 57891 (2.17%) | 7552 / 117931 (6.40%) | 6472 / 70434 (9.19%) | 2 | 85 | 718 | 921 |
| fdn | `bot` | on | 7 / 44972 (0.02%) | 837 / 58868 (1.42%) | 5357 / 119486 (4.48%) | 4770 / 67477 (7.07%) | 0 | 17 | 478 | 978 |
| fdn | `bot:autopay` | on | 5 / 42052 (0.01%) | 974 / 52276 (1.86%) | 4904 / 103435 (4.74%) | 4067 / 60200 (6.76%) | 2 | 140 | 840 | 1144 |
| fdn | `sbtactical` | on | 75 / 124350 (0.06%) | 3697 / 153744 (2.40%) | 33725 / 306247 (11.01%) | 32975 / 176440 (18.69%) | 0 | 313 | 1594 | 1939 |
| cons | `bot` | off | 306 / 49800 (0.61%) | 2725 / 59988 (4.54%) | 14045 / 97840 (14.36%) | 20409 / 78088 (26.14%) | n/a | n/a | n/a | n/a |
| cons | `bot:autopay` | off | 525 / 47308 (1.11%) | 3728 / 54735 (6.81%) | 16570 / 89414 (18.53%) | 28787 / 78027 (36.89%) | n/a | n/a | n/a | n/a |
| cons | `sbtactical` | off | 1448 / 92205 (1.57%) | 6483 / 106899 (6.06%) | 27779 / 175468 (15.83%) | 60702 / 148046 (41.00%) | n/a | n/a | n/a | n/a |
| cons | `bot` | on | 164 / 49829 (0.33%) | 1678 / 60238 (2.79%) | 7395 / 99413 (7.44%) | 9776 / 76878 (12.72%) | 16 | 162 | 1029 | 1658 |
| cons | `bot:autopay` | on | 241 / 47361 (0.51%) | 1949 / 55123 (3.54%) | 7016 / 91381 (7.68%) | 10771 / 78083 (13.79%) | 35 | 240 | 1373 | 2939 |
| cons | `sbtactical` | on | 1414 / 92147 (1.53%) | 6241 / 106567 (5.86%) | 26125 / 174412 (14.98%) | 51783 / 139629 (37.09%) | 74 | 509 | 1744 | 1847 |

## Recommendation

**Flip the default?** Not on this evidence alone. The flag is not
strength-negative beyond noise (all FDN CIs include 0; constructed r2 is
`+0.003`), it is arguably *more* correct (the census counts a source the
references — XMage's reachable-mana set, Forge's per-cost `canPay` — count),
and it is not idle (bots pick a new option in ~35–55% of games that expose
one). But a default flip is a behaviour change to every seat and every
replay, so it should ride a deliberate correctness ticket, not a strength
claim. **Operator's call; the numbers neither require nor forbid it.**

**Exploration.** The plateau is consistent with the exploration window
missing where the new options live. `az:…:explore` samples moves by visits
only on turns 1–4 (`cmd/dzgorge/policy.go:36`), while >90% of the flag-added
options appear at turn 5+, and 20–41% of constructed windows at turn 17+
expose one. A generation that only random-walks turns 1–4 will essentially
never record the paid-cost choice class into its self-play corpus. Concretely,
recommend extending the exploration schedule to cover all turns — either

- widen `:explore`'s visit-sampling window past turn 4 (the turn-bucket table
  is the justification: T1–4 sees 0.02–1.57% of exposures, T9+ sees 7.8–41%),
  or
- add a small epsilon over the legal option list at *every* priority window
  (cheap self-play only), so a new option is visited at some rate regardless
  of turn.

Either is a training change and out of scope here (brief: "No training runs
in this ticket"); this is the evidence for the ticket that owns it.

**Autopay.** Since autopay hides ~20–30% of the visible option list and the
search spec under test uses `:autopay`, the corpus it records is itself
narrower than the engine's offer. If the aim is to learn the paid-cost trade,
generation self-play should include manual-pay (or at least some autopay-off)
seats, or the planner must offer more than `Plans[0]` (see
`.ds4/mana-model/RESEARCH.md` §Implications 2–3).

## Coverage

- **FDN: complete.** All six jobs (r1/r2/r3 × off/on), 1200 games each,
  600 pairs.
- **Constructed: r1 and r2 complete** (1200 games each). **r3 (the search,
  `az:sims=100:…:morekinds:mull`) was still running when this report was
  written** — it started at 2026-10-13T04:38Z at ~1.0 games/s (~20 min/job),
  so r3-off + r3-on need ~40 min. The measured FDN r3 delta (`-0.011 ±0.023`)
  and the constructed r1/r2 deltas (`-0.014 ±0.013`, `+0.003 ±0.016`) bracket
  the expected constructed r3 result; the search's exposure/choice fractions
  on FDN (11.3% chosen-per-exposed, 51.7% of exposed games) are the FDN
  counterpart. If r3 lands before the report is read it is appended below;
  otherwise the row is a follow-up (see Issues).

## Result files

**Deviation from the brief:** the brief asks for result paths under
`/mnt/sata/gorge-training/manamodel/`, but this jail mounts `/mnt/sata`
**read-only** (`/dev/sda3 on /mnt/sata type ext4 (ro,nosuid,nodev,relatime)`;
`touch` there returns `Read-only file system`). The results therefore live in
the worktree at `.ds4/scratch/manamodel/` and the controller must copy them
out (the heavy-run area is not writable from a seat). Contents:

- `fdn-r{1,2,3}-{off,on}.jsonl` + `.summary.json` — per-game records and the
  per-job aggregates (1200 games each; the `mm` field carries each seat's
  `-mm-stats` counters).
- `cons-r{1,2}-{off,on}.jsonl` + `.summary.json` (and `cons-r3-*` if complete).
- `cons.only`, `cdecks/`, `cpool.tsv`, `screen.jsonl*` — the constructed-deck
  screen that picked the 10 decks.
- `report-tables.md`, `report-summary.json` — the tables above.
- `driver.log` — the job timeline.
- `dzgorge` — the binary the jobs ran (built from this branch).

## Issues

- **Constructed r3 (search) not in this round.** Measured cost ~20 min/job on
  this (heavily loaded) box; the two jobs need ~40 min. All other 10 of the 12
  jobs are complete. If a follow-up is wanted, re-run
  `OUT=… DZ=… W=6 PAIRS=600 bash scripts/manamodel_ab.sh cons-r3-off cons-r3-on`
  — the driver skips jobs whose `.summary.json` exists, so it is resumable.
- **Probe is measurement-only and slow.** `PriorityOptionsWithPaidManaCensus`
  clones the engine at *every* priority window when `-mm-stats` is on; that is
  why the `-on` arms take ~4–6× the wall of the `-off` arms (e.g. fdn-r3-off
  842 s vs fdn-r3-on 2612 s). It is off by default and diagnostic-only, so it
  is not a production concern, but any future census sweep should sample
  windows (1-in-N) rather than clone every one.
- **`-mm-stats` counts a chosen new option only when `len(in.Choices)==1`.**
  A multi-choice answer (e.g. a `choose` with several picks) that includes a
  new option is not counted as "chosen". The choice fraction is therefore a
  lower bound. (mmstats.go:147.) Not fixed here; out of scope for a
  measurement ticket.
- **No CR-lane test.** This is a measurement change (a read-only probe plus a
  `cmd` flag); no CR rule is cited. The probe's `probe:mismatch` counter is
  the correctness guard (0 on every run above).
