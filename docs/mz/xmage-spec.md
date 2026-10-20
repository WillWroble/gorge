# MageZero in XMage: the behaviour spec

What the mage fork (`WillWroble/mage`, `graph-encoder` @ `3b76645`, 116 commits on top of upstream `09a423ae6`) does.
It is the ground truth for mz-gorge wherever `mz-gorge-design.md` is silent. File references are to that fork.

## System

- **MageZero module** (`Mage.MageZero`). `MageZeroMain` loads `config.yml` and runs `ParallelDataGenerator`:
  - It plays N games on a fixed thread pool, one game per thread with a per-thread seed (`RandomUtil` is thread-local).
  - Two model servers: player A on `port`, player B on `opponent_port`. Each player falls back to offline mode
    when its server is unreachable or `offline_mode` is set.
  - `goes_first` is player_a, player_b, or alternating by game count.
  - `max_turns` stops a game. `max_minutes` cancels it, and a cancelled game is dropped.
  - A writer thread appends each finished game to HDF5 (`LabeledStateWriter`), then writes the win rate and the
    feature map.
- **Player types.** `mcts` is `ComputerPlayerMCTS2`. `minimax` is `ComputerPlayer8`: a `ComputerPlayer7` with
  autotap, which only passes at checkpoints. Opponent nodes in the tree are always the MCTS player's own sims,
  whatever the real opponent is.
- **Config knobs.** Each knob maps onto a player field (`configurePlayer`):
  - priors per head (priority, target, binary, opponent), `prior_temperature`, `prior_bonus`;
  - noise `enabled`, `dirichlet_noise`, `selection_temperature`;
  - `search_budget`, `timeout_ms`, `td_discount`, `backprop_discount`, `offline_mode`, `prune_duplicate_states`;
  - `mulligans_enabled`, `manual_tapping`, `see_opponent_hand`.
  - `MCTSDefaults` holds the same values for runs outside the data generator.
- **Tests** (`Mage.Tests/.../AI/RL/MCTSDeckValidationTest`). 16 games per deck, offline, on 4 threads, for:
  - every deck vs MCTS;
  - a commander pool;
  - every deck vs minimax, in both seats;
  - a fixed-seed single game (`test_single_game`).
  - A deck passes when every game completes. `TestComputerPlayerMonteCarlo2` lets card tests script an MCTS2 player.

## Determinism (principle 0)

- Each game has its own `gameLocalRandom`, deep-copied with the `GameState`. It drives:
  - library shuffles (a Fisher-Yates of its own);
  - token, stack-ability and triggered-ability IDs, and mode IDs;
  - X and random-mode choices.
- Player IDs are `nameUUIDFromBytes(name)`. `PassAbility`, `STOP_CHOOSING` and the combat pseudo-abilities have fixed
  IDs.
- Every unordered collection that can affect play is sorted by ID or name:
  - the mana pool (`ManaItems`), mana sources, special actions, approvers, replacement effects;
  - cards added to the library, `getCardsSorted`, exile zones;
  - available attackers and blockers.
- `ChoiceImpl.setRandomChoice` picks the first value.
- `ComputerPlayerMCTS2`'s constructor reseeds `RandomUtil` with `DEFAULT_SEED` (`0` means random).

## Atomic decisions (principle 2)

Five types, `ActionEncoder.ActionType`:

| Type | Comes from | Children | Prior head / label |
|---|---|---|---|
| PRIORITY | `priority` | the playables (ability copies), pass included | `policy_priority`, keyed by ability ID |
| CHOOSE_TARGET | every target/card pick, attackers, blockers | each remaining possible target; plus `STOP_CHOOSING` once the target `isChosen` | `policy_target`, keyed by target ID / STOP node |
| CHOOSE_USE | `chooseUse`, mulligan | false, true (false first) | `policy_binary` |
| CHOOSE_NUM | X/amounts (`max-min+1`, capped at 64), modes (index into available modes, `null` first once `minModes` is met) | 0..n-1 | none |
| MAKE_CHOICE | `choose(Choice)`: colour, type, … | the key choices, else the choices | none |

- **Combat is one creature at a time, in ObjID order.**
  - Attack: for each available attacker, a CHOOSE_TARGET over the defenders with min 0. Its source is
    `ChooseToAttackAbility(attacker)`, and STOP means "doesn't attack".
  - Block: for each available blocker, a CHOOSE_TARGET over the attacking creatures (0..1), with source
    `ChooseCreatureToBlockAbility(blocker)`.
  - An illegal block combination in a sim marks the state illegal (`Combat`, `aiTries > 1`).
- **Multi-target picks are one target per node**, recursing until STOP or the target is complete.
- **No node at all when there is one option or none:** a single target, a single choice, `min >= max`, a fizzle.
- **Fixed defaults, never searched:**
  - the starting player (the chooser starts);
  - trigger order (first) and replacement effect (index 0, sorted by `toString`);
  - piles (left); creature type (a hand/board heuristic);
  - the opponent's mulligan in a sim (keep). Mulligans only count with 6+ cards and `allowMulligans`.
- **Partial state the encoder sees:** what has been picked so far (targets, choices, uses, nums), via
  `PlayerHistory` (`processMicroDecisions`). mz-gorge replaces this with view state (design doc).

## Priority, checkpoints, tapping

- **Auto-pass rule.** Priority with only one playable (pass), at a non-checkpoint, passes without creating a node.
  This applies in the base game (`ComputerPlayerMCTS.priority`) and in sims (`MCTSPlayer.priority`) alike.
- **Checkpoints** (`GameImpl.isCheckPoint`):
  - **Autotap:** turn-1 upkeep, begin combat, declare attackers, declare blockers.
  - **Manual tap:** every priority (if either player taps manually).
