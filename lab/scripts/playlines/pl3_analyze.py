"""PL-3 verdict table. usage: python -I pl3_analyze.py OUTDIR [--replays N] [--boot N]

Inputs (written by pl3_run.sh): <arm>.jsonl (win runs, -lines), <arm>.r<k>.jsonl (diversity replays),
base2.jsonl (null control: the baseline on another seed). Arm A is always the arm under test.

Pre-registered rule (docs/superpowers/reports/2026-10-16-pl-3-randomised-lines.md), applied literally:
  usable  = paired delta win rate >= -1.0 pp AND paired 95% lower bound on delta > -1.5 pp
            AND divergence-before-turn-4 >= baseline's own + 25 pp
  PURSUE  = some usable arm with a timing shift >= 5 pp on either row, or a split-half archetype gain >= +3 pp
  ABORT   = no arm usable, or every usable arm shows neither
  INCONCLUSIVE = (interpretive, fixed before any data) an arm passes the point-estimate tests (delta >= -1.0 pp
            and divergence) but fails only the CI bound -- the run was underpowered, not the arm refuted -- and no
            arm is PURSUE; or the data are missing.
"""
import glob, json, math, os, re, sys
from collections import Counter, defaultdict
import numpy as np

ARMS = ["base", "ex2", "ex4", "ex8", "exn", "t05", "t10", "s05", "s15"]


def load(path):
    rows = []
    with open(path) as f:
        for l in f:
            l = l.strip()
            if l:
                rows.append(json.loads(l))
    return rows


def arch(deck):  # FDN_top_19893_UB -> UB
    return deck.rsplit("_", 1)[-1]


def a_seat(r):
    return 0 if r["side"][0] == "A" else 1


def score(r):
    return 1.0 if r["result"] == "A" else 0.5 if r["result"] in ("draw", "stall", "error") else 0.0


def pair_scores(rows):
    d = defaultdict(float)
    for r in rows:
        d[r["pair"]] += score(r)
    return d  # pair -> sum over its legs


def wilson(p, n, z=1.96):
    if n == 0:
        return (float("nan"),) * 2
    d = 1 + z * z / n
    c = p + z * z / (2 * n)
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    return (c - h) / d, (c + h) / d


def paired(arm, base):
    a, b = pair_scores(arm), pair_scores(base)
    ks = sorted(set(a) & set(b))
    legs = len(arm) / max(1, len(a))
    x = np.array([(a[k] - b[k]) / legs for k in ks])
    if len(x) < 2:
        return float("nan"), float("nan"), len(x)
    return x.mean(), x.std(ddof=1) / math.sqrt(len(x)), len(x)


def timing(rows):
    s = Counter()
    for r in rows:
        if "lines" not in r or r["lines"] is None:
            continue
        t = r["lines"]["timing"][a_seat(r)]
        for k, v in t.items():
            s[k] += v
    opp = s["inst_casts_opp_turn"] / s["inst_casts"] if s["inst_casts"] else float("nan")
    resp = s["cast_opp_spell"] / s["offered_opp_spell"] if s["offered_opp_spell"] else float("nan")
    return opp, resp, s["inst_casts"], s["offered_opp_spell"]


def turn_of(lbl):
    return int(lbl[1 : lbl.index(":")])


def first_diff_turn(a, b):
    """Turn of the first differing arm choice, None if identical."""
    for i in range(min(len(a), len(b))):
        if a[i] != b[i]:
            return turn_of(a[i])
    if len(a) != len(b):
        longer = a if len(a) > len(b) else b
        return turn_of(longer[min(len(a), len(b))])
    return None


def diversity(outdir, arm, replays):
    reps = []
    for k in range(1, replays + 1):
        p = os.path.join(outdir, f"{arm}.r{k}.jsonl")
        if not os.path.exists(p):
            return None
        reps.append({r["game"]: r for r in load(p)})
    games = sorted(set.intersection(*[set(r) for r in reps]))
    if not games:
        return None
    div4, ents, outs = [], [], []
    for g in games:
        seqs = [r[g]["lines"]["seq"][a_seat(r[g])] for r in reps]
        for s in seqs[1:]:
            t = first_diff_turn(seqs[0], s)
            div4.append(t is not None and t < 4)
        c = Counter(tuple(s) for s in seqs)
        ents.append(-sum(v / len(seqs) * math.log2(v / len(seqs)) for v in c.values()))
        outs.append(len({(r[g]["result"], r[g]["turns"]) for r in reps}))
    return dict(div4=float(np.mean(div4)), ent=float(np.mean(ents)), outcomes=float(np.mean(outs)), games=len(games))


def arch_cells(rows):
    """archetype -> (even-pair scores, odd-pair scores) of the arm's own deck."""
    c = defaultdict(lambda: ([], []))
    for r in rows:
        c[arch(r["decks"][a_seat(r)])][r["pair"] % 2].append(score(r))
    return c


def split_half_gain(arm, base):
    """Pick the archetype with the largest even-half delta; score it on the odd half."""
    ca, cb = arch_cells(arm), arch_cells(base)
    best, bd = None, -9
    for k in sorted(set(ca) & set(cb)):
        if len(ca[k][0]) < 4 or len(cb[k][0]) < 4:
            continue
        d = np.mean(ca[k][0]) - np.mean(cb[k][0])
        if d > bd:
            best, bd = k, d
    if best is None or not ca[best][1] or not cb[best][1]:
        return None, float("nan"), float("nan"), 0
    return best, bd, float(np.mean(ca[best][1]) - np.mean(cb[best][1])), len(ca[best][1])


