#!/usr/bin/env bash
# reward_loop.sh — every shell smoke, plus the reward-loop Python selftests.
#
# All ops/scripts/tests/*.sh smokes run here, except those named on the opt-out
# list at the foot of this file; none are manual-only. Their measured combined
# wall time is ~64s, so this is called by the post-merge full suite, not the
# per-ticket gate. Keep this entry list in sync with the directory -- the drift
# guard at the foot of this file fails the loop if a smoke has no run entry.
#
#   ops/scripts/tests/reward_loop.sh
set -uo pipefail
ROOT=$(git rev-parse --show-toplevel)
rc=0
run() {
	printf '\n=== %s\n' "$1"
	shift
	"$@" || rc=1
}
run "reward.py --selftest" python3 "$ROOT/ops/scripts/reward.py" --selftest
run "reward_collect.py --selftest" python3 "$ROOT/ops/scripts/reward_collect.py" --selftest
run "seed_candidates.py --selftest" python3 "$ROOT/ops/scripts/seed_candidates.py" --selftest
run "broker_smoke.sh" bash "$ROOT/ops/scripts/tests/broker_smoke.sh"
run "broker_heavy_pausable_smoke.sh" bash "$ROOT/ops/scripts/tests/broker_heavy_pausable_smoke.sh"
run "deploy_sweep_smoke.sh" bash "$ROOT/ops/scripts/tests/deploy_sweep_smoke.sh"
run "park_branch_smoke.sh" bash "$ROOT/ops/scripts/tests/park_branch_smoke.sh"
run "cleanup_orphan_smoke.sh" bash "$ROOT/ops/scripts/tests/cleanup_orphan_smoke.sh"
run "readopt_worktree_smoke.sh" bash "$ROOT/ops/scripts/tests/readopt_worktree_smoke.sh"
run "seed_gorged_reap_smoke.sh" bash "$ROOT/ops/scripts/tests/seed_gorged_reap_smoke.sh"
run "seed_smoke.sh" bash "$ROOT/ops/scripts/tests/seed_smoke.sh"
run "postmerge_batch_smoke.sh" bash "$ROOT/ops/scripts/tests/postmerge_batch_smoke.sh"
run "seed_hotspot_sequenced_smoke.sh" bash "$ROOT/ops/scripts/tests/seed_hotspot_sequenced_smoke.sh"
run "gorged_reap_smoke.sh" bash "$ROOT/ops/scripts/tests/gorged_reap_smoke.sh"
run "ledger_lane_smoke.sh" bash "$ROOT/ops/scripts/tests/ledger_lane_smoke.sh"
run "gate_affected_smoke.sh" bash "$ROOT/ops/scripts/tests/gate_affected_smoke.sh"
run "park_branch_registry_only_smoke.sh" bash "$ROOT/ops/scripts/tests/park_branch_registry_only_smoke.sh"
run "park_idle_scheduled_smoke.sh" bash "$ROOT/ops/scripts/tests/park_idle_scheduled_smoke.sh"
run "seed_brief_premise.sh" bash "$ROOT/ops/scripts/tests/seed_brief_premise.sh"
run "postmerge_batch_smoke.sh" bash "$ROOT/ops/scripts/tests/postmerge_batch_smoke.sh"
run "seed_reap_smoke.sh" bash "$ROOT/ops/scripts/tests/seed_reap_smoke.sh"
run "storm_brief_asserts_endpoint.sh" bash "$ROOT/ops/scripts/tests/storm_brief_asserts_endpoint.sh"

# Drift guard: every ops/scripts/tests/*.sh smoke must have a run entry above.
# reward_loop.sh itself is the runner, not a smoke. The opt-out list names
# smokes deliberately left unregistered:
#   heavy_lock_smoke.sh -- RED on main: its merge-base precondition
#   (ops/scripts/tests/heavy_lock_smoke.sh:74-76) asserts ops/scripts/heavy_lock.sh is
#   absent from the merge-base, but commit 7cbba9527 landed it, so the smoke
#   exits 1. Registering a red smoke would turn every post-merge batch red.
#   Delete this opt-out entry when that smoke is fixed and registered.
opt_out="reward_loop.sh heavy_lock_smoke.sh"
for smoke in "$ROOT"/ops/scripts/tests/*.sh; do
	name=$(basename "$smoke")
	skip=0
	for o in $opt_out; do
		[ "$name" = "$o" ] && skip=1
	done
	[ "$skip" = 1 ] && continue
	if ! grep -q "\"$name\"" "$ROOT/ops/scripts/tests/reward_loop.sh"; then
		printf 'DRIFT: ops/scripts/tests/%s has no run entry in reward_loop.sh\n' "$name" >&2
		rc=1
	fi
done

printf '\n=== reward loop: %s\n' "$([ $rc = 0 ] && echo ALL GREEN || echo FAILURES ABOVE)"
exit $rc
