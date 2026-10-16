# E3 · Draft events: Bo3 series, sideboarding, multi-human pods, pick timers

Base: `main@dea4a74b413e63869566a38ddd0d19c7cc5e3b06` (dea4a74b4). Every `path:line`
below was read at that SHA (`git show dea4a74b4:<path>`, `git grep -n`). Anything
not read from code is marked **INFERRED** or **Hypothesis**.

Design of record: `docs/design/2026-10-13-draft-events-design.md` (§1
decisions Q6/Q8/Q9/Q10, §5 E3) plus the operator's E3 scope in the planning brief
(Bo3 hook, loser chooses via the existing `starting_player` kind, sideboarding,
up to 4 humans, XMage-style pick timers, Bo3 standings, UI). §11 lists every place
the spec, the brief and the code disagree and the fix adopted.

**Queued prerequisite.** E1-8 (`cli-20261013T233146Z-953c2986`, dispatched, not on
main) removes the client seed, adds `LobbyTimeout`/`IdleTimeout` auto-act with new
log kinds and an injected clock/timer hook, adds `DraftCard` facts, `/api/events/sets`
and `/personas`, initial `pod_progress`, empty standings before results and
snake_case JSON tags on `build.Deck`. Its API names are **not on main**; this plan
names what E3 needs from it by behaviour and every E3 brief that leans on it says
"read E1-8's merged code; use its names".

---

## 1. What E3 delivers

- **Bo3 series.** `best_of` 1 (default, E1 behaviour byte-for-byte) or 3 per event.
  A Bo3 series is played on **one host table** (the players keep the same
  `/t/{table}` URL across games 1–3). Game 1: the existing CR 103.1 toss, the toss
  winner chooses. Games 2–3: the **loser of the previous game** is posed the
  existing `starting_player` decision (MTR 2.2); after a drawn game the previous
  chooser chooses again. No new `decision.Kind`, no new `events.Kind`.
- **Sideboarding between games.** The whole pool is the sideboard, basics unlimited
  within E1-0's bounds (`build.Validate`). Each seat submits a new deck revision or
  keeps; bots keep. A human who does nothing is auto-kept by E1-8's `IdleTimeout`.
- **Multi-human pods.** Up to 4 humans (default cap 4); per-seat invite links (already
  minted per human seat at create); the lobby waits for every human (E1-8's
  `LobbyTimeout` bounds it). A human-vs-human series seats two humans at one table,
  each with its own game token.
- **Pick timers** for pods with two or more humans: per-pick limit by cards left in
  the pack (75,70,65,60,55,50,45,40,30,25,20,15,12,10,7 s for 15..1 cards; "relaxed"
  ×1.5), auto-pick by the seat's drafter, logged as `Picked{Auto: true}`. Off for
  single-human pods. Timers live in eventhost; replay never reads the clock.
- **Standings.** Match points from series results; GW% from games (MTR App. C) — the
  E0 code already counts games (`lab/draft/event/standings.go:73-85`); E3 pins it for Bo3.
- **UI** (two tickets, not for local seats): create (best-of, pick timer, invite
  management), hub Bo3 scores, visible pick timer, and the 3e between-games overlay.

## 2. Packages and responsibilities

| package | E3 change | ticket |
|---|---|---|
| `rules` | `Config.StartChooser *int` (a configured CR 103.1 chooser), `TossWinner(seed, seats)`; one call in `genesis.go`, prompt text in `starting_player_choice.go` | E3-1 |
| `view` | render the configured-chooser genesis Note through `player()` like the toss Note | E3-1 |
| `host` | `TableConfig.Series`, `Options.Series SeriesFunc`, `SeriesGame`, `MatchTossWinner`; run loop consults the hook before every match of a series table; per-seat planned decks; chooser persisted in the sidecar, the disk replay and the feedback capture | E3-2 |
| `internal/testutil/feedback` | read `start_chooser` back into `rules.Config` | E3-2 |
| `event` (pure) | Bo3 series rules (first to 2, cap 3 games), `Sideboarded` kind, `Entry.Chooser`, `DeckSaved` legal between games, `SeriesState`/`SeriesOf` accessors, `SeriesOutcome.Games` | E3-3 |
| `protocol` | Bo3/timer fields on existing wire types, `SideboardRequest`; goldens; gentypes root; `web/src/protocol.ts` | E3-3 |
| `eventhost` | Bo3 runtime (series tables + hook, chooser, bot keep, crash/restart, serial bot-only series) | E3-4 |
| `eventhost`, `server/host/httpapi` | sideboard API, ≤4 humans, per-slot game tokens, idle auto-keep | E3-5 |
| `eventhost` | pick timers | E3-6 |
| `cmd/gorged` | relay `Options.Series` (E3-4); acceptance e2e lives in `server/host/httpapi` (E3-7) | E3-4, E3-7 |
| `web/` | create/hub/timer (E3-8), between-games overlay (E3-9) | E3-8, E3-9 |

No new package, so no `docs/site/architecture/packages.tiers.json` entry and no
`docs/agents/repo-map*.md` row. No `internal/archtest` allow-list change: the only
`time` user is `eventhost`, already allowed (`internal/archtest/arch_test.go:145`).

## 3. Existing mechanisms relied on (verified at dea4a74b4)

### 3.1 The engine's starting-player choice

- `rules.NewStartingPlayerChoice` (`engine/rules/genesis.go:223`) deals, then leaves the
  CR 103.1 choice pending; `AskStartingPlayer` (`engine/rules/starting_player_choice.go:56`)
  poses `decision.KStartingPlayer` to `e.tossChoice.winner` with the hard-coded prompt
  "You won the toss. Choose who takes the first turn." (`:69-70`). Options are the
  living seats, each carrying `Option.Player` (`:64-68`).
