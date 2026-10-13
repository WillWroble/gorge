package event

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/adams-shaun/gorge/limited"
)

// DraftDataCSV writes the event's draft as one row per pick in 17Lands'
// draft_data shape: the columns expansion, event_type ("gorge_pod"),
// draft_id (the event seed in decimal), pack_number (the draft round),
// pick_number (the seat's pick within the round) and pick (the picked
// card's name), then one pack_card_<name> column per name in cardNames —
// the count of that name in the pack as the seat saw it at the pick — and
// one pool_<name> column — the count in the seat's pool before the pick.
// cardNames is sorted internally, so the column order is deterministic.
// The pack and pool state is rebuilt by replaying the event's own log, so
// the export depends on nothing but the log.
func DraftDataCSV(e *Event, w io.Writer, cardNames []string) error {
	if !e.cooked {
		return fmt.Errorf("event: DraftDataCSV: event not created")
	}
	names := append([]string(nil), cardNames...)
	sort.Strings(names)

	header := []string{"expansion", "event_type", "draft_id", "pack_number", "pick_number", "pick"}
	for _, n := range names {
		header = append(header, "pack_card_"+n)
	}
	for _, n := range names {
		header = append(header, "pool_"+n)
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("event: DraftDataCSV: %w", err)
	}
	if err := draftDataRows(e, cw, names); err != nil {
		return err
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("event: DraftDataCSV: %w", err)
	}
	return nil
}

// DraftDataCSVRows writes an event's draft_data ROWS with no header line:
// the shape a run appends to an existing -draftlog (a second header line
// mid-file is not a 17Lands draft_data CSV). The column order is the same
// header's. The event must be created.
func DraftDataCSVRows(e *Event, w io.Writer, cardNames []string) error {
	if !e.cooked {
		return fmt.Errorf("event: DraftDataCSVRows: event not created")
	}
	names := append([]string(nil), cardNames...)
	sort.Strings(names)
	cw := csv.NewWriter(w)
	if err := draftDataRows(e, cw, names); err != nil {
		return err
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("event: DraftDataCSVRows: %w", err)
	}
	return nil
}

// draftDataRows replays the log onto a throwaway event and writes one
// draft_data row per pick, capturing each pick's pack and pool from the pod
// state just before the pick is applied. The header line is the caller's.
func draftDataRows(e *Event, cw *csv.Writer, names []string) error {
	re := New()
	idToName := map[string]string{}
	entries := e.Log()
	for i := range entries {
		en := entries[i]
		switch en.Kind {
		case KindPackOpened:
			for _, c := range en.Cards {
				idToName[c.ID] = c.Name
			}
		case KindPicked:
			pack, ok := re.Next(en.Seat)
			if !ok {
				return fmt.Errorf("event: DraftDataCSV: log entry %d: seat %d has no pack at its pick", i, en.Seat)
			}
			name := idToName[en.ID]
			if name == "" {
				return fmt.Errorf("event: DraftDataCSV: log entry %d: picked ID %q is in no logged pack", i, en.ID)
			}
			row := make([]string, 0, 6+2*len(names))
			row = append(row,
				e.set,
				"gorge_pod",
				strconv.FormatUint(e.seed, 10),
				strconv.Itoa(en.Round),
				strconv.Itoa(en.PickNo),
				name,
			)
			row = appendRowCounts(row, pack.Pack, names)
			row = appendRowCounts(row, re.Pool(en.Seat), names)
			if err := cw.Write(row); err != nil {
				return fmt.Errorf("event: DraftDataCSV: %w", err)
			}
		}
		if err := re.Apply(en); err != nil {
			return fmt.Errorf("event: DraftDataCSV: log entry %d does not replay: %w", i, err)
		}
	}
	return nil
}

// appendRowCounts appends one column per name: the count of that card name
// in cards.
func appendRowCounts(row []string, cards []limited.CardRef, names []string) []string {
	counts := make(map[string]int, len(cards))
	for _, c := range cards {
		counts[c.Name]++
	}
	for _, n := range names {
		row = append(row, strconv.Itoa(counts[n]))
	}
	return row
}
