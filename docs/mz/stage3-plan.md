# Stage 3: encoder, network I/O, data generation

Stage 3 ports the mage fork's `StateEncoder`/`FeatureGraph`, `RemoteModelEvaluator`, `LabeledStateWriter` and
`ParallelDataGenerator` onto stage 2's search. Where this file and the fork disagree, the fork wins unless
`mz-gorge-design.md` says otherwise. Stage 2 and stage 3 are reviewed together on `mz-gorge`.

## Pieces

| Piece | XMage | gorge |
|---|---|---|
| Graph | `FeatureGraph`: nodes keyed by UUID, `id` = bucket of the name's hash, numeric `value`, labelled edges to shared feature nodes | the same, keyed by object ID for objects and by the name's hash for features; `GraphArrays` in sorted key order |
| Encoder | `StateEncoder.processState` over `Game` | the same walk over `view.ProjectDynamic`: root, decision type and text, `StopChoosing`, exile, both players (hand only for the decider unless perfect info), stack, option pile, decision source; the picks so far from `Decision.Staged`/`Declined` |
| Names | XMage's | the fork's names for every shared concept; card and ability text from gorge's card data |
| Functional hash | `getStateHash` (16 rounds) | the same over the encoder's graph; replaces stage 2's view hash |
| Logical hash | functional + `GameState.getValue` | functional + every object ID in the graph + the decision's options |
| Actions | priority child → ability node; target → its node; pass → `PassAbility`; stop → `StopChoosing`; use → `USE_TRUE/FALSE` | every option maps to a graph node (a cast or activation to its card's ability node) |
| Priors and noise | `setPriors`, Dirichlet at the root, visit sampling in `bestChild` | the same |
| Evaluator | `RemoteModelEvaluator`: msgpack CSR batches of 4 every 2.5 ms, 16 calls in flight, `MAX_PENDING` 4 | the same, standard library only |
| Training rows | `LabeledState`, TD-blended labels, HDF5 | the same rows and datasets, written as NumPy `.npz` |
| Data generation | `MageZeroMain`/`ParallelDataGenerator` with `config.yml` | `cmd/mzdatagen` reading the same `config.yml` keys |

## Measurement

Offline only until offline play is verified: sims/s and cost per search step on the fork's decks in both tap modes (the
search stays within 2× the engine work per sim), win rate against gorge's `bot`, and a determinism check.
