// Package encode is mz-gorge's state encoder: the mage fork's StateEncoder
// and FeatureGraph. It builds a state's graph from the dynamic view
// (principle 1 of mz-gorge-design.md) plus each object's printed card data.
// The graph is the network's input and its hash the functional hash.
package encode

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
)

// Node types, FeatureGraph.Node.Type.
const (
	tPlayer      = "PLAYER"
	tZone        = "ZONE"
	tStackObject = "STACK_OBJECT"
	tPermanent   = "PERMANENT"
	tCard        = "CARD"
	tAbility     = "ABILITY"
)

const none = "NONE" // the fork's default edge label

// Hashes are a state's functional and logical hashes (the design doc's
// glossary).
type Hashes struct {
	Functional, Logical uint64
}

// A Decision is what a state is encoded for: processState's decision
// arguments.
type Decision struct {
	Player  state.PlayerID // who decides
	Atom    atomic.Atom    // the atomic decision; its type is ActionEncoder.ActionType
	Choices []int          // the atom's choices, the search's children
	Text    string         // the decision's text
	Source  state.ObjID    // the decision's source object, or 0
	Pile    []state.ObjID  // cards offered only by this decision (a library search, a scry)
}

// An Encoder is StateEncoder: it builds the graph of a state from the view
// of it (principle 1), plus each object's printed card data.
type Encoder struct {
	Me          state.PlayerID // the agent: its player node comes first
	PerfectInfo bool           // see_opponent_hand: encode the other player's hand too

	g    *Graph
	e    *rules.Engine
	v    *view.View
	buf  view.View
	d    Decision
	pd   *decision.Decision
	can  map[abilityRef]bool // the abilities their controllers can activate now
	ids  uint64              // every object in the graph: the logical hash's IDs
	acts []Key
	objs map[state.ObjID]*view.CardView
}

type abilityRef struct {
	obj  state.ObjID
	kind string
	i    int
}

// Encode builds the graph of e's state, at its pending decision pd, from v
// (omniscient). The graph is reused by the next call.
func (x *Encoder) Encode(e *rules.Engine, v *view.View, pd *decision.Decision, d Decision) *Graph {
	if x.g == nil {
		x.g = NewGraph()
		x.objs = map[state.ObjID]*view.CardView{}
		x.can = map[abilityRef]bool{}
	}
	x.g.Reset()
	x.e, x.v, x.pd, x.d, x.ids = e, v, pd, d, 0
	x.index()
	g := x.g

	if step, ok := phaseSteps[v.Step]; ok && v.Turn > 0 {
		x.feature(step, RootKey)
	}
	x.feature(d.Atom.Type.String(), RootKey)
	x.feature(d.Text, RootKey)
	x.node(tCard, "StopChoosing", StopKey, RootKey, none)

	x.node(tZone, "Exile", ExileKey, RootKey, none)
	x.exile()

	for _, p := range x.order() {
		pv := &v.Players[p]
		x.node(tPlayer, pv.Name, playerKey(pv.Name), RootKey, none)
		x.player(pv, playerKey(pv.Name))
	}

	x.node(tZone, "Stack", StackKey, RootKey, none)
	x.stack()

	for _, id := range d.Pile {
		cv := x.objs[id]
		if cv == nil {
			f := x.face(id)
			if f == nil {
				continue
			}
			// A library card: the view projects no library.
			cv = &view.CardView{ID: id, Name: f.Name}
		}
		if x.node(tCard, cv.Name, x.objKey(id), RootKey, "OptionPile") {
			x.card(cv, x.objKey(id))
		}
	}
	if cv := x.objs[d.Source]; cv != nil {
		x.node(tCard, cv.Name, x.objKey(d.Source), RootKey, "DecisionSource")
	}
	x.actions()
	return g
}

// Actions are the graph nodes of d's choices, in order (getActionId): the
// node a policy head scores for each child. A choice has none (false) under
// CHOOSE_NUM and MAKE_CHOICE, which have no prior.
func (x *Encoder) Actions() ([]Key, bool) {
	t := x.d.Atom.Type
	return x.acts, t != atomic.ChooseNum && t != atomic.MakeChoice
}

