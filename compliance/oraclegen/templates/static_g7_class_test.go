package templates

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestStaticG7ClassServed is the level-B wave-4 class G7 ("static effect
// unobservable") ratchet over the rows lb4-g7 served: each row's requirement
// is generated, and its scenario carries the observation that serves it.
// A row that goes back to a skip, or loses its observation, fails here by
// name.
func TestStaticG7ClassServed(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, tc := range []struct {
		card, key string
		// want is a substring of the item's JSON the observation must put
		// there: the expectation, the fixture or the compare opt-in.
		want string
	}{
		// A stack-scoped Delve grant priced by the offer gate (rules'
		// hasCastDelve): the probe one short of mana is offered.
		{"Teval, Arbiter of Virtue", "static#0.0", `"offered":{"seat":0,"kind":"cast","card":"p0:Hill Giant"},"want":true`},
		// Granted cast-trigger keywords (static_granted_spell_trigger.go).
		{"Prismari, the Inspiration", "static#0.0", `"trigger_on_stack":"p0:Divination","want":true`},
		{"Quandrix, the Proof", "static#0.0", `"trigger_on_stack":"p0:Divination","want":true`},
		// Split second (static_granted_split_second.go).
		{"Samut, Tyrant of Naktamun", "static#0.0", `"offered":{"seat":0,"kind":"cast","card":"p0:Shock"},"want":false`},
		// Colour hexproof-from under its own compare opt-in.
		{"Tam, Mindful First-Year", "static#0.0", `"compare":["keywords_hexproof_from"]`},
		// Token recipient and token target (static_granted_token.go).
		{"Insidious Roots", "static#0.0", `"card":"p0:token:Goblin"`},
		{"Ghired, Mirror of the Wilds", "static#0.0", `"card":"p0:Dragon Fodder"`},
		// Gated self grant target fixtures (static_granted_gated_targets.go).
		{"Slick Imitator", "static#0.0", `"card":"p0:Accumulated Knowledge"`},
		{"Adagia, Windswept Bastion", "static#0.0", `"Ornithopter"`},
		// Behold riders: the Condition$ OptionalCost gate and the surveil
		// graveyarded by the paid branch.
		{"Osseous Exhale", "static#0.0", `"cast_mode":"optionalcost"`},
		{"Piercing Exhale", "static#0.0", `{"kind":"arrange","pick":[]}`},
	} {
		t.Run(tc.card, func(t *testing.T) {
			req := counterReq(t, reg, tc.card, tc.key)
			item, skip := GenerateB(reg, tc.card, req)
			if skip != nil {
				t.Fatalf("skip = %q, want served", skip.Reason)
			}
			b, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Fatalf("item lacks %s:\n%s", tc.want, b)
			}
		})
	}
}
