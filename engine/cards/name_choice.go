package cards

// nameChoiceMarkers are the script words of a card-NAME choice that reads
// rules.Config.NameUniverse: NameCard (the mid-resolution and as-enters name
// ask) and ChosenName (the chosen-name copy and the readers that follow one).
// It is nameUniverseMarkers without AllNonBasicLandType, whose land-type
// vocabulary is not a name choice.
var nameChoiceMarkers = []string{"NameCard", "ChosenName"}

// MentionsNameChoice reports whether any face of c mentions a card-name
// choice anywhere in its compiled text (the same deliberately textual,
// over-approximating walk NeedsFullNameUniverse runs).
func MentionsNameChoice(c *Card) bool { return cardMentions(c, nameChoiceMarkers) }