- **Autotap:**
  - playables = `getPlayable` minus mana abilities, plus pass;
  - payment uses XMage's auto-tapper (sorted deterministically);
  - mana abilities are never recorded as actions.
- **Manual tap:**
  - playables are built from the floating pool only. A spell is offered only once the pool can pay;
    mana abilities are actions.
  - Payment is from the pool only (`autoPayFromPool`), with no mana-ability activation while paying.
  - X is capped by the pool.

## Tree

- **Node.** A node is a decision point: `playerId` (who acts), `actionType`, its children, and the action that led
  to it. It also holds `featureGraph` and `stateString`, the terminal and winner flags, and visits, score and prior.
  - A priority node stores its `GameState`. A micro node keeps its parent's state and re-reaches itself from
    there with prefix scripts.
  - `validateState` resets the shared sim game to the parent's state, runs the scripts to the next decision
    (`freezeState` pauses the game), and fills in the fields. A failed script marks the node illegal.
- **Both players' decisions are nodes.**
  - Value is always from the searching player's perspective, in [-1, 1].
  - Selection flips the sign of Q at opponent nodes.
  - Opponent priors are used only if `opponent` priors are on.
- **Selection** is PUCT: `sign·Q + C_PUCT·P·sqrt(N)/(1+n)` with `C_PUCT = 1`.
  - Unvisited children have Q = 0.
  - A single child is taken without scoring.
  - Children are sorted by action ID plus source/target, so order is deterministic.
- **Priors.**
  - Uniform `1/K`.
  - With a network, `softmax(logit/priorTemp)` plus `priorBonus` for every child except mana abilities and pass.
    No renormalization after the bonus.
  - No prior for MAKE_CHOICE or CHOOSE_NUM.
  - Root Dirichlet noise (α 0.03, ε `dirichletNoiseEps`) is applied only when a policy exists and noise is on.
- **Leaf.**
  - **Offline:** `GameStateEvaluator3` (`tanh((mine−theirs)/15)` over life·0.6 + harmonic(hand) + Σ(1 + MV) of
    permanents) at priority nodes; micro nodes inherit their parent's value.
  - **Online:** the network's value, signed by perspective.
  - **Terminal:** +1 for a win, −1 otherwise.
- **Backup** discounts by `backpropDiscount` (0.99) per level up the tree.
  - Virtual loss: the expansion backs up −1 with a visit, then the evaluation adds `1 + value` with no visit.
  - Up to `MAX_PENDING = 4` evaluations are in flight; descent waits on a pending node.
- **Search loop** (`applyMCTS`). The root is pre-expanded and evaluated. Each iteration:
  1. Select to a leaf.
  2. Validate it. Illegal → prune. Then deduplicate (below).
  3. Evaluate and expand, then back up.
  - Exit only once the tree contains a legal priority node, then on the timeout or `root.visits >= budget`.
    Reused visits count towards the budget.
  - Hard limits: 300 s, 100k nodes, depth 1000.
- **Dedup** (`prune_duplicate_states`).
  - Match the new node's functional hash within the decision scope, starting from `getPlayerScope`, descending only
    through same-player or single-child nodes, skipping unexpanded ones.
  - At the common ancestor, the branch with more visits keeps the state; ties go to the existing node.
  - A pruned new node first backs up the match's mean score. `prune` corrects the stats up the tree.
- **Choosing** (`bestChild`).
  - Children with no legal priority state below them are reset (masked).
  - Then: the most visits; or visit^(1/τ) sampling when noise is on and τ ≥ 0.01.
  - A single child must be legal or lead to a legal node.
- **Reuse** (`getNextAction`).
  - Build the new root from `lastPriority` plus both players' histories, and validate, expand and evaluate it.
  - Search the old tree breadth-first for a node matching functional hash **and** logical string.
  - Use the match, or else the new root, and detach it.
  - If `bestChild` is null, rebuild from the new root and search again.
  - The chosen child becomes the root for the next decision.
- **Training data.**
  - Per searched decision: the root graph, the child visits keyed by action ID, the root mean, the type, and the
    seat. There is no row when the action map is null (MAKE_CHOICE, CHOOSE_NUM).
  - The label is TD-blended backwards: `label = td·future + (1−td)·rootMean`, seeded with ±1 by the result.

## Hashes

- **Functional:** `FeatureGraph.getStateHash()`. 16 rounds of graph mixing over nodes hashed from names, numbers and
  edge labels. It doesn't depend on IDs or insertion order.
  - The graph includes the decision type, the decision text, the option pile, the decision source and the picks
    so far.
  - It depends on the perspective: `perfectInfo` (`see_opponent_hand`) decides whether the opponent's hand is
    encoded. That same graph is the network's input.
- **Logical:** the functional hash **and** `GameState.getValue(game, targetPlayer)`. That string covers:
  - turn and step; active, priority and seat order; passed flags; life and pool;
  - the target player's hand (name+ID); the opponent's hand and the libraries by size only; graveyards (name+ID);
  - permanents, sorted (`getValue(GameState)`: controller, name, tapped, damage, **ID**, types, P/T, abilities,
    attachment IDs, counters);
  - stack objects with ability **ID**, modes and targets; exile zones; combat groups.

## Network I/O

- **Request.** `RemoteModelEvaluator` sends graphs as CSR msgpack (`indices`, `values`, `offsets`, `edge_*`).
  - Nodes are in sorted-UUID order; batches hold up to 4 graphs, flushed every 2.5 ms; at most 16 calls in flight.
- **Response.** Per-node `policy_priority` and `policy_target` scores (read at the children's action nodes),
  `policy_binary`, and `value`.
