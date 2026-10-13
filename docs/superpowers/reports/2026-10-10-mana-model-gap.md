# Mana-model gap: how often the source census withholds a payable cast

Measured 2026-10-10 on `wt/mm-quantify` (base main 34867835c). Tool:
`cmd/manamodelstats` + `rules/zz_manamodel.go`, both `-tags manamodel`
(no default-build code, no offer behaviour change). Brief:
`.ds4/mana-model/A-quantify.md`.

## What is measured

At every posed priority decision of a seat, three per-source censuses are
compared (each source pays through at most one of its alternatives):

- **W**, the shared free-cost census `windowManaUnits` (feeds
  `AvailableMana`, the attack-tax budget, unless costs);
- **P**, the auto-pay planner's census (`paymentPlanManaUnits` expanded by
  `PaymentPlanQueryAlternatives`, normal + last-resort tiers): the only
  sources an offered `PaymentAction` plan can spend;
- **F**, the full model: every mana ability of every reachable source whose
  tap part is available, at printed production, the rest of its activation
  cost ignored (amounts via the engine's own `castWindowAmount`).

**L1** (every window, both seats auto-pay): each ordinary cast the offer
walk would make with unbounded mana is priced from the floating pool plus
W, P and F. A cast is L1-positive for W (P) when F pays it and W (P) does
not. It is attributed to the first class in the order Sac, Life, ManaFee,
SubCounter, TapOther, Discard, Exile, AddCounter, OtherCost, Free:Restricted,
Free:InstantSpeed, Free:Amount, Free:Choice, Free:Tier whose excluded
alternatives *alone* pay it ("combo" when no single class does), suffixed
`:token`/`:card`/`:mixed` by source. The Free:* classes are free-cost
abilities a census still excludes: RestrictValid$, InstantSpeed$, a
non-literal Amount$, a choice-shaped Produced$, or a plain production the
census still drops (the planner's tier gate, a rider, a granted ability).

**L2** (here every L1-positive window, at most 4 casts per window): on engine
clones, real mana activations (`activate` options, every sub-decision
branched) are searched until the engine's own floating-pool offer walk
offers the cast. The F witness's sources are tried first (no search), then
a free search with node and depth caps; a cut search is *unknown*.

Option lists: per priority window, the list the seat's policy ranks, minus
pass, concede and mana taps (auto-pay: `seat.Bot.paymentIntent`'s candidate
list, i.e. legacy options without `activate` plus plan-only casts; manual:
the raw options). Manual runs use the same pairs and seeds, both seats manual.

## Verification before measuring (brief hypothesis)

The brief's hypothesis that auto-pay misses Lotus Petal like the manual walk
does is **false**: with Lotus Petal or Black Lotus alone and Lightning Bolt in
hand, the posed decision carries a payment plan (the planner admits a
self-sacrifice as a last-resort source); so do City of Brass and a Mountain.
Gemstone Mine and Lion's Eye Diamond get no plan; Chromatic Sphere, Mana
Vault, Springleaf Drum, Wasteland and Treasure Vault correctly get none. A
Treasure-like token (Eldrazi Scion, Treasure) is in P too: the W-only
`Sac:token` rows below are exactly that. So the operator's "Treasure not
counted" is the **W** census (the view's `AvailableMana`, the attack and
unless windows), not the auto-pay plan.

## Results

Games: 2000 FDN (1000 random pool pairs x 2, `pool.tsv`), 2100 constructed
(15 sixty-card repo decks, 105 pairs x 20), 2100 commander (15 repo
Commander decks, 105 pairs x 20, 40 life). Seed 7700000, turn cap 100. No
stalls in any run.

#### Headline

| family | games | priority windows | L1 windows (W) | L1 windows (P) | L2-sampled windows | L2-confirmed windows (W) | L2-confirmed windows (P) | games with >=1 confirmed (W) | games with >=1 confirmed (P) |
|---|---|---|---|---|---|---|---|---|---|
| fdn | 2000 | 810535 | 7765 (1.0%) | 512 (0.1%) | 7915 | 7507 (0.9%) | 263 (0.0%) | 882 (44.1%) | 38 (1.9%) |
| constructed | 2100 | 845506 | 7210 (0.9%) | 1838 (0.2%) | 7210 | 6232 (0.7%) | 804 (0.1%) | 863 (41.1%) | 233 (11.1%) |
| commander | 2100 | 1006388 | 42813 (4.3%) | 19510 (1.9%) | 44369 | 34893 (3.5%) | 11038 (1.1%) | 1787 (85.1%) | 1108 (52.8%) |

#### Casts

| family | gap | L1 casts | L2 real | L2 false (proxy over-count) | L2 unknown | precision real/(real+false) |
|---|---|---|---|---|---|---|
| fdn | W | 11093 | 10629 | 393 | 27 | 96.4% |
| fdn | P | 681 | 296 | 364 | 20 | 44.8% |
| constructed | W | 12925 | 10477 | 1940 | 85 | 84.4% |
| constructed | P | 3403 | 1203 | 1935 | 80 | 38.3% |
| commander | W | 76986 | 59168 | 14008 | 1105 | 80.9% |
| commander | P | 31568 | 15670 | 13629 | 1163 | 53.5% |

#### P gap by class

| family | class | L1 casts | L2 real | L2 false | L2 unknown |
|---|---|---|---|---|---|
| fdn | Free:Restricted:card | 471 | 86 | 364 | 20 |
| fdn | Free:Tier:card | 176 | 176 | 0 | 0 |
| fdn | Free:Tier:token | 34 | 34 | 0 | 0 |
| constructed | Sac:card | 2300 | 348 | 1690 | 79 |
| constructed | Free:Restricted:card | 1103 | 855 | 245 | 1 |
| commander | ManaFee:card | 12004 | 5614 | 5596 | 282 |
| commander | Free:Restricted:card | 5852 | 2238 | 3138 | 377 |
| commander | Free:Tier:card | 3774 | 3404 | 146 | 148 |
| commander | Sac:card | 3033 | 438 | 2268 | 204 |
| commander | TapOther:card | 2660 | 634 | 1969 | 12 |
| commander | Free:Choice:card | 951 | 776 | 36 | 26 |
| commander | combo | 941 | 415 | 391 | 100 |
| commander | Free:Amount:card | 938 | 908 | 23 | 0 |
| commander | Exile:card | 474 | 414 | 56 | 0 |
| commander | Free:Choice:mixed | 437 | 363 | 0 | 0 |
| commander | SubCounter:card | 393 | 381 | 2 | 0 |
| commander | Free:Choice:token | 39 | 39 | 0 | 0 |
| commander | Free:Tier:mixed | 38 | 30 | 0 | 0 |
| commander | Life:card | 29 | 14 | 1 | 14 |
| commander | Free:Restricted:mixed | 3 | 0 | 3 | 0 |
| commander | Sac:token | 1 | 1 | 0 | 0 |
| commander | Free:Tier:token | 1 | 1 | 0 | 0 |

#### W gap by class

| family | class | L1 casts | L2 real | L2 false | L2 unknown |
|---|---|---|---|---|---|
| fdn | Free:Choice:card | 10338 | 10260 | 29 | 7 |
| fdn | Free:Restricted:card | 451 | 99 | 332 | 20 |
| fdn | Sac:token | 95 | 95 | 0 | 0 |
| fdn | combo | 87 | 53 | 32 | 0 |
| fdn | Free:Tier:card | 81 | 81 | 0 | 0 |
| fdn | Free:Amount:card | 41 | 41 | 0 | 0 |
| constructed | Free:Choice:card | 5057 | 4934 | 0 | 0 |
| constructed | Free:Amount:card | 3657 | 3603 | 0 | 2 |
| constructed | Sac:card | 2855 | 1024 | 1560 | 41 |
| constructed | Free:Restricted:card | 1099 | 851 | 245 | 1 |
| constructed | combo | 191 | 4 | 130 | 41 |
| constructed | Sac:token | 66 | 61 | 5 | 0 |
| commander | Free:Choice:card | 32619 | 30770 | 480 | 60 |
| commander | ManaFee:card | 12453 | 6919 | 4774 | 223 |
| commander | Free:Restricted:card | 6718 | 3398 | 2904 | 324 |
| commander | combo | 6199 | 3658 | 1837 | 326 |
| commander | Free:Tier:card | 4279 | 4201 | 3 | 3 |
| commander | Sac:card | 3255 | 1031 | 2003 | 129 |
| commander | TapOther:card | 3241 | 1306 | 1879 | 7 |
| commander | Free:Amount:card | 3211 | 3160 | 29 | 0 |
| commander | Life:card | 2322 | 2226 | 21 | 24 |
| commander | Sac:token | 1093 | 1039 | 18 | 3 |
| commander | Free:Choice:mixed | 630 | 572 | 0 | 0 |
| commander | Exile:card | 379 | 332 | 44 | 0 |
| commander | SubCounter:card | 375 | 367 | 0 | 1 |
| commander | Sac:mixed | 130 | 112 | 13 | 5 |
| commander | Free:Choice:token | 76 | 74 | 0 | 0 |
| commander | Free:Tier:mixed | 3 | 3 | 0 | 0 |
| commander | Free:Restricted:mixed | 3 | 0 | 3 | 0 |

#### What the auto-pay seat did in a P-confirmed window

| family | L2-confirmed (P) windows | seat answered: payment (a plan) | activate (manual fallback) | cast (pool) | pass | other |
|---|---|---|---|---|---|---|
| fdn | 263 | 24 | 105 | 1 | 114 | 19 |
| constructed | 804 | 21 | 769 | 3 | 1 | 10 |
| commander | 11038 | 781 | 7525 | 77 | 2153 | 502 |

#### Option lists (casts and abilities)

| family | mode | scope | windows | 0 options | exactly 1 | 2+ | mean list length |
|---|---|---|---|---|---|---|---|
| fdn | auto | all priority | 810535 | 90.3% | 5.5% | 4.3% | 0.17 |
| fdn | auto | own main, empty stack | 150471 | 64.7% | 15.8% | 19.4% | 0.69 |
| fdn | manual | all priority | 927263 | 89.6% | 5.1% | 5.3% | 0.20 |
| fdn | manual | own main, empty stack | 252350 | 66.0% | 15.3% | 18.7% | 0.68 |
| constructed | auto | all priority | 845506 | 87.7% | 6.3% | 6.0% | 0.24 |
| constructed | auto | own main, empty stack | 161941 | 59.3% | 17.0% | 23.7% | 0.92 |
| constructed | manual | all priority | 894968 | 86.8% | 6.3% | 6.9% | 0.28 |
| constructed | manual | own main, empty stack | 221391 | 61.3% | 16.2% | 22.5% | 0.90 |
| commander | auto | all priority | 1006388 | 86.0% | 7.0% | 7.0% | 0.30 |
| commander | auto | own main, empty stack | 203619 | 53.3% | 17.1% | 29.7% | 1.17 |
| commander | manual | all priority | 1162410 | 84.3% | 7.3% | 8.4% | 0.36 |
| commander | manual | own main, empty stack | 303751 | 55.5% | 16.8% | 27.8% | 1.15 |

#### Option lists including land plays

| family | mode | scope | exactly 1 | mean (incl. land plays) |
|---|---|---|---|---|
| fdn | auto | all priority | 5.4% | 0.25 |
| fdn | auto | own main, empty stack | 15.3% | 1.15 |
| fdn | manual | all priority | 6.4% | 0.37 |
| fdn | manual | own main, empty stack | 20.1% | 1.31 |
| constructed | auto | all priority | 6.4% | 0.31 |
| constructed | auto | own main, empty stack | 17.7% | 1.27 |
| constructed | manual | all priority | 7.1% | 0.39 |
| constructed | manual | own main, empty stack | 19.3% | 1.36 |
| commander | auto | all priority | 7.0% | 0.39 |
| commander | auto | own main, empty stack | 16.8% | 1.60 |
| commander | manual | all priority | 8.0% | 0.52 |
| commander | manual | own main, empty stack | 19.3% | 1.76 |

#### Per deck

| family | deck | priority windows | L1 windows (P) | L2-confirmed windows (P) | L2-confirmed windows (W) |
|---|---|---|---|---|---|
| constructed | eldrazi-stompy | 47406 | 679 | 579 | 621 |
| constructed | mono-red-goblins | 42161 | 195 | 195 | 195 |
| constructed | death-n-taxes | 62266 | 65 | 30 | 30 |
| constructed | tron | 66350 | 575 | 0 | 2450 |
| constructed | boros-moxite-burn | 57580 | 0 | 0 | 2059 |
| constructed | uw-control | 92571 | 0 | 0 | 656 |
| constructed | the-epic-storm | 50315 | 324 | 0 | 221 |
| commander | ulalek-eldrazi | 69977 | 2098 | 1711 | 4173 |
| commander | vivi-ornitier-cedh | 56768 | 2742 | 1458 | 2597 |
| commander | valgavoth-endless-punishment | 57394 | 2335 | 1322 | 2777 |
| commander | avengers-assemble | 69148 | 2129 | 1248 | 4506 |
| commander | science-pip | 79570 | 1931 | 1215 | 5428 |
| commander | deadly-disguise | 72475 | 1808 | 1185 | 3247 |
| commander | foundations-calling-all-angels | 64956 | 1877 | 934 | 929 |
| commander | cavalry-charge | 64570 | 916 | 756 | 3552 |
| commander | rakdos-muscle-scam-exe | 68407 | 2279 | 596 | 2502 |
| commander | pro-shaper | 71738 | 375 | 298 | 1198 |
| commander | foundations-keen-engineering | 74455 | 605 | 140 | 160 |
| commander | hearthhull-worldseed-landfall | 75746 | 353 | 137 | 3493 |
| commander | foundations-reign-of-dragons | 56641 | 20 | 17 | 7 |
| commander | foundations-wretched-ranks | 72002 | 17 | 11 | 6 |
| commander | foundations-tramplesaurus-rex | 52541 | 25 | 10 | 318 |

#### Run health

| family | stalls | big-board windows skipped | windows with an unknown L1 search | windows where P pays but no plan is offered | L2 false with a trigger on the stack | L2 panics | L2 real on the guided witness |
|---|---|---|---|---|---|---|---|
| fdn | 0 | 0 | 0 | 1588 | 0 | 0 | 10787 |
| constructed | 0 | 0 | 0 | 402 | 528 | 0 | 10445 |
| commander | 0 | 910 | 0 | 11760 | 1849 | 0 | 56898 |


`L2 false` rows are the proxy's over-count (the activation cost F ignores is
not payable, or the mana is restricted away from that spell). "Run health"
also counts the commander windows skipped above 600 objects.

