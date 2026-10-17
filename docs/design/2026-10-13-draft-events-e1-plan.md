# E1 · Hosted draft events: plan

Base: `main@4338fbe3c7721e4ed01c094ce28f18d263dbbac0`. Every `path:line` below was
read at that SHA (`git show 4338fbe3c:<path>` or `git grep -n`). Anything not
read from code is marked **INFERRED** or **Hypothesis**.

Design of record: `docs/design/2026-10-13-draft-events-design.md`
(§5 E1, decisions Q7–Q11). The operator's E1 decisions in the brief are taken
as given. Where the spec and the code disagree, §9 below names the conflict and
the smallest fix this plan adopts.

---

## 1. What E1 delivers

One gorged process hosts 4-seat FDN (or BLB) draft events: 0 or 1 human plus
bots (E1 caps humans at 1; the code is written for N and the cap is one
option). A human gets an invite link (seat token), types a display name, drafts
through HTTP (`GET pack` / `POST pick`, stale pick = 409), builds (pool, bot
suggestion, auto lands, validate, save, ready) and then plays a Bo1 round robin
whose games are ordinary host tables; bot-vs-bot games are spectatable on
existing `/t/{table}` pages. Events persist under `<dir>/events/<id>/`
(`event.json`, append-only `log.jsonl`, `decks/`) and resume after a restart;
a game in flight at restart is logged aborted and replayed with a new seed.
No UI (E2).

## 2. Packages and responsibilities

| package | E1 change | ticket |
|---|---|---|
| `event` (pure) | 2 new log kinds (`GameStarted`, `GameAborted`), 4 new `Entry` fields, read accessors, `LogFor` redaction | E1-1 |
| `protocol` | new file `server/protocol/draftevent.go`: 4 frame kinds + wire structs; goldens; `cmd/gentypes` roots; `web/src/protocol.ts` regenerated | E1-1 |
| `host` | `TableConfig.SeatBotPolicies` (per-seat bot policy), `TableConfig.Event` (event-owned table exempt from `MaxOnDemandTables`), `Registry.Publish` (push an embedder frame to a session) | E1-2 |
| `eventhost` (new, top level) | the runtime: store, lobby/join, draft, build, rounds, resume, SSE fan-out, deck loader, OnMatchEnd relay | E1-3, E1-4, E1-5 |
| `server/host/httpapi` | `/api/events/...` routes behind an `EventService` interface (`Options.Events`) | E1-6 |
| `cmd/gorged` | `-events`, `-max-events` flags; wiring; smoke test; the acceptance e2e lives in `server/host/httpapi` | E1-7 |

Dependency direction (no cycles): `eventhost → host, protocol, event, lab/draft/limited/*, cards, deck, bots, view`;
`httpapi → protocol` only for events (it does **not** import `eventhost`; it
defines the `EventService` interface and eventhost satisfies it);
`cmd/gorged → eventhost, httpapi`. The compile-time check
`var _ httpapi.EventService = (*eventhost.Host)(nil)` lives in
`cmd/gorged/events.go` (E1-7), the first package that imports both.

## 3. Existing mechanisms relied on (verified)

### 3.1 The pure E0 packages

- `event.Event` is the only mutator path: `Apply` (`lab/draft/event/event.go:223`),
  `Replay` (`:261`), `ReplayLog` (`:272`); log kinds and `Entry`
  (`lab/draft/event/log.go:28-38`, `:44-88`); `ParseLog` whitelists kinds
  (`lab/draft/event/log.go:138-143`) so new kinds must be added there.
- `Next`/`Pool`/`SavedDeck`/`PendingSeries` (`lab/draft/event/event.go:133,142,151,163`);
  `Pairings` (`:184`, R1 0–3,1–2; R2 0–2,1–3; R3 0–1,2–3).
- `applyPicked` rejects a wrong pick number with a **plain** error, not
  `draft.ErrStale` (`lab/draft/event/event.go:382-384`), so eventhost must pre-check
  staleness against `Next` to answer 409 `stale_pick` (draft's own
  `ErrStale`/`ErrNotInPack`/`ErrNotYourTurn` are at `lab/draft/limited/draft/draft.go:35-39`).
- `applyDeckSaved` resets ready (`lab/draft/event/event.go:413`); `applyReady` starts
  round 1 when all four are ready (`:441-443`); `applySeriesEnded` advances the
  round (`:559`, `roundAdvance` `:578`); `EventEnded` computes standings
  (`:594-614`).
- `seriesFor` lazily **creates** a series entry (`lab/draft/event/event.go:467-479`): new
  read accessors must read `e.series[p]` directly, not call it.
- The event package's purity test forbids `time`, `os`, `host`, `bots`, ...
  (`lab/draft/event/imports_test.go:15-33`).