// actions finds every choice's node, adding one for an action the card data
// has no node for (a keyword's or a granted ability): every candidate must
// be a node.
func (x *Encoder) actions() {
	x.acts = x.acts[:0]
	for _, c := range x.d.Choices {
		k := x.action(c)
		if c >= 0 && slices.Contains(x.acts, k) {
			// Two ways to take one ability (a cast's alternative costs): the
			// fork's are two abilities, so two nodes.
			o := x.pd.Options[c]
			k = Key(Mix64(uint64(k) ^ Hash64(o.Kind+":"+o.Key+":"+o.Mode+":"+strconv.Itoa(o.AltCostIndex)+":"+strconv.Itoa(c))))
			x.node(tAbility, o.Label, k, x.objKey(o.Obj), none)
		}
		x.acts = append(x.acts, k)
	}
}

func (x *Encoder) action(c int) Key {
	d, a := x.pd, x.d.Atom
	if c == atomic.Stop {
		if a.Type == atomic.ChooseUse {
			return UseFalseKey
		}
		return StopKey
	}
	o := d.Options[c]
	switch a.Type {
	case atomic.ChooseUse:
		switch o.Kind {
		case "no", "keep", "decline", "leave":
			return UseFalseKey
		}
		return UseTrueKey
	case atomic.ChooseTarget:
		t := pickTarget(d.Kind, o)
		if t.isPlayer {
			if int(t.player) < len(x.v.Players) {
				return playerKey(x.v.Players[t.player].Name)
			}
			return StopKey
		}
		k := x.objKey(t.obj)
		if !x.g.Has(k) {
			if f := x.face(t.obj); f != nil {
				x.node(tCard, f.Name, k, RootKey, "OptionPile")
			}
		}
		return k
	case atomic.Priority:
		if o.Kind == "pass" {
			return PassKey
		}
		k := x.priorityKey(o)
		if !x.g.Has(k) {
			x.node(tAbility, o.Label, k, x.objKey(o.Obj), none)
		}
		return k
	}
	return 0
}

// priorityKey is a priority option's ability node: a cast its card's spell
// ability, an activation its face ability, a land play its play ability;
// any other action a node of its own on its object.
func (x *Encoder) priorityKey(o decision.Option) Key {
	f := x.face(o.Obj)
	switch {
	case o.Kind == "play_land":
		return x.abilityKey(o.Obj, "land", 0)
	case f != nil && o.Kind == "cast":
		for i, sa := range f.Abilities {
			if sa.Kind == "SP" {
				return x.abilityKey(o.Obj, "ab", i)
			}
		}
	case f != nil && (o.Kind == "ability" || o.Kind == "activate") && o.Ability >= 0 && o.Ability < len(f.Abilities):
		return x.abilityKey(o.Obj, "ab", o.Ability)
	case f != nil && o.Kind == "activate":
		for i, sa := range f.Abilities {
			if sa.API == "Mana" {
				return x.abilityKey(o.Obj, "ab", i)
			}
		}
	}
	return x.abilityKey(o.Obj, o.Kind+":"+o.Keyword+":"+o.Key, o.Ability)
}

// unread are the parts of the view the encoder does not read.
const unread = view.OmitAvailable | view.OmitArchetype | view.OmitAbilityCosts | view.OmitEffectiveCost |
	view.OmitDecision | view.OmitOwnDeck | view.OmitPotential

// State encodes e's state at its pending decision for d, over the
// omniscient view, and returns the graph and its hashes.
func (x *Encoder) State(e *rules.Engine, d Decision) (*Graph, Hashes) {
	pd := e.Pending()
	view.ProjectOmniscientLeanInto(&x.buf, e.G, e, pd, unread)
	g := x.Encode(e, &x.buf, pd, d)
	return g, x.Hashes()
}

// Hashes are the functional hash (the graph's) and the logical one: plus
// every object in the graph and the decision's options, since an action is
// an option index (the design doc's glossary).
func (x *Encoder) Hashes() Hashes {
	f := x.g.Hash()
	l := Mix64(f ^ x.ids)
	if x.pd != nil {
		for _, o := range x.pd.Options {
			l = Mix64(l ^ Hash64(o.Kind) ^ uint64(o.Obj)<<1 ^ uint64(o.Player)<<40 ^ uint64(o.Attacker)<<20)
		}
	}
	return Hashes{Functional: f, Logical: l}
}

