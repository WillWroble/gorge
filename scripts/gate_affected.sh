#!/usr/bin/env bash
# Per-ticket lean test gate (operator, 2026-10-05): target 1-2 min wall.
#
# The gate runs this script from the BASE commit
# (`git show {base}:scripts/gate_affected.sh | bash -s {base}`), so a branch
# cannot weaken its own gate by editing it.
#
# What it runs, in parallel:
#   - ./rules (always: nearly every ticket's behaviour surfaces there), minus
#     the process-global tests and the slow whole-corpus censuses in POSTMERGE;
#   - the two Kr8 tests in their own processes (as the old module gate did);
#   - every package whose directory the branch touched (testdata maps to its
#     package), plus ./internal/codeshape and ./view;
#   - the always-run structural set, the fast reverse-dependency closure and
#     the corpus / templates triggers (gate_affected_plan; see "selection
#     gaps" below). `gate_affected.sh --list <base>` prints the selection
#     without running anything.
#
# The skipped tests are NOT dropped: scripts/postmerge_full.sh runs the full
# module suite on main after landings, and a red there is bisected and fixed
# at once (main red is never pre-existing).
set -euo pipefail

# Default-build packages in a list of candidate directories, one per line.
#
# A directory whose every .go file is build-tagged (cmd/autopayaudit) is not a
# package in a default build: naming it to `go vet`/`go test` fails the whole
# run with "build constraints exclude all Go files" (9b9bd27a, 2026-10-06).
# `go list -e` reports such a package with no .GoFiles/.TestGoFiles/
# .XTestGoFiles and exits 0 (the -e tolerates the load error), so keep only
# candidates with at least one default-build file. The class -- not just
# cmd/autopayaudit -- is covered: any candidate the default build cannot
# compile is dropped, and scripts/tests/gate_affected_smoke.sh pins that
# behavior so the filter cannot be dropped by a later edit.
#
# Input: candidate package paths on stdin (./dir form). Output: the subset
# buildable under the default tags, order preserved.
gate_affected_default_build_pkgs() {
  local p
  while IFS= read -r p; do
    [ -n "$p" ] || continue
    [ -n "$(go list -e -f '{{if or .GoFiles .TestGoFiles .XTestGoFiles}}y{{end}}' "$p" 2>/dev/null)" ] && echo "$p"
  done
  return 0
}

# Split the top-level ./rules tests into six concurrent `go test -run`
# patterns (see the call site below). Output: exactly six regexes, one per
# line, whose union is every top-level test. A test is bucketed by the TWO
# characters after "Test" (a 2-char prefix), prefixes are balanced by test
# count, and the prefix set comes from `go test -list`, so a new test under an
# existing prefix is always covered. Any surprise in the listing (empty list,
# a name shorter than six characters, fewer than six distinct prefixes, an
# empty bucket) returns nonzero and the caller falls back to the single
# unsplit run, so a malformed list can never silently drop a test.
#
# Why six 2-char buckets, not four 1-char ones
# (cli-20261010T064434Z-b50b2a6a): the rules phase is the affected gate's long
# pole, and the four 1-char buckets balance by test COUNT while the heavy
# tests cluster by name -- the four shards measured 9.8 / 16.5 / 20.1 / 31.2 s
# wall on 2026-10-10 (this worktree, `systemd-run ... -p MemoryMax=16G -p
# CPUQuota=1600%`, gate env, rules binary warm, each shard alone), a ~3x
# spread whose tail is the phase. A 2-char prefix is still a compact regex
# (`^Test(Co|Pa|...)`, ~40 prefixes per shard) and spreads those clusters
# across six buckets, balancing by count at finer grain. Measured the same
# way, six 2-char buckets measured 15.5-25.6 s wall (max 25.6 vs the 1-char
# 31.2), and a full affected-gate run (rules shards forced uncached with
# -cpuprofile=/dev/null, every other job cached) fell 43-45 s -> 37 s over
# two orderings. A full-NAME partition (one regex alternative per test) is
# NOT viable: the ~6600-alternative arg overflows ARG_MAX (exec rc=126) and
# its per-shard regexes measured 49-71 s, worse than the compact classes.
#
# Each shard line also overrides GOMEMLIMIT to 3GiB with the operator's
# doubled 2026-10-09 per-test budget (cli-20261009T114433Z-45f2f307): the gate
# env's 1536MiB sits UNDER a shard's 1.2-1.9 GiB working set, and the GC
# thrash stretched the rules phase under this gate's own scope (GOMAXPROCS=2
# also serialises the 1693 t.Parallel tests). At the doubled budget the
# count-balanced shards run at 590-611% CPU, serially peaking 1.2-1.9 GiB RSS
# each.
rules_shard_patterns_from_list() {
  local list
  list=$(cat)
  [ -n "$list" ] || return 1
  printf '%s\n' "$list" | awk 'length($0) < 6 { exit 1 }' || return 1
  printf '%s\n' "$list" | cut -c5-6 | sort | uniq -c | sort -rn | awk '
    function esc(s) { gsub(/[][\\^()|.*+?{}$]/, "\\\\&", s); return s }
    { cnt[NR] = $1; p[NR] = $2 }
    END {
      nb = 6
      for (i = 1; i <= NR; i++) {
        b = 1
        for (j = 2; j <= nb; j++) if (l[j] < l[b]) b = j
        c[b] = c[b] (c[b] == "" ? "" : "|") esc(p[i]); l[b] += cnt[i]
      }
      # An empty bucket would print an invalid empty alternation; fail the
      # helper instead so the caller falls back to the unsplit run.
      for (j = 1; j <= nb; j++) if (c[j] == "") exit 1
      for (j = 1; j <= nb; j++) print "^Test(" c[j] ")"
    }'
}