- The toss is the game's **first rng draw**: `toss = e.rng.IntN(len(cfg.Names))`
  (`engine/rules/genesis.go:356-359`), announced by a Note "<deck name> won the toss"
  (`:371-374`), resolved over the survivors (`resolveToss`, `:514`), and the choice is
  pending only when `tossAsk` and the toss winner survived the deal (`:530-534`).
  `newRNG(seed)` is `rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)` (`engine/rules/rng.go:73-77`).
  So the toss winner is a pure function of (seed, seats) — **INFERRED from the code
  comment at `:347-351`; E3-1 pins it with a test**.
- `tossChoice{active, winner}` (`engine/rules/starting_player_choice.go:40-43`) is copied by
  value in the generated clone (`engine/rules/clone_gen.go:197`; generator
  `//go:generate` at `engine/rules/clone.go:28`, test `TestCloneGenIsUpToDate`
  `engine/rules/clone_gen_test.go:509`).
- The host builds every live match with `NewStartingPlayerChoice` and calls
  `AskStartingPlayer` before `Advance` (`server/host/match.go:219,235`); replay re-poses the
  ask when the log recorded one (`engine/replay/replay.go:223-240`; a log that never asked replays through plain `rules.New`) and is built from the
  `rules.Config` it is handed.
- Bots answer `KStartingPlayer` by naming themselves (`bots/botpolicy/policy.go:665-679`).
- `engine/view/describe.go:543-545` renders the genesis toss Note (Seq 1, exact
  `tossNoteText`, `:925-933`) through `player()` so a human reads their own name.

### 3.2 host tables and matches

- `TableConfig` (`server/host/table.go:115-230`); `Event` is its last field (`:229`); an
  event table must be OnDemand and not Perpetual (`server/host/seatpolicy.go:46-47`) and is
  exempt from `MaxOnDemandTables` (`server/host/registry.go:321-331`).
- A table with `Humans` or without `Perpetual` is single-shot (`server/host/table.go:441-443`);
  the run loop stops after one match then (`server/host/registry.go:621-624`); otherwise it
  sleeps `Options.Cooldown` and plays match k+1 (`:625-632`). The loop holds **no
  lock** between `retire` and the next `newMatch` (`server/host/registry.go:592-632`).
  `run`'s `ctx` is cancelled when `Registry.Close` closes `t.stop`
  (`server/host/registry.go:580-591`, `:777-791`).
- Deck rotation: seat `i` of match `k` plays `Decks[(i+k)%len]` (`server/host/match.go:165`);
  `PlayerNames`, `SeatBotPolicies` and `Humans` never rotate. `newMatch(t, k)`
  (`server/host/match.go:136`) is also called directly by tests
  (`server/host/host_test.go:491`, `server/host/botenv_test.go:564`, …), so its signature stays.
- A new match gets fresh `HumanSeat`s for the `Humans` slots (`seats[h] = NewHumanSeat()`,
  `server/host/match.go:679`); a seat claim is (table, seat), not (table,
  match) (`cmd/gorged/seats.go:83-95`), so a human's game token works for every match
  of the table.
- The match's `rules.Config` is persisted through the sidecar (`server/host/persist.go:21-104`,
  built by `(*match).sidecar` `server/host/match.go:300-313`); the disk replay rebuilds its
  Config from the sidecar alone (`server/host/viewat.go:271-284`); the feedback capture
  mirrors the sidecar (`server/host/feedback.go:37-…`, `feedbackMatch` `:348-361`) and a test
  pins that the two marshal the same keys (`TestFeedbackMatchMarshalsTheSidecarsKeys`,
  `server/host/feedback_test.go:15`); `internal/testutil/feedback` rebuilds the Config
  (`matchJSON` `:51`, `config` `:243-279`).
- `MatchSeed(tableSeed, k)` (`server/host/seed.go:15`); `protocol.MatchInfo.Seed` is that
  match seed (`server/host/match.go:290-294`).
- `OnMatchEnd` fires after the match lock is released, on the table goroutine, before
  `play` returns (`server/host/match.go:1040-1041`, abort `:1046-1054`, crash `:1064-1075`);
  a crash halts the table (`server/host/registry.go:612-614`).
- Restart drops every OnDemand table before validation (`server/host/restart.go:72-75`), so
  event tables never come back.

### 3.3 event (E0/E1)

- Bo1 only: `bestOfOnly = 1` (`lab/draft/event/event.go:47-48`), `SeriesStarted` refuses any
  other length (`:494-496`), `SeriesEnded` requires `len(games) == bestOf` (`:615-617`).
- `DeckSaved` and `Ready` are BUILD-only (`:407`, `:430`); `DeckSaved` resets the
  seat's ready (`:422`).
- `GameStarted` needs `Game == len(games)+1` and `Attempt == attempts+1` (`:562-567`);
  `GameEnded` resets `attempts` (`:535`). `seriesFor` lazily creates state (`:476-488`);
  read accessors use `e.series[p]` directly (`lab/draft/event/attempt.go:32-44`).
- Kinds and `Entry` (`lab/draft/event/log.go:28-40`, `:46-101`); `ParseLog` whitelists kinds
  (`:151-157`); `cloneEntry` deep-copies pointers/slices (`:105-120`).
- Standings: game points 3/1/0 per game, GW% = game points/(3×games) floored at 0.33
  (`lab/draft/event/standings.go:73-85`, `pct` `:138-147`) — already Bo3-correct.
