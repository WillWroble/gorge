#!/usr/bin/env python3
"""Encoding ablation harness (2026-10-10, docs/superpowers/reports/2026-10-10-encoding-ablation.md).

Reads a cmd/encablate dump and trains a small bag-of-tokens policy+value net:

  state h  = MLP(EmbeddingBag(state tokens [+ identity tokens]))
  cand  c  = EmbeddingBag(candidate option tokens [+ option identity tokens])
  policy   = softmax_k  MLP([h, c_k, h*c_k])        (target: teacher_choice)
  value    = sigmoid(w.h)                            (target: game outcome)
  opphand  = sigmoid(W.h) over card names            (optional auxiliary head)

Modes (--mode):
  ablate   train with per-family input dropout (p=--famdrop), then evaluate
           held-out loss with each family removed; paired bootstrap CI over
           held-out GAMES.
  retrain  train with --drop FAM[,FAM] removed entirely (confirmation).
  opphand  arms: base / +aux opp-hand head / opp-hand as input (diag ceiling).
  decks    held-out-deck split; identity = none | name | ir | name+ir.
Results are appended as one JSON line per run to --results.
"""
import argparse, gzip, json, math, random, sys, time, zlib
import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F

p = argparse.ArgumentParser()
p.add_argument("--data", required=True)
p.add_argument("--mode", default="ablate")
p.add_argument("--seeds", default="1,2,3")
p.add_argument("--epochs", type=int, default=6)
p.add_argument("--dim", type=int, default=64)
p.add_argument("--famdrop", type=float, default=0.15)
p.add_argument("--drop", default="")
p.add_argument("--identity", default="name")
p.add_argument("--holdout", default="")
p.add_argument("--exclude-src", default="c1.labels")
p.add_argument("--only-diag", action="store_true")
p.add_argument("--results", required=True)
p.add_argument("--tag", default="")
p.add_argument("--aux-weight", type=float, default=0.5)
p.add_argument("--min-count", type=int, default=3)
a = p.parse_args()
dev = torch.device("cuda" if torch.cuda.is_available() else "cpu")

voc = json.load(open(a.data + ".vocab.json"))
TOK, FAMS = voc["tokens"], voc["families"]
IR = json.load(open(a.data + ".ir.json"))

recs = []
for line in gzip.open(a.data):
    r = json.loads(line)
    if a.exclude_src and a.exclude_src in r["src"]:
        continue
    if a.only_diag and not r.get("diag"):
        continue
    recs.append(r)
print(f"{len(recs)} records", file=sys.stderr)

# identity families: the mzenc walker's card-name families plus our Names bag.
NAME_FAMS = {i for i, f in enumerate(FAMS) if f.endswith(".name") or f in ("Stack", "Exile")}
FAM_IDENT = len(FAMS)      # pseudo family: zone|name identity bag
FAM_IR = len(FAMS) + 1     # pseudo family: zone|IR-bag identity
FAM_OH = len(FAMS) + 2     # pseudo family: opponent hand (diagnostic input)
ALLF = FAMS + ["Ident.Names", "Ident.IR", "Diag.OppHand"]

# second vocabulary for synthetic tokens
xv = {}
def xt(s):
    if s not in xv:
        xv[s] = len(TOK) + len(xv)
    return xv[s]

def ir_tokens(zone, name):
    return [xt(f"IR|{zone}|{t}") for t in IR.get(name, ["?"])]

