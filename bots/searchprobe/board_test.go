package searchprobe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestHistoryDigestMatchesTheJSONBoardEncoding pins the seed root across the
// change that made Frame.Board typed: before it, Sample hashed
// json.Marshal(h) with every board as its JSON bytes. The golden digests are
// that old hash of the bench fixture's history and three of its prefixes,
// measured on the tree before the change (2026-09-30, 308f7e0b9). If one
// moves, every sampled world, label and seeded search moves with it.
//
// Re-pinned 2026-10-06 for CR 103.8a's turn-1 draw STEP skip: the bench
// fixture game loses the starting player's turn-1 draw-step priority frames
// (122 -> 120 frames), so every prefix's history moves. The legacy-encoding
// equivalence itself stays held on every prefix of a real game by
// TestHistoryChainEqualsTheLegacyEncodingOnEveryPrefix; these goldens are
// the new fixture's chained digests.
//
// Re-pinned 2026-10-12 (eng-turnwatch): view.PlayerView gained the four
// per-turn watcher fields (SpellsCastThisTurn, LifeGainedThisTurn,
// LifeLostThisTurn, TokensCreatedThisTurn), which every board's JSON and
// canonical encoding now carries, so every prefix digest moves; the
// legacy-encoding equivalence tests above still hold.
//
// Re-pinned 2026-10-16 (token-art-pt-key): view.CardView gained PrintedPT
// (the face's printed P/T, so the token-art cache key stops keying on the
// current, counter-modified value), which every board's JSON and canonical
// encoding now carries, so every prefix digest moves. Attributed by hiding
// only that field (json:"-"): the old digests return byte-for-byte.
func TestHistoryDigestMatchesTheJSONBoardEncoding(t *testing.T) {
	f := benchRoot(t)
	golden := map[int]string{
		1:   "5074d37819a5cb8a83a96aa5edcff463a4db54036de9802e82ae134fc2c5224a",
		2:   "5782b165ba5cc5e68722a37cfa33ac960a9b6428564b4e2409918141b82aaaa4",
		17:  "355ddaed8cfd831073e1d22407c556dd49277ac80b0d75f13c83d8d083d4e8e3",
		120: "90d19be27cae25c186c00c8a4eb9565febfe7dc6642ff93a11fa0035f043e3cf",
	}
	if len(f.h.Frames) != 120 {
		t.Fatalf("bench fixture has %d frames, the goldens were measured on 120", len(f.h.Frames))
	}
	for _, n := range []int{1, 2, 17, 120} {
		h := f.h
		h.Frames = h.Frames[:n]
		if chainState(h) == nil {
			t.Fatalf("prefix %d is not one chain", n)
		}
		got, err := historyDigest(h)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(got[:]) != golden[n] {
			t.Errorf("history digest of %d frames = %x, want %s", n, got, golden[n])
		}
	}
}

type legacyHistory struct {
	Actor   state.PlayerID
	Frames  []legacyFrame
	Answers map[int][]Action
}