// index maps every projected card to its view, and the abilities that can
// be activated now (CanActivate): the potential-mana walk's actions for
// each player.
func (x *Encoder) index() {
	clear(x.objs)
	clear(x.can)
	for i := range x.v.Players {
		pv := &x.v.Players[i]
		for _, zone := range [][]view.CardView{pv.Hand, pv.Battlefield, pv.Graveyard, pv.Exile, pv.Command} {
			for j := range zone {
				x.objs[zone[j].ID] = &zone[j]
			}
		}
		for _, a := range x.e.PotentialActions(state.PlayerID(i)) {
			x.can[abilityRef{a.Obj, a.Kind, a.Ability}] = true
		}
	}
	for i := range x.v.Stack {
		if c := x.v.Stack[i].Card; c != nil {
			x.objs[c.ID] = c
		}
	}
}

// order is the players, the agent first.
func (x *Encoder) order() []state.PlayerID {
	out := []state.PlayerID{x.Me}
	for i := range x.v.Players {
		if p := state.PlayerID(i); p != x.Me {
			out = append(out, p)
		}
	}
	return out
}

// node is StateEncoder.addNode: an object node keyed k whose bucket is its
// type's, with its name as a feature; false if it already existed (only the
// edge is added).
func (x *Encoder) node(typ, name string, k, parent Key, label string) bool {
	if !x.g.Add(typ, k, parent, label) {
		return false
	}
	x.feature(name, k, "name")
	return true
}

// feature is addFeature(name, parent[, edge]).
func (x *Encoder) feature(name string, parent Key, label ...string) {
	if name == "" {
		name = "Null"
	}
	l := none
	if len(label) > 0 {
		l = label[0]
	}
	x.g.Add(name, NameKey(name), parent, l)
}

func (x *Encoder) numeric(name string, n int32, parent Key) {
	x.g.AddNumeric(name, n, parent, none)
}

func (x *Encoder) objKey(id state.ObjID) Key {
	x.ids = Mix64(x.ids ^ uint64(id))
	return Key(Mix64(uint64(id) ^ 0x6a09e667f3bcc909))
}

func playerKey(name string) Key { return NameKey("Player:" + name) }

func zoneKey(zone Key, player Key) Key { return Key(Mix64(uint64(zone) ^ uint64(player))) }

// AbilityKey is the node of ability i of kind kind ("ab" a face ability,
// "kw" a keyword, "trig", "static", "land") on object id: the action node a
// priority option names.
func (x *Encoder) abilityKey(id state.ObjID, kind string, i int) Key {
	return Key(Mix64(uint64(x.objKey(id)) ^ Hash64(kind+":"+strconv.Itoa(i))))
}