def build(r, identity, opphand_input):
    fam = list(r["fam"]); tok = list(r["tok"])
    if identity in ("none", "ir"):
        keep = [i for i, f in enumerate(fam) if f not in NAME_FAMS]
        fam = [fam[i] for i in keep]; tok = [tok[i] for i in keep]
    if identity in ("name", "name+ir"):
        for t in r["names"]:
            fam.append(FAM_IDENT); tok.append(t)
    if identity in ("ir", "name+ir"):
        for t in r["names"]:
            s = TOK[t].split("|")  # N|side|zone|name or N|stack|name
            zone = "|".join(s[1:-1]); name = s[-1]
            for x in ir_tokens(zone, name):
                fam.append(FAM_IR); tok.append(x)
    if opphand_input and r.get("diag"):
        for n in r.get("opphand") or []:
            fam.append(FAM_OH); tok.append(xt("OH|" + n))
            for x in ir_tokens("oh", n):
                fam.append(FAM_OH); tok.append(x)
    cands = []
    for ct, cn in zip(r["cands"], r["cnames"] or [None] * len(r["cands"])):
        cn = cn or []
        c = list(ct or [])
        for t in cn:
            nm = TOK[t].split("|")
            if identity in ("name", "name+ir"):
                c.append(t)
            if identity in ("ir", "name+ir"):
                c += [xt(f"OIR|{nm[1]}|{x}") for x in IR.get(nm[2], ["?"])]
        cands.append(c)
    return fam, tok, cands

def gsplit(g, k=5):
    return zlib.crc32(g.encode()) % k == 0

def make(identity, opphand_input, test_fn, train_fn):
    tr, te = [], []
    for r in recs:
        if test_fn(r):
            te.append(r)
        elif train_fn(r):
            tr.append(r)
    return tr, te

# card-name label space for the opp-hand head
OHN = sorted({n for r in recs for n in (r.get("opphand") or [])})
OHI = {n: i for i, n in enumerate(OHN)}

class Net(nn.Module):
    def __init__(s, V, d, nh):
        super().__init__()
        s.E = nn.EmbeddingBag(V, d, mode="sum", padding_idx=0)
        s.C = nn.EmbeddingBag(V, d, mode="sum", padding_idx=0)
        s.st = nn.Sequential(nn.LayerNorm(d), nn.Linear(d, 2 * d), nn.ReLU(), nn.Linear(2 * d, d), nn.ReLU())
        s.cn = nn.LayerNorm(d)
        s.sc = nn.Sequential(nn.Linear(3 * d, d), nn.ReLU(), nn.Linear(d, 2))
        s.v = nn.Linear(d, 1)
        s.oh = nn.Linear(d, max(nh, 1))
    def forward(s, st, st_off, ca, ca_off, cand_owner):
        h = s.st(s.E(st, st_off))
        c = s.cn(s.C(ca, ca_off))
        hh = h[cand_owner]
        o = s.sc(torch.cat([hh, c, hh * c], 1))
        return h, o[:, 0], o[:, 1], s.v(h).squeeze(1), s.oh(h)

def remap(examples, vocab_map, train):
    """compress token ids to a dense id space (0 = pad/unknown)."""
    out = []
    for fam, tok, cands, r in examples:
        if train:
            for t in tok:
                vocab_map[t] = vocab_map.get(t, 0) + 1
            for c in cands:
                for t in c:
                    vocab_map[t] = vocab_map.get(t, 0) + 1
        out.append((fam, tok, cands, r))
    return out

def to_ids(examples, idmap):
    res = []
    for fam, tok, cands, r in examples:
        f = np.array(fam, dtype=np.int64); t = np.array([idmap.get(x, 0) for x in tok], dtype=np.int64)
        m = t > 0
        cs = [np.array([idmap.get(x, 0) for x in c], dtype=np.int64) for c in cands]
        oh = np.zeros(len(OHN), dtype=np.float32)
        for n in r.get("opphand") or []:
            oh[OHI[n]] = 1
        res.append(dict(cv=[min(max(x, 0.0), 1.0) for x in (r.get("cv") or [0.0] * len(cs))], f=f[m], t=t[m], c=cs, y=r["y"], ok=r["ok"], tc=r["t"], b=r["b"], kind=r["kind"],
                        game=r["game"], diag=bool(r.get("diag")), oh=oh))
    return res

