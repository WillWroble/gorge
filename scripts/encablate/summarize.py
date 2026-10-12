#!/usr/bin/env python3
"""Aggregate ablate.py --mode ablate results: per family, mean over seeds of
the held-out deltas (x1000) with the mean bootstrap CI, sorted by value delta."""
import json, sys, collections
import numpy as np
tag = sys.argv[2] if len(sys.argv) > 2 else None
rows = [json.loads(l) for l in open(sys.argv[1])]
rows = [r for r in rows if r.get("mode") == "ablate" and (tag is None or r.get("tag") == tag)]
base = [r for r in rows if r["family"] == "<none>"]
print("base: " + ", ".join(f"{k}={np.mean([r[k] for r in base]):.4f}±{np.std([r[k] for r in base]):.4f}"
      for k in ("pol_ce", "pol_acc", "ovr_ce", "q_mse", "val_bce", "val_acc")) + f" (seeds={len(base)}, n={base[0]['n']})")
g = collections.defaultdict(list)
for r in rows:
    if r["family"] != "<none>":
        g[r["family"]].append(r)
def fmt(rs, k):
    m = np.mean([r[k] for r in rs]) * 1000
    if k + "_ci" in rs[0]:
        lo = np.mean([r[k + "_ci"][0] for r in rs]) * 1000; hi = np.mean([r[k + "_ci"][1] for r in rs]) * 1000
        star = "*" if lo > 0 else ("-" if hi < 0 else "")
        return f"{m:+.2f} [{lo:+.2f},{hi:+.2f}]{star}"
    return f"{m:+.2f}"
print("| family | Δ value BCE ×1e3 [95% CI] | Δ value acc (pp) | Δ cand-Q MSE ×1e3 [CI] | Δ policy CE ×1e3 [CI] | Δ override CE ×1e3 |")
print("|---|---|---|---|---|---|")
for f, rs in sorted(g.items(), key=lambda kv: -np.mean([r["d_val_bce"] for r in kv[1]])):
    print(f"| {f} | {fmt(rs,'d_val_bce')} | {np.mean([r['d_val_acc'] for r in rs])*100:+.2f} | {fmt(rs,'d_q_mse')} | {fmt(rs,'d_pol_ce')} | {fmt(rs,'d_ovr_ce')} |")