// player is processPlayer.
func (x *Encoder) player(pv *view.PlayerView, pk Key) {
	p := pv.ID
	if x.pd != nil && x.pd.Player == p && PaymentWindow(x.pd) {
		x.feature("InPayManaMode", pk)
	}
	if c, ok := x.e.Casting(); ok && c == p {
		x.feature("Activating", pk)
	}
	x.picks(p, pk)
	x.node(tAbility, "PassAbility", PassKey, pk, none)

	if x.v.Active == p {
		x.feature("IsActivePlayer", pk)
	}
	if x.d.Player == p {
		x.feature("IsDecisionPlayer", pk)
	}
	x.numeric("LifeTotal", pv.Life, pk)
	if !pv.LandDropSpent && x.v.Active == p {
		x.feature("CanPlayLand", pk)
	}
	x.numeric("LibrarySize", int32(pv.LibrarySize), pk)

	for i := range x.v.Players {
		for _, cv := range x.v.Players[i].Battlefield {
			if cv.AttachedToPlayer && cv.AttachedPlayer == p {
				x.node(tPermanent, cv.Name, x.objKey(cv.ID), pk, "attachment")
			}
		}
	}
	for _, k := range sortedKeys(pv.Counters) {
		x.numeric(k, pv.Counters[k], pk)
	}
	x.pool(pv, pk)

	gy := zoneKey(GraveyardKey, pk)
	x.node(tZone, "Graveyard", gy, pk, none)
	x.numeric("GraveyardSize", int32(len(pv.Graveyard)), pk)
	for i := range pv.Graveyard {
		cv := &pv.Graveyard[i]
		x.node(tCard, cv.Name, x.objKey(cv.ID), gy, none)
		x.cardInZone(cv, x.objKey(cv.ID), state.ZGraveyard)
	}

	hand := zoneKey(HandKey, pk)
	x.node(tZone, "Hand", hand, pk, none)
	x.numeric("HandSize", int32(pv.HandSize), pk)
	if p == x.d.Player || x.PerfectInfo {
		for i := range pv.Hand {
			cv := &pv.Hand[i]
			x.node(tCard, cv.Name, x.objKey(cv.ID), hand, none)
			x.cardInZone(cv, x.objKey(cv.ID), state.ZHand)
		}
	}

	cz := zoneKey(CommandKey, pk)
	x.node(tZone, "CommandZone", cz, pk, none)
	for _, l := range [][]view.CardView{pv.Command, pv.Commanders} {
		for i := range l {
			cv := &l[i]
			if x.node(tCard, cv.Name, x.objKey(cv.ID), cz, none) {
				x.card(cv, x.objKey(cv.ID))
			}
		}
	}

	x.numeric("SpellsCastThisTurn", pv.SpellsCastThisTurn, pk)
	x.numeric("LifeGainedThisTurn", pv.LifeGainedThisTurn, pk)
	x.numeric("LifeLostThisTurn", pv.LifeLostThisTurn, pk)
	x.numeric("TokensCreatedThisTurn", pv.TokensCreatedThisTurn, pk)

	bf := zoneKey(BattlefieldKey, pk)
	x.node(tZone, "Battlefield", bf, pk, none)
	var perms []*view.CardView
	for i := range x.v.Players {
		for j := range x.v.Players[i].Battlefield {
			if cv := &x.v.Players[i].Battlefield[j]; cv.Controller == p {
				perms = append(perms, cv)
			}
		}
	}
	x.numeric("BattlefieldSize", int32(len(perms)), bf)
	for _, cv := range perms {
		x.node(tPermanent, cv.Name, x.objKey(cv.ID), bf, none)
		x.permanent(cv, x.objKey(cv.ID), p)
	}
}

// picks is processMicroDecisions: the picks staged so far in p's pending
// decision, in their (order-invariant) staged order.
func (x *Encoder) picks(p state.PlayerID, pk Key) {
	d := x.pd
	if d == nil || d.Player != p {
		return
	}
	for i, j := range d.Staged {
		o := d.Options[j]
		switch x.d.Atom.Type {
		case atomic.ChooseTarget:
			x.target(pickTarget(d.Kind, o), pk, "TARGET@"+strconv.Itoa(i))
		case atomic.ChooseNum:
			x.numeric(strconv.Itoa(j), int32(i), pk)
		case atomic.ChooseUse:
			x.numeric(strconv.FormatBool(o.Kind != "no"), int32(i), pk)
		default:
			x.numeric(o.Label, int32(i), pk)
		}
	}
}

// pickTarget is what a staged pick chooses: an attacker its defender, a
// blocker its attacker, else the option's object or player.
func pickTarget(k decision.Kind, o decision.Option) target {
	switch {
	case k == decision.KAttackers && o.Battle != 0:
		return target{obj: o.Battle}
	case k == decision.KAttackers:
		return target{player: o.Player, isPlayer: true}
	case k == decision.KBlockers:
		return target{obj: o.Attacker}
	case o.Obj != 0:
		return target{obj: o.Obj}
	}
	return target{player: o.Player, isPlayer: true}
}

type target struct {
	obj      state.ObjID
	player   state.PlayerID
	isPlayer bool
}

// target is processTarget: an edge to the targeted object's or player's node.
func (x *Encoder) target(t target, parent Key, label string) {
	if t.isPlayer {
		if int(t.player) < len(x.v.Players) {
			name := x.v.Players[t.player].Name
			x.node(tPlayer, name, playerKey(name), parent, label)
		}
		return
	}
	k := x.objKey(t.obj)
	for i := range x.v.Stack {
		if s := &x.v.Stack[i]; s.ID == t.obj {
			x.node(tStackObject, stackName(s), k, parent, label)
			return
		}
	}
	if cv := x.objs[t.obj]; cv != nil {
		typ := tCard
		if o := x.e.G.Obj(t.obj); o != nil && o.Zone == state.ZBattlefield {
			typ = tPermanent
		}
		x.node(typ, cv.Name, k, parent, label)
	}
}