def batchify(B, drop_mask_fn):
    st, st_off, ca, ca_off, own, tgt, start = [], [], [], [], [], [], []
    o = 0; oc = 0
    for i, e in enumerate(B):
        f, t = e["f"], e["t"]
        keep = drop_mask_fn(f)
        t = t[keep] if keep is not None else t
        st_off.append(o); st.append(t); o += len(t)
        start.append(len(own))
        for c in e["c"]:
            ca_off.append(oc); ca.append(c); oc += len(c); own.append(i)
        tgt.append(start[-1] + e["tc"])
    cat = lambda xs: torch.from_numpy(np.concatenate(xs) if xs else np.zeros(0, np.int64)).to(dev)
    return (cat(st), torch.tensor(st_off, device=dev), cat(ca), torch.tensor(ca_off, device=dev),
            torch.tensor(own, device=dev), torch.tensor(start + [len(own)], device=dev), torch.tensor(tgt, device=dev))

def seg_logsoftmax(logit, start):
    out = torch.empty_like(logit)
    n = len(start) - 1
    # loop-free: per-group max via scatter
    own = torch.repeat_interleave(torch.arange(n, device=dev), start[1:] - start[:-1])
    mx = torch.full((n,), -1e9, device=dev).scatter_reduce(0, own, logit, "amax")
    z = torch.exp(logit - mx[own])
    s = torch.zeros(n, device=dev).index_add(0, own, z)
    return logit - mx[own] - torch.log(s[own]), own

def run_model(net, B, drop_fn):
    st, so, ca, co, own, start, tgt = batchify(B, drop_fn)
    h, logit, q, v, oh = net(st, so, ca, co, own)
    cv = torch.tensor([x for e in B for x in e["cv"]], device=dev, dtype=torch.float32)
    ql = torch.zeros(len(B), device=dev).index_add(0, own, (torch.sigmoid(q) - cv) ** 2) / (start[1:] - start[:-1]).float()
    lsm, ownv = seg_logsoftmax(logit, start)
    pl = -lsm[tgt]
    # argmax per group
    n = len(B)
    mx = torch.full((n,), -1e9, device=dev).scatter_reduce(0, ownv, logit, "amax")
    pred_hit = (logit[tgt] >= mx - 1e-6).float()
    y = torch.tensor([e["y"] for e in B], device=dev, dtype=torch.float32)
    okm = torch.tensor([e["ok"] for e in B], device=dev, dtype=torch.float32)
    vl = F.binary_cross_entropy_with_logits(v, y, reduction="none") * okm
    vacc = (((v > 0).float() == (y > 0.5).float()).float()) * okm
    ohy = torch.from_numpy(np.stack([e["oh"] for e in B])).to(dev)
    dg = torch.tensor([e["diag"] for e in B], device=dev, dtype=torch.float32)
    ohl = F.binary_cross_entropy_with_logits(oh[:, :ohy.shape[1]], ohy, reduction="none").sum(1) * dg if ohy.shape[1] else torch.zeros(n, device=dev)
    return pl, pred_hit, vl, vacc, okm, ohl, ql

def train(tr, V, seed, drop_fams=(), famdrop=0.0, aux=0.0, epochs=None):
    torch.manual_seed(seed); random.seed(seed); np.random.seed(seed)
    net = Net(V, a.dim, len(OHN)).to(dev)
    opt = torch.optim.AdamW(net.parameters(), lr=2e-3, weight_decay=1e-4)
    dset = np.array(sorted(drop_fams), dtype=np.int64)
    rng = np.random.default_rng(seed)
    nf = len(ALLF)
    for ep in range(epochs or a.epochs):
        idx = list(range(len(tr))); random.shuffle(idx)
        for i in range(0, len(idx), 256):
            B = [tr[j] for j in idx[i:i + 256]]
            dropped = rng.random((len(B), nf)) < famdrop if famdrop > 0 else None
            k = [0]
            def dfn(f, _B=B):
                j = k[0]; k[0] += 1
                m = ~np.isin(f, dset) if len(dset) else np.ones(len(f), bool)
                if dropped is not None:
                    m &= ~dropped[j][f]
                return m
            pl, _, vl, _, okm, ohl, ql = run_model(net, B, dfn)
            loss = pl.mean() + ql.mean() + vl.sum() / okm.sum().clamp(min=1) + aux * ohl.mean()
            opt.zero_grad(); loss.backward(); opt.step()
    return net

