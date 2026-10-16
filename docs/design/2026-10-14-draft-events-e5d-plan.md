# E5-d · External agents for draftlab: a JSON-lines wire, batched Drafter / Builder / Player adapters

Base: `main@8ae5062a351078e5ec2725041bbae15e96d60c72` (8ae5062a3; E5-c5 merged). Every
`path:line` below was read at that SHA (`git show 8ae5062a3:<path>`, `git grep -n`), except
the `mzrepro` prior art, read at the branch tip (`git show mzrepro:<path>`). Anything not
read from code is marked **INFERRED** or **Hypothesis**.

Design of record: `docs/design/2026-10-13-draft-events-design.md` §7 (E5-Q1:
gorge drives games, an external model sits behind a seat adapter; E5-Q2: compiled Go, no
RPC service, adapters call OUT), the E5 plan's seam
(`docs/design/2026-10-14-draft-events-e5-plan.md` §9), and the operator's E5-d
planning brief (2026-10-14). §13 lists every place those and the code disagree and the fix
adopted.

**Commit this file** as `docs/design/2026-10-14-draft-events-e5d-plan.md` before
queueing; the briefs cite it as `E5d-plan.md` and restate what they need (seats cannot read
the scratchpad). §4 (the wire) is the protocol of record that the `wire` package doc and the
Python example point at.

---

## 1. What E5-d delivers

- **`gorge-agent/1`**, one versioned JSON-lines protocol carried over either a child
  process's stdin/stdout (`ext:exec:<path>`) or a Unix socket the agent listens on
  (`ext:unix:<path>`). gorge is always the initiator and the only party that drives games;
  the agent only answers (§4).
- **Three request kinds**: `pick` (a draft PickView → one card instance id), `build` (a pool
  → a deck), `decide` (the deciding seat's own redacted `view.View`, decision attached → a
  `decision.Intent`). Plus `hello` (agent name, model version, capabilities), batched
  `batch`/`result` envelopes, `stream_end`, `bye`, and a fatal `error`.