rules_shard_run_patterns() {
  go test -list '.*' ./rules/ 2>/dev/null | /usr/bin/grep '^Test' \
    | rules_shard_patterns_from_list
}

# Pack one package's test names into nb groups balanced by test count, one
# alternation regex per group on stdout. Unlike the character buckets above,
# the unit here is a single test, so a package's own chunked census tests
# (TestSameNameAnswerCensusChunk00..NN share one first character) spread
# across the groups. Any surprise (empty list, an empty group) exits nonzero
# and the caller falls back to running the package whole, so a malformed
# listing can never silently drop a test.
test_name_packs_from_list() {
  local nb=${1:?usage: test_name_packs_from_list <nb>}
  awk -v nb="$nb" '
    { name[NR] = $0 }
    END {
      if (NR == 0) exit 1
      for (i = 1; i <= NR; i++) {
        b = 1
        for (j = 2; j <= nb; j++) if (l[j] < l[b]) b = j
        g[b] = g[b] (g[b] == "" ? "" : "|") name[i]; l[b] += 1
      }
      for (j = 1; j <= nb; j++) if (g[j] == "") exit 1
      for (j = 1; j <= nb; j++) print "^(" g[j] ")$"
    }'
}
# The four compliance/oraclegen/templates shard selectors (tpl=1 in the gate
# body), one per line: three anchored -run patterns plus the -skip remainder.
# The remainder is the ANCHORED union of the other three, so h4 runs exactly
# what the named patterns miss -- a future name that merely extends one of
# the prefixes (TestSameNameAnswerCensusChunk00Extra) runs in h4 instead of
# being skipped by it and matched by no shard (r2 review of
# cli-20261009T182406Z-a616bfea: the unanchored skip that used to sit in the
# gate body deleted it from every shard silently). The smoke test
# (scripts/tests/gate_affected_smoke.sh) pins the partition against the
# package's real `go test -list` output.
templates_shard_patterns() {
  local even='^TestSameNameAnswerCensusChunk[0-9][02468]$'
  local odd='^TestSameNameAnswerCensusChunk[0-9][13579]$'
  local named='^TestSetup(Colour|CreatureType)Census$'
  local bare_even bare_odd bare_named rest
  bare_even="${even#?}"; bare_odd="${odd#?}"; bare_named="${named#?}"
  rest="${bare_even%?}|${bare_odd%?}|${bare_named%?}"
  printf '%s\n' "$even" "$odd" "$named" "^(${rest})$"
}

# --- selection gaps (docs/agents/test-throughput-2026-10-12.md §2.7) --------
# Most post-merge RED culprits on .ds4/postmerge-culprits.log (2026-10-05..12)
# were cheap tests in packages this gate never selected: structural censuses
# that read the TREE, not Go imports (a new test package without the
# testbudget TestMain shim, a package missing from the pkgmap/archtest
# classification, a stale web/src/protocol.ts), and corpus/engine censuses in
# reverse dependencies of what the branch touched. Three mechanisms close
# them; each package's cost is measured (2026-10-12, this worktree, test
# binary run alone under MemoryMax=4G CPUQuota=400% GOMAXPROCS=4, wall /
# user+sys CPU):
#
# 1. gate_structural_pkgs, ALWAYS run (whole package, the batch line). A tree
#    census is invisible to an import-based selection, and the go test result
#    cache already re-runs only the ones whose inputs moved:
#      internal/testbudget  0.98 s /  1.0 cpu-s  (TestEveryTestPackageRecordsRSS)
#      cmd/testbudget       <0.1 s /  0.0 cpu-s  (TestRunCheckExitCodes)
#      cmd/pkgmap           0.40 s /  0.5 cpu-s  (TestClassificationIsComplete)
#      cmd/gentypes         6.6 s  / 10.0 cpu-s  (protocol.ts freshness)
#      compliance/levelb    6.0 s  /  7.6 cpu-s  (TestLevelBRequirementCensus)
#      internal/policynet   6.8 s  /  8.4 cpu-s  (pinned real-game features)
#    plus internal/archtest with -skip TestEngineCompilesFor32Bit (3.2 s /
#    4.4 cpu-s; the 32-bit compile alone is ~50 s and stays post-merge only,
#    unless the branch touches internal/archtest, which then runs whole).
# 2. gate_fast_rdeps: the in-repo reverse-dependency closure of the touched
#    packages, RESTRICTED to gate_fast_rdep_pkgs -- packages whose whole test
#    binary measured <= 2.5 cpu-s and loads no card corpus (each test binary
#    is ~0.6-1 cpu-s to link). The corpus-loading packages pay ~6.5-10 cpu-s
#    each just to open the corpus and ~20 of them depend on ./rules, so they
#    keep the existing selection (touched dir, the triggers below, or
#    post-merge).
# 3. Cheap triggers for the two expensive culprit classes:
#    - a non-test Go change under cards/ or internal/testutil/ (the corpus
#      loader) runs internal/corpusheap whole (10.8 s / 14.0 cpu-s) and
#      cmd/botbench's TestSpellbenchSubsetCorpusMatchesFull (5.5 s / 6.9 cpu-s);
#    - a non-test Go change under rules|effects|events|state|cards, or any
#      change under compliance/oraclegen/, runs the templates REMAINDER shard
#      (everything but the SameNameAnswer chunks and the two Setup censuses:
#      41 s / 50.5 cpu-s; every one of the 8 templates culprits was a
#      remainder test). It is the exact h4 invocation of the tpl=1 path, so it
#      shares that path's result-cache entry.
gate_structural_pkgs() {
  printf '%s\n' ./internal/testbudget ./cmd/testbudget ./cmd/pkgmap \
    ./cmd/gentypes ./compliance/levelb ./internal/policynet
}