@torch.no_grad()
def evaluate(net, te, drop_fams=()):
    net.eval()
    dset = np.array(sorted(drop_fams), dtype=np.int64)
    out = {k: [] for k in ("pl", "hit", "vl", "vacc", "ok", "ohl", "ql")}
    for i in range(0, len(te), 1024):
        B = te[i:i + 1024]
        dfn = (lambda f: ~np.isin(f, dset)) if len(dset) else (lambda f: None)
        r = run_model(net, B, dfn)
        for k, x in zip(out, r):
            out[k].append(x.cpu().numpy())
    net.train()
    return {k: np.concatenate(v) for k, v in out.items()}

def summary(ev, te):
    ov = np.array([e["tc"] != e["b"] for e in te])
    ok = ev["ok"] > 0
    return dict(pol_ce=float(ev["pl"].mean()), pol_acc=float(ev["hit"].mean()),
                ovr_acc=float(ev["hit"][ov].mean()) if ov.any() else None,
                ovr_ce=float(ev["pl"][ov].mean()) if ov.any() else None, q_mse=float(ev["ql"].mean()),
                val_bce=float(ev["vl"][ok].mean()), val_acc=float(ev["vacc"][ok].mean()),
                ohl=float(ev["ohl"].mean()), n=len(te), n_ovr=int(ov.sum()))

def boot_ci(delta, games, B=1000, seed=0):
    """paired bootstrap over games of mean(delta)."""
    ug, inv = np.unique(games, return_inverse=True)
    sums = np.bincount(inv, weights=delta); cnt = np.bincount(inv)
    rng = np.random.default_rng(seed); ms = []
    for _ in range(B):
        s = rng.integers(0, len(ug), len(ug))
        ms.append(sums[s].sum() / cnt[s].sum())
    return float(np.percentile(ms, 2.5)), float(np.percentile(ms, 97.5))

def emit(d):
    d["tag"] = a.tag; d["mode"] = a.mode
    with open(a.results, "a") as fh:
        fh.write(json.dumps(d) + "\n")
    print(json.dumps(d), flush=True)

def prepare(identity, opphand_input, test_fn, train_fn):
    tr, te = make(identity, opphand_input, test_fn, train_fn)
    trx = [build(r, identity, opphand_input) + (r,) for r in tr]
    tex = [build(r, identity, opphand_input) + (r,) for r in te]
    cnt = {}
    remap(trx, cnt, True)
    global idmap
    idmap = {}
    for t, c in cnt.items():
        if c >= a.min_count:
            idmap[t] = len(idmap) + 1
    return to_ids(trx, idmap), to_ids(tex, idmap), len(idmap) + 1

seeds = [int(s) for s in a.seeds.split(",")]
t0 = time.time()
if a.mode in ("ablate", "retrain"):
    tr, te, V = prepare(a.identity, False, lambda r: gsplit(r["game"]), lambda r: True)
    print(f"train {len(tr)} test {len(te)} vocab {len(idmap)}", file=sys.stderr)
    games = np.array([e["game"] for e in te])
    okm = np.array([e["ok"] for e in te], bool)
    present = sorted({int(f) for e in tr for f in np.unique(e["f"])})
    drop = tuple(ALLF.index(x) for x in a.drop.split(",") if x) if a.drop else ()
    for sd in seeds:
        net = train(tr, V, sd, drop_fams=drop, famdrop=a.famdrop if a.mode == "ablate" else 0.0)
        base = evaluate(net, te, drop)
        emit(dict(seed=sd, family="<none>", drop=a.drop, **summary(base, te), secs=time.time() - t0))
        if a.mode != "ablate":
            continue
        for f in present:
            ev = evaluate(net, te, drop + (f,))
            s = summary(ev, te)
            dpl = ev["pl"] - base["pl"]; dhit = ev["hit"] - base["hit"]
            dvl = (ev["vl"] - base["vl"])[okm]
            emit(dict(seed=sd, family=ALLF[f], d_pol_ce=float(dpl.mean()), d_pol_ce_ci=boot_ci(dpl, games, 300, sd),
                      d_pol_acc=float(dhit.mean()), d_q_mse=float((ev["ql"] - base["ql"]).mean()),
                      d_q_mse_ci=boot_ci(ev["ql"] - base["ql"], games, 300, sd),
                      d_ovr_ce=float(dpl[np.array([e["tc"] != e["b"] for e in te])].mean()), d_val_bce=float(dvl.mean()),
                      d_val_bce_ci=boot_ci(dvl, games[okm], 300, sd), d_val_acc=float((ev["vacc"] - base["vacc"])[okm].mean()),
                      **s))
