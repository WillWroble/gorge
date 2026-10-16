"""PL-8: fit gih/iwd/hlogit/deck values from a cardval run directory, writing
values.json next to games.jsonl. Reuses hand_values/regression_values from
/mnt/sata/gorge-training/cardval/analyze.py unchanged (its source is exec'd
without the trailing main() call), so the numbers are comparable to the
az-c4-s25 control. Also prints split-half reliability.
Usage: pl8_fit.py <run dir>"""
import json, sys
import numpy as np
from scipy import stats

SRC = "/mnt/sata/gorge-training/cardval/analyze.py"
src = open(SRC).read()
assert src.rstrip().endswith("main()")
ns = {"__name__": "analyze"}
exec(compile(src.rstrip()[: -len("main()")], SRC, "exec"), ns)

run = sys.argv[1]
cards = json.load(open(f"{run}/cards.json"))
rows = []
for l in open(f"{run}/games.jsonl"):
    try:
        rows.append(json.loads(l))
    except json.JSONDecodeError:
        pass
K = len(cards)
print(f"{run}: {len(rows)} games, {K} cards")
hv = ns["hand_values"](rows, K)
dv = ns["regression_values"](rows, K)
va = ns["hand_values"](rows[0::2], K)
vb = ns["hand_values"](rows[1::2], K)
for k in hv:
    print(f"split-half {k} {stats.spearmanr(va[k], vb[k]).statistic:.3f}")
json.dump({c["name"]: {"deck": float(dv[i]), **{k: float(hv[k][i]) for k in hv}} for i, c in enumerate(cards)},
          open(f"{run}/values.json", "w"), indent=1)
print(f"wrote {run}/values.json")
