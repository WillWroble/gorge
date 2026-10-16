# Packages

The tables below are generated from the source tree by
[`cmd/pkgmap`](https://github.com/adams-shaun/gorge/tree/main/cmd/pkgmap) each
time the site is built, so they never go stale. Tier and status come from one
committed classification file,
[`docs/site/architecture/packages.tiers.json`](https://github.com/adams-shaun/gorge/blob/main/docs/site/architecture/packages.tiers.json);
`go run ./cmd/pkgmap -check` (and the package's test) fails when a package is
missing from it or an entry names a path that no longer exists.

Columns:

- **Purpose**: the first sentence of the package doc comment (or the
  classification's `purpose` when the package has none).
- **LOC / Test LOC**: non-test / test lines of Go in that directory (not
  recursive), counting every build tag.
- **Imp**: in-repo packages importing it from non-test code; `+N` adds the
  packages that import it only from tests.
- **Status**: **CORE** (the engine), **SUPP** (supports the product),
  **EXT** (optional extension: bots, search, ML, alternate wires),
  **TOOL** (tooling, tests, ratchets), **SUSP** (suspected obsolete) or
  **RED** (redundant). SUSP and RED entries are discussed on the
  [cleanup roadmap](cleanup.md).

The tier-dependency graph at the end is built from the actual imports, and
flags any import from a tier into one listed after it.

For where a given kind of change belongs, the contributor-facing repo map is
[`docs/agents/repo-map.md`](https://github.com/adams-shaun/gorge/blob/main/docs/agents/repo-map.md).

To preview locally:

```sh
go run ./cmd/pkgmap > docs/site/architecture/_generated_packages.md
```

--8<-- "architecture/_generated_packages.md"

## Non-Go top-level paths

| Path | What | Status |
|---|---|---|
| `web/` | Svelte+Vite client | SUPP |
| `scripts/` | contributor and CI helpers: worktree creation, CI sharding, smokes, enginebench | TOOL |
| `ops/scripts/` | the landing pipeline: post-merge suite, ratchet and affected-package gates, heavy-job lock, reward/seed agent, fleet and demo deploy | TOOL (live) |
| `ops/orchestrator/` | `hooks.py` merge-gate hooks (`ops.orchestrator.hooks:*`) for agentctl | TOOL (live) |
| `ops/deploy/` | systemd units installed by `ops/scripts/seed-install.sh` | TOOL |
| `lab/scripts/` | oracle, training and SpellBench-arena drivers | EXT |
| `lab/oracle/xmageoracle/` | Java XMage replay driver | EXT |
| `lab/oracle/validation/oracle/` | verdict refresh runbook | EXT |
| `docs/` | agent guide (`docs/agents/`), design docs (`docs/design/`), decisions (`docs/decisions/`), this site (`docs/site/`) | SUPP |