- **Adapters** (`lab/draft/draftlab/extern`): a `pickbot.Drafter`, a `pickbot.Builder` and a seat
  (`draftlab.NamedPlayer`) backed by one dialed agent. One agent process serves every role
  and every concurrent game; its requests are **batched by a barrier** — a batch flushes
  when every live game is waiting on an external agent (no timers; the lab cannot import
  `time`) — with a `ready` mode (inferbroker's rule) selectable for measurement (§6).
- **Refusal and failure behaviour**: an answer the agent refuses or that fails local
  validation is replaced by a deterministic built-in fallback, counted and logged; a dead or
  misbehaving agent fails the game by default (fail closed) or falls back on request (§8).
- **Replay without the model**: draft picks and builds are already logged in the event log;
  game answers are the engine log's accepted intents, which draftlab now keeps on request
  (`Options.RecordTape` → `GameResult.Tape`) and re-drives with no player at all
  (`Lab.ReplayTape`, `Lab.VerifyEventTapes`) (§7).
- **Freezing a generation**: an agent spec may pin `@<model_version>`; the handshake refuses
  a mismatch, and every manifest names the agent as `role:ext:<agent>@<model_version>` (§10).
- **A reference agent** (`lab/draft/draftlab/extern/refagent`, `lab/cmd/extagent`): uniform-random and
  first-legal policies over the protocol, used by every test so CI never needs a model; plus
  a dependency-free Python example (`examples/extagent/random_agent.py`).
- **`lab/cmd/labdemo`** accepts `ext:` specs for its drafter, builder, player and opponents.
- **A benchmark** (games/hour, decisions/game, mean batch) for an external random agent vs
  the built-in `bot`, with targets in §11.

No UI ticket; no edit to `server/protocol/`, `server/host/`, `server/host/httpapi`, `lab/draft/eventhost/`, `web/`,
`cmd/gorged/`, `bots/` or `internal/bench/`.

## 2. Packages and responsibilities

| package | change | ticket |
|---|---|---|
| `lab/draft/draftlab/extern/wire` (new) | message types, constants, validation, line codec with a size cap, view-omit names, `Seed53` | E5d-1 |
| `lab/draft/draftlab/extern/refagent` (new) | `Serve(r, w, Config)`; policies `random`, `first`, `illegal` (tests); observation hooks | E5d-1 |
| `lab/cmd/extagent` (new) | the reference agent binary: stdio or `-listen unix:<path>` | E5d-1 |
| `lab/draft/draftlab/extern` (new) | spec parsing, `Dial` (exec / unix), `DialConn`, handshake, the process-wide batching hub, session failure, `Stats`, `Close` | E5d-2 |
| `draftlab` | `GameToken` + `GameScoped` seam in `playGame`; `FallibleDrafter`/`FallibleBuilder` seams; `Lab.Card`; `Options.RecordTape`, `Tape`, `GameResult.Tape`, `Lab.ReplayTape`, `Lab.VerifyEventTapes` | E5d-3 |
| `lab/draft/draftlab/extern` | `Agent.Drafter/Builder/Player`, the external seat, fallbacks, `Registry` (spec → Named*, builtins pass through), version freeze | E5d-4 |
| `lab/cmd/labdemo` | `-drafter`, `-builder`, `-ext-*` flags; `ext:` specs everywhere a policy is named; stats to stderr | E5d-5 |
| `lab/draft/draftlab/extern` (bench test), `examples/extagent/` (new dir, no Go), `docs/agents/repo-map-*.md` | `BenchmarkGames`; Python example + README; one repo-map row each | E5d-6 |

New Go packages get their `docs/site/architecture/packages.tiers.json` entry **in the
ticket that creates them** (`TestClassificationIsComplete`, `cmd/pkgmap/main_test.go:14`):

```json
"lab/draft/draftlab/extern/wire":     {"tier": "bots", "status": "EXT",  "purpose": "The gorge-agent/1 wire: JSON-lines messages an out-of-process drafter, builder or player speaks, carrying view.View, decision.Intent and protocol.DraftCard."},
"lab/draft/draftlab/extern/refagent": {"tier": "bots", "status": "TOOL", "purpose": "Reference gorge-agent/1 agent (uniform random, first legal) for tests, benchmarks and as a worked example."},
"lab/cmd/extagent":             {"tier": "cmd",  "status": "TOOL"},
"lab/draft/draftlab/extern":          {"tier": "bots", "status": "EXT",  "purpose": "Out-of-process agents for draftlab: dial an agent over a child's stdio or a Unix socket, batch its requests across concurrent games, and expose it as a Drafter, Builder and Player."}
```

No `internal/archtest` allow-list change: none of these packages imports `time`
(`TestTimeIsImportedOnlyByTheHost`, `internal/archtest/arch_test.go:122-152`, checks DIRECT
non-test imports — `p.imports["time"]`, `:148` — so `os/exec` and `net`, which reach `time`
transitively, are fine). `draftlab` itself keeps its ban on `os/exec` and `net`
(`lab/draft/draftlab/imports_test.go:18-19`, a per-directory check), which is why the adapters live in
the subpackage (E5 plan §9).

## 3. Existing mechanisms relied on (verified at 8ae5062a3)

### 3.1 draftlab

- `Options{AllowDecklistBots, Workers, Verify, MaxTurns, MaxIntents, Mulligans}`
  (`lab/draft/draftlab/lab.go:22-40`); `Lab{reg, opts}` immutable after `Open` (`:45-48`).
- Agents: `AgentRef{Name, Version}`, `ParseAgent` cuts on the first `@` and refuses a second
  (`lab/draft/draftlab/agents.go:65-78`); `ErrUnknownAgent`, `ErrVersion`, `ErrDecklistBot`
  (`:91-95`); `NamedDrafter`/`NamedBuilder`/`NamedPlayer{Ref, New func(seed) (seat.Seat,
  error), ReadsDecklists}` (`:99-123`); `resolveVersion` (`:128-143`); the unexported
  card lookup `l.lookup()` (`:146-151`); `Lab.Drafter/Builder/Player` (`:155-241`).
- Games: `GameResult` (`lab/draft/draftlab/play.go:34-52`) — note `Intents int` (`:46`) is already the
  intent COUNT; `playGame` builds seat 0 with `a.Player.New(seed ^ 1)` and seat 1 with
  `b.Player.New(seed ^ 2)` (`:80-87`), runs `envGate` (`:88`, `:161-200`), the hooks
  (`:95-112`: `NewEngine: rules.NewStartingPlayerChoice`, `Setup: AskStartingPlayer`, a
  `Decision` hook counting mulligans) and `bench.PlayGame` (`:113`); `Verify` replays the
  chain with `replay.Replay(eng.L, cfg)` (`:145-149`); `gameConfig` (`:204-213`).
- Every game path funnels through `playGame`: `PlayGame` (`:58-66`), `Games` via
  `runGames` (`lab/draft/draftlab/games.go:15-19,26-51`), `Play` (`lab/draft/draftlab/tournament.go:89`),
  `Gauntlet` (`lab/draft/draftlab/gauntlet.go:67`), `RunEvent` (`lab/draft/draftlab/event.go:250-262`) and
  `VerifyEvent`'s `replayGameHead` (`:380-409`).
- `runGames` starts one goroutine per game and bounds concurrency with a `Workers`-sized
  semaphore acquired (`lab/draft/draftlab/games.go:39`) BEFORE `fn(i)` runs (`:41`): seats are constructed
  only for running games.
- Drafting: `runDraftLoop` is serial over seats and calls `Drafter.Pick(v, rngs[s])` with
  one persistent PCG per seat (`lab/draft/draftlab/draft.go:171-198`, the call at `:185`); an id not
  in the pack is refused by `event.Apply` (`:186-190`). Building: `buildNamed` calls
  `b.Builder.Build(set, pool)` then `build.Validate` (`lab/draft/draftlab/deck.go:56-80`, call at
  `:64`).
- Events log picks (`KindPicked`) and builds (`KindDeckSaved`), but a game only as its
  chain head (`KindGameEnded{Head}`, `lab/draft/draftlab/event.go:271`). `VerifyEvent` re-drives
  every game with caller-supplied players (`:340-373`).
- Manifest `Agents` is sorted unique `role:name@version` (`lab/draft/draftlab/manifest.go:63-105`).

### 3.2 bench drive loop (unchanged by E5-d)

- `bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks)` (`internal/bench/bench.go:179`)
  dispatches each decision, in order: Env (`:313`), search (`:350`), `seat.BoardSeat`
  (`:393-399`), `view.LeanReader` — `view.ProjectLeanInto(lv, …, lr.ViewOmit(d))` into a
  per-seat reused View with `Round` set (`:400-409`), else `view.Project` (`:410-418`). A seat
  error aborts the game with `seed, intent, seat` context (`:419-421`).
- A refused `Submit` runs `reanswerRefused`: `bots.RefusalAnswerer.AnswerRefused` first, then
  `bots.Fallbacks` (`:434-450`, `:492-514`).
- `PaymentPlanConsumer`/`TargetHintConsumer` seats get `EnsurePaymentActions` /
  `EnsureTargetHints` before deciding (`:277-282`). Both are pure derived reads that emit no
  event and touch no RNG (`engine/rules/engine_host.go:53-55`, `engine/rules/target_hints.go:10-18`).

### 3.3 seats, bots, pickbot

- `seat.Seat{Decide(ctx, v, d)}`, `PaymentPlanConsumer`, `TargetHintConsumer`, `BoardSeat`
  (`engine/seat/seat.go:19-45`); `seat.BoardFromView(v)` (`bots/heuristic/bot.go:475`).
- `bots.RefusalAnswerer` (`bots/refusal.go:15-17`); `bots.Fallbacks(d, brd, seatIdx)` returns
  `[minimal, botpolicy.Decide over PCG(d.Seq, seat+1)]`, deterministic (`:27-42`).
- The bots registry is closed and static: names must match `^[a-z0-9-]+$`
  (`bots/registry.go:103-113`); `bots.EnvSeat` (`bots/env.go:16-31`).
- `pickbot.Drafter{Pick(v PickView, r *rand.Rand) string}` and
  `pickbot.Builder{Build(set, pool) build.Deck}` have no error path
  (`lab/draft/limited/pickbot/registry.go:93-100`); `rating-colour` and `best23` are registered at
  `:112-119`. `PickView{Set, Round, PickNo, Pack, Picks, Seen}` (`lab/draft/limited/draft/draft.go:45-60`).
- `build.Deck{Main []string json:"main,omitempty"; Basics [5]int json:"basics"}`
  (`lab/draft/limited/build/build.go:35-45`); `build.Validate` (`:69-99`): ≥ 40 and ≤ 100 cards, basics
  0..40 each, every main id in the pool, no duplicate.
- `limited.CardRef{ID, Name, Set, Number, Rarity}` (`lab/draft/limited/limited.go:45-56`).

### 3.4 the wire types reused

- `view.View` is "one seat's complete picture": `Decision *decision.Decision
  json:"decision,omitempty"` (`engine/view/view.go:95-136`). `view.Project` attaches a decision only
  to the player it was asked of, as a copy (`:400-407`, `:793-799`). It is exactly what a
  human seat reads from `GET /api/tables/{t}/matches/{k}/view?seat=` (`server/host/httpapi/rest.go:188-232`,
  route `server/host/httpapi/handler.go:99`); a human answers with a `decision.Intent` body
  (`rest.go:305-324`).
- `view.Omit` bits (`engine/view/lean.go:14-43`: Library, Available, Archetype, AbilityCosts,
  EffectiveCost, Decision, OwnDeck, Potential, CardLists, DerivedChars), `view.LeanReader`
  (`:51-53`), `ProjectLeanInto` (`:57-65`).
- `decision.Decision` (`engine/decision/decision.go:617-…`), `decision.Option{Index, Kind, …}`
  (`:175-…`), `decision.Kinds` (`:161-164`), `decision.Intent{Seq, Player, Choices, Rest,
  Payment, Announce}` (`:1242-1268`), `Decision.Validate` (`:1292-…`).
- `protocol.DraftCard{ID, Name, Set, Number, Rarity, ManaCost, ManaValue, Colors, TypeLine}`
  (`server/protocol/draftevent.go:17-27`) and `protocol.DeckRequest{Main, Basics}` (`:215-218`, the
  `build.Deck` JSON shape). eventhost fills a DraftCard's facts from the corpus
  (`lab/draft/eventhost/cards.go:58-96`); extern copies that logic (it cannot import eventhost).
- `protocol.Version` doc: "bumps only on a breaking change … additive fields do not bump it"
  (`server/protocol/protocol.go:15-17`) — the same rule `gorge-agent/1` adopts.

### 3.5 logs and replay

- `events.Log{… Intents []decision.Intent json:"intents"}` (`engine/events/log.go:17-20`); an intent
  is logged only after `Submit` validated it (`engine/rules/engine.go:189-217`, kernel append
  `engine/rules/resolve/kernel.go:170-176`), so the log holds accepted intents only.
- `replay.Replay(l, cfg)` re-submits a log's intents and compares events (`engine/replay/replay.go:105-160`).

### 3.6 prior art

- `lab/training/inferbroker` (tag `broker`): batches learner decisions from many games "with no
  all-games barrier" — it serves whatever is ready (`lab/training/inferbroker/broker.go:1-27`,
  `README.md`). `lab/training/gamepool` (tag `gamepool`) flushes at `MaxBatch` or `FlushEvery`
  (`lab/training/gamepool/pool.go:1-30`). Both read the clock and are on the archtest allow-list
  (`arch_test.go:112-115,142-143`).
