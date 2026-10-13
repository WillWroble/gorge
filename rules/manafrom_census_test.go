package rules

// Corpus census of the spent-mana PROVENANCE filters: every script spelling
// of Forge's `ManaFrom<X>` spell-ability property ("a spell cast using mana
// from a Treasure / an artifact / produced by CARDNAME"), at every site that
// reads it -- a trigger's ValidSA$ (printed or an SVar trigger body), a
// static's Affected$ (`CastSa Spell.ManaFrom<X>`) and a Count$ SVar. The
// AI-only `SVar:AIPreference:ManaFrom$...` hints are not filters and are
// skipped.
//
// Each row names whether the engine HONOURS the spelling at that site:
//
//   - Spell.ManaFromCard.StrictlySelf on a SpellCast trigger: the
//     producing-source provenance (rules/cast_mana_sources.go; fire and
//     control pinned by TestManaFromCardStrictlySelfReadsTheSpentSource);
//   - Spell.ManaFrom<Tag> on a SpellCast trigger and `CastSa
//     Spell.ManaFrom<Tag>` elsewhere, for a tag in castSaTokens (Treasure,
//     Cave, Desert, Artifact): the typed spend window
//     (validsa_manafrom_test.go, castsa_artifact_test.go,
//     castsa_provenance_test.go).
//
// Every other spelling fails closed (the trigger stays silent, the filter
// matches nothing). The table is a two-way ratchet: a new corpus spelling, a
// row that disappears, or a reader that starts or stops honouring one fails
// the test, so the honoured set is always the measured one.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// manaFromRow is one site of one ManaFrom spelling. site is "trigger:<Mode>"
// for a ValidSA$ (printed T: or an SVar trigger body), "static" for an
// Affected$ and "svar" for any other SVar body.
type manaFromRow struct {
	card, site, spelling string
	honoured             bool
}

// wantManaFromCensus is the measured corpus at the FORGE_REF pin.
var wantManaFromCensus = []manaFromRow{
	{"Alchemist's Talent", "trigger:SpellCast", "Spell.ManaFromTreasure", true},
	{"Brass's Tunnel-Grinder", "trigger:SpellCast", "Spell.ManaFromCard.StrictlySelf", true},
	{"Inga and Esika", "trigger:SpellCast", "Spell.ManaFromCreature_3", false},
	{"Mastermind Plum", "trigger:SpellCast", "Spell.ManaFromTreasure", true},
	{"Rain of Riches", "static", "Spell.ManaFromTreasure", true},
	{"Rain of Riches", "svar", "Spell.ManaFromTreasure", true},
	{"Shadow the Hedgehog", "static", "Spell.ManaFromArtifact", true},
	{"Smaug, Wicked Worm", "trigger:SpellCast", "Spell.ManaFromTreasure", true},
	{"The Everflowing Well", "trigger:SpellCast", "Spell.ManaFromCard.StrictlySelf", true},
	{"Thousand Moons Smithy", "trigger:SpellCast", "Spell.ManaFromCard.StrictlySelf", true},
	{"Vazi, Keen Negotiator", "trigger:SpellAbilityCast", "SpellAbility.ManaFromTreasure", false},
}

var manaFromSpelling = regexp.MustCompile(`[A-Za-z]+\.ManaFrom[A-Za-z0-9_]*(?:\.[A-Za-z]+)*`)

// manaFromHonoured is the reader table the header names.
func manaFromHonoured(site, spelling string) bool {
	switch site {
	case "trigger:SpellCast":
		if spelling == "Spell.ManaFromCard.StrictlySelf" {
			return true
		}
		// ValidSAMatches answers Spell.ManaFrom<Tag> through the CastSa strip.
		return len(castSaTokensIn("Card.CastSa "+spelling)) > 0
	case "static", "svar":
		return len(castSaTokensIn("Card.CastSa "+spelling)) > 0
	}
	// SpellAbilityCast's spell arm reads no ManaFrom constraint.
	return false
}

func TestManaFromProvenanceCensus(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	var got []manaFromRow
	add := func(card, site, text string) {
		for _, sp := range manaFromSpelling.FindAllString(text, -1) {
			got = append(got, manaFromRow{card, site, sp, manaFromHonoured(site, sp)})
		}
	}
	for _, c := range reg.AllCards() {
		if len(c.Faces) == 0 {
			continue
		}
		name := c.Faces[0].Name
		for _, f := range c.Faces {
			for i := range f.Triggers {
				add(name, "trigger:"+f.Triggers[i].Mode, f.Triggers[i].ParamStr(cards.PKValidSA))
			}
			for i := range f.Statics {
				add(name, "static", f.Statics[i].Params["Affected"])
			}
			for key, body := range f.SVars {
				if key == "AIPreference" || !strings.Contains(body, "ManaFrom") {
					continue
				}
				if tr, ok := cards.ParseTriggerLine(body); ok && tr.Mode != "" {
					add(name, "trigger:"+tr.Mode, tr.ParamStr(cards.PKValidSA))
					continue
				}
				add(name, "svar", body)
			}
		}
	}
	sort.Slice(got, func(i, j int) bool {
		a, b := got[i], got[j]
		if a.card != b.card {
			return a.card < b.card
		}
		if a.site != b.site {
			return a.site < b.site
		}
		return a.spelling < b.spelling
	})
	// One row per (card, site, spelling): Alchemist's Talent's class-level
	// trigger body is reachable both as an SVar and as the trigger the
	// level grants, and it is one filter.
	got = slicesCompactRows(got)
	honoured := 0
	for _, r := range got {
		if r.honoured {
			honoured++
		}
	}
	t.Logf("ManaFrom provenance census: %d sites, %d honoured", len(got), honoured)
	if len(got) != len(wantManaFromCensus) {
		t.Errorf("census has %d rows, want %d:\n got %+v", len(got), len(wantManaFromCensus), got)
	}
	for i := 0; i < len(got) && i < len(wantManaFromCensus); i++ {
		if got[i] != wantManaFromCensus[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], wantManaFromCensus[i])
		}
	}
}

func slicesCompactRows(rows []manaFromRow) []manaFromRow {
	out := rows[:0]
	for i, r := range rows {
		if i > 0 && r == rows[i-1] {
			continue
		}
		out = append(out, r)
	}
	return out
}
