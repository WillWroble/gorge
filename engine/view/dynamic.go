package view

import (
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// ProjectDynamic is ProjectForControlledFor plus the dynamic state no other
// projection carries: Status and Links on every projected card, stack object
// and player, and the game's. Under Omniscient visibility it also copies the
// game's dynamic records (View.Delayed and the rest) and the engine's own
// state (View.Engine, when ch offers EngineState). It is the view
// mz-gorge's bot reads; every other caller's projection is unchanged.
func ProjectDynamic(g *state.Game, ch Chars, viewer state.PlayerID, vis Visibility, alsoVisible []state.PlayerID, d *decision.Decision) View {
	v := ProjectForControlledFor(g, ch, viewer, vis, alsoVisible, d)
	if g == nil {
		return v
	}
	v.Status = gameFacts(g)
	if vis == Omniscient {
		v.Delayed = slices.Clone(g.Delayed)
		v.ExtraTurnQueue = slices.Clone(g.ExtraTurnQueue)
		v.ExtraPhases = slices.Clone(g.ExtraPhases)
		v.Entered = slices.Clone(g.Entered)
		v.ExcessDamageVictims = slices.Clone(g.ExcessDamageVictims)
		if es, ok := ch.(interface {
			EngineState(visit func(field string, v reflect.Value))
		}); ok {
			v.Engine = map[string]string{}
			es.EngineState(func(field string, x reflect.Value) {
				var b strings.Builder
				stateText(&b, x, 0)
				v.Engine[field] = b.String()
			})
		}
	}
	derived, _ := ch.(interface{ Derived(state.ObjID) effects.Chars })
	card := func(cv *CardView) {
		o := g.Obj(cv.ID)
		if o == nil {
			return
		}
		cv.Status, cv.Links = objectFacts(o)
		if derived != nil && (o.Zone == state.ZBattlefield || o.Zone == state.ZStack) {
			cv.Status = derivedFacts(cv.Status, derived.Derived(cv.ID))
		}
	}
	for i := range v.Players {
		pv := &v.Players[i]
		if int(pv.ID) < len(g.Players) {
			pv.Status, pv.Links = playerFacts(&g.Players[pv.ID])
		}
		for _, l := range [][]CardView{pv.Hand, pv.Battlefield, pv.Graveyard, pv.Exile, pv.Command, pv.Commanders, pv.PlanarDeck, pv.Library} {
			for j := range l {
				card(&l[j])
			}
		}
		if pv.LibraryTop != nil {
			card(pv.LibraryTop)
		}
	}
	for i := range v.Stack {
		sv := &v.Stack[i]
		if sv.Kind != "spell" {
			if o := g.Obj(sv.ID); o != nil {
				sv.Status, sv.Links = objectFacts(o)
			}
		}
		if sv.Card != nil {
			card(sv.Card)
		}
	}
	return v
}

// Status is the dynamic state of an object or player that has no typed view
// field: per-turn history, designations, cast provenance, choices, notes.
// Numeric facts keep their value, true facts are 1, and a string-valued fact
// is the key "name:value" with value 1. Zero facts are absent, so the map is
// nil for most objects. With Links it completes principle 1 of
// mz-gorge-design.md: everything dynamic is in the view.
type Status map[string]int32

// Links is the dynamic state of an object or player that names other objects,
// in the order the game recorded them.
type Links map[string][]state.ObjID

// facts builds a Status and Links pair, allocating only when a fact is set.
type facts struct {
	s Status
	l Links
}

func (f *facts) num(k string, v int32) {
	if v == 0 {
		return
	}
	if f.s == nil {
		f.s = Status{}
	}
	f.s[k] += v
}

func (f *facts) flag(k string, b bool) {
	if b {
		f.num(k, 1)
	}
}

func (f *facts) str(k, v string) {
	if v != "" {
		f.num(k+":"+v, 1)
	}
}

func (f *facts) seat(k string, p state.PlayerID, ok bool) {
	if ok {
		f.num(k+":"+strconv.Itoa(int(p)), 1)
	}
}

func (f *facts) ids(k string, ids ...state.ObjID) {
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if f.l == nil {
			f.l = Links{}
		}
		f.l[k] = append(f.l[k], id)
	}
}

