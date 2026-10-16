# ops/restructure

`restructure.py` is the mechanical half of the 2026-10-16 repo layout move
(docs/decisions/0001-repo-layout.md). It is idempotent on an old-layout tree.

## Bringing a pre-restructure branch across

A branch cut before the move cannot be rebased onto the new main file by
file: every path changed. Replay the move on it instead.

```sh
git worktree add .worktrees/<id>-x wt/<id>              # never in the main checkout
cd .worktrees/<id>-x
git merge-base --is-ancestor <pre-move-main> HEAD        # branch must contain the parent of the move
cp ../../ops/restructure/restructure.py /tmp/r.py        # the script from NEW main
python3 /tmp/r.py && python3 /tmp/r.py --stage=ops
goimports -w $(git diff --name-only -- '*.go')
git add -A && git commit -m "restructure replay"
git merge main                                           # now only the branch's own edits conflict
```

The script does not carry the hand fixes that landed with the move (archtest
lists, codeshape dirs, gate regexes, smoke fixtures); a branch that edits one
of those files resolves that conflict by hand. Build every tag set after the
merge: `go vet ./... && go vet -tags lab ./...`.
