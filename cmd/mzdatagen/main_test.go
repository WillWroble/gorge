package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestParsesTheForksConfig reads the mage fork's own config.yml
// (Mage.MageZero/config/config.yml, copied to testdata) with Config's keys
// and defaults, including the fork's hiddenInfo key.
func TestParsesTheForksConfig(t *testing.T) {
	f, err := os.Open("testdata/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := parseConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	a := c.PlayerA
	switch {
	case c.GoesFirst != "random" || c.GameMode != "normal":
		t.Errorf("goes_first %q game_mode %q", c.GoesFirst, c.GameMode)
	case a.DeckPath != "decks/Pauper-Faeries.txt" || a.Type != "mcts" || a.OutputFile != "":
		t.Errorf("player_a %+v", a)
	case a.Priority || a.PriorTemperature != 1.5 || a.PriorBonus != 0.1:
		t.Errorf("priors %+v", a)
	case a.NoiseEnabled || a.DirichletNoise != 0.15 || a.SelectionTemperature != 2:
		t.Errorf("noise %+v", a)
	case a.SearchBudget != 300 || a.TimeoutMs != 60000 || a.TDDiscount != 0.92 || a.BackpropDiscount != 0.99 || a.OfflineMode || !a.PruneDuplicateStates:
		t.Errorf("mcts %+v", a)
	case a.Mulligans || a.ManualTap || !a.OpponentHand:
		t.Errorf("gameplay %+v", a)
	case c.PlayerB.DeckPath != "decks/Pauper-CawGates.txt":
		t.Errorf("player_b %+v", c.PlayerB)
	case c.Training != TrainingConfig{Games: 100, GamesPerFile: 200, MaxTurns: 50, MaxMinutes: 60, Threads: 8}:
		t.Errorf("training %+v", c.Training)
	case c.Server != ServerConfig{Host: "localhost", Port: 50052, OpponentPort: 50053}:
		t.Errorf("server %+v", c.Server)
	case !c.Logging.WriteFinalWR || c.Logging.LogFeatureHash:
		t.Errorf("logging %+v", c.Logging)
	}
}

// TestGeneratesAGame plays one offline game from the fork's config and
// checks both players' files hold its rows.
func TestGeneratesAGame(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	f, err := os.Open("testdata/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg.Training.Games, cfg.Training.Threads = 1, 1
	cfg.Logging.WriteFinalWR = false
	for i, p := range []*PlayerConfig{&cfg.PlayerA, &cfg.PlayerB} {
		p.SearchBudget, p.OfflineMode = 10, true
		p.OutputFile = filepath.Join(dir, string(rune('a'+i))+".hdf5")
	}
	var log strings.Builder
	g, err := newGenerator(&log, cfg, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.generate(1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "successful 1, failed 0") {
		t.Fatalf("game did not complete:\n%s", log.String())
	}
	for _, name := range []string{"a.npz", "b.npz"} {
		z, err := zip.OpenReader(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var rows int64
		for _, e := range z.File {
			if e.Name == "row.npy" {
				rc, _ := e.Open()
				b, _ := io.ReadAll(rc)
				rc.Close()
				rows = int64(len(b))
			}
		}
		z.Close()
		if rows <= 128 {
			t.Errorf("%s holds no rows", name)
		}
	}
}
