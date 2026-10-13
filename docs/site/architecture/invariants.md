# Invariants

gorge relies on a small number of design rules. Each one is enforced by a test
or a build ratchet rather than by review alone; where enforcement is
after-the-fact or by convention, the table says so. *As of 2026-10-12,
`main` @ `f1ac002c4`.*

| Invariant | Rule | Enforced by |
|---|---|---|
| **Sole mutator** | Only `events.Apply` writes to `state.Game`. | After the fact: `rules/heads_test.go` (`TestHeads`, pinned chain heads), `TestRepoDeckGamesReplayExactly`, `make sim` replay verification, the replay first-divergence check, and `internal/archtest/tape_owner_test.go`. |
| **Determinism** | No wall clock, a seeded PCG RNG (`rules/rng.go`), and no `map` iteration whose order can reach an event. | `internal/archtest/arch_test.go`: `TestTimeIsImportedOnlyByTheHost`, `TestNoLegacyMathRand`. Map order is by convention and caught by the golden heads. |
| **Append-only event kinds** | A new `events.Kind` is appended after the last constant, with a `kindinfo.go` entry, so ordinals and recorded hash chains never move. | `events/kindinfo_test.go`, golden heads. |
| **Closed decision kinds** | Every seat client must answer every `decision.Kind`; adding one is a deliberate decision. | `decision/decision_test.go` `TestKindsListsEveryKindOnce`. |
| **Dependency direction** | `effects`, `view`, `botpolicy` and `protocol` never import `rules`; the core never imports the host. | `arch_test.go` `TestDependencyOrderHolds`, `layering*_test.go`. |
| **Information boundary** | Seats see only a `view.View`, never the engine. Clairvoyant search never reaches a hosted or rated game. | `TestNoExportLeaksAnEngineGame`, the `view` redaction tests. |
| **Forge GPL boundary** | Forge card scripts are never committed. | `cards/boundary_test.go` `TestNoForgeScriptsTracked`. |
| **No cgo, no third-party deps, 32-bit clean** | Pure Go in the card pipeline and rules core. | `TestEngineCompilesFor32Bit`, `CGO_ENABLED=0` in the `Makefile`. There is no check on `go.mod` `require` lines; see the note below. |
| **Ratchets only fall** | Known-unsupported cards, unread card parameters and known approximations can only shrink. | `rules/testdata/known-unsupported/` + `TestEveryRepoDeckIsFullySupported` (fails in both directions), the param census, `internal/testutil/agentsdoc_test.go`, `scripts/ratchet_gate.py`. |
| **Per-test budget** | Every test fits 4 GB RSS, 4 vCPU and 1 minute of wall time. | `cmd/testbudget` in the post-merge suite, with shrink-only exception inventories in `internal/testutil/testdata/`. |
| **Corpus or it didn't run** | Corpus-dependent tests skip when `.cards/` is absent. | Convention: a fast green run without the corpus has tested very little. |

!!! note "Known gap: one third-party dependency"
    `go.mod` currently requires `github.com/ebitengine/purego`, used only by
    the experimental GPU game pool behind the `gamepool && gamepool_gpu` build
    tags. It is not linked into the engine or the server, but nothing yet
    stops a `require` line from being added. Moving the tag-gated experiments
    into a nested module and adding an architecture test on `require` lines is
    the first item on the [cleanup roadmap](cleanup.md).

## Changing an invariant

Each invariant has a legitimate way to change it (for example, moving a golden
chain head with a named cause, or adding a decision kind with every client
updated in the same change). The contributor-facing version of this page,
with that procedure for each row and the workflow around it, is
[`docs/agents/invariants.md`](https://github.com/adams-shaun/gorge/blob/main/docs/agents/invariants.md).
Deliberate behaviours that can look like bugs are listed in the
[engine contracts spec](https://github.com/adams-shaun/gorge/blob/main/docs/superpowers/specs/2026-09-22-engine-contracts.md).