- `SeriesOutcome` has no games list (`lab/draft/event/attempt.go:22-27`); `finishedSeries` does
  (`lab/draft/event/event.go:98-104`).

### 3.4 eventhost (E1)

- `Options` (`lab/draft/eventhost/host.go:27-42`); `PickTimeout` refused (`:90-92`); human cap
  `maxHumans()` default 1 (`:395-400`); seat tokens sha256 in `meta`
  (`lab/draft/eventhost/store.go:24-42`); `meta` has no best-of/timer fields.
- `startGame` (`lab/draft/eventhost/rounds.go:41-115`): logs `GameStarted` before `AddTable`;
  **game number fixed at 1** (`:45-49`, `:82`); `r.tokens[id] = tok` keeps **one token
  per table**, the last minted slot's (`:71-80`) — wrong for a two-human table;
  `Decks: [B, A]` + match-1 rotation seats A at slot 0 (`:88-93`).
- `advanceRounds` applies `SeriesStarted{BestOf: 1}` (`:150-158`).
- `OnMatchEnd` ignores `k != 1` (`lab/draft/eventhost/results.go:238`) and hands work to a
  goroutine under `h.wg` (`:245-252`); `gameOver` drops the table from `h.tables`
  (`:272-274`); `flush` applies only the **first** pending series' outcome (`:282-301`),
  i.e. results are logged in pairing order.
- `stateFor`/`seriesFor` hard-code `BestOf: 1` (`lab/draft/eventhost/state.go:51`) and build the
  `Join` URL from the single per-table token (`:91-98`).
- `saveDeck` refuses outside BUILD (`lab/draft/eventhost/build.go:53-56`).
- `prepare` gives every seat, humans included, a drafter persona ("rating-colour" when
  unset) (`lab/draft/eventhost/host.go:168-178`); `advanceDraft` drives only bot seats
  (`lab/draft/eventhost/draft.go:326-352`); per-pick rng `pickRNG(seed, seat, round, pickNo)`
  (`:222-225`).
- `Relay` chains only `OnMatchEnd` (`lab/draft/eventhost/relay.go`); gorged installs it
  (`cmd/gorged/events.go:47-53`) and sets the target after `eventhost.New` (`:96`).
- Measured (2026-10-13, dea4a74b4, capped): `TestBotOnlyEventCompletes` 0.90 s,
  `TestBotOnlyEventIsDeterministic` 0.21 s (`go test -run … -v ./lab/draft/eventhost`).

### 3.5 protocol, httpapi, web

- Wire types `server/protocol/draftevent.go` (EventSeries `:45-56`, EventState `:75-85`,
  DraftPack `:91-99`, CreateEventRequest `:127-131`, EventPool `:186-194`); gentypes
  roots `cmd/gentypes/main.go:33-40`; goldens `server/protocol/testdata/event_state.json`,
  `draft_pack.json`, `pod_progress.json`, `series_update.json`.
- `EventService` (`server/host/httpapi/events.go:15-28`), routes + 405 twins (`:41-75`).
  `cmd/gorged/events.go:17` asserts `*eventhost.Host` satisfies it, so an interface
  method and its eventhost implementation land in **one** ticket.
- Table seat routes for a game: `GET /api/tables/{t}/matches/{k}/pending`,
  `POST …/intent` (`server/host/httpapi/handler.go:99-102`); concede is a `"concede"` option
  on every priority decision (`bots/botpolicy/policy.go:551`).
- Web: `NewEvent.svelte` says "Inviting more humans comes later" (`:96`) and follows
  only `res.invites?.[0]` (`:46`); `HubView.svelte` `score()` (`:48-50`), join link
  (`:77-78`); the event seat token is remembered per event in localStorage
  (`web/src/lib/eventseat.ts`); `starting_player` renders through `ListPrompt`
  (`web/src/components/prompts/ListPrompt.svelte:18`).

## 4. API of record

### 4.1 rules (E3-1)

```go
// engine/rules/engine.go, Config, appended after LoopGuard's block (any position; keep the doc)
// StartChooser, when non-nil, names the seat posed CR 103.1's starting-player
// choice instead of the toss winner (MTR 2.2: the loser of the previous game
// of a match chooses). The toss is still drawn (the rng stream and the deal
// are unchanged); its genesis Note reads "<name> chooses who takes the first
// turn". nil — every Config before E3 — is byte-identical to today.
StartChooser *int

// engine/rules/starting_player_choice.go
// TossWinner is the seat that wins the CR 103.1 toss of a game seeded with
// seed and seats players: the first draw of the game's rng.
func TossWinner(seed uint64, seats int) int
```

`genesisDeal` change (one call at `engine/rules/genesis.go:356-374`): after the toss draw,
`toss, note := e.tossOutcome(cfg, toss)` where the new helper (in
`starting_player_choice.go`) returns `*cfg.StartChooser` and the "chooses who takes
the first turn" text when the field is set and in range, else the toss and the
"won the toss" text. `tossChoice` gains `configured bool`; `AskStartingPlayer`'s
prompt is "Choose who takes the first turn." when configured. An out-of-range
`StartChooser` is ignored (the toss stands) — the host validates before it gets here.
The override applies in **both** constructors (`rules.New` and
`NewStartingPlayerChoice`): replay rebuilds a log that never recorded the ask through
plain `rules.New` (`engine/replay/replay.go:233-240`), and it must reproduce the same Note and
starting seat.

### 4.2 host (E3-2)

