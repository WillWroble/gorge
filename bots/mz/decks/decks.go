// Package decks embeds the mage fork's MageZero decks (WillWroble/mage,
// graph-encoder, Mage.MageZero/decks) as gorge deck files, so mz-gorge is
// measured on the same decks as the fork. File stems are the fork's deck
// names lower-cased with punctuation folded to '-'; each file's "name" is the
// fork's own.
package decks

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/deck"
)

//go:embed *.json
var fsys embed.FS

// Names lists the embedded decks by file stem, sorted.
func Names() []string {
	entries, err := fsys.ReadDir(".")
	if err != nil {
		panic("decks: embed unreadable: " + err.Error())
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// File reads one embedded deck file, unresolved.
func File(name string) (deck.File, error) {
	raw, err := fsys.ReadFile(path.Join(".", name+".json"))
	if err != nil {
		return deck.File{}, fmt.Errorf("decks: %q: %w", name, err)
	}
	f, err := deck.Parse(raw)
	if err != nil {
		return deck.File{}, fmt.Errorf("decks: %q: %w", name, err)
	}
	return f, nil
}

// Load resolves one embedded deck against r into the flat card list
// rules.Config.Decks takes.
func Load(r *cards.Registry, name string) ([]*cards.Card, error) {
	f, err := File(name)
	if err != nil {
		return nil, err
	}
	cs, err := f.Resolve(r)
	if err != nil {
		return nil, fmt.Errorf("decks: %q: %w", name, err)
	}
	return cs, nil
}