- `lab/cmd/dzgorge/remote.go:22-33`: a binary socket client batching across games with a 300 µs
  timer (`remoteWait`, `:31`).
- `mzrepro:internal/mzbridge/mzclient/client.go:20-60`: MessagePack over HTTP, micro-batched
  at `MaxBatch` 4 or a 2.5 ms flush, `MaxInFlight` 16; hand-rolled msgpack
  (`mzrepro:internal/mzbridge/msgpack.go`).
- The MageZero checklist names a batched bridge and the action mapping as missing
  (`archived 2026-10-02-magezero-style-training-checklist.md (docs/design/ARCHIVE.md)`, "Missing" items
  1–2). E5-d delivers the bridge at the ACTION level (the agent answers gorge's own
  positional options); a vocabulary mapping stays the agent's job (§15 Q8).

### 3.7 measured costs (this planning session, main@8ae5062a3)

- `draftsim -set FDN -seats 8 -structure roundrobin -players bot -events 1 -seed 1`: 28
  bot-vs-bot games plus the corpus load and an 8-seat draft in **1.40 s wall on one core**
  (`GOMAXPROCS=1`, `CPUQuota=100%`, 357 MB RSS) — under 50 ms per limited game, so at least
  ~72k games/hour/core.
- A scratch probe (outside the repo, not committed: `rules.New` + two `seat.Bot` with
  autopay, 20 games each of two FDN starter-deck pairings from `internal/testutil/decks/`,
  timing `view.Project` and `json.Marshal` at seat 0's decisions): **209–241 decisions per
  game for one seat; a Seat view marshals to 34–36 KB on average (max ~44 KB);
  `view.Project` 72–138 µs and `json.Marshal` 95–139 µs per decision.** So a full-view
  external seat costs gorge ~40–65 ms per game in projection + encoding alone (about one
  whole bot game) and moves ~8 MB of JSON per game. This is why `view_omit` (lean views) is
  in v1 (§4.3).

## 4. The wire: `gorge-agent/1`

### 4.1 Framing and evolution

- One JSON object per line, UTF-8, `\n`-terminated, at most `wire.MaxLine = 64 MiB`. A
  longer line is a protocol violation (the reader refuses it without buffering it whole).
- Every message carries `"v": 1` and a `"type"`. Unknown FIELDS are ignored by both sides
  (additive evolution never bumps `v`); an unknown `type`, a wrong `v` or malformed JSON is a
  protocol violation that ends the session.
- gorge initiates every exchange and is the only writer of `hello`, `batch`, `stream_end`
  and `bye`; the agent writes `hello`, `result` and `error`.
- Seeds are integers below 2^53 (`wire.Seed53(x) = x >> 11`) so every JSON implementation
  reads them exactly. Item ids and batch numbers are small counters.
- Same framing on both transports. Over stdio the agent's stderr is free-form logging
  (gorge copies it to `Options.Stderr`).

### 4.2 Handshake

gorge → agent, first line:

```json
{"v":1,"type":"hello","protocol":"gorge-agent/1","gorge_revision":"8ae5062a351078e5ec2725041bbae15e96d60c72","kinds":["pick","build","decide"]}
```

agent → gorge, its first line:

```json
{"v":1,"type":"hello","protocol":"gorge-agent/1","agent":"random","model_version":"1","kinds":["pick","build","decide"],"max_batch":256,"skip_forced":false,"payment_actions":false,"target_hints":false,"view_omit":["library","archetype"]}
```

| field | rule |
|---|---|
| `protocol` | must equal `gorge-agent/1` |
| `agent` | `^[a-z0-9][a-z0-9._-]{0,63}$`; the recorded agent name is `ext:<agent>` |
| `model_version` | `^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$` (no `@`); identifies the weights — §10 |
| `kinds` | non-empty subset of `pick`, `build`, `decide`; the roles the agent serves |
| `max_batch` | 0 = no preference, else the most items it wants per batch |
| `skip_forced` | true: gorge answers a forced decision itself (§8.4) and never sends it |
| `payment_actions` | true: decisions carry `payment_actions` and the agent may answer with `intent.payment` (the seat implements `seat.PaymentPlanConsumer`); false: casts pay through the ordinary posed decisions |
| `target_hints` | true: priority options carry `target_hints` (`seat.TargetHintConsumer`) |
| `view_omit` | names from: `library`, `available`, `archetype`, `ability_costs`, `effective_cost`, `own_deck`, `potential`, `card_lists`, `derived_chars` (the `view.Omit` bits except `Decision`, `engine/view/lean.go:18-42`); an unknown name or `decision` fails the handshake |

gorge `gorge_revision` is `runtime/debug` `vcs.revision` (+`+dirty`), else the module
version, else `devel` — informational. A handshake failure closes the session with
`ErrHandshake` naming the bad field.

### 4.3 Requests: `batch`

```json
{"v":1,"type":"batch","batch":17,"items":[ITEM, ...]}
```

`batch` increases from 1 per session; ids are unique per session. One batch is in flight
per session at a time (v1). Items, by `kind`:

**pick** — one seat's draft decision (a PickView; never another seat's picks):

```json
{"id":41,"kind":"pick","seed":123456789,"set":"FDN","round":1,"pick_no":3,
 "pack":[DRAFTCARD,...],"picks":[DRAFTCARD,...],"seen":[[DRAFTCARD,...],...]}
```

**build** — one seat's pool:

```json
{"id":42,"kind":"build","seed":987654321,"set":"FDN","deck_size":40,"pool":[DRAFTCARD,...]}
```

**decide** — one decision of one seat in one game:

```json
{"id":43,"kind":"decide","seed":555,"stream":"g12.1","view":VIEW}
```

