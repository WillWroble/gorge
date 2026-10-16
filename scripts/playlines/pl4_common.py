"""Shared pieces of the PL-4 experiment (shaped value targets, offline).

Everything here is a pure function of its inputs: no clock, no ambient randomness.
dzg (the training library) lives outside this repo; its source dir is PL4_DZG_SRC
(default /mnt/sata/gorge-training/dzg/src). `python -I` ignores PYTHONPATH, so it is
inserted into sys.path explicitly.
"""
from __future__ import annotations

import os
import sys
import zlib

DZG_SRC = os.environ.get("PL4_DZG_SRC", "/mnt/sata/gorge-training/dzg/src")
if DZG_SRC not in sys.path:
    sys.path.insert(0, DZG_SRC)

import numpy as np  # noqa: E402

# ---- fixed formulas (written down before any result; see the report) -------------------------
# Dense layout (bots/policynet/policynet.go): seat block b starts at 28 + 10*b; block 0 is the
# VIEWER's. Offsets inside a block: 0 life, 6 total battlefield power (zoneTotals' pw).
COL_LIFE0, COL_POW0, COL_LIFE1, COL_POW1 = 28, 34, 38, 44
SIGNAL_SCALE = 2.0      # S = sigmoid(SIGNAL_SCALE * raw), raw = (own-opp life)/20 + (own-opp power)/10
HORIZONS = (2, 4)       # k own-turns later = 2k global turns later (turns alternate in a 2-player game)
LAMBDAS = (0.25, 0.5)
BUCKETS = ((1, 2), (3, 4), (5, 6), (7, 10**6))   # held-out turn buckets (global turn)
BUCKET_NAMES = ("1-2", "3-4", "5-6", "7+")
SPLIT_SALT = "pl4:"     # split = crc32(SPLIT_SALT + game id) % 10: 0-7 train, 8 val, 9 test
TRAIN, VAL, TEST, NONE = 0, 1, 2, 3

# Same architecture for every head, small. policy_layers=1: the policy head is never used.
MODEL_CONFIG = {"d": 192, "ff": 512, "layers": 2, "d_emb": 64, "card_d": 128, "head_hidden": 192,
                "policy_layers": 1, "value_layers": 1, "dropout": 0.0, "emb_std": 0.05}

# head name -> (target spec, init-seed offset). Z_null is the null control: same target as Z,
# different init seed, identical batches.
HEADS = {
    "Z": ("Z", 0), "S2": ("S2", 0), "S4": ("S4", 0),
    "M2_25": ("M2_25", 0), "M2_50": ("M2_50", 0), "M4_25": ("M4_25", 0), "M4_50": ("M4_50", 0),
    "Z_null": ("Z", 1),
}
MIX_HEADS = ("M2_25", "M2_50", "M4_25", "M4_50")


def split_of(game_id: str) -> int:
    h = zlib.crc32((SPLIT_SALT + game_id).encode("utf-8")) % 10
    return TRAIN if h < 8 else (VAL if h == 8 else TEST)


def head_target(name: str, z: np.ndarray, s2: np.ndarray, s4: np.ndarray) -> np.ndarray:
    """Training target of a head. Z/S2/S4 as stored; M{k}_{pct} = (1-lambda) Z + lambda S_k."""
    spec = HEADS[name][0]
    if spec == "Z":
        return z
    if spec == "S2":
        return s2
    if spec == "S4":
        return s4
    k, pct = spec[1:].split("_")
    lam = int(pct) / 100.0
    return ((1.0 - lam) * z + lam * (s2 if k == "2" else s4)).astype(np.float32)


def shardset(paths):
    from dzg import data as D
    return D.ShardSet(list(paths), in_ram=False)   # memory-mapped; never --in-ram


def value_only_batch(ss, idx_sorted: np.ndarray):
    """Batch of the records idx (ascending global indices, so ShardSet keeps their order) with the
    options stripped: the model then returns just the value logit and skips the policy head."""
    from dzg import data as D
    p = dict(ss.take(idx_sorted, targets=False))
    B = len(idx_sorted)
    p["opt_off"] = np.zeros(B + 1, np.int64)
    p["opt_dense"] = np.zeros((0, 24), np.float32)
    p["opt_bot"] = np.zeros(0, np.uint8)
    p["opt_ea"] = np.zeros(0, np.int32)
    p["opt_eb"] = np.zeros(0, np.int32)
    for q in ("os", "oh"):
        p[f"{q}_off"] = np.zeros(1, np.int64)
        p[f"{q}_row"] = np.zeros(0, np.int64)
        p[f"{q}_val"] = np.zeros(0, np.float32)
    return D.to_batch(p, targets=False)
