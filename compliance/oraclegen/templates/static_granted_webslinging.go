package templates

import (
	"strings"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen"
)

// webSlingingProbe is the legendary coloured spell a web-slinging grant
// reaches: its printed {2}{R}{R} is out of reach of the grant's pool, so only
// the web-slinging cost makes it castable.
const webSlingingProbe = "Krenko, Mob Boss"

// staticWebSlingingGrantItem serves a stack-scoped Web-slinging grant (the
// Amazing Spider-Man face's "Each legendary spell you cast that's one or more
// colors has web-slinging {G}{W}{U}"). Web-slinging (CR 702.188a) is an
// alternative cost that also returns a tapped creature you control to its
// owner's hand, so the scenario taps p0's Grizzly Bears by attacking with it,
// moves to main 2, floats exactly the web-slinging mana and asserts the probe
// is offered. The control without the card asserts it is not.
func staticWebSlingingGrantItem(reg *cards.Registry, f *cards.Face, name string, req levelb.Requirement, st cards.Static) (oraclegen.Item, bool) {
	kw := strings.TrimSpace(st.ParamStr(cards.PKAddKeyword))
	rest, ok := strings.CutPrefix(kw, "Web-slinging:")
	if !ok || strings.Contains(kw, " & ") {
		return oraclegen.Item{}, false
	}
	cost, _, _ := strings.Cut(rest, ":")
	pool, why := oraclegen.PoolFor(cost)
	if why != "" || pool == "" {
		return oraclegen.Item{}, false
	}
	if _, ok := reg.Lookup(webSlingingProbe); !ok {
		return oraclegen.Item{}, false
	}
	steps := func(want bool) []oraclegen.Step {
		return []oraclegen.Step{
			{Op: "attack", Seat: 0, Attackers: []string{"p0:" + staticProbe}, Defender: "p1"},
			{Op: "pass_to", Seat: 0, Step: "main2"},
			{Op: "mana", Seat: 0, Mana: pool},
			{Op: "pass_to", Seat: 0, Decision: "priority", Expect: []oraclegen.Expect{
				{Offered: &oraclegen.Offered{Seat: 0, Kind: offerKindCast, Card: "p0:" + webSlingingProbe}, Want: boolPtr(want)},
			}},
		}
	}
	control := staticScenario(f, name, []string{staticProbe}, []string{webSlingingProbe}, steps(false))
	if res, ok := runStatic(reg, control); !ok || len(res.Fails) != 0 {
		return oraclegen.Item{}, false
	}
	sc := withBackFace(staticScenario(f, name, []string{name, staticProbe}, []string{webSlingingProbe}, steps(true)), name, req)
	res, ok := runStatic(reg, sc)
	if !ok || len(res.Fails) != 0 {
		return oraclegen.Item{}, false
	}
	it := oraclegen.NewLevelBItem(name, req.Key, StaticApplies.Version, []string{"611.3", "601.2b"}, sc)
	it.XAnswers = oraclegen.XAnswersForScenario(res, sc, oraclegen.ModeNumbers(f), nil)
	return it, true
}