- `DRAFTCARD` is `protocol.DraftCard` with its facts filled from the lab's corpus exactly as
  `lab/draft/eventhost/cards.go:58-96` fills them (`mana_cost`, `mana_value`, `colors` in WUBRG order,
  `type_line`); a card the corpus does not know carries only its ref fields.
- `VIEW` is the `view.View` JSON of the deciding seat at Seat visibility — the bytes bench's
  projection produced, lean when the agent declared `view_omit` — with `view.decision` the
  decision to answer (always present). `view.viewer` is the seat.
- `stream` is opaque: one id per (game, seat), stable for the game, `"g<n>.<seat>"`
  (`n` is a per-process counter assigned when the game begins, so its numbering depends on
  scheduling and must never change an answer) or `"u<n>.<seat>"` for a seat used outside a
  draftlab game.
- Seeds: pick = `Seed53(r.Uint64())` drawn from the seat's pick rng (E5 plan §9); build =
  `Seed53(FNV-1a 64 of set + "\x00" + every pool id joined by "\x00")`; decide =
  `Seed53(rand.New(rand.NewPCG(seatSeed ^ 0xE5D1<<48, d.Seq)).Uint64())`, `seatSeed` being the
  seed `NamedPlayer.New` received (game seed ^ (seat+1), `lab/draft/draftlab/play.go:80,84`).

### 4.4 Answers: `result`

```json
{"v":1,"type":"result","batch":17,"answers":[ANSWER, ...]}
```

Every item id of batch 17 exactly once, any order. One answer carries exactly one of:

| item kind | answer field | shape |
|---|---|---|
| pick | `"pick"` | a card instance id from `pack` |
| build | `"deck"` | `protocol.DeckRequest` = `{"main":[ids from pool],"basics":[W,U,B,R,G]}` |
| decide | `"intent"` | `decision.Intent` JSON: `choices` (option `index` values), optional `rest`, `payment`, `announce`; `seq`/`player` may be omitted — gorge sets them from the decision |
| any | `"error"` | `{"code":"refused","message":"..."}`: the agent declines this one item |

A wrong batch number, a missing / duplicate / unknown id, an answer field that does not fit
the item's kind, or more than one field is a protocol violation (session ends). An answer
that is well-formed but illegal (a pick not in the pack, a deck `build.Validate` refuses, an
intent `Decision.Validate` refuses) is a **refusal** (§8), not a violation.

### 4.5 Other messages

- `{"v":1,"type":"stream_end","stream":"g12.1"}` (gorge → agent, no reply): that seat's game
  ended; per-stream state may be dropped.
- `{"v":1,"type":"bye"}` (gorge → agent, no reply): the session ends; the agent should exit
  (stdio) or close the connection (unix).
- `{"v":1,"type":"error","code":"bad_request"|"unsupported"|"internal","message":"..."}`
  (agent → gorge): fatal; every pending request fails with `ErrAgentGone`.

## 5. Go API of record

### 5.1 `lab/draft/draftlab/extern/wire` (E5d-1)

```go
package wire // imports encoding/json, bufio, io, errors, fmt, regexp, decision, view, protocol

const (
    Protocol = "gorge-agent/1"
    Version  = 1
    MaxLine  = 64 << 20

    THello = "hello"; TBatch = "batch"; TResult = "result"; TStreamEnd = "stream_end"
    TBye = "bye"; TError = "error"

    KindPick = "pick"; KindBuild = "build"; KindDecide = "decide"

    CodeRefused = "refused"; CodeBadRequest = "bad_request"
    CodeUnsupported = "unsupported"; CodeInternal = "internal"
)

// Message is the one flat envelope; every field omitempty except V and Type.
type Message struct {
    V    int    `json:"v"`
    Type string `json:"type"`
    // hello (both directions)
    Protocol       string   `json:"protocol,omitempty"`
    GorgeRevision  string   `json:"gorge_revision,omitempty"`
    Agent          string   `json:"agent,omitempty"`
    ModelVersion   string   `json:"model_version,omitempty"`
    Kinds          []string `json:"kinds,omitempty"`
    MaxBatch       int      `json:"max_batch,omitempty"`
    SkipForced     bool     `json:"skip_forced,omitempty"`
    PaymentActions bool     `json:"payment_actions,omitempty"`
    TargetHints    bool     `json:"target_hints,omitempty"`
    ViewOmit       []string `json:"view_omit,omitempty"`
    // batch / result
    Batch   uint64   `json:"batch,omitempty"`
    Items   []Item   `json:"items,omitempty"`
    Answers []Answer `json:"answers,omitempty"`
    // stream_end
    Stream string `json:"stream,omitempty"`
    // error
    Code    string `json:"code,omitempty"`
    Message string `json:"message,omitempty"`
}

type Item struct {
    ID       uint64                  `json:"id"`
    Kind     string                  `json:"kind"`
    Seed     uint64                  `json:"seed"`
    Set      string                  `json:"set,omitempty"`      // pick, build
    Round    int                     `json:"round,omitempty"`    // pick
    PickNo   int                     `json:"pick_no,omitempty"`  // pick
    Pack     []protocol.DraftCard    `json:"pack,omitempty"`     // pick
    Picks    []protocol.DraftCard    `json:"picks,omitempty"`    // pick
    Seen     [][]protocol.DraftCard  `json:"seen,omitempty"`     // pick
    Pool     []protocol.DraftCard    `json:"pool,omitempty"`     // build
    DeckSize int                     `json:"deck_size,omitempty"`// build
    Stream   string                  `json:"stream,omitempty"`   // decide
    View     json.RawMessage         `json:"view,omitempty"`     // decide: view.View JSON
}

type Answer struct {
    ID     uint64                `json:"id"`
    Pick   string                `json:"pick,omitempty"`
    Deck   *protocol.DeckRequest `json:"deck,omitempty"`
    Intent *decision.Intent      `json:"intent,omitempty"`
    Error  *ItemError            `json:"error,omitempty"`
}
type ItemError struct{ Code, Message string } // json "code", "message"

// Direction says who sent a message; Validate checks the shape for that sender
// (field rules of §4: types, kinds, regexes, exactly-one answer field, …).
type Direction int
const ( FromGorge Direction = iota; FromAgent )
func (m Message) Validate(dir Direction) error

var (
    ErrProtocol    = errors.New("wire: protocol violation")
    ErrLineTooLong = errors.New("wire: line exceeds MaxLine")
)

type Reader struct{ /* bufio.Reader, cap */ }
func NewReader(r io.Reader, maxLine int) *Reader // maxLine <= 0 = MaxLine
func (r *Reader) Read() (Message, error)          // io.EOF at a clean end
type Writer struct{ /* io.Writer */ }
func NewWriter(w io.Writer) *Writer
func (w *Writer) Write(m Message) error // json.Marshal + '\n' in ONE Write call; not goroutine-safe

// ViewOmitNames is the fixed, ordered name table of §4.2.
var ViewOmitNames = []struct{ Name string; Bit view.Omit }{ /* library … derived_chars */ }
func ParseViewOmit(names []string) (view.Omit, error)

func Seed53(x uint64) uint64 { return x >> 11 }

// DecisionOf decodes just view.decision from a decide item's raw view (agents that
// never need the rest), and DecodeView the whole view.View.
func DecisionOf(raw json.RawMessage) (*decision.Decision, error)
func DecodeView(raw json.RawMessage) (view.View, error)
```

