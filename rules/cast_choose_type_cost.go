package rules

import (
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/rules/pay"
)

// chooseTypeCostAsk poses the ChooseCreatureType<N> cost part ("as an
// additional cost to cast this spell, choose a creature type": Caller of the
// Hunt's cost, Celestial Reunion's optional cost). The choice is the payment
// (CR 601.2h) and has no resource, so it is always payable; it is asked
// before every part that reads it (Celestial Reunion's
// Behold<2/Creature.ChosenType>), over the same creature-type vocabulary the
// "as this enters, choose a creature type" ask offers (creatureTypeOptions,
// CR 205.3m). Only the measured one-type form exists (ParseCost reports any
// other N as unmodelled).
func (e *Engine) chooseTypeCostAsk() bool {
	pc := e.cast
	if pc == nil || pc.cost.ChooseType <= 0 || pc.chooseTypeDone {
		return false
	}
	verb := "cast"
	if pc.isAbility() {
		verb = "activate"
	}
	opts := e.creatureTypeOptions(pc.player, "", "")
	// A Behold part of the chosen type in the same cost (Celestial Reunion's
	// "behold two creatures of that type") keeps only the types it can be
	// paid with: choosing any other could only end in the CR 733.1 reversal.
	for _, part := range pc.cost.Behold {
		if !pay.BeholdChosenTypeOpen(asPayer(e), pc.card, &pc.cost, part) {
			continue
		}
		kept := opts[:0:0]
		for _, o := range opts {
			if pay.BeholdChosenTypeCount(asPayer(e), pc.player, pc.card, part, o.Label) >= int(part.N) {
				o.Index = len(kept)
				kept = append(kept, o)
			}
		}
		opts = kept
	}
	if len(opts) == 0 {
		e.abortCast(pc, "creature type cost no longer payable; cast aborted", true)
		return true
	}
	d := &decision.Decision{Player: pc.player, Kind: decision.KChoose, Min: 1, Max: 1,
		Prompt: "Choose a creature type to " + verb + " " + e.targetName(pc.card), Source: pc.card,
		Options: opts}
	e.choosing = chooseCast
	e.ask(d)
	return true
}

// answerChooseTypeCost records the chosen creature type on the card being
// cast through the existing Choose "type" event: the spell's Card.ChosenType
// filters (the cost's Behold, the resolution's Remembered$Valid
// Card.ChosenType) read it off the spell, and the permanent the spell
// becomes keeps it (events.Apply keeps Chosen* across stack -> battlefield),
// which is what Caller of the Hunt's characteristic-defining P/T counts.
// abortCast restores the prior value.
func (e *Engine) answerChooseTypeCost(pc *pendingCast, chosen []decision.Option) {
	if len(chosen) == 0 || pc.chooseTypeDone {
		return
	}
	pc.chooseTypeDone = true
	if o := e.G.Obj(pc.card); o != nil {
		pc.preChosenType = o.ChosenType
	}
	e.emit(events.Event{Kind: events.Choose, Obj: pc.card, Player: pc.player, Counter: "type", Text: chosen[0].Label})
}