func (x *Encoder) pool(pv *view.PlayerView, pk Key) {
	x.mana(pv.Pool, "", pk)
	for _, r := range pv.PoolRestrictions {
		x.mana(map[string]int32{r.Color: r.Amount}, "_"+r.Text, pk)
	}
}

// mana is processMana: the fork adds every colour, zero included.
func (x *Encoder) mana(m map[string]int32, cond string, pk Key) {
	for _, c := range [...]struct{ sym, name string }{
		{"G", "GreenMana"}, {"R", "RedMana"}, {"U", "BlueMana"}, {"W", "WhiteMana"}, {"B", "BlackMana"}, {"C", "ColorlessMana"},
	} {
		x.numeric(c.name+cond, m[c.sym], pk)
	}
}

// exile is processExile: one zone per exiling object (the fork's exile
// zones), the rest in the main exile zone.
func (x *Encoder) exile() {
	main := NameKey("ExileZone:exile")
	for i := range x.v.Players {
		for j := range x.v.Players[i].Exile {
			cv := &x.v.Players[i].Exile[j]
			zk, name := main, "exile"
			if src := x.e.G.Obj(cv.ID).ExiledWith; src != 0 {
				zk = x.exileZone(src)
				if o := x.objs[src]; o != nil {
					name = o.Name
				}
			}
			x.node(tZone, name, zk, ExileKey, none)
			x.node(tCard, cv.Name, x.objKey(cv.ID), zk, none)
			x.cardInZone(cv, x.objKey(cv.ID), state.ZExile)
		}
	}
}

func (x *Encoder) exileZone(src state.ObjID) Key {
	return Key(Mix64(uint64(x.objKey(src)) ^ 0xbb67ae8584caa73b))
}

// card is processCard: the printed card, and its counters.
func (x *Encoder) card(cv *view.CardView, k Key) {
	for _, c := range sortedKeys(cv.Counters) {
		x.numeric(c, cv.Counters[c], k)
	}
	f := x.face(cv.ID)
	if f == nil {
		return
	}
	if f.IsPermanent() {
		x.feature("PermanentType", k, "static")
	}
	types, subtypes := splitTypes(f.Types)
	for _, t := range types {
		x.feature(t, k, "static")
	}
	x.colors(f.Colors, k, "")
	for _, s := range subtypes {
		x.feature(s, k, "static")
	}
	x.manaCost(f.ManaCost, k)
}

func (x *Encoder) face(id state.ObjID) *cards.Face {
	if o := x.e.G.Obj(id); o != nil {
		return o.Face()
	}
	return nil
}

// colors adds the fork's colour features for a Forge colour word list.
func (x *Encoder) colors(colors string, k Key, label string) {
	n := 0
	for _, c := range [...]struct{ word, name string }{
		{"red", "RedCard"}, {"white", "WhiteCard"}, {"black", "BlackCard"}, {"green", "GreenCard"}, {"blue", "BlueCard"},
	} {
		if strings.Contains(strings.ToLower(colors), c.word) {
			x.featureLabel(c.name, k, label)
			n++
		}
	}
	switch {
	case n == 0:
		x.featureLabel("ColorlessCard", k, label)
	case n > 1:
		x.featureLabel("MultiColored", k, label)
	}
}

func (x *Encoder) featureLabel(name string, k Key, label string) {
	if label == "" {
		x.feature(name, k)
	} else {
		x.feature(name, k, label)
	}
}

// manaCost is processManaCosts: the mana value and each symbol, as the
// fork's ManaCost.getText writes them ({1}, {G}, {X}).
func (x *Encoder) manaCost(cost string, k Key) {
	syms := manaSymbols(cost)
	if len(syms) == 0 {
		return
	}
	x.numeric("ManaValue", manaValue(syms), k)
	for _, s := range syms {
		x.feature(s, k)
	}
}

