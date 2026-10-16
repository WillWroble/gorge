//go:build lab

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/lab/draft/limited/setdata"
)

// prewarmSetArt fills the cache for every card on every sheet of every
// embedded draft set (lab/draft/limited/setdata), after the deck prewarm. No deck file
// names most of a set's cards, so without it a draft pack is the first
// request for its art, the client's wait budget expires on a burst of cold
// names, and a fresh pack renders blank until those fetches settle.
func prewarmSetArt(ctx context.Context, ac *artCache, logf func(string, ...any)) {
	names, err := setCardNames()
	if err != nil {
		logf("art prewarm (sets): %v", err)
		return
	}
	_, _ = fillNames(ctx, ac, names, "draft sets "+strings.Join(setdata.Codes(), ","), "art prewarm (sets)", logf, fillLimits{})
}

// setCardNames is every card name on any sheet of any collation of every
// embedded set, sorted and de-duplicated.
func setCardNames() ([]string, error) {
	seen := make(map[string]bool)
	for _, code := range setdata.Codes() {
		for _, c := range setdata.Collations(code) {
			spec, err := setdata.LoadCollation(code, c)
			if err != nil {
				return nil, fmt.Errorf("set %s collation %s: %w", code, c, err)
			}
			for _, sh := range spec.Config.Sheets {
				for _, sc := range sh.Cards {
					seen[sc.Name] = true
				}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