- `booster.OpenPod(spec, seed, seats, packs, ids, colourOf)`
  (`lab/draft/limited/booster/pod.go:21`); draftsim's colour adapter
  (`lab/cmd/draftsim/event.go:152-171`) and pack-open order seat-major
  (`lab/cmd/draftsim/event.go:74-80`).
- `pickbot.Lookup(name) (Drafter, Builder, bool)` returns the registered
  zero-value instances (`lab/draft/limited/pickbot/registry.go:103-122`); the instances
  need a `CardLookup` injected (`RatingColour.Lookup`
  `lab/draft/limited/pickbot/drafter.go:16-20`, `Best23.Lookup`
  `lab/draft/limited/pickbot/builder.go:38-42`), which is how draftsim builds them
  (`lab/cmd/draftsim/run.go:83-89`).
- `build.Validate` / `AutoLands` / `ToDeckFile` (`lab/draft/limited/build/build.go:42,69,122`).
  Pip counting for `AutoLands` mirrors `factsOf` (`lab/draft/limited/pickbot/drafter.go:43-58`,
  `cards.ManaCostChroma` `engine/cards/face.go:718`).
- `bots.ReadsDecklists(name)` (`bots/registry.go:149`), `bots.Normalize`
  (`:117`), `bots.Lookup` (`:115`, `Info.Env/Search/Formats/MaxSeats`
  `:34-57`). Every Search policy registered at this SHA is also Env
  (`bots/azc3/azc3.go:98`, `bots/azredeal/azredeal.go:62`,
  `bots/sbsearch/sbsearch.go:65`, `bots/search/search.go:47`); so refusing
  Env at events also keeps the search-slot gate (`server/host/registry.go:374-382`,
  `server/host/botenv.go:210`) out of E1.

### 3.2 host

- `Options` are fixed at `host.New` (`server/host/registry.go:51-236`, `New` `:271`);
  `LoadDeck` is required (`:272-274`); `OnMatchEnd` (`:218-221`, type
  `:40-47`) "must not call back into the Registry or block".
- `OnMatchEnd` fires from `settleNaturalEndOrUndo` (`server/host/match.go:1035`),
  `abort` (`:1046`) and `crash` (`:1070`) via `observeMatchEnd`
  (`server/host/snapshot.go:65-73`) **after** `m.mu` is released; and from
  `load()` for a sidecar rewritten aborted (`server/host/restart.go:119`).
- `AddTable` validates via `TableConfig.validated(LoadDeck)` which **loads
  every deck** (`server/host/registry.go:307-309`, `server/host/table.go:308-311`), so an
  event deck must be loadable before its table is added.
- `MaxOnDemandTables` counts every `OnDemand` table, finished or not, for the
  life of the process (`server/host/registry.go:321-331`; finished tables are never
  removed — `server/host/table.go:150-157`).
- Restart drops `OnDemand` tables before validation (`server/host/restart.go:72-75`).
  A non-OnDemand human table is revived by `StartAll` for a **new** match
  (`server/host/registry.go:618-623` comment and `singleShot`), which is why event
  tables must be OnDemand.
- Deck rotation: seat `i` of match `k` plays `Decks[(i+k)%len]`
  (`server/host/match.go:165`); for match 1 of a 2-seat table, `Decks: [B, A]` gives
  seat 0 deck A (`cmd/gorged/game.go:141-143` relies on exactly this).
  `PlayerNames[i]` does not rotate (`server/host/match.go:153-158`).
- `MatchSeed(tableSeed, k)` (`server/host/seed.go:15`).
- Bot seats are built from ONE policy (`server/host/match.go:661`,
  `defaultSeatsWithAutoPayMana` `:329-347`); `Humans` slots get a `HumanSeat`
  (`:672-674`) whose caretaker uses `t.cfg.BotPolicy` (`:694`) and fires only
  after `Options.ThinkTimeout` (`:698`, `server/host/registry.go:127-139`).
- The Env refusal at a human table and the `AllowDecklistBots` opt-in
  (`server/host/table.go:262-269`).
- Sessions: `OpenSession`/`Session(id)` (`server/host/session.go:42,64`); the
  non-blocking ring push is unexported (`(*Session).push` `:122`) and
  overflow cleanup is `dropOverflowed` (`:314`). `Subscribe` accepts only
  `overview`/`focus` and table IDs (`:230-275`).
- `TableConfig` field block ends at `AllowDecklistBots` (`server/host/table.go:216`).

### 3.3 httpapi and gorged

- Routes are registered in `newHandler` (`server/host/httpapi/handler.go:89-117`),
  with 405 twins (`:106-111`) and a JSON 404 catch-all (`:112-114`).
  `Options` (`:26-60`); `CreateGame` is the precedent for an optional
  embedder-supplied backend (nil = 404) (`:40-50`). `decodeBody`
  (`server/host/httpapi/rest.go:353`), `writeJSON`/`writeError`
  (`server/host/httpapi/errors.go:18,28`).