// cardInZone is processCardInZone: the printed card plus the abilities that
// work in zone z.
func (x *Encoder) cardInZone(cv *view.CardView, k Key, z state.Zone) {
	x.card(cv, k)
	f := x.face(cv.ID)
	if f == nil {
		return
	}
	for i, st := range f.Statics {
		if zoneOf(st.ParamStr(cards.PKEffectZone)) == z {
			ak := x.abilityKey(cv.ID, "static", i)
			x.node(tAbility, staticName(st), ak, k, none)
		}
	}
	if z == state.ZBattlefield {
		for i, kw := range f.Keywords {
			x.node(tAbility, kw, x.abilityKey(cv.ID, "kw", i), k, none)
		}
	}
	for i, sa := range f.Abilities {
		if x.functions(sa, z) {
			ak := x.abilityKey(cv.ID, "ab", i)
			x.node(tAbility, abilityName(sa), ak, k, none)
			x.activated(cv.ID, sa, i, ak)
		}
	}
	if f.IsLand() && (z == state.ZHand || z == state.ZGraveyard || z == state.ZExile) {
		ak := x.abilityKey(cv.ID, "land", 0)
		x.node(tAbility, "Play "+f.Name, ak, k, none)
		if x.can[abilityRef{cv.ID, "play_land", 0}] {
			x.feature("CanActivate", ak)
		}
	}
	for i, t := range f.Triggers {
		if zoneOf(t.ParamStr(cards.PKTriggerZones)) == z {
			ak := x.abilityKey(cv.ID, "trig", i)
			x.node(tAbility, triggerName(t), ak, k, none)
			x.ability(t.Effect, ak)
			x.feature(t.Mode, ak)
		}
	}
}

// functions reports whether face ability sa can be activated in zone z: a
// spell from hand (and, as the fork does, from a graveyard or exile, where a
// permission may let it be cast), an activated ability in its activation zone.
func (x *Encoder) functions(sa *cards.SA, z state.Zone) bool {
	if sa.Kind == "SP" {
		return z == state.ZHand || z == state.ZGraveyard || z == state.ZExile || z == state.ZStack
	}
	return sa.Kind == "AB" && zoneOf(sa.ParamStr(cards.PKActivationZone)) == z
}

// activated is processActivatedAbility.
func (x *Encoder) activated(id state.ObjID, sa *cards.SA, i int, ak Key) {
	x.ability(sa, ak)
	if sa.API == "Mana" {
		x.feature("ManaAbility", ak)
	}
	kind := "ability"
	if sa.Kind == "SP" {
		kind = "cast"
	} else if sa.API == "Mana" {
		kind = "activate"
	}
	if x.can[abilityRef{id, kind, i}] || (kind != "ability" && x.can[abilityRef{id, kind, 0}]) {
		x.feature("CanActivate", ak)
	}
}

// ability is processAbility: its costs and each effect's text, down the
// sub-ability chain.
func (x *Encoder) ability(sa *cards.SA, ak Key) {
	if sa == nil {
		return
	}
	if cost := sa.ParamStr(cards.PKCost); cost != "" {
		x.costs(cost, ak)
	}
	for s := sa; s != nil; s = s.Sub {
		x.feature(effectText(s), ak)
	}
}

// costs is processCosts over a Forge cost line: its mana symbols as mana
// costs, every other part as a cost's text.
func (x *Encoder) costs(cost string, ak Key) {
	var mana []string
	for _, part := range strings.Fields(cost) {
		if isManaPart(part) {
			mana = append(mana, "{"+strings.ToUpper(part)+"}")
		} else {
			x.feature(costText(part), ak)
		}
	}
	if len(mana) > 0 {
		x.numeric("ManaValue", manaValue(mana), ak)
		for _, s := range mana {
			x.feature(s, ak)
		}
	}
}

