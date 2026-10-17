# Regenerating the set tables

The per-set figures on [Support by set](sets.md) come from the compliance
oracle, which compares gorge with XMage scenario by scenario. Since
2026-10-16 that oracle runs from a private companion repository, not from
gorge, so the docs build no longer measures sets: the page shows its hand
snapshot.

## What stays in gorge

The renderer does. `scripts/docs_sets.py` turns the oracle's per-set status
JSONL into `docs/site/support/_generated_sets.md`, which `sets.md` includes at
the end through `pymdownx.snippets`. The fragment is gitignored; when it is
absent the page shows only the hand snapshot. Only counts and percentages
are published, never card text.

```sh
python3 scripts/docs_sets.py status-A.jsonl          # -> docs/site/support/_generated_sets.md
uvx --with mkdocs-material==9.7.7 mkdocs serve        # preview at http://127.0.0.1:8000/
```

`docs_sets.py` needs only the Python standard library. It skips any line that
is not a JSON object. Options: `-o PATH`, `--commit SHA` and
`--date YYYY-MM-DD` (shown in the fragment's header).

Each JSONL line is one set:

| Field | Meaning |
|---|---|
| `set`, `name`, `set_type`, `released` | set code, name, XMage set type, release date |
| `formats` | formats the set is legal in |
| `level`, `declared` | level checked; level the set is declared at |
| `printed` | whether a full printed card list exists |
| `cards` | cards the claim covers |
| `outstanding`, `problem_cards` | problems, and distinct cards with at least one problem |
| `buckets` | problems by reason: `unsupported`, `not in corpus`, `template gap`, `no verdict`, `diverge`, `harness`, `no printed list`, … |
| `non_tournament` | outstanding problems on non-tournament cards (planes, schemes, …) |

`docs_sets.py` computes **playable %** as
`1 − (unsupported + not in corpus) / cards` and **Level A %** as the share of
cards with no Level A problem (`no printed list` is a per-set pseudo-problem
and is not counted as a card).

## What you can measure from gorge alone

```sh
make fetch-cards && make compile-cards
make coverage             # .coverage/{summary,coverage}.md: corpus coverage by type, colour, mana value
make report               # corpus-wide playable count
```

The separate [`card coverage`](https://github.com/adams-shaun/gorge/blob/main/.github/workflows/coverage.yml)
workflow publishes `make coverage`'s tables as a job summary and artifact on
every push to `main`.