## Reading

1. **The bot-visible gap (P) is small outside Commander.** L2-confirmed P
   windows: FDN 0.03% of priority windows (1.9% of games), constructed 0.1%
   (11% of games), commander 1.1% (53% of games). In constructed it is almost all two
   decks: eldrazi-stompy (Eldrazi Temple, `Free:Restricted`) and
   mono-red-goblins (Skirk Prospector, `Sac` of another permanent); death-n-taxes (Cavern of Souls) adds 30.
2. **In Commander the P gap is broad**: real P-withheld casts are led by
   ManaFee (5,614: filter lands, Signet-style `{1},{T}` rocks), Free:Tier
   (3,404: plain sources the planner still drops, e.g. Exotic Orchard,
   Thran Dynamo, Talismans), Free:Restricted (2,238), Free:Amount (908, Urza
   lands / Ugin's Labyrinth), Free:Choice (776+363+39), TapOther (634, Relic
   of Legends), Exile (414, Simian Spirit Guide), SubCounter (381).
3. **The auto-pay seat mostly recovers by hand.** In P-confirmed windows the
   seat's answer was a manual mana tap (the adapter's `wantsManual`
   fallback) in 68% (commander), 96% (constructed), 40% (FDN); it passed in
   20% / 0.1% / 43%, and took a different planned cast in 7% / 3% / 9%.
   So the cost is not "never castable" but: a pass, or a lesser planned
   spell, in roughly a third of commander and half of FDN confirmed windows.
