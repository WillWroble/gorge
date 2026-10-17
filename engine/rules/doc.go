// Package rules is the authoritative rules engine: turn structure, priority,
// casting and the stack, combat, triggers, replacement effects, the layer
// system, mana payment and state-based actions. It owns the only state.Game
// a match has (Engine.G) and its event log (Engine.L), and it mutates the
// game exclusively by emitting events (Engine.emit, emit.go), which
// events.Apply folds into state.
//
// Driving a game: New(Config) builds an Engine (genesis.go). Advance runs
// engine work until a decision is needed or the game ends; Pending returns
// the posed decision.Decision, and Submit applies a seat's decision.Intent,
// rejecting anything the engine did not offer. Clone and CloneInto/Release
// fork an engine at an intent boundary; NewHypothetical builds one with a
// fresh chance stream for search.
//
// Where things live: legal.go enumerates what a player may do; cast.go and
// stack.go cast and resolve; combat.go plus the combat subpackage handle
// attacks and blocks; trigger_*.go collect and order triggers; layers*.go
// and the chars subpackage compute characteristics (CR 613); sba.go runs
// state-based actions (CR 704); mayplay*.go grants play permissions. The
// subpackages (chars, combat, cost, pay, resolve, scriptfacts, trigmatch)
// each read the game through a narrow Board interface and never import
// rules; internal/archtest pins that.
//
// Neighbours: card behaviour (api: effects, filters, counts) lives in
// package effects, which reaches the engine only through effects.Host, and
// *Engine implements that interface (host_assert.go). Keyword, trigger,
// static and replacement primitives are implemented here and declared with
// effects.RegisterNonAPI. Seats never see an Engine: they get a view.View
// (package view) and answer through package seat.
//
// The event log is NOT a complete match description by itself. Genesis --
// state.NewGame and the initial AddObject calls that build each player's
// deck into the object arena, both in New -- runs before the log exists and
// legitimately bypasses events. Replaying L.Events alone reconstructs
// everything that happens from that point on (turn structure, zone moves,
// life, damage, priority and so on), but it can never recover deck contents
// or player names: those are never logged. A faithful replay needs the
// original Config together with the log, not the log alone.
package rules
