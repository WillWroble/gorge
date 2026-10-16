"""PL-4 step 3: score every head against the held-out FINAL OUTCOME Z and print the verdict.

Per turn bucket (1-2, 3-4, 5-6, 7+, plus 1-4): log loss and Brier of sigmoid(value logit) vs Z (soft
labels 1/0.5/0), bootstrap 95% CIs resampling GAMES, relative gain vs the Z-trained head from the
SAME resamples, and a per-turn calibration slope. Selection (which Mix head) is made on the VAL
split and scored on the TEST split. Z_null (identical target, other init seed) is the noise floor.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import zlib

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pl4_common as C  # noqa: E402

import numpy as np  # noqa: E402
import torch  # noqa: E402
from dzg import models as M  # noqa: E402

PURSUE_EARLY, PURSUE_LATE_MAX = 0.02, 0.005
ABORT_EARLY, ABORT_LATE = 0.005, 0.01
BOOT = 1000


def predict(model, ss, idx, chunk=2048):
    out = np.empty(len(idx), np.float32)
    with torch.no_grad():
        for s in range(0, len(idx), chunk):
            sub = idx[s:s + chunk]
            _, v = model(C.value_only_batch(ss, sub))
            out[s:s + chunk] = v.float().numpy()
    return out


def bce(x, z):   # stable, soft labels
    return np.maximum(x, 0) - z * x + np.log1p(np.exp(-np.abs(x)))


def bucket_ids(turn):
    b = np.full(len(turn), -1, np.int64)
    for i, (lo, hi) in enumerate(C.BUCKETS):
        b[(turn >= lo) & (turn <= hi)] = i
    return b


def game_sums(game, bk, vals):
    """[G, nb] per-game sums of vals per bucket, and counts."""
    G = game.max() + 1
    nb = len(C.BUCKETS)
    s = np.zeros((G, nb))
    n = np.zeros((G, nb))
    m = bk >= 0
    np.add.at(s, (game[m], bk[m]), vals[m])
    np.add.at(n, (game[m], bk[m]), 1.0)
    return s, n


def with_early(a):   # append the 1-4 column
    return np.concatenate([a, a[:, :2].sum(1, keepdims=True)], 1)


def calib_slope(x, z, iters=10):
    """slope b of logistic z ~ a + b*x (soft labels), Newton with a tiny ridge."""
    if len(x) < 50 or x.std() < 1e-6:
        return float("nan")
    w = np.zeros(2)
    X = np.stack([np.ones_like(x), x], 1)
    for _ in range(iters):
        p = 1 / (1 + np.exp(-(X @ w)))
        g = X.T @ (p - z) + 1e-6 * w
        H = (X * (p * (1 - p))[:, None]).T @ X + 1e-6 * np.eye(2)
        w -= np.linalg.solve(H, g)
    return float(w[1])


def evaluate(split_idx, logits, T, heads):
    """per head: loss/brier means per bucket (+1-4) with bootstrap resamples. Returns dict."""
    game = np.unique(T["game"][split_idx], return_inverse=True)[1]
    bk = bucket_ids(T["turn"][split_idx])
    z = T["z"][split_idx]
    cols = list(C.BUCKET_NAMES) + ["1-4"]
    stats = {}
    for h in heads:
        x = logits[h]
        p = 1 / (1 + np.exp(-x))
        ls, n = game_sums(game, bk, bce(x, z))
        bs, _ = game_sums(game, bk, (p - z) ** 2)
        stats[h] = (with_early(ls), with_early(bs))
    n = with_early(n)
    G = game.max() + 1
    rng = np.random.default_rng(7)
    boots = {h: [] for h in heads}
    bootb = {h: [] for h in heads}
    for _ in range(BOOT // 100):
        W = rng.multinomial(G, np.full(G, 1.0 / G), size=100).astype(np.float64)
        den = W @ n
        for h in heads:
            boots[h].append((W @ stats[h][0]) / den)
            bootb[h].append((W @ stats[h][1]) / den)
    res = {"cols": cols, "n_records": int(len(z)), "n_games": int(G), "heads": {}}
    for h in heads:
        L = np.concatenate(boots[h])
        Bs = np.concatenate(bootb[h])
        point = stats[h][0].sum(0) / n.sum(0)
        pointb = stats[h][1].sum(0) / n.sum(0)
        res["heads"][h] = {"logloss": point.tolist(), "brier": pointb.tolist(),
                           "logloss_boot": L, "brier_boot": Bs}
    return res


def rel_gain(res, h, base="Z"):
    """(point, lo, hi) relative gain (base-h)/base per column, bootstrap on shared resamples."""
    a, b = res["heads"][h], res["heads"][base]
    pt = (np.array(b["logloss"]) - np.array(a["logloss"])) / np.array(b["logloss"])
    bt = (b["logloss_boot"] - a["logloss_boot"]) / b["logloss_boot"]
    return pt, np.percentile(bt, 2.5, axis=0), np.percentile(bt, 97.5, axis=0)


def verdict(val, test, mix_heads):
    cols = val["cols"]
    ie, il = cols.index("1-4"), cols.index("7+")
    sel = max(mix_heads, key=lambda h: rel_gain(val, h)[0][ie])
    pt, lo, hi = rel_gain(test, sel)
    early, early_lo, late_cost = pt[ie], lo[ie], -pt[il]
    if early >= PURSUE_EARLY and early_lo > 0 and late_cost <= PURSUE_LATE_MAX:
        v = "PURSUE"
    elif early < ABORT_EARLY or late_cost >= ABORT_LATE:
        v = "ABORT"
    else:
        v = "INCONCLUSIVE"
    return {"verdict": v, "selected_on_val": sel, "test_early_gain": float(early), "test_early_gain_lo95": float(early_lo),
            "test_late_cost": float(late_cost)}


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--packs", nargs="+", required=True)
    ap.add_argument("--targets", required=True)
    ap.add_argument("--models", required=True, help="pl4_train.py --out dir")
    ap.add_argument("--out", required=True, help="results json")
    ap.add_argument("--game-stride", type=int, default=1,
                    help="evaluate every Nth game (crc32 of its id) of val/test; 1 = all")
    ap.add_argument("--threads", type=int, default=2)
    a = ap.parse_args(argv)
    torch.set_num_threads(a.threads)
    T = np.load(a.targets)
    gids = T["gids"]
    keep = np.array([zlib.crc32(("pl4e:" + g).encode()) % a.game_stride == 0 for g in gids])[T["game"]]
    ss = C.shardset(a.packs)
    if len(ss) != len(T["z"]):
        sys.exit("packs and targets disagree on the record count")
    heads = [h for h in C.HEADS if os.path.exists(os.path.join(a.models, f"{h}.pt"))]
    sets = {"val": np.nonzero((T["split"] == C.VAL) & keep)[0], "test": np.nonzero((T["split"] == C.TEST) & keep)[0]}
    res, logits = {}, {}
    for name, idx in sets.items():
        logits[name] = {}
        for h in heads:
            m = M.build("mlp", C.MODEL_CONFIG)
            m.load_state_dict(torch.load(os.path.join(a.models, f"{h}.pt"), weights_only=True))
            logits[name][h] = predict(m.eval(), ss, idx)
        # evaluation arrays must be restricted to this split
        sub = {k: T[k][idx] for k in ("z", "turn", "game")}
        res[name] = evaluate(np.arange(len(idx)), logits[name], sub, heads)
    mix = [h for h in C.MIX_HEADS if h in heads]
    out = {"heads": heads, "game_stride": a.game_stride}
    for name in sets:
        r = res[name]
        out[name] = {"n_records": r["n_records"], "n_games": r["n_games"], "cols": r["cols"], "heads": {}}
        print(f"\n== {name}: {r['n_records']:,} records, {r['n_games']:,} games ==")
        print(f"{'head':8s} " + " ".join(f"{c:>9s}" for c in r["cols"]) + "   (log loss vs Z)")
        for h in heads:
            hh = r["heads"][h]
            gp = rel_gain(r, h)
            out[name]["heads"][h] = {"logloss": hh["logloss"], "brier": hh["brier"],
                                     "gain_vs_Z": gp[0].tolist(), "gain_lo95": gp[1].tolist(), "gain_hi95": gp[2].tolist()}
            print(f"{h:8s} " + " ".join(f"{v:9.4f}" for v in hh["logloss"]))
        print(f"{'head':8s} " + " ".join(f"{c:>9s}" for c in r["cols"]) + "   (Brier)")
        for h in heads:
            print(f"{h:8s} " + " ".join(f"{v:9.4f}" for v in r["heads"][h]["brier"]))
        print("relative log-loss gain vs Z, point [lo95, hi95]:")
        for h in heads:
            if h == "Z":
                continue
            pt, lo, hi = rel_gain(r, h)
            print(f"{h:8s} " + " ".join(f"{p*100:+6.2f}[{l*100:+.2f},{u*100:+.2f}]" for p, l, u in zip(pt, lo, hi)))
    if mix and "Z" in heads and "Z_null" in heads:
        v = verdict(res["val"], res["test"], mix)
        v["null_control_test_early_gain"] = float(rel_gain(res["test"], "Z_null")[0][res["test"]["cols"].index("1-4")])
        out["verdict"] = v
        print("\nVERDICT", json.dumps(v, indent=1))
    # per-turn calibration slope on TEST
    idx = sets["test"]
    t = T["turn"][idx]
    z = T["z"][idx]
    out["calibration_slope_test"] = {}
    print("\ncalibration slope per turn (TEST; 1 = calibrated):")
    turns = list(range(1, 13)) + [13]
    print(f"{'head':8s} " + " ".join(f"{('t'+str(x)+('+' if x == 13 else '')):>6s}" for x in turns))
    for h in heads:
        row = []
        for tt in turns:
            m = (t >= tt) if tt == 13 else (t == tt)
            row.append(calib_slope(logits["test"][h][m], z[m]))
        out["calibration_slope_test"][h] = row
        print(f"{h:8s} " + " ".join(f"{v:6.2f}" for v in row))
    with open(a.out, "w") as f:
        json.dump(out, f, indent=1)


if __name__ == "__main__":
    main()
