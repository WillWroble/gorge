package rules

import (
	"slices"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/state"
)

// loyaltyRemovalAPIs are the head/sub-chain APIs whose resolution takes a
// permanent away from its controller (or hurts it), the effect the bot's
// loyalty ranking prices as "removes a threat". ChangeZone and ChangeZoneAll
// are classified separately (abilityRemovesPermanent) because only their
// battlefield-leaving shape is removal; ChangeZoneAll's other shapes (a
// graveyard or library exile) leave every battlefield permanent untouched.
var loyaltyRemovalAPIs = map[string]struct{}{
	"Destroy": {}, "DestroyAll": {}, "DealDamage": {}, "DamageAll": {},
	"Sacrifice": {}, "SacrificeAll": {}, "Fight": {},
}

// abilityLoyaltyFact is decision.Option.Loyalty for an offered activated
// ability: nil for an ability that is not a planeswalker loyalty ability,
// else the loyalty cost the activation settles (net of its [+N] gain and
// [-N] removal), whether it is the walker's marked ultimate, whether its
// effect chain removes a permanent, and whether paying the cost leaves the
// walker at zero. Read from the compiled cost and the printed ability, never
// from the display label, so the bot's ranking cannot drift from what the
// activation charges.
func abilityLoyaltyFact(o *state.Object, ab *cards.SA, c *Cost, loyal bool) *decision.Loyalty {
	if !loyal {
		return nil
	}
	f := &decision.Loyalty{
		Ultimate: strings.EqualFold(strings.TrimSpace(ab.ParamStr(cards.PKUltimate)), "True"),
		Removal:  abilityRemovesPermanent(ab),
		Restart:  abilityRestartsGame(ab),
	}
	for _, part := range c.AddCounter {
		if strings.EqualFold(part.Spec, "LOYALTY") {
			f.Delta += part.N
		}
	}
	for _, part := range c.SubCounter {
		if !strings.EqualFold(part.Spec, "LOYALTY") {
			continue
		}
		if part.Announced {
			f.X = true
			continue
		}
		f.Delta -= part.N
	}
	if !f.X && o != nil {
		f.Dies = int32(o.Counter("LOYALTY"))+f.Delta <= 0
	}
	return f
}

// abilityRemovesPermanent reports whether any link of ab's Sub chain destroys,
// damages, sacrifices, fights or moves a permanent off the battlefield.
func abilityRemovesPermanent(ab *cards.SA) bool {
	for sa, depth := ab, 0; sa != nil && depth < 32; sa, depth = sa.Sub, depth+1 {
		if _, ok := loyaltyRemovalAPIs[sa.API]; ok {
			return true
		}
		switch sa.API {
		case "ChangeZone":
			// The compiled record, not a raw Origin$/Destination$ read: a
			// raw read here would be attributed to every head API's census.
			cz := effects.ChangeZoneOf(sa)
			if cz.OriginMask.Has(state.ZBattlefield) && cz.DestinationKnown && cz.Destination != state.ZBattlefield {
				return true
			}
		case "ChangeZoneAll":
			cza := effects.ChangeZoneAllOf(sa)
			if slices.Contains(cza.Origin, state.ZBattlefield) && cza.Destination != state.ZBattlefield {
				return true
			}
		}
	}
	return false
}

// abilityRestartsGame reports whether any link of ab's Sub chain restarts the
// game (api:RestartGame).
func abilityRestartsGame(ab *cards.SA) bool {
	for sa, depth := ab, 0; sa != nil && depth < 32; sa, depth = sa.Sub, depth+1 {
		if sa.API == "RestartGame" {
			return true
		}
	}
	return false
}
