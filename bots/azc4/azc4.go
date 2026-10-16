// Package azc4 hosts the az-c4 policy: the az-redeal search seat with the C4
// network of the aztrain closed-list, census-on loop
// (docs/design/2026-10-14-az-c4-hosted.md), playing CLOSED
// opponent lists only:
//
//	az:sims=100:autopay:oppnodes:oppfull:morekinds:closedlist:topk=6:gonet=c4.dzgw
//
// with the London mulligan by land count (dzgorge play -mull-heuristic) and
// the engine's paid-cost mana census left at its default (on), exactly as the
// loop trained and measured it.
//
// Closed lists are what set it apart from az-c3. The seat knows its own deck
// list; it never knows its opponent's. At every searched decision the
// opponent's unseen hand and library are re-identified from a belief: a list
// drawn from a corpus of 17lands FDN train decks whose colours cover the
// opponent's seen cards, minus what the seat has seen. Because nothing of the
// opponent's real list reaches the search, the policy does not read
// decklists (bots.Info.ClosedLists) and may play humans; the host enforces
// that on the seats it builds (server/host/closedlist.go), not on this declaration.
//
// Like az-c3 it registers only when asked: Enable(path, belief) loads the
// network and registers the policy, and refuses without a belief. The
// implementation is az-c3's (bots/azc3.Hosted); only the entry differs.
//
// Trained on one distribution. The net saw only 17lands FDN limited decks;
// the server keeps it to them (cmd/gorged's -az-c4-decks pool).
package azc4

import (
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azmcts"
	"github.com/adams-shaun/gorge/bots/searchprobe"
)

// Policy is the registry key.
const Policy = "az-c4"

// report is the source every claim below cites.
const report = "docs/design/2026-10-14-az-c4-hosted.md"

var c4 = azc3.NewHosted(azc3.HostedSpec{Info: info(), ErrPrefix: "azc4", Closed: true})

// Enable loads the network at path and registers az-c4 with its opponents'
// lists closed under b. A nil b is refused, as is a path that is not a
// valid .dzgw network; a refusal registers nothing. It must run before the
// server resolves its policy flags, once per process.
func Enable(path string, b *searchprobe.Belief) error { return c4.EnableClosed(path, b) }

// Config is the search config C4 was trained and measured with: az-c3's
// (azc3.Config) closed under b. b must be the belief Enable was given for
// the config to be the hosted one.
func Config(b *searchprobe.Belief) azmcts.SeatConfig { return azc3.ClosedConfig(b) }

func info() bots.Info {
	return bots.Info{
		Name:  Policy,
		Label: "az-c4 (AlphaZero search + trained net, closed decklists)",
		Description: "The az search seat with a small network trained on its own self-play, playing without knowing your deck: " +
			"it knows its own list, and at every searched decision it guesses yours from 28,366 17lands FDN draft decks " +
			"(the ones whose colours match the cards you have shown), redeals your unseen hand and library from that guess, " +
			"and runs 100 MCTS simulations with the net ranking candidates and scoring leaves. " +
			"Trained and measured on 17lands FDN limited decks only; it plays only those.",
		Tier: bots.Experimental,
		Strength: []bots.Measurement{
			{
				Claim:   "67.8% [64.0, 71.4] vs sb-tactical (407-193)",
				Versus:  "sb-tactical",
				Setting: "dzgorge play, az sims=100 closed list with C4, census on, 600 paired games, FDN eval decks, seed 78, seats swapped, London mulligan by land count",
				Source:  report,
			},
			{
				Claim:   "60.8% [56.9, 64.7] vs the same closed search without a net (365-235)",
				Versus:  "az full search, closed list, no net",
				Setting: "dzgorge play, 600 paired games, FDN eval decks, seed 78, seats swapped",
				Source:  report,
			},
			{
				Claim:   "53.7% [49.7, 57.6] vs C3 of the same loop (322-278)",
				Versus:  "az closed list with C3 (census on)",
				Setting: "dzgorge play, 600 paired games, FDN eval decks, seed 78, seats swapped",
				Source:  report,
			},
			{
				Claim:   "55.2% [51.2, 59.1] vs the best census-off closed-list net (331-269)",
				Versus:  "az closed list with cloop-closed c3",
				Setting: "dzgorge play, 600 paired games, FDN eval decks, seed 78, seats swapped",
				Source:  report,
			},
		},
		Cost: bots.Cost{
			MeanMS: 91, P95MS: 136, Scope: "per bot-turn response",
			Note: "hosted smoke: 5 games against a human who only passes, one game at a time on an otherwise idle server; " +
				"the 73 waits of 20 ms or more (the ones carrying a bot search; max 242 ms). All 697 waits that included a bot " +
				"decision: mean 12 ms, p95 92. The training loop measured 110-141 ms mean per searched decision with 24 workers in parallel",
			Source: report,
		},
		Env: true, Search: true, ClosedLists: true,
		Formats:      []string{"constructed"},
		MaxSeats:     2,
		Caretaker:    "bot",
		NeedsAutoPay: true,
	}
}
