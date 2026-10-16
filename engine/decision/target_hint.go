package decision

// TargetHints is the packed target hint of one priority option: whether the
// option's FIRST targeting step is a hostile effect (direct damage, or removal
// of kind destroy, exile, graveyard or sacrifice) and how many legal targets
// that step has under the deciding seat's control (or being the seat) versus
// under an opponent's. It is information only: the engine never filters an
// option on it. Zero means "no hint" (no target step, a non-hostile effect, or
// the seat did not ask for hints), and omitempty keeps every option list that
// carries none byte-identical.
//
// Layout: bit 0 present, bit 1 hostile, bits 2-15 own count, bits 16-29 opp
// count, each count saturating at TargetHintMax.
type TargetHints uint32

// TargetHintMax is the saturation value of each count.
const TargetHintMax = 1<<14 - 1

// MakeTargetHints packs a hint.
func MakeTargetHints(own, opp int, hostile bool) TargetHints {
	h := TargetHints(1)
	if hostile {
		h |= 2
	}
	h |= TargetHints(min(max(own, 0), TargetHintMax)) << 2
	h |= TargetHints(min(max(opp, 0), TargetHintMax)) << 16
	return h
}

// Present reports whether the option carries a hint.
func (h TargetHints) Present() bool { return h&1 != 0 }

// Hostile reports whether the first target step is a hostile effect.
func (h TargetHints) Hostile() bool { return h&2 != 0 }

// Own is the count of legal targets controlled by (or being) the deciding seat.
func (h TargetHints) Own() int { return int(h >> 2 & TargetHintMax) }

// Opp is the count of legal targets controlled by opponents.
func (h TargetHints) Opp() int { return int(h >> 16 & TargetHintMax) }
