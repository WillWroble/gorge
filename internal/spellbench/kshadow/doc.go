// Package kshadow is the kernel shadow: it rebuilds, at every mtg-kernel v1
// decision, a gorge engine consistent with what the acting seat can see
// (the decision's x_kernel_v5 ObservationV5), maps the kernel's candidates
// onto that engine's options, lets a gorge policy (sb-tactical, the
// az-redeal search) answer, and maps the answer back. Any decision it
// cannot stage or map is answered by v1agent.Tactical and counted.
//
// Design (spec D§4.4): the shadow is REBUILT from each observation, never
// replayed incrementally. The seat never sees the opponent's decisions
// (between two of our decisions the opponent may act several times, and the
// kernel's rules differ from gorge's in places), so an incremental replay
// desynchronises for good at the first divergence, while a rebuild bounds a
// divergence to one decision. Everything the observation does not carry
// (hidden zones, durations, delayed triggers, linked exile) is lossy and
// measured by the fidelity check (Fidelity).
//
// Hidden information: SpellBench's rotating pools are deck mirrors with the
// opponent's list visible, so both lists are known. The opponent's hand and
// both libraries are dealt uniformly from each list minus every card the
// observation shows (the M1 redeal world, spec D§5.2); a seed picks the
// deal. Nothing here reads anything but the acting seat's own observation.
//
// Staging goes through events.Emit on the new engine's own log (events.Apply
// is the only mutation, AGENTS.md) and deliberately bypasses the rules
// engine's emit: a staged permanent must not fire its enters trigger again.
// The staged engine is a throwaway hypothetical, never replayable from its
// Config.
//
// Bridge build tag: every other file in this package carries
// //go:build spellbench_bridge. The SpellBench protocol v1/v2 bridge is a
// parked program, so it is excluded from go test ./..., make test and the
// pipeline gates; build or test it with -tags spellbench_bridge
// (make test-spellbench-bridge). This file stays untagged so go list ./...
// and go vet ./... still see the package.
package kshadow