// permanent is processPermBattlefield.
func (x *Encoder) permanent(cv *view.CardView, k Key, p state.PlayerID) {
	x.cardInZone(cv, k, state.ZBattlefield)
	name := cv.CardName
	if name == "" {
		name = cv.Name
	}
	x.feature(name, k, "static")
	if cv.Tapped {
		x.feature("Tapped", k)
	}
	types, subtypes := splitTypes(strings.Fields(cv.Types))
	for _, t := range types {
		x.feature(t, k)
	}
	for _, s := range subtypes {
		x.feature(s, k)
	}
	if f := x.face(cv.ID); f != nil {
		x.colors(f.Colors, k, "static")
	}
	// Granted keywords: the dynamic abilities (getAbilities(game)).
	for _, kw := range cv.Keywords {
		x.node(tAbility, kw, Key(Mix64(uint64(k)^Hash64("granted:"+kw))), k, none)
	}

	for i := range x.v.Players {
		for _, a := range x.v.Players[i].Battlefield {
			if a.AttachedTo == cv.ID && !a.AttachedToPlayer {
				x.node(tPermanent, a.Name, x.objKey(a.ID), k, "attachment")
			}
		}
	}
	for _, id := range cv.Imprinted {
		if o := x.objs[id]; o != nil {
			x.node(tCard, o.Name, x.objKey(id), k, "imprinted")
		}
	}
	if o := x.objs[cv.Paired]; cv.Paired != 0 && o != nil {
		x.node(tCard, o.Name, x.objKey(cv.Paired), k, "paired")
	}
	if len(cv.ExiledCards) > 0 {
		x.node(tZone, cv.Name, x.exileZone(cv.ID), k, "exiled")
	}

	if !strings.Contains(cv.Types, "Creature") {
		return
	}
	if cv.SummonSick {
		x.feature("SummoningSick", k)
	}
	if x.e.CanAttack(cv.ID) {
		x.feature("CanAttack", k)
	}
	if x.e.CanBlockAny(cv.ID) {
		x.feature("CanBlock", k)
	}
	if cv.Attacking {
		x.feature("Attacking", k)
		if cv.AttackingPlayer != nil {
			x.target(target{player: *cv.AttackingPlayer, isPlayer: true}, k, "defender")
		} else if b := x.e.G.Obj(cv.ID).AttackingBattle; b != 0 {
			x.target(target{obj: b}, k, "defender")
		}
		for _, b := range cv.BlockedBy {
			if o := x.objs[b]; o != nil {
				x.node(tPermanent, o.Name, x.objKey(b), k, "blocker")
			}
		}
	}
	x.numeric("Damage", cv.Damage, k)
	x.numeric("Power", cv.Power, k)
	x.numeric("Toughness", cv.Toughness, k)
}

// stack is processStack: top first.
func (x *Encoder) stack() {
	x.numeric("StackSize", int32(len(x.v.Stack)), StackKey)
	for i := range x.v.Stack {
		s := &x.v.Stack[len(x.v.Stack)-1-i]
		k := x.objKey(s.ID)
		x.node(tStackObject, stackName(s), k, StackKey, none)
		x.numeric("Depth", int32(i+1), k)
		x.stackObject(s, k)
	}
}

// stackObject is processStackObject.
func (x *Encoder) stackObject(s *view.StackView, k Key) {
	if s.Controller == x.Me {
		x.feature("isController", k)
	}
	src := s.Source
	if s.Card != nil {
		src = s.Card.ID
	}
	if cv := x.objs[src]; cv != nil {
		if x.node(tCard, cv.Name, x.objKey(src), k, none) {
			x.cardInZone(cv, x.objKey(src), state.ZStack)
		}
	}
	x.feature(s.Text, k)
	for i, t := range s.Targets {
		x.target(target{obj: t.Obj, player: t.Player, isPlayer: t.IsPlayer}, k, "TARGET@"+strconv.Itoa(i))
	}
	x.numeric("Kicks", s.Kicks, k)
	for _, m := range s.Modes {
		x.feature(m, k)
	}
	x.numeric("XValue", s.X, k)
}

func stackName(s *view.StackView) string {
	if s.Text != "" {
		return s.Name + ": " + s.Text
	}
	return s.Name
}

// phaseSteps are gorge's step names as the fork's PhaseStep names.
var phaseSteps = map[string]string{
	"untap": "UNTAP", "upkeep": "UPKEEP", "draw": "DRAW", "main1": "PRECOMBAT_MAIN",
	"begin-combat": "BEGIN_COMBAT", "declare-attackers": "DECLARE_ATTACKERS", "declare-blockers": "DECLARE_BLOCKERS",
	"combat-damage": "COMBAT_DAMAGE", "end-combat": "END_COMBAT", "main2": "POSTCOMBAT_MAIN",
	"end": "END_TURN", "cleanup": "CLEANUP",
}

// cardTypes are the fork's CardType names; supertypes are not types there.
var (
	cardTypes = map[string]string{
		"Artifact": "ARTIFACT", "Battle": "BATTLE", "Conspiracy": "CONSPIRACY", "Creature": "CREATURE",
		"Dungeon": "DUNGEON", "Enchantment": "ENCHANTMENT", "Instant": "INSTANT", "Kindred": "KINDRED",
		"Tribal": "KINDRED", "Land": "LAND", "Phenomenon": "PHENOMENON", "Plane": "PLANE",
		"Planeswalker": "PLANESWALKER", "Scheme": "SCHEME", "Sorcery": "SORCERY", "Vanguard": "VANGUARD",
	}
	superTypes = map[string]bool{"Basic": true, "Legendary": true, "Snow": true, "World": true, "Ongoing": true, "Elite": true, "Host": true}
)

