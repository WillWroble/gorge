#!/bin/sh
# gpu-fault-watch.sh — snapshot the box the moment amdgpu reports a GPU page
# fault or ring timeout, so a desktop freeze while viewing the demo leaves a
# record of what Chrome and the demo were doing.
#
# Context (2026-10-13): Chrome's GPU process on the Granite Ridge iGPU
# (renderD130) raised `[gfxhub] page fault ... PERMISSION_FAULTS: 0x3`, then
# `ring gfx_0.0.0 timeout` resets every ~10s wedged the whole desktop.
#
#   systemd-run --user --unit=gpu-fault-watch scripts/gpu-fault-watch.sh
#   journalctl --user -u gpu-fault-watch      # the watcher's own log
#   scripts/gpu-fault-watch.sh --snapshot-now # one snapshot, to test the capture
#
# Each event writes $OUT/<UTC stamp>/: kernel lines, Chrome processes and the
# GPU process's flags, AMD sysfs counters, the demo's live tables, and the
# amdgpu devcoredump when `sudo -n` may read it (it is root-only and the
# kernel drops it ~5 minutes after the hang). Snapshots are debounced to one
# per DEBOUNCE seconds; a reset storm is one event.

OUT=${GPU_FAULT_DIR:-/mnt/sata/gorge-data/gpu-faults}
CARD=${GPU_FAULT_CARD:-/sys/class/drm/card3/device}
DEMO=${GPU_FAULT_DEMO:-http://127.0.0.1:8080}
DEBOUNCE=${GPU_FAULT_DEBOUNCE:-60}
PATTERN='amdgpu.*(page fault|ring .* timeout|GPU reset|device wedged)'

mkdir -p "$OUT"
last=0
echo "gpu-fault-watch: writing to $OUT"

snapshot() {
	d="$OUT/$(date -u +%Y%m%dT%H%M%SZ)"
	mkdir -p "$d"
	echo "$1" > "$d/trigger.txt"
	date -Is > "$d/time.txt"
	# Let the burst of lines that follows a fault land first.
	sleep 2
	journalctl -k --since '-3 min' --no-pager -o short-iso | /usr/bin/grep -E 'amdgpu|drm' > "$d/kernel.txt"
	for f in gpu_busy_percent mem_info_vram_used mem_info_gtt_used; do
		echo "$f $(cat "$CARD/$f" 2>/dev/null)"
	done > "$d/amdgpu-sysfs.txt"
	# Chrome processes: pid, type, RSS, and the GPU process's full flags.
	for p in /proc/[0-9]*; do
		exe=$(readlink "$p/exe" 2>/dev/null) || continue
		case "$exe" in */chrome) ;; *) continue ;; esac
		cmd=$(tr '\0' ' ' < "$p/cmdline" 2>/dev/null)
		type=$(printf '%s' "$cmd" | /usr/bin/grep -oE -- '--type=[a-z-]+' | head -n 1)
		rss=$(/usr/bin/grep -E '^VmRSS' "$p/status" 2>/dev/null | awk '{print $2}')
		echo "${p#/proc/} ${type:---type=browser} rss_kb=$rss"
		case "$cmd" in *--type=gpu-process*) echo "$cmd" >> "$d/chrome-gpu-cmdline.txt" ;; esac
	done > "$d/chrome-procs.txt"
	curl -s -m 3 "$DEMO/api/tables" > "$d/demo-tables.json"
	if [ -r "$CARD/devcoredump/data" ]; then
		cp "$CARD/devcoredump/data" "$d/devcoredump.bin"
	elif sudo -n true 2>/dev/null && sudo -n test -e "$CARD/devcoredump/data"; then
		sudo -n cat "$CARD/devcoredump/data" > "$d/devcoredump.bin"
	else
		echo "devcoredump not captured (needs root, or already dropped)" > "$d/devcoredump.missing"
	fi
	echo "gpu-fault-watch: snapshot $d"
}

if [ "${1:-}" = --snapshot-now ]; then
	snapshot "manual snapshot"
	exit 0
fi

journalctl -k -f -n 0 --no-pager -o short-iso | while IFS= read -r line; do
	printf '%s\n' "$line" | /usr/bin/grep -qE "$PATTERN" || continue
	now=$(date +%s)
	[ $((now - last)) -lt "$DEBOUNCE" ] && continue
	last=$now
	snapshot "$line"
done
