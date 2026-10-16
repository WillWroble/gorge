// Package effects implements the primitives Forge card scripts reference:
// the api: effects (DealDamage, Draw, ChangeZone, Pump, Token, ...), the
// filter language that matches cards and players (ValidTgts$, Defined$,
// Affected$), and the count: value expressions (Count$..., SVar amounts).
// It reaches the engine only through the Host interface, so it never imports
// package rules and the dependency graph stays acyclic.
//
// Registration: each effect is an Effect, func(h Host, c *Ctx, sa
// *cards.SA), installed by Register("Name", fn) from an init() in this
// package (registry.go). Keyword, trigger, static and replacement primitives
// are implemented in package rules and only declared here, through
// RegisterNonAPI("kw:Flying", ...). Supported returns the whole implemented
// set in the prefixed form cards.Face.Primitives uses, which is what card
// coverage (cards.Registry.Coverage, `make report`) is measured against.
//
// Resolution: rules calls Resolve(h, ctx, sa), which walks the ability and
// its SubAbility$ chain, checks each node's conditions and dispatches to the
// registered Effect. An Effect reads the game through Host (HostRead) and
// changes it only by proposing events (HostEmit's Emit and friends;
// host_roles.go); it never writes a state.Game field. An effect that needs
// more than Host offers is doing rules work that belongs in package rules.
//
// Main entry points besides those: MatchesSpec and MatchesSpecFrom
// (filter.go) evaluate a card filter; EvalCount (count.go) evaluates a value
// expression; modelledValueHeads (value_heads.go) lists the count: heads the
// evaluator models. The compiled parameter records live in the leaf
// subpackage effects/params.
//
// Neighbours: cards (the IR an Effect reads), state and events (what it
// reads and proposes), rules (the Host implementation and every non-api
// primitive).
package effects