- The SSE stream is session-based (`server/host/httpapi/sse.go:23-138`); frame ids
  come from the session push, so embedder frames pushed through the same
  session resume through `Last-Event-ID` for free.
- Table seat tokens: `seatGate.mint(table, seat)` (`cmd/gorged/seats.go:83-95`),
  resolved from `Authorization: Bearer` or `?token=` (`:62-70`, `:116-123`);
  process-local, never persisted (`:17-20` doc). gorged creates a gate only
  for `-humans` or `-vsbot` (`cmd/gorged/main.go:466-491`).
- gorged builds the loader and the registry in this order:
  `load := deckLoaders(...)` (`cmd/gorged/main.go:446`),
  `host.New(c.hostOptions(reg, load))` (`:447`, `hostOptions` `:805-829`),
  `StartAll` (`:462`), `httpapi.Options{...}` (`:465`), mux (`:550`).
  `hostOptions` does not set `OnMatchEnd` today.
- Flag defaults: `-dir gorged-data` (`:308`), `-mulligans 1` (`:314`),
  `-max-on-demand-tables 32` (`:328`), `-bot-policy lethal-pressure`
  (`host.LethalPressurePolicy`).

### 3.4 Gates every ticket must satisfy

- `internal/archtest` time allow-list contains exactly
  `module + "/eventhost"` (`internal/archtest/arch_test.go:145`): only the
  package at import path `.../eventhost` may import `time`; **no
  sub-packages of eventhost may import time**, so eventhost stays one
  package. No new allow-list entries, ever.
- `host`, `server/host/httpapi`, `cmd/gorged` may not depend on
  `testkit/testutil` (`internal/archtest/arch_test.go:166-168`, transitive).
  Since gorged will import eventhost, **eventhost's non-test code must not
  import `testkit/testutil`** (tests may).
- `TestNoLegacyMathRand` (`internal/archtest/arch_test.go:402`): use
  `math/rand/v2` with `rand.NewPCG`.
- `cmd/pkgmap` `TestClassificationIsComplete` (`cmd/pkgmap/main_test.go:14`,
  checks `cmd/pkgmap/main.go:334-358`): every package must have an entry in
  `docs/site/architecture/packages.tiers.json` and every entry must exist —
  so the entry is added **in the same ticket** that creates the package.
- `make lint` runs `go run ./cmd/gentypes -check` (`Makefile:491`):
  `web/src/protocol.ts` must be regenerated (`make gentypes`, `Makefile:281`)
  and committed with any protocol change. `cmd/gentypes` hard-codes the
  root list and the `FrameType` union (`cmd/gentypes/main.go:25-60`), and
  `TestFrameTypeUnionListsEveryConstant` (`cmd/gentypes/main_test.go:27`)
  lists the constants it checks.
- Protocol goldens: one `server/protocol/testdata/<frame type>.json` per fixture
  frame (`server/protocol/protocol_test.go:23-80`), regenerated with
  `go test ./server/protocol -run TestGoldens -update`.
- Hot files touched: `web/src/protocol.ts` (generated; hot per
  `ops/scripts/hotfiles-notes.json`) — E1-1 only. `docs/agents/repo-map-engine.md`
  gets one row (E1-3). `AGENTS.md` is never touched.

## 4. API of record

### 4.1 event (E1-1)

```go
// log.go — appended after KindEventEnded
const (
    KindGameStarted Kind = "GameStarted" // one attempt at a series game is being played on Table
    KindGameAborted Kind = "GameAborted" // that attempt ended without a result (restart, crash, table refused)
)
// Entry — four fields appended after Standings, all omitempty so every E0 log line is byte-identical
Attempt  int    `json:"attempt,omitempty"`   // GameStarted/GameAborted/GameEnded: 1-based attempt
Table    string `json:"table,omitempty"`     // GameStarted: host table id
GameSeed uint64 `json:"game_seed,omitempty"` // GameStarted: the table seed
Reason   string `json:"reason,omitempty"`    // GameAborted: why

type GameAttempt struct {
    Round, A, B, Game, Attempt int
    Table string
    Seed  uint64
}
type SeriesOutcome struct {
    Round, A, B int
    Score       [2]int
    Winner      int // -1 drawn series
}
func (e *Event) InFlight() []GameAttempt        // current round, pairing order
func (e *Event) NextAttempt(a, b int) int       // 0 when (a,b) is not a current pairing
func (e *Event) Started(a, b int) bool          // SeriesStarted logged for the current round's (a,b)
func (e *Event) Results() []SeriesOutcome       // ended series, log order
func (e *Event) SavedRev(seat int) int          // 0 = none
func (e *Event) ReadyRev(seat int) int          // 0 = not ready
func (e *Event) Queued(seat int) int            // packs in seat's draft queue; 0 outside DRAFT
func (e *Event) LogFor(viewer int) []Entry      // viewer -1 = spectator
```

