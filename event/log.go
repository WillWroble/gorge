// Package event implements the draft-event lifecycle of spec §4.7: the
// LOBBY → DRAFT → BUILD → ROUND(1..3) → COMPLETE state machine, the
// append-only log with its Apply mutator, the circle-method pairings, the
// MTR Appendix C standings and the 17Lands draft-data export.
//
// The package is pure: it never plays a game. The caller drives the draft
// through Next/Apply, plays the pending series and feeds the results back as
// GameEnded entries, so the engine is never imported here. Event state
// changes only through Apply; replaying the log reproduces the state
// exactly, and every log line round-trips as JSON.
package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/build"
)

// Kind discriminates one log entry's shape. The kind names follow spec
// §4.7's entry list.
type Kind string

// The log entry kinds.
const (
	KindEventCreated  Kind = "EventCreated"
	KindPackOpened    Kind = "PackOpened"
	KindPicked        Kind = "Picked"
	KindDeckSaved     Kind = "DeckSaved"
	KindReady         Kind = "Ready"
	KindSeriesStarted Kind = "SeriesStarted"
	KindGameEnded     Kind = "GameEnded"
	KindSeriesEnded   Kind = "SeriesEnded"
	KindEventEnded    Kind = "EventEnded"
	KindGameStarted   Kind = "GameStarted"
	KindGameAborted   Kind = "GameAborted"
	// The two timeout entries (E1-8) are appended after every earlier kind.
	// A LobbyTimeout cancels an event nobody joined in time; an IdleTimeout
	// marks one seat the host acted for because its human went quiet. The
	// timer appends entries only; replay never reads the clock.
	KindLobbyTimeout Kind = "LobbyTimeout"
	KindIdleTimeout  Kind = "IdleTimeout"
	// Sideboarded (E3-3) is appended after every earlier kind: it locks one
	// side's deck revision for its series' next game. Auto marks the server
	// keeping the deck for an absent human.
	KindSideboarded Kind = "Sideboarded"
)

// Entry is one log entry: a Kind discriminator plus the fields of that
// kind's shape. Fields a kind does not use stay at their zero values, so
// the struct marshals to one compact JSON line and unmarshals back exactly.
// Entry is a value: Apply stores its own copy, and callers may reuse theirs.
type Entry struct {
	// Kind is the entry's discriminator.
	Kind Kind `json:"kind"`

	// Seed, Set and Seats are the EventCreated fields.
	Seed  uint64     `json:"seed,omitempty"`
	Set   string     `json:"set,omitempty"`
	Seats []SeatSpec `json:"seats,omitempty"`

	// Round is the 1-based draft round or event round the entry belongs to.
	Round int `json:"round,omitempty"`
	// PickNo is the seat's 1-based pick number within a draft round.
	PickNo int `json:"pick_no,omitempty"`
	// Seat is the acting seat of PackOpened, Picked, DeckSaved and Ready.
	Seat int `json:"seat,omitempty"`
	// Game is the 1-based game index within a series.
	Game int `json:"game,omitempty"`
	// Rev is the deck revision of DeckSaved and Ready.
	Rev int `json:"rev,omitempty"`
	// A and B are the two seats of a series; A must be the lower seat.
	A int `json:"a,omitempty"`
	// B is the higher seat of a series.
	B int `json:"b,omitempty"`
	// BestOf is the series length: 1 (Bo1) or 3 (Bo3).
	BestOf int `json:"best_of,omitempty"`
	// Auto marks a pick the drafter made without asking its controller.
	Auto bool `json:"auto,omitempty"`

	// ID is the picked CardRef ID.
	ID string `json:"id,omitempty"`
	// Head is the played game's chain head, carried for -verify.
	Head string `json:"head,omitempty"`
	// Cards is a PackOpened's full CardRef list, logged when the pack is
	// opened so a replay survives a later change to the set data.
	Cards []limited.CardRef `json:"cards,omitempty"`
	// Deck is a DeckSaved's deck list. A nil deck means no deck (a
	// redacted LogFor line, or a non-DeckSaved entry).
	Deck *build.Deck `json:"deck,omitempty"`
	// Winner is the GameEnded winner's seat, or nil for a draw.
	Winner *int `json:"winner,omitempty"`
	// Score is a SeriesEnded's [A wins, B wins].
	Score [2]int `json:"score,omitempty"`
	// Standings is the EventEnded final table, computed by Apply and
	// validated against the caller's value when the caller supplies one.
	Standings []Standing `json:"standings,omitempty"`

	// Attempt is the 1-based attempt of a series game (GameStarted, GameAborted,
	// and optionally GameEnded). A game replayed after a restart or a crash
	// gets the next attempt and a new seed.
	Attempt int `json:"attempt,omitempty"`
	// Table is the host table id a GameStarted attempt is played on.
	Table string `json:"table,omitempty"`
	// GameSeed is a GameStarted attempt's table seed.
	GameSeed uint64 `json:"game_seed,omitempty"`
	// Reason says why a GameAborted attempt produced no result.
	Reason string `json:"reason,omitempty"`
	// Chooser is a best-of-three GameStarted's play/draw chooser: the event
	// seat posed the starting_player decision (game 1: the CR 103.1 toss
	// winner; later games: MTR 2.2's loser of the previous game, or that
	// game's chooser after a draw). nil in every Bo1 log.
	Chooser *int `json:"chooser,omitempty"`
}

// cloneEntry deep-copies an entry's slice and pointer fields so the log
// never aliases a caller's memory.
func cloneEntry(en Entry) Entry {
	if en.Cards != nil {
		en.Cards = append([]limited.CardRef(nil), en.Cards...)
	}
	if en.Deck != nil {
		d := *en.Deck
		d.Main = append([]string(nil), d.Main...)
		en.Deck = &d
	}
	if en.Seats != nil {
		en.Seats = append([]SeatSpec(nil), en.Seats...)
	}
	if en.Winner != nil {
		w := *en.Winner
		en.Winner = &w
	}
	if en.Standings != nil {
		en.Standings = append([]Standing(nil), en.Standings...)
	}
	if en.Chooser != nil {
		c := *en.Chooser
		en.Chooser = &c
	}
	return en
}

// MarshalLog writes entries as JSON lines: one entry per line, each with
// its kind discriminator. ParseLog reads the same shape back.
func MarshalLog(entries []Entry) ([]byte, error) {
	var b bytes.Buffer
	for i := range entries {
		line, err := json.Marshal(&entries[i])
		if err != nil {
			return nil, fmt.Errorf("event: marshal log entry %d: %w", i, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

// ParseLog reads a JSON-lines log written by MarshalLog. An unknown kind or
// a malformed line is an error naming the line.
func ParseLog(data []byte) ([]Entry, error) {
	s := strings.TrimSuffix(string(data), "\n")
	if s == "" {
		return nil, nil
	}
	lines := strings.Split(s, "\n")
	out := make([]Entry, 0, len(lines))
	for i, line := range lines {
		var en Entry
		if err := json.Unmarshal([]byte(line), &en); err != nil {
			return nil, fmt.Errorf("event: parse log line %d: %w", i+1, err)
		}
		switch en.Kind {
		case KindEventCreated, KindPackOpened, KindPicked, KindDeckSaved,
			KindReady, KindSeriesStarted, KindGameEnded, KindSeriesEnded,
			KindEventEnded, KindGameStarted, KindGameAborted,
			KindLobbyTimeout, KindIdleTimeout, KindSideboarded:
		default:
			return nil, fmt.Errorf("event: parse log line %d: unknown kind %q", i+1, en.Kind)
		}
		out = append(out, en)
	}
	return out, nil
}