### 5.2 `lab/draft/draftlab/extern/refagent` and `lab/cmd/extagent` (E5d-1)

```go
package refagent
type Config struct {
    Agent          string   // hello agent; "" = Policy
    ModelVersion   string   // "" = "1"
    Policy         string   // "random" | "first" | "illegal"
    Kinds          []string // nil = all three
    MaxBatch       int
    SkipForced, PaymentActions, TargetHints bool
    ViewOmit       []string
    DieAfter       int                  // > 0: end the session (error) after this many batches — tests
    ObserveItem    func(it wire.Item)   // tests: every request item, on the serving goroutine
    ObserveBatch   func(n int)          // tests: every batch's item count
}
func Serve(r io.Reader, w io.Writer, c Config) error // hello, then batches until bye/EOF; nil on a clean end
func Answer(c Config, it wire.Item) wire.Answer      // the policy alone: a pure function of (c.Policy, it)
```

Policies (all seeded `rand.New(rand.NewPCG(it.Seed, 0))`):
- `random`: pick a uniform pack card; build: the two most frequent colours among the pool's
  non-land cards (WUBRG order breaks ties), up to 23 non-land pool cards whose colours lie in
  that pair (shuffled; topped up with other non-land cards), the rest basics split between
  the pair (larger share to the first), always `build.Validate`-legal; decide: drop options
  of kind `concede`, choose k uniform in `[Min, min(Max, len)]` and k distinct options in
  random order (`choices` = their `index`).
- `first`: the first pack card; the same build; decide: the `pass` option at priority, else
  the first `Min` options in offered order.
- `illegal` (tests): decide answers `choices: [max index + 1]` (always refused); picks an id
  not in the pack; builds an empty deck.

`lab/cmd/extagent [-policy random] [-agent NAME] [-model-version V] [-listen unix:<path>]
[-skip-forced] [-payment-actions] [-target-hints] [-view-omit all|none|a,b,…]` (default
`all`: the reference policies read only the decision, and a spec cannot pass flags): stdio by default;
with `-listen` it serves each accepted connection on its own goroutine and refuses an
existing socket path.

### 5.3 `lab/draft/draftlab/extern` session and hub (E5d-2)

```go
package extern // imports context, io, os, os/exec, net, sync, errors, fmt, strings, runtime/debug, wire, draftlab

type FlushMode string
const (
    FlushBarrier FlushMode = "barrier" // default (E5 plan §9): flush when every live game waits on an agent
    FlushReady   FlushMode = "ready"   // flush whatever is queued whenever the session is idle (inferbroker's rule)
)
type Failure string
const (
    FailError    Failure = "error"    // default: a dead/misbehaving agent fails the game, draft or build
    FailFallback Failure = "fallback" // answer with the built-in fallback, counted and logged
)
type Options struct {
    Flush          FlushMode         // "" = FlushBarrier
    MaxBatch       int               // 0 = the hello's max_batch if > 0, else 256; capped at 1024
    OnFailure      Failure           // "" = FailError
    StrictRefusals bool              // a refused/illegal answer is an error (ErrRefused) instead of a fallback
    Log            func(line string) // one line per refusal, fallback and session failure; nil = silent
    Stderr         io.Writer         // exec: the child's stderr; nil = os.Stderr
}

type Endpoint struct {
    Transport string // "exec" | "unix"
    Target    string // exec: program path (exec.LookPath when it has no '/'); unix: socket path
}
// ParseSpec reads "ext:exec:<path>[@<model_version>]" or "ext:unix:<path>[@<model_version>]";
// the version half follows draftlab.ParseAgent (one '@', non-empty).
func ParseSpec(spec string) (Endpoint, string, error)
func IsSpec(s string) bool // strings.HasPrefix(s, "ext:")

var (
    ErrSpec      = errors.New("extern: bad agent spec")
    ErrHandshake = errors.New("extern: handshake failed")
    ErrProtocol  = errors.New("extern: protocol violation")
    ErrAgentGone = errors.New("extern: agent is gone")
    ErrRefused   = errors.New("extern: agent answer refused")
    ErrRole      = errors.New("extern: agent does not serve this role")
)

type Agent struct{ /* session, hub *hub, hello, stats, dead */ }
func Dial(ctx context.Context, ep Endpoint, o Options) (*Agent, error) // exec.CommandContext / net.Dial("unix"); ctx bounds the session
func DialConn(ctx context.Context, conn io.ReadWriteCloser, o Options) (*Agent, error) // tests, embedding
func (a *Agent) Hello() wire.Message
func (a *Agent) Ref() draftlab.AgentRef // {Name: "ext:" + hello.Agent, Version: hello.ModelVersion}
func (a *Agent) Stats() Stats
func (a *Agent) Close() error // bye, close stdin/conn, wait for the child

type Stats struct {
    Picks, Builds, Decides int64 // items sent
    Batches                int64
    MaxBatch               int
    Forced, Refused, Fallbacks int64
    Dead                   bool
}

// Package-internal, used by E5d-4 (same package):
func (a *Agent) call(ctx context.Context, game any, it wire.Item) (wire.Answer, error) // game nil = unscoped
func (a *Agent) beginGame(game any) uint64 // refcounted on the process hub; returns the game's process-wide number (assigned at its first begin)
func (a *Agent) endGame(game any)
func (a *Agent) sendStreamEnd(stream string)
```

### 5.4 draftlab seams (E5d-3)

```go
// scoped.go (new)
// GameToken identifies one game for GameScoped seats; only its address means anything.
// It is not zero-sized, so two tokens never share an address.
type GameToken struct{ _ byte }

// GameScoped is an optional seat interface (lab/draft/draftlab/extern's batched seats). playGame
// calls BeginGame on every constructed seat implementing it — both seats of one game get
// the same token — after the Env gate and before the engine is built, and EndGame with
// that token after bench.PlayGame returns, on every path (deferred).
type GameScoped interface {
    BeginGame(t *GameToken)
    EndGame(t *GameToken)
}

// fallible.go (new)
// FallibleDrafter / FallibleBuilder are optional: the draft loop and buildNamed prefer
// the Err form and fail the draft / build with its error (wrapped with seat, round, pick).
type FallibleDrafter interface {
    pickbot.Drafter
    PickErr(v pickbot.PickView, r *rand.Rand) (string, error)
}
type FallibleBuilder interface {
    pickbot.Builder
    BuildErr(set string, pool []limited.CardRef) (build.Deck, error)
}

// Card resolves a card name against the lab's read-only registry.
func (l *Lab) Card(name string) (*cards.Card, bool)

// tape.go (new)
// Options gains:  RecordTape bool — keep each game's accepted intents (GameResult.Tape).
// GameResult gains:  Tape *Tape — nil unless Options.RecordTape.
type Tape struct {
    Names     [2]string         // the engine config's player names (deck names, or "p<seat>")
    Mulligans int               // the config's London round
    Intents   []decision.Intent // eng.L.Intents, copied
}
// ReplayTape re-drives one game from its tape alone — no player policy, no agent — with the
// same hooks playGame uses, and returns the chain head. A tape that does not fit (seq/player
// mismatch, exhausted early, left over) or a lab whose mulligan round differs is an error.
func (l *Lab) ReplayTape(a, b Deck, seed uint64, t *Tape) (string, error)
// VerifyEventTapes is VerifyEvent without players: the log replays byte-identically and every
// GameEnded head re-derives from r.Games' tapes over the event's own saved decks.
func (l *Lab) VerifyEventTapes(r EventResult) error
```