func (f *facts) targets(k string, ts []state.Target) {
	for _, t := range ts {
		if t.IsPlayer {
			f.seat(k+"_player", t.Player, true)
		} else {
			f.ids(k, t.Obj)
		}
	}
}

func (f *facts) mana(k string, m state.Mana) {
	for i, n := range m {
		f.num(k+":"+strconv.Itoa(i), n)
	}
}

// objectFacts is o's dynamic state beyond CardView's and StackView's typed
// fields. TestViewCoversState pins that every state.Object field is either
// typed in the view, read here, or not game state.
func objectFacts(o *state.Object) (Status, Links) {
	var f facts
	f.num("face_idx", int32(o.FaceIdx))
	if o.CopyFace != nil {
		f.str("copy_of", o.CopyFace.Name)
	}
	f.flag("phased_out", o.PhasedOut)
	f.num("class_level", int32(o.ClassLevelValue))
	f.str("at_eot_trigger", o.AtEOTTrigBody)
	for _, m := range o.MergedCards {
		f.ids("merged", m.Obj)
	}

	// Per-turn and per-game history.
	if o.EnteredThisTurn {
		f.str("entered_this_turn_from", o.EnteredFrom.String())
	}
	f.flag("was_dealt_damage_this_turn", o.WasDealtDamageThisTurn)
	f.flag("was_dealt_excess_damage_this_turn", o.WasDealtExcessDamageThisTurn)
	f.num("damage_received_this_turn", o.DamageReceivedThisTurn)
	for _, r := range o.DamageDealtThisTurn {
		k := "dealt_damage_this_turn"
		if r.Combat {
			k = "dealt_combat_damage_this_turn"
		}
		f.ids(k, r.Recipient)
		f.num(k+":"+strconv.Itoa(int(r.Recipient)), r.Amount)
	}
	f.ids("damage_taken_this_game_by", o.DamageTakenByGame...)
	f.ids("damage_taken_this_turn_by", o.DamageTakenThisTurnBy...)
	f.flag("dealt_damage_this_game", o.DealtDamageToAnyGame)
	f.flag("dealt_combat_damage_this_game", o.DealtCombatDamageToAnyGame)
	f.num("acquired_turn", o.AcqTurn)
	if o.AcqTurn != 0 {
		f.str("acquired_step", o.AcqStep.String())
	}
	f.num("attacks_this_turn", o.AttacksThisTurn)
	f.flag("exerted_this_turn", o.ExertedThisTurn)
	f.flag("exert_skip_untap", o.ExertSkipUntap)
	f.flag("cant_untap_next_step", o.CantUntapNextStep)
	f.num("enlisted_turn", o.EnlistedTurn)
	f.num("enlisted_combat", o.EnlistedCombat)
	f.num("attacked_turn", o.AttackedTurn)
	f.num("attacked_combat", o.AttackedCombat)
	f.ids("crewed", o.CrewedVehicles...)
	f.num("crewed_turn", o.CrewedTurn)
	if o.HasLatestCast {
		f.str("latest_cast_from", o.LatestCastFrom.String())
		f.seat("latest_cast_by", o.LatestCastBy, true)
	}
	if o.HasPreStackEntry {
		f.flag("pre_stack_entry_this_turn", o.PreStackEntryThisTurn)
		f.str("pre_stack_entry_from", o.PreStackEntryFrom.String())
	}
	f.num("pre_stack_entered_len", int32(o.PreStackEnteredLen))
	f.num("incarnation", int32(o.Incarnation))
	f.num("timestamp", int32(o.Timestamp))
	f.num("drawn_turn", o.DrawnTurn)
	f.ids("blocked_by_this_turn", o.BlockedByThisTurn...)
	f.ids("blocked_this_turn", o.BlockedThisTurn...)

	// Stack provenance.
	f.num("source_incarnation", int32(o.SourceIncarnation))
	if o.PushSourceStamped {
		f.flag("push_source_stamped", true)
		f.flag("push_source_on_battlefield", o.PushSourceOnBattlefield)
		f.num("push_source_incarnation", int32(o.PushSourceIncarnation))
	}
	f.ids("granted_by", o.GrantedBy)
	f.targets("sub_target", o.SubTargets)
	for _, s := range o.TargetStamps {
		f.num("target_incarnation:"+strconv.Itoa(int(s.Obj)), int32(s.Incarnation))
	}
	f.targets("remembered", o.Remembered)

	// Combat.
	f.ids("attacking_object", o.AttackingBattle)
	f.flag("blocking", o.IsBlocking)
	if o.EncoreAttackTurn != 0 {
		f.num("encore_attack_turn", o.EncoreAttackTurn)
		f.seat("encore_attack_defender", o.EncoreAttackDefender, true)
	}
	for _, g := range o.Goads {
		f.seat("goaded_by", g.Player, true)
		f.ids("goad_source", g.Source)
		f.str("goad_duration", g.Duration)
	}

	// Designations.
	f.flag("prepared", o.Prepared)
	f.ids("prepared_source", o.PreparedSource)
	f.num("saddled_turn", o.SaddledTurn)
	f.flag("wont_phase_in_normally", o.WontPhaseInNormal)
	f.flag("suspend_granted", o.SuspendGranted)
	f.num("plotted_turn", o.PlottedTurn)

	// Cast provenance.
	f.num("x", o.X)
	for i := 0; i < 64; i++ {
		if o.CastFlags&(1<<i) != 0 {
			f.str("cast", events.FlagsString(1<<i))
		}
	}
	f.num("replicate_times", o.ReplicateTimes)
	f.num("squad_paid", o.SquadPaid)
	f.flag("offspring_paid", o.OffspringPaid)
	f.flag("optional_cost_paid", o.OptionalCostPaid)
	f.num("converge_colours", o.ConvergeColours)
	f.num("times_kicked", o.TimesKicked)
	f.flag("conspired", o.Conspired)
	f.flag("teamwork_paid", o.TeamworkPaid)
	f.ids("convoked", o.Convoked...)
	f.ids("mana_spent_source", o.ManaSpentSources...)
	f.num("mana_spent", o.ManaSpent)
	f.num("mana_spent_snow", o.ManaSnowSpent)
	f.num("mana_spent_treasure", o.ManaTreasureSpent)
	f.num("mana_spent_cave", o.ManaCaveSpent)
	f.num("mana_spent_desert", o.ManaDesertSpent)
	f.num("mana_spent_artifact", o.ManaArtifactSpent)
	f.mana("mana_spent_colour", o.ManaColorSpent)
	for _, g := range o.ManaAddsCounterGrants {
		f.num("mana_adds_counter:"+g.Filter+":"+g.Kind+":"+g.Amount, g.Count)
	}
	f.num("compleated_life_paid", o.CompleatedLifePaid)
	f.num("noted_number", o.NotedNumber)
	f.flag("cast_battlefield_snapshot", o.CastBattlefield != nil)

	// Choices and notes.
	for k, v := range o.RuntimeSVars { // a map, but Status is order-free
		f.num("svar:"+k, v)
	}
	for _, n := range o.Notes {
		f.str("note", n)
	}
	f.str("chosen_name", o.ChosenName)
	f.str("chosen_type", o.ChosenType)
	f.num("chosen_number", o.ChosenNumber)
	f.str("chosen_color", o.ChosenColor)
	f.str("riot_choice", o.RiotChoice)
	f.str("untap_choice", o.UntapChoice)
	f.str("unleash_choice", o.UnleashChoice)
	f.seat("gift_promised_to", o.GiftPromisedTo, o.CastFlags&state.FlagPromisedGift != 0)
	f.seat("protector", o.Protector, o.ProtectorValid)
	if o.SneakDefenderValid {
		f.seat("sneak_defender", o.SneakDefender, true)
		f.ids("sneak_defender_object", o.SneakDefenderObject)
	}
	f.str("last_noted_mana", o.LastNotedMana)
	for _, k := range o.IntrinsicKeywords {
		f.str("intrinsic_keyword", k)
	}
	if o.Chosen != nil {
		f.flag("chosen_answered", true)
		f.targets("chosen", o.Chosen)
	}
	if o.ETBCloneChoiceValid {
		f.flag("etb_clone_answered", true)
		f.ids("etb_clone_choice", o.ETBCloneChoice)
	}
	for _, m := range o.ModeChoices {
		f.num("mode_choice:"+m.Scope+":"+m.Mode+":"+strconv.Itoa(int(m.Turn)), m.Combat+1)
	}
	f.num("current_combat_turn", o.CurCombatTurn)
	f.num("current_combat", o.CurCombatCombat)

	// Associations.
	f.ids("encoded", o.EncodedCards...)
	f.ids("imprinted_token", o.ImprintTokens...)
	f.ids("seek_found", o.SeekFound...)
	for _, r := range o.ExileReturn {
		f.ids("exile_until_leaves", r.Obj)
		f.str("exile_until_leaves_from:"+strconv.Itoa(int(r.Obj)), r.From.String())
	}
	f.num("times_mutated", o.TimesMutated)
	f.ids("last_bearer", o.LastBearer)
	f.ids("exiled_with", o.ExiledWith)
	f.seat("may_look", o.MayLookPlayer, o.HasMayLook)
	f.str("face_down_set_type", o.FaceDownSetType)
	if o.FaceDownHasPT {
		f.num("face_down_power", o.FaceDownPower)
		f.num("face_down_toughness", o.FaceDownToughness)
		f.flag("face_down_has_pt", true)
	}
	f.ids("melded_with", o.MeldedWith)
	f.flag("myriad", o.IsMyriad)
	f.flag("copy_may_choose_target", o.CopyMayChooseTarget)
	f.flag("copy_non_legendary", o.CopyNonLegendary)
	if o.CopyLoyaltySet {
		f.num("copy_loyalty", o.CopyLoyalty)
		f.flag("copy_loyalty_set", true)
	}
	f.flag("copy_gain_this_ability", o.CopyGainThisAbility)
	if o.GainedFace != nil {
		f.str("gained_face", o.GainedFace.Name)
	}
	f.ids("gained_from", o.GainedFrom)
	f.flag("unlocked", o.Unlocked)
	f.flag("cast_door", o.CastDoor)
	f.num("locked_doors", int32(o.LockedDoors))
	return f.s, f.l
}

