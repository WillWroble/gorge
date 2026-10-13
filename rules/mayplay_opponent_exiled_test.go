package rules

// mayplay_opponent_exiled_test.go pins the may-play spell walk over cards an
// OPPONENT owns (CR 601.2, CR 108.3): Azula, Cunning Usurper's "During your
// turn, you may cast cards exiled with Azula" names cards its ETB made the
// opponent exile, so they sit in the opponent's exile slice with the
// opponent as their controller. The walk must still offer them to Azula's
// controller -- and must never hand an opponent-owned card's OWN self-grant
// (Misthollow Griffin's "You may cast this card from exile") to anyone but
// its owner.

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// azulaScenario casts Azula with Grizzly Bears on p1's battlefield and
// Lightning Bolt in p1's graveyard (its ETB makes p1 exile both, with Azula
// as their exiled-with source), plus the extra p1 exile cards a variant
// places by setup (exiled by NOTHING, so never "exiled with" Azula), then
// asserts the offers at p0's next priority and runs the variant's tail steps.
func azulaScenario(p1Exile, expect, tail string) string {
	return `{"name":"azula-opponent-exiled","cr":["601.2","611.3"],"why":"inline",` +
		`"setup":{"p0":{"hand":["Azula, Cunning Usurper"]},` +
		`"p1":{"battlefield":["Grizzly Bears"],"graveyard":["Lightning Bolt"],"exile":[` + p1Exile + `]}},"steps":[` +
		`{"op":"cast","seat":0,"card":"p0:Azula, Cunning Usurper","mana":"CCUBB"},` +
		`{"op":"resolve"},` +
		`{"op":"mana","seat":0,"mana":"GGGRUUUU"},` +
		`{"op":"pass_to","seat":0,"decision":"priority","expect":[` + expect + `]}` + tail + `]}`
}

func runAzula(t *testing.T, p1Exile, expect, tail string) OracleResult {
	t.Helper()
	res, err := RunOracleScenarioJSON(testutil.CorpusRegistry(t), []byte(azulaScenario(p1Exile, expect, tail)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAzulaOffersOpponentOwnedExiledWithCards(t *testing.T) {
	res := runAzula(t, "",
		`{"offered":{"seat":0,"kind":"cast","card":"p1:Grizzly Bears"},"want":true},`+
			`{"offered":{"seat":0,"kind":"cast","card":"p1:Lightning Bolt"},"want":true}`,
		`,{"op":"cast","seat":0,"card":"p1:Grizzly Bears"},{"op":"resolve"}`)
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	// Precondition: both cards really sat in p1's exile after Azula's ETB
	// (the setup placed neither there), so the offer was over
	// opponent-owned cards.
	var exiled bool
	for _, s := range res.Snapshots {
		if len(s.Players) > 1 && containsStr(s.Players[1].Exile, "Grizzly Bears") && containsStr(s.Players[1].Exile, "Lightning Bolt") {
			exiled = true
			break
		}
	}
	if !exiled {
		t.Fatalf("precondition: Azula's ETB never left Grizzly Bears and Lightning Bolt in p1's exile: %+v", res.Snapshots)
	}
	// The cast resolves under p0's control; the card stays p1's (CR 108.3).
	final := res.Snapshots[len(res.Snapshots)-1]
	bears, ok := snapPerm(final, "p1:Grizzly Bears")
	if !ok || bears.Controller != 0 || bears.Owner != 1 {
		t.Fatalf("cast Grizzly Bears = %+v (on battlefield %v), want controller 0, owner 1", bears, ok)
	}
}

// A card in the opponent's exile that Azula did NOT exile is not covered:
// the Affected$ Card.ExiledWithSource match decides, not the zone walk.
func TestAzulaDoesNotOfferUnrelatedOpponentExile(t *testing.T) {
	res := runAzula(t, `"Savannah Lions"`,
		`{"offered":{"seat":0,"kind":"cast","card":"p1:Grizzly Bears"},"want":true},`+
			`{"offered":{"seat":0,"kind":"cast","card":"p1:Savannah Lions"},"want":false}`, "")
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
}

// An opponent-owned card's OWN exile self-grant (Misthollow Griffin) belongs
// to its owner: walking the opponent's exile slice must not hand it to p0.
func TestOpponentSelfGrantInExileIsNotOfferedToYou(t *testing.T) {
	res := runAzula(t, `"Misthollow Griffin"`,
		`{"offered":{"seat":0,"kind":"cast","card":"p1:Grizzly Bears"},"want":true},`+
			`{"offered":{"seat":0,"kind":"cast","card":"p1:Misthollow Griffin"},"want":false}`, "")
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	final := res.Snapshots[len(res.Snapshots)-1]
	if len(final.Players) < 2 || !containsStr(final.Players[1].Exile, "Misthollow Griffin") {
		t.Fatalf("precondition: p1 exile = %+v, want Misthollow Griffin", final.Players)
	}
}
