# Stage 2: search

The search is the mage fork's MageZero MCTS (`docs/mz/xmage-spec.md`), ported onto gorge with the stage 1
groundwork. Where this file and the fork disagree, the fork wins unless `mz-gorge-design.md` says otherwise.

## Pieces (`bots/mz`)

| Piece | XMage | gorge |
|---|---|---|
| Config | `MCTSDefaults`, `Config.PlayerConfig` | `Options`: the same knobs and defaults (budget, timeout, `backprop_discount` 0.99, priors, noise, `C_PUCT` 1, node/depth caps, `prune_duplicate_states`, `manual_tapping`, mulligans, `offline_mode`) |
| Node | `MCTSNode`/`MCTSNode2` | `node`: action, acting player, atomic type, visits, score, prior, virtual loss, terminal/winner, the two hashes. Every node holds its own engine; a micro-node's is its parent's with the pick staged (stage 1's `Stage`/`Decline`) |
| Validate | `validateState` (replay scripts to the next pause) | no replay: clone the parent's engine; apply the atom (`atomic.Apply`); answer what the search does not node (forced passes, the 601.2g tapper, single-choice atoms) until the next node decision |
| Actions | `MCTSPlayer` (playables, `selectAttackersOneAtATime`, …) | `bots/mz/atomic` for the factorization; per tap mode: autotap offers non-mana options plus `EnsurePotentialCasts`, manual all options; both sim players use the searching bot's tap mode |
| Forced pass and checkpoints | `priority` auto-pass, `isCheckPoint` | no node for a priority whose only playable is pass, unless it is a checkpoint (turn-1 upkeep, begin combat, declare attackers, declare blockers; every priority under manual tap) |
| Tapper | `ComputerPlayer.playManaHandling` | answers the CR 601.2g mana window for autotap, never a node; if it cannot pay, the cast reverses and the node is illegal |
| Leaf | `GameStateEvaluator3`, parent's value at micro-nodes | the same heuristic over the dynamic view |
| Search loop | `applyMCTS` | the same loop: select, validate, prune illegal, dedup in scope, evaluate, expand, back up with virtual loss; exit only once a legal priority node exists, on budget or timeout; node and depth caps |
| Dedup | `getMatchingStateInScope` and the common-ancestor rule | the same, on the functional hash |
| Reuse | `getNextAction`: match the new root by functional hash and logical string in the old tree, else the new root; rebuild if no best child | the same, on the logical hash (functional + IDs) |
| Choice | `bestChild` (mask nodes with no legal priority below, most visits, temperature sampling under noise) | the same |
| Hashes | `FeatureGraph` hash; `GameState.getValue` | the encoder's graph built from `ProjectDynamic` and hashed as the fork does (functional); plus the IDs (logical). The network I/O and training rows are stage 3 |
| Seat | `ComputerPlayerMCTS2` in the base game | a registered `mz` bot: a `seat.EngineSeat` (the driver hands it a clone of the live engine: perfect information, principle 10) and a `seat.PotentialCastConsumer` under autotap. Each gorge decision is answered by searching each atom in turn, as XMage searches each microdecision, then composed into one intent |

## Memory

No replay anywhere: every node, micro-nodes included, holds its own engine (its parent's clone with its pick applied, staged
picks living in the engine's pending decision). A stored engine owns its event log and object arena: about 300–450 KB
mid-game. A node's engine is needed only to validate its children, so it is released once every child is validated; only the
frontier holds engines. The root takes a fresh clone of the base game, after tree reuse as well.

## Measurement

Offline MCTS only. On the fork's config decks, in both tap modes: sims/s and cost per search step, logged in `docs/mz/LOG.md`, and
win rate against gorge's `bot` as a sanity check.
