package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// smokeDigestGolden is the games.jsonl digest of the sb-uniform,sb-heuristic,bot
// smoke run (1 pair/deck, the Burn and Faeries mirrors, base seed 20260926,
// max turns 40, max intents 8000): every line re-encoded with its wall_ms key
// dropped (the one wall-clock field), the SHA-256 of the canonical lines. It
// pins the -spellbench mode's per-game output: any change to the schedule, the
// seats a policy name builds, the per-seat seed derivation, or the recorded
// extras shows up here. The sb-* and bot seats are registry-built
// (internal/spellbench/registry); the goldens were taken before that refactor,
// so the test also proves the registry resolves the same seats the old
// policies map did.
//
// The golden was re-pinned at the spellbench-prep merge (mrg1): prep's
// builtins.Seat.Refused feature (commit 5f78dfd84, "builtins never lose a
// chosen, payable play" and the sb-actions merge) asks a refused answer again
// before falling back, which changes the per-game fallbacks/first_reject
// extras this digest records. Measured: with sbPlay building seats through
// the old policies map instead of registry.Build the digest is identical, so
// the registry refactor is seat-identical and the move is prep's behaviour
// alone.
//
// Re-pinned again at sb-pursuit: the sb seats no longer pursue a play the
// planner cannot price by naive tapping. rules.PotentialPaymentPlans now
// witnesses mode, {X} and hybrid casts and answers scripted prefixes for
// sources outside its census, the seat decides any other unpriced play with
// the exact search (rules.Engine.PotentialPlayScript), and builtins.Stats,
// which games.jsonl records per seat, gained the script counters.
//
// Re-pinned at the -spellbench-mulligans knob (cli-20260929T235514Z-d23dd19b):
// every games.jsonl row gained a "mulligans" tally field (per-seat keep/mull
// asks and taken answers), the mechanism evidence a mulligan policy A/B
// reads. The field is the only mover: dropping the key from the canonical
// rows reproduces the sb-pursuit golden byte for byte.
//
// Re-pinned for CR 103.8a's turn-1 draw STEP skip (2026-10-06): the
// starting player's first turn no longer has a draw step, so every game's
// intent stream (and the per-game rows recording it) moves.
//
// Re-pinned for the paid-cost mana census defaulting ON
// (cli-20261012T170859Z-8fbcbbcf): the census now counts a payable paid-cost
// source (a self-sacrificing Treasure/Lotus Petal, a fixed PayLife that leaves
// the payer alive) without pre-tapping it, so a seat is offered an activated
// ability the floating pool alone could not pay and auto-pays it; every game's
// option list and intent stream moves. Attributed: reverting only
// rules/genesis.go's `paidManaCensus: !cfg.DisablePaidManaCensus` to `false`
// reproduces the pre-flip golden dc4be0b6 byte for byte.
//
// Re-pinned for botmanual (2026-10-14): the payment planner now offers a plan
// for a graveyard card's flashback cast (decision.OriginFlashback; Burn's Lava
// Dart) and for a fixed-count discard additional cost, and the sb seats cast
// through those plans. Attributed: disabling either half alone still moves
// the digest (flashback off: dd82184d, discard off: 3ae6d37d), while the
// auto-pay bot's tap-gate cast census does not reach this run (disabling it
// alone reproduces the new digest below).
const smokeDigestGolden = "1a9c15a3d4fb3344b582580b86bb7f4a73641310064b713e8fe5345539b48e76"

// TestSpellbenchSmokeDigestIsStable plays the smoke run and compares its
// games.jsonl digest against the golden above.
func TestSpellbenchSmokeDigestIsStable(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	o := sbOpts{
		bots: "sb-uniform,sb-heuristic,bot", pairs: 1, decks: "Burn,Faeries",
		out: t.TempDir(), baseSeed: 20260926,
		// catalog is the -spellbench-catalog flag default. The test
		// constructs sbOpts directly (bypassing flag parsing), so it must
		// set the same default the CLI does or spellbenchExit rejects the
		// empty id. Burn/Faeries come from the pauper-kernel catalog.
		catalog: "pauper-kernel",
	}
	if code := spellbenchExit(o, dir, 2, 40, 8000, "", io.Discard, io.Discard); code != 0 {
		t.Fatalf("spellbenchExit = %d, want 0", code)
	}
	got := digestGamesJSON(t, filepath.Join(o.out, "games.jsonl"))
	if got != smokeDigestGolden {
		t.Fatalf("games.jsonl digest %s != golden %s", got, smokeDigestGolden)
	}
}

// digestGamesJSON hashes a games.jsonl: each line decoded, wall_ms dropped,
// re-encoded (map keys sort), the whole stream SHA-256'd.
func digestGamesJSON(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	h := sha256.New()
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
		delete(row, "wall_ms")
		canonical, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("encoding canonical row: %v", err)
		}
		fmt.Fprintf(h, "%s\n", canonical)
	}
	return hex.EncodeToString(h.Sum(nil))
}
