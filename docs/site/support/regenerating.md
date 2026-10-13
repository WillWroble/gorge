# Regenerating the set tables

The support tables are measured from the repository; nothing on the
[Support by set](sets.md) page needs XMage to reproduce. It needs the Forge
corpus in `.cards/` and the compiled IR cache.

## What the docs build does

The [`docs` workflow](https://github.com/adams-shaun/gorge/blob/main/.github/workflows/docs.yml)
runs on every push to `main` that touches the docs, the set script or the
committed verdicts:

1. `make fetch-cards` fetches the corpus at `FORGE_REF` into `.cards/` (thrown
   away with the runner; never committed).
2. `go run ./cmd/oraclediff status -all -json -level A -procs 4 > status-A.jsonl`
   measures every set (about 7.5 minutes on 4 CPUs). The command also checks
   `compliance/ratchet.json` and exits non-zero when a set moves past it; the
   build tolerates that exit and uses the output if it is non-empty.
3. `python3 scripts/docs_sets.py status-A.jsonl` writes
   `docs/site/support/_generated_sets.md`, which `sets.md` includes at the end
   through `pymdownx.snippets`. The fragment is gitignored; when it is absent
   the page shows only the hand snapshot.

Only counts and percentages are published, never card text.

## Running it locally

```sh
make fetch-cards && make compile-cards
go run ./cmd/oraclediff status -all -json -level A -procs 2 > status-A.jsonl
python3 scripts/docs_sets.py status-A.jsonl          # -> docs/site/support/_generated_sets.md
uvx --with mkdocs-material==9.7.7 mkdocs serve        # preview at http://127.0.0.1:8000/
```

`docs_sets.py` needs only the Python standard library. It skips any line that
is not a JSON object, so the ratchet summary `oraclediff` prints alongside the
JSON is harmless. Options: `-o PATH`, `--commit SHA` and `--date YYYY-MM-DD`
(shown in the fragment's header).

Each JSONL line is one set:

| Field | Meaning |
|---|---|
| `set`, `name`, `set_type`, `released` | set code, name, XMage set type, release date |
| `formats` | formats the set is legal in |
| `level`, `declared` | level checked; level declared in `compliance/declared.json` |
| `printed` | whether a full printed card list exists (`compliance/printed/`) |
| `cards` | cards the claim covers |
| `outstanding`, `problem_cards` | problems, and distinct cards with at least one problem |
| `buckets` | problems by reason: `unsupported`, `not in corpus`, `template gap`, `no verdict`, `diverge`, `harness`, `no printed list`, … |
| `non_tournament` | outstanding problems on non-tournament cards (planes, schemes, …) |

`docs_sets.py` computes **playable %** as
`1 − (unsupported + not in corpus) / cards` and **Level A %** as the share of
cards with no Level A problem (`no printed list` is a per-set pseudo-problem
and is not counted as a card).

## Other useful commands

```sh
go run ./cmd/oraclediff status -all -json -level B -sets FDN,DSK -procs 2   # Level B for chosen sets
go run ./cmd/oraclediff sets -out compliance-sets.md                        # markdown summary, ~1.6 min
make coverage             # .coverage/{summary,coverage}.md: corpus coverage by type, colour, mana value
make compliance-status    # compliance/status.md (gitignored)
```

The separate [`card coverage`](https://github.com/adams-shaun/gorge/blob/main/.github/workflows/coverage.yml)
workflow publishes `make coverage`'s tables as a job summary and artifact on
every push to `main`.