### 5.5 `lab/draft/draftlab/extern` adapters (E5d-4)

```go
func (a *Agent) Drafter(l *draftlab.Lab) (draftlab.NamedDrafter, error) // ErrRole without "pick"; fallback rating-colour
func (a *Agent) Builder(l *draftlab.Lab) (draftlab.NamedBuilder, error) // ErrRole without "build"; fallback best23
func (a *Agent) Player(l *draftlab.Lab) (draftlab.NamedPlayer, error)   // ErrRole without "decide"

// Registry resolves role specs for one lab: "ext:" specs dial once per endpoint and share
// the Agent across roles; anything else is draftlab.ParseAgent + the lab's built-in resolver.
type Registry struct{ /* ctx, lab, opts, agents in dial order */ }
func NewRegistry(ctx context.Context, l *draftlab.Lab, o Options) *Registry
func (r *Registry) Drafter(spec string) (draftlab.NamedDrafter, error)
func (r *Registry) Builder(spec string) (draftlab.NamedBuilder, error)
func (r *Registry) Player(spec string) (draftlab.NamedPlayer, error)
func (r *Registry) Agents() []*Agent // dial order
func (r *Registry) Close() error      // every agent, dial order; the first error
```

The external seat implements `seat.Seat`, `view.LeanReader` (`ViewOmit` = the hello's mask),
`seat.PaymentPlanConsumer`, `seat.TargetHintConsumer` (from the hello), `bots.RefusalAnswerer`
and `draftlab.GameScoped` — and **never** `seat.BoardSeat` (a BoardSeat receives no View, so
it would have nothing honest to send) or `bots.EnvSeat` (draftlab's `envGateResult` would
treat it as a decklist reader, `lab/draft/draftlab/play.go:181-200`).

## 6. Batching

### 6.1 The barrier (default, decided)

One **process-wide hub** coordinates every `Agent` in the process. It counts:

- `live`: games that have begun (`beginGame`, refcounted per token — both seats of a game
  on one or two agents count once) plus in-flight **unscoped** requests (a draft pick, a
  build, or a seat used outside a draftlab game), each counting as its own one-request
  "game";
- `waiting`: live games with at least one request queued or in flight on ANY agent.

A session (one per Agent) with a non-empty queue and no batch in flight flushes up to
`MaxBatch` items (FIFO) when **`waiting == live`** (every live game is blocked on an
external agent) **or** its queue holds `MaxBatch` items. Re-evaluated on every enqueue,
every result and every `endGame`. With one batch in flight per session, a session whose
batch returns re-flushes as soon as the condition holds again.

Why process-wide: with one hub per agent, a game whose seats sit on agents A and B is "live
but not waiting" for A while it waits on B, and B's barrier can wait on a game waiting on A
— a deadlock. Counting "waiting on ANY agent" removes it (E5d-2 pins it with a two-agent test).

Why `GameScoped`: a `seat.Seat` never learns that its game ended (`engine/seat/seat.go:19-21`), and
`NamedPlayer.New` is per game, so without `EndGame` a finished game would stay live forever
and the barrier would never fire again. `playGame` is the single place every lab game passes
(§3.1), so the seam is one call after `bench.PlayGame` (plus a guarded `defer`) there.

Liveness argument: between `BeginGame` and `EndGame` a lab game is either computing (engine
or a built-in seat — finite) or blocked on the hub; `runGames` constructs seats only for
running games (the semaphore precedes `fn(i)`, `lab/draft/draftlab/games.go:39-41`). So when nothing
can make progress every live game is waiting, the barrier fires, and some batch is in flight.

Properties: Workers = 1 gives batches of 1 (correct, slow); the batch size approaches the
number of games blocked on the model, so **an external player wants `Options.Workers` set to
the batch size wanted (e.g. 64), not to the core count**. Games advance in lock-step rounds; a
game whose other seat is a slow built-in (a search bot) delays every flush (§12 R3).

### 6.2 Ready mode (selectable)

`FlushReady`: a session flushes its whole queue (up to `MaxBatch`) whenever it is idle —
inferbroker's "serve the ready set, no all-lane barrier". The first request after idle goes
alone; under load batches grow to the number of games that blocked while the previous batch
was served. No membership needed. E5d-6 measures both; §15 Q1 asks which is the default.

### 6.3 What batching may and may not change

Batch composition and order depend on scheduling. **No answer may depend on them**: the
agent contract is that an answer is a pure function of its item (content + seed) and the
model version — `stream` ids are scheduling-dependent too. Under that contract every lab
result is independent of Workers, flush mode and MaxBatch, as draftlab promises
(`lab/draft/draftlab/doc.go`); without it (an agent with batch-size-dependent numerics, per-stream
memory that reads other streams), results are still exactly recorded and replayable (§7) but
a re-run may differ. The refagent policies satisfy the contract; E5d-4 tests Workers 1 vs 4.

## 7. Determinism and replay without the model

| artifact | where the agent's answers live | replays without the model by |
|---|---|---|
| draft | event log `Picked` entries | `event.ReplayLog` (unchanged) |
| build | event log `DeckSaved` entries | `event.ReplayLog` |
| game | engine log intents → `GameResult.Tape` (opt-in `Options.RecordTape`) | `Lab.ReplayTape`, `Lab.VerifyEventTapes` |

- A tape holds the ACCEPTED intents only (§3.5), so a game whose agent was refused and fell
  back replays the fallback answer; refusals leave no trace in the engine (a refused Submit
  leaves the pending decision intact, `internal/bench/bench.go:436-441`).
- Tape replay drives BOTH seats with tape seats through the same `playGame` hooks (starting
  player ask, mulligan counting). Payment-action and target-hint offers are pure derived
  reads (§3.2), so a tape seat need not mirror the original seat's consumer flags. A game
  that ran under the bench Env driver replays under plain hooks; the Env branch never writes
  the live engine (`internal/bench/bench.go:313-349`) — **INFERRED**, pinned by an E5d-3 test
  with `sb-tactical`.
- A stalled game's tape replays to the same watchdog and head; tape verification compares
  heads for stalls too (unlike `VerifyEvent`, which refuses a stalled re-drive,
  `lab/draft/draftlab/event.go:405-407`).
- The tape seat's decision sequence equals the log's intent sequence — **INFERRED** from the
  kernel stepping over recorded intents internally (`engine/rules/resolve/kernel.go:170-176`);
  E5d-3's replay tests over whole games pin it.
- `VerifyEvent(r, players)` stays as is: with external players it re-asks the agent (valid
  only for a per-request-pure agent). `VerifyEventTapes` is the model-free proof.
- No `time` anywhere in E5-d's packages; no map range reaches a request, a result or a log
  line (the hub's game map is keyed by token and never ranged for output; sessions flush in
  registration order); every stream is `math/rand/v2` PCG.

## 8. Refusal, failure and "timeouts"

### 8.1 Refusals (per item; always recoverable unless `StrictRefusals`)

A refusal is: an answer `{"error":{"code":"refused"}}`, a pick not in the pack, a deck
`build.Validate` refuses, or an intent `Decision.Validate` refuses (checked on the decision
the seat was handed, `engine/decision/decision.go:1292`). The adapter counts it (`Stats.Refused`),
logs one line through `Options.Log`, and answers with:

| role | fallback |
|---|---|
| pick | the lab's `rating-colour@1` drafter on the same PickView and rng |
| build | the lab's `best23@1` builder |
| decide | `bots.Fallbacks(&d, seat.BoardFromView(v), seat)[1]` (botpolicy.Decide, deterministic) |

An intent that passes `Decision.Validate` but the engine still refuses reaches bench's
ladder, which calls the seat's `AnswerRefused` (counted, answered with the same fallback),
then `bots.Fallbacks` (`internal/bench/bench.go:492-514`). With `StrictRefusals` every
refusal is an error wrapping `ErrRefused` (for debugging a model).

### 8.2 Session failures (`OnFailure`)

The child exits, the socket closes, the agent sends `error`, a protocol violation, or the
Dial context is cancelled: the session is dead, every pending request fails with
`ErrAgentGone`, and later requests fail at once. `FailError` (default) returns the error —
the game errors (`bench` wraps it with seed and intent), and so does the draft or build
(through the `Fallible*` seams). `FailFallback` answers every later request with §8.1's
fallbacks (`Stats.Fallbacks`) and logs once per request. Fail-closed is the default because
a training run that silently switched policy mid-run would mislabel its data.

### 8.3 Timeouts

gorge reads no clock here (archtest allow-list closed; `draftlab` and its subpackages may
not import `time`). The only deadline is the caller's: `Dial(ctx, …)` bounds the session (an
exec child is started with `exec.CommandContext`, so cancelling kills it and fails every
pending request); a training program that wants a per-run timeout builds `ctx` with its own
`time` import. Inside gorge a hung agent hangs its games — documented, and §15 Q2 asks
whether `lab/draft/draftlab/extern` should get an allow-list line (operator hand edit) for a
per-request deadline.