gate_fast_rdep_pkgs() {
  printf '%s\n' ./state ./events ./protocol ./internal/tsgen ./rules/chars \
    ./rules/combat ./rules/pay ./rules/scriptfacts ./rules/trigmatch \
    ./effects/params ./cards/oracletext ./cmd/keywordbench ./internal/botobs \
    ./internal/mzenc ./compliance/shape ./compliance/oraclediff ./cmd/xmageport \
    ./internal/divergence ./bots ./bots/azredeal ./bots/search ./cmd/codeshape \
    ./cmd/compliance ./cmd/paymirror ./internal/corpuscov ./cmd/exitloop \
    ./cmd/forgec ./cmd/searchbench ./internal/spellbench/registry \
    ./internal/azmcts/clairvoyant ./cmd/ledger ./cmd/gcgate ./cmd/traindash \
    ./protocol/manabrew/internal/schemagen ./cmd/autopayaudit \
    ./internal/searchbench/statespec ./internal/broker ./internal/gamepool
}

# Input: touched package dirs (./dir form) on stdin. Output: every
# gate_fast_rdep_pkgs member whose test binary transitively imports one of
# them (`go list -test` Deps of the pkg.test main, so test-only imports
# count). A package with no test files has no .test entry and is never
# printed. Order follows gate_fast_rdep_pkgs.
gate_fast_rdeps() {
  local touched mod
  touched=$(tr '\n' ' ')
  [ -n "${touched// /}" ] || return 0
  mod=$(go list -m 2>/dev/null) || return 0
  # shellcheck disable=SC2046
  go list -e -test -f '{{.ImportPath}}|{{join .Deps " "}}' $(gate_fast_rdep_pkgs) 2>/dev/null \
    | awk -F'|' -v mod="$mod" -v touched="$touched" '
      BEGIN {
        n = split(touched, t, " ")
        for (i = 1; i <= n; i++) if (t[i] ~ /^\.\/./) want[mod "/" substr(t[i], 3)] = 1
      }
      $1 ~ /\.test$/ {
        p = substr($1, 1, length($1) - 5)
        m = split($2, d, " ")
        for (i = 1; i <= m; i++) if (d[i] in want) { print "./" substr(p, length(mod) + 2); break }
      }'
  return 0
}

