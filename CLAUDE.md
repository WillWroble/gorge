# CLAUDE.md

This fork runs MageZero, Will Wroble's AlphaZero-style system for Magic, on the gorge rules engine. Read
`mz-gorge-design.md` fully before touching code. It decides the design. Where it is silent, the mage fork
(`WillWroble/mage`, `graph-encoder` HEAD) fills in the details.

## The plan

The work runs in three stages, in order:

1. Engine factorization and view exposure
2. Search design
3. Encoder design

One stage at a time. Each ends in a PR that Will signs off before the next one starts. Every stage should know the
whole plan, not just its own part.

## Testing

- All of gorge's existing tests are used. Record which pass before changing anything; that is the baseline.
- gorge is deterministic. Same seed and same answers give the same game, so use that to catch regressions.
- Report everything for both autotap and manual tap.

## Measurement

- All measurement is offline MCTS. Never start the model server or include inference time.
- Speed is sims per second and cost per search step on the same decks as the mage fork, never games per second.
- Every number goes in `docs/mz/LOG.md` with the command, the machine and the commit.

## Rules

- Never push to `main`. Work on the branch the session started on; Will merges via PR.
- Don't pull upstream gorge without Will.
- Don't change the mage fork or the Python side.
- If a decision isn't settled by the design doc or the mage fork, stop and ask.
- gorge's own agent docs (`AGENTS.md`, `docs/agents/`, `CONTRIBUTING.md`, `docs/design/`, `docs/decisions/`) describe
  the upstream design. Where they conflict with `mz-gorge-design.md`, the design doc wins. Will is the maintainer of
  this fork.

## Environment

- Go 1.25.8 (`go.mod`).
- First run `make fetch-cards compile-cards`. Without the card corpus, the tests that need it fail on purpose.
- The mage fork is in the session as a second repo, read-only.