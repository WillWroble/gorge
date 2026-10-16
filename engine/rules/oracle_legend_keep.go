package rules

import "github.com/adams-shaun/gorge/engine/decision"

// legendKeepSurvivor is the oracle harness's fallback for a CR 704.5j
// legend-rule keep whose duplicate set also holds a member that dies to
// another state-based action of the same batch (CR 704.3: the batch is
// performed simultaneously). The generic fallback keeps option 0, the
// battlefield-order first member; when that member is itself a lethal
// casualty of the batch (a 0-toughness creature, CR 704.5f), keeping it
// loses BOTH objects, which no player chooses. Grub, Notorious Auntie is the
// measured shape: blight 1 makes Grub a 1/0 as the token copy of him enters,
// and the first-option keep sent the surviving token to the graveyard too,
// while XMage, which applies the toughness action before it looks for
// duplicates, never asks and keeps the token. Answering the first member
// that survives the batch is the choice a player makes. Reports that member's
// option index; false when option 0 already survives, when every member
// dies, or when no legend batch is parked behind d.
func legendKeepSurvivor(e *Engine, d *decision.Decision) (int, bool) {
	b := e.legendBatch
	if b == nil || d.Kind != decision.KChoose || len(d.Options) < 2 || d.Options[0].Kind != "keep" {
		return 0, false
	}
	dead := func(o decision.Option) bool {
		for _, c := range b.dead {
			if c.id == o.Obj {
				return true
			}
		}
		return false
	}
	if !dead(d.Options[0]) {
		return 0, false
	}
	for _, o := range d.Options {
		if o.Kind == "keep" && !dead(o) {
			return o.Index, true
		}
	}
	return 0, false
}
