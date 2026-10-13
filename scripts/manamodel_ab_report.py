#!/usr/bin/env python3 -I
"""Tables for the Config.PaidManaCensus A/B (scripts/manamodel_ab.sh output).

    python3 -I scripts/manamodel_ab_report.py OUT [-json summary.json] > report-tables.md

Per job file OUT/<set>-<run>-<off|on>.jsonl (dzgorge play -mm-stats games).
"""
import collections, json, math, os, sys

def load(path):
    out = {}
    with open(path) as f:
        for line in f:
            g = json.loads(line)
            out[(g["pair"], g["leg"])] = g
    return out

def score(r):
    return {"A": 1.0, "B": 0.0}.get(r, 0.5)

def mean_ci(xs):
    n = len(xs)
    m = sum(xs) / n
    if n < 2:
        return m, 0.0
    v = sum((x - m) ** 2 for x in xs) / (n - 1)
    return m, 1.96 * math.sqrt(v / n)

def side_stats(games):
    """policy -> Counter of summed mm counters, plus 'games' and per-game flags."""
    agg = collections.defaultdict(collections.Counter)
    for g in games.values():
        for s in (0, 1):
            pol = g["pols"][s]
            mm = g.get("mm") or [None, None]
            mm = mm[s] or {}
            c = agg[pol]
            c["games"] += 1
            for k, v in mm.items():
                c[k] += v
            if mm.get("new:windows", 0):
                c["games_with_new"] += 1
            if mm.get("new:chosen", 0):
                c["games_with_chosen"] += 1
    return agg

def pct(a, b):
    return f"{100.0 * a / b:.2f}%" if b else "-"

def main():
    out = sys.argv[1]
    js = sys.argv[sys.argv.index("-json") + 1] if "-json" in sys.argv else None
    summary = {}
    names = ["fdn", "cons"]
    runs = ["r1", "r2", "r3"]
    print("### Win rate: flag on vs off (A = the first policy; paired by deck pair, seed and leg)\n")
    print("| set | run | A vs B | games | A score off | A score on | delta (on-off) | 95% CI | games identical | A-seat result flipped |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for s in names:
        for r in runs:
            po, pn = f"{out}/{s}-{r}-off.jsonl", f"{out}/{s}-{r}-on.jsonl"
            if not (os.path.exists(po) and os.path.exists(pn)):
                continue
            off, on = load(po), load(pn)
            keys = sorted(set(off) & set(on))
            pairs = collections.defaultdict(list)
            same = flipped = 0
            for k in keys:
                a, b = off[k], on[k]
                pairs[k[0]].append(score(b["result"]) - score(a["result"]))
                if (a["result"], a["turns"], a["intents"]) == (b["result"], b["turns"], b["intents"]):
                    same += 1
                if a["result"] != b["result"]:
                    flipped += 1
            d = [sum(v) / len(v) for v in pairs.values()]
            m, ci = mean_ci(d)
            so = sum(score(off[k]["result"]) for k in keys) / len(keys)
            sn = sum(score(on[k]["result"]) for k in keys) / len(keys)
            ga = off[keys[0]]
            ab = f"`{[p for p, sd in zip(ga['pols'], ga['side']) if sd == 'A'][0]}` vs `{[p for p, sd in zip(ga['pols'], ga['side']) if sd == 'B'][0]}`"
            print(f"| {s} | {r} | {ab} | {len(keys)} | {so:.3f} | {sn:.3f} | {m:+.3f} | ±{ci:.3f} | {pct(same, len(keys))} | {pct(flipped, len(keys))} |")
            summary[f"{s}-{r}"] = {"games": len(keys), "score_off": so, "score_on": sn, "delta": m, "ci95": ci,
                                   "identical_games": same, "flipped": flipped,
                                   "errors": sum(1 for k in keys for g in (off[k], on[k]) if g["result"] == "error"),
                                   "stalls": sum(1 for k in keys for g in (off[k], on[k]) if g["result"] == "stall")}
    print("\n### Exposure and choice per policy (new option = offered with the flag on, not with it off, same state)\n")
    print("| set | policy | flag run | games | priority windows | windows with a new option | of which own main, empty stack | new options / window-with-new | windows chosen new | chosen / windows-with-new | games with a new option | games with a new option chosen | probe mismatches |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    vis = []
    for s in names:
        for flag in ("off", "on"):
            tot = collections.defaultdict(collections.Counter)
            for r in runs:
                p = f"{out}/{s}-{r}-{flag}.jsonl"
                if not os.path.exists(p):
                    continue
                for pol, c in side_stats(load(p)).items():
                    tot[pol].update(c)
            for pol in sorted(tot):
                c = tot[pol]
                w, nw = c["windows"], c["new:windows"]
                kinds = ", ".join(f"{k[len('new:kind:'):]}={v}" for k, v in sorted(c.items()) if k.startswith("new:kind:"))
                print(f"| {s} | `{pol}` | {flag} | {c['games']} | {w} | {nw} ({pct(nw, w)}) | {c['new:ownmain:windows']} | "
                      f"{c['new:options'] / nw:.2f} | {c['new:chosen']} | {pct(c['new:chosen'], nw) if flag == 'on' else 'n/a'} | "
                      f"{c['games_with_new']} ({pct(c['games_with_new'], c['games'])}) | "
                      f"{c['games_with_chosen']} ({pct(c['games_with_chosen'], c['games'])}) | {c['probe:mismatch']} |")
                summary.setdefault("policy", {})[f"{s}|{pol}|{flag}"] = dict(c)
                vis.append((s, pol, flag, c))
    print("\n### Offered-option visibility at priority windows (pass, concede and mana taps excluded)\n")
    print("| set | policy | flag | windows | mean list | single-option windows | empty-list windows | own-main windows | own-main mean list | own-main single | own-main empty |")
    print("|---|---|---|---|---|---|---|---|---|---|---|")
    for s, pol, flag, c in vis:
        w, o = c["windows"], c["ownmain:windows"]
        print(f"| {s} | `{pol}` | {flag} | {w} | {c['list:all:sum'] / w:.3f} | {pct(c['list:all:1'], w)} | {pct(c['list:all:0'], w)} | "
              f"{o} | {c['ownmain:list:all:sum'] / max(o, 1):.3f} | {pct(c['ownmain:list:all:1'], o)} | {pct(c['ownmain:list:all:0'], o)} |")
    if js:
        with open(js, "w") as f:
            json.dump(summary, f, indent=1, sort_keys=True)

main()
