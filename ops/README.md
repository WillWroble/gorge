# ops/ — the agent pipeline's repo-side half

What runs gorge's landing pipeline from this repository. agentctl itself
(daemon, dashboard, seats) lives in `~/projects/agentctl`; this directory holds
what it calls. Contributor tooling is in `scripts/`, research drivers in
`lab/scripts/`.

| Path | What it holds |
|---|---|
| `orchestrator/` | `hooks.py`, the gate hooks agentctl resolves as `ops.orchestrator.hooks:*` ([README](orchestrator/README.md)). |
| `deploy/` | systemd user units: `gorge-seed.{service,timer}`, `gorge-nightly.{service,timer}`, `gorge-heavy.slice`, `gorge-probe.slice`. |
| `scripts/` | Pipeline scripts: the post-merge suite (`postmerge_batch.sh`, `postmerge_full.sh`), the gates' helpers (`ratchet_gate.py`, `gate_affected.sh`), the heavy-job lock (`heavy.sh`, `heavy_lock.sh`, `broker.sh`), the reward loop and seed agent (`reward*.py`, `seed-*.sh`), `fleet.sh` (ports), `deploy-demo.sh` (operator only), `reward_collect.py` and `hotfiles-notes.json` (AGENTS.md's hot-file table). |
| `scripts/tests/` (i.e. `ops/scripts/tests/`) | Shell smoke tests for the scripts above. |
| `restructure/` | `restructure.py`, the ADR 0001 move script. Re-run it on a stale branch; it deliberately still names the old paths. |

## Hazards when touching it

- **The gates read two scripts from the base commit.** `.agentctl/config.toml`
  runs `git show {base}:ops/scripts/ratchet_gate.py` and fetches
  `ops/scripts/gate_affected.sh` from `{base}`. Renaming or moving either
  breaks every gate whose base predates the change: land the config change in
  the same commit as the move, then restart the daemon.
- **The daemon caches its TOML.** A change to `.agentctl/config.toml`
  (including a hook rename) takes effect only after the daemon restarts, and
  the dashboard shows the file, not the live config.
- **agentctl keeps a mirror.** `~/projects/agentctl/consumers/gorge/config.toml`
  mirrors `.agentctl/config.toml`; agentctl's
  `tests/test_consumer_parity.py` pins the two. Change both together.
- **Units are installed as copies.** `ops/scripts/seed-install.sh install`
  copies the seed units and slices into `~/.config/systemd/user/`;
  `gorge-nightly.*` is installed by hand (see its unit header). Editing a file
  here changes nothing until it is re-installed and `systemctl --user
  daemon-reload` runs.
- **Transient units run scripts from here.** `gorge-postmerge-batch`
  (`ops/scripts/postmerge_batch.sh`), `gorge-driver-replay-batch`
  (`ops/scripts/driver_replay_batch.sh`) and `gpu-fault-watch`
  (`ops/scripts/gpu-fault-watch.sh`) are started with `systemd-run --user
  --unit=…` and keep the path they were launched with; relaunch them after a
  move. `gorge-supervisor` (agentctl's supervisor, also a `systemd-run`
  unit) runs the daemon against this checkout and needs the same relaunch.
- **Scripts find the root by path.** Most locate the repo with
  `git rev-parse --show-toplevel`; a few use `dirname "$0"` (siblings they
  source, or `/../..` to the root in `seed-ask.sh` and `start-autonomous.sh`),
  so moving one a level changes its root.
- **The demo is the operator's.** Never run `deploy-demo.sh` or bind ports
  8080-8081.
- **Post-merge red pauses the pipeline.** `postmerge_batch.sh` writes the
  pause file on red and bisects; it is what pushes `main`.
