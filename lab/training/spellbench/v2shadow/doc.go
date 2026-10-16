// Package v2shadow is the v2 shadow: at a SpellBench protocol v2 decision
// it rebuilds a gorge engine consistent with what the acting seat can see
// (the decision's neutral observation, spec §6), lets a gorge-native policy
// (sb-tactical, optionally arbitrated by a rollout search over redealt
// worlds) answer there, and maps the gorge answer back onto one of the v2
// candidates. It is the v2 counterpart of the kernel shadow (package kshadow
// on wt/sb-kernel-shadow, design spec D§3.6), whose staging it follows.
//
// Design (spec D§4, D§11.2): the shadow is REBUILT from every observation,
// never replayed: between two decisions of our seat the opponent may act any
// number of times and the seat never sees those decisions as actions, so an
// incremental replay would desynchronise for good at the first divergence.
// Gorge object ids stay stable across rebuilds: a v2 object id keeps the deck
// object it claimed first (Tracker), so a policy that plans across several
// wire decisions (a payment lowering, a combat declaration) sees the same
// objects at every one of them.
//
// Hidden information: SpellBench's rotating pools are mirrors with the
// opponent's list visible (spec §12.2), so both lists are known; the
// opponent's hand and both libraries are dealt uniformly from each list
// minus every card the observation shows (the M1 redeal world, D§5.2).
// Nothing here reads anything but the seat's own message stream.
//
// Every decision the shadow cannot stage or map is answered by the fallback
// policy and counted by reason (Stats): the agent never forfeits, never
// refuses and never loses a legal action.
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
package v2shadow
