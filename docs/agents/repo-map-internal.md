# Repo map — `internal/` infrastructure and shared tooling

Part of the [repo map](repo-map.md) index. Bot runtime packages are in
[repo-map-engine.md](repo-map-engine.md#bots); research harnesses are under
`lab/` ([repo-map-research.md](repo-map-research.md)).

## `internal/`

| Package | What it owns |
|---|---|
| `internal/archtest` | Import-graph and structural tests (dependency order, `time` allowlist, no `math/rand`, 32-bit build, root `go.mod` has no `require`, layering budgets for the `engine/rules` sub-packages, no engine leaks from host). |
| `internal/testutil` | Corpus registry (`CorpusRegistry` — **skips when `.cards/` is absent**), repo decks (`internal/testutil/decks/*.json`), shared invariants, the per-test budget exception inventories (`internal/testutil/testdata/`), and the AGENTS.md ratchet (`agentsdoc_test.go`). |
| `internal/testutil/feedback` | Loads player feedback snapshots; `feedback.EngineAt`. |
| `internal/bench` | Policy-agnostic game runner: `PlayGame`, `RunPairs`, watchdogs, livelock recovery. Shared by `lab/cmd/botbench` and `lab/cmd/policytune`. |
| `internal/botobs` | The observability checklist the reward loop's obs axis scores against ([`docs/design/2026-09-29-reward-loop-and-seed-agent-design.md`](../design/2026-09-29-reward-loop-and-seed-agent-design.md) §6). |
| `internal/codeshape` | Code-shape metrics and ratchets for `engine/rules` and `engine/effects` (function length, interface width); `cmd/codeshape` prints them. |
| `internal/corpusheap` | Heap-footprint ratchet for the opened corpus (tests only). |
| `internal/paymirror` | A/B check of planned vs manual cast payment; `cmd/paymirror` drives it. |
| `internal/testbudget` | Per-test-binary peak-RSS records for `cmd/testbudget`. |
| `internal/tsgen` | Go→TypeScript for `web/src/protocol.ts` (`cmd/gentypes`). |
