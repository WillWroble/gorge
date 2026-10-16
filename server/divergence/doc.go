// Package divergence holds the record types and comparison predicates of
// the human-divergence capture (design:
// docs/design/2026-10-10-human-divergence-design.md).
//
// A divergence records one place where a human's answer A and a shadow
// policy's answer B to the same decision differ materially; the triage
// record is the offline teacher's verdict over that pair. The package owns
// their JSON shapes, the deterministic JSONL read/write, the materiality
// predicate (Material), the shadow seed derivation (ShadowSeed) and the
// choice-summary renderer shared with cmd/repro.
//
// Layering contract: this is a leaf. Its only gorge imports are decision
// and state; host, rules, internal/testutil and time are forbidden, so
// anything in the engine or server tiers may depend on it without a cycle.
// It owns no clock, no files and no engine state: the JSONL helpers take
// an io.Writer/io.Reader and nothing here mutates a game.
package divergence