# The gate's selection, as a pure function of the touched file list (stdin,
# one repo-relative path per line, as `git diff --name-only` prints it).
# Output, one record per line:
#   others <./pkg>    a package the gate runs (batch, or heavy packs)
#   tpl 0|1           templates touched: run its four shards
#   tplrest 0|1       run only the templates remainder shard (trigger 3)
#   traj 0|1          trajectory-pinned packages + cardfuzz findings
#   archtest_lean 0|1 run internal/archtest minus the 32-bit compile
#   botbench_subset 0|1  run cmd/botbench's subset-corpus test (trigger 3)
# The gate body and `gate_affected.sh --list <base>` both read it, and
# scripts/tests/gate_affected_smoke.sh feeds it synthetic lists.
gate_affected_plan() {
  local files pkgs src others tpl=0 traj=0 tplrest=0 corpus=0 nontest p
  files=$(cat)
  pkgs=$(printf '%s\n' "$files" | while read -r f; do
    [ -n "$f" ] || continue
    d=$(dirname "$f"); d=${d%%/testdata*}
    if [ -d "$d" ] && compgen -G "$d/*.go" >/dev/null; then echo "./$d"; fi
  done | sort -u | /usr/bin/grep -v -x -E '\./rules' || true)
  # Drop candidates the default build cannot compile (see the helper above).
  if [ -n "$pkgs" ]; then
    pkgs=$(printf '%s\n' $pkgs | gate_affected_default_build_pkgs)
  fi
  others=$(printf '%s\n' $pkgs ./internal/codeshape ./view | sort -u)
  # Any change can move compliance verdicts: generator/harness edits did
  # (a407ddeff, FDN:A) and so did rules edits that reshape target asks
  # (40c7823d7/c97355932 staled FRA:A). compliance/gate and compliance/adopt
  # are the two tests that notice; they run on every ticket, in parallel with
  # ./rules, so a stale verdict parks the ticket for a host replay instead of
  # turning main red.
  others=$(printf '%s\n' $others ./compliance/gate ./compliance/adopt | sort -u)
  # Non-test, non-testdata files: what can change an importer's binary.
  nontest=$(printf '%s\n' "$files" | /usr/bin/grep -v -E '_test\.go$|(^|/)testdata/' || true)
  # Mechanism 2: the fast reverse-dependency closure. Its sources are the
  # packages whose non-test files moved, ./rules included.
  src=$(printf '%s\n' "$nontest" | while read -r f; do
    [ -n "$f" ] || continue
    d=$(dirname "$f")
    if [ -d "$d" ] && compgen -G "$d/*.go" >/dev/null; then echo "./$d"; fi
  done | sort -u)
  if [ -n "$src" ]; then
    others=$(printf '%s\n' $others $(printf '%s\n' $src | gate_fast_rdeps | gate_affected_default_build_pkgs) | sort -u)
  fi
  # Mechanism 1: the always-run structural set.
  others=$(printf '%s\n' $others $(gate_structural_pkgs) | sort -u)
  # Mechanism 3, corpus loader trigger: corpusheap runs whole (it is one test).
  if printf '%s\n' "$nontest" | /usr/bin/grep -q -E '^(cards|internal/testutil)/([^/]+/)*[^/]+\.go$'; then
    corpus=1
    others=$(printf '%s\n' $others ./internal/corpusheap | sort -u)
  fi
  # compliance/oraclegen/templates is the touched-package long pole when a
  # generator ticket lands; it is sharded four ways in the gate body (see
  # templates_shard_patterns), so it is pulled out of $others here.
  if printf '%s\n' $others | /usr/bin/grep -qx './compliance/oraclegen/templates'; then
    others=$(printf '%s\n' $others | /usr/bin/grep -vx './compliance/oraclegen/templates')
    tpl=1
  elif printf '%s\n' "$nontest" | /usr/bin/grep -q -E '^(rules|effects|events|state|cards)/([^/]+/)*[^/]+\.go$' \
    || printf '%s\n' "$files" | /usr/bin/grep -q -E '^compliance/oraclegen/'; then
    tplrest=1
  fi
  # Trajectory-pinned packages: tests that replay seeded bot games and pin a
  # finding by (seed, event seq), or replay a committed recorded game event for
  # event. Any engine change that emits, drops or reorders an event renumbers
  # them even when TestHeads is honestly re-pinned; 2eaca9010 (ExcessDamage
  # history events) re-pinned heads 4/6/8 and turned main red in
  # ./internal/paymirror unseen by this gate. They run when the branch moves a
  # chain head or touches engine code (rules/effects/events/state, non-test Go).
  # The cardfuzz half is only its two seed-pinned finding tests.
  if printf '%s\n' "$files" | /usr/bin/grep -v -E '_test\.go$' | /usr/bin/grep -q -E \
    '^rules/testdata/heads/|^(rules|effects|events|state)/([^/]+/)*[^/]+\.go$'; then
    traj=1
    others=$(printf '%s\n' $others ./internal/paymirror ./cmd/repro | sort -u)
  fi
  for p in $others; do echo "others $p"; done
  echo "tpl $tpl"
  echo "tplrest $tplrest"
  echo "traj $traj"
  if printf '%s\n' $others | /usr/bin/grep -qx './internal/archtest'; then
    echo "archtest_lean 0"
  else
    echo "archtest_lean 1"
  fi
  if [ "$corpus" = 1 ] && ! printf '%s\n' $others | /usr/bin/grep -qx './cmd/botbench'; then
    echo "botbench_subset 1"
  else
    echo "botbench_subset 0"
  fi
}

# When sourced by scripts/tests/gate_affected_smoke.sh, expose the helpers
# without running the gate. Detect sourcing structurally: an environment
# variable must never bypass the gate when the script is executed normally.
if (return 0 2>/dev/null); then return 0; fi

# `gate_affected.sh --list <base>` prints the selection and exits without
# running anything. It is an ARGUMENT, not an environment variable, so the
# gate's own `bash -s {base}` invocation can never reach it.
list_only=0
if [ "${1:-}" = --list ]; then list_only=1; shift; fi
base=${1:?usage: gate_affected.sh [--list] <base>}
mb=$(git merge-base "$base" HEAD)

plan=$(git diff --name-only "$mb" HEAD | gate_affected_plan)
if [ "$list_only" = 1 ]; then
  printf '%s\n' "$plan"
  exit 0
fi
plan_flag() { printf '%s\n' "$plan" | /usr/bin/grep -qx "$1 1"; }

