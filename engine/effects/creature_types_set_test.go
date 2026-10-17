package effects

import (
	"slices"
	"strings"
	"testing"
)

// TestCreatureTypeVocabularyFoldDistinct holds the premise CreatureTypeSet's
// users rest on: no two vocabulary words are equal under strings.EqualFold, so
// an exact vocabulary index match is exactly the case-insensitive match the
// type readers make. A future vocabulary entry that differs from another only
// in case would make rules' all-creature-types dedup (and TypeMatchWords'
// already-materialised shortcut) disagree with the EqualFold reference.
func TestCreatureTypeVocabularyFoldDistinct(t *testing.T) {
	vocab := CreatureTypeWordList()
	if !slices.IsSorted(vocab) {
		t.Fatal("CreatureTypeWordList is not sorted; CreatureTypeSet's binary search needs it")
	}
	for i, a := range vocab {
		for _, b := range vocab[i+1:] {
			if strings.EqualFold(a, b) {
				t.Fatalf("vocabulary words %q and %q fold to each other", a, b)
			}
		}
	}
}

func TestCreatureTypeSetAddWords(t *testing.T) {
	vocab := CreatureTypeWordList()
	var s CreatureTypeSet
	if n := s.AddWords([]string{"Creature", "Elf", "elf", "Elf", "Arcane", "Zombie"}); n != 2 {
		t.Fatalf("AddWords counted %d new vocabulary words, want 2 (Elf, Zombie; elf is not a vocabulary word)", n)
	}
	for i, w := range vocab {
		if s.Has(i) != (w == "Elf" || w == "Zombie") {
			t.Fatalf("set membership of %q is %v", w, s.Has(i))
		}
	}
	// A materialised run, then the same words out of order: every word is
	// found once whichever path (cursor or binary search) finds it.
	var full CreatureTypeSet
	if n := full.AddWords(append([]string{"Land"}, vocab...)); n != len(vocab) {
		t.Fatalf("materialised run counted %d, want %d", n, len(vocab))
	}
	rev := slices.Clone(vocab)
	slices.Reverse(rev)
	var back CreatureTypeSet
	if n := back.AddWords(rev); n != len(vocab) || back != full {
		t.Fatalf("reversed vocabulary counted %d (set equal %v), want %d", n, back == full, len(vocab))
	}
	if n := full.AddWords(vocab); n != 0 {
		t.Fatalf("re-adding the vocabulary counted %d new words, want 0", n)
	}
}

// TestTypeMatchWordsMaterialisedMembership: the shortcut that returns an
// already-materialised list must answer every membership question exactly as
// the appended list it replaces.
func TestTypeMatchWordsMaterialisedMembership(t *testing.T) {
	vocab := CreatureTypeWordList()
	types := append([]string{"Land", "Creature"}, vocab...)
	got := TypeMatchWords(types, true)
	if &got[0] != &types[0] || len(got) != len(types) {
		t.Fatal("a materialised list was copied")
	}
	want := append(slices.Clone(types), vocab...)
	probes := append([]string{"land", "CREATURE", "elf", "Arcane", "Artifact", "Kindred", ""}, vocab...)
	for _, p := range probes {
		has := func(list []string) bool {
			return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, p) })
		}
		if has(got) != has(want) {
			t.Fatalf("membership of %q differs: shortcut %v, appended %v", p, has(got), has(want))
		}
	}
	// A list missing one word still gets the appended copy.
	partial := slices.DeleteFunc(slices.Clone(types), func(w string) bool { return w == "Elf" })
	if out := TypeMatchWords(partial, true); len(out) != len(partial)+len(vocab) {
		t.Fatalf("partial list: got %d words, want the appended %d", len(out), len(partial)+len(vocab))
	}
}
