# Report cs1: idle unmerged branches

Round l0 left no uncommitted work in this worktree (git status clean at start).

## Decisions
- wt/bot-sanity (+10): parked. Clean status, merges cleanly with main (merge-tree ok) but no gates were run on it here; landing is out of scope without passing gates.
- wt/llm-bridge (+3): parked. Conflicts with main in cmd/searchteacher/main.go (merge-tree), needs a rebase by an owner.
- wt/forgepass-r2 (+6): WIP committed first (068ab9405: 4 verdict files, compliance/adjudication, status note) then parked. Unfinished per its STATUS note (XMage leg, P2-6, P3-4/5 remain).
- wt/qab-s2port (+6): parked. Clean status, POC (GORGE_QUIET_INC); not finished.

## Result
`git worktree list` has no entry for any of the four; branches kept.
`git worktree remove` printed "Read-only file system" for the directories (jail), but the git registration (.git/worktrees/<name>) was removed, so the files are released from git. The orphaned directories under .worktrees/ remain on disk and need `rm -rf` by the operator.

## Compliance check / tests
No code changed; not run.

## Issues
- Orphan dirs: .worktrees/{bot-sanity,llm-bridge,forgepass-r2,qab-s2port} still on disk (operator rm -rf).
- forgepass-r2 also noted an untracked ledger in the main checkout compliance/adjudication (per its STATUS note).