global='TestHeads|TestLargeEliminationSweepDoesNotTripLivelockWatcher'
kr8='TestKr8WorldsInFuzzGames|TestKr8HeadsCheckpointAll'
# The four are sharded into chunk tests (2026-10-05 per-test budget: 2 GB,
# 4 vCPU, 1 min each); the suffix patterns skip every chunk.
postmerge='TestCloneFidelityShort[0-9A-Za-z]*|TestCostStaticPlannedCastsNeverCostChange[0-9]*|TestChainTargetOfferCensusAgreesWithCastFlow[0-9]*|TestPaymentPlanOnePassMatchesReferenceOverAutoPayGameKernel[0-9A-Za-z]*'

# The selection itself is gate_affected_plan (above), a pure function of the
# touched file list. In the tpl=1 case:
# compliance/oraclegen/templates is the touched-package long pole when a
# generator ticket lands (3 of 8 gate runs on 2026-10-09 measured it at
# 79-93 s vs the ~50 s Kr8Worlds pole): measured standalone under the gate
# env (this worktree, 2026-10-09, MemoryMax=4G CPUQuota=400% GOMAXPROCS=4
# GOMEMLIMIT=3GiB) the whole package is 80.3 s over 454 tests whose times sum
# to 106 s -- the SameNameAnswer census chunks (55.2 s over 9) and the two
# Setup censuses (26.3 s over 11) carry 81 s of it. It is pulled out of the
# `$others` run and sharded four ways below, next to the rules shards. The
# split is a PARITY/complement partition, so it covers every test by
# construction and needs no listing step: the odd/even chunk index handles a
# future Chunk05+ automatically, a new Setup census falls to the -skip
# remainder (which runs everything the other three do not), and no test can
# be dropped. The remainder's skip is anchored to the exact union of the
# three named patterns (templates_shard_patterns), so a future name that
# merely extends one of the prefixes runs in the remainder instead of being
# dropped from every shard. Perf drift is the only failure mode: a heavy
# test the named patterns miss lands in the remainder and the pole creeps up
# -- never a coverage hole.
others=$(printf '%s\n' "$plan" | awk '$1 == "others" { print $2 }')
tpl=0; plan_flag tpl && tpl=1
tplrest=0; plan_flag tplrest && tplrest=1
traj=0; plan_flag traj && traj=1
archlean=0; plan_flag archtest_lean && archlean=1
bbsub=0; plan_flag botbench_subset && bbsub=1
echo "gate_affected: rules + $(echo $others)$( [ "$tpl" = 1 ] && echo ' + oraclegen/templates shards' )$( [ "$tplrest" = 1 ] && echo ' + oraclegen/templates remainder shard' )$( [ "$archlean" = 1 ] && echo ' + archtest (no 32-bit)' )$( [ "$bbsub" = 1 ] && echo ' + botbench subset-corpus test' )$([ $traj = 1 ] && echo ' + cardfuzz findings')"

# Heavy $others packages. Measured 2026-10-09 (this worktree, the gate's own
# concurrent phase re-run with -json after `go clean -testcache`): each of
# these runs whole inside ONE test binary and its slowest tests are serial, so
# the phase's wall is the slowest single package's wall:
#   compliance/oraclegen/templates 166.8s (TestSameNameAnswerCensusChunk00 27.7s)
#   compliance/adopt                101.1s (TestLevelBRatchet 51.1s serial)
#   compliance/gate                 88.9s  (TestDeclaredSetsCompliant 26.8s serial)
#   internal/paymirror              89.1s  (TestRoundTenFindingsMirror 31.6s serial)
#   effects                         76.2s  (TestCompiledFilterMatchesTextualOracle 29.2s serial)
#   cmd/repro                       55.4s  (TestReproEmitTestIntoRulesCompilesAndFailsOnTODO 26.4s)
# Each is split by test name into three packs run as concurrent `go test`
# processes -- the ./rules shard mechanism, applied per package. The pack
# invocations are deterministic (same -run regex, same flags, every gate: the
# regex derives from the package's own test list, which only changes when the
# binary does), so the go test result cache does the freshness check for
# free: a package whose binary is unchanged cache-hits all three packs
# (~1s), and a package whose binary changed re-executes them. A heavy
# package is NEVER put back in the batch: the result cache is keyed per
# invocation (measured: the same -run/-skip rerun cache-hits, a whole-package
# run after pack runs does not), so a fresh package's batch line would pay
# the full package again to repopulate a differently-keyed entry.
heavy='^\./compliance/adopt$|^\./compliance/gate$|^\./internal/paymirror$|^\./cmd/repro$|^\./effects$|^\./compliance/oraclegen/templates$'
heavy_pkgs=$(printf '%s\n' $others | /usr/bin/grep -E "$heavy" || true)
heavy_bins=
build_pids=
if [ -n "$heavy_pkgs" ]; then
  WORK=$(mktemp -d "${TMPDIR:-/tmp}/gate-shards.XXXXXX")
  # A sourced run (scripts/tests/gate_affected_smoke.sh) returns at the guard
  # above and never reaches this body, so the trap is body-local.
  trap 'rm -rf "$WORK"' EXIT
  : > "$WORK/bins"
  for p in $heavy_pkgs; do
    # Build each heavy package's test binary ONCE, into the build cache: the
    # three packs below are separate `go test` processes and a process does
    # not see a compile another one is still running, so they would otherwise
    # each compile and link the same test variant themselves (the same effect
    # the ./rules build-once removes; measured there as 112 s -> 38 s for
    # three concurrent runs).
    GOMAXPROCS=6 go test -c -o /dev/null "$p" & build_pids="$build_pids $!"
    printf '%s\n' "$p" >>"$WORK/bins"
  done
  heavy_bins=$(cat "$WORK/bins")