```go
// server/host/series.go (new)
// SeriesGame is a series table's plan for one match.
type SeriesGame struct {
    Decks   []string // per table SEAT (not rotated by k); len == Seats
    Chooser int      // table seat posed the starting_player choice; -1 = the CR 103.1 toss
}
// SeriesFunc plans match k (k = 1 included) of a Series table. It runs on the
// table's own goroutine holding no Registry or table lock, may block (between
// games it waits for sideboarding), and must return promptly once ctx is done.
// ok=false ends the table (idle) without playing match k; an error halts it.
// It must not call Registry.Close or Registry.Wait.
type SeriesFunc func(ctx context.Context, t TableID, k int) (g SeriesGame, ok bool, err error)
// MatchTossWinner is the table seat that wins match k's toss when no chooser
// is configured: rules.TossWinner(MatchSeed(tableSeed, k), seats).
func MatchTossWinner(tableSeed uint64, k, seats int) int

// registry.go Options, after OnMatchEnd
Series SeriesFunc
// table.go TableConfig, after Event
Series bool `json:"series,omitempty"` // requires Event != "" and Options.Series

// persist.go sidecar, FeedbackMatch (same key, both)
StartChooser *int `json:"start_chooser,omitempty"`
```

Run loop (`server/host/registry.go:593-631`): at the loop head, for a `Series` table,
return idle if `t.stop` is closed; set state `cooldown` when `k > 1`; call
`Options.Series(ctx, id, k)`; error → `halt`; `!ok` → idle, return; validate
(`len(Decks) == Seats`, `-1 <= Chooser < Seats`) → `halt` on failure; then
`newMatchWith(t, k, &g)`. After `play`, a Series table skips the single-shot exit and
the cooldown sleep and loops (the next hook call decides). `singleShot()` returns
false for `Series`. `newMatch(t, k)` becomes `newMatchWith(t, k, nil)`; with a plan,
seat `i` loads `g.Decks[i]` and `cfg.StartChooser = &g.Chooser` when `g.Chooser >= 0`.
A non-Series table never reaches any new line.

### 4.3 event (E3-3)

```go
// log.go — appended after the last kind on main (E1-8 appends its own first)
KindSideboarded Kind = "Sideboarded" // a seat's deck revision for its series' next game
// Entry — appended field
Chooser *int `json:"chooser,omitempty"` // Bo3 GameStarted: event seat posed play/draw

// attempt.go
type GameAttempt struct { …existing…; Chooser int } // -1 when the GameStarted carried none
type SeriesOutcome struct { …existing…; Games []int } // winner seat per game, -1 a draw
type SeriesView struct {
    Round, A, B, BestOf int
    Started, Ended      bool
    Games       []int   // winner seat per played game, -1 a draw
    Choosers    []int   // chooser per played game, -1 unknown
    Wins        [2]int  // games won by A, B
    NextGame    int     // the game to play next (len(Games)+1), 0 once decided or full
    Between     bool    // Bo3: a game ended, the next has not started, the series is open
    Locked      [2]int  // revisions A, B locked (Sideboarded) for NextGame; 0 = not yet
    PlayRevs    [2]int  // revisions A, B play in the in-flight (or last) game
    NextChooser int     // MTR 2.2 chooser for NextGame >= 2; -1 for game 1 or unknown
    InFlight    *GameAttempt
}
func (e *Event) SeriesState(a, b int) (SeriesView, bool) // current round only; read-only
func (e *Event) SeriesOf(seat int) (a, b int, ok bool)   // seat's current-round pairing
```

Rules (exact text in E3-3): `BestOf ∈ {1,3}`; a series is **decided** when a side has
`BestOf/2+1` wins; `GameStarted`/`GameEnded`/`Sideboarded` are refused once decided;
`SeriesEnded` is legal when decided **or** `len(games) == BestOf`. Bo3 `GameStarted`
must carry `Chooser ∈ {A,B}`; for game ≥ 2 both sides must be locked and `Chooser`
must equal the loser of the previous game (after a draw, that game's chooser; any
seat when unknown). Bo1 `GameStarted` must not carry `Chooser`. `Sideboarded` is legal
only `Between`, for `Game == NextGame`, once per side, with a revision the seat saved.
`DeckSaved` is legal in BUILD (unchanged) **or** in a round phase for a seat whose
series is `Between` and not yet locked; in a round it never touches `ready`.
`GameStarted` sets `PlayRevs` (ready revisions for game 1, the locks after);
`GameEnded` records the chooser and clears the locks; locks survive a `GameAborted`
so a retry plays the same decks.

### 4.4 protocol (E3-3)

| type | added | JSON |
|---|---|---|
| `CreateEventRequest` | `BestOf int`, `PickTimer string` | `best_of,omitempty` (0=1; 1 or 3), `pick_timer,omitempty` ("", off, standard, relaxed) |
| `EventState` | `BestOf int`, `PickTimer string` | `best_of`, `pick_timer,omitempty` |
| `EventSeries` | `State` gains `"between"`; `Game int`, `Games []int`, `Chooser *int`, `Waiting []int` | `game,omitempty`, `games,omitempty`, `chooser,omitempty`, `waiting,omitempty` |
| `DraftPack` | `TimeLimitMS int64`, `RemainingMS int64` | `time_limit_ms,omitempty`, `remaining_ms,omitempty` |
| `EventPool` | `Playing *DeckRequest`, `PlayingRev int` | `playing,omitempty`, `playing_rev,omitempty` |
| new `SideboardRequest` | `Game int`, `Keep bool`, `Deck *DeckRequest` | `game`, `keep,omitempty`, `deck,omitempty` |

No new frame kind: `series_update` and `event_state` carry the between-games state;
`draft_pack` carries the timer.

### 4.5 eventhost (E3-4/5/6)