elif a.mode == "keepone":
    tr, te, V = prepare(a.identity, False, lambda r: gsplit(r["game"]), lambda r: True)
    games = np.array([e["game"] for e in te]); okm = np.array([e["ok"] for e in te], bool)
    present = sorted({int(f) for e in tr for f in np.unique(e["f"])})
    for sd in seeds:
        empty = evaluate(net := train(tr, V, sd, drop_fams=tuple(present)), te, tuple(present))
        emit(dict(seed=sd, family="<empty>", **summary(empty, te)))
        for f in present + ["<all>"]:
            drop = tuple(x for x in present if x != f) if f != "<all>" else ()
            net = train(tr, V, sd, drop_fams=drop)
            ev = evaluate(net, te, drop)
            dvl = (ev["vl"] - empty["vl"])[okm]; dq = ev["ql"] - empty["ql"]
            emit(dict(seed=sd, family=ALLF[f] if f != "<all>" else f, g_val_bce=float(-dvl.mean()), g_val_bce_ci=boot_ci(-dvl, games[okm], 300, sd),
                      g_q_mse=float(-dq.mean()), g_q_mse_ci=boot_ci(-dq, games, 300, sd),
                      g_pol_ce=float((empty["pl"] - ev["pl"]).mean()), **summary(ev, te)))
elif a.mode == "opphand":
    arms = {"base": (prepare(a.identity, False, lambda r: gsplit(r["game"]), lambda r: True), 0.0)}
    arms["aux"] = (arms["base"][0], a.aux_weight)
    arms["input"] = (prepare(a.identity, True, lambda r: gsplit(r["game"]), lambda r: True), 0.0)
    te0 = arms["base"][0][1]
    games = np.array([e["game"] for e in te0]); ok = np.array([e["ok"] for e in te0], bool)
    for sd in seeds:
        res = {}
        for arm, ((TR, TE, V), aux) in arms.items():
            net = train(TR, V, sd, aux=aux)
            ev = evaluate(net, TE); res[arm] = ev
            emit(dict(seed=sd, arm=arm, **summary(ev, TE)))
        for arm in ("aux", "input"):
            d = res[arm]["pl"] - res["base"]["pl"]
            dv = (res[arm]["vl"] - res["base"]["vl"])[ok]
            emit(dict(seed=sd, arm=arm + "-vs-base", d_pol_ce=float(d.mean()), d_pol_ce_ci=boot_ci(d, games, 300, sd),
                      d_pol_acc=float((res[arm]["hit"] - res["base"]["hit"]).mean()),
                      d_val_bce=float(dv.mean()), d_val_bce_ci=boot_ci(dv, games[ok], 300, sd)))
elif a.mode == "decks":
    hold = set(a.holdout.split(","))
    unseen = lambda r: r["deck"] in hold
    clean = lambda r: r["deck"] not in hold and r["odeck"] not in hold
    for ident in a.identity.split(","):
        tr, ten, V = prepare(ident, False, lambda r: clean(r) and gsplit(r["game"]), clean)
        teu = to_ids([build(r, ident, False) + (r,) for r in recs if unseen(r)], idmap)
        for sd in seeds:
            net = train(tr, V, sd)
            emit(dict(seed=sd, identity=ident, split="seen-decks", **summary(evaluate(net, ten), ten)))
            emit(dict(seed=sd, identity=ident, split="unseen-decks", **summary(evaluate(net, teu), teu)))
print(f"done {time.time()-t0:.0f}s", file=sys.stderr)
