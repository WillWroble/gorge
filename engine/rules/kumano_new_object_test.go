package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// CR 400.7: a permanent that leaves the battlefield and returns is a new
// object with no memory of its previous existence. Kumano's Pupils ("If a
// creature dealt damage by CARDNAME this turn would die, exile it
// instead") damages Pillarfield Ox in combat; the Ox is bounced, recast and
// destroyed the same turn. The recast Ox was never dealt damage by the
// Pupils, so it dies to the graveyard. XMage's
// ZoneChangeReplacementTest#testPermanentNewInstanceAndKumano; gorge kept
// the old object's this-turn damage record across the zone change and
// exiled it.
const kumanoNewObjectScenario = `{
  "name": "kumano-new-object",
  "setup": {
    "p0": {"hand": ["Unsummon", "Terminate"], "battlefield": ["Kumano's Pupils", "Island", "Swamp", "Mountain"]},
    "p1": {"battlefield": ["Pillarfield Ox", "Plains", "Plains", "Plains", "Plains"]}
  },
  "steps": [
    {"op": "pass_to", "seat": 0, "step": "upkeep", "active": "p1"},
    {"op": "attack", "seat": 1, "attackers": ["p1:Pillarfield Ox"], "defender": "p0"},
    {"op": "block", "seat": 0, "blocks": [["p0:Kumano's Pupils", "p1:Pillarfield Ox"]]},
    {"op": "pass_to", "seat": 0, "step": "combat-damage", "active": "p1"},
    {"op": "pass", "seat": 1},
    {"op": "cast", "seat": 0, "card": "p0:Unsummon", "mana": "U", "targets": ["p1:Pillarfield Ox"]},
    {"op": "pass_to", "seat": 0, "step": "main2", "active": "p1"},
    {"op": "resolve", "seat": 0},
    {"op": "cast", "seat": 1, "card": "p1:Pillarfield Ox", "mana": "CCCW"},
    {"op": "resolve", "seat": 0},
    {"op": "pass", "seat": 1},
    {"op": "cast", "seat": 0, "card": "p0:Terminate", "mana": "BR", "targets": ["p1:Pillarfield Ox"]},
    {"op": "pass_to", "seat": 0, "step": "end", "active": "p1"},
    {"op": "resolve", "seat": 0}
  ],
  "expect": [
    {"count": {"seat": 1, "zone": "battlefield", "name": "Pillarfield Ox"}, "eq": 0},
    {"count": {"seat": 1, "zone": "exile", "name": "Pillarfield Ox"}, "eq": 0},
    {"count": {"seat": 1, "zone": "graveyard", "name": "Pillarfield Ox"}, "eq": 1}
  ]
}`

func TestKumanosPupilsForgetsACreatureThatLeftTheBattlefield(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(kumanoNewObjectScenario))
	if err != nil {
		t.Fatal(err)
	}
	sc.xmageFixture = true
	fails, transcript, _ := runOracleScenario(reg, sc)
	if len(fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", fails, strings.Join(transcript, "\n"))
	}
}
