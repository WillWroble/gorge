#!/usr/bin/env python3
"""Aggregate ablate.py --mode keepone: per family, the held-out GAIN over the
empty-state model when trained with that family alone (x1000, mean over seeds,
mean bootstrap CI)."""
import json, sys, collections
import numpy as np
rows = [json.loads(l) for l in open(sys.argv[1]) if '"keepone"' in l]
emp = [r for r in rows if r["family"] == "<empty>"]
print("empty-state: " + ", ".join(f"{k}={np.mean([r[k] for r in emp]):.4f}" for k in ("pol_ce", "q_mse", "val_bce", "val_acc")))
g = collections.defaultdict(list)
for r in rows:
    if r["family"] != "<empty>":
        g[r["family"]].append(r)
def f(rs, k):
    m = np.mean([r[k] for r in rs]) * 1000
    lo = np.mean([r[k + "_ci"][0] for r in rs]) * 1000; hi = np.mean([r[k + "_ci"][1] for r in rs]) * 1000
    return f"{m:+.1f} [{lo:+.1f},{hi:+.1f}]" + ("*" if lo > 0 else "")
print("| family alone | value BCE gain ×1e3 [CI] | value acc | cand-Q MSE gain ×1e3 [CI] | policy CE gain ×1e3 |")
print("|---|---|---|---|---|")
for k, rs in sorted(g.items(), key=lambda kv: -np.mean([r["g_val_bce"] for r in kv[1]])):
    print(f"| {k} | {f(rs,'g_val_bce')} | {np.mean([r['val_acc'] for r in rs])*100:.1f}% | {f(rs,'g_q_mse')} | {np.mean([r['g_pol_ce'] for r in rs])*1000:+.1f} |")