```go
// Options: PickTimeout is no longer refused: 0 = the per-pick table, > 0 = one flat
// limit for every pick (ops/tests). MaxHumans 0 = 4.
func (h *Host) Series(ctx context.Context, t host.TableID, k int) (host.SeriesGame, bool, error) // E3-4
func (r *Relay) SetSeries(fn host.SeriesFunc)                                                     // E3-4
func (r *Relay) Series(ctx context.Context, t host.TableID, k int) (host.SeriesGame, bool, error) // E3-4
func (h *Host) Sideboard(id, token string, req protocol.SideboardRequest) (protocol.EventState, error) // E3-5
```

`meta` gains `BestOf int \`json:"best_of,omitempty"\`` and `PickTimer string
\`json:"pick_timer,omitempty"\`` (an old `event.json` reads back Bo1, timer off).

### 4.6 httpapi (E3-5)

`POST /api/events/{id}/sideboard` → `EventService.Sideboard` → 200 `EventState`;
405 twin. Errors: `unauthorized` 401 (no/unknown token), `conflict` 409 (not between
games, `game` is not the next game, already submitted), `bad_request` 400 (both or
neither of `keep`/`deck`), `invalid_deck` 422.

## 5. How a series is played

### 5.1 One table per series (and per retry)

A Bo3 series gets a **series table** (`TableConfig.Series: true`, `OnDemand`, `Event`)
when its game `g` (normally 1) must start and no live series table exists for the
pair. The id keeps E1's shape, naming the first game it hosts and that game's attempt:
`gameTableID(ev, round, a, b, g, attempt)` (`lab/draft/eventhost/rounds.go:29-31`), e.g.
`e3-r1-s03-g1-a1`. Its seed is `gameSeed(seed, round, a, b, g, attempt)`
(`:20-24`); match k's engine seed is `host.MatchSeed(tableSeed, k)`. Game `g + k - 1`
is match `k` of the table. A crash halts the table (§3.2), so a retry always opens a
new table (`…-g<g>-a<attempt+1>`), and every later game of the series continues on
that new table. A restart drops the table (§3.2); the series continues on a new table
too. Table ids are never reused.

`gameRef` gains `series bool` and `firstGame int`; a series ref stays in `h.tables`
until the series ends or the table crashes. `r.seriesTable[[2]int{a,b}]` names the
live series table of a pair.

### 5.2 The hook (eventhost `Host.Series`)

1. Under `h.mu`: find the ref for `t` (unknown or Host closed → `ok=false`).
2. Under `r.mu`: `game := ref.firstGame + k - 1`; if the series' in-flight attempt is on
   `t` with that game → **ready**: `SeriesGame{Decks: [DeckID(ev,a,PlayRevs[0]),
   DeckID(ev,b,PlayRevs[1])], Chooser: -1 for game 1, else the slot of the logged
   Chooser}`. If the series ended, or `r.seriesTable[pair] != t` → **stop**
   (`ok=false`). Otherwise register `r.waiters[t] = waiter{k, ch}` (`ch` buffered 1)
   and **wait**.
3. Release `r.mu`; `select` on `ch` or `ctx.Done()` (on ctx: delete the waiter under
   `r.mu`, return `ok=false`).
4. `r.wakeWaiters()` runs at the end of every `advance()`: for every waiter (sorted by
   table id) whose step 2 now answers ready or stop, send and delete.

The hook **never applies a log entry**. `GameStarted` for a continuing game is
applied by `advanceRounds` (§5.4), so the log is a function of the event's own state
and never of when the table goroutine reached the hook. A hook call that arrives
before `gameOver` has logged game k's result simply waits.

### 5.3 Play/draw to the loser

- **Game 1** (k = 1 on a table whose `firstGame` is 1): the plan's `Chooser` is -1, so
  the engine draws and announces the toss and poses the choice to the toss winner,
  exactly as every table does today. eventhost logs the chooser on `GameStarted`
  anyway: `slot := host.MatchTossWinner(tableSeed, 1, 2)`, seat `a` for slot 0 else
  `b` — it is needed only if game 1 is drawn.
- **Game g ≥ 2**: `Chooser` = the loser of game g-1, or game g-1's chooser after a
  draw (`SeriesView.NextChooser`, validated by `event.Apply`). The plan passes its
  table slot (slot 0 = A, slot 1 = B, never rotated); the host sets
  `rules.Config.StartChooser`; the engine poses the ordinary `starting_player`
  decision to that seat with the prompt "Choose who takes the first turn." A bot names
  itself (plays first); a human answers through the table's existing decision UI.
- A human who never answers is the E1-8 in-game idle path (Q8: never auto-replaced
  mid-game; caretaker only with `ThinkTimeout`).

### 5.4 The series state machine (event + eventhost)

```
PENDING --SeriesStarted--> GAME(g=1)
GAME(g) --GameEnded--> decided or g == BestOf ? --SeriesEnded--> ENDED
                                            : BETWEEN(g+1)
GAME(g) --GameAborted--> RETRY(g)  --GameStarted(attempt+1, new table)--> GAME(g)
BETWEEN(g) --[DeckSaved] Sideboarded(A), [DeckSaved] Sideboarded(B), any order--> READY(g)
READY(g) --GameStarted(g, live series table or a new one)--> GAME(g)
```

`advanceRounds` for a Bo3 event, per pending series in pairing order:
1. bot-only event: process a series only when every earlier series of the round has
   ended (serial; §7);
2. `SeriesStarted{BestOf: 3}` if not started;
3. in flight → skip;
4. decided or full → `SeriesEnded{Score: Wins}`, clear `seriesTable`, wake waiters;
5. `Between`: every **bot** side not locked → `Sideboarded{Game, Seat, Rev: PlayRevs[side]}`
   (keep); when both are locked → if `seriesTable[pair]` is set, `GameStarted{Game,
   Attempt: 1, Table: that table, GameSeed: its table seed, Chooser: NextChooser}`
   (the waiting hook then answers ready); else `startGame(sp, NextGame)` on a new table;
6. otherwise (game 1, or any retry) → `startGame(sp, NextGame)`.

`gameOver` for a series ref (no buffering, no pairing-order flush): stale callbacks
(no in-flight attempt on this table with game `firstGame+k-1`) are dropped;
`MatchFinished` → `GameEnded{Game, Attempt, Winner, Head}`; `MatchCrashed` →
`GameAborted{Reason: "crashed"}`, drop the table from `h.tables` and `seriesTable`, and
past `MaxAttempts` also `GameEnded{Winner: nil, Head: m.Head or "crashed"}` (the game
counts as drawn and the series continues); then `advance(); broadcast()`.
Bo1 events keep E1's tables, `OnMatchEnd` filter and flush unchanged.

### 5.5 Log entries for a 2–1 series (round 1, seats 0 v 3, seat 0 human)

```
{"kind":"SeriesStarted","round":1,"b":3,"best_of":3}
{"kind":"GameStarted","round":1,"b":3,"game":1,"attempt":1,"table":"e1-r1-s03-g1-a1","game_seed":…,"chooser":3}
{"kind":"GameEnded","round":1,"b":3,"game":1,"attempt":1,"winner":3,"head":"…"}
{"kind":"Sideboarded","round":1,"b":3,"game":2,"seat":3,"rev":1}          bot keeps
{"kind":"DeckSaved","seat":0,"rev":2,"deck":{…}}                            human swaps a card
{"kind":"Sideboarded","round":1,"b":3,"game":2,"rev":2}                     seat 0 (omitempty)
{"kind":"GameStarted","round":1,"b":3,"game":2,"attempt":1,"table":"e1-r1-s03-g1-a1","game_seed":…,"chooser":0}
{"kind":"GameEnded",…,"game":2,"winner":0,…}
{"kind":"Sideboarded",…,"game":3,"seat":3,"rev":1}
{"kind":"Sideboarded",…,"game":3,"rev":2,"auto":true}                       E1-8 idle auto-keep
{"kind":"GameStarted",…,"game":3,…,"chooser":3}
{"kind":"GameEnded",…,"game":3,"winner":3,…}
{"kind":"SeriesEnded","round":1,"b":3,"score":[1,2]}
```

(`a:0` and `seat:0` are omitted by `omitempty`, as in E1 logs.)

### 5.6 Sideboarding (E3-5)

`POST /api/events/{id}/sideboard {game, keep | deck}`: the seat comes only from the
token; `SeriesOf(seat)` → `SeriesState`; refuse unless `Between`, `game == NextGame`
and the side is unlocked. `keep` → `Sideboarded{Rev: PlayRevs[side]}`; `deck` →
`saveDeck` (deck file first, then `DeckSaved`, E1-4 order; now legal between games)
then `Sideboarded{Rev: new}`. Then `advance(); broadcast()`. `EventPool` in a round
adds `Playing`/`PlayingRev` (the deck the seat played last game). `CheckDeck` stays
read-only and legal. Every series starts from the **readied** BUILD deck (no
continuous construction, open question 2).

### 5.7 Multi-human (E3-5)

`MaxHumans` 0 means 4. `run.tokens` becomes `map[host.TableID][2]string` (one per slot);
`startGame` mints every human slot. `Join` = `/t/<table>?seat=<slot>&token=<tok>&event=<id>`
(the `event` parameter lets the table page find the event seat token remembered by the
event page). Lobby: unchanged rule (EventCreated once every human has joined); E1-8's
`LobbyTimeout` decides what an incomplete lobby does (open question 3).

### 5.8 Pick timers (E3-6)

- Mode from create: `pick_timer` "" → "standard" when the event has ≥ 2 humans, else
  "off"; any value with ≤ 1 human is stored "off"; unknown value → 400. Stored in `meta`.
- `pickLimit(mode, cardsLeft)`: `[75,70,65,60,55,50,45,40,30,25,20,15,12,10,7][15-clamp(cardsLeft,1,15)]`
  seconds, ×3/2 for "relaxed"; `Options.PickTimeout > 0` replaces the table.
- Arming: at the end of every `advance()` (under `r.mu`), for every human seat whose
  `Next` (round, pickNo) differs from its armed key: stop the old timer, start a new
  one through E1-8's injected timer seam, remember (key, start). Expiry callback:
  `h.mu` (closed? `wg.Add`) → `r.mu`: if `Next(seat)` is still that key and the phase
  is DRAFT, pick with `r.drafters[seat].Pick(v, pickRNG(seed, seat, round, pickNo))`,
  apply `Picked{Auto: true}`, `advance(); broadcast()`.
- `DraftPack.TimeLimitMS`/`RemainingMS` from the armed key and the seam's clock.
- A restart re-arms every timer with its full limit (no deadline is persisted). Replay
  reads only the logged `Picked{Auto: true}` entries.

## 6. Persistence and resume

| file | E3 addition |
|---|---|
| `<events>/<id>/event.json` | `best_of`, `pick_timer` (absent = Bo1, off) |
| `<events>/<id>/log.jsonl` | `Sideboarded` lines, `chooser` on Bo3 `GameStarted`, round-phase `DeckSaved` |
| `<events>/<id>/decks/<seat>-<rev>.json` | sideboard revisions, same scheme |
| host `<dir>/<table>/<k>.json` sidecar | `start_chooser` on matches 2–3 of a series table |

Resume (`eventhost.New`, `abortInFlight` then `advance`, `lab/draft/eventhost/host.go:146-151`):

| state at restart | resume |
|---|---|
| a Bo3 game in flight | `GameAborted{"server restart"}`; `seriesTable` is empty in the new process, so the retry opens a new series table (`…-g<g>-a<n+1>`) whose match 1 is game g with the same locked decks; game 1's chooser is the new table's toss |
| between games, some seats locked | locks are in the log; bots re-lock if their entries were lost; humans are asked again (hub panel; E1-8 idle timer restarts) |
| between games, both locked, no `GameStarted` | `advanceRounds` starts game g on a new series table |
| decided, `SeriesEnded` not logged | `advanceRounds` applies it |
| match finished, `GameEnded` not logged | in flight at restart → aborted and replayed (as E1) |
| DRAFT with timers | timers re-armed with full limits |

## 7. Determinism

- Bo1 events: unchanged byte-for-byte (no new field is emitted; E1 tests stay green).
- Bo3, bot-only event: series of a round are played one at a time in pairing order,
  every entry is applied by `advance` from event state, bots keep, choosers are
  derived; with `host.Options.BotUnboundedDecisions` two runs of one seed give
  byte-identical logs (E3-4 test). Measured cost of a 6-game bot event: 0.21 s per
  run (§3.4), so serial Bo3 (≤ 18 games) is ~1 s — **INFERRED** from that number.
- Events with humans: each series' own subsequence of entries is a pure function of
  the seed, the humans' intents and the game outcomes; the interleaving of two live
  series is wall-clock (as human timing always is). Replay never depends on it.
- Timers append entries; replay never reads the clock.

## 8. Concurrency

- Lock order unchanged: `run.mu → Host.mu` (leaf) and `run.mu → Registry` calls.
- `Host.Series` (table goroutine): `h.mu` alone (find the ref), then `r.mu` alone; it
  waits holding nothing. Waiter channels are buffered 1; `wakeWaiters` sends without
  blocking under `r.mu`. The table goroutine holds no host lock while in the hook
  (§3.2), so `startGame` (under `r.mu`) calling `Registry.Start` cannot deadlock with
  a hook waiting on `r.mu`.
- `Host.Close`: set `closed`, stop every pick timer, answer every waiter `ok=false`
  (under each `r.mu`), then `wg.Wait()`. `Registry.Close` cancels the hook's ctx, so
  either close order ends every series table.
- Timer callbacks take `h.mu` (closed check, `wg.Add`) then `r.mu`, like `OnMatchEnd`.

## 9. Server-authoritative checklist additions (E3)

- `best_of` ∈ {omitted, 1, 3}; `pick_timer` ∈ {"", off, standard, relaxed}; seat count
  of humans ≤ `MaxHumans`. Sideboard: seat from the token only; `game` compared with
  `NextGame` (stale → 409); deck IDs from the seat's own pool; basics bounded by
  `build.Validate`; play/draw is answered in the game (host decision validation), never
  through an event call. Auto-picks and auto-keeps are computed on the server.
- Hidden information: a seat's sideboarded deck (DeckSaved during a round) stays
  redacted for every other seat until COMPLETE (`event.LogFor`, `lab/draft/event/attempt.go:148-151`);
  `EventSeries.Waiting` and `Chooser` are public; `PlayRevs` revision numbers are public
  log facts, deck contents are not.

## 10. Risks

1. **`engine/rules/genesis.go` is a hot file** (CPU-perf engine pooling,
   `ops/scripts/hotfiles-notes.json`). E3-1 keeps its edit to one call; the logic lives in
   `starting_player_choice.go`.
2. **A series table that fails `newMatch`** (deck load or persistence) halts without an
   `OnMatchEnd`, so the event keeps an in-flight attempt until a restart aborts it.
   Mitigation: eventhost resolves both planned deck ids through the store loader before
   it applies a continuing game's `GameStarted`; residual risk accepted.
3. **The table page across matches.** A human stays on `/t/{table}` while match k+1
   starts. **INFERRED** that `Table.svelte` follows a table's new match the way
   perpetual spectating does; E3-9 verifies with a test.
4. **Chooser sees the opening hand.** The engine deals before posing the choice
   (`engine/rules/starting_player_choice.go:13-16`: the choice stays available "once the opening deal has fixed the survivors"). Same for game 1 today;
   not changed by E3. **INFERRED** whether the view exposes the hand while the choice is
   pending.
5. **TossWinner coupling.** The logged game-1 chooser is computed outside the engine;
   E3-1 pins `TossWinner` against genesis for 200 seeds, so a later change to the toss
   fails a test instead of silently mislogging.
6. **E1-8 names unknown.** E3-5 and E3-6 integrate with E1-8's idle/lobby timers and
   clock seam by behaviour; if E1-8's forfeit path cannot end a Bo3 series legally,
   the E3-5 seat stops and reports.
7. **Spectate links still die at restart** (E1 risk 1); with Bo3 the series continues on
   a new table, which the hub shows.
