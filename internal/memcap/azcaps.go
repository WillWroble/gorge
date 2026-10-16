// The az caps guard is the fail-fast protection against a kernel memcg OOM
// kill of an az run: 2026-10-14 the azfix seat launched `cardval -az
// -workers 4 -az-sims 25` under GOMEMLIMIT=3GiB and a 4 GiB cgroup cap seven
// times in five minutes (19:52-19:57Z); every attempt was OOM-killed at ~4.0
// GiB RSS and the seven kernel kills vetoed the reward scoreboard at head
// 8b70eaf23. A clean, immediate refusal replaces that: the launcher sees the
// reason on stderr and no oom_kill counter ever moves.
//
// The calibration is the proven az caps (cardval/runs.sh, the az-c4-s25
// run): 16 workers completed under GOMEMLIMIT=16GiB with MemoryMax=24G --
// one GiB of soft memory per worker, and a hard cap at least 1.4x the soft
// limit for the transient the scavenger has not returned yet. Below -az-sims
// 25 the per-worker estimate scales down linearly (the search tree is the
// per-worker cost); it is never raised above 1 GiB, because nothing above
// sims 25 has been measured. The calibration is engine-tier-neutral: any az
// search this engine trains uses the same shape.
package memcap

import (
	"fmt"
	"math"
	"runtime/debug"
)

const (
	azPerWorkerBytes = int64(1) << 30 // proven soft need per worker at sims 25
	azCalSims        = 25             // sims the per-worker figure was measured at
	// headroom the hard cap needs over the soft limit: the proven pair is
	// 24G over 16GiB = 1.5x, so 1.4x is the accepted floor.
	azCapNum, azCapDen = 7, 5
)

// AzCapsError refuses to start an az run whose memory settings cannot hold
// its working set. workers and sims are the run's flags; soft is the runtime
// soft limit in bytes (debug.SetMemoryLimit(-1), math.MaxInt64 when
// GOMEMLIMIT is unset) and hard is the effective cgroup cap in bytes (0 when
// none applies). It returns nil when no binding limit applies or the settings
// fit the proven shape.
func AzCapsError(workers, sims int, soft, hard int64) error {
	per := azPerWorkerBytes
	if sims < azCalSims {
		per = per * int64(sims) / azCalSims
	}
	need := per * int64(workers)
	// Anything at or above 1/16 of MaxInt64 is "unset": the runtime reports
	// GOMEMLIMIT=unset as math.MaxInt64, and no real soft limit approaches
	// that. Keeping the sentinel out of the arithmetic below also keeps
	// required*azCapNum from overflowing int64.
	softKnown := soft > 0 && soft < math.MaxInt64>>4
	if softKnown && soft < need {
		return fmt.Errorf(
			"az needs ~%d GiB for %d workers (-az-sims %d; proven ~1 GiB/worker at sims 25) but GOMEMLIMIT is %d GiB: the run would thrash the GC and be OOM-killed. Raise GOMEMLIMIT to at least %d GiB (scripts/heavy.sh --mem with MemoryMax >= 1.4x GOMEMLIMIT, the proven 16GiB/24G pair) or lower -workers",
			need>>30, workers, sims, soft>>30, need>>30)
	}
	if hard <= 0 {
		return nil
	}
	required := need
	src := "no GOMEMLIMIT"
	if softKnown {
		required = soft
		src = fmt.Sprintf("GOMEMLIMIT %d GiB", soft>>30)
	}
	if hard*azCapDen < required*azCapNum {
		capGiB := (required*azCapNum + azCapDen - 1) / azCapDen >> 30
		return fmt.Errorf(
			"az working set ~%d GiB (%s, %d workers at -az-sims %d) needs a hard cap of at least %d GiB (proven 1.4x headroom; 1.5x in cardval/runs.sh) but the cgroup cap is %d GiB: the run would be memcg OOM-killed. Raise the cap (scripts/heavy.sh --mem) or lower -workers/GOMEMLIMIT",
			required>>30, src, workers, sims, capGiB, hard>>30)
	}
	return nil
}

// CheckAzCaps runs AzCapsError against this process's real limits and fails
// the run when they cannot hold it. Callers invoke it before the az belief is
// built, so the refusal costs nothing.
func CheckAzCaps(workers, sims int) error {
	return AzCapsError(workers, sims, debug.SetMemoryLimit(-1), Limit("", ""))
}