fi

# Gate wall is the longest chain, so the phases below overlap everything that
# does not depend on another phase (measured on the 2026-10-06 gate logs: for
# a ticket that moves engine code the `$others` packages summed to ~115 s of
# test time run one after another, longer than the ~70 s ./rules run).
#
# Vet and the ./rules test build overlap: vet type-checks from source and the
# build compiles, so they share no work (go-build has already warmed the
# non-test packages). Both must pass before any test starts, as before.
# GOMAXPROCS=6/-p=6: the scope's CPUQuota is 800% (8 cores) but the gate env
# exports GOMAXPROCS=2, which caps each compile action to two threads. Measured
# on a fresh `rules/mana.go` edit, inside the gate's own scope
# (`systemd-run -p MemoryMax=8G -p CPUQuota=800%`): `go vet -p=2` over the
# `$others` set plus ./rules took 75.9 s / 147 cpu-s; `GOMAXPROCS=6 go vet -p=6`
# took 18.8 s / 68 cpu-s, peak RSS 3.7 GiB (under the 8 GiB scope). The quota,
# not the flag, is the ceiling -- at 800% the extra -p is real parallelism.
vet_pkgs="$others ./rules"
if [ "$tpl" = 1 ]; then
  # templates was pulled out of $others for the sharding below; vet it here
  # anyway, so a vet-only defect (copylocks, unusedresult, lostcancel, ...)
  # still fails the per-ticket gate (r2 review of
  # cli-20261009T182406Z-a616bfea; postmerge's `go vet ./...` only catches
  # it on main after the batch).
  vet_pkgs="$vet_pkgs ./compliance/oraclegen/templates"
fi
GOMAXPROCS=6 go vet -p=6 $vet_pkgs & v=$!
# Build the ./rules test binary ONCE before the concurrent rules runs.
# They are separate `go test` processes, and a process does not see a compile
# another one is still running, so each used to compile and link the same
# (large) test variant itself. Measured under a 200% cpu cap after a rules
# edit: three concurrent runs 112 s wall / 200 cpu-s, build-once-then-run
# 38 s / 62 cpu-s (the three then hit the build cache). The runs below are
# unchanged, so the result cache and the reported output are too.
GOMAXPROCS=6 go test -c -o /dev/null ./rules/ & w=$!
rc=0
wait "$v" || rc=1
wait "$w" || rc=1
# The heavy packages' test binaries (built above, concurrently with vet
# and the rules build) must be complete before the shard pool starts.
for pid in $build_pids; do wait "$pid" || rc=1; done
[ "$rc" = 0 ] || exit 1

# The main ./rules run is the longest single test job in the gate and it does
# not use the whole scope quota (241% of 400% measured above for one process;
# the scope is CPUQuota=1600%), so split it six ways by test name and run the
# shards concurrently, each with GOMAXPROCS=4: the gate env exports
# GOMAXPROCS=2, which would hold every shard's t.Parallel pool to two tests at
# a time. Six 2-char-prefix buckets rather than four 1-char ones
# (cli-20261010T064434Z-b50b2a6a): the 1-char buckets balanced by count still
# left a ~3x wall spread (9.8 / 16.5 / 20.1 / 31.2 s, this worktree
# 2026-10-10), because the heavy tests cluster by name; the finer 2-char
# buckets spread them (15.5-25.6 s) and the full affected gate fell 43-45 s ->
# 37 s. See the partition helper's comment for the full measurements. Each
# shard line also overrides GOMEMLIMIT to 3GiB, the operator's doubled
# 2026-10-09 per-test budget: the gate env's 1536MiB sits under the shard
# working set and the GC thrash stretched the two-shard rules phase to 78.8 s
# under this gate's own scope (cli-20261009T114433Z-45f2f307).
# `-skip` still removes the process-global tests (they run in their own gate
# or post-merge); the six `-run` patterns are a complete, disjoint partition
# of every remaining test (verified by construction in
# rules_shard_run_patterns, which falls back to the unsplit run if it cannot
# list the tests). Every shard keeps -p=1 (single package); the extra
# concurrent binaries fit the scope: no test's peak RSS is above ~2 GiB
# (internal/testutil/testdata/rss_exceptions.txt is EMPTY) and each binary is
# held near its GOMEMLIMIT=3GiB soft limit.
shard1=; shard2=; shard3=; shard4=; shard5=; shard6=
{ read -r shard1; read -r shard2; read -r shard3; read -r shard4; read -r shard5; read -r shard6; } < <(rules_shard_run_patterns) || true
if [ -n "$shard1" ] && [ -n "$shard2" ] && [ -n "$shard3" ] && [ -n "$shard4" ] && [ -n "$shard5" ] && [ -n "$shard6" ]; then
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard1" ./rules/ & a1=$!
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard2" ./rules/ & a2=$!
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard3" ./rules/ & a3=$!
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard4" ./rules/ & a4=$!
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard5" ./rules/ & a5=$!
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" -run "$shard6" ./rules/ & a6=$!
else
  GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -skip "^($global|$kr8|$postmerge)$" ./rules/ & a1=$!
  a2=; a3=; a4=; a5=; a6=