// derivedFacts adds a permanent's or spell's layer-derived characteristics
// that CardView has no typed field for (CR 613): colours, base P/T, a
// grant of every creature type, and changed rules text.
func derivedFacts(s Status, d effects.Chars) Status {
	f := facts{s: s}
	for _, c := range d.Colors {
		f.num("color:"+string(c), 1)
	}
	f.num("base_power", d.BasePower)
	f.num("base_toughness", d.BaseToughness)
	f.flag("all_creature_types", d.AllCreatureTypes)
	f.str("text", d.Text)
	return f.s
}

// playerFacts is p's dynamic state beyond PlayerView's typed fields.
func playerFacts(p *state.Player) (Status, Links) {
	var f facts
	f.num("lands_played", p.LandsPlayed)
	f.flag("drew_from_empty", p.DrewFromEmpty)
	f.num("coin_flips_this_turn", p.CoinFlipsThisTurn)
	for d := 0; d < 64; d++ {
		f.flag("attacked_player_this_turn:"+strconv.Itoa(d), p.AttackedPlayersThisTurn&(1<<d) != 0)
		f.flag("attacked_planeswalker_this_turn:"+strconv.Itoa(d), p.AttackedPlaneswalkersThisTurn&(1<<d) != 0)
	}
	f.num("last_upkeep_turn", p.LastUpkeepTurn)
	f.mana("pool_snow", p.Snow)
	for i, m := range p.TypedMana {
		f.mana("pool_"+state.TypedManaTags[i], m)
	}
	for i, m := range p.ArtifactTyped {
		f.mana("pool_artifact_"+state.TypedManaTags[i], m)
	}
	f.mana("pool_persistent", p.PersistentMana)
	f.mana("pool_combat", p.CombatMana)
	f.ids("damage_taken_this_game_by", p.DamageTakenByGame...)
	f.flag("dungeon_completed", p.DungeonCompleted)
	f.num("speed", p.Speed)
	f.num("ring_tempted", p.RingTempted)
	f.flag("blessing", p.Blessing)
	f.flag("enduring_story", p.EnduringStory)
	for _, n := range p.Notes {
		f.str("note", n)
	}
	return f.s, f.l
}

