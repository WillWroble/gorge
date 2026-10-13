// Package eventhost is the draft-event runtime that lives inside gorged:
// the event store (<dir>/events/<id>/event.json + log.jsonl), create and
// join, the full DRAFT phase (humans pick over the API, bots pick
// synchronously and deterministically between their picks), per-event SSE
// frames, and restart resume for LOBBY and DRAFT. BUILD (E1-4) and the
// games (E1-5) slot into the two stubs this package carries
// (advanceBuild/advanceRounds and Options.MintSeat).
//
// The package never imports net/http: handlers read an Error's HTTP status
// and code through the Status/Code methods.
//
// # Concurrency
//
// One mutex guards the whole Host (map of runs, closing flag) and one mutex
// guards each run. Host.mu is a leaf: no code path holding a run.mu may
// take Host.mu, so the exported methods take Host.mu only to find the run,
// release it, and then take run.mu for the whole call. run.mu also orders
// against the host.Registry (Session/Publish now; AddTable/Start in E1-5):
// take run.mu first, call the Registry beneath it, never the reverse.
//
// # Clock
//
// eventhost is the only package of the event stack that may read the
// clock, and today it reads it for nothing: PickTimeout (E3, pick timers)
// is the seam, and New refuses a non-zero one.
package eventhost
