# Invariants

gorge relies on a small number of design rules. Each one is enforced by a test
or a build ratchet rather than by review alone; where enforcement is
after-the-fact or by convention, the table says so. *As of 2026-10-16,
`main` @ `6d95c6a54`.*

| Invariant | Rule | Enforced by |
|---|---|---|
| **Sole mutator** | Only `events.Apply` writes to `state.Game`. | After the fact: `engine/rules/heads_test.go` (`TestHeads`, pinned chain heads), `TestRepoDeckGamesReplayExactly`, `make sim` replay verification, the replay first-divergence check, and `internal/archtest/tape_owner_test.go`. |
| **Determinism** | No wall clock, a seeded PCG RNG (`engine/rules/rng.go`), and no `map` iteration whose order can reach an event. | `internal/archtest/arch_test.go`: `TestTimeIsImportedOnlyByTheHost`, `TestNoLegacyMathRand`. Map order is by convention and caught by the golden heads. |
| **Append-only event kinds** | A new `events.Kind` is appended after the last constant, with a `kindinfo.go` entry, so ordinals and recorded hash chains never move. | `engine/events/kindinfo_test.go`, golden heads. |
| **Closed decision kinds** | Every seat client must answer every `decision.Kind`; adding one is a deliberate decision. | `engine/decision/decision_test.go` `TestKindsListsEveryKindOnce`. |
| **Dependency direction** | `effects`, `view`, `botpolicy` and `protocol` never import `rules`; `engine/` never imports `bots/` or `server/`; `engine/seat` imports only `decision` and `view`. | `arch_test.go` `TestDependencyOrderHolds` (named edges, e.g. `botpolicy` → `view`/`rules`/`seat`, `protocol` → `bots`), `layering*_test.go`. The group rule `engine/` ↛ `bots/`/`server/` holds in the measured import graph but is not yet pinned as one test. |
| **Extensions live outside** | Draft events, compliance oracles, training research and the research bots live in a private companion module, four-yay, that imports gorge; gorge never imports it. Extensions plug in through `bots.Register`, `server/gorged.RegisterNetBot` and `server/gorged.Extensions` ([ADR 0001](https://github.com/adams-shaun/gorge/blob/main/docs/decisions/0001-repo-layout.md), 2026-10-16 amendment). | Structural: the root `go.mod` has no `require` lines (`TestRootModuleHasNoRequires`), so nothing in gorge can import four-yay. This replaced the earlier in-tree fence. |
| **Information boundary** | Seats see only a `view.View`, never the engine. Clairvoyant search never reaches a hosted or rated game. | `TestNoExportLeaksAnEngineGame`, the `view` redaction tests. |
| **Forge GPL boundary** | Forge card scripts are never committed. | `engine/cards/boundary_test.go` `TestNoForgeScriptsTracked`. |
| **No cgo, no third-party deps, 32-bit clean** | Pure Go in the card pipeline and rules core. | `TestEngineCompilesFor32Bit`, `CGO_ENABLED=0` in the `Makefile`, and `internal/archtest/gomod_test.go` `TestRootModuleHasNoRequires`. |
| **Ratchets only fall** | Known-unsupported cards, unread card parameters and known approximations can only shrink. | `engine/rules/testdata/known-unsupported/` + `TestEveryRepoDeckIsFullySupported` (fails in both directions), the param census, `testkit/testutil/agentsdoc_test.go`, and review (the maintainers' landing pipeline also gates it). |
| **Per-test budget** | Every test fits 4 GB RSS, 4 vCPU and 1 minute of wall time. | `cmd/testbudget` in the post-merge suite, with shrink-only exception inventories in `testkit/testutil/testdata/`. |
| **Corpus or it didn't run** | Corpus-dependent tests skip when `.cards/` is absent. | Convention: a fast green run without the corpus has tested very little. |

!!! note "No third-party dependency"
    The experimental GPU game pool that needed `github.com/ebitengine/purego`
    left gorge with the research tree. The root `go.mod` has no `require`
    lines, and `TestRootModuleHasNoRequires` keeps it that way.

## Changing an invariant

Each invariant has a legitimate way to change it (for example, moving a golden
chain head with a named cause, or adding a decision kind with every client
updated in the same change). The contributor-facing version of this page,
with that procedure for each row and the workflow around it, is
[`docs/agents/invariants.md`](https://github.com/adams-shaun/gorge/blob/main/docs/agents/invariants.md).
Deliberate behaviours that can look like bugs are listed in the
[engine contracts spec](https://github.com/adams-shaun/gorge/blob/main/docs/design/2026-09-22-engine-contracts.md).
