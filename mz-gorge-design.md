# mz-gorge design

mz-gorge is MageZero running on the gorge rules engine.

This document decides. Where it is silent, the mage fork (`WillWroble/mage`, `graph-encoder` HEAD) fills in the details: it is the ground-truth spec for behaviour and design unless this document says otherwise. gorge provides the rules.

## Principles

**0.** Everything should be deterministic.

**1. MTG is an MDP.** Everything dynamic should be in the view. This needs global changes to gorge, but they shouldn't conflict with the other bots. Avoid replay scripts if possible.

**2.** Microdecisions (non-priority) should be factorized into reusable atomic decisions (e.g. combat should be one attacker at a time). Look at the mage fork to see what these atomic decision types are. This can live in the bot implementation, as it did in XMage, but it needs principle 1. Not every decision type needs a prior.

**3.** The immediate action set should be at least a superset of the true action set. A legal action that's never offered can never be explored, but an illegal one can be pruned afterwards at almost no cost.

**4.** The tree should be reused. This needs a logical-state match.

**5.** Functionally identical states should be pruned dynamically during search. (The original path is always kept. Search for duplicates only within decision scopes.)

**6.** The API, config and output of the packaged engine should be identical to my XMage.

**7.** Network calls should be async and batched the way XMage does it.

**8.** Gorge's existing bots keep working. Engine changes are additive, so my bot can play against gorge's bots and be swapped in for any of them. Like how all bots inherit from the ComputerPlayer entity in XMage.

**9.** The search and the encoder behave like my XMage's.

**10. Hidden information** is the one departure from XMage. Start with the existing perfect-information MCTS, but leave the door open for IS-MCTS afterwards.

## Behaviour carried over from XMage

- **Microstate is in the view.** Every partial state, including a half-made factorized decision, is in the view, and order invariant. No separate microstate store and no decision sequences (XMage's `PlayerHistory` target/choice/use/num sequences are not carried over).
- **Each microstate is its own tree node.**
- **Sim game vs base game.** Principle 8 binds only the base game: there the bot answers gorge's ordinary decisions. Inside its sim games it may use any decision API it needs.
- **Tree reuse falls back to a rebuild.** When the tree can't be reused (no logical match, or the match was pruned), it is rebuilt from scratch, as in XMage.
- **Tap mode in sims is the searching bot's own, for both players** (a departure from the fork's code, where each sim player keeps its original player's `autoTap`). In the base game only the mz seat's own mode applies; its autotap action set reaches it through `seat.PotentialCastConsumer` and the driver's `Engine.EnsurePotentialCasts`. As in XMage, every priority window is a checkpoint when the mode is manual tap.
- **Trigger order and replacement choice are searched** (a departure from XMage, which takes the first): one pick at a time, like any other atomic decision.
- **The encoder is the fork's.** Its graph is the network's input and its hash is the functional hash, so dedup is as coarse
  as the fork's: what the encoder leaves out (library order, most engine internals) does not separate states. Feature names
  match the fork's wherever gorge has the concept; card and ability text is gorge's own.
- **The logical hash** is the functional hash plus the ID of every object in the graph and the decision's option list (an
  action is an option index, so a reused node must offer the same options on the same objects). A tree rebuild only costs
  time.
- **Training data is not HDF5** (the one allowed departure from principle 6): the same datasets and row layout, written as
  NumPy `.npz` with Go's standard library.
- **Every action leads to a legal priority state.** The bot only takes an action whose subtree reaches a legal priority state, and search time is extended until that holds, as in XMage.

## Layers

- **Engine:** 0, 1
- **Bot:** 2, 4, 5, 7, 9, 10
- **Engine and bot:** 3, 8
- **Packaging:** 6

## Glossary

- **Microdecision:** any non-priority decision.
- **Atomic decision:** a reusable decision type that every microdecision is built from.
- **Decision scope:** a run of consecutive decisions made by the same player.
- **Functionally identical:** the same on paper from here on, whatever the order things were picked in or which identical object is which. Playing either of two copies of a card in hand is the same state. This applies to factorized microdecision states too: attacking with A then B is the same state as B then A. Used for deduplication. The functional hash comes from the encoder, as in XMage.
- **Logical-state match:** functionally identical with the same object IDs and the same decision options, because actions passed from the sim tree to the base game reference them. Weaker than XMage's `getValue` string, to limit tree rebuilds (a rebuild only costs time). Used for tree reuse.
- **Hashes:**

  | | Functional (dedup) | Logical = functional + IDs (tree reuse) |
  |---|---|---|
  | **Perfect info** | the encoder over the omniscient view | plus the IDs of every object |
  | **Hidden info** (per observer) | the encoder over what the deciding player can see | plus the IDs of the objects that player can see |

  Perfect-information search uses the perfect-info pair. The hidden-info pair leaves the door open for IS-MCTS.
- **Autotap / manual tap:** mana paid automatically / mana abilities taken as actions.
- **Offline MCTS:** search with the heuristic value, no network.

## Out of scope

- Changes to the Python side.
- IS-MCTS, for now.