# PL-4: shaped (short/medium-term) value targets, tested offline

Status: experiment BUILT and smoke-tested; **Results and Verdict are empty until the orchestrator runs the full sweep.**

## Question
The value head is trained on the final game outcome only. Would a medium-horizon target (a blend of the outcome
with a state-derived signal k own-turns ahead) make the value head predict the final outcome better EARLY in the
game, without hurting late-game accuracy? Decided offline: no search, no new self-play generation.

## Method
Data: the az self-play visit corpora `/mnt/sata/gorge-training/aztrain/cloop-census/c1..c6.visits.jsonl.gz`
(`azvisits-v1`, `dzgorge play -record-features entity`). Verified: each record carries `game_id`, `seat`, `turn`
(global game turn, both players' turns count), `sequence`, `outcome`/`outcome_known` (the recording seat's result
1 / 0.5 / 0) and the encoded state, whose dense block holds both players' life and battlefield power
(`internal/policynet/policynet.go` seat blocks: base 28 = viewer, 38 = opponent; offset 0 life, 6 total
battlefield power). So no fresh corpus is needed. Records are searched decisions, not every turn, so "k turns
later" is looked up by turn number (below).

Pipeline (all new files under `scripts/playlines/`, nothing else changed; dzg is outside the repo and is used
as an unmodified library, so no `--value-target` flag was added to it):

1. `dzgorge pack -in cN.visits.jsonl.gz -out PACKS/cN` per corpus (existing tool, streaming shards). One pack dir
   per corpus because game ids (`p0000000g0`) repeat across corpora.
2. `pl4_build.py`: reads only small columns of the shards (game, turn, seat, outcome, 4 dense columns; mmap)
   and writes Z, S2, S4, turn, game, split per record.
3. `pl4_train.py`: trains 8 small dzg `MLPNet`s (`dzg.models.build("mlp", ...)`, value head only: options are
   stripped from the batch) on IDENTICAL batches in identical order, same init seed, same steps/lr/schedule;
   only the target differs. Shards stay memory-mapped (never `--in-ram`).
4. `pl4_eval.py`: scores every head against the held-out FINAL OUTCOME Z, prints tables and the verdict.

### Fixed formulas (written before any result was produced; do not edit after the full run)
- `Z` = the record's outcome (1 / 0.5 / 0). Records with unknown outcome are dropped.
- `raw_view(r) = (life_viewer - life_other)/20 + (power_viewer - power_other)/10` from dense columns 28, 38 (life)
  and 34, 44 (battlefield power).
- `S_k(i) = sigmoid(2 * raw)` evaluated at the first record j of the SAME game with `turn_j >= turn_i + 2k`
  (k own-turns later = 2k global turns later in a 2-player game), expressed from record i's seat
  (raw sign flipped if `seat_j != seat_i`). If the game has no such record (it ended first) `S_k(i) = Z(i)`.
  k in {2, 4}. The logistic scale 2 maps the target into [0,1] for BCE; it was fixed, not tuned.
- `Mix(lambda) = (1-lambda) Z + lambda S_k`, lambda in {0.25, 0.5}: heads `M2_25 M2_50 M4_25 M4_50`.
  Reference heads `S2`, `S4` (pure signal) are reported but are not eligible for the verdict.
  `Z_null` = Z target, init seed + 1, same batches: the noise floor / null control.
- Model: `dzg` mlp with config `{d:192, ff:512, layers:2, d_emb:64, card_d:128, head_hidden:192, policy_layers:1,
  value_layers:1, dropout:0}`; AdamW lr 1e-3, wd 0.01, 200 warmup steps then cosine to 0.1x, grad clip 1.0,
  batch 512, 6000 steps (~1.1 epoch of the train split), CPU fp32, seed 0.
- Split by GAME ID, never record: `crc32("pl4:" + game_id) % 10` -> 0-7 train (80%), 8 val (10%), 9 test (10%).
  The salted id is shared across corpora c1..c6 so a replayed deck/seed pair never straddles train and test.
- Evaluation: held-out target is always Z (soft labels 0 / 0.5 / 1). Log loss and Brier per global-turn bucket
  1-2, 3-4, 5-6, 7+ (and 1-4 combined). 95% CIs: bootstrap over GAMES (1000 resamples, rng seed 7), the relative
  gain vs the Z head is computed on the same resamples. Per-turn calibration slope = slope of logistic
  `z ~ a + b * logit(p)` on the TEST split, turns 1..12 and 13+.
- Selection (which Mix head) is made on the VAL split (largest turns 1-4 relative gain) and SCORED on TEST.
  Never scored on the data that selected it. `Z_null`'s turns 1-4 test gain is printed as the noise floor
  (advisory only; it does not change the verdict).

## Pre-registered rule (copied verbatim from the ticket)
- PURSUE if some Mix model lowers held-out Z log loss on turns 1-4 by >= 2% relative with the
  bootstrap lower bound > 0 AND does not raise turns 7+ log loss by more than 0.5% relative.
  PURSUE means: next ticket trains one az generation with that target and gates it against c5
  (600 paired games); it does NOT mean adopting it.
- ABORT if no Mix model improves turns 1-4 by >= 0.5% relative, or every improvement costs >= 1%
  on turns 7+.
- INCONCLUSIVE otherwise.

Caveat: better outcome prediction is necessary, not sufficient, for better play; the gate in the next ticket is
the real test.

Implementation of the rule (`pl4_eval.py: verdict`): with the split-half protocol, "some Mix model" is the Mix
head selected on VAL, judged on TEST with: early = relative gain on turns 1-4; late cost = relative loss increase on
turns 7+. PURSUE if early >= 2% and its bootstrap 95% lower bound > 0 and late cost <= 0.5%; else ABORT if early
< 0.5% or late cost >= 1%; else INCONCLUSIVE. Thresholds are point estimates; only the PURSUE lower bound uses the CI.

## Run commands
Inputs on the corpora (counted with `zcat | wc -l`): c1 777,838; c2 494,113; c3 562,420; c4 524,846; c5 523,750;
c6 608,750 = **3,491,717 records**. Packed size is ~7.2 KB/record (measured on the smoke: 143 MB / 20k) -> ~25 GB of
shards. Only the smoke was run by the seat; the wall times below for the full run are estimates scaled from it.

Build the binary from this worktree (orchestrator; `/mnt/sata` was read-only in the seat sandbox):

    O=/mnt/sata/gorge-training/playlines/pl-4; mkdir -p $O
    go build -o $O/dzgorge ./cmd/dzgorge
    for c in c1 c2 c3 c4 c5 c6; do
      $O/dzgorge pack -in /mnt/sata/gorge-training/aztrain/cloop-census/$c.visits.jsonl.gz -out $O/packs/$c -shard 200000
    done
    PY=/mnt/sata/gorge-training/dzg/venv/bin/python     # numpy + torch; no scipy/pandas needed
    P="$O/packs/c1 $O/packs/c2 $O/packs/c3 $O/packs/c4 $O/packs/c5 $O/packs/c6"
    $PY -I scripts/playlines/pl4_build.py --packs $P --out $O/targets.npz
    $PY -I scripts/playlines/pl4_train.py --packs $P --targets $O/targets.npz --out $O/models --steps 6000 --batch 512 --threads 8
    $PY -I scripts/playlines/pl4_eval.py  --packs $P --targets $O/targets.npz --models $O/models --out $O/results.json --threads 8 | tee $O/eval.txt

(run under `scripts/heavy.sh` and a memory scope; the scripts do not read the clock or any ambient randomness.)
Expected, from the smoke: `pl4_build` reads ~4 small columns per record (minutes, <2 GB RSS); `pl4_train` ~4 steps/s
at batch 256 for all 8 heads on 2 threads (~0.5 s/step at batch 512 -> ~50 min for 6000 steps, more or less with
threads), RSS under 1 GB plus page cache of the mmapped shards; `pl4_eval` scores ~0.35M val + ~0.35M test records x 8
heads (~10 min). `--game-stride N` thins val/test by game if needed.
Training uses `--device cpu` by default (the GPU is held by vLLM); `--device cuda` is untested.

## Smoke (seat, 20,000 records of evaldecks.visits.jsonl.gz, 2 threads, 100 steps x 256)
Exercises build -> train -> eval end to end (build 1 s, train 25 s, eval 6 s, 750 MB RSS). Its numbers carry no
evidential weight (34 test games, 100 steps) and are not reproduced here beyond the fact that all steps ran and
the verdict logic fired (ABORT on that tiny model). Build stats of the smoke worth knowing for the full run:
S4 falls back to Z for ~48% of records because the game ends before turn t+8; corr(S2,Z)=0.70, corr(S4,Z)=0.83.
Hypothesis: at the full-run scale the late-game Mix targets are therefore nearly Z, which limits the late-game cost.

## Results
(empty until the orchestrator fills them: paste `eval.txt` and `build` stats here)

## Verdict
(PURSUE / ABORT / INCONCLUSIVE, from `results.json["verdict"]`)

## If PURSUE, the next ticket is ...
Train one az generation (from c5) with the selected Mix target as the value target and gate it against c5 with
600 paired games on the FDN eval split (seed 78, sims=100); adoption is decided only by that gate.

## Issues
- `/mnt/sata/gorge-training/playlines/` was read-only in the seat sandbox, so the smoke ran under `.ds4/scratch/pl4`
  and the full-run paths above are untested for write permission.
- `dzg` is not in this repo (`/mnt/sata/gorge-training/dzg/src`), so the "opt-in `--value-target` flag" the ticket
  allowed was not needed; `pl4_train.py` drives `dzg.models` / `dzg.data` directly (value head only).
- Turn buckets are on the global turn counter (both players), so "turns 1-4" are the first two turns of each player.
