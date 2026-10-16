# Card support by set

How much of each *Magic* set gorge supports, and how sure we are. Standard is
measured card by card against an oracle; every other set is an estimate.

!!! info "Snapshot"
    The tables on this page are a hand snapshot **as of 2026-10-12, `main` @
    `f1ac002c4`** (Forge pin `25a608a4fc5f`, XMage pin `6b602a1c85e8`, verdicts
    last written at `e606fe616`). When the docs build can run the measurement
    itself, its output appears in the *Automated measurement* section at the
    end of this page. See [Regenerating the tables](regenerating.md).

**Headline.** All 20 Standard sets pass **Level A** (5,331 / 5,331 cards).
**Level B** stands at 5,002 / 5,331 (**93.8%**), with 329 cards outstanding;
Duskmourn is the weakest at 76.8%. Most other sets are an *estimated* 96–98%
playable. Corpus-wide, `forgec report` gives
`cards: 34074  playable: 32089 (94.2%)`.

## What "supported" means

Support is layered. Each tier certifies more than the one above it.

| Tier | Produced by | Certifies | Does **not** certify |
|---|---|---|---|
| Compiles | `forgec` → IR cache | the Forge script parses into the IR | behaviour |
| **Playable** | `forgec report` / `make coverage` | every primitive the card uses (`api:`, `trig:`, `kw:`, `stat:`, `count:`, …) is implemented | that the card behaves correctly; a parameter can still be unread (tracked by a separate census ratchet) |
| **Level A** | `lab/oracle/compliance/gate` against `lab/oracle/compliance/verdicts/*.jsonl` | playable **and** a generated cast-and-resolve, play-land or counter-spell scenario agrees with XMage (or a ruled divergence, or a hand-written oracle scenario). Basic lands are skipped; cards XMage lacks need a hand scenario | activated abilities, trigger modes, attacks and blocks, static abilities |
| **Level B** | the same gate, level B ([spec](https://github.com/adams-shaun/gorge/blob/main/docs/superpowers/specs/2026-10-05-compliance-level-b.md)) | Level A plus one XMage-compared scenario **per requirement**: each activated ability (mana abilities included), each trigger, each static ability, and attack/block for creatures with combat keywords or legality statics | requirements no scenario template serves yet are reported as a *template gap*; keywords are not compared on their own |

**Oracle verdicts.** Each (card, template, scenario hash) gets one verdict:
*agree*, *diverge*, *harness* (the test rig, not the engine, failed) or
*xmage_wrong*. A verdict stays valid until the scenario's hash changes, and is
keyed by card name, so reprints inherit it. Disagreements are decided by the
card's Oracle text and the Comprehensive Rules; an *xmage_wrong* verdict needs
a recorded ruling. The current inventory is 12,789 verdicts over 5,140 cards:
12,512 agree, 164 diverge, 87 harness and 26 xmage_wrong.

A third oracle, Forge itself, is being added for three-way adjudication. It
only corroborates; it never writes verdicts.

**Set membership.** Forge scripts carry no set information, so set lists
come from XMage's set classes (`lab/oracle/compliance/manifests/<SET>.json`, 589 sets,
only cards XMage implements) and, for the 20 Standard sets, from full printed
lists (`lab/oracle/compliance/printed/<SET>.json`). `lab/oracle/compliance/declared.json` declares
all 20 Standard sets at Level A; none is declared at Level B yet.

## Standard (measured)

Playable is 100% for every set, since Level A requires full support.
*Outstanding* lists the cards failing Level B, by reason: *gap* = template gap,
*nv* = no verdict yet. A card can have more than one reason.

| Set | Name | Released | Cards | Playable % | A % | B % | Outstanding (B) |
|---|---|---|---|---|---|---|---|
| FRA | Reality Fracture | 2026-10-02 | 285 | 100 | 100 | 96.8 | 9: 5 diverge, 3 gap, 1 nv, 1 harness |
| HOB | The Hobbit | 2026-08-14 | 193 | 100 | 100 | 94.3 | 11: 5 gap, 3 diverge, 3 harness |
| MSH | Marvel Super Heroes | 2026-06-26 | 281 | 100 | 100 | 92.9 | 20: 12 diverge, 5 harness, 4 gap, 1 nv |
| SOS | Secrets of Strixhaven | 2026-04-24 | 271 | 100 | 100 | 97.0 | 8: 4 gap, 3 harness, 1 diverge |
| TMT | Teenage Mutant Ninja Turtles | 2026-03-06 | 190 | 100 | 100 | 96.8 | 6: 4 gap, 1 diverge, 1 harness |
| ECL | Lorwyn Eclipsed | 2026-01-23 | 273 | 100 | 100 | 89.4 | 29: 25 diverge, 8 harness, 5 gap, 1 nv |
| TLA | Avatar: The Last Airbender | 2025-11-21 | 286 | 100 | 100 | 93.4 | 19: 8 gap, 8 diverge, 4 harness, 2 nv |
| SPM | Marvel's Spider-Man | 2025-09-26 | 193 | 100 | 100 | 92.2 | 15: 12 gap, 3 diverge, 3 harness |
| EOE | Edge of Eternities | 2025-08-01 | 266 | 100 | 100 | 96.2 | 10: 5 harness, 3 gap, 2 diverge |
| FIN | Final Fantasy | 2025-06-13 | 312 | 100 | 100 | 92.6 | 23: 9 gap, 8 harness, 6 diverge, 1 nv |
| TDM | Tarkir: Dragonstorm | 2025-04-11 | 276 | 100 | 100 | 94.9 | 14: 6 gap, 5 harness, 3 diverge |
| DFT | Aetherdrift | 2025-02-14 | 276 | 100 | 100 | 95.3 | 13: 7 gap, 3 diverge, 2 harness, 1 stale |
| FDN | Foundations | 2024-11-15 | 517 | 100 | 100 | 99.8 | 1: 1 diverge |
| DSK | Duskmourn | 2024-09-27 | 276 | 100 | 100 | **76.8** | 64: 45 diverge (mostly Rooms), 20 gap, 2 nv, 2 harness |
| BLB | Bloomburrow | 2024-08-02 | 279 | 100 | 100 | 95.7 | 12: 5 diverge, 3 gap, 3 harness, 1 nv |
| OTJ | Outlaws of Thunder Junction | 2024-04-19 | 276 | 100 | 100 | 94.6 | 15: 5 gap, 5 diverge, 4 harness, 2 nv |
| BIG | The Big Score | 2024-04-19 | 30 | 100 | 100 | 90.0 | 3: 2 diverge, 1 gap, 1 harness |
| MKM | Murders at Karlov Manor | 2024-02-09 | 279 | 100 | 100 | 89.6 | 29: 16 diverge, 8 gap, 7 harness, 1 nv |
| LCI | Lost Caverns of Ixalan | 2023-11-17 | 291 | 100 | 100 | 93.8 | 18: 8 gap, 7 nv, 5 diverge, 1 harness |
| WOE | Wilds of Eldraine | 2023-09-08 | 281 | 100 | 100 | 96.4 | 10: 5 diverge, 3 harness, 2 gap |
| **Total** | | | **5,331** | 100 | 100 | **93.8** | **329** |

Level B has risen from 81.2% (2026-10-08) to 93.8%. Duskmourn's outstanding
cards are mostly Rooms.

## Other sets (estimated)

!!! warning "Estimates"
    These sets have no printed card list and no full oracle audit. *Playable %*
    is `(cards − unsupported − not in corpus) / cards` over XMage's manifest for
    the set. *A-carried %* is the share already passing Level A through a
    same-name verdict or as a basic land, a hard lower bound on Level A. See
    [method and error bars](#method-and-error-bars).

| Set | Name | Released | Cards | Unsupported | Playable % (est) | A-carried % |
|---|---|---|---|---|---|---|
| TRK | Star Trek (unreleased, partial) | 2026-11-13 | 81 | 0 | 100.0 | 18.5 |
| ACR | Assassin's Creed | 2024-07-05 | 136 | 13 | 90.4 | 8.8 |
| MH3 | Modern Horizons 3 | 2024-06-07 | 309 | 5 | 98.4 | 13.6 |
| LTR | LotR: Tales of Middle-earth | 2023-06-23 | 289 | 6 | 97.9 | 2.8 |
| MAT | MOM: Aftermath | 2023-05-12 | 50 | 2 | 96.0 | 4.0 |
| MOM | March of the Machine | 2023-04-21 | 296 | 1 | 99.7 | 7.1 |
| ONE | Phyrexia: All Will Be One | 2023-01-10 | 271 | 7 | 97.4 | 6.3 |
| BRO | The Brothers' War | 2022-11-18 | 280 | 21 | 92.5 | 6.1 |
| DMU | Dominaria United | 2022-09-09 | 271 | 13 | 95.2 | 10.3 |
| SNC | Streets of New Capenna | 2022-04-29 | 266 | 14 | 94.7 | 6.0 |
| NEO | Kamigawa: Neon Dynasty | 2022-02-18 | 287 | 6 | 97.9 | 8.4 |
| VOW | Innistrad: Crimson Vow | 2021-11-19 | 272 | 32 | 88.2 | 9.2 |
| MID | Innistrad: Midnight Hunt | 2021-09-24 | 272 | 45 | 83.5 | 8.1 |
| AFR | Adventures in the Forgotten Realms | 2021-07-23 | 266 | 6 | 97.7 | 3.8 |
| MH2 | Modern Horizons 2 | 2021-06-11 | 309 | 19 | 93.9 | 5.2 |
| STX | Strixhaven | 2021-04-23 | 280 | 26 | 90.7 | 3.6 |
| KHM | Kaldheim | 2021-02-05 | 305 | 4 | 98.7 | 7.5 |
| ZNR | Zendikar Rising | 2020-09-25 | 270 | 1 | 99.6 | 9.6 |
| M21 | Core Set 2021 | 2020-07-03 | 285 | 2 | 99.3 | 17.5 |
| IKO | Ikoria | 2020-04-24 | 265 | 0 | 100.0 | 10.9 |
| THB | Theros Beyond Death | 2020-01-24 | 273 | 5 | 98.2 | 6.6 |
| ELD | Throne of Eldraine | 2019-10-04 | 285 | 2 | 99.3 | 11.9 |
| M20 | Core Set 2020 | 2019-07-12 | 329 | 3 | 99.1 | 18.2 |
| WAR | War of the Spark | 2019-05-03 | 265 | 5 | 98.1 | 6.4 |
| RNA | Ravnica Allegiance | 2019-01-25 | 268 | 13 | 95.1 | 7.8 |
| GRN | Guilds of Ravnica | 2018-10-05 | 268 | 3 | 98.9 | 10.4 |
| M19 | Core Set 2019 | 2018-07-13 | 299 | 0 | 100.0 | 19.1 |
| DOM | Dominaria | 2018-04-27 | 265 | 1 | 99.6 | 10.6 |
| RIX / XLN | Rivals / Ixalan | 2018-01 / 2017-09 | 205 / 274 | 1 / 3 | 99.5 / 98.9 | 10.2 / 9.1 |
| HOU / AKH | Hour of Devastation / Amonkhet | 2017 | 199 / 272 | 0 / 0 | 100 / 100 | 6.5 / 4.4 |
| AER / KLD | Aether Revolt / Kaladesh | 2017 / 2016 | 194 / 264 | 1 / 2 | 99.5 / 99.2 | 4.6 / 9.1 |
| EMN / SOI | Eldritch Moon / Shadows over Innistrad | 2016 | 208 / 287 | 0 / 4 | 100 / 98.6 | 2.4 / 4.5 |
| OGW / BFZ | Oath / Battle for Zendikar | 2016 / 2015 | 183 / 254 | 0 / 25 | 100 / 90.2 | 9.3 / 9.1 |
| ORI / DTK / FRF / KTK | Origins → Khans | 2015–2014 | 273/254/180/254 | 23/18/0/2 | 91.6/92.9/100/99.2 | 11.7/9.8/13.3/11.4 |
| M15 / JOU / BNG / THS | | 2014–2013 | 269/165/165/234 | 0/1/15/3 | 100/99.4/90.9/98.7 | — |
| M14 / DGM / GTC / RTR | | 2013–2012 | 234/155/249/254 | 3/4/2/20 | 98.7/97.4/99.2/92.1 | — |

The weakest older sets are Saviors and Betrayers of Kamigawa (81.8%) and
Champions of Kamigawa (82.1%), held back by Bushido, Soulshift and Splice, and
Nemesis (83.2%). Daybound/Nightbound holds back Midnight Hunt and Crimson Vow.

## Roll-up by set type (estimated)

Per-set entries: a card reprinted in several sets is counted in each.

| Group | Sets | Entries | Unsupported | Playable % (est) | A-carried % |
|---|---|---|---|---|---|
| Standard (measured) | 20 | 5,331 | 0 | 100.0 | 100.0 |
| Expansion (non-Std) | 98 | 21,261 | 862 | 95.9 | 5.8 |
| Core | 22 | 6,579 | 176 | 97.3 | 10.9 |
| Supplemental | 164 | 26,701 | 579 | 97.8 | 16.1 |
| Supplemental Modern-legal | 8 | 2,025 | 81 | 96.0 | 8.9 |
| Promotional | 223 | 8,723 | 264 | 97.0 | 16.3 |
| Arena / MTGO | 22 / 10 | 2,082 / 2,171 | 67 / 68 | 96.8 / 96.9 | — |
| Joke | 13 | 177 | 7 (+53 not in corpus) | 66.1 | — |
| Custom | 3 | 361 | 2 (+330 not in corpus) | 8.0 | — |

## Method and error bars

`oraclediff status -all -json -level A` runs the compliance gate over all 589
manifests and puts each card in one bucket: *unsupported*, *not in corpus*,
*template gap* or *no verdict*. Agreeing reprints and basic lands pass. As a
cross-check, the corpus-wide figure (94.2%, distinct cards including Un-sets
and Alchemy) sits below the manifest-weighted figures above, as expected.

1. **Denominator.** Manifests omit cards XMage has not implemented. In
   Standard that moves a set's count by −16 to +5 cards (5.6% of Reality
   Fracture, 6.7% of The Hobbit), so the newest sets may be overstated by
   1–5 points; older sets by under 1 point.
2. **Name folding** between XMage and Forge naming: ±0–1 card per set.
3. **Playable is not correct.** The first full XMage audit of Standard
   (2026-10-03) found ~11% of cards outstanding. Expect an unaudited set's
   first-pass Level A to land 5–10 points below its playable figure, and
   Level B at roughly 75–90%.
4. **A-carried** is a hard lower bound on Level A.
5. **Verdict noise** is negligible: 4 flips in 12,196 verdicts on re-run.

--8<-- "support/_generated_sets.md"
