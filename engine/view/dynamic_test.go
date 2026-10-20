package view

import (
	"reflect"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// Every field of the game state is classified here: shown by a typed view
// field, carried as dynamic state (Status, Links or a View record), or not
// game state at all. A new state field fails TestViewCoversState until it is
// classified, which is how principle 1 of mz-gorge-design.md (everything
// dynamic is in the view) stays true as the engine grows.
const (
	typed    = "typed"    // a typed View/PlayerView/CardView/StackView field
	dynamic  = "dynamic"  // Status or Links, checked by setting the field
	record   = "record"   // a View list ProjectDynamic copies under Omniscient
	internal = "internal" // derived cache or static card data, not game state
)

var gameFields = map[string]string{
	"goadsSeen": internal, "blockersLive": internal, "preparedSources": internal, "crewedObjects": internal,
	"Players": typed, "Objs": typed, "Stack": typed, "zones": typed,
	"Turn": typed, "Active": typed, "Priority": typed, "Step": typed, "Over": typed, "Winner": typed, "Draw": typed,
	"Initiative": typed, "HasInitiative": typed,
	"Passes": dynamic, "StartingPlayer": dynamic, "HasStartingPlayer": dynamic,
	"ExtraTurns": dynamic, "SkipTurns": dynamic, "CombatsThisTurn": dynamic, "EndStepsThisTurn": dynamic,
	"ResolvedThisTurn": dynamic, "Monarch": dynamic, "HasMonarch": dynamic,
	"TurnStartMonarch": dynamic, "HasTurnStartMonarch": dynamic, "ControlledBy": dynamic, "ControlArmedTurn": dynamic,
	"NextID": dynamic, "Clock": dynamic, "DelayedNext": dynamic,
	"Delayed": record, "ExtraTurnQueue": record, "ExtraPhases": record, "Entered": record, "ExcessDamageVictims": record,
	"Tokens": internal, "NameUniverse": internal, "NamedCorpus": internal, "NameUniverseNames": internal, "HypotheticalCards": internal,
}

var playerFields = map[string]string{
	"ID": typed, "Name": typed, "PlayerName": typed, "Life": typed, "Lost": typed,
	"SpellsCastThisTurn": typed, "LifeGainedThisTurn": typed, "LifeLostThisTurn": typed, "TokensCreatedThisTurn": typed,
	"Pool": typed, "RestrictedMana": typed, "Counters": typed, "Commanders": typed, "CmdCasts": typed, "CmdDamage": typed,
	"DungeonObj": typed, "DungeonRoom": typed, "CompletedDungeons": typed, "RingBearer": typed,
}

var objectFields = map[string]string{
	"face": internal, "_": internal,
	"ID": typed, "Card": typed, "Owner": typed, "Controller": typed, "Zone": typed,
	"Tapped": typed, "SummonSick": typed, "FaceDown": typed, "Damage": typed, "IsToken": typed, "IsCopy": typed,
	"Counters": typed, "ActivatedThisTurn": typed, "Suspected": typed, "Renowned": typed, "Monstrous": typed,
	"Harnessed": typed, "Solved": typed, "Cloaked": typed, "Ability": typed, "Source": typed,
	"StackKind": typed, "StackKindKnown": typed, "Targets": typed, "IsAttacking": typed, "Attacking": typed,
	"BlockedBy": typed, "Imprinted": typed, "Paired": typed, "ExiledCards": typed,
	"AttachedTo": typed, "AttachedPlayer": typed, "HasAttachedPlayer": typed, "ChosenModes": typed,
}

// companions are fields a dynamic field is only read together with.
var companions = map[string][]string{
	"EnteredFrom": {"EnteredThisTurn"}, "AcqStep": {"AcqTurn"},
	"LatestCastFrom": {"HasLatestCast"}, "LatestCastBy": {"HasLatestCast"},
	"PreStackEntryThisTurn": {"HasPreStackEntry"}, "PreStackEntryFrom": {"HasPreStackEntry"},
	"PushSourceOnBattlefield": {"PushSourceStamped"}, "PushSourceIncarnation": {"PushSourceStamped"},
	"EncoreAttackDefender": {"EncoreAttackTurn"}, "GiftPromisedTo": {"CastFlags"},
	"Protector": {"ProtectorValid"}, "SneakDefender": {"SneakDefenderValid"}, "SneakDefenderObject": {"SneakDefenderValid"},
	"ETBCloneChoice": {"ETBCloneChoiceValid"}, "MayLookPlayer": {"HasMayLook"},
	"FaceDownPower": {"FaceDownHasPT"}, "FaceDownToughness": {"FaceDownHasPT"}, "CopyLoyalty": {"CopyLoyaltySet"},
	"StartingPlayer": {"HasStartingPlayer"}, "Monarch": {"HasMonarch"}, "TurnStartMonarch": {"HasTurnStartMonarch"},
}

func TestViewCoversState(t *testing.T) {
	// The zero state has no dynamic facts, so each field below is checked
	// on its own.
	if s, l := objectFacts(&state.Object{}); len(s)+len(l) > 0 {
		t.Fatalf("zero object has facts %v %v", s, l)
	}
	if s, l := playerFacts(&state.Player{}); len(s)+len(l) > 0 {
		t.Fatalf("zero player has facts %v %v", s, l)
	}
	if s := gameFacts(&state.Game{}); len(s) > 0 {
		t.Fatalf("zero game has facts %v", s)
	}
	check := func(name string, typ reflect.Type, classes map[string]string, read func(v reflect.Value) bool) {
		seen := map[string]bool{}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i).Name
			seen[f] = true
			class, ok := classes[f]
			if !ok {
				class = dynamic // unlisted fields must be dynamic state
			}
			if class != dynamic {
				continue
			}
			v := reflect.New(typ).Elem()
			fill(v.FieldByName(f), 0)
			for _, c := range companions[f] {
				fill(v.FieldByName(c), 0)
			}
			if !read(v) {
				t.Errorf("%s.%s is neither in the view nor classified (engine/view/dynamic_test.go)", name, f)
			}
		}
		for f := range classes {
			if !seen[f] {
				t.Errorf("%s.%s is classified but no longer exists", name, f)
			}
		}
	}
	check("state.Object", reflect.TypeOf(state.Object{}), objectFields, func(v reflect.Value) bool {
		s, l := objectFacts(v.Addr().Interface().(*state.Object))
		return len(s)+len(l) > 0
	})
	check("state.Player", reflect.TypeOf(state.Player{}), playerFields, func(v reflect.Value) bool {
		s, l := playerFacts(v.Addr().Interface().(*state.Player))
		return len(s)+len(l) > 0
	})
	check("state.Game", reflect.TypeOf(state.Game{}), gameFields, func(v reflect.Value) bool {
		return len(gameFacts(v.Addr().Interface().(*state.Game))) > 0
	})
}

// fill sets v, settable or not, to a non-zero value.
func fill(v reflect.Value, depth int) {
	if !v.CanSet() {
		v = reflect.NewAt(v.Type(), v.Addr().UnsafePointer()).Elem()
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.String:
		v.SetString("x")
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fill(v.Index(0), depth+1)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fill(v.Index(i), depth+1)
		}
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k, depth+1)
		fill(e, depth+1)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.Pointer:
		if depth > 2 {
			return
		}
		p := reflect.New(v.Type().Elem())
		fill(p.Elem(), depth+1)
		v.Set(p)
	case reflect.Struct:
		if depth > 2 {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if !strings.HasPrefix(v.Type().Field(i).Name, "_") {
				fill(v.Field(i), depth+1)
			}
		}
	}
}
