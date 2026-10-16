#!/usr/bin/env bash
# ci_shard.sh <index> <count> -- print the Go packages CI shard <index> of
# <count> runs, one per line.
#
# The heaviest packages each get a shard of their own (index 0..H-1 of the
# HEAVY list below), measured on the post-merge suite: rules ~170 s, host
# ~155 s, eventhost ~143 s. Every other package goes to one of the remaining
# shards by a stable hash of its import path, so adding a package never
# reshuffles the others. Rebalance HEAVY from the CI run times when a light
# shard outgrows the heavy ones.
#
# Shard index is 0-based. `ci_shard.sh all <count>` prints every package with
# its shard, which is how the self-check in CI proves no package was dropped.
set -euo pipefail
idx=${1:?usage: ci_shard.sh <index|all> <count>}
count=${2:?usage: ci_shard.sh <index|all> <count>}

HEAVY=(
	github.com/adams-shaun/gorge/rules
	github.com/adams-shaun/gorge/host
	github.com/adams-shaun/gorge/eventhost
)
nheavy=${#HEAVY[@]}
if [ "$count" -le "$nheavy" ]; then
	echo "ci_shard.sh: need more than $nheavy shards, got $count" >&2
	exit 2
fi
light=$((count - nheavy))

shard_of() {
	local p=$1 i
	for i in "${!HEAVY[@]}"; do
		if [ "${HEAVY[$i]}" = "$p" ]; then
			echo "$i"
			return
		fi
	done
	# cksum is POSIX and deterministic across runners.
	local h
	h=$(printf '%s' "$p" | cksum | cut -d' ' -f1)
	echo $((nheavy + h % light))
}

go list ./... | while read -r p; do
	s=$(shard_of "$p")
	if [ "$idx" = all ]; then
		echo "$s $p"
	elif [ "$s" = "$idx" ]; then
		echo "$p"
	fi
done