// splitTypes splits a type line into the fork's card type and subtype names
// (SubType's constant names: ELF, TIME_LORD).
func splitTypes(words []string) (types, subtypes []string) {
	for _, w := range words {
		switch {
		case w == "-" || w == "—" || superTypes[w]:
		case cardTypes[w] != "":
			types = append(types, cardTypes[w])
		default:
			subtypes = append(subtypes, subtypeName(w))
		}
	}
	return types, subtypes
}

var subtypeNames sync.Map // gorge type word → SubType constant name

// subtypeName is a type word as the fork's SubType constant name.
func subtypeName(w string) string {
	if n, ok := subtypeNames.Load(w); ok {
		return n.(string)
	}
	n := strings.ToUpper(subtypeReplacer.Replace(w))
	subtypeNames.Store(w, n)
	return n
}

var subtypeReplacer = strings.NewReplacer("-", "_", "'", "", " ", "_")

// manaSymbols splits a Forge mana cost ("1 G G", "X R", "W/U") into the
// fork's symbol texts ("{1}", "{G}", "{X}", "{W/U}").
func manaSymbols(cost string) []string {
	if cost == "" || cost == "no cost" {
		return nil
	}
	var out []string
	for _, part := range strings.Fields(cost) {
		if !isManaPart(part) {
			continue
		}
		out = append(out, "{"+strings.ToUpper(part)+"}")
	}
	return out
}

func isManaPart(p string) bool {
	if p == "" {
		return false
	}
	for _, c := range p {
		if !strings.ContainsRune("0123456789WUBRGCXSP/", c) {
			return false
		}
	}
	return true
}

// manaValue is the mana value of symbol texts: a number counts its value, X
// nothing, a hybrid with a number its number, everything else one.
func manaValue(syms []string) int32 {
	var n int32
	for _, s := range syms {
		body := strings.Trim(s, "{}")
		switch {
		case body == "X":
		case strings.Contains(body, "/"):
			if v, err := strconv.Atoi(strings.SplitN(body, "/", 2)[0]); err == nil {
				n += int32(v)
			} else {
				n++
			}
		default:
			if v, err := strconv.Atoi(body); err == nil {
				n += int32(v)
			} else {
				n++
			}
		}
	}
	return n
}

// costText is a Forge non-mana cost part as the fork's cost text.
func costText(part string) string {
	switch part {
	case "T":
		return "{T}"
	case "Q":
		return "{Q}"
	}
	return part
}

// zoneOf reads a Forge zone parameter (default: the battlefield).
func zoneOf(z string) state.Zone {
	switch strings.SplitN(z, ",", 2)[0] {
	case "Hand":
		return state.ZHand
	case "Graveyard":
		return state.ZGraveyard
	case "Exile":
		return state.ZExile
	case "Library":
		return state.ZLibrary
	case "Command":
		return state.ZCommand
	case "Stack":
		return state.ZStack
	}
	return state.ZBattlefield
}

func abilityName(sa *cards.SA) string {
	if d := sa.ParamStr(cards.PKSpellDescription); d != "" {
		return d
	}
	return sa.Line
}

func effectText(sa *cards.SA) string {
	if d := sa.ParamStr(cards.PKSpellDescription); d != "" {
		return d
	}
	if d := sa.ParamStr(cards.PKStackDescription); d != "" {
		return d
	}
	return sa.API
}

func staticName(st cards.Static) string {
	if d := st.ParamStr(cards.PKDescription); d != "" {
		return d
	}
	return st.Mode
}

func triggerName(t cards.Trigger) string {
	if d := t.ParamStr(cards.PKTriggerDescription); d != "" {
		return d
	}
	return t.Mode
}

// PaymentWindow reports a mana payment window: a choose that activates mana
// sources or spends mana until done (the fork's isInPayManaMode).
func PaymentWindow(d *decision.Decision) bool {
	if d.Kind != decision.KChoose {
		return false
	}
	pay, done := false, false
	for _, o := range d.Options {
		pay = pay || o.Kind == "activate" || o.Kind == "mana"
		done = done || o.Kind == "done"
	}
	return pay && done
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