4. **The W gap is ~3-10x the P gap** and almost all `Free:Choice` (dual,
   pain and tri lands whose choice-shaped production W prices as nothing),
   `Free:Amount` (Tron lands) and self-sacrifice. It does not hide casts from
   auto-pay (P includes those) but it is what the view's available-mana line
   and the attack/unless payment windows see.
5. **The proxy is noisy for P**: precision 38-54%. Most false positives are
   LED (Sac+Discard: activating it at priority discards the hand, and the
   engine's cast window does not offer InstantSpeed$ abilities, so it cannot
   pay a card in hand in gorge -- rules-wise it can, during CR 601.2g),
   Signets/filter lands with no other mana to pay the fee, Relic of Legends
   with no legendary creature untapped, and restricted mana (Cavern of Souls,
   Eldrazi Temple, Giada) offered to the wrong spell. A widening ticket
   should model the activation cost (reachable-set fixpoint), not the
   production alone.
6. **Option lists: auto-pay does not shrink the decision set.** Own-main
   empty-stack windows with exactly one cast/ability option: FDN 15.8% auto
   vs 15.3% manual, constructed 17.0% vs 16.2%, commander 17.1% vs 16.8%;
   mean list length 0.69 vs 0.68, 0.92 vs 0.90, 1.17 vs 1.15. Over all
   priority windows 84-90% have no cast/ability option in either mode. The
   operator's hypothesis that auto-pay bots see far fewer decisions is not
   supported (manual sees more *windows* -- each tap re-poses priority --
   not longer lists). Including land plays, manual is slightly longer.
