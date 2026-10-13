package eventhost

// advanceRounds drives the ROUND(1..3) phases: series starts, game tables
// and results. E1-5 replaces this stub.
func (r *run) advanceRounds() bool { return false }

// abortInFlight aborts every game attempt a restart caught in flight, so
// each replays from scratch with a fresh seed. E1-5 replaces this stub.
func (r *run) abortInFlight() {}
