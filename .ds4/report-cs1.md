# Report cs1 — idle unmerged branches

Measured at start of this round (`git worktree list`, `git rev-list --left-right --count main...<b>`, `git merge-tree --write-tree`):

| branch | branch-only commits | worktree | merge vs main | decision |
|---|---|---|---|---|
| wt/bot-sanity | +10 | none | clean | parked: branch kept, no worktree holds files; not merged (gates not run in this seat) |
| wt/llm-bridge | +3 | none | CONFLICT | parked: branch kept, no worktree; needs rebase by its owner |
| wt/forgepass-r2 | +6 | none | clean | parked: WIP commit 068ab9405 already present (uncommitted verdict edits salvaged), no worktree |
| wt/qab-s2port | +6 | none | clean | parked: branch kept, no worktree |
| wt/rs-encablate | +2 (+WIP) | /home/sadams/projects/gorge/.worktrees/rs-encablate | clean | uncommitted ablate.py + report committed by path as 92e5d0482; worktree NOT removed (see Issues) |

`scripts/reward_collect.py flow --repo .` now reports `idle_unmerged_branches` = 1 (rs-encablate, +2, 48.2h idle); it was 5.

## Issues
- Removing the rs-encablate worktree needs `git worktree remove`, which this seat's rules forbid (shared git state). The controller should run it; the branch is fully committed (92e5d0482), nothing is lost.
- llm-bridge conflicts with main (merge-tree); landing it needs a rebase by someone who owns it.
- bot-sanity and qab-s2port merge cleanly but I did not run their gates; merging is out of scope.