7. **A planner limit outside the mana model is as large as the P gap**:
   windows where P's census pays a cast but no plan (and no pool cast) is
   offered: FDN 1,588, constructed 402, commander 11,760 (examples: Wildwood
   Scourge, Luminous Rebuke, Dispatch, Melded Moxite, Arcane Denial). Not
   classified here (X costs, hybrid, cost-modifier statics, search limits are
   the likely causes -- HYPOTHESIS).

## Suggested order for the widening ticket (HYPOTHESIS, from the counts)

1. Commander-weighted: ManaFee (`{N},{T}` rocks/filter lands) and the
   Free:Tier/Free:Choice/Free:Amount sources the planner drops (cheap: they
   are free-cost; the census already prices most of them in W or the cast
   window).
2. Restricted mana (Eldrazi Temple, Cavern, Giada, Plaza): needs per-spell
   restriction matching in the planner; high false-positive share, so model
   it properly rather than counting it.
3. Sac of another permanent (Skirk Prospector, Evendo Brushrazer), TapOther,
   SubCounter, Exile-from-hand (Spirit Guides): small but deck-defining.
4. The W census (view/attack/unless): Free:Choice lands and Treasure-class
   self-sacrifice -- the operator's visible symptom.

## Limitations

- L2 pre-activates mana at priority, then asks whether the cast is offered.
  A trigger from an activation (sacrifice triggers) closes sorcery timing
  (counted: "L2 false with a trigger on the stack"), and LED-style sources
  that discard the hand fail, though both are payable inside a CR 601.2g
  window. L2 is "payable by this engine", not "payable by the rules".
