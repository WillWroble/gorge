# Bots and learned policies

gorge ships a small set of bot policies, and has been used as an environment
for search and machine-learning experiments. This page inventories the bots
gorge ships, their status and their best measured result. The research bots,
their training and their evaluation tooling live in a private companion
repository and are not covered here. *Compiled 2026-10-12 against `main` @
`f1ac002c4` from recorded evaluations, trimmed 2026-10-16 to the bots that
remain in gorge; dates are when each result was measured. Win rates are
against the named opponent with 95% confidence intervals where recorded.*

## The finding so far

!!! abstract "Through-line, 2026-09-06 to 2026-10-10 (interpretation)"
    **Decision-time search is the only source of playing strength found so
    far.** Every attempt to distil search into a cheap policy (cast profiles,
    policy networks, imitation of human play, an LLM fine-tune) reproduced the
    heuristic bot rather than beating it. Among the bots gorge ships,
    `sb-tactical` is the strongest heuristic and `az-redeal` the search seat.

## Inventory

The hosted policies are the ones `bots/all` links (each registers itself in
the `bots` registry, `bots/registry.go`):

| Policy | Code | Status | Headline result | Cost |
|---|---|---|---|---|
| `bot` default heuristic | `bots/botpolicy/`, `bots/bot`, `bots/heuristic` | **live**; also the opponent and environment model inside every search seat | auto-pay attack simulation +3.17pp constructed / +3.69pp Commander | ~121 games/s per core |
| `lethal-pressure` | `bots/lethalpressure` | live, to be retired; plays identically to `bot` with manual taps | 24/24 identical games | same as `bot` |
| **`sb-tactical`** | `bots/sbtactical`, `bots/spellbench/builtins` | **live; strongest heuristic** | held-out 1538 vs `bot` 1277 Elo; 0.687 [0.650, 0.724] vs `bot` on FDN (2026-10-08) | ~2.9M games/h on 12 workers |
| `az-redeal` (MCTS) | `bots/azredeal`, `bots/azmcts` | live | 25 sims: 64.7% vs `bot`; 100 sims: 70.5%; loses to `sb-tactical` (32.2%); 400 vs 100 sims +4.4pp; exploration constant has no effect | 65.8 ms per decision at 25 sims |

Support packages the hosted bots and outside policies build on:

| Package | What it is |
|---|---|
| `bots/searchseat`, `bots/searchprobe` | The PIMC search seat and its hidden-information world sampler. The L10 seat measured 53.7% vs a 50.4% control (+3.3pp) on the held-out gate (2026-09-22) at 664 ms mean / 2.2 s p95 per decision; it is not a hosted policy. |
| `bots/policynet` | The pure-Go learned policy (features, MLP scorer, value head, `.gpol` checkpoints). Every distillation, expert-iteration and PPO arm reproduced `bot`; the plumbing stays merged behind flags. |
| `bots/gonet` | The pure-Go forward pass for `.dzgw` MLP networks (`gonet.Load`). |
| `bots/sanity` | The common-sense seat decorator the heuristic policies apply (it drops, for example, a hostile effect aimed only at the seat's own permanents). |
| `bots/spellbench` (+`builtins`, `payexec`) | The SpellBench benchmark deck catalogs; the ported SpellBench builtin bots and the `sb-tactical` heuristic; `payexec`, which lowers a payment plan onto the manual mana surface. |

## Evaluating a policy yourself

`testkit/bench` (`PlayGame`, `RunPairs`) plays seeded games and deck-pair
matrices between any `seat.Seat` implementations, with watchdogs and
livelock recovery. The adoption ladder, the evaluation protocol and the rules
for bot code are in
[*Bot player training and adoption guidelines*](../research/bots.md).
