package policynet

import (
	"fmt"
	"strings"

	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
)

// The per-card graveyard tokens (FeaturesMZGrave). mz sees a graveyard only
// as a count; the encoding ablation (docs/superpowers/reports/
// 2026-10-10-encoding-ablation.md, section 2) measured graveyard properties
// as significant signal for both seats. Each card in a graveyard emits, under
// zone tag z = "gy-me" (the viewer's) or "gy-opp" (every other seat's):
//
//	mz|<z>|type|<Type>         one per type word
//	mz|<z>|mv<n>               mana value bucketed 0..8
//	mz|<z>|api|<SpellAPI>      the card's spell API
//	mz|<z>|kw|<keyword>        one per keyword
//	mz|<z>|cr|p<P>|t<T>        creature power/toughness buckets 0..9
//
// plus an aggregate "mz|<z>|n" (count) and "mz|<z>|n|type|<Type>" per type.
// Graveyards are public, so the set reads the redacted view only and is
// checkpointable and playable. Same geometry as mz (sparse rows only).
const graveFormat = 1

// graveHashSuffix is the token-format part of the mz-grave encoder hash.
func graveHashSuffix() string {
	return fmt.Sprintf("\x1fgrave-format=%d\x1fpins=%d,%d", graveFormat,
		hashID("mz|gy-me|type|Creature"), hashID("mz|gy-opp|n"))
}

func graveyardState(v view.View, seat state.PlayerID, push func(string, float32)) {
	for i := range v.Players {
		z := "gy-opp"
		if v.Players[i].ID == seat {
			z = "gy-me"
		}
		gy := v.Players[i].Graveyard
		if len(gy) == 0 {
			continue
		}
		typeN := map[string]float32{}
		for _, cv := range gy {
			for _, t := range strings.Fields(cv.Types) {
				push("mz|"+z+"|type|"+t, 1)
				typeN[t]++
			}
			push("mz|"+z+"|mv"+mzBucket(int32(mvOf(cv.ManaCost)), 8), 1)
			if cv.SpellAPI != "" {
				push("mz|"+z+"|api|"+cv.SpellAPI, 1)
			}
			for _, k := range cv.Keywords {
				push("mz|"+z+"|kw|"+strings.ToLower(k), 1)
			}
			if hasTypeWord(cv.Types, "Creature") {
				push("mz|"+z+"|cr|p"+mzBucket(cv.Power, 9)+"|t"+mzBucket(cv.Toughness, 9), 1)
			}
		}
		push("mz|"+z+"|n", float32(len(gy)))
		// The aggregate rows are order-sensitive only through push order, and
		// the sparse bag is sorted by row afterwards; no map range reaches an
		// output order that matters, but iterate the type words in card order
		// anyway for a stable emission.
		seen := map[string]bool{}
		for _, cv := range gy {
			for _, t := range strings.Fields(cv.Types) {
				if !seen[t] {
					seen[t] = true
					push("mz|"+z+"|n|type|"+t, typeN[t])
				}
			}
		}
	}
}
