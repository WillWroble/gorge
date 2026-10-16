#!/usr/bin/env python3
"""perf-record.py: turn one enginebench-pair.sh JSONL into schema-1 perf history rows.

    perf-record.py --anchor SHA --commit SHA --host ID PAIR.jsonl > rows.jsonl

PAIR.jsonl is what scripts/enginebench-pair.sh ANCHOR COMMIT writes: one
enginebench result line per run, labelled "base" (the anchor build) and
"cand" (the commit being recorded), paired by "rep" within each (row, pair).
Lines with "err" set, or any other label, are skipped.

One output row per scenario x metric (stdlib only, deterministic in its input
apart from the host fields read from this machine):

  scenario          metric              unit           higher_is_better
  random/<pair>     turns_per_cpu_s     turns/cpu-s    true
  bot/<pair>        turns_per_cpu_s     turns/cpu-s    true   (manual taps)
  bot-autopay/<p>   turns_per_cpu_s     turns/cpu-s    true
  clone|step        us_per_op_cpu_gc    us/op          false  (GC paid)
  clone|step        alloc_bytes_per_op  B/op           false
  az<sims>/<pair>   sims_per_cpu_s      sims/cpu-s     true
  sampler/<pair>    sims_per_cpu_s      sims/cpu-s     true
  corpus            heap_live_mb        MB             false  (every run's
                    post-load live heap, paired per (row, pair, rep))

Turns per CPU-second (not wall) because a loaded shared box makes wall time
include waiting for a core; this matches enginebench-summarize.py.

ratio_to_anchor is the median over reps of cand/base from the SAME rep (so a
load swing hitting one rep hits both sides), kept RAW: it is NOT inverted for
lower-is-better metrics. Read it with higher_is_better: for a throughput
metric ratio > 1 is an improvement, for us/op, B/op and MB ratio < 1 is.
"value" and "anchor_value" are the medians of the cand and base runs; "ratios"
lists the per-rep ratios in rep order and "reps" is their count.
"""
import argparse
import collections
import datetime
import hashlib
import json
import os
import platform
import statistics
import subprocess
import sys

SCHEMA = 1


def _turns(r):
    d = r.get("detail") or {}
    t = d.get("Turns") if isinstance(d, dict) else None
    if t is None:
        t = r.get("turns")
    return t


def _per_cpu(n, r):
    cpu = r.get("cpu_secs")
    if not n or not cpu:
        return None
    return n / cpu


def metrics_for(r):
    """[(scenario, metric, unit, higher_is_better, value)] for one result line."""
    row, pair = r.get("row", ""), r.get("pair", "")
    scen = f"{row}/{pair}" if pair else row
    out = []
    if row == "random" or row.startswith("bot"):
        v = _per_cpu(_turns(r), r)
        if v is not None:
            out.append((scen, "turns_per_cpu_s", "turns/cpu-s", True, v))
    elif row in ("clone", "step"):
        # The clone/step rows use the A+B roots whatever -pair says.
        if r.get("us_per_op_cpu_gc"):
            out.append((row, "us_per_op_cpu_gc", "us/op", False, r["us_per_op_cpu_gc"]))
        if r.get("alloc_bytes_per_op") is not None:
            out.append((row, "alloc_bytes_per_op", "B/op", False, r["alloc_bytes_per_op"]))
    elif row.startswith("az") or row == "sampler":
        if r.get("sims_per_cpu_s"):
            out.append((scen, "sims_per_cpu_s", "sims/cpu-s", True, r["sims_per_cpu_s"]))
    if r.get("heap_live_mb"):
        out.append(("corpus", "heap_live_mb", "MB", False, r["heap_live_mb"]))
    return out


