package effects

import "github.com/adams-shaun/gorge/state"

// rememberCtxOnce records t in the resolution's Remembered set unless the
// body has already remembered it. The dedupe is against the set WITHOUT the
// fire-time event capture (rememberedExcludingCapture): a trigger whose
// referent is the very object a Remember* rider records (Coveted Falcon's
// "when turned face up, target opponent gains control of ... Draw a card for
// each one they gained control of this way", where the Falcon may gain
// itself) seeds that object into Remembered as the capture, and a plain
// membership test would skip the append, leaving Remembered$Amount at zero
// once the readers drop the capture. The capture occurrence and the real one
// are separate multiset entries, exactly as rememberedExcludingCapture
// expects.
func rememberCtxOnce(h Host, c *Ctx, t state.Target) {
	if !targetIn(rememberedExcludingCapture(h, c), t) {
		c.Remembered = append(c.Remembered, t)
	}
}
