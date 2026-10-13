package cards

// atEOTValues is the set of AtEOT$ rider values the engine schedules
// (effects/ateot.go's atEOTRiders is the matching behaviour table; a test in
// effects keeps the two key sets identical). Primitives reports any other
// value as the unsupported primitive "api:AtEOT.<value>" so `make report`
// counts the card as not playable instead of the engine silently keeping the
// affected object (Library's "shuffle into library" rider is the one corpus
// value outside the set).
var atEOTValues = map[string]bool{
	"Exile": true, "YourExile": true,
	"Sacrifice": true, "YourSacrifice": true, "SacrificeCtrl": true,
	"Hand": true, "Destroy": true,
	"ExileCombat": true, "SacrificeCombat": true,
	"SacrificeUpkeep": true, "YourSacrificeUpkeep": true, "YourExileUpkeep": true,
}

// AtEOTValueSupported reports whether the AtEOT$ value has a scheduled rider.
func AtEOTValueSupported(value string) bool { return atEOTValues[value] }

// AtEOTValues lists the supported AtEOT$ values (unordered; for tests).
func AtEOTValues() []string {
	out := make([]string, 0, len(atEOTValues))
	for k := range atEOTValues {
		out = append(out, k)
	}
	return out
}
