# E5-b / E5-c · Draft events: 8-seat pods, sealed, Swiss, and a Go composition API for training

Base: `main@705b3811ac535d56633362e5debc17284bdc3dba` (705b3811a; E3-7 merged). Every
`path:line` below was read at that SHA (`git show 705b3811a:<path>`, `git grep -n`).
Between ae9e133a8 (the spec-e5 merge) and 705b3811a only
`host/httpapi/events_bo3_e2e_test.go` changed. Anything not read from code is marked
**INFERRED** or **Hypothesis**.

Design of record: `docs/superpowers/specs/2026-10-13-draft-events-design.md` §7 (E5
decisions E5-Q1..Q6) plus the operator's E5-b/E5-c scope in the planning brief. §12
lists every place the spec, the brief and the code disagree and the fix adopted.

**Commit this file** as `docs/superpowers/plans/2026-10-14-draft-events-e5-plan.md`
before queueing; the briefs cite it as `E5-plan.md` and restate everything they need
(seats cannot read the scratchpad).

**Queued prerequisite.** E5-a1 (`cli-20261014T032241Z-c35ed10b`, dispatched, not on
main) replaces the hand-authored booster spec with MTGJSON configs: `setdata.Collation`
(`Play = "play"`, `Arena = "play-arena"`, `DefaultCollation = Arena`),
`setdata.LoadCollation(code, c)`, `Spec.PackSize()`, weighted sheets, and
`draftsim -collation`. It keeps `setdata.Load`, `setdata.Codes`, `booster.Open` and
`booster.OpenPod` callable unchanged. Its names are **not on main**; every brief that
leans on it says "read E5-a1's merged code; use its names". E5-b1..E5-b3 do not
touch `limited/setdata` or `limited/booster` and do not wait for it.

---

## 1. What E5-b and E5-c deliver

- **Pod size as a parameter (4 | 8).** `limited/draft` runs any pod shape;
  `event` accepts 4- or 8-seat events; `draftsim -seats 4|8`.
  **eventhost stays at 4 seats** (§4.4): it is not trivially parameterised (its persisted
  `meta.Seats [4]metaSeat`, `eventhost/store.go:30`; per-seat arrays at
  `eventhost/host.go:82-108`; `draftSeats = 4` at `eventhost/cards.go:48`), so hosted
  8-seat pods are E5-e. E5-b changes **no eventhost file**: the 4-seat constants and
  APIs eventhost calls keep their meaning.
