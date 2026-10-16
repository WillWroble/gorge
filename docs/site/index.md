# gorge

**gorge** is a deterministic, event-sourced *Magic: The Gathering* rules engine
written in pure Go. Card behaviour is compiled from
[Forge](https://github.com/Card-Forge/forge)'s card scripts into an
intermediate representation (IR); the engine implements the primitives that IR
references. It is built to replace an XMage/JVM bridge in a hosted play
service, and to be fast and reproducible enough to serve as an environment for
search and learned bots.

Every state change goes through one function, `events.Apply`, into a
hash-chained event log. A match is therefore a pure function of its
configuration and the players' answers, so replay, resume, undo and search
clones are all one mechanism.

!!! info "Status"
    gorge plays full games, serves bot tables to a browser and replays them
    byte for byte. It is **not** yet at rules parity with mature engines and is
    not ready for production use. The numbers below are a snapshot; see each
    page for how to re-measure them.

## At a glance

*As of 2026-10-12, `main` @ `f1ac002c4`.*

| | |
|---|---|
| Forge corpus coverage | **32,089 of 34,074** card scripts playable (**94.2%**) at the pinned Forge commit `25a608a4fc5f` |
| Standard (20 sets, 5,331 cards) | **100%** at compliance Level A; **93.8%** at Level B (329 cards outstanding) |
| Other sets | an *estimated* 96–98% playable for most sets; see [Support by set](support/sets.md) |
| Bundled test decks | 30 constructed and Commander decks; 2 cards in them are listed as not fully supported (Incinerate, Vines of Vastwood) |
| Determinism | golden games at 2/4/6/8 seats replay byte-identically; their chain heads are pinned in the repo |
| Engine speed | ~3,200 turns/s per CPU core on random-play FDN games; ~120 heuristic-bot games/s per core ([details](performance/engine.md)) |
| Dependencies | no cgo, and no third-party dependencies in the card pipeline or rules core |

"Level A" and "Level B" are gorge's compliance tiers: each card's behaviour is
compared scenario by scenario against [XMage](https://github.com/magefree/mage)
as an oracle. [Support by set](support/sets.md) explains exactly what each level
certifies.

## Where to go next

- **[Getting started](getting-started.md)**: fetch the card corpus, build, run a
  local server, and replay games.
- **[Architecture](architecture/overview.md)**: the tiers, the packages, and the
  invariants that hold the engine together.
- **[Performance](performance/engine.md)**: how engine speed is measured, current
  numbers, and the state of the bot and ML work.
- **[Card support](support/sets.md)**: per-set support for Standard and estimates
  for everything else.
- **[Cleanup roadmap](architecture/cleanup.md)**: code that is redundant,
  parked or optional, and what is planned for it.

## Licensing

gorge is licensed under Apache-2.0. Forge's card scripts are GPL-3.0, so gorge
ships a *compiler* for the script format and never the scripts themselves: the
corpus is fetched into a gitignored `.cards/` directory at a pinned commit,
and a test fails the build if a script is ever tracked. See
[Getting started](getting-started.md#licensing).

## Contributing

The contributor guide lives in the repository rather than on this site:
[`docs/agents/`](https://github.com/adams-shaun/gorge/tree/main/docs/agents)
(the repo map, the invariants and the contributor workflow) and
[`AGENTS.md`](https://github.com/adams-shaun/gorge/blob/main/AGENTS.md).