Apply rules (precise text in E1-1): `GameStarted` needs a started, unended
series with no attempt in flight, `Game == len(games)+1`,
`Attempt == attempts+1`, non-empty `Table`. `GameAborted` needs the matching
in-flight attempt and a non-empty `Reason`. `GameEnded` keeps every E0 rule;
when an attempt is in flight it must name that `Game` (and `Attempt` when
non-zero) and clears it; with nothing in flight it is accepted as before
(draftsim never logs `GameStarted`). `SeriesEnded` is refused while an attempt
is in flight.

### 4.2 protocol (E1-1) — `server/protocol/draftevent.go`

Frame kinds `TEventState "event_state"`, `TDraftPack "draft_pack"`,
`TPodProgress "pod_progress"`, `TSeriesUpdate "series_update"`; event frames
use `NewFrame(t, "", 0, 0, body)` (no table/match/seq; the body carries
`Event`). Wire structs (exact field list and JSON tags are in E1-1):
`DraftCard, EventSeat, EventSeries, EventStanding, EventState, DraftPack,
PodProgress, SeriesUpdate, CreateEventSeat, CreateEventRequest, EventInvite,
CreateEventResponse, JoinEventRequest, PickRequest, DeckRequest, DeckCheck,
EventPool, ReadyRequest, EventLog, EventSubscribe`. protocol imports nothing
new (the types are self-contained mirrors, not `limited`/`event` types, so the
TS generator never sees `build.Deck`'s untagged fields).

### 4.3 host (E1-2)

```go
// table.go TableConfig, appended after AllowDecklistBots
SeatBotPolicies []string `json:"seat_bot_policies,omitempty"`
Event           string   `json:"event,omitempty"`

// publish.go (new)
func (r *Registry) Publish(s *Session, f protocol.Frame) bool
```

`SeatBotPolicies[i]`, when non-empty, is the policy of bot slot `i` (no
rotation, like `PlayerNames`); empty entries and a short slice fall back to
`BotPolicy`; an entry may not name a human slot, an unknown policy, or an Env
policy; `len <= Seats`. With the field empty the seat build is byte-identical
(the existing call at `server/host/match.go:661` is untouched on that path). `Event`
non-empty requires `OnDemand` and not `Perpetual`, and exempts the table from
the `MaxOnDemandTables` count (both the admission check and the counting loop,
`server/host/registry.go:321-331`).

### 4.4 eventhost (E1-3/4/5)

```go
package eventhost

type Options struct {
    Registry       *host.Registry  // required: games + SSE sessions
    Store          *Store          // required: OpenStore result; its DeckLoader is chained into the Registry
    Cards          *cards.Registry // required: colour balance, pickbot facts, deck resolution
    Policies       []string        // offered bot game policies; empty = every registered non-Env policy
    DefaultPolicy  string          // bot seat with no policy; also the human-seat caretaker policy
    MaxHumans      int             // 0 = 1 (E1); E3 raises it
    MaxLiveEvents  int             // events not COMPLETE; 0 = 4
    MaxAttempts    int             // crashed-game replays before a draw is recorded; 0 = 3
    Mulligans      int             // copied onto every game table
    AutoMana       bool
    BotAutoPayMana bool
    BotCommonSense bool
    MintSeat       func(table host.TableID, seat int) (string, error) // human game-seat token (gorged: seatGate.mint)
    PickTimeout    time.Duration   // E3 seam: E1 refuses any value but 0
}

func New(o Options) (*Host, error)        // validates, then resumes every stored event (§6)
func (h *Host) Close() error              // stop advancing; wait for in-flight callbacks

// the httpapi.EventService method set (E1-6 pins the interface; E1-3/E1-4/E1-5 implement it)
func (h *Host) CreateEvent(req protocol.CreateEventRequest) (protocol.CreateEventResponse, error) // E1-3
func (h *Host) Events() []protocol.EventState                                                    // E1-3
func (h *Host) EventState(id, token string) (protocol.EventState, error)                         // E1-3
func (h *Host) JoinEvent(id, token string, req protocol.JoinEventRequest) (protocol.EventState, error) // E1-3
func (h *Host) DraftPack(id, token string) (protocol.DraftPack, error)                           // E1-3
func (h *Host) Pick(id, token string, req protocol.PickRequest) (protocol.DraftPack, error)       // E1-3
func (h *Host) SubscribeEvent(id, token, session string) error                                   // E1-3
func (h *Host) EventPool(id, token string) (protocol.EventPool, error)                           // E1-4
func (h *Host) CheckDeck(id, token string, req protocol.DeckRequest) (protocol.DeckCheck, error) // E1-4
func (h *Host) SaveDeck(id, token string, req protocol.DeckRequest) (protocol.DeckCheck, error)  // E1-4
func (h *Host) Ready(id, token string, req protocol.ReadyRequest) (protocol.EventState, error)   // E1-4
func (h *Host) EventLog(id, token string) (protocol.EventLog, error)                             // E1-4
func (h *Host) OnMatchEnd(t host.TableID, k int, m protocol.MatchInfo) error                     // E1-5

type Store struct{ /* unexported */ }
func OpenStore(dir string) (*Store, error) // dir "" = memory only (tests)
func (s *Store) DeckLoader(reg *cards.Registry, next func(string) (host.Deck, error)) func(string) (host.Deck, error) // E1-4
func DeckID(event string, seat, rev int) string // "ev/<event>/<seat>/<rev>"                                         // E1-4

type Relay struct{ Next host.OnMatchEndFunc /* + atomic target */ }                                   // E1-5
func (r *Relay) Set(fn host.OnMatchEndFunc)
func (r *Relay) OnMatchEnd(t host.TableID, k int, m protocol.MatchInfo) error

type Error struct{ /* status, code, msg */ }                                                          // E1-3
func (e *Error) Error() string
func (e *Error) Status() int
func (e *Error) Code() string
```

Error codes (status): `not_found` 404, `unauthorized` 401 (seat-scoped call
without a valid seat token, or a non-empty token that matches no seat),
`bad_request` 400 (malformed request, unknown set/policy/drafter, bad name),
`not_in_pack` 400,
`not_your_turn` 409, `stale_pick` 409, `conflict` 409 (wrong phase, already
joined under another name, ready on a non-latest rev), `capacity` 409
(`MaxLiveEvents`), `invalid_deck` 422 (`SaveDeck` with a deck
`build.Validate` rejects), `unavailable` 503 (event halted by a persistence
failure, or Host closing).

### 4.5 httpapi (E1-6)

`Options.Events EventService` (nil = every `/api/events` path answers the
JSON 404 the catch-all gives today). Routes:

| method + path | service call | ok status |
|---|---|---|
| `POST /api/events` | `CreateEvent` | 200 |
| `GET /api/events` | `Events` | 200 |
| `GET /api/events/{id}` | `EventState` | 200 |
| `POST /api/events/{id}/join` | `JoinEvent` | 200 |
| `GET /api/events/{id}/pack` | `DraftPack` | 200 |
| `POST /api/events/{id}/pick` | `Pick` | 200 |
| `GET /api/events/{id}/pool` | `EventPool` | 200 |
| `POST /api/events/{id}/deck/check` | `CheckDeck` | 200 |
| `POST /api/events/{id}/deck` | `SaveDeck` | 200 |
| `POST /api/events/{id}/ready` | `Ready` | 200 |
| `GET /api/events/{id}/log` | `EventLog` | 200 |
| `POST /api/events/{id}/subscribe` | `SubscribeEvent` | 204 |

The event seat token is read from `Authorization: Bearer` first, then
`?token=` (same precedence as `cmd/gorged/seats.go:62-70`), and passed
verbatim ("" when absent). An error implementing
`interface{ Status() int; Code() string }` sets the status and
`ErrorBody.Code`; any other error is 500 `internal`.

## 5. How eventhost drives the host

### 5.1 Tables for games

One table per game attempt (Bo1 ⇒ one per series unless a game is replayed):

```go
host.TableConfig{
    ID:     host.TableID(fmt.Sprintf("%s-r%d-s%d%d-g%d-a%d", ev, round, a, b, game, attempt)), // e.g. "e3-r1-s03-g1-a1"
    Name:   fmt.Sprintf("%s round %d: %s vs %s", ev, round, nameA, nameB),
    Seats:  2,
    Decks:  []string{DeckID(ev, b, revB), DeckID(ev, a, revA)}, // match 1 rotates by one: seat 0 plays A (server/host/match.go:165)
    Seed:   GameSeed(eventSeed, round, a, b, game, attempt),
    PlayerNames:     []string{nameA, nameB},
    SeatBotPolicies: []string{policyA or "", policyB or ""}, // "" on the human slot
    Humans:          []int{humanSlot} or nil,
    BotPolicy:       o.DefaultPolicy, // caretaker policy for the human slot
    Spectator:       view.Public,     // never omniscient: hands stay hidden on spectated games
    Mulligans:       o.Mulligans, AutoMana: o.AutoMana, BotAutoPayMana: o.BotAutoPayMana, BotCommonSense: o.BotCommonSense,
    Format:          host.FormatConstructed,
    OnDemand:        true,  // dropped at restart (server/host/restart.go:72): resume re-plays in-flight games instead
    Event:           ev,    // exempt from MaxOnDemandTables (E1-2)
}
```

`revA`/`revB` are the readied revisions (`ReadyRev`). `GameSeed` is a pure
function: `rand.New(rand.NewPCG(seed, 0x6A111E<<32 | round<<24 | a<<20 | b<<16 | game<<8 | attempt)).Uint64()`
(salt base shared with `lab/cmd/draftsim/event.go:21`).

Start order, all under the event's mutex: (1) `Apply(GameStarted{..., Attempt,
Table, GameSeed})` and persist; (2) if a human sits in the series,
`MintSeat(table, humanSlot)` and remember the token; (3) `Registry.AddTable`;
(4) `Registry.Start`. Any failure in (2)–(4) is logged as
`GameAborted{Reason: <err>}` and the event records a `blocked` reason (shown
as `EventState.Error`) and stops starting games until resume. Because the
attempt is logged **before** the table exists, a crash between (1) and (3)
leaves an in-flight attempt that resume aborts, and the next attempt gets a new
table id, so a table directory is never reused (the reason
`host.NextGameID` reserves on-disk ids, `server/host/seed.go:41-70`).

### 5.2 OnMatchEnd chaining

Registry options are set once (`server/host/registry.go:271`) but the eventhost needs
the Registry to exist. gorged therefore builds a `Relay` before `host.New`:

```go
relay := &eventhost.Relay{Next: hopts.OnMatchEnd} // today nil; keeps a future gorged hook (HD-4) chained
hopts.OnMatchEnd = relay.OnMatchEnd
r, _ := host.New(hopts)
evh, _ := eventhost.New(eventhost.Options{Registry: r, ...})
relay.Set(evh.OnMatchEnd)
```

`Relay.OnMatchEnd` calls `Next` (if non-nil), then the target loaded from an
`atomic.Pointer` (nil before `Set`: nothing to do; no event table can exist
before `eventhost.New`, and restart drops OnDemand tables before
`callOnMatchEnd` can fire for them, `server/host/restart.go:72` vs `:119`).

`Host.OnMatchEnd(t, k, m)` honours the "do not re-enter or block" contract: it
ignores `k != 1` and `m.State == protocol.MatchAborted` (only `Close` aborts,
and resume handles it), looks the table up in `h.tables` under `h.mu`
(a leaf lock), registers the work with `h.wg`, and hands it to
`go run.gameOver(ref, m)`; it never takes the event mutex itself and never
calls the Registry.

### 5.3 Result order is a function of outcomes, not timing

Two series of a round run concurrently, so `OnMatchEnd` arrival order is wall
time. To keep the log a pure function of (seed, human inputs, game outcomes),
`gameOver` stores the outcome in `run.outcomes[pair]` and `flush()` applies
outcomes **in pairing order**: series `i` is applied only after every series
`j < i` of the round has ended. A crash outcome on series `i` logs
`GameAborted` and starts the next attempt (or, past `MaxAttempts`, logs
`GameEnded{Winner: nil, Head: m.Head or "crashed"}` + `SeriesEnded{0,0}`), then
stops the flush at `i`. A finished bot game buffered behind a human game is
shown in `EventState` as `"ended"` but is not durable until flushed; if the
server restarts in that window, resume aborts and replays it (logged). This
costs nothing in the common case (bots finish in well under a second,
`TestDraftsimEventRunsAndVerifies` runs six games plus verify in 0.62 s on this
box, measured 2026-10-13).

### 5.4 Human seat tokens

Two kinds, never confused:

1. **Event seat token** (the invite link): 16 crypto/rand bytes hex, minted at
   `CreateEvent`, returned once in `CreateEventResponse.Invites`, stored in
   `event.json` only as `sha256` hex, compared with `crypto/subtle`. Survives
   restart (the invite link keeps working). Never written to `log.jsonl`.
2. **Game seat token**: minted per human game table through
   `Options.MintSeat` (gorged: `seatGate.mint`, `cmd/gorged/seats.go:83`),
   kept only in the event's memory (`run.tokens[table]`), surfaced only in
   `EventState.Series[i].Join = "/t/<table>?seat=<slot>&token=<tok>"` of the
   state built **for the token-holding seat** (REST and SSE). After a restart
   the in-flight game is replayed on a new table and re-minted; finished-table
   tokens are not needed (spectate links carry none).

### 5.5 Bots act synchronously and deterministically

`run.advance()` (called with the event mutex held after every state change and
at resume) loops until nothing changes:

- **LOBBY → DRAFT**: when every human seat has joined, apply `EventCreated`
  then the 12 `PackOpened` (seat-major, pack-minor, from `booster.OpenPod`
  with one `booster.IDs`).
- **DRAFT**: repeatedly, for seats 0..3 in order, every bot seat whose
  `Next` is ready picks with its persona drafter and
  `pickRNG(seed, seat, round, pickNo) = rand.New(rand.NewPCG(seed, 0x5011C1<<32 | seat<<16 | round<<8 | pickNo))`
  — a fresh stream per pick, so a resumed event needs no rng state. The pod is
  interleaving-independent (`lab/draft/limited/draft/draft.go:12-16`), so the pools are a
  pure function of the human's picks.
- **BUILD**: every bot seat with no saved deck builds (`Best23`), its deck file
  is written, then `DeckSaved{Rev: 1}` and `Ready{Rev: 1}` are applied.
- **ROUNDn** (E1-5): each pending series gets `SeriesStarted` (if not yet) and a
  game attempt (if none in flight and no buffered outcome); after round 3's last
  `SeriesEnded`, `EventEnded`.

Picks/builds never consult the clock. The PickTimeout seam: a future
`time.AfterFunc` per human pack would call the same `botPick(seat)` path with
`Auto: true`; E1 refuses a non-zero `PickTimeout` in `New`.

## 6. Persistence and resume

Layout under `Store` dir (gorged passes `filepath.Join(c.dir, "events")`):

```
<dir>/<id>/event.json        meta: id, set, seed, seats[4]{kind, name, policy, drafter, token_sha256}; atomic tmp+rename
<dir>/<id>/log.jsonl         one event.Entry per line (json.Marshal + '\n'), O_APPEND, fsync per append
<dir>/<id>/decks/<seat>-<rev>.json   deck.File (build.ToDeckFile), written BEFORE its DeckSaved is applied
```

Event ids are `e<N>`, `N = 1 + max(existing)`, allocated under `h.mu`.

**Write path** (`run.apply(en)`): `ev.Apply(en)` first (validation; an illegal
entry changes nothing), then `store.appendLog`. If the append fails the event
is marked halted (`run.halted = err`), every later call answers 503
`unavailable`, and on the next start the event resumes from what is on disk.

**Resume** (`New`): for each id in sorted order: read `event.json`; read
`log.jsonl`, dropping and truncating a final line that has no trailing `\n`
(torn write; same idea as `host.reconcileLog`, `server/host/persist.go:412`);
`event.Replay`. A replay error halts that event (read-only, `EventState.Error`)
but never fails `New`. Then, under the event mutex:

| phase | resume action |
|---|---|
| LOBBY | nothing (waiting for joins) |
| DRAFT / BUILD | `advance()` (bots pick/build where they had not) |
| ROUNDn | for each `InFlight()` attempt: apply `GameAborted{Reason: "server restart"}`; then `advance()` starts the next attempt (new table id, new seed) for every started-unended series, and starts unstarted ones |
| ROUND3, all series ended | `advance()` applies `EventEnded` |
| COMPLETE | nothing |

## 7. Concurrency model

- `Host.mu` (leaf): guards `events map[string]*run`, `tables map[host.TableID]gameRef`,
  `closing`, id allocation. Never held while taking a `run.mu`.
- `run.mu`: one per event; serialises every HTTP call, every `gameOver`
  callback goroutine and resume for that event. Lock order:
  `run.mu → Host.mu` and `run.mu → Registry` (AddTable/Start/Publish/Session).
  The Registry never calls into eventhost while holding its own locks:
  `OnMatchEnd` runs after `m.mu` is released (§3.2) and eventhost only
  enqueues there.
- SSE fan-out happens under `run.mu` through `Registry.Publish`, which never
  blocks (`server/host/session.go:122-148`), so per-event frame order is the apply
  order.
- `Close`: set `closing` under `h.mu` (callbacks then drop work; advance stops
  starting games), then `wg.Wait()` for in-flight `gameOver` goroutines.
  gorged closes the Registry first (aborting matches; eventhost ignores
  aborted ends), then eventhost.

## 8. Risks

1. **Spectate links die at restart.** Event tables are OnDemand so restart
   drops their records (`server/host/restart.go:72`); the match files stay on disk but
   `/t/<table>` 404s afterwards. E1 accepts this (see open question 1).
2. **A human who never finishes a game stalls the event.** With
   `ThinkTimeout` 0 (gorged's default unless `-manabrew`,
   `cmd/gorged/manabrew.go:66-71`) the game waits; the human can concede
   (concede is a priority option, AGENTS.md "Concede (M2d-3)"). Timers are E3.
3. **Bot mulligans on 40-card decks.** Hosted bots use their own London answer
   unless `-bot-common-sense` (`bots/registry.go:73-81`: otherwise "the
   policy's 1/3 coin"); draftsim uses `MulliganLands` (`lab/cmd/draftsim/game.go:123`).
   Hypothesis: hosted-event bot play is weaker than draftsim's measurement.
   Not fixed in E1.
4. **Feedback capture of an event game.** Hypothesis: `host.SnapshotForFeedback`
   records resolved decks, so `cmd/repro` needs no `ev/` loader; unverified.
5. **Torn writes**: one fsync per log line; a crash mid-line is repaired at
   resume (§6). A crash between writing a deck file and logging `DeckSaved`
   leaves an orphan file that the next save of that rev overwrites.
6. **Memory**: finished OnDemand tables stay registered for the process
   lifetime (`server/host/table.go:150-157`); `Event` tables are exempt from the 32
   cap, bounded instead by `MaxLiveEvents` × ~6 tables (more on crashes).
7. **Determinism scope**: the log is a pure function of the seed, the human's
   actions and the games' outcomes (§5.3). Two bot-only events with one seed
   and `host.Options.BotUnboundedDecisions` give byte-identical logs (E1-5
   test); with the served default bots carry a wall-clock budget
   (`server/host/registry.go:200-209`), so served games are not guaranteed
   reproducible — same as every hosted table today.

## 9. Spec vs code: conflicts and the fix adopted

| # | spec says | code says | fix |
|---|---|---|---|
| C1 | wire types in `server/protocol/event.go` | that file already holds the GAME event wire type (`server/protocol/event.go:8-27`) | new file `server/protocol/draftevent.go` |
| C2 | "subscribe mode for events" via the existing stream | `Registry.Subscribe` accepts only overview/focus on table ids (`server/host/session.go:230-233`) | keep `/api/stream`; subscribe with `POST /api/events/{id}/subscribe {session}`; frames pushed by `Registry.Publish` (E1-2) |
| C3 | Q9 "both facts are logged" | §4.7 has no entry for a started or aborted game | append `GameStarted`/`GameAborted` kinds (E1-1) |
| C4 | `EventCreated{Seats{Kind,Name,Persona}}` first; humans type a name when they join | `applyEventCreated` requires every name non-empty (`lab/draft/event/event.go:291-293`) | LOBBY lives in `event.json` until every human joined; `EventCreated` is logged then |
| C5 | per-seat bot policy on TableConfig | one policy for every bot (`server/host/match.go:661`) | `SeatBotPolicies` (E1-2) |
| C6 | spectate bot games via table pages | OnDemand tables count toward `MaxOnDemandTables` (32) forever (`server/host/registry.go:321-331`) | `TableConfig.Event` exemption (E1-2) |
| C7 | stale pick → 409 | `applyPicked`'s pick-number error is not `draft.ErrStale` (`lab/draft/event/event.go:382-384`) | eventhost checks `Next` before `Apply` |
| C8 | "Same seed and same inputs give a byte-identical log" (§3, pure packages) | concurrent series finish in wall-clock order | pairing-order flush (§5.3) |

## 10. Tickets

| # | title | depends on |
|---|---|---|
| E1-1 | event: game-attempt log kinds + accessors + LogFor; protocol draft-event wire types | — |
| E1-2 | host: per-seat bot policies, event-owned tables, Registry.Publish | — |
| E1-3 | eventhost: store, lobby/join, draft with synchronous bots, SSE frames | E1-1, E1-2 |
| E1-4 | eventhost: build phase, event deck files + DeckLoader, redacted log | E1-3 |
| E1-5 | eventhost: round-robin games on host tables, OnMatchEnd relay, resume | E1-4 |
| E1-6 | httpapi: /api/events routes behind an EventService interface | E1-1 |
| E1-7 | gorged: -events wiring + acceptance e2e (1 human + 3 bots over HTTP) | E1-5, E1-6 |

Seven, one over the 4–6 aim: eventhost is split in three so each slice is new
files plus tests a cheap seat can land in 1–3 rounds; E1-6 runs in parallel
with E1-3..E1-5.

## Server-authoritative rule (operator, 2026-10-13: "don't trust anything to the client")

Every request is untrusted. Review against this checklist; a reviewer treats a
miss as a MAJOR finding.
- **Identity:** the acting seat comes ONLY from the event seat token. Never
  read a seat index from a body or query for authorisation. A body's
  `pick_no` / `rev` / `round` are only compared against server state (stale → 409).
- **Intents only:** the client sends card instance IDs and basic-land counts.
  Card names, rarity, colour, legality, auto-lands, bot suggestions, pairings,
  standings and game results are computed on the server; nothing the client
  sends besides those IDs/counts/display name reaches event state.
- **Bounds:** `http.MaxBytesReader` on every body (64 KiB); reject unknown JSON
  fields; display name 1–32 printable chars; IDs must exist in the server's
  own lists (pack at the seat's queue head; that seat's pool); basics bounded
  (see E1-0); policy names must be registered, offered and not Env.
- **Hidden information:** a response or SSE frame for seat A never contains
  another seat's pack, pool, picks or deck before COMPLETE; spectators get the
  public view only (never omniscient). Add a test that serialises every
  seat-scoped response for seat A and asserts none of seat B's private CardRef
  IDs appear.
- **Results:** game outcomes enter the event only from host `OnMatchEnd`,
  never from an HTTP call.