### 8.4 Forced decisions

With `skip_forced`, the seat answers locally — never sending it — any decision with no
`payment_actions`, no `mana_payment`, exactly one option and `Min == Max == 1`: `choices =
[options[0].index]` (`Stats.Forced`). Deterministic; the tape records it like any answer.

## 9. Hidden information (server-authoritative rule)

- `decide` sends the deciding seat's own Seat-visibility projection — the same JSON a human
  seat reads (§3.4) — and nothing else: never the engine, the botpolicy Board, a feed, a
  redeal or another seat's view. A lean view only REMOVES parts (`engine/view/lean.go:9-13`).
- `pick` sends one seat's PickView (its pack, its own picks, packs it saw); `build` one
  seat's pool.
- **One process, several seats.** An agent that plays both seats of a game, or several seats
  of a pod, receives each seat's redacted view on one connection and could combine them —
  just as one program running two built-in bots could. Seat isolation therefore means one
  endpoint (process) per seat; the protocol never joins two seats' streams for the agent.
  §15 Q6 asks whether the Registry should offer per-seat isolation.
- E5-d adds no HTTP, SSE or `server/protocol/` surface; the agent is never human (a human seat is
  eventhost's, E5-e).

## 10. draftlab integration and freezing a generation

- Specs: `ext:exec:<path>[@<model_version>]` and `ext:unix:<path>[@<model_version>]`, parsed
  by `extern.ParseSpec`; `extern.Registry` resolves `ext:` specs and passes every other spec
  to the lab's built-ins, so a CLI flag takes either. `ext:` names never enter the `bots`
  registry (its names cannot contain `:`, `bots/registry.go:103-107`).
- The resolved ref is `ext:<hello.agent>@<hello.model_version>`; the path or socket is a run
  knob like Workers and never reaches a result, a persona or a manifest.
- **Freeze**: `@<v>` in a spec must equal the hello's `model_version`, else `ErrVersion`
  (draftlab's, wrapped) — the same rule as built-ins (`lab/draft/draftlab/agents.go:128-143`). The
  training system serves a frozen generation behind a stable endpoint and the run refuses
  if it is not that generation. The manifest's `Agents` then reads e.g.
  `player:ext:mz@gen7`, `drafter:ext:mz@gen7`.
- `model_version` must change whenever the weights change; an agent that changes weights
  mid-session breaks §6.3's contract (a reconnect = a new hello = a new version).
- Workers: one Agent serves every concurrent game; set `Options.Workers` to the wanted
  batch size.

## 11. Performance target and measurement

Baseline (§3.7): bot-vs-bot limited games run at ≥ ~72k games/hour/core; a full Seat view
costs ~0.17–0.28 ms to project and encode per decision, ~230 decisions per game per seat.

Targets (provisional, **INFERRED** from §3.7; E5d-6 measures and reports, they are not a
merge gate):

| configuration (Workers 4, 4 cores, Go refagent `random` over stdio, barrier) | target |
|---|---|
| lean view (`view_omit`: every name) | ≥ 50% of bot-vs-bot games/hour |
| full view | ≥ 25% of bot-vs-bot games/hour |
| mean batch size | ≥ 3 (0.75 × Workers) |

Measured by `BenchmarkGames` in `lab/draft/draftlab/extern` (E5d-6): sub-benchmarks `builtin-bot`,
`ext-pipe-lean` (in-process `DialConn` over `io.Pipe`: protocol cost without a process),
`ext-stdio-lean`, `ext-stdio-full`, `ext-unix-lean`, `ext-stdio-lean-ready`,
`ext-stdio-lean-w16`; each plays `b.N` games of two `fdn-limited` catalog decks through
`Lab.Games` and reports `games/h` (= 3.6e12 / ns per game), `decisions/game` and
`batch/mean` with `b.ReportMetric`:

```
systemd-run --user --scope -q -p MemoryMax=4G -p CPUQuota=400% env GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -timeout 2m -run '^$' -bench 'BenchmarkGames' -benchtime 40x ./lab/draft/draftlab/extern
```

An end-to-end number for the report: `/usr/bin/time -v go run ./lab/cmd/labdemo -seed 1 -player
ext:exec:<built extagent> -workers 4` against the same command with `-player bot@1`.

## 12. Risks

1. **JSON volume** (~35 KB per full view, §3.7). Mitigation: `view_omit` in v1; a binary
   framing is out of scope (no third-party deps; a hand-rolled codec is a later ticket if the
   benchmark says so).
2. **Barrier lock-step.** One slow game delays every flush; Workers = 1 serialises. Ready
   mode exists for comparison; §15 Q1.
3. **Slow built-in opponents** (search bots) hold the barrier; documented.
4. **Hung agent** hangs the run (§8.3); the caller's ctx is the only exit.
5. **Random agents and manual payment.** With `payment_actions: false` casts pay through
   posed decisions; a random agent may wander there and hit the intent watchdog — stalls are
   counted, not failures. **INFERRED** that the non-autopay payment path is reachable from a
   plain seat as it is from a human seat without AutoMana; E5d-4's tests report stall counts.
6. **Tape fidelity** (§7 INFERRED points). Pinned by E5d-3 tests over whole games, including
   an Env player.
7. **Concurrency bugs in the hub.** Tests are channel-driven (no sleeps, wall-clock ratchet
   `internal/testutil/wallclock_threshold_test.go:55`); the barrier's begin-all-then-call shape
   makes batch sizes exactly assertable; `-race` on the hub tests.
8. **Unix socket path length** (~108 bytes): tests use a short name inside `t.TempDir()`.
9. **Hot docs**: only E5d-6 touches `docs/agents/repo-map-engine.md` and
   `repo-map-paths.md`, one row each.

## 13. Design of record, brief and code: conflicts and the fix adopted

| # | says | code says | fix |
|---|---|---|---|
| D1 | brief: Player "registered as a bots policy name like `ext:<endpoint>`" | bots names match `^[a-z0-9-]+$`; the registry is static (`bots/registry.go:103-113`) | resolved by `extern.Registry` and recorded as a draftlab `AgentRef`; never `bots.Register` |
| D2 | E5 plan §9: a pick id not in the pack "is a hard error" | `pickbot.Drafter.Pick` has no error path (`lab/draft/limited/pickbot/registry.go:93-95`) | `FallibleDrafter`/`FallibleBuilder`; an illegal pick is a refusal → counted fallback by default (consistent with decide, where random agents' illegal answers are routine), a hard error with `StrictRefusals` |
| D3 | §9: flush "when every in-flight game is blocked on the model" | a seat never learns its game ended (`engine/seat/seat.go:19-21`); one hub per agent deadlocks across agents | `GameScoped` seam in `playGame`; ONE process-wide hub counting "waiting on any agent" (§6.1) |
| D4 | brief: the game "replays without the model" | events log only a game's head (`lab/draft/draftlab/event.go:271`); `VerifyEvent` re-drives with players (`:340-373`) | `Options.RecordTape`, `GameResult.Tape`, `ReplayTape`, `VerifyEventTapes` |
| D5 | §9: `Player` adapter returns "a `seat.Seat` (or `seat.BoardSeat`)" | a BoardSeat gets no View (`internal/bench/bench.go:393-399`) | plain Seat + `view.LeanReader`; never BoardSeat |
| D6 | brief: "errors/timeouts" | no `time` outside the allow-list (`internal/archtest/arch_test.go:122-152`) | ctx-only deadlines; §15 Q2 |
| D7 | — | `GameResult.Intents` is already an `int` (`lab/draft/draftlab/play.go:46`) | the new field is `Tape` |
| D8 | prior art: msgpack, timer flushes (`mzclient`, `dzgorge/remote.go`) | no third-party deps (`internal/archtest/gomod_test.go:26`); no clock | JSON lines; barrier/ready flushes; lean views for volume |
| D9 | §9: versioned "by the adapter's `AgentRef.Version` (the model id)" | — | adopted; the NAME is normalised to `ext:<hello.agent>` so a path never reaches a manifest |
| D10 | §9: `seed: r.Uint64()` | JSON numbers above 2^53 are inexact in many runtimes | `Seed53` |
| D11 | MageZero checklist item 2 (action mapping) | gorge offers positional options (`engine/decision/decision.go:175-178`) | the wire carries gorge's own options; mapping is the agent's job (§15 Q8) |

## 14. Tickets

| # | title | depends on | seat |
|---|---|---|---|
| E5d-1 | `lab/draft/draftlab/extern/wire` + `refagent` + `lab/cmd/extagent` | — | local OK |
| E5d-2 | `lab/draft/draftlab/extern`: transports, handshake, process-wide batching hub | E5d-1 | paid (DeepSeek) preferred: concurrency |
| E5d-3 | draftlab seams: `GameScoped`, `Fallible*`, `Lab.Card`, tapes | — | local OK |
| E5d-4 | `lab/draft/draftlab/extern` adapters + `Registry` + version freeze | E5d-2, E5d-3 | paid preferred |
| E5d-5 | `lab/cmd/labdemo`: `ext:` agents everywhere, `-drafter`/`-builder`/`-ext-*` | E5d-4 | local OK |
| E5d-6 | benchmark, Python example, repo-map rows | E5d-4 | local OK |

E5d-1 and E5d-3 start at once (disjoint packages). E5d-5 and E5d-6 run in parallel
(disjoint files). `packages.tiers.json` is edited by E5d-1 then E5d-2 (sequential).

## 15. Open questions for the operator

1. **Flush default.** Barrier (decided) or ready (inferbroker's rule, no membership, pipelines
   better under uneven games)? E5d-6 measures both; keep barrier unless ready wins clearly?
2. **Per-request timeout.** Accept ctx-only deadlines, or hand-add `lab/draft/draftlab/extern` to the
   archtest `time` allow-list (timers only decide when a request is abandoned, like
   `lab/training/gamepool`) so the adapter can enforce `Options.RequestTimeoutMS`?
3. **Failure default.** `FailError` (proposed: fail closed) or `FailFallback`?
4. **Spec grammar.** `ext:exec:<path>` takes no arguments (wrapper scripts select a
   checkpoint). Allow an argv form (e.g. `ext:exec:<path>,<arg>,…`)?
5. **Recorded name** `ext:<hello.agent>` (proposed) rather than the endpoint — OK?
6. **Seat isolation.** Offer a Registry mode that dials one process per seat, so an agent
   never holds two seats' views of one game or pod?
7. **History in `decide`.** v1 sends state snapshots only. Add the seat's redacted public
   event transcript since its previous decision (for history-dependent models) in a later
   additive field?
8. **Action mapping.** The agent answers gorge's positional options; MageZero's vocabulary
   mapping (checklist item 2) is out of scope — OK?
9. **Batched drafting.** `runDraftLoop` is serial over seats, so 8 external drafters cost 336
   round trips per pod. A sweep-batched loop changes the event log's entry ORDER (not the
   picks); worth a later opt-in?
10. **`examples/`** as a new top-level directory for the Python example — OK, or under
   `docs/`?
11. **`payment_actions` default false** (the agent pays through posed decisions, like a human
   without AutoMana) — OK, given built-in lab players use autopay plans?

## 16. Operator decisions (2026-10-14)

The operator took the recommended default on every §15 question:

1. Flush: the barrier stays the default; `ready` is selectable and E5d-6 measures both.
2. No per-request timeouts: deadlines come only from the caller's context. `lab/draft/draftlab/extern`
   is NOT added to the archtest `time` allow-list.
3. A dead agent fails the game (`FailError`); `FailFallback` is opt-in.
4. `ext:exec:` takes no argv; wrapper scripts select checkpoints.
5. The recorded name is `ext:<hello.agent>@<model_version>`; the path or socket is never recorded.
6. One-process-per-seat dialing: deferred.
7. Redacted public event history in `decide`: deferred.
8. Mapping to MageZero's action vocabulary is the agent's job.
9. A batched draft loop: deferred.
10. `examples/` as a new top-level directory: approved.
11. `payment_actions` defaults to false.
