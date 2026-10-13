package policynet

import (
	"reflect"
	"testing"
)

// A label row flagged explore:true carries a forced exploration answer, not
// the teacher's choice: policy imitation (Load, and LoadWith by default)
// drops it, and LoadOptions.KeepExplore keeps it for value/outcome learning.
func TestLoadDropsExploreFlaggedRows(t *testing.T) {
	recs := fixtureRecords()
	base := writeCorpus(t, recs, false)
	baseEx, baseStats, err := Load(base)
	if err != nil {
		t.Fatal(err)
	}
	if baseStats.Explored != 0 || len(baseEx) == 0 {
		t.Fatalf("precondition: unflagged corpus loads %d examples, Explored=%d", len(baseEx), baseStats.Explored)
	}

	flagged := fixtureRecords()
	if len(flagged[0].Candidates) == 0 {
		t.Fatal("precondition: record 0 must be a loadable row")
	}
	flagged[0].Explore = true
	path := writeCorpus(t, flagged, true)

	got, st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Explored != 1 || len(got) != len(baseEx)-1 || st.Records != baseStats.Records {
		t.Fatalf("Load: %d examples (want %d), stats %+v (base %+v)", len(got), len(baseEx)-1, st, baseStats)
	}
	if !reflect.DeepEqual(got, baseEx[1:]) {
		t.Fatal("Load kept the wrong rows: not the unflagged remainder")
	}

	with, wst, err := LoadWith(path, LoadOptions{})
	if err != nil || wst != st || !reflect.DeepEqual(with, got) {
		t.Fatalf("LoadWith(zero) must match Load: %v stats %+v vs %+v", err, wst, st)
	}

	kept, kst, err := LoadWith(path, LoadOptions{KeepExplore: true})
	if err != nil || len(kept) != len(baseEx) || kst.Explored != 0 {
		t.Fatalf("KeepExplore: %d examples (want %d), stats %+v, %v", len(kept), len(baseEx), kst, err)
	}
	if !reflect.DeepEqual(kept, baseEx) {
		t.Fatal("KeepExplore examples differ from the unflagged corpus")
	}
}