fi
# The two Kr8 runs sit under the gate env's inherited GOMAXPROCS=2, which
# holds each one's parallel game pool to two tests at a time; both are
# process-global tests (cloneFuzzTapeWorldHook / tapeCheckpointAll), so they
# cannot join the shards' binary. GOMAXPROCS=4, as the shards run: the Kr8
# pair was the longest pole of the whole affected phase (measured on the
# 2026-10-09 gate logs, agent-20261009T044926Z-8ec278bb/r2: 69.2s and 34.3s
# against the shards' 19-34s), and standalone in this worktree the worlds
# test reads 15.3s at GOMAXPROCS=2 vs 9.5s at 4 -- the gate figure is that
# run inflated ~4.5x by the ~30 concurrent threads the phase starts against
# the scope's 1600% quota, so the extra parallelism is exactly what a
# quota-bound phase shares out. GOMEMLIMIT=3GiB is the operator's 2026-10-09
# per-test budget, set explicitly as on the shard lines; the override is inert
# on TestKr8HeadsCheckpointAll (sequential, it flips a process-wide default)
# but keeps the two Kr8 processes consistent (cli-20261009T114433Z-45f2f307).
# GC relaxation at these two lines is measured and REJECTED
# (agent-20261009T195921Z-c0900c9d): standalone (-cpuprofile vehicle) GOGC=off
# reads 13.2s / 3.09 GiB peak vs GOGC=200's 16.3s / 1.24 GiB (GOGC=400 a wash,
# 16.1s / 1.96 GiB), but under this phase's shape the worlds pole's on-CPU
# work GREW 26.4 -> 32.3 cpu-s and its wall got worse (45.4s -> 50.5s), while
# the unmodified shape itself read 46.1s and 64.9s twenty minutes apart with
# the shared box's load at 33-40/32: the residual gate wall here is the pole
# being descheduled by the fleet's box load, which no line-local env knob buys
# back. Do not re-try GOGC/GOMEMLIMIT tuning at these lines.
GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -run '^TestKr8WorldsInFuzzGames$' ./rules/ & b=$!
GOMAXPROCS=4 GOMEMLIMIT=3GiB go test -p=1 -run '^TestKr8HeadsCheckpointAll$' ./rules/ & c=$!
# -p=6: the $others packages are independent test binaries; with -p=1 they
# ran strictly one at a time and were the long pole of the gate. Measured on
# the `$others` set alone under the gate scope (800% quota, test results
# expired with `go clean -testcache`): -p=2 62.3 s, -p=4 40.8 s, -p=6 32.5 s,
# peak RSS ~1.0 GiB. With ./rules now four concurrent shards the peak resident
# set is ~14 test binaries, each held near its GOMEMLIMIT=3GiB soft limit (the
# doubled 2026-10-09 per-test budget; a rules shard peaks 1.2-1.9 GiB RSS
# serially, no test above ~2 GiB and internal/testutil/testdata/
# rss_exceptions.txt is EMPTY): ~10 GiB against the scope's 16 GiB
# MemoryMax, and most $others binaries are far smaller.
# The batch keeps the non-heavy packages only. The heavy packages run as
# their own sharded packs below (fresh ones cache-hit there; see the heavy
# block above for why a whole-package batch line would instead pay the full
# package again).
batch=$(printf '%s\n' $others | /usr/bin/grep -v -E "$heavy" || true)
d=
if [ -n "$(printf '%s' $batch)" ]; then
  GOMAXPROCS=6 go test -p=6 -skip "^($global)$" $batch & d=$!