- **Sealed supply.** An event in mode `sealed` opens 6 packs per seat (the collation is
  the caller's choice), the packs become the seat's private pool, and the event goes
  straight to BUILD. Bots build with their Builder.
- **Swiss** for 4 or 8 seats beside round robin: points-bracket pairing, no rematch when
  avoidable, tie-breaks from the event seed; 3 rounds for 8 seats by default. Pairings are
  **logged** (`RoundPaired`), so a log survives a later change to the pairing algorithm
  (the reason packs are logged, spec §3).
- **`draftsim -seats 4|8 -mode draft|sealed -structure roundrobin|swiss|none`**, with
  `-verify` and `-draftlog` kept. `none` stops after the build (fast self-play data).
- **`draftlab`**, a new top-level Go package a training program imports: open packs,
  draft, build, play, gauntlet, run whole events, and write artifacts (the event log, deck
  lists, 17Lands-shaped `draft_data` and `game_data` CSVs, a manifest). Every call is a
  pure function of its inputs and seed, is safe to call concurrently, and gives the same
  result for any worker count. Built-in agents are named `name@version`, so a workflow can
  freeze a generation.
- **"Sealed deck vs N agents"**: `Lab.Gauntlet(hero, opponents, games, seed)`.
- **Bot-only Env opt-in**: decklist-reading policies (`bots.Info.Env`) stay refused unless
  `draftlab.Options.AllowDecklistBots` is set (and E5-c5 has landed the Env driver).
  `draftsim` keeps refusing them (spec §4.8 item 3).
- **The E5-d seam** (§9): the shapes external Drafter/Builder/Player adapters plug into,
  designed here and not implemented.
- `cmd/labdemo`: the example workflow (random-deck games → sealed builds → 8-seat drafts →
  gauntlet).

No UI ticket; no wire/protocol/HTTP change.

## 2. Packages and responsibilities

| package | change | ticket |
|---|---|---|
| `limited/draft` (pure) | `NewPod` over `[][][]CardRef` (2..8 seats, any rounds), pass direction by parity, `Seats()`/`Rounds()`; `New`, `Seats = 4`, `Rounds = 3` kept | E5-b1 |
| `event` (pure) | N-seat events (4 \| 8), `Mode` draft \| sealed, `Structure` roundrobin \| none, event rounds decoupled from pack rounds, `RoundRobinPairings(n, r)`, `ComputeStandingsN`; informational `Collation` | E5-b2 |
| `event` (pure) | `SwissPairings`, `KindRoundPaired`, `Structure` swiss, `AwaitingPairing`/`SuggestedPairings`/`SeriesResults` | E5-b3 |
| `cmd/draftsim` | `-seats`, `-mode`, `-structure`, `-rounds`; N-seat tallies/personas; `-draftlog` refused for sealed | E5-b4 |
| `draftlab` (new) | `Open`, `Options`, agents (`AgentRef`, versions, fingerprints, Env gate), `OpenPacks`/`OpenPod`/`SealedPool`/`CardNames`, `Draft`, `Deck`/`Build`/`RandomDeck`/`ListDeck`/`CatalogDecks` | E5-c1 |
| `draftlab` | game driver `PlayGame` (+ hand/mulligan telemetry, `Verify`), `Games` (parallel), `Play` (roundrobin \| swiss \| none) | E5-c2 |
| `draftlab` | `Gauntlet`, `RunEvent` (event log pipeline), `VerifyEvent`, `Manifest` | E5-c3 |
| `event` (csv.go), `draftlab`, `cmd/labdemo` (new) | `DraftDataOptions`/`DraftDataCSVWith` (per-seat draft id), `draftlab.DraftDataCSV`/`GameDataCSV`/`WriteDecksJSON`, the example workflow | E5-c4 |
| `internal/bench`, `draftlab` | `Hooks.EnvSeats`: drive `bots.EnvSeat` with an honest-root `bots.Env` like the host; lab admits Env players under the opt-in | E5-c5 |

New packages: `draftlab` (E5-c1) and `cmd/labdemo` (E5-c4). Each adds its
`docs/site/architecture/packages.tiers.json` entry **in the same ticket**
(`TestClassificationIsComplete`, `cmd/pkgmap/main_test.go:14`):

```json
"draftlab":    {"tier": "bots", "status": "EXT", "purpose": "Training composition API over the draft stack: open packs, draft, build, play, gauntlet and whole events with named, versioned built-in agents; every call a pure function of inputs and seed; 17Lands-shaped artifacts."},
"cmd/labdemo": {"tier": "cmd", "status": "TOOL"}
```

(`EXT` = "optional extension: bots, search, ML, alternate wires",
`docs/site/architecture/packages.md:19-20`.) No `internal/archtest` allow-list change:
neither package imports `time` (`TestTimeIsImportedOnlyByTheHost`,
`internal/archtest/arch_test.go:122-152`; the allow-list is closed).

## 3. Existing mechanisms relied on (verified at 705b3811a)

### 3.1 limited/draft

- `const Seats = 4; Rounds = 3` (`limited/draft/draft.go:27-28`); `Pod.packs
  [Seats][Rounds][]limited.CardRef` and every per-seat field is a `[Seats]` array
  (`:81-92`); `New(seed, packs [Seats][Rounds][]limited.CardRef) *Pod` (`:100`).
- Passing: `next := (seat + passOffset(p.round)) % Seats` (`:167`); `passOffset` is
  `Seats-1` in round 2 and `1` otherwise (`:203-208`); `openNextRound` opens round r+1
  when every queue is empty (`:212-226`).
- Interleaving independence is pinned by `TestPodInterleavingIndependence`
  (`limited/draft/draft_test.go:257`); helpers `mkPack`/`mkPacks` (`:12-36`).
- Users of the fixed shape outside the package: `event/event.go:66-67,402-408` and
  eventhost's 4-seat arrays (§4.4). `draft.New` has exactly one non-test caller
  (`event/event.go:408`).

### 3.2 event

- 4-seat state: `seats [4]SeatSpec` (`event/event.go:61`); `packs [draft.Rounds][draft.Seats]`,
  `packOpen` (`:66-67`); `decks [draft.Seats]`, `ready [draft.Seats]int` (`:73-74`);
  `Seats() [4]SeatSpec` (`:141`, only caller outside the package's own code is
  `event/logtags_test.go:74`).
- `EventCreated` demands exactly `draft.Seats` seats (`:333-335`); `PackOpened` round
  `1..draft.Rounds`, seat `0..draft.Seats-1` (`:361-366`); the pod is built once
  `packCount == draft.Seats*draft.Rounds` (`ensurePod`, `:398-410`).
- **Pack rounds double as event rounds**: `roundAdvance` stops at `draft.Rounds`
  (`:815`), `EventEnded` requires `RoundPhase(draft.Rounds)` (`:828`). Round phases are
  listed literally as `RoundPhase(1), RoundPhase(2), RoundPhase(3)` in `applyDeckSaved`
  (`:458`) and `applyIdleTimeout` (`event/timeouts.go:33`).
- Round robin: `Pairings(round)` = `roundRobin(draft.Seats, round)` (`:220-222`);
  `roundRobin(n, r)` already handles any even n (`:227-254`). `startRounds` pairs round 1
  (`:542-547`) once `allReady` (`:525-527`).
- Seat bounds use `draft.Seats` in `Next`/`Pool`/`SavedDeck`/`DeckRev` (`:155,164,172,183`),
  every Apply guard (`:423,463,508,730,859`), `SeriesOf` (`event/series.go:104`),
  `SavedRev`/`ReadyRev`/`Queued` (`event/attempt.go:108,117,130`), `applyIdleTimeout`
  (`event/timeouts.go:37`); `LogFor`'s doc says "viewer >= 4 is a spectator"
  (`event/attempt.go:139`).
- Standings: `ComputeStandings(results)` aggregates into `[4]seatAgg` and silently
  ignores seats ≥ 4 (`event/standings.go:50-58`); `Standings()` converts the finished
  series (`:150-167`).
- `StateJSON` snapshot uses `[4]SeatSpec`, `[4][]CardRef`, `[4][]deckJSON`, `[4]int`
  (`event/event.go:887-898`). No golden pins its bytes today (no `event/testdata/`).
- Log: kinds (`event/log.go:28-50`, `Sideboarded` last); `Entry` (`:56-117`) — every
  field `omitempty`; `cloneEntry` deep-copies slices/pointers (`:121-145`); `ParseLog`
  whitelists kinds (`:176-183`). Old logs are pinned by `TestE0LogShapeUnchanged`
  (`event/attempt_test.go:138`) and `TestOldLogReplaysAfterTagChange`
  (`event/logtags_test.go:16`).
- Export: `DraftDataCSV` writes `expansion, "gorge_pod", draft_id = the event seed (for
  every seat), pack_number, pick_number, pick, pack_card_*, pool_*`
  (`event/csv.go:96-103`); rows come from replaying the log (`draftDataRows`, `:75-115`).

### 3.3 cmd/draftsim

- Flags (`cmd/draftsim/main.go:26-40`); `-players` must name exactly 4 (`:74-82`).
- Env refusal before anything else (`cmd/draftsim/run.go:70-77`, and for the gauntlet
  player `cmd/draftsim/gauntlet.go:76-78`) — spec §4.8 item 3.
- Corpus via `testutil.OpenCorpusRegistry` (`run.go:96`), which is `cards.SharedCorpus`
  (`internal/testutil/decks.go:186-188`). A registry "is read-only after open"
  (`cards/open.go:56-58`).
- Seeds: `pickRNG(seed, seat)` salt `0x5011C1<<32|seat`, one stream per seat
  (`cmd/draftsim/event.go:18,26-28`); `gameSeed(seed, round, a, b, game)` salt
  `0x6A111E` (`:21,32-36`); gauntlet salts `0x6A111F`, random deck `0x8AD5E5`
  (`gauntlet.go:21-24`), `gauntletSeed` (`:216-220`).
- `runOneEvent` (`event.go:56-136`): `[4]event.SeatSpec` personas
  `"<set>/<drafter>/<player>"` (`:58-65`), PackOpened seat-major (`:74-80`), the draft
  loop (`:84-104`), `buildDecks` (`:182-205`), the `noGames` early return (`:112-117`),
  Bo1 `playSeries` (`:209-240`); `openPacks` = `booster.OpenPod(spec, seed, draft.Seats,
  draft.Rounds, ids, colourOf)` (`:151-154`).
- Games: `driveGame` (`game.go:149-183`) builds seats with `bots.New(p,
  bots.Options{Seed: seed ^ uint64(i+1), AutoPayMana: true, Deps})` (`:152-156`), London
  by land count for `*seat.Bot` (`:163-165`), `bench.PlayGame` with
  `NewEngine: rules.NewStartingPlayerChoice`, `Setup: AskStartingPlayer` (`:168-178`);
  watchdogs 150 turns / 40000 intents, 2 mulligans (`:21-28`); `gameConfig` (`:189-198`);
  `-verify` replays each chain with `replay.Replay` (`:99-103`) and re-drives each event
  game for its head (`replayGameHead`, `:110-129`; `verifyEvent`, `run.go:242-270`).
- N=4 arrays: `printStandings` personas `[4]string` (`run.go:293`); gauntlet tallies
  `[draft.Seats]gauntletTally` (`gauntlet.go:283-301`).
- `-draftlog` header-once append (`run.go:114-126,159-168`); `sheetNames(spec)` reads
  `spec.Sheets` (`run.go:222-235`) — E5-a1 changes that shape.
- `randomDeck` (23 random non-land pool cards + `AutoLands`, `gauntlet.go:226-259`);
  `wilsonCI` (`:333-343`).
- Measured (705b3811a, capped): `TestDraftsimEventRunsAndVerifies` 1.17 s for 6 games
  with -verify (`go test -run 'TestDraftsimEventRunsAndVerifies' -v ./cmd/draftsim`).

### 3.4 eventhost (unchanged by E5-b/E5-c)

- Opens its pod with local `draftSeats = 4`, `draftRounds = 3`
  (`eventhost/cards.go:43,48-49`); refuses `len(req.Seats) != draft.Seats`
  (`eventhost/host.go:354-356`); `meta.Seats [4]metaSeat` persisted (`store.go:30`).
- Reads 4-seat round-robin schedules through `event.Pairings(rd)` for rd 1..3
  (`eventhost/state.go:41`, `eventhost/frames.go:71`) and lists `event.RoundPhase(1..3)`
  (`eventhost/build.go:57,136`, `rounds.go:198`, `sideboard.go:30`, `timeouts.go:220,278`).
- So E5-b must keep: `draft.Seats == 4`, `draft.Rounds == 3`, `draft.New`'s signature,
  `event.Pairings(round)` as the 4-seat round robin, `event.ComputeStandings(results)` for
  4 seats (`host/httpapi/events_bo3_e2e_test.go:690`, `eventhost/series_test.go:241`), and
  a 4-seat default event (no Mode/Structure fields) behaving byte-for-byte as today.

### 3.5 Engine, bots and bench

- `bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks)` (`internal/bench/bench.go:171`)
  is a pure function of (cfg, seats) (`:169-170`); `Outcome{WinnerSeat, Draw, StallOn,
  Turns, Intents, Starter, StarterSet}` (`:61-85`); `IsAbort` (`:95`); `Hooks.NewEngine`
  (`:159`), `Hooks.HonestRoot` (`:144`), `Hooks.Decision` (`:133`).
- bench feeds and drives **`searchseat.SearchSeat`** only (`:208-224`, `:285-327`). It has no
  `bots.EnvSeat` branch: an Env policy built by `bots.New` (a hosted wrapper, e.g.
  sb-tactical's `hostedSeat.DecideEnv`, `bots/sbtactical/sbtactical.go:140`) would be driven
  as a plain seat and never receive its `bots.Env`. The host builds that Env in
  `(*match).envData` (`host/botenv.go:178-230`) after detecting `bots.EnvSeat`
  (`host/match.go:549`, feeds `host/botenv.go:50-70`).
- `bots.Info.Env` (`bots/registry.go:47`), `ReadsDecklists` (`:149`), `Normalize`/`New`
  (`:117,127`); `Options.DecisionDeadlineMS` zero = unbounded, the bench/training setting
  (`:64-72`). `bots.EnvSeat`/`bots.Env` (`bots/env.go:16-43`), `RootSeed` (`:53`).
  Env policies at 705b3811a: `az-redeal`, `az-c3` (registers only via `azc3.Enable`),
  `sb-search`, `sb-tactical`, `search`, the sbbuiltin trio; non-Env: `bot`,
  `lethal-pressure`, `cast-profile` (`bots/all/all.go`, each package's `Register`).
- The host's opt-in: `TableConfig.AllowDecklistBots` (`host/table.go:205-216`), refused at
  any human table and otherwise required for an Env policy (`host/table.go:280-287`).
- `deck.File` (`deck/deck.go:31`), `File.Resolve(reg)` (`:281`); `build.ToDeckFile`
  (`limited/build/build.go:160`); spellbench catalogs `CatalogByID`
  (`internal/spellbench/fdn.go:42`), `CatalogIDs`/`File(dir, id) (deck.File, error)`
  (`internal/spellbench/decks.go:50,68`).
- The "card in hand" fold (`inHand`, `cmd/dzgorge/play.go:651-682`): library→hand moves and
  draws, minus hand→library before the first `TurnChange`.
  `decision.KMulligan` (`decision/decision.go:29`).
- Pickbot: `Drafter{Pick(v PickView, r *rand.Rand) string}`, `Builder{Build(set, pool)
  build.Deck}` (`limited/pickbot/registry.go:93-100`); registry `rating-colour` and `best23`
  both map to (RatingColour, Best23) (`:112-119`); `New(name, lookup)` builds fresh
  instances (`:135-143`).

## 4. API of record

### 4.1 limited/draft (E5-b1)

```go
// Seats and Rounds are the hosted pod's shape (eventhost, draft.New). A pod built
// with NewPod may have any shape in [MinSeats, MaxSeats] x [1, ...].
const (
    Seats    = 4
    Rounds   = 3
    MinSeats = 2
    MaxSeats = 8
)

// ErrPodShape reports packs that do not form a pod: fewer than MinSeats or more than
// MaxSeats seats, no rounds, or seats holding different numbers of rounds.
var ErrPodShape = errors.New("draft: bad pod shape")

// NewPod starts a pod of len(packs) seats and len(packs[0]) rounds: packs[s][r-1] is the
// pack seat s opens in round r. Odd rounds pass left (seat+1)%n, even rounds right
// (seat+n-1)%n — for n == 4 and 3 rounds exactly New's flow.
func NewPod(seed uint64, packs [][][]limited.CardRef) (*Pod, error)

// New is NewPod over the hosted 4x3 shape (unchanged signature; cannot fail).
func New(seed uint64, packs [Seats][Rounds][]limited.CardRef) *Pod

func (p *Pod) Seats() int  // pod size
func (p *Pod) Rounds() int // pack rounds
```

`Next` with a seat out of range returns `(PickView{}, false)`; `Pick` returns
`ErrNotYourTurn`; `Pool` returns nil; `QueueLen` returns 0 (today they index arrays and
would panic). `Done`, `Record`, `PickView` unchanged.

### 4.2 event (E5-b2)

```go
// shape.go (new)
const (
    ModeDraft  = "draft"  // the zero Mode: a booster draft (spec §4.4)
    ModeSealed = "sealed" // every seat opens PacksPerSeat packs into its pool

    StructureRoundRobin = "roundrobin" // the zero Structure: circle method, n-1 rounds by default
    StructureSwiss      = "swiss"      // E5-b3
    StructureNone       = "none"       // no games: the event ends after BUILD

    DraftPacks         = 3  // packs per seat in a draft (draft.Rounds)
    DefaultSealedPacks = 6
    MaxSealedPacks     = 12
)

// log.go Entry: appended EventCreated fields, all omitempty (a 4-seat draft round-robin
// EventCreated marshals exactly as before).
Mode      string `json:"mode,omitempty"`      // "" = draft
Structure string `json:"structure,omitempty"` // "" = roundrobin
Packs     int    `json:"packs,omitempty"`     // packs per seat; 0 = the mode's default
Rounds    int    `json:"rounds,omitempty"`    // event rounds; 0 = the structure's default
Collation string `json:"collation,omitempty"` // the booster collation (E5-a1), informational

// accessors (shape.go)
func (e *Event) NumSeats() int        // 0 before EventCreated
func (e *Event) Mode() string         // ModeDraft or ModeSealed (normalized)
func (e *Event) Structure() string    // normalized
func (e *Event) Rounds() int          // event rounds; 0 for StructureNone
func (e *Event) PacksPerSeat() int
func (e *Event) Collation() string
func (e *Event) Seats() []SeatSpec    // WAS [4]SeatSpec; a copy
func (e *Event) CurrentPairings() [][2]int // a copy of the current round's pairs

// pairings.go (new; roundRobin moves here, exported)
func RoundRobinPairings(n, round int) [][2]int // circle method, seat 0 fixed; nil for odd n or round outside 1..n-1
func Pairings(round int) [][2]int               // unchanged: RoundRobinPairings(draft.Seats, round)

// standings.go
func ComputeStandingsN(n int, results []SeriesResult) []Standing // n rows
func ComputeStandings(results []SeriesResult) []Standing        // unchanged: ComputeStandingsN(4, results)
```

EventCreated validation (exact): `len(Seats)` ∈ {4, 8}; `Mode` ∈ {"", draft, sealed};
draft: `Packs` ∈ {0, 3}; sealed: `Packs` = 0 (→ 6) or 1..12; `Structure` ∈ {"",
roundrobin, none} (E5-b3 adds swiss; until then swiss is refused with "not implemented
until E5-b3"); roundrobin: `Rounds` = 0 (→ n-1) or 1..n-1; none: `Rounds` must be 0. The
logged entry is stored **as given** (defaults are derived, never written back), so old
logs re-marshal byte-identically.

Behaviour:
- The **supply phase** keeps the name `DRAFT` for both modes (a sealed event is in DRAFT
  only while its packs are logged). `PackOpened.Round` is the pack index `1..PacksPerSeat`.
- Draft mode: unchanged semantics, the pod built with `draft.NewPod` once all `n*3` packs
  are logged.
- Sealed mode: when the last of `n*packs` packs is logged the phase becomes BUILD;
  `Pool(seat)` is the seat's packs concatenated in pack order; `Next` is always false;
  `Picked` is refused ("a sealed event has no draft"); `Queued` is 0.
- Round phases: one helper `inRound()` (`round >= 1 && phase == RoundPhase(round)`)
  replaces the literal `RoundPhase(1..3)` lists in `applyDeckSaved` and
  `applyIdleTimeout`.
- `startRounds` (round robin): round 1 = `RoundRobinPairings(n, 1)`. `roundAdvance` stops
  at `e.Rounds()`. `StructureNone`: `allReady` does not open a round; `EventEnded` is legal
  in BUILD once every seat is ready, with n zero-score standings rows.
- `EventEnded` otherwise requires `RoundPhase(e.Rounds())` with every series ended.
- `StateJSON`'s snapshot uses slices of length n; a cooked 4-seat event marshals
  byte-identically to 705b3811a (a JSON array of 4 and a slice of 4 encode the same).
- `LogFor(viewer)`: a viewer outside `0..n-1` is a spectator.

### 4.3 event Swiss (E5-b3)

```go
// swiss.go (new)
// SwissPairings pairs n seats (n even, >= 2) for the given round from the completed
// series results. Pure: the same inputs give the same pairs.
func SwissPairings(n, round int, results []SeriesResult, seed uint64) ([][2]int, error)

// log.go — appended after Sideboarded
KindRoundPaired Kind = "RoundPaired" // a Swiss round's pairs, chosen by the driver
Pairs [][2]int `json:"pairs,omitempty"` // RoundPaired: lower seat first, pairing order

// event methods
func (e *Event) AwaitingPairing() (round int, ok bool) // swiss: the round a RoundPaired must open
func (e *Event) SuggestedPairings() ([][2]int, error)  // SwissPairings(n, round, e.SeriesResults(), e.Seed())
func (e *Event) SeriesResults() []SeriesResult         // finished series in log order (Standings' conversion)
```

Swiss default rounds: `ceil(log2 n)` — 2 for 4 seats, **3 for 8**; allowed 1..n-1.

**Algorithm (exact, so two implementations agree):**
1. `rng := rand.New(rand.NewPCG(seed, swissSalt<<32|uint64(round)))` with
   `swissSalt = 0x5A1550`; `key := rng.Perm(n)` (key[s] is seat s's tie-break rank).
2. `points[s]` = 3 per series won, 1 per drawn series, from `results`; `met[s][t]` true
   when s and t have a result.
3. `order` = seats sorted by points descending, then `key` ascending. (Round 1: everyone
   has 0 points, so `order` is the seeded permutation.)
4. Backtracking: take the first unpaired seat `u` in `order`; try every unpaired `v`
   after it in `order` with `!met[u][v]`; recurse; the first complete matching wins.
5. If no rematch-free matching exists, pair adjacent seats of `order` (rematches allowed).
6. Each pair is normalized lower seat first; pairs are returned in the order their first
   member was taken in step 4/5 (table 1 = the top bracket).
Errors: odd n, n < 2, round < 1, a result naming a seat outside 0..n-1.

**Event flow:** after every seat is Ready, a swiss event stays in BUILD with
`AwaitingPairing() == (1, true)`; `RoundPaired{Round: 1, Pairs}` opens ROUND1. When every
series of round r has ended and r < Rounds, the event stays in ROUND r awaiting
`RoundPaired{r+1}`. `applyRoundPaired` validates the shape only — structure is swiss, the
round is the awaited one, `Pairs` is a perfect matching of 0..n-1 with `a < b` — **not**
equality with `SuggestedPairings` (a log keeps its pairings across an algorithm change).
Round-robin events never log `RoundPaired` (and refuse it).

Driver loop (draftsim and draftlab):

```go
for {
    if r, ok := e.AwaitingPairing(); ok {
        pairs, err := e.SuggestedPairings() // ...
        apply(Entry{Kind: KindRoundPaired, Round: r, Pairs: pairs})
        continue
    }
    pending := e.PendingSeries()
    if len(pending) == 0 { break }
    // play every pending series, apply its entries in pairing order
}
apply(Entry{Kind: KindEventEnded})
```

### 4.4 cmd/draftsim (E5-b4)

`draftsim [-seats 4|8] [-mode draft|sealed] [-structure roundrobin|swiss|none]
[-rounds N] [-collation …] …` — `-structure` default "" = roundrobin at 4 seats, swiss at 8;
`-players` names one policy (every seat) or exactly `-seats` policies; `-draftlog` with
`-mode sealed` is refused; Env policies stay refused (spec §4.8.3; draftsim gets no
opt-in). With default flags the output is byte-identical to today's.

### 4.5 draftlab (E5-c1..c5)

```go
package draftlab

// ---------- lab.go (c1)
type Options struct {
    // AllowDecklistBots admits Player policies with bots.Info.Env (they read every
    // seat's real decklist). The lab seats bots only, so this is the bot-only opt-in of
    // the operator ruling of 2026-10-13 (host.TableConfig.AllowDecklistBots's twin).
    AllowDecklistBots bool
    Workers    int  // games run at once; 0 or 1 = serial; results never depend on it
    Verify     bool // replay every game's chain (replay.Replay); a mismatch is an error
    MaxTurns   int  // 0 = 150 (draftsim)
    MaxIntents int  // 0 = 40000 (draftsim)
    Mulligans  int  // London mulligans per seat; 0 = 2 (draftsim); < 0 = none
}
type Lab struct{ /* reg *cards.Registry; opts Options — immutable after Open */ }
func Open(corpusDir string, o Options) (*Lab, error) // cards.SharedCorpus; o validated
func (l *Lab) Options() Options

// ---------- agents.go (c1)
type Role string
const (
    RoleDrafter Role = "drafter"
    RoleBuilder Role = "builder"
    RolePlayer  Role = "player"
)
// AgentRef names an agent by registry name and frozen version ("bot@1").
// Version "" accepts whatever this build carries; the effective version is recorded.
type AgentRef struct{ Name, Version string }
func ParseAgent(s string) (AgentRef, error) // "name" | "name@version"
func (a AgentRef) String() string           // "name@version", or "name" when Version == ""

// BuiltinVersion is the version this build carries for a built-in agent; false for a
// name the lab's version table does not list (resolvable only with Version "",
// recorded as version "unversioned").
func BuiltinVersion(role Role, name string) (string, bool)

var (
    ErrUnknownAgent = errors.New("draftlab: unknown agent")
    ErrVersion      = errors.New("draftlab: agent version not available in this build")
    ErrDecklistBot  = errors.New("draftlab: player reads every seat's real decklist")
)

type NamedDrafter struct { Ref AgentRef; Drafter pickbot.Drafter }
type NamedBuilder struct { Ref AgentRef; Builder pickbot.Builder }
type NamedPlayer struct {
    Ref AgentRef
    // New builds a fresh seat for one game from the seat's own seed. It is called once
    // per game per seat, possibly from several goroutines at once.
    New func(seed uint64) (seat.Seat, error)
    // ReadsDecklists marks an Env player: refused unless Options.AllowDecklistBots.
    ReadsDecklists bool
}
func (l *Lab) Drafter(ref AgentRef) (NamedDrafter, error) // pickbot.New(name, lookup) drafter half
func (l *Lab) Builder(ref AgentRef) (NamedBuilder, error) // builder half
func (l *Lab) Player(ref AgentRef) (NamedPlayer, error)   // bots.New(name, {Seed, AutoPayMana: true, Deps}) + MulliganLands for *seat.Bot

// ---------- supply.go (c1)
func (l *Lab) OpenPacks(set string, c setdata.Collation, seed uint64, n int) ([][]limited.CardRef, error)            // booster.OpenPod(spec, seed, 1, n)[0]
func (l *Lab) OpenPod(set string, c setdata.Collation, seed uint64, seats, rounds int) ([][][]limited.CardRef, error) // booster.OpenPod(spec, seed, seats, rounds)
func (l *Lab) SealedPool(set string, c setdata.Collation, seed uint64, packs int) ([]limited.CardRef, error)        // OpenPacks flattened, pack order
func (l *Lab) CardNames(set string, c setdata.Collation) ([]string, error)                                          // the sheet universe, sorted (CSV columns)

// ---------- draft.go (c1)
type DraftResult struct {
    Set      string
    Seed     uint64
    Drafters []AgentRef          // effective ref per seat
    Pools    [][]limited.CardRef // per seat, pick order
    DraftIDs []string            // "<seed>-s<seat>": the 17Lands draft_id per seat
    Log      []byte              // event log (JSON lines) through the end of the draft
}
// Draft runs packs ([seat][round], 4 or 8 seats, one set, unique CardRef IDs — open them
// with one OpenPod call) through an event in ModeDraft/StructureNone. drafters has 1
// entry (every seat) or len(packs). Seat s draws its tie-break rng from
// rand.NewPCG(seed, 0x5011C1<<32|s) (draftsim's stream).
func (l *Lab) Draft(packs [][][]limited.CardRef, drafters []NamedDrafter, seed uint64) (DraftResult, error)

// ---------- deck.go (c1)
type Deck struct {
    Name    string
    Set     string            // "" for a list deck
    Source  string            // "build:<ref>", "random", "list", "catalog:<id>"
    DraftID string            // the 17Lands draft_id of the pool, "" when none
    Pool    []limited.CardRef // nil for a list deck
    Build   *build.Deck       // nil for a list deck
    File    deck.File         // the engine-loadable list (basics as entries)
}
func (l *Lab) Build(pool []limited.CardRef, b NamedBuilder) (Deck, error) // set from the pool's CardRefs; build.Validate
func (l *Lab) RandomDeck(pool []limited.CardRef, seed uint64) (Deck, error) // draftsim's randomDeck, stream rand.NewPCG(seed, 0x8AD5E5<<32)
func (l *Lab) ListDeck(f deck.File) (Deck, error)                           // resolves every card or errors
func (l *Lab) CatalogDecks(id string) ([]Deck, error)                       // spellbench catalog ("fdn-limited"), catalog order

// ---------- play.go (c2)
type Entrant struct{ Deck Deck; Player NamedPlayer }
type Matchup struct{ A, B Entrant }
type GameResult struct {
    Seed        uint64
    Round, Game int          // 0 outside a structured Play / event
    A, B        int          // entrant indexes (Play, Gauntlet: 0 = hero) or -1
    Players     [2]string    // AgentRef strings, player 0 first
    Decks       [2]string    // deck names
    DraftIDs    [2]string    // Deck.DraftID per player
    Main        [2][]string  // the deck's card names, sorted, one per copy
    Sideboard   [2][]string  // pool cards not in the main, sorted; nil for list decks
    Winner      int          // 0 or 1 (player index); -1 for a draw or a stall
    Stall       string       // "", "turns", "intents"
    Turns       int
    Intents     int
    Starter     int          // the player on the play
    Mulligans   [2]int       // KMulligan asks answered "mulligan"
    OpeningHand [2][]string  // kept opening hand, sorted
    Drawn       [2][]string  // cards drawn after the first turn began, sorted
    Head        string       // engine chain head
}
func (l *Lab) PlayGame(a, b Entrant, seed uint64) (GameResult, error)
// Games plays ms[i] with seed rand.New(rand.NewPCG(seed, 0x1AB6A3<<32|uint64(i))).Uint64(),
// up to Options.Workers at once; results in input order.
func (l *Lab) Games(ms []Matchup, seed uint64) ([]GameResult, error)

type Structure string
const (
    RoundRobin Structure = "roundrobin"
    Swiss      Structure = "swiss"
    NoGames    Structure = "none"
)
type PlayOptions struct {
    Structure Structure
    Rounds    int // 0 = the structure default (rr: n-1, swiss: ceil(log2 n))
}
type Tournament struct {
    Structure Structure
    Rounds    int
    Pairings  [][][2]int     // per round
    Games     []GameResult   // round order, pairing order within a round
    Standings []event.Standing
}
// Play: Bo1 series between an even number (2..64) of entrants; game seeds
// event-style gameSeed(seed, round, a, b, 1) (salt 0x6A111E); within a round games run in
// parallel, results applied in pairing order.
func (l *Lab) Play(entrants []Entrant, o PlayOptions, seed uint64) (Tournament, error)
// PlayDecks is the brief's shape: decks[i] played by players[i] (or players[0] for all).
func (l *Lab) PlayDecks(decks []Deck, players []AgentRef, s Structure, seed uint64) (Tournament, error)

// ---------- gauntlet.go (c3)
type Tally struct {
    Wins, Losses, Draws, Games int
    WinRate, Lo95, Hi95        float64 // Wilson 95% (draftsim's wilsonCI)
}
type GauntletResult struct {
    Hero        string
    Opponents   []string // "<deck name> / <player ref>"
    PerOpponent []Tally
    Pooled      Tally
    Games       []GameResult // opponent order, then game order
}
// Gauntlet: the hero plays every opponent `games` times, alternating player 0's seat
// (game n: hero sits in seat n%2); seed per game rand.NewPCG(seed,
// 0x1AB6A4<<32 | uint64(j)<<16 | uint64(n)).Uint64().
func (l *Lab) Gauntlet(hero Entrant, opponents []Entrant, games int, seed uint64) (GauntletResult, error)

// ---------- event.go (c3)
type EventSpec struct {
    Set       string
    Collation setdata.Collation
    Mode      string    // event.ModeDraft | event.ModeSealed
    Seats     int       // 4 | 8
    Packs     int       // 0 = the mode's default
    Structure Structure // "" = roundrobin at 4 seats, swiss at 8
    Rounds    int
    Drafters  []NamedDrafter // 1 or Seats; ignored for sealed
    Builders  []NamedBuilder // 1 or Seats
    Players   []NamedPlayer  // 1 or Seats
    Seed      uint64
}
type EventResult struct {
    Log       []byte // the complete event log (JSON lines), COMPLETE
    Decks     []Deck // per seat, DraftID "<seed>-s<seat>"
    Games     []GameResult
    Standings []event.Standing
    Manifest  Manifest
}
func (l *Lab) RunEvent(s EventSpec) (EventResult, error)
// VerifyEvent replays r.Log byte-identically and re-drives every game for its head.
func (l *Lab) VerifyEvent(r EventResult, players []NamedPlayer) error

// ---------- manifest.go (c3)
type Manifest struct {
    Format            string   `json:"format"`         // "gorge-draftlab/1"
    GorgeRevision     string   `json:"gorge_revision"` // vcs.revision (+"+dirty"), else module version, else "devel"
    Kind              string   `json:"kind"`           // "event" | "tournament" | "gauntlet" | "games"
    Seed              uint64   `json:"seed"`
    Set               string   `json:"set,omitempty"`
    Collation         string   `json:"collation,omitempty"`
    Mode              string   `json:"mode,omitempty"`
    Structure         string   `json:"structure,omitempty"`
    Seats             int      `json:"seats,omitempty"`
    Agents            []string `json:"agents"` // sorted unique "role:name@version"
    AllowDecklistBots bool     `json:"allow_decklist_bots,omitempty"`
    MaxTurns          int      `json:"max_turns"`
    MaxIntents        int      `json:"max_intents"`
    Mulligans         int      `json:"mulligans"`
    Games             int      `json:"games"`
    Draws             int      `json:"draws"`
    Stalls            int      `json:"stalls"`
}
// RoleRef is one agent in its role, spelled "role:name@version".
type RoleRef struct{ Role Role; Ref AgentRef }
func (l *Lab) NewManifest(kind string, seed uint64, agents []RoleRef, games []GameResult) Manifest
// Tournament and GauntletResult also carry a Manifest (kind "tournament" / "gauntlet").

// ---------- csv.go (c4)
// event/csv.go gains DraftDataOptions{EventType string; PerSeatDraftID bool; Header bool}
// and DraftDataCSVWith(e, w, names, o); DraftDataCSV / DraftDataCSVRows unchanged.
func DraftDataCSV(w io.Writer, eventLog []byte, names []string, header bool) error // per-seat draft_id
func GameDataCSV(w io.Writer, games []GameResult, set, eventType string, names []string, header bool) error
func WriteDecksJSON(w io.Writer, decks []Deck) error
```

(`Workers`, `Verify` are run options, not results: they never appear in a result or a
manifest.)

### 4.6 bench Env driver (E5-c5)

```go
// internal/bench/bench.go Hooks, appended:
// EnvSeats drives every seat that implements bots.EnvSeat: when WantsEnv(d) is true the
// seat gets DecideEnv(ctx, bots.Env{...}) built exactly as host/botenv.go envData builds
// it (the deciding seat's own feed, an honest root from searchseat.HonestRoot at
// bots.RootSeed(cfg.Seed^(seat+1), d.Seq), the projected View, the Board), never the
// live engine. false (the zero value) changes nothing.
EnvSeats bool
```

The lab sets `Hooks{EnvSeats: true, HonestRoot: true}` for any game with an Env player and
leaves Hooks zero otherwise (so non-Env games stay byte-identical to draftsim's driver).

## 5. Determinism

| stream | derivation | where |
|---|---|---|
| pack (seat, i) | `rand.NewPCG(seed, seat<<32 \| i)` | `booster.OpenPod` (`limited/booster/pod.go:21-45`), unchanged |
| picks, seat s | `rand.NewPCG(seed, 0x5011C1<<32 \| s)`, one stream per seat | draftsim, lab Draft/RunEvent |
| event game | `gameSeed(seed, round, a, b, game)` salt 0x6A111E | draftsim, lab Play/RunEvent |
| swiss round r | `rand.NewPCG(seed, 0x5A1550<<32 \| r).Perm(n)` | `event.SwissPairings` |
| random deck | `rand.NewPCG(seed, 0x8AD5E5<<32)` (caller derives per-seat seeds) | lab RandomDeck |
| lab Games i | `rand.NewPCG(seed, 0x1AB6A3<<32 \| i).Uint64()` | lab Games |
| lab gauntlet (j, n) | `rand.NewPCG(seed, 0x1AB6A4<<32 \| j<<16 \| n).Uint64()` | lab Gauntlet |
| bot seat i | game seed `^ uint64(i+1)` | `cmd/draftsim/game.go:153`, lab |

- A sealed event at seed s opens packs (seat, 0..5); its packs 0..2 equal a draft's at
  seed s (same salts). Accepted and documented; workflows that do both use distinct seeds.
- Within a round, games run in parallel and are **applied in pairing order**; a Swiss round
  r+1 is paired only after round r's entries are applied. The worker count never reaches a
  result, a log line or a manifest.
- No `time` import in any E5-b/E5-c package (`TestTimeIsImportedOnlyByTheHost`). 17Lands
  wall-clock columns (`draft_time`, `game_time`) are written empty. No map range reaches
  output: sort keys (the CSV name lists are sorted; `Manifest.Agents` sorted).
- `GorgeRevision` comes from `runtime/debug.ReadBuildInfo` and is constant within a
  process; tests compare manifests from one process.
- `cards.Registry` is read-only after open (`cards/open.go:56-58`); the lab never mutates
  it. Concurrency safety of the rules layer's registry-keyed memos is **INFERRED** from
  cmd/dzgorge's parallel workers sharing one registry; E5-c2 pins it with a
  workers-1-vs-4 equality test.

## 6. Bot-only Env opt-in

- `draftsim` refuses every Env policy, always (spec §4.8 item 3; `run.go:70-77`,
  `gauntlet.go:76-78`) — unchanged.
- The lab has no human seat type, so every lab run is bot-only. `Lab.Player(ref)` returns
  `ErrDecklistBot` for `bots.ReadsDecklists(name)` unless `Options.AllowDecklistBots`; a
  caller-built `NamedPlayer` declares `ReadsDecklists`, and `PlayGame` additionally treats
  any constructed seat implementing `bots.EnvSeat` as Env (fail closed).
- Until E5-c5 lands, even with the opt-in an Env player is refused with "Env players need
  the bench Env driver (E5-c5)": bench would otherwise drive the hosted wrapper as a plain
  seat (§3.5) and the player would silently play without its Env.
- With E5-c5 the Env player gets the host's honest-root Env (redeal from its own feed), and
  every result touching it is marked by `Manifest.AllowDecklistBots = true`.
- `az-c3` is registered only by `azc3.Enable(path)` (`bots/azc3/azc3.go:65-79`); a
  training program that wants it calls `Enable` before `Lab.Player`.

## 7. Hidden information and the server-authoritative rule

E5-b/E5-c touch no HTTP, wire or SSE surface, so the E1/E3 server-authoritative checklist
has no new entries. If a seat finds itself editing `protocol/`, `host/httpapi`, `eventhost`
or `web/`, it stops and reports. `event.LogFor` redaction is unchanged in kind; E5-b2 only
generalizes "spectator" to `viewer outside 0..n-1`. Sealed `PackOpened` cards are private
to their seat until COMPLETE exactly as draft packs are.

## 8. Test budget and runtime

- 4-seat round robin: 6 games; 8-seat Swiss 3 rounds: 12 games; 8-seat round robin: 28.
  At 1.17 s per 6 verified games (§3.3) an 8-seat Swiss event with -verify is ~2.5 s and
  an 8-seat round robin ~6 s — **INFERRED** from that one measurement.
- Lab tests use small shapes: 4-seat events, `MaxTurns` default, gauntlet ≤ 2 opponents ×
  2 games, one 8-seat draft without games. Every test ≤ 1 min, capped per AGENTS.md.

## 9. The E5-d seam (designed, not implemented)

E5-d adds out-of-process agents. Everything plugs into the three E5-c value types; no lab
signature changes.

- **Drafter adapter** — a `pickbot.Drafter` whose `Pick(v, r)` sends
  `{set, round, pick_no, pack, picks, seen, seed: r.Uint64()}` and reads back one CardRef
  ID. Passing a drawn seed instead of the `*rand.Rand` keeps the adapter a pure function of
  the request. An ID not in the pack is a hard error (`event.Apply` already rejects it,
  `ErrNotInPack`).
- **Builder adapter** — a `pickbot.Builder` sending `{set, pool}` and reading
  `build.Deck` (IDs + basics); the lab's `Build` already runs `build.Validate`.
- **Player adapter** — a `NamedPlayer.New` returning a `seat.Seat` (or `seat.BoardSeat`)
  whose decisions go out as the seat's own `view.View` (never omniscient) plus the
  decision; answers come back as `decision.Intent`. **Batching**: one adapter instance per
  external model serves every concurrent game (`Options.Workers` games); requests are
  queued and flushed when every in-flight game is blocked on the model (a barrier, not a
  timer — the lab cannot import `time`), so a batch's composition depends only on the game
  set, and each answer only on its own request. A model that is a pure function per
  request keeps results worker-count independent.
- **Wire** — JSON lines over a subprocess's stdin/stdout (`os/exec`) or a Unix socket;
  versioned by the adapter's `AgentRef.Version` (the model id), recorded in the manifest.
  Cancellation is the caller's `context`; the lab has no deadline (no `time`).
- **Never human** — an adapter is a bot. A human-facing seat is eventhost's job (E5-e).
- Package: `draftlab/extern` (new, E5-d), so `draftlab` itself never gains `os/exec`.

## 10. Risks

1. **The event refactor reaches eventhost and E3.** event's arrays become slices and four
   literal round lists become a helper. Mitigation: E5-b2 commits a `StateJSON` golden for a
   4-seat event captured **before** its edit, keeps every 4-seat API eventhost calls, and
   runs the eventhost/httpapi acceptance tests in its Done means.
2. **Seat size of E5-b2.** It touches most of `event/event.go`. Mitigation: new logic goes
   in new files (`shape.go`, `sealed.go`, `pairings.go`); `event.go` edits are mechanical
   substitutions listed line by line in the brief.
3. **No rematch-free Swiss matching.** Possible for larger n; the algorithm falls back to
   adjacent pairing (tested by a forced case).
4. **Fingerprint brittleness.** Drafter/builder fingerprints are over committed name-only
   packs (independent of booster/setdata); a corpus pin that changes FDN card facts can
   still move the builder's fingerprint. The fix is regenerate-with-a-note, not a version
   bump, unless the agent's own code or ratings changed (open question 2). Players carry a
   declared version only (open question 3).
5. **Parallel determinism** rests on the registry memos being race-free (INFERRED, §5);
   E5-c2's workers-1-vs-4 test and `-race` on that one test pin it.
6. **Env driver fidelity (E5-c5).** bench's copy of the host's Env build can drift from
   `host/botenv.go`. Mitigation: c5 cites the host function line by line and adds a
   determinism test; a parity test against the host is out of scope (noted).
7. **17Lands column fidelity.** The `game_data` column list is from public 17Lands datasets
   as remembered, **INFERRED** (§11); gorge writes a declared subset in a fixed order.
8. **E5-a1 drift.** E5-b4 and E5-c1 use E5-a1's names; if E5-a1 lands with a different
   collation API, those seats use what landed and report the difference.
9. **Hot docs.** `docs/agents/repo-map-engine.md` / `repo-map-paths.md` are hot
   (`scripts/hotfiles-notes.json`); only E5-b4 and E5-c4 touch them, one line each, and
   they are sequenced (c4 after b4).

## 11. Artifacts

- **Event log**: `event.MarshalLog` JSON lines (unchanged format; new fields omitempty).
- **Decks**: `WriteDecksJSON` — one JSON array of `{name, set, source, draft_id, file:
  deck.File, build: build.Deck|null}` in seat/input order.
- **draft_data** (17Lands shape, `event/csv.go:13-22`): unchanged columns; the lab writes
  `draft_id = "<seed>-s<seat>"` (one draft per drafter, as 17Lands' id is per user draft)
  and `event_type` `gorge_pod` (4 seats) / `gorge_pod8` (8 seats). `draftsim -draftlog`
  keeps today's `draft_id = seed` (open question 4).
- **game_data** (17Lands shape, **INFERRED** column names): one row per player per
  finished game; draws and stalls are omitted (17Lands has no draws; counted in the
  manifest). Columns, in this order: `expansion, event_type, draft_id, draft_time,
  game_time, build_index, match_number, game_number, rank, opp_rank, main_colors,
  splash_colors, on_play, num_mulligans, opp_num_mulligans, opp_colors, num_turns, won`,
  then `deck_<name>`, `sideboard_<name>`, `opening_hand_<name>`, `drawn_<name>` count
  columns over the sorted name list. gorge values: `draft_time`, `game_time`, `rank`,
  `opp_rank` empty; `build_index` 1; `match_number` = round (1 outside an event);
  `game_number` = game (1); colours: a colour is **main** when ≥ 4 non-land main cards
  carry it in their colour identity, **splash** when 1–3 do, WUBRG order; `on_play`,
  `won` are `True`/`False`.
- **Manifest**: §4.5, JSON with sorted keys as declared.

## 12. Spec, brief and code: conflicts and the fix adopted

| # | says | code says | fix |
|---|---|---|---|
| C1 | brief: "a Go composition API package (e.g. `limited/lab`)" | spec §3: every `limited/*` package is pure — no `rules`, `bots`, … (`limited/imports_test.go:16-31` pattern); the lab plays games | top-level **`draftlab`** (tier bots/EXT) |
| C2 | pod size 4 or 8 (§7) | `draft.Seats = 4` fixed arrays (`limited/draft/draft.go:27,81-92`); event `[4]` state (`event/event.go:61-74`) | slices; keep `Seats`, `Rounds`, `New` for eventhost (E5-b1, b2) |
| C3 | Swiss "3 rounds for 8" | pack rounds double as event rounds (`event/event.go:815,828`) | `Rounds` decoupled: event field + structure default (E5-b2) |
| C4 | Swiss pairing | round robin only (`event/event.go:220-254`) | `SwissPairings` + logged `RoundPaired` (E5-b3) |
| C5 | determinism / replay (spec §3) | a derived Swiss pairing would re-pair old logs after an algorithm change | `RoundPaired` logged; Apply validates the shape only |
| C6 | standings for 8 | `[4]seatAgg` ignores seats ≥ 4 (`event/standings.go:51,57`) | `ComputeStandingsN` (E5-b2) |
| C7 | ROUNDn generic | `RoundPhase(1..3)` literal lists (`event/event.go:458`, `event/timeouts.go:33`, eventhost) | `inRound()` in event; eventhost's lists stay valid at 4 seats |
| C8 | sealed "6 packs → pool" | PackOpened round ≤ `draft.Rounds` (`event/event.go:361`); pool only from the pod (`:162-168`) | Mode sealed: pack index ≤ Packs, pool = concat, BUILD after supply (E5-b2) |
| C9 | eventhost "keeps 4 unless trivially parameterised" | persisted `meta.Seats [4]metaSeat` (`eventhost/store.go:30`), 4-wide arrays (`eventhost/host.go:82-108`) | **not trivial: eventhost stays at 4**; no eventhost edit in E5 b/c; hosted 8 = E5-e |
| C10 | Env opt-in for bot-only runs | draftsim refuses Env (spec §4.8.3, `run.go:70-77`); bench cannot drive `bots.EnvSeat` (`internal/bench/bench.go:208-224,285-327`) | draftsim unchanged; lab `AllowDecklistBots` + bench Env driver (E5-c5); refused until then |
| C11 | brief `Draft(packs, drafters, seed)`, `Build(pool, builder)` | `pickbot.Builder.Build(set, pool)` needs the set; `EventCreated` needs `Set` | set taken from the CardRefs; mixed or empty set is an error |
| C12 | brief `Play(decks, players []string, …)` | a player is a constructor (E5-d seam) | `Play(entrants, …)` plus `PlayDecks(decks, []AgentRef, …)` in the brief's shape |
| C13 | 17Lands `draft_data` per drafter | `draft_id` = the event seed for every seat (`event/csv.go:99`) | `DraftDataOptions.PerSeatDraftID` (lab uses it; default unchanged) |
| C14 | 17Lands `game_data` time columns | no `time` outside the allow-list (`internal/archtest/arch_test.go:122-152`) | written empty |
| C15 | spec §1 "Out of scope: 8-seat, sealed, Swiss" | — | superseded by spec §7 (E5 decisions) |
| C16 | E5-a1 collation API | not on main | E5-b4/E5-c1 depend on E5-a1 and use its names |
| C17 | "version string" per agent | neither `bots.Info` nor the pickbot registry carries a version (`bots/registry.go:34-57`, `limited/pickbot/registry.go:102-129`) | the lab's own version table + drafter/builder fingerprints (E5-c1); no registry change |

## 13. Tickets

| # | title | depends on | seat |
|---|---|---|---|
| E5-b1 | limited/draft: N-seat pod (`NewPod`, 2..8 seats, parity passing) | — | local OK |
| E5-b2 | event: 4/8-seat events, sealed supply, structure roundrobin/none, decoupled rounds | E5-b1 | local OK (largest b ticket) |
| E5-b3 | event: Swiss pairing, logged `RoundPaired` | E5-b2 | local OK |
| E5-b4 | draftsim: `-seats`, `-mode`, `-structure`, `-rounds` | E5-b3, E5-a1 | local OK |
| E5-c1 | draftlab: package, agents@versions, packs, draft, decks | E5-b2, E5-a1 | local OK |
| E5-c2 | draftlab: game driver, `Games`, `Play` (rr/swiss/none) | E5-c1, E5-b3 | local OK |
| E5-c3 | draftlab: `Gauntlet`, `RunEvent`, `VerifyEvent`, `Manifest` | E5-c2 | local OK |
| E5-c4 | draftlab artifacts (CSV/decks) + `cmd/labdemo` example workflow | E5-c3, E5-b4 | local OK |
| E5-c5 | internal/bench Env driver; lab admits Env players under the opt-in | E5-c2 | paid (DeepSeek) preferred: engine-adjacent |

Nine. E5-b is serial (b1 → b2 → b3 → b4). E5-c1 can start once E5-b2 and E5-a1 are in;
c2 → c3 → c4 serial (one package); c5 runs beside c3/c4 (it touches `internal/bench` and
`draftlab/players.go` only).

## 14. Open questions for the operator

1. Package name `draftlab` (top level) instead of `limited/lab` — OK?
2. Fingerprint failure after a corpus pin bump: regenerate the golden with the version
   unchanged (proposed), or always bump the version?
3. Players: a declared version only, no behaviour fingerprint (engine changes move every
   chain head). OK, or fingerprint players on a fixed decision corpus later?
4. `draftsim -draftlog`: keep `draft_id = seed` (proposed, byte-compatible) or switch to
   the per-seat id the lab writes?
5. game_data: omit drawn/stalled games (proposed) or write them with `won` empty?
6. Swiss default for 4 seats: 2 rounds (`ceil(log2 4)`) — or 3 (= round robin)?
7. Should `draftsim` ever get an Env opt-in (it is bot-only too), or does the spec's
   refusal stand for the CLI?

## Operator decisions on the E5 plan's open questions (2026-10-13)
The plan of record is `docs/superpowers/plans/2026-10-14-draft-events-e5-plan.md`.
1. Top-level package `draftlab` (not `limited/lab`) — accepted, keeps `limited/*` pure.
2. A corpus pin bump that moves a Drafter/Builder fingerprint regenerates it with the version unchanged.
3. Players carry a declared version only (no behaviour fingerprint).
4. `draftsim -draftlog` keeps `draft_id = seed`.
5. 17Lands `game_data` omits drawn and stalled games.
6. A 4-seat Swiss defaults to 2 rounds (round robin remains the 4-seat default structure).
7. draftsim never gets the decklist-reading (Env) opt-in; only `draftlab` has it, bot-only, default off.
