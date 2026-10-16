#!/usr/bin/env bash
# Post-merge full suite (operator, 2026-10-05): the counterpart of the lean
# per-ticket gate in scripts/gate_affected.sh. Runs the whole module suite,
# including the tests the per-ticket gate skips, on one commit of main.
#
#   ops/scripts/postmerge_full.sh <worktree> <sha>
#
# The worktree must be a dedicated one made by scripts/agent-worktree.sh (it
# needs .cards); it is detached onto <sha>. Exit status is the suite's.
set -euo pipefail
wt=${1:?worktree}; sha=${2:?sha}
git -C "$wt" switch -q --detach "$sha"
cd "$wt"
skip='^(TestHeads)$'
# Opt-in exhaustive audits that the lean per-ticket gate leaves out
# (db57ab8e6 made the oraclegen target audit opt-in); the full suite runs them.
export GORGE_ORACLEGEN_FULL_TARGET_AUDIT=1
# The platform build links no lab/ code (docs/decisions/0001-repo-layout.md):
# prove it builds and that gorged refuses -events there, then run everything
# else with -tags lab so the draft-event wiring (cmd/gorged events.go, the
# httpapi event e2e tests) stays covered exactly as before the fence.
go build ./...
go test -run '^TestEventsNeedLabBuild$' ./cmd/gorged
export GOFLAGS="-tags=lab${GOFLAGS:+ $GOFLAGS}"
go vet -p=8 ./...
# Per-test budget (operator, 2026-10-05, doubled 2026-10-09): every test fits 4 GB RSS, 4 vCPU,
# 1 min wall. `go test -exec "testbudget exec"` runs every test binary and
# records its peak RSS (wait4 rusage) per package -- no package carries a
# TestMain for it -- while keeping the test result cache usable. cmd/testbudget
# reads those persistent records and the per-test wall from the -json stream
# (tee reprints the plain text the batch's failure parser reads).
# The RSS directory survives runs so cached packages retain their valid record;
# GORGE_TESTBUDGET_RSS=0 checks wall only.
budget=$PWD/.ds4/scratch/testbudget
mkdir -p "$budget"
go build -o "$budget/testbudget" ./cmd/testbudget
rss_flag=()
if [ "${GORGE_TESTBUDGET_RSS:-1}" = 1 ]; then
	export GORGE_TESTBUDGET_RSS_DIR=$budget/rss
	rss_flag=(-rss-dir "$budget/rss")
fi
go test -p=8 -json -exec "$budget/testbudget exec" -skip "$skip" ./... | "$budget/testbudget" tee -events "$budget/events.json"
"$budget/testbudget" check -events "$budget/events.json" "${rss_flag[@]}"
go test ./engine/rules -run '^TestHeads$'
make sim
# Shell smokes run in the post-merge batch, not the lean per-ticket gate (~63s).
bash ops/scripts/tests/reward_loop.sh
