package heuristic

import (
	"context"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/seat"
)

// BoardSeat is a seat that can answer from a botpolicy.Board and needs no
// projected View. host builds the Board under the engine lock and skips
// view.Project entirely for such a seat. Implementing it is an opt-in: a
// seat that does not is still handed a full View through Decide.
// decision.Decision keeps the options copy host already makes.
type BoardSeat interface {
	seat.Seat
	DecideBoard(ctx context.Context, b botpolicy.Board, d decision.Decision) (decision.Intent, error)
}