def _git(*args):
    try:
        return subprocess.run(["git", *args], capture_output=True, text=True, check=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return ""


def _sha256(path):
    if not path:
        return ""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _first(records, key):
    for r in records:
        if r.get(key) not in (None, ""):
            return r[key]
    return ""


def local_host():
    return {"threads": os.cpu_count() or 0, "kernel": platform.release()}


def build_rows(records, anchor, commit, host_id, commit_date="", bin_sha256="", host_local=None):
    """The schema-1 rows for a pair run's parsed records (a list of dicts)."""
    if host_local is None:
        host_local = local_host()
    ok = [r for r in records if not r.get("err") and r.get("label") in ("base", "cand")]
    cand = [r for r in ok if r["label"] == "cand"]
    # (scenario, metric) -> pairing key -> label -> value
    vals = collections.defaultdict(lambda: collections.defaultdict(dict))
    meta = {}
    secs = collections.defaultdict(list)
    for r in ok:
        for scen, metric, unit, hib, v in metrics_for(r):
            k = (scen, metric)
            meta[k] = (unit, hib)
            pk = (r.get("row", ""), r.get("pair", ""), r.get("rep", 0))
            vals[k][pk][r["label"]] = v
            if r["label"] == "cand" and r.get("secs"):
                secs[k].append(r["secs"])
    unix = max((r.get("unix", 0) for r in ok), default=0)
    run_date = datetime.datetime.fromtimestamp(unix, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ") if unix else ""
    loads = [r["load1"] for r in ok if r.get("load1") is not None]
    host = {
        "id": host_id,
        "cpu": _first(cand, "cpu_model"),
        "threads": host_local.get("threads", 0),
        "kernel": host_local.get("kernel", ""),
        "go": _first(cand, "go_version"),
        "gomaxprocs": _first(cand, "gomaxprocs") or 0,
        "pin": _first(cand, "pin"),
        "gogc": _first(cand, "gogc") or "default",
        "load1_med": round(statistics.median(loads), 2) if loads else 0,
    }
    rows = []
    for k in sorted(vals):
        scen, metric = k
        unit, hib = meta[k]
        paired = vals[k]
        ratios, cs, bs = [], [], []
        for pk in sorted(paired, key=lambda p: (str(p[0]), str(p[1]), p[2])):
            lv = paired[pk]
            if "cand" in lv:
                cs.append(lv["cand"])
            if "base" in lv:
                bs.append(lv["base"])
            if "cand" in lv and "base" in lv and lv["base"]:
                ratios.append(lv["cand"] / lv["base"])
        if not ratios:
            continue
        rows.append({
            "schema": SCHEMA,
            "commit": commit,
            "commit_date": commit_date,
            "run_date": run_date,
            "anchor": anchor,
            "scenario": scen,
            "metric": metric,
            "unit": unit,
            "higher_is_better": hib,
            "value": statistics.median(cs),
            "anchor_value": statistics.median(bs),
            "ratio_to_anchor": statistics.median(ratios),
            "reps": len(ratios),
            "ratios": ratios,
            "host": host,
            "harness": {
                "tool": "enginebench-pair",
                "secs": round(statistics.median(secs[k]), 3) if secs[k] else 0,
                "bin_sha256": bin_sha256,
            },
        })
    return rows


def check_commit(records, commit, out=sys.stderr):
    """Warn when a cand line's recorded commit disagrees with --commit."""
    for r in records:
        c = r.get("commit") if r.get("label") == "cand" else None
        if c and not (c.startswith(commit) or commit.startswith(c)):
            print(f"perf-record: warning: cand line records commit {c}, --commit is {commit}", file=out)
            return False
    return True


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("pair_jsonl")
    ap.add_argument("--anchor", required=True, help="anchor (base) commit SHA")
    ap.add_argument("--commit", required=True, help="recorded (cand) commit SHA")
    ap.add_argument("--host", required=True, help="stable host id for the history")
    ap.add_argument("--commit-date", default=None, help="commit date (default: git show -s --format=%%cI COMMIT, else empty)")
    ap.add_argument("--cand-bin", default="", help="cand enginebench binary, hashed into harness.bin_sha256")
    a = ap.parse_args(argv)
    records = []
    with open(a.pair_jsonl) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    check_commit(records, a.commit)
    cd = a.commit_date if a.commit_date is not None else _git("show", "-s", "--format=%cI", a.commit)
    rows = build_rows(records, a.anchor, a.commit, a.host, cd, _sha256(a.cand_bin))
    if not rows:
        print("perf-record: no paired base/cand runs in input", file=sys.stderr)
        return 1
    for row in rows:
        print(json.dumps(row, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