fi
# The heavy packages' shard pool. The packs are `go test` runs (not raw
# binary runs) so they keep writing the result cache: the gate after this one
# cache-hits the same deterministic invocations. Each pack is GOMAXPROCS=2
# GOMEMLIMIT=1536MiB, and the pool admits at most six packs at a time,
# refilling as they finish, so the pack binaries share the 16 GiB scope with
# the four rules shards, the Kr8 pair and the batch. The function runs in a
# subshell so the slot count reads only the pool's own job table, never the
# gate's other background jobs.
heavy_shard_pool() (
  local running=0 bad=0 i=0 kind p pat
  dispatch_heavy_shard() {
    IFS=$'\t' read -r kind p pat <<<"$1"
    if [ "$kind" = whole ]; then
      GOMAXPROCS=2 GOMEMLIMIT=1536MiB go test -p=1 -skip "^($global)$" "$p"
    else
      GOMAXPROCS=2 GOMEMLIMIT=1536MiB go test -p=1 -run "$pat" -skip "^($global)$" "$p"
    fi
  }
  local jobs=("$@")
  for job in "${jobs[@]}"; do
    ( dispatch_heavy_shard "$job" ) >"$WORK/hshard-$i.log" 2>&1 &
    i=$((i + 1))
    running=$((running + 1))
    while [ "$running" -ge 6 ]; do
      wait -n || bad=1
      running=$((running - 1))
    done
  done
  while [ "$running" -gt 0 ]; do
    wait -n || bad=1
    running=$((running - 1))
  done
  # Every pack's verdict line goes to the gate's own output (a fresh package
  # reads "ok ... (cached)" here), and a failed pack's full log follows so
  # the failure is visible even though the per-pack logs are trap-cleaned.
  cat "$WORK"/hshard-*.log | /usr/bin/grep -E '^(ok|FAIL|--- FAIL|panic)' || true
  if [ "$bad" = 1 ]; then
    /usr/bin/grep -l -E 'FAIL|panic' "$WORK"/hshard-*.log | while read -r f; do
      echo "gate_affected: failed heavy pack log $f:"
      cat "$f"
    done || true
  fi
  return "$bad"
)
heavy_jobs=()
hp=
if [ -n "$heavy_bins" ]; then
  while read -r p; do
    [ -n "${p:-}" ] || continue
    pats=$(go test -list '.*' "$p" 2>/dev/null | /usr/bin/grep '^Test' | test_name_packs_from_list 3 || true)
    if [ -n "$pats" ]; then
      while IFS= read -r pat; do heavy_jobs+=("shard	$p	$pat"); done <<<"$pats"
    else
      heavy_jobs+=("whole	$p	")
    fi
  done <<<"$heavy_bins"
fi
if [ "${#heavy_jobs[@]}" -gt 0 ]; then
  heavy_shard_pool "${heavy_jobs[@]}" & hp=$!
fi
# Event-text changes (any new or reworded event) move the committed
# overshoot capture and the searchprobe digests; e2e19ebae and 5fa9f31a both
# broke them unseen by this gate on 2026-10-05. Both checks are seconds, so
# they start with everything else instead of waiting for the two Kr8 runs to
# finish first.
go test -p=1 ./internal/searchprobe/ & e=$!
go test -p=1 -run '^TestCommittedOvershootCaptureReplaysToTheParkedAsk$' ./host/ & f=$!
pids="$a1 $a2 $a3 $a4 $a5 $a6 $b $c $d $e $f${hp:+ $hp}"
# The four templates shards (tpl=1, extracted above): three named -run
# patterns plus the -skip remainder, read one per line from
# templates_shard_patterns. Each stays under the operator's 1-minute
# per-test budget (measured split below, whole package 80.3 s), and the
# remainder's skip is anchored to the exact union of the other three, so the
# union is the whole package whatever a future generator ticket adds -- a
# name extending a prefix runs in the remainder, never in no shard.
if [ "$tpl" = 1 ]; then
  { read -r tpl1; read -r tpl2; read -r tpl3; read -r tpl4; } \
    < <(templates_shard_patterns)
  go test -p=1 -run "$tpl1" ./compliance/oraclegen/templates & h1=$!
  go test -p=1 -run "$tpl2" ./compliance/oraclegen/templates & h2=$!
  go test -p=1 -run "$tpl3" ./compliance/oraclegen/templates & h3=$!
  go test -p=1 -skip "$tpl4" ./compliance/oraclegen/templates & h4=$!
  pids="$pids $h1 $h2 $h3 $h4"
elif [ "$tplrest" = 1 ]; then
  # Trigger 3 (engine or generator change, templates untouched): only the
  # remainder shard, byte-identical to h4 above so the two share a
  # result-cache entry. 41 s / 50.5 cpu-s measured alone (2026-10-12).
  tpl4=$(templates_shard_patterns | sed -n 4p)
  go test -p=1 -skip "$tpl4" ./compliance/oraclegen/templates & h4=$!
  pids="$pids $h4"
fi
# internal/archtest without its ~50 s 32-bit compile (post-merge keeps it).
# A branch that touches internal/archtest runs it whole in the batch instead.
if [ "$archlean" = 1 ]; then
  go test -p=1 -skip '^TestEngineCompilesFor32Bit$' ./internal/archtest/ & i=$!
  pids="$pids $i"
fi
# Trigger 3 (corpus loader change): cmd/botbench's subset-corpus census only,
# 5.5 s / 6.9 cpu-s alone; the rest of the package (~80 s) stays post-merge.
if [ "$bbsub" = 1 ]; then
  go test -p=1 -run '^TestSpellbenchSubsetCorpusMatchesFull$' ./cmd/botbench/ & j=$!
  pids="$pids $j"
fi
if [ "$traj" = 1 ]; then
  go test -p=1 -run '^(TestRoundTenFindings|TestForbiddenRitualRepeatYesFinding)$' ./cmd/cardfuzz/ & g=$!
  pids="$pids $g"
fi
for p in $pids; do wait "$p" || rc=1; done
exit "$rc"