// gameFacts is g's dynamic state beyond View's typed fields; the records it
// keeps as lists are on View directly.
func gameFacts(g *state.Game) Status {
	var f facts
	f.num("passes", g.Passes)
	f.seat("starting_player", g.StartingPlayer, g.HasStartingPlayer)
	for p, n := range g.ExtraTurns { // maps, but Status is order-free
		f.num("extra_turns:"+strconv.Itoa(int(p)), int32(n))
	}
	for p, n := range g.SkipTurns {
		f.num("skip_turns:"+strconv.Itoa(int(p)), int32(n))
	}
	f.num("combats_this_turn", g.CombatsThisTurn)
	f.num("end_steps_this_turn", g.EndStepsThisTurn)
	for k, n := range g.ResolvedThisTurn {
		f.num("resolved_this_turn:"+k, n)
	}
	f.seat("monarch", g.Monarch, g.HasMonarch)
	f.seat("turn_start_monarch", g.TurnStartMonarch, g.HasTurnStartMonarch)
	for p, c := range g.ControlledBy {
		f.num("controlled_by:"+strconv.Itoa(int(p)), int32(c)+1)
	}
	for p, t := range g.ControlArmedTurn {
		f.num("control_armed_turn:"+strconv.Itoa(int(p)), t)
	}
	f.num("next_id", int32(g.NextID))
	f.num("delayed_next", int32(g.DelayedNext))
	f.num("clock", int32(g.Clock))
	return f.s
}

