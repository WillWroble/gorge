package effects

import (
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

var triggeredControlSpelling = regexp.MustCompile(`\b(?:ControlledBy|OwnedBy) (Triggered[A-Za-z]+)`)

// knownUnmodelledTriggeredControlRefs is the ratchet: the corpus spellings
// `ControlledBy|OwnedBy Triggered<X>` that controlReferent does not yet
// recognise (and so fail closed). It only shrinks.
var knownUnmodelledTriggeredControlRefs = map[string]bool{
	// The controller of the spell's targets, or of the spell itself
	// (Exterminator Magmarch): needs a spell-target role on the trigger.
	"TriggeredSpellAbilityTargetsOrController": true,
	// Deliberately out of scope: TestTriggerReferentGrammarScope pins it as
	// unknown (the object-target-controller spelling is a separate decision).
	"TriggeredTargetController": true,
}

func collectStrings(v reflect.Value, seen map[uintptr]bool, add func(string)) {
	switch v.Kind() {
	case reflect.String:
		add(v.String())
	case reflect.Ptr:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		collectStrings(v.Elem(), seen, add)
	case reflect.Interface:
		if !v.IsNil() {
			collectStrings(v.Elem(), seen, add)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				collectStrings(v.Field(i), seen, add)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			collectStrings(v.Index(i), seen, add)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			collectStrings(k, seen, add)
			collectStrings(v.MapIndex(k), seen, add)
		}
	}
}

// TestTriggeredControlReferentCensus walks every corpus face for
// `ControlledBy|OwnedBy Triggered*` spellings and requires controlReferent to
// recognise each one, except the ratcheted unmodelled set.
func TestTriggeredControlReferentCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	found := map[string]int{}
	seen := map[uintptr]bool{}
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			collectStrings(reflect.ValueOf(f), seen, func(s string) {
				for _, m := range triggeredControlSpelling.FindAllStringSubmatch(s, -1) {
					found[m[1]]++
				}
			})
		}
	}
	if found["TriggeredActivator"] == 0 || found["TriggeredTarget"] == 0 {
		t.Fatalf("census precondition: walk found no spellings: %v", found)
	}
	var refs []string
	for r := range found {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	for _, r := range refs {
		for _, op := range []string{"ControlledBy", "OwnedBy"} {
			_, _, ok := controlReferent(op + " " + r)
			if ok == knownUnmodelledTriggeredControlRefs[r] {
				t.Errorf("%s %s (%d corpus uses): recognised=%v but ratchet says unmodelled=%v",
					op, r, found[r], ok, knownUnmodelledTriggeredControlRefs[r])
			}
		}
	}
	for r := range knownUnmodelledTriggeredControlRefs {
		if found[r] == 0 {
			t.Errorf("ratchet entry %s no longer appears in the corpus; delete it", r)
		}
	}
}

// TestTriggeredActivatorControlReferent: "ControlledBy TriggeredActivator"
// (Magebane Lizard) binds the casting player, and an unbound role fails
// closed instead of matching everything.
func TestTriggeredActivatorControlReferent(t *testing.T) {
	g, ids := board(t)
	mine, theirs := g.Obj(ids["myBear"]), g.Obj(ids["theirBig"])
	if mine.Controller == theirs.Controller {
		t.Fatal("precondition: the two creatures need different controllers")
	}
	for _, ref := range []string{"TriggeredActivator", "TriggeredAttackingPlayer"} {
		spec := "Creature.ControlledBy " + ref
		if un := UnknownPredicates(spec); len(un) != 0 {
			t.Fatalf("%s unknown: %v", spec, un)
		}
		role := state.Target{IsPlayer: true, Player: theirs.Controller}
		sc := SpecContext{}
		if ref == "TriggeredActivator" {
			sc.TriggerActivator = role
		} else {
			sc.AttackingPlayer = role
		}
		if !MatchesSpecCtx(g, spec, ids["theirBig"], sc) || MatchesSpecCtx(g, spec, ids["myBear"], sc) {
			t.Errorf("%s did not bind the role player", spec)
		}
		if MatchesSpecCtx(g, spec, ids["theirBig"], SpecContext{}) || MatchesSpecCtx(g, spec, ids["myBear"], SpecContext{}) {
			t.Errorf("%s unbound role must fail closed", spec)
		}
	}
}
