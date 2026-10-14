package feedback

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
)

// TestStartChooserRestoresTheConfigField pins the E3-2 replay input: a
// match.json that records start_chooser rebuilds a rules.Config with the
// same chooser, and one without it (every match before E3) rebuilds the
// toss — nil.
func TestStartChooserRestoresTheConfigField(t *testing.T) {
	reg := cards.NewRegistry()
	c, diags := cards.ParseBytes("test_fixture_card.txt", []byte("Name:Test Pipewhale\nTypes:Creature Whale\nPT:2/2\nOracle:x\n"))
	if len(diags) != 0 {
		t.Fatalf("fixture card: %v", diags)
	}
	reg.Add(c)

	chooser := 1
	m := matchJSON{DeckCards: [][]string{{"Test Pipewhale"}}, StartChooser: &chooser}
	cfg, err := config(m, reg, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.StartChooser == nil || *cfg.StartChooser != 1 {
		t.Fatalf("config dropped start_chooser 1: %v", cfg.StartChooser)
	}

	m.StartChooser = nil
	cfg, err = config(m, reg, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.StartChooser != nil {
		t.Fatalf("config invented a chooser for a nil start_chooser: %v", cfg.StartChooser)
	}
}
