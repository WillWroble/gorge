#!/usr/bin/env python3
"""Render cmd/manamodelstats JSON results as the report's markdown tables.

    python3 scripts/manamodel_report.py <runs-dir> [family ...]

Reads <runs-dir>/<family>-auto.json and <family>-manual.json for each family
(default: fdn constructed commander) and prints markdown to stdout.
"""
import json
import os
import sys

CLASSES = ["Sac", "Life", "ManaFee", "SubCounter", "TapOther", "Discard", "Exile", "AddCounter", "OtherCost",
           "Free:Restricted", "Free:InstantSpeed", "Free:Amount", "Free:Choice", "Free:Tier"]


def load(d, fam, mode):
    p = os.path.join(d, f"{fam}-{mode}.json")
    if not os.path.exists(p):
        return None
    return json.load(open(p))


def pct(a, b):
    return "-" if not b else f"{100.0 * a / b:.1f}%"


def g(c, k):
    return c.get(k, 0)


def headline(rows):
    print("| family | games | priority windows | L1 windows (W) | L1 windows (P) | L2-sampled windows | L2-confirmed windows (W) | L2-confirmed windows (P) | games with >=1 confirmed (W) | games with >=1 confirmed (P) |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        gm, w = g(c, "games"), g(c, "windows")
        print(f"| {fam} | {gm} | {w} | {g(c,'L1W_windows')} ({pct(g(c,'L1W_windows'), w)}) | {g(c,'L1P_windows')} ({pct(g(c,'L1P_windows'), w)}) "
              f"| {g(c,'L2_sampled_windows')} | {g(c,'L2W_real_windows')} ({pct(g(c,'L2W_real_windows'), w)}) "
              f"| {g(c,'L2P_real_windows')} ({pct(g(c,'L2P_real_windows'), w)}) "
              f"| {g(c,'games_with:L2W')} ({pct(g(c,'games_with:L2W'), gm)}) | {g(c,'games_with:L2P')} ({pct(g(c,'games_with:L2P'), gm)}) |")


def candidates(rows):
    print("| family | gap | L1 casts | L2 real | L2 false (proxy over-count) | L2 unknown | precision real/(real+false) |")
    print("|---|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        for gap in ("W", "P"):
            r, f, u = g(c, f"L2{gap}:real"), g(c, f"L2{gap}:false"), g(c, f"L2{gap}:unknown")
            print(f"| {fam} | {gap} | {g(c, f'cand:L1{gap}')} | {r} | {f} | {u} | {pct(r, r + f)} |")


def classes(rows, gap):
    print(f"| family | class | L1 casts | L2 real | L2 false | L2 unknown |")
    print("|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        keys = sorted({k.split(":class:")[1] for k in c if k.startswith(f"cand:L1{gap}:class:")},
                      key=lambda k: -g(c, f"cand:L1{gap}:class:{k}"))
        for k in keys:
            print(f"| {fam} | {k} | {g(c, f'cand:L1{gap}:class:{k}')} | {g(c, f'L2{gap}:real:class:{k}')} "
                  f"| {g(c, f'L2{gap}:false:class:{k}')} | {g(c, f'L2{gap}:unknown:class:{k}')} |")


def answers(rows):
    print("| family | L2-confirmed (P) windows | seat answered: payment (a plan) | activate (manual fallback) | cast (pool) | pass | other |")
    print("|---|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        tot = g(c, "L2P_real_windows")
        known = {"payment", "activate", "cast", "pass"}
        other = sum(v for k, v in c.items() if k.startswith("L2P_real_windows:answer:") and k.split(":")[-1] not in known)
        print(f"| {fam} | {tot} | {g(c,'L2P_real_windows:answer:payment')} | {g(c,'L2P_real_windows:answer:activate')} "
              f"| {g(c,'L2P_real_windows:answer:cast')} | {g(c,'L2P_real_windows:answer:pass')} | {other} |")


def lists(d, fams):
    print("| family | mode | scope | windows | 0 options | exactly 1 | 2+ | mean list length |")
    print("|---|---|---|---|---|---|---|---|")
    for fam in fams:
        for mode in ("auto", "manual"):
            doc = load(d, fam, mode)
            if doc is None:
                continue
            c = doc["counts"]
            for scope, pre, wk in (("all priority", "", "windows"), ("own main, empty stack", "ownmain:", "ownmain:windows")):
                for kind in ("spells",):
                    w = g(c, wk)
                    z, o, t = g(c, f"{pre}list:{kind}:0"), g(c, f"{pre}list:{kind}:1"), g(c, f"{pre}list:{kind}:2+")
                    s = g(c, f"{pre}list:{kind}:sum")
                    print(f"| {fam} | {mode} | {scope} | {w} | {pct(z, w)} | {pct(o, w)} | {pct(t, w)} | {s / w if w else 0:.2f} |")


def lists_with_lands(d, fams):
    print("| family | mode | scope | exactly 1 | mean (incl. land plays) |")
    print("|---|---|---|---|---|")
    for fam in fams:
        for mode in ("auto", "manual"):
            doc = load(d, fam, mode)
            if doc is None:
                continue
            c = doc["counts"]
            for scope, pre, wk in (("all priority", "", "windows"), ("own main, empty stack", "ownmain:", "ownmain:windows")):
                w = g(c, wk)
                print(f"| {fam} | {mode} | {scope} | {pct(g(c, f'{pre}list:all:1'), w)} | {g(c, f'{pre}list:all:sum') / w if w else 0:.2f} |")


def decks(rows):
    print("| family | deck | priority windows | L1 windows (P) | L2-confirmed windows (P) | L2-confirmed windows (W) |")
    print("|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        names = sorted({k.split(":")[1] for k in c if k.startswith("deck:")})
        names.sort(key=lambda n: (-g(c, f"deck:{n}:L2P_real_windows"), -g(c, f"deck:{n}:L2W_real_windows")))
        for n in names:
            if not g(c, f"deck:{n}:L2P_real_windows") and not g(c, f"deck:{n}:L2W_real_windows"):
                continue
            print(f"| {fam} | {n} | {g(c, f'deck:{n}:windows')} | {g(c, f'deck:{n}:L1P_windows')} "
                  f"| {g(c, f'deck:{n}:L2P_real_windows')} | {g(c, f'deck:{n}:L2W_real_windows')} |")


def misc(rows):
    print("| family | stalls | big-board windows skipped | windows with an unknown L1 search | windows where P pays but no plan is offered | L2 false with a trigger on the stack | L2 panics | L2 real on the guided witness |")
    print("|---|---|---|---|---|---|---|---|")
    for fam, doc in rows:
        c = doc["counts"]
        st = sum(v for k, v in c.items() if k.startswith("stall:"))
        print(f"| {fam} | {st} | {g(c,'skipped_big_board')} | {g(c,'windows_unknown')} | {g(c,'windows_payP_unplanned')} "
              f"| {g(c,'L2_false_stack_grew')} | {g(c,'L2_panics')} | {g(c,'L2_real_guided')} |")


def main():
    d = sys.argv[1]
    fams = sys.argv[2:] or ["fdn", "constructed", "commander"]
    rows = [(f, load(d, f, "auto")) for f in fams]
    rows = [(f, doc) for f, doc in rows if doc is not None]
    for title, fn in (("Headline", lambda: headline(rows)), ("Casts", lambda: candidates(rows)),
                      ("P gap by class", lambda: classes(rows, "P")), ("W gap by class", lambda: classes(rows, "W")),
                      ("What the auto-pay seat did in a P-confirmed window", lambda: answers(rows)),
                      ("Option lists (casts and abilities)", lambda: lists(d, fams)),
                      ("Option lists including land plays", lambda: lists_with_lands(d, fams)),
                      ("Per deck", lambda: decks(rows)), ("Run health", lambda: misc(rows))):
        print(f"### {title}\n")
        fn()
        print()


if __name__ == "__main__":
    main()
