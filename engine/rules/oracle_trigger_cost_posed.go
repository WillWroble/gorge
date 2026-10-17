package rules

// xmageAlwaysAsksCost reports a triggered optional cost whose XMage
// DoIfCostPaid always poses its chooseUse, however little gorge could pay: a
// pure mana cost (DoIfCostPaid's own comment: mana costs "are true all the
// time") or a mana cost plus "discard your hand" (DiscardHandCost.canPay is
// unconditionally true). Any other component (sacrifice, tap, exile, life,
// counters, a card-count discard) has a real canPay in XMage, so a cost gorge
// cannot pay is one XMage cannot either and it poses no ask.
func xmageAlwaysAsksCost(c Cost) bool {
	for _, p := range c.Discard {
		if p.Spec != "Hand" {
			return false
		}
	}
	c.Discard = nil
	return c.Priceable() && c.Life == 0
}
