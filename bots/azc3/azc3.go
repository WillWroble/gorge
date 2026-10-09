// Package azc3 hosts the az-c3 policy: the az-redeal search seat with the c3
// network of the aztrain self-play loop (docs/superpowers/reports/
// 2026-10-09-az-c3-hosted.md) in place of the heuristic prior and leaf.
//
// It is the search seat exactly as the loop trained and measured it, honest
// worlds throughout:
//
//	az:sims=100:autopay:oppnodes:oppfull:morekinds:topk=6:gonet=c3.dzgw
//
// with the London mulligan by land count (dzgorge play -mull-heuristic). The
// net is the in-process mlp (internal/gonet) read from a .dzgw file, shared by
// every table in the process.
//
// Unlike every other entry it registers only when asked: Enable(path) loads
// the network and registers the policy, and gorged calls it for -az-c3-net.
// Linking the package registers nothing, because gorged offers every
// registered entry by default and an offered policy with no network behind it
// would fail at table creation. A network is a deployment artifact, not a
// source file.
//
// Trained on one distribution. The net saw only 17lands FDN limited decks
// (31,516 of them); it has never seen a constructed deck, and what it does
// there is unmeasured. The server keeps the policy to the decks it was
// trained on: see the deck pool in cmd/gorged.
package azc3

import (
	"errors"
	"fmt"
	"sync"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azredeal"
	"github.com/adams-shaun/gorge/internal/azmcts"
	"github.com/adams-shaun/gorge/internal/gonet"
	"github.com/adams-shaun/gorge/internal/policynet"
	"github.com/adams-shaun/gorge/seat"
)

// Policy is the registry key.
const Policy = "az-c3"

var (
	mu      sync.Mutex
	enabled bool
	model   *policynet.Model
)

// Enable loads the network at path and registers the az-c3 policy. It must run
// before the server resolves its policy flags, once per process; a path that
// is not a valid .dzgw network is an error and registers nothing.
func Enable(path string) error {
	mu.Lock()
	defer mu.Unlock()
	if enabled {
		return errors.New("azc3: already enabled")
	}
	m, err := gonet.Model(path)
	if err != nil {
		return fmt.Errorf("azc3: %w", err)
	}
	model = m
	bots.Register(bots.Entry{Info: info(), New: newSeat})
	enabled = true
	return nil
}

func info() bots.Info {
	return bots.Info{
		Name:  Policy,
		Label: "az-c3 (AlphaZero search + trained net)",
		Description: "The az search seat in its honest world with a small network trained on its own self-play: " +
			"at every searched decision it runs 100 MCTS simulations over freshly redealt worlds rebuilt from its own " +
			"observations, the net ranking the candidates and scoring the leaves, and the opponent modelled as a " +
			"searching player. Trained and measured on 17lands FDN limited decks only; play it on those.",
		Tier: bots.Experimental,
		Strength: []bots.Measurement{
			{
				Claim:   "63.0% [59.1, 66.8] vs sb-tactical (378-222)",
				Versus:  "sb-tactical",
				Setting: "dzgorge play, az sims=100 with c3, 600 paired games, FDN eval decks, seats swapped, London mulligan by land count",
				Source:  "docs/superpowers/reports/2026-10-09-az-c3-hosted.md",
			},
			{
				Claim:   "55.8% [51.8, 59.8] vs the same search without a net (335-265)",
				Versus:  "az-redeal-class full search, no net",
				Setting: "dzgorge play, 600 paired games, FDN eval decks, seats swapped",
				Source:  "docs/superpowers/reports/2026-10-09-az-c3-hosted.md",
			},
		},
		Cost: bots.Cost{
			MeanMS: 314, Scope: "per searched decision",
			Note:   "mean of the c3-vs-sb-tactical evaluation, 24 workers in parallel (202-344 ms across the three evaluations); p95 not measured",
			Source: "docs/superpowers/reports/2026-10-09-az-c3-hosted.md",
		},
		Env: true, Search: true,
		Formats:      []string{"constructed"},
		MaxSeats:     2,
		Caretaker:    "bot",
		NeedsAutoPay: true,
	}
}

// Config is the search config the c3 net was trained and measured with:
// azredeal.Hosted (honest redeal world, no clairvoyant source) plus the keys
// of the spec in the package comment. cmd/dzgorge/policy.go maps the same
// keys onto the same fields; TestConfigMatchesTheMeasuredSpec pins them.
func Config() azmcts.SeatConfig {
	cfg := azredeal.Hosted()
	cfg.Search.Kinds = azmcts.FullKinds() // :morekinds
	cfg.Search.AutoPayment = true         // :autopay
	cfg.Search.OpponentNodes = true       // :oppnodes
	cfg.Search.OpponentMirror = true      // :oppfull
	cfg.Search.PriorTopK = 6              // :topk=6
	cfg.Mulligan = botpolicy.MulliganLands
	return cfg
}

// newSeat is the registry factory. The seat pays its own mana through the
// engine's automatic payment plans, as it did in training, so a table built
// without them is refused here rather than played at a handicap.
func newSeat(o bots.Options) (seat.Seat, error) {
	if !o.AutoPayMana {
		return nil, errors.New("az-c3 needs automatic mana payment (gorged -bot-auto-mana, the default)")
	}
	mu.Lock()
	m := model
	mu.Unlock()
	if m == nil {
		return nil, errors.New("az-c3: no network loaded (Enable was not called)")
	}
	return azredeal.NewWithModel(o, Config(), m)
}
