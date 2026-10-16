package draftnet

import (
	"encoding/json"
	"fmt"

	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// MetaTableHash is the .dpkw meta key naming the draftfeat.SetTable the
// weights were trained against (SetTable.Hash). The card embedding's rows
// are that table's rows, so weights paired with any other table read the
// wrong card's embedding at every token.
const MetaTableHash = "table_hash"

// CheckTable refuses a table the weights were not trained against: the
// meta must be a JSON object carrying MetaTableHash equal to t.Hash(), and
// the card embedding must have exactly one row per table card.
func (n *Net) CheckTable(t *draftfeat.SetTable) error {
	if t == nil {
		return fmt.Errorf("draftnet: no set table")
	}
	var meta map[string]any
	if err := json.Unmarshal(n.Meta, &meta); err != nil {
		return fmt.Errorf("draftnet: meta is not a JSON object: %w", err)
	}
	h, _ := meta[MetaTableHash].(string)
	if h == "" {
		return fmt.Errorf("draftnet: meta carries no %q; cannot check the weights against table %s", MetaTableHash, t.Hash())
	}
	if h != t.Hash() {
		return fmt.Errorf("draftnet: weights were trained against table %s, given table %s", h, t.Hash())
	}
	if n.cards != len(t.Cards) {
		return fmt.Errorf("draftnet: card embedding has %d rows, table %s has %d cards", n.cards, t.Hash(), len(t.Cards))
	}
	return nil
}
