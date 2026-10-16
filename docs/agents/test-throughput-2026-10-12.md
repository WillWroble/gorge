# gorge test-runtime audit — 2026-10-12 (internal)

No edits, no tests run (box at load 78–95/32 cores). **est.** / **hypothesis** where unmeasured. Verified by orchestrator: `scripts/heavy_lock.sh:~162` does `kill -STOP`; `cmd/testbudget/check.go:221-228` returns 0 when wall exceptions are unseeded.

## TL;DR

1. **Post-merge wall is the suite sitting SIGSTOPped.** `heavy_lock.sh` gate-yield (029087326, merged 2f3ba9c27 10-09 ~22:45) STOPs the post-merge process group whenever a gate bracket is active. Go's `-timeout` counts wall time while stopped → "test timed out after 10m" → FLAKY → pipeline pause. Post-merge wall jumped ~140–500 s → ~1,600–1,800 s exactly at that merge. 10-10 11:18 timeout RED deadlocked the pipeline **47 h**. In-flight run (764ea988e, 13:42) had 7 packages killed at 11 m; `templates.test` 1,425 s elapsed / 74 s CPU; scope ~1,112 CPU-s over ~1,750 s wall.
2. **Per-test budget not enforced.** `wall_exceptions.txt` is `!unseeded` → `testbudget check` returns 0; same branch skips RSS. `rss/host` = 7,159,808 KiB yet 75df07f44 GREEN.
3. **Gate is CPU-bound on an oversubscribed box.** ~20 sharding/parallelism commits 10-09/10 left gate_wall median 172–213 s/day. Lever is less CPU work (cache, selection), not more shards.
4. **Most post-merge catches are cheap tests in packages the gate never selects** (RSS TestMain census, gentypes freshness, templates/levelb/corpusheap censuses, archtest).
5. **Corpus-IR thundering herd.** Fingerprint covers every non-test `engine/cards/*.go` (73 commits/7 d); `cards.openCorpus` has no cross-process lock → every concurrent missing binary recompiles.

## 1. Current state

| Stage | What runs | Wall | Source |
|---|---|---|---|
| Seat self-test | focused capped `go test -run` | not instrumented | AGENTS.md |
| Gate (agentctl; landings in `.ds4/orchestrator/staging`, merge_train since 10-12) | xmage driver ≈0 · ratchets ≈0 · broker ≈0 · `go build ./...` p50 13/p90 27 · `go test (affected)` (`scripts/gate_affected.sh`) **p50 144/p90 271/max 436** · CR conformance p50 9 · TestHeads p50 11 · make sim p50 12 · **total p50 196/p90 313/max 453 s** | n=28 since 10-09, `.ds4/orchestrator/gates/*/` mtimes |
| Gate wall trend (daily median) | 10-05 211 · 10-07 91 · 10-08 106 · 10-09 213 · 10-10 190 · 10-12 172 s | `.ds4/reward/scoreboard.jsonl` |
| Heavy-lane wait in affected gate | 1,500 s then exit 75 | 20/612 logs |
| Post-merge (`postmerge_batch.sh` → `postmerge_full.sh`) | vet; `go test -p=8 -json ./...` + testbudget; TestHeads; sim; reward_loop smoke | mean 10-05 417 · 10-06 270 · 10-07 195 · 10-08 207 · 10-09 415 · **10-10 1,412** · 10-12 619 (max 1,819). 36 RED, 31 CULPRIT, 13 FLAKY | `.ds4/postmerge-batch.log`, `postmerge-culprits.log` |

Gate failures since 10-05: go test (affected) 190, xmage driver 76, go build 3; **CR conformance 0, make sim 0, TestHeads 0**. Inside affected: lab/oracle/compliance/gate & adopt 54 FAIL lines each, rules 84.

### Slowest (contended numbers)

| # | Package / test | Number | Source |
|---|---|---|---|
| 1 | `cards` (when touched) | gate p50 166, max 236 s | gate logs |
| 2 | `lab/oracle/compliance/oraclegen/templates` | 166.8 s clean cache | gate_affected.sh comment |
| 3 | `lab/oracle/compliance/adopt` | 101 s; TestLevelBRatchet 51 s | same; events.json |
| 4 | `rules` (max of 6 shards) | gate p50 97.5, max 175; alone 15.5–25.6/shard; whole 66 s / 6,470 tests (10-06) | gate logs |
| 5 | `internal/paymirror` | 89 s; 0/53 cached | |
| 6 | `lab/oracle/compliance/gate` | 89 s; TestDeclaredSetsCompliant 27–34 s | |
| 7 | `effects` | 76 s; TestCompiledFilterMatchesTextualOracle 29 s | |
| 8 | `cmd/repro` | 55 s; 0/56 cached | |
| 9 | TestKr8WorldsInFuzzGames | 9.5 s alone; 46–69 s in gate | |
| 10 | archtest TestEngineCompilesFor32Bit | 50.6 s | |
| 11 | rules TestDecisionTextHasNoEngineSyntax | 43.7 s | |
| 12 | rules TestAirbendIncrementalIndex… | 28.3 s | |
| 13 | `lab/cmd/botbench` | 80.8 s (n=1) | |
| 14 | corpusheap TestCorpusHeapCensus | 19 s; pkg 60 s in gate | |
| 15 | v2shadow TestShadowPlaysWholeGames | 22.6 s | |
| 16 | `bots/searchprobe` | p50 29.5 s (always in gate) | |
| 17 | `view` | p50 25, max 83 s | |
| 18 | `cmd/gentypes` | 10.6 s | |
| 19 | TestRepoDecksPlayAtEverySeatCount / ReplayExactly | **4.6 / 3.8 s — golden games are cheap** | |
| 20 | `host` | p50 12.6, max 159 s; **RSS 7.16 GB** | |

RSS peaks: host 7.16 GB, lab/oracle/compliance/gate 2.58, cards 2.39, cardfuzz 2.22, paymirror 1.99; rest ≤1.6.

Static: 4,002 test files; 11,941 tests; 672k test LOC. `rules` 6,714 tests, 418k test LOC / 2,148 files over 150k non-test; 5,784 `t.Parallel()`. 120 packages.

## 2. Root causes

1. SIGSTOP yield + wall-clock timeouts (140 gate brackets on 10-09).
2. Oversubscription: 3 heavy lanes × 1600% quota on 32 cores + seats; ~80 runnable threads on a 16-core gate quota; 4.5–7× inflation. est. gate CPU ≈1,400–1,800 CPU-s → floor ~90–110 s; p50 144 is within 1.5×.
3. Dep fan-out: cards 100, state 89, decision 86, events 82, effects 74, rules 62 reverse deps (of 120). 63% of commits since 10-05 touch rules' test closure.
4. Cache misses: rules cached 14% vs ~37% possible. Tests reading files by absolute path/mtime (89 files); gate vs post-merge use different `-run`/`-skip` shapes so never share cache; `GORGE_ORACLEGEN_FULL_TARGET_AUDIT=1` only post-merge. `-trimpath` already global; GOCACHE 302 GB.
5. IR fingerprint churn (`engine/cards/fingerprint_sources.go` embeds fetch/gitenv/open.go…) + no compile lock (`engine/cards/open.go:96-117`). CompileDir cost unmeasured.
6. Monolithic rules test binary (build 112 s → 38 s at 200% CPU); split under way (75f499519).
7. Selection gaps → post-merge RED/bisect: testbudget TestEveryTestPackageRecordsRSS (4 culprits + ~6 fix-ups), gentypes (2), templates censuses (8), levelb, corpusheap ×2, archtest, policynet, botbench, cmd/testbudget, searchprobe.
8. Budget enforcement off.

## 3. Ranked recommendations

| # | Change | Expected saving | Coverage risk | Evidence |
|---|---|---|---|---|
| 1 | Replace SIGSTOP with `CPUWeight=idle` on post-merge scope (keep memory-floor kill); or `-timeout 0` + testbudget wall check | post-merge back to ≈350–450 s from 1,400–1,800; removes timeout REDs/pauses | none | batch log, ps `T`, cpu.stat |
| 2 | Seed testbudget inventories, make check bite; fix/split host (7.16 GB) | prevents regrowth | none | check.go |
| 3 | Replace TestMain-RSS with `go test -exec testbudget exec`; delete TestEveryTestPackageRecordsRSS + ~90 shims (verify cache works with -exec) | removes most frequent post-merge RED class | none | culprits |
| 4 | Fix selection in `gate_affected.sh`: reverse-dep closure (fast pkgs) + always-run cheap structural set (testbudget, cmd/testbudget, gentypes, levelb, archtest minus 32-bit, policynet) | +10–30 CPU-s/gate; removes ~18 cheap culprits/week | coverage up | culprits |
| 5 | Delete `CR conformance` gate — TestCR* already in rules shards; env var only read by an unrun botpolicy test | ~9–10 s serial/gate | none (strict dup; 0 failures) | `.agentctl/config.toml:426` |
| 6 | Run TestHeads + make sim concurrently with affected gate | ~20–25 s off serial chain | none | gate mtimes |
| 7 | Persistent per-lane checkout for every gate round; post-merge reuses gate invocation shapes | est. −30–50 s gate p50; post-merge rules reuse | none | cache tally |
| 8 | flock single-flight corpus compile; narrow fingerprint to parser/compiler files | (N−1) compiles saved per fingerprint change, N≈20–45; measure first | low | open.go |
| 9 | Keep 32-bit compile post-merge only | 50 s/gate if archtest added | guarded post-merge | |
| 10 | Cap gate concurrency ≈ quota cores | hypothesis; A/B | none | Kr8 comment |
| 11 | Split heavy censuses (adopt TestLevelBRatchet, TestDeclaredSetsCompliant, TestCompiledFilter…, TestDecisionText…, paymirror findings) into partitioned chunks | 20–30 s pole shrink | none | |
| 12 | Tidy dead `TestInvariantsUnderSeedFuzz` skip pattern (`//go:build fuzz`) | 0 | — | engine/rules/fuzz_test.go:1 |

**Not recommended to omit:** golden/whole-game tests (4.6/3.8/~11/~12 s) are cheap and catch real moves; lab/oracle/compliance/gate & adopt earn their place (54 fails each). No strict-subset tests found; deletion would need per-candidate coverage diff.

## 4. Tiered policy

| Tier | When | Contents |
|---|---|---|
| T0 seat (≤2 min) | pre-commit | build, ratchet_gate.py, vet touched, focused `-run` |
| T1 gate (p50 ≤150 s) | per ticket / merge train | build; vet affected + rdeps; rules 6 shards minus post-merge censuses; Kr8; touched + fast rdeps; always codeshape, view, lab/oracle/compliance/gate+adopt (sharded), searchprobe, host overshoot, testbudget/gentypes/levelb/policynet/archtest(−32-bit); engine change → paymirror, cmd/repro, cardfuzz findings; TestHeads + sim concurrent; **drop CR conformance gate** |
| T2 post-merge (CPUWeight=idle, no STOP) | per landing burst | vet ./...; full go test incl. rules post-merge chunks, full target audit, 32-bit compile, searchseat/searchbench/spellbench/azmcts; seeded testbudget; heads, sim, reward smoke |
| T3 nightly (new; no timer today) | daily on origin/main | `make fuzz`, `make conformance`, `make test-manabrew`, race pass, `make cover`, cold-cache run |
| T4 on-demand | operator | set audits, broker/gamepool/policyexp tags, enginebench, oracle-audit, xmage/forge replays |

Key paths: `scripts/{postmerge_batch,postmerge_full,gate_affected,heavy_lock,broker}.sh`, `.agentctl/config.toml` (gates 254–480), `cmd/testbudget/check.go`, `internal/testutil/testdata/wall_exceptions.txt`, `engine/cards/open.go`, `engine/cards/fingerprint_sources.go`, `internal/testutil/sharedcorpus.go`. Evidence: `.ds4/postmerge-batch.log`, `.ds4/postmerge-culprits.log`, `.ds4/postmerge-full.out`, `.worktrees/postmerge-full/.ds4/scratch/testbudget/`, `.ds4/orchestrator/gates/`, `.ds4/orchestrator/journal.jsonl`, `.ds4/reward/scoreboard.jsonl`.
