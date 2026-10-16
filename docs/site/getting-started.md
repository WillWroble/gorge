# Getting started

gorge needs Go (the version in `go.mod`), `make` and `git`. Node is needed only
to build the browser client.

## 1. Fetch and compile the card corpus

```sh
git clone https://github.com/adams-shaun/gorge && cd gorge
make fetch-cards      # one-time, ~25 MB: Forge card + token scripts at FORGE_REF into .cards/
make compile-cards    # parse the scripts into the IR cache (.cards/ir.gob.gz)
make report           # print corpus coverage against the implemented primitives
```

`make fetch-cards` builds `bin/forgec` and does a sparse checkout of the Forge
card scripts at the commit pinned by `FORGE_REF` in the `Makefile`
(`FORGE_REPO` points at gorge's fork of Card-Forge/forge). Coverage numbers are
always relative to that pin. `make report` ends with a line like:

```text
cards: 34074  playable: 32089 (94.2%)
```

`make coverage` writes the same measurement as markdown tables to
`.coverage/` (gitignored), broken down by card type, colour and mana value,
with the primitives the remaining cards are waiting on.

## 2. Play verified games

```sh
make sim
```

This builds `bin/mtgsim` and plays 20 four-seat games between the built-in bot,
replaying each one and checking that the rebuilt event chain matches the live
run. Every game should print `replay OK`. `mtgsim` takes `-seats`, `-games`,
`-seed`, `-decks` and `-verify` directly:

```sh
bin/mtgsim -seats 2 -games 5 -seed 42 -verify -v
```

## 3. Run a local server

`gorged` hosts perpetual bot tables and a play-vs-bot flow over HTTP (JSON +
server-sent events) and serves the browser client.

```sh
make web                                   # optional: build the Svelte client (needs Node)
go build -o bin/gorged ./cmd/gorged
bin/gorged -addr :8090 -dir /tmp/gorge-local -tables 2 -seats 4 -vsbot
```

Then open `http://localhost:8090/`. Without the web build the root answers
`503`, but the REST endpoints such as `/api/tables` still work. `gorged -h`
lists every flag (`-tables`, `-seats`, `-format`, `-pace`, `-mulligans`,
`-spectator`, ...).

!!! tip "Pick a free port and a private data directory"
    `make gorged` uses the default `:8080` and `gorged-data/`. If anything else
    on your machine might already use those, pass your own `-addr` and `-dir`.
    Two servers must never share one `-dir`: they silently corrupt each other's
    persisted matches.

## 4. Run the tests

```sh
make test lint        # the full suite; heavy, see the note below
go test ./engine/rules/ -run 'TestHeads$|TestRepoDeckGamesReplayExactly' -v
```

The full suite is large (about 12,000 tests). For day-to-day work run single
packages with `-run`. Tests that need the corpus **skip** when `.cards/` is
missing, so a fast green run without the corpus has tested very little.

`make conformance` runs the focused CR 601/733 conformance audit on its own.

## 5. Reproduce a reported game

Every match is replayable from its configuration plus the recorded answers. A
feedback snapshot (`match.json`, `log.json`, the seat's `view.json`) can be
replayed and turned into a failing test:

```sh
go run ./cmd/repro <feedback-dir>                   # replay, verify the chain head, print the board
go run ./cmd/repro -list <feedback-dir>             # the timeline of recorded answers
go run ./cmd/repro -at N <feedback-dir>             # stop at answer N
go run ./cmd/repro -emit-test <pkg> <feedback-dir>  # copy the snapshot and write a failing test skeleton
```

Exit status 0 means the rebuilt event stream matched the recording event for
event. A `DIVERGED` result names the first mismatching event; a corpus pin
change since the snapshot was taken is an expected cause.

## Licensing

gorge is Apache-2.0. Forge's card scripts are GPL-3.0, so they are **never
committed** to this repository:

- `forgec fetch` downloads them into `.cards/`, which is gitignored, pinned to
  a commit SHA (`FORGE_REF`).
- `engine/cards/boundary_test.go` fails the build if any Forge script is tracked in
  the working tree, the index or `HEAD`.
- Replay fixtures strip embedded token-script text before they are committed;
  it is resolved from the local corpus at load time.
- Published coverage tables, including the ones on this site, contain only
  counts and percentages, never card text.
