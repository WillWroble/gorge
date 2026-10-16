"""PL-8 verdict table. Usage: pl8_compare.py [OUT] [--cards .cards]
Reads OUT/values/<arm>/values.json and OUT/gauntlet/<arm>/gate.json
(OUT default /mnt/sata/gorge-training/playlines/pl-8). Missing inputs are
skipped, so it runs on a partial result set.

Pre-registered rule (PL-8), fixed before any data:
  PURSUE       some c5 set beats the c4hlogit control by >= +1.5 pp, lower 95% bound > 0
  ABORT        every c5 set within +/-1.0 pp of control with upper bound < +1.5 pp
  INCONCLUSIVE otherwise (games needed printed)
Secondary: pilot dependence = 1 - Spearman(c5 hlogit, c4 hlogit); < 0.05 means
the pilots agree on card order and the gap is not about card strength.
Note: gate.json holds per-seat tallies, not per-event ones, so the difference
SE is the unpaired one; events share seeds (packs), so it is conservative."""
import json, math, os, re, sys
import numpy as np
from scipy import stats

args = [a for a in sys.argv[1:] if not a.startswith("--")]
OUT = args[0] if args else "/mnt/sata/gorge-training/playlines/pl-8"
CARDS = ".cards"
if "--cards" in sys.argv:
    CARDS = sys.argv[sys.argv.index("--cards") + 1]
CV = "/mnt/sata/gorge-training/cardval"
CTRL, C5 = "c4hlogit", ["c5s25hlogit", "c5s100hlogit", "c5s25gih"]


def load(p):
    return json.load(open(p)) if os.path.exists(p) else None


def valsets():
    v = {}
    for name, path, f in [("c4hlogit", f"{CV}/az-c4-s25/values.json", "hlogit"),
                          ("bot-gih", f"{CV}/hands-bot/values.json", "gih"),
                          ("c5s25hlogit", f"{OUT}/values/c5s25/values.json", "hlogit"),
                          ("c5s25gih", f"{OUT}/values/c5s25/values.json", "gih"),
                          ("c5s100hlogit", f"{OUT}/values/c5s100/values.json", "hlogit")]:
        d = load(path)
        if d:
            v[name] = {c: x[f] for c, x in d.items()}
    ref = load(f"{CV}/ref17l.json")
    if ref:
        v["17l-gih"] = {c: x["gih_wr"] for c, x in ref["cards"].items() if x["gih_wr"] is not None and x["gp_n"] >= 500}
    return v


def rho(a, b):
    ks = sorted(set(a) & set(b))
    return stats.spearmanr([a[k] for k in ks], [b[k] for k in ks]).statistic, len(ks)


def oracle(name):
    fn = re.sub(r"[^a-z0-9_]", "", name.lower().replace(" ", "_").replace("-", "_"))
    p = f"{CARDS}/cardsfolder/{fn[0]}/{fn}.txt"
    if not os.path.exists(p):
        return ""
    keep = [l.strip() for l in open(p) if l.split(":")[0] in ("ManaCost", "Types", "PT", "Oracle")]
    return " | ".join(keep)[:160]


def wilson(w, n, z=1.96):
    p = w / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return p, c - h, c + h


V = valsets()
print("== Spearman rho between value sets (n cards) ==")
for a in ("c5s25hlogit", "c5s25gih", "c5s100hlogit"):
    if a not in V:
        continue
    row = []
    for b in ("c4hlogit", "bot-gih", "17l-gih"):
        if b in V:
            r, n = rho(V[a], V[b]); row.append(f"{b} {r:.3f} (n={n})")
    print(f"{a:14s} " + "  ".join(row))
if "c5s25hlogit" in V and "c4hlogit" in V:
    r, _ = rho(V["c5s25hlogit"], V["c4hlogit"])
    dep = 1 - r
    print(f"pilot dependence 1-rho(c5s25hlogit, c4hlogit) = {dep:.3f}  ->",
          "pilots agree on card order; gap is not about card strength" if dep < 0.05 else "pilots disagree on card order")

for a in ("c5s25hlogit", "c5s100hlogit"):
    if a in V and "c4hlogit" in V:
        ks = sorted(set(V[a]) & set(V["c4hlogit"]))
        rk = lambda d: {k: i for i, k in enumerate(sorted(ks, key=lambda k: -d[k]))}
        ra, rb = rk(V[a]), rk(V["c4hlogit"])
        mv = sorted(ks, key=lambda k: rb[k] - ra[k])  # + = moved up under c5
        print(f"\n== {a}: rank moved vs c4hlogit (rank 0 = best). Add a one-line reason from the card text ==")
        for lab, lst in (("UP", mv[::-1][:20]), ("DOWN", mv[:20])):
            for k in lst:
                print(f"{lab:4s} {k:28s} c4 #{rb[k]:3d} -> c5 #{ra[k]:3d}  {oracle(k)}")

print("\n== Gauntlet (az-c5 pilot, sims 25, seeds 301-324) ==")
G = {}
for arm in [CTRL] + C5 + ["17l"]:
    g = load(f"{OUT}/gauntlet/{arm}/gate.json")
    if g:
        G[arm] = g
        t = g["BuiltPooled"]; p, lo, hi = wilson(t["Wins"], t["Games"])
        extra = ""
        if g["RandomPooled"]["Games"]:
            r = g["RandomPooled"]; extra = f"  random-deck baseline {r['WinRate']:.3f}"
        print(f"{arm:14s} {t['Wins']}-{t['Losses']} n={t['Games']} win {p:.3f} [{lo:.3f}, {hi:.3f}]{extra}")


def diff(arm):
    a, c = G[arm]["BuiltPooled"], G[CTRL]["BuiltPooled"]
    pa, pc = a["Wins"] / a["Games"], c["Wins"] / c["Games"]
    se = math.sqrt(pa * (1 - pa) / a["Games"] + pc * (1 - pc) / c["Games"])
    return pa - pc, se


verdict = "INCONCLUSIVE (no gauntlet data)"
if CTRL in G:
    pts, dd = [], {}
    print(f"\n== Difference to {CTRL} control (pp, unpaired 95%) ==")
    for arm in C5:
        if arm in G:
            d, se = diff(arm); dd[arm] = (d, se)
            print(f"{arm:14s} {100*d:+.2f} pp  [{100*(d-1.96*se):+.2f}, {100*(d+1.96*se):+.2f}]")
    if "17l" in G:
        d, se = diff("17l"); print(f"{'17l':14s} {100*d:+.2f} pp  [{100*(d-1.96*se):+.2f}, {100*(d+1.96*se):+.2f}]")
    if dd:
        pursue = [a for a, (d, se) in dd.items() if d >= 0.015 and d - 1.96 * se > 0]
        abort = all(abs(d) <= 0.010 and d + 1.96 * se < 0.015 for d, se in dd.values()) and len(dd) == len(C5)
        if pursue:
            verdict = "PURSUE (" + ", ".join(pursue) + ")"
        elif abort:
            verdict = "ABORT (saturated)"
        else:
            se = max(s for _, s in dd.values())
            n = G[CTRL]["BuiltPooled"]["Games"]
            need = int(n * (se * 1.96 / 0.0075) ** 2)  # games/arm so the 95% half-width is ~0.75 pp
            verdict = f"INCONCLUSIVE (~{need} games per arm for a +/-0.75 pp interval; have {n})"
print("\nVERDICT:", verdict)