8. **A human between games pins one idle table goroutine** until sideboarding resolves
   (bounded by E1-8's `IdleTimeout`, default 2 h).

## 11. Spec, brief and code: conflicts and the fix adopted

| # | says | code says | fix |
|---|---|---|---|
| C1 | Bo3 series (spec Q6, §5 E3) | Bo1 only: `bestOfOnly` (`lab/draft/event/event.go:48`), `SeriesStarted` refuses others (`:494`), `SeriesEnded` needs `len(games)==bestOf` (`:615`) | BestOf ∈ {1,3}; ended when decided or full (E3-3) |
| C2 | sideboarding as event actions | `DeckSaved`/`Ready` BUILD-only (`:407`, `:430`), `DeckSaved` resets ready (`:422`) | `DeckSaved` legal between games without touching ready; new `Sideboarded` kind (E3-3) |
| C3 | loser chooses via the existing `starting_player` | the engine poses it only to the toss winner (`engine/rules/genesis.go:530-534`, `starting_player_choice.go:69`) | `rules.Config.StartChooser` (E3-1), planned per match by the host hook (E3-2) |
| C4 | game 1 = existing toss; draws → previous chooser | no API reports who chose game 1 | `rules.TossWinner` / `host.MatchTossWinner`; chooser logged on `GameStarted` (E3-1/2/4) |
| C5 | one table hosts one series, same `/t/{table}` | human tables are single-shot (`server/host/table.go:441`, `registry.go:621`); decks rotate by k (`match.go:165`) | `TableConfig.Series` + `Options.Series` hook with per-seat decks (E3-2) |
| C6 | players keep the same URL | restart drops OnDemand tables (`server/host/restart.go:72`) | after a restart (or a crash) the series continues on a new table; the hub shows the new join link; the between-games panel also lives in the hub (E3-9) |
| C7 | game k+1 results | eventhost ignores `k != 1` (`lab/draft/eventhost/results.go:238`) | series refs accept every k (E3-4) |
| C8 | determinism (spec §3) | pairing-order flush applies only the first pending series (`results.go:282-301`), which would hold a Bo3 series' game 2 behind another series' whole Bo3 | Bo3 does not buffer; bot-only Bo3 events play a round's series serially; Bo1 keeps the flush (E3-4) |
| C9 | multi-human | one token per table, last slot wins (`rounds.go:79`, `state.go:91-98`) | tokens per slot (E3-5) |
| C10 | up to 4 humans | `maxHumans()` default 1 (`host.go:395-400`) | default 4 (E3-5) |
| C11 | pick timers for multi-human pods | spec §1 lists them out of scope; `PickTimeout` refused (`host.go:90-92`) | brief and spec §5 E3 win: table-based timers (E3-6) |
| C12 | XMage 75..7 s "for picks 1..15" | FDN/BLB Play Boosters are 14 cards (`lab/draft/limited/setdata/sets/FDN.json` `pack_size`) | index by cards left: pick 1 of a 14-card pack gets 70 s |
| C13 | MTR Bo3 (first to two wins) | no round clock exists | cap at 3 games: a series can end 1–1 or 1–0 with draws; standings use game points as MTR App. C |
| C14 | mockup 3e: overlay on `/t/{table}` | the table page has no event id or event token | `Join` gains `&event=<id>`; the page reads the event token remembered by `eventseat.ts` (E3-5, E3-9) |
| C15 | `EventSeries.BestOf` | hard-coded 1 (`lab/draft/eventhost/state.go:51`) | from the series (E3-4) |
| C16 | E1-8 timers/clock seam | not on main | E3-5/E3-6 depend on E1-8 and use its names |

## 12. Tickets

| # | title | depends on | seat |
|---|---|---|---|
| E3-1 | rules + view: configured starting-player chooser, `TossWinner` | — | local OK |
| E3-2 | host: series tables (`Options.Series` hook), chooser through sidecar/replay/feedback | E3-1 | local OK |
| E3-3 | event + protocol: Bo3 series rules, `Sideboarded`, chooser; wire types | E1-8 | local OK |
| E3-4 | eventhost: Bo3 runtime for bot seats; gorged relays the hook | E3-2, E3-3 | local OK |
| E3-5 | eventhost + httpapi: sideboard API, ≤4 humans, per-slot game tokens, idle auto-keep | E3-4 | local OK |
| E3-6 | eventhost: pick timers | E3-5 | local OK |
| E3-7 | acceptance e2e: 2 humans + 2 bots Bo3 over HTTP | E3-6 | local OK |
| E3-8 | **UI**: create (best-of, timer, invites), hub Bo3 scores, pick timer | E3-6 | **UI — not for local seats** |
| E3-9 | **UI**: between-games overlay + hub panel | E3-5, E3-8 | **UI — not for local seats** |

Nine, one over the 5–8 aim: the engine half (E3-1/E3-2) is split so the hot
`engine/rules/genesis.go` edit lands alone and small, and the UI is split so each UI ticket
is one screen family. E3-1 → E3-2 and E3-3 run in parallel; eventhost is serial
(E3-4 → E3-5 → E3-6, one package, shared files).

## Operator decisions on the E3 plan's open questions (2026-10-13)
The plan of record is committed as `docs/design/2026-10-13-draft-events-e3-plan.md`
(this brief cites it as `E3-plan.md`).
1. `rules.Config.StartChooser` is accepted (honest transcript: "X chooses who takes the first turn").
2. Each series starts from the deck readied in BUILD; sideboard changes do NOT carry into later rounds.
3. **Partial lobby → bots fill.** When `LobbyTimeout` fires and at least one human has joined, every
   unjoined human seat becomes a bot seat (default persona, an Env==false policy, logged in
   `EventCreated` as a bot) and the event starts; with zero humans joined the event is cancelled.
   This is implemented in E3-5 (it changes E1-8's cancel-only behaviour), with a test.
4. A series is capped at 3 games; draws may end it 1–0 or 0–0. OK.
5. Pick timers default to "standard" with ≥2 humans and are always off with one human. OK.
6. Relabel every two-seat `starting_player` prompt "Play first / Draw first". OK.
7. Invite links are shown once at create (tokens stored hashed); a reissue endpoint is out of scope.