- L2 tries at most 4 casts per window (capped 38 / 265 / 1,862 times), so
  confirmed windows are a lower bound; unknowns are 2-4% of L1 casts.
- Only ordinary casts (no mode, no alternative cost) are measured; activated
  abilities, flashback/escape modes and mana from spells (rituals) are not.
- Hybrid/phyrexian are priced by `resolveManaWith`; snow costs are priced
  with no snow mana (under-count).
- Commander windows above 600 objects are skipped (910 windows).
- Purity: `-verify` replayed every FDN auto game (2000) and 105 constructed
  auto games hook-free: 2105 of 2105 games had the identical chain head (no `verify:MISMATCH`).

## Commands

```sh
cd .worktrees/mm-quantify
go build -tags manamodel -o /mnt/sata/gorge-training/manamodel/manamodelstats ./cmd/manamodelstats
# each run under: flock -o .ds4/heavy.lock systemd-run --user --scope -q -p MemoryMax=16G -p CPUQuota=600% \
#   env GOMAXPROCS=6 GOMEMLIMIT=12GiB GOTMPDIR=/mnt/sata/gorge-training/manamodel/gotmp
manamodelstats -dir .cards -workers 6 -seed 7700000 -l2-nodes 1500 -family fdn -mode auto -games 2000 -verify -out fdn-auto.json
manamodelstats ... -family fdn -mode manual -games 2000 -out fdn-manual.json
manamodelstats ... -family constructed -mode auto|manual -games 2000 -out constructed-<mode>.json
manamodelstats ... -family commander -mode auto|manual -games 2000 -out commander-<mode>.json
manamodelstats ... -family constructed -mode auto -games 105 -verify -out constructed-auto-verify.json
python3 scripts/manamodel_report.py /mnt/sata/gorge-training/manamodel/runs
# focused test
go test -tags manamodel -run TestManaModel ./rules
```

Results: `/mnt/sata/gorge-training/manamodel/runs/` (`*.json` with examples,
`*.txt` raw counters, `run_all.sh`, `run_all.log`).

## Landing note (ported to main 9ea243625, 2026-10-12)

This report and the tool were authored by the operator on `wt/mm-quantify`
(base main 34867835c) and are landed here through ticket
`cli-20261010T041236Z-d8868a62`. The tool is ported unchanged. On main
9ea243625 all of these pass: `go build ./...` (the default build carries no
diagnostic code), `go build -tags manamodel ./...`, the focused
`go test -tags manamodel -run TestManaModel ./rules/`, the package's
default-build tag guard `go test ./cmd/manamodelstats/`, and the structural
goldens `./internal/archtest`, `./internal/codeshape` and
`cmd/botbench TestConstructedDefaultIsByteIdentical`.

A 105-game constructed `-mode auto` re-run on current main (`.cards`, seed
7700000) reproduced the original `constructed-auto-verify` run within the
engine drift since the base: `windows` 39306 vs 39290, `ownmain:windows`
7519 vs 7504, `windows_payP_unplanned` 8 vs 8. The tables above are the
original measurement against 34867835c, not a re-measurement.

The operator's output directory `/mnt/sata/gorge-training/manamodel/` is
mounted read-only in the landing sandbox, so the fresh verification run
wrote its JSON under the worktree's `.ds4/scratch/`.
