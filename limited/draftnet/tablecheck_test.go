package draftnet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/limited/draftfeat"
)

func checkTable(t *testing.T, n int) *draftfeat.SetTable {
	t.Helper()
	var rows []draftfeat.Card
	for i := 0; i < n; i++ {
		rows = append(rows, draftfeat.Card{Name: fmt.Sprintf("C%02d", i), Mask: 1 << (i % 5), MV: 2, Types: draftfeat.TypeCreature, V: float64(i%7) - 3})
	}
	tab, err := draftfeat.NewSetTable("TST", rows, draftfeat.DefaultModel(), []float64{0})
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

// TestCheckTable: weights load only against the table their meta names.
func TestCheckTable(t *testing.T) {
	n, _ := loadFixture(t)
	tab := checkTable(t, n.Cards())
	other := checkTable(t, n.Cards()+1)
	n.Meta = []byte(`{"set":"TST"}`)
	if err := n.CheckTable(tab); err == nil || !strings.Contains(err.Error(), "table_hash") {
		t.Fatalf("meta without a table hash: err = %v", err)
	}
	n.Meta = []byte(`{"table_hash":"` + tab.Hash() + `"}`)
	if err := n.CheckTable(tab); err != nil {
		t.Fatalf("matching table: %v", err)
	}
	if err := n.CheckTable(other); err == nil {
		t.Fatal("a different table was accepted")
	}
	n.Meta = []byte(`{"table_hash":"` + other.Hash() + `"}`)
	if err := n.CheckTable(other); err == nil || !strings.Contains(err.Error(), "rows") {
		t.Fatalf("right hash, wrong embedding size: err = %v", err)
	}
	n.Meta = []byte(`not json`)
	if err := n.CheckTable(tab); err == nil {
		t.Fatal("non-JSON meta was accepted")
	}
}