def main():
    args = sys.argv[1:]
    if not args:
        sys.exit(__doc__)
    out = args[0]
    replays = int(args[args.index("--replays") + 1]) if "--replays" in args else 8
    boot = int(args[args.index("--boot") + 1]) if "--boot" in args else 1000
    wins = {a: load(os.path.join(out, a + ".jsonl")) for a in ARMS + ["base2"] if os.path.exists(os.path.join(out, a + ".jsonl"))}
    if "base" not in wins:
        print("INCONCLUSIVE: no baseline run")
        return
    base = wins["base"]
    bt_opp, bt_resp, _, _ = timing(base)
    bdiv = diversity(out, "base", replays)
    bdiv4 = bdiv["div4"] if bdiv else float("nan")
    n = len(base)
    p = sum(map(score, base)) / n
    lo, hi = wilson(p, n)
    print(f"reference arm score vs opponent: base {p:.3f} [{lo:.3f},{hi:.3f}] n={n}; timing opp-turn share {bt_opp:.3f}, response {bt_resp:.3f}")
    print(f"baseline own divergence<T4: {bdiv4:.3f}  (should be ~0)\n")
    hdr = "arm   n    score [wilson95]        d_pp  se_pp  dlo_pp | div4  d_div_pp  ent_bits  outcomes | opp%  d_pp  resp% d_pp | arch  scored_d_pp | usable"
    print(hdr)
    usable, underpowered, pursue = [], [], []
    for a in ARMS[1:] + ["base2"]:
        if a not in wins:
            continue
        rows = wins[a]
        n = len(rows)
        p = sum(map(score, rows)) / n
        lo, hi = wilson(p, n)
        d, se, npair = paired(rows, base)
        dlo = d - 1.96 * se
        dv = diversity(out, a, replays) if a != "base2" else None
        ddiv = (dv["div4"] - bdiv4) * 100 if dv and bdiv == bdiv and bdiv else (dv["div4"] * 100 if dv else float("nan"))
        if dv and bdiv:
            ddiv = (dv["div4"] - bdiv4) * 100
        o, rsp, _, _ = timing(rows)
        sh_o, sh_r = (o - bt_opp) * 100, (rsp - bt_resp) * 100
        ar, _, sc, _ = split_half_gain(rows, base)
        ok_point = d * 100 >= -1.0 and dv is not None and ddiv >= 25
        ok_ci = dlo * 100 > -1.5
        us = ok_point and ok_ci
        if a != "base2":
            if us:
                usable.append(a)
                if max(abs(sh_o), abs(sh_r)) >= 5 or (sc == sc and sc * 100 >= 3):
                    pursue.append(a)
            elif ok_point:
                underpowered.append(a)
        f = lambda x, w=6: "  nan" if x != x else f"{x:{w}.1f}"
        print(f"{a:5s} {n:4d} {p:.3f} [{lo:.3f},{hi:.3f}] {f(d*100)} {f(se*100)} {f(dlo*100)} | "
              f"{'  nan' if not dv else f'{dv['div4']:.3f}'} {f(ddiv)} {'  nan' if not dv else f'{dv['ent']:.2f}'} {'  nan' if not dv else f'{dv['outcomes']:.2f}'} | "
              f"{o*100:5.1f} {f(sh_o)} {rsp*100:5.1f} {f(sh_r)} | {ar} {f(sc*100 if sc==sc else sc)} | {'yes' if us else 'no'}")
    # null control: what the archetype split-half selection "finds" between two identical arms
    if "base2" in wins:
        ar, ev, sc, _ = split_half_gain(wins["base2"], base)
        print(f"\nnull control base2 vs base (identical arms, seed differs): selected {ar}, even-half gain {ev*100:+.1f} pp, scored odd-half {sc*100:+.1f} pp (selection bias to subtract)")
        d2, se2, _ = paired(wins["base2"], base)
        print(f"  base2 paired delta {d2*100:+.1f} pp (pairs differ across seeds, so this is a spread, not a paired SE)")
    print("\nper-arm split-half archetype detail (arm, archetype chosen on even pairs, odd-pair delta pp, bootstrap-over-odd-games 95%):")
    rng = np.random.default_rng(0)
    for a in ARMS[1:]:
        if a not in wins:
            continue
        ar, ev, sc, nn = split_half_gain(wins[a], base)
        if ar is None:
            print(f"  {a}: no archetype with enough games")
            continue
        ca, cb = arch_cells(wins[a])[ar][1], arch_cells(base)[ar][1]
        bs = [rng.choice(ca, len(ca)).mean() - rng.choice(cb, len(cb)).mean() for _ in range(boot)]
        print(f"  {a}: {ar} even {ev*100:+.1f} -> odd {sc*100:+.1f} pp [{np.percentile(bs,2.5)*100:+.1f},{np.percentile(bs,97.5)*100:+.1f}] (n_odd={nn})")
    print()
    if pursue:
        print(f"VERDICT: PURSUE (usable arm(s) with a timing shift or archetype gain: {', '.join(pursue)})")
    elif not usable and underpowered:
        print(f"VERDICT: INCONCLUSIVE (point estimates pass but the CI bound fails -- underpowered: {', '.join(underpowered)})")
    elif not usable:
        print("VERDICT: ABORT (no arm usable)")
    else:
        print(f"VERDICT: ABORT (usable arm(s) {', '.join(usable)} leave timing unchanged and give no archetype win)")


if __name__ == "__main__":
    main()
