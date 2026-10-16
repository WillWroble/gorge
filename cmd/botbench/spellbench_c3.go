package main

// -spellbench-az-c3-net and -spellbench-decks-dir: a SpellBench run of the
// az-c3 policy (bots/azc3) over a directory of decks, the deployment's pool
// (docs/superpowers/reports/2026-10-09-az-c3-hosted.md). Like az-redeal, the
// bench builds the az seat itself from the hosted config, so it reaches the
// concrete *azmcts.Seat; the hosted EnvSeat wrapper is the server's entry.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/internal/azmcts"
	"github.com/adams-shaun/gorge/internal/gonet"
	"github.com/adams-shaun/gorge/internal/spellbench/builtins"
	"github.com/adams-shaun/gorge/internal/spellbench/registry"
	"github.com/adams-shaun/gorge/seat"
)

// registerAZC3 loads the network at path and registers the az-c3 spellbench
// policy: azc3.Config, the search the net was trained and measured with.
func registerAZC3(path string) error {
	m, err := gonet.Model(path)
	if err != nil {
		return fmt.Errorf("-spellbench-az-c3-net: %w", err)
	}
	registry.Register(azc3.Policy, func(seed uint64, _ builtins.ManaMode) seat.Seat {
		s, err := azmcts.NewSeat(seed, m, azc3.Config())
		if err != nil {
			panic("botbench: " + err.Error())
		}
		return s
	})
	return nil
}

// loadDeckDir lists the decks in dir (file stems, sorted) and parses each, so
// a run can take a pool that is not one of the embedded catalogs.
func loadDeckDir(dir string) ([]string, []deck.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("-spellbench-decks-dir: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("-spellbench-decks-dir: no deck files in %s", dir)
	}
	files := make([]deck.File, len(ids))
	for i, id := range ids {
		raw, err := os.ReadFile(filepath.Join(dir, id+".json"))
		if err != nil {
			return nil, nil, err
		}
		if files[i], err = deck.Parse(raw); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", id, err)
		}
	}
	return ids, files, nil
}
