"""PL-4 step 2: train the value heads (one small dzg MLP each) on the SAME batches.

Every head sees identical batches in identical order and starts from the same init seed (Z_null
from seed+1: the null control). Loss = BCE(value logit, head target) on TRAIN-split records; the
policy head is unused (options are stripped from the batch). Only the data order and the target
differ between heads. Memory-mapped shards; never --in-ram.
"""
from __future__ import annotations

import argparse
import json
import math
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pl4_common as C  # noqa: E402

import numpy as np  # noqa: E402
import torch  # noqa: E402
import torch.nn.functional as F  # noqa: E402
from dzg import models as M  # noqa: E402


def lr_at(step, peak, warmup, total, min_frac=0.1):
    if step < warmup:
        return peak * (step + 1) / warmup
    p = (step - warmup) / max(1, total - warmup)
    return peak * (min_frac + (1 - min_frac) * 0.5 * (1 + math.cos(math.pi * min(1.0, p))))


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--packs", nargs="+", required=True)
    ap.add_argument("--targets", required=True, help="pl4_build.py output .npz")
    ap.add_argument("--out", required=True)
    ap.add_argument("--steps", type=int, default=6000)
    ap.add_argument("--batch", type=int, default=512)
    ap.add_argument("--lr", type=float, default=1e-3)
    ap.add_argument("--wd", type=float, default=0.01)
    ap.add_argument("--warmup", type=int, default=200)
    ap.add_argument("--seed", type=int, default=0)
    ap.add_argument("--max-train", type=int, default=0, help="0 = every train-split record, else a seeded subsample")
    ap.add_argument("--threads", type=int, default=2)
    ap.add_argument("--device", default="cpu")
    ap.add_argument("--heads", default=",".join(C.HEADS))
    ap.add_argument("--log-every", type=int, default=50)
    a = ap.parse_args(argv)
    torch.set_num_threads(a.threads)
    dev = torch.device(a.device)
    heads = a.heads.split(",")

    T = np.load(a.targets)
    ss = C.shardset(a.packs)
    if len(ss) != len(T["z"]):
        sys.exit(f"packs hold {len(ss)} records but the targets have {len(T['z'])}")
    tr = np.nonzero(T["split"] == C.TRAIN)[0]
    if a.max_train and len(tr) > a.max_train:
        tr = np.sort(np.random.default_rng(a.seed).choice(tr, a.max_train, replace=False))
    tgt = {h: C.head_target(h, T["z"], T["s2"], T["s4"]) for h in heads}
    models, opts = {}, {}
    for h in heads:
        torch.manual_seed(a.seed + C.HEADS[h][1])
        models[h] = M.build("mlp", C.MODEL_CONFIG).to(dev).train()
        opts[h] = torch.optim.AdamW(models[h].parameters(), lr=a.lr, weight_decay=a.wd)
    os.makedirs(a.out, exist_ok=True)
    cfg = {"args": vars(a), "model_config": C.MODEL_CONFIG, "n_train": int(len(tr)), "heads": heads,
           "params": models[heads[0]].parameter_counts()}
    with open(os.path.join(a.out, "config.json"), "w") as f:
        json.dump(cfg, f, indent=1, default=str)
    print(f"pl4_train: {len(tr):,} train records, {len(heads)} heads, {a.steps} steps x {a.batch}", file=sys.stderr, flush=True)

    rng = np.random.default_rng(a.seed + 99)
    perm, pos = rng.permutation(len(tr)), 0
    t0 = time.time()
    run = {h: 0.0 for h in heads}
    curves = open(os.path.join(a.out, "curves.jsonl"), "w")
    for step in range(a.steps):
        if pos + a.batch > len(perm):
            perm, pos = rng.permutation(len(tr)), 0
        idx = np.sort(tr[perm[pos:pos + a.batch]])
        pos += a.batch
        b = C.value_only_batch(ss, idx)
        if dev.type != "cpu":
            b = b.to(dev) if hasattr(b, "to") else b
        for h in heads:
            lr = lr_at(step, a.lr, a.warmup, a.steps)
            for g in opts[h].param_groups:
                g["lr"] = lr
            _, v = models[h](b)
            y = torch.from_numpy(tgt[h][idx]).to(dev)
            loss = F.binary_cross_entropy_with_logits(v.float(), y)
            opts[h].zero_grad(set_to_none=True)
            loss.backward()
            torch.nn.utils.clip_grad_norm_(models[h].parameters(), 1.0)
            opts[h].step()
            run[h] += loss.item()
        if (step + 1) % a.log_every == 0 or step + 1 == a.steps:
            rec = {"step": step + 1, "sec": round(time.time() - t0, 1),
                   **{h: round(run[h] / a.log_every, 5) for h in heads}}
            curves.write(json.dumps(rec) + "\n")
            curves.flush()
            print(json.dumps(rec), file=sys.stderr, flush=True)
            run = {h: 0.0 for h in heads}
    for h in heads:
        torch.save({k: v.detach().cpu() for k, v in models[h].state_dict().items()}, os.path.join(a.out, f"{h}.pt"))
    print("done", round(time.time() - t0, 1), "s")


if __name__ == "__main__":
    main()
