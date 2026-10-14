package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/draftnet"
)

// writeFixtureNet writes the draftnet parity weights re-sized to the
// table in dir/table.json (a zero card embedding, one row per card), with
// meta naming hash.
func writeFixtureNet(t *testing.T, dir string, rows int, hash string) string {
	t.Helper()
	b, err := os.ReadFile("../../limited/draftnet/testdata/parity.dpkw")
	if err != nil {
		t.Fatal(err)
	}
	f, err := draftnet.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	for i := range f.Tensors {
		if f.Tensors[i].Name == "card.emb" {
			f.Tensors[i].Shape = []int{rows, draftnet.CardEmb}
			f.Tensors[i].Data = make([]float32, rows*draftnet.CardEmb)
		}
	}
	f.Meta = []byte(`{"table_hash":"` + hash + `"}`)
	p := filepath.Join(dir, "w.dpkw")
	if err := os.WriteFile(p, f.Encode(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestGateNetDrafter: gate's -drafters takes the registered net kind, with
// the table beside the weights, and refuses weights whose meta names
// another table.
func TestGateNetDrafter(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	data := filepath.Join(t.TempDir(), "bc")
	if _, err := bc([]string{"-corpus", dir, "-pods", "1", "-teacher", "17l", "-out", data}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	tb, err := os.ReadFile(filepath.Join(data, "table.json"))
	if err != nil {
		t.Fatal(err)
	}
	tab, err := decodeTableForTest(tb)
	if err != nil {
		t.Fatal(err)
	}
	w := writeFixtureNet(t, data, len(tab.Cards), tab.Hash())
	var out bytes.Buffer
	r, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1", "-pilot", "bot", "-baseline", "none",
		"-drafters", "net:" + w, "-builders", "17l"}, &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if r.BuiltPooled.Games != 4 || !strings.Contains(out.String(), "net:w.dpkw") {
		t.Fatalf("games %d\n%s", r.BuiltPooled.Games, out.String())
	}
	if _, err := gate([]string{"-corpus", dir, "-events", "1", "-drafters", "net:" + w}, &bytes.Buffer{}); err == nil {
		t.Fatal("a net drafter with no -builders was accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad")
	os.MkdirAll(bad, 0o755)
	os.WriteFile(filepath.Join(bad, "table.json"), tb, 0o644)
	wb := writeFixtureNet(t, bad, len(tab.Cards), "0000")
	if _, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1", "-baseline", "none",
		"-drafters", "net:" + wb, "-builders", "17l"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "trained against") {
		t.Fatalf("weights for another table: err = %v", err)
	}
}

func decodeTableForTest(b []byte) (*draftfeat.SetTable, error) { return draftfeat.DecodeSetTable(b) }