// opaqueID reports whether a field names rather than carries state: an
// identifier the engine derives for an offered payment (a payment action's
// or plan's id), or a position in the event log (Seq, SourceZoneSeq), which
// is history.
func opaqueID(t reflect.Type, field string) bool {
	return field == "ActionID" || strings.HasSuffix(field, "Seq") ||
		(field == "ID" && t == reflect.TypeOf(decision.PaymentPlan{}))
}

// stateText writes v as deterministic text: struct fields by name, map
// entries sorted, pointers followed, card data named rather than expanded.
func stateText(b *strings.Builder, v reflect.Value, depth int) {
	if depth > 12 {
		b.WriteString("…")
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, 64))
	case reflect.String:
		b.WriteString(strconv.Quote(v.String()))
	case reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		stateText(b, v.Elem(), depth+1)
	case reflect.Pointer:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		switch v.Type().Elem() {
		case reflect.TypeOf(cards.SA{}):
			b.WriteString("sa:" + strconv.Quote(v.Elem().FieldByName("Line").String()))
		case reflect.TypeOf(cards.Card{}):
			b.WriteString("card:" + strconv.Quote(v.Elem().FieldByName("Path").String()))
		case reflect.TypeOf(cards.Face{}):
			b.WriteString("face:" + strconv.Quote(v.Elem().FieldByName("Name").String()))
		case reflect.TypeOf(state.Game{}):
			b.WriteString("game-snapshot")
		default:
			stateText(b, v.Elem(), depth+1)
		}
	case reflect.Slice, reflect.Array:
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(' ')
			}
			stateText(b, v.Index(i), depth+1)
		}
		b.WriteByte(']')
	case reflect.Map:
		entries := make([]string, 0, v.Len())
		for it := v.MapRange(); it.Next(); {
			var e strings.Builder
			stateText(&e, it.Key(), depth+1)
			e.WriteByte(':')
			stateText(&e, it.Value(), depth+1)
			entries = append(entries, e.String())
		}
		sort.Strings(entries)
		b.WriteString("map[" + strings.Join(entries, " ") + "]")
	case reflect.Struct:
		b.WriteByte('{')
		first := true
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).IsZero() || opaqueID(v.Type(), v.Type().Field(i).Name) {
				continue
			}
			if !first {
				b.WriteByte(' ')
			}
			first = false
			b.WriteString(v.Type().Field(i).Name + ":")
			stateText(b, v.Field(i), depth+1)
		}
		b.WriteByte('}')
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		b.WriteString(v.Kind().String())
	default:
		b.WriteString("?")
	}
}
