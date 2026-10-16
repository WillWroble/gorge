"""PL-4 step 1: packed visit corpora -> per-record target arrays (Z, S2, S4), turn, game, split.

Input: one `dzgorge pack` output dir per corpus (c1..c6), in a fixed order; the global record index
is dzg.ShardSet's (dirs in order). Only small columns are read (game, turn, seat, outcome, 4 dense
columns); no pack is loaded into RAM.

  raw_view(r)  = (life_viewer - life_other)/20 + (power_viewer - power_other)/10   (dense 28,38,34,44)
  S_k(i)       = sigmoid(2 * raw)  of the first record j of the SAME game with turn_j >= turn_i + 2k,
                 expressed from record i's seat (sign flipped when seat_j != seat_i);
                 if the game has no such record (it ended first) S_k(i) = Z(i).
  Z(i)         = the record's outcome (1 / 0.5 / 0); records with unknown outcome are dropped (split NONE).
"""
from __future__ import annotations

import argparse
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pl4_common as C  # noqa: E402

import numpy as np  # noqa: E402
from dzg import packfmt as pf  # noqa: E402

TURN_CAP = 1000


def sigmoid(x):
    return 1.0 / (1.0 + np.exp(-x))


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--packs", nargs="+", required=True, help="pack dirs, one per corpus, in order")
    ap.add_argument("--out", required=True, help="targets .npz")
    a = ap.parse_args(argv)

    gk, turn, seat, z, vec, dirs = [], [], [], [], [], []
    gid_ix: dict[tuple[int, str], int] = {}
    gids: list[str] = []
    for ci, path in enumerate(a.packs):
        for d in pf.shard_dirs(path):
            pack, meta = pf.read_shard(d)
            local = np.empty(len(meta["games"]), np.int64)
            for li, g in enumerate(meta["games"]):
                key = (ci, g)
                if key not in gid_ix:
                    gid_ix[key] = len(gids)
                    gids.append(g)
                local[li] = gid_ix[key]
            gk.append(local[np.asarray(pack["game"])])
            turn.append(np.asarray(pack["turn"]).astype(np.int64))
            seat.append(np.asarray(pack["seat"]).astype(np.int64))
            z.append(np.asarray(pack["outcome"]).astype(np.float32))
            dn = np.asarray(pack["dense"])
            vec.append(np.stack([dn[:, c] for c in (C.COL_LIFE0, C.COL_POW0, C.COL_LIFE1, C.COL_POW1)], 1)
                       .astype(np.float32))
            dirs.append(str(d))
    gk, turn, seat = np.concatenate(gk), np.concatenate(turn), np.concatenate(seat)
    z, vec = np.concatenate(z), np.concatenate(vec)
    n = len(gk)
    if turn.max() >= TURN_CAP:
        sys.exit(f"turn {turn.max()} >= {TURN_CAP}")
    raw = (vec[:, 0] - vec[:, 2]) / 20.0 + (vec[:, 1] - vec[:, 3]) / 10.0   # viewer's perspective

    # sorted key (game, turn); stable so ties keep file order
    key = gk * TURN_CAP + turn
    order = np.argsort(key, kind="stable")
    skey = key[order]
    out = {}
    stats = {"n_records": int(n), "n_games": len(gids), "unknown_outcome": int((z < 0).sum())}
    for k in C.HORIZONS:
        want = key + 2 * k
        pos = np.searchsorted(skey, want, side="left")
        ok = pos < n
        pos_c = np.minimum(pos, n - 1)
        j = order[pos_c]
        ok &= gk[j] == gk
        sign = np.where(seat[j] == seat, 1.0, -1.0)
        s = sigmoid(C.SIGNAL_SCALE * sign * raw[j])
        zz = np.where(z >= 0, z, 0.5)
        out[f"s{k}"] = np.where(ok, s, zz).astype(np.float32)
        gap = (turn[j] - (turn + 2 * k))[ok]
        stats[f"s{k}_terminal_fraction"] = float(1 - ok.mean())
        stats[f"s{k}_gap_hist(0,1,2,3+)"] = [int((gap == g).sum()) for g in range(3)] + [int((gap >= 3).sum())]
    split = np.array([C.split_of(g) for g in gids], np.int8)[gk]
    split[z < 0] = C.NONE
    stats["split_counts(train,val,test,none)"] = [int((split == s).sum()) for s in range(4)]
    stats["z_mean"] = float(z[z >= 0].mean())
    for k in C.HORIZONS:
        stats[f"s{k}_mean/std"] = [float(out[f"s{k}"][z >= 0].mean()), float(out[f"s{k}"][z >= 0].std())]
        stats[f"corr(s{k},z)"] = float(np.corrcoef(out[f"s{k}"][z >= 0], z[z >= 0])[0, 1])
    os.makedirs(os.path.dirname(os.path.abspath(a.out)), exist_ok=True)
    np.savez(a.out, z=z, s2=out["s2"], s4=out["s4"], turn=turn.astype(np.int32), game=gk.astype(np.int32),
             seat=seat.astype(np.int8), split=split, dirs=np.array(dirs), gids=np.array(gids))
    print(json.dumps(stats, indent=1))
    with open(a.out + ".stats.json", "w") as f:
        json.dump(stats, f, indent=1)


if __name__ == "__main__":
    main()
