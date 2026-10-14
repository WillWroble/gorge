# Stability veto oom_kills=1 at 7c5be0db5 — report

## Cause (measured)
`/proc/vmstat oom_kill` is 2; `collect_stability` (scripts/reward_collect.py:996) scores the DELTA since the last probe.
Kernel journal: `Oct 14 10:30:16 cv-one invoked oom-killer ... Memory cgroup out of memory: Killed process 3942206 (cv-one) anon-rss:6263596kB`.
journalctl --user: scope run-p3942206 = `systemd-run ... GOMAXPROCS=2 GOMEMLIMIT=5GiB /tmp/claude-1000/.../scratchpad/cv-one -corpus .cards -set FDN -mode one -pilot bot -n 86179 -workers 1 -hands=false` ("6G memory peak").
It was an interactive session's ad-hoc run of a scratch-built cardval binary (a `-mode one` that is not in cmd/cardval on main: modes are random|variance|swap), launched under its own systemd-run scope, outside the broker and outside the repo. GOMEMLIMIT=5GiB vs a 6G scope leaves no headroom for non-heap memory. The first kill (08:50, cardval, 24.8 GB, cardval-hands.service) was already in the baseline.
No committed repo code caused this event; the same run was relaunched at 10:30:57 without incident.

## Change
None to code: the counter is delta-based so the veto clears on the next probe with no new kill. Nothing in-repo to remove.

## Gates
`bash scripts/tests/broker_smoke.sh` -> `0 failure(s)`.

## Issues
- cmd/cardval retains per-game state for large -n (hands-bot run peaked at 22-24 GB, killed once at 24.8 GB anon-rss). Hypothesis: lines/census accumulation in main.go; not measured here, out of scope.
- Ad-hoc heavy runs by interactive sessions bypass the broker lease; a sweep launched like this cannot be paused for a gate.