// legacyDigest is the old seed root: sha256(json.Marshal(h)) with each
// board as the JSON the capture encoded (Board.raw, retainJSON).
func legacyDigest(t *testing.T, h History) [sha256.Size]byte {
	t.Helper()
	lh := legacyHistory{Actor: h.Actor, Answers: h.Answers}
	for _, f := range h.Frames {
		if f.Board.raw == nil {
			t.Fatal("frame captured without retainJSON")
		}
		lh.Frames = append(lh.Frames, legacyFrame{Board: f.Board.raw, Identities: f.Identities, Events: f.Events, Decision: f.Decision})
	}
	b, err := json.Marshal(lh)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// TestHistoryChainEqualsTheLegacyEncodingOnEveryPrefix replays a real game,
// keeping each board's JSON beside the chain, and requires the chained digest
// to equal the legacy encoding's hash at every prefix, for a history copied
// frame by frame, and for a branch continued on a cloned collector (the
// hindsight shape). It also checks each board's facts and digests against
// its bytes.
func TestHistoryChainEqualsTheLegacyEncodingOnEveryPrefix(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	names := []string{"mono-red-prowess", "mono-blue-tempo"}
	decks := make([][]*cards.Card, len(names))
	for i, n := range names {
		var err error
		if decks[i], err = testutil.LoadRepoDeck(reg, n); err != nil {
			t.Fatal(err)
		}
	}
	e := rules.New(rules.Config{Seed: 30_000_001, Names: names, Decks: decks, Tokens: reg.Tokens})
	e.Advance()
	c := NewCollector(1)
	c.retainJSON = true
	h := History{Actor: 1, Answers: make(map[int][]Action)}
	rngs := BotRandoms(9, len(names))
	board := botpolicy.NewBoard(len(names))
	pos, branched := 0, false
	for i := 0; i < 160; i++ {
		frame, err := c.Capture(e, e.L.Events[pos:])
		if err != nil {
			t.Fatal(err)
		}
		// The digests are the canonical encoding's, whose equality is the
		// JSON's: re-encoding the decoded JSON (and its stripped form) must
		// reproduce them.
		for _, want := range []struct {
			raw []byte
			sum [sha256.Size]byte
		}{{frame.Board.raw, frame.Board.Sum}, {stripPotentialActions(frame.Board.raw), frame.Board.Stripped}} {
			var v view.View
			if err := json.Unmarshal(want.raw, &v); err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(canonOf(t, &v)) != want.sum {
				t.Fatalf("frame %d: board digests disagree with its encoding", i)
			}
		}
		var decoded struct {
			Step    string `json:"step"`
			Players []struct {
				Seat        state.PlayerID `json:"seat"`
				LibrarySize int            `json:"library_size"`
				HandSize    int            `json:"hand_size"`
				Hand        []struct {
					ID uint32 `json:"id"`
				} `json:"hand"`
			} `json:"players"`
			Stack []struct {
				ID uint32 `json:"id"`
			} `json:"stack"`
		}
		if err := json.Unmarshal(frame.Board.raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Step != frame.Board.Step || len(decoded.Players) != len(frame.Board.Players) || len(decoded.Stack) != len(frame.Board.Stack) {
			t.Fatalf("frame %d: board facts disagree with its encoding", i)
		}
		for j, p := range decoded.Players {
			q := frame.Board.Players[j]
			if p.Seat != q.Seat || p.LibrarySize != q.LibrarySize || p.HandSize != q.HandSize || len(p.Hand) != len(q.Hand) || (p.Hand == nil) != (q.Hand == nil) {
				t.Fatalf("frame %d seat %d: board facts disagree with its encoding", i, j)
			}
			for k := range p.Hand {
				if p.Hand[k].ID != q.Hand[k].ID {
					t.Fatalf("frame %d seat %d: hand refs disagree", i, j)
				}
			}
		}
		h.Frames = append(h.Frames, frame)
		d := e.Pending()
		if d == nil {
			break
		}
		in := botpolicy.Decide(botpolicy.BoardFromGameInto(e.G, e, d.Player, &board), d, rngs[d.Player])
		if d.Player == 1 {
			if h.Answers[i], err = c.Actions(d, in); err != nil {
				t.Fatal(err)
			}
		}
		got, err := historyDigest(h)
		if err != nil {
			t.Fatal(err)
		}
		if got != legacyDigest(t, h) {
			t.Fatalf("prefix of %d frames: chained digest differs from the legacy encoding", len(h.Frames))
		}
		if !branched && i == 40 {
			branched = true
			// A branch: a copied prefix continued by a cloned collector on a
			// cloned engine, one step further.
			be, bc := e.Clone(), c.Clone()
			bpos := len(be.L.Events)
			if err := be.Submit(in); err != nil {
				t.Fatal(err)
			}
			bf, err := bc.Capture(be, be.L.Events[bpos:])
			if err != nil {
				t.Fatal(err)
			}
			bh := History{Actor: 1, Frames: append(append([]Frame(nil), h.Frames...), bf), Answers: h.Answers}
			bd, err := historyDigest(bh)
			if err != nil {
				t.Fatal(err)
			}
			if chainState(bh) == nil || bd != legacyDigest(t, bh) {
				t.Fatal("branched history: chained digest differs from the legacy encoding")
			}
		}
		pos = len(e.L.Events)
		if err := e.Submit(in); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.Frames) < 100 {
		t.Fatalf("fixture too short: %d frames", len(h.Frames))
	}
	// A reordered history is not a chain: it falls back to hashing its own
	// encoding rather than claiming a chain it is not.
	swapped := History{Actor: 1, Frames: []Frame{h.Frames[1], h.Frames[0]}, Answers: h.Answers}
	if chainState(swapped) != nil {
		t.Fatal("a reordered history passed as one chain")
	}
	if chainState(History{Actor: 0, Frames: h.Frames[:1]}) != nil {
		t.Fatal("another seat's chain passed as this seat's")
	}
}
