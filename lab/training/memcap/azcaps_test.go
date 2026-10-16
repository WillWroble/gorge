package memcap

import (
	"strings"
	"testing"
)

const softMax = int64(1) << 62

// The proven pair from cardval/runs.sh (the az-c4-s25 run): 16 workers under
// GOMEMLIMIT=16GiB with MemoryMax=24G completed.
func TestAzCapsAcceptTheProvenPair(t *testing.T) {
	if err := AzCapsError(16, 25, 16*gib, 24*gib); err != nil {
		t.Fatalf("AzCapsError(16, 25, 16GiB, 24G) = %v, want nil", err)
	}
	// Unset GOMEMLIMIT under a cap wide enough for the estimated working set.
	if err := AzCapsError(16, 25, softMax, 24*gib); err != nil {
		t.Fatalf("AzCapsError(16, 25, unset, 24G) = %v, want nil", err)
	}
}

// The killed shape: 2026-10-14 the azfix seat ran 4 workers at sims 25 under
// GOMEMLIMIT=3GiB inside a 4 GiB cap, seven times; every attempt was
// OOM-killed at ~4.0 GiB RSS. Both limits must refuse it, loudly.
func TestAzCapsRefuseTheKilledShape(t *testing.T) {
	err := AzCapsError(4, 25, 3*gib, 4*gib)
	if err == nil {
		t.Fatal("AzCapsError(4, 25, 3GiB, 4G) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "GOMEMLIMIT") {
		t.Fatalf("refusal does not name GOMEMLIMIT: %v", err)
	}
	// Same shape with no soft limit at all: the hard-cap arm refuses.
	err = AzCapsError(4, 25, softMax, 4*gib)
	if err == nil {
		t.Fatal("AzCapsError(4, 25, unset, 4G) = nil, want a refusal")
	}
	// Soft limit that fits the working set but a cap without the 1.4x
	// headroom the proven pair carries: the transient between GCs is what
	// the kernel killed, so the cap arm still refuses.
	err = AzCapsError(4, 25, 5*gib, 5*gib)
	if err == nil {
		t.Fatal("AzCapsError(4, 25, 5GiB, 5G) = nil, want a refusal: cap has no headroom over the soft limit")
	}
}

// No binding limit means no opinion: an unconfined run must start.
func TestAzCapsStaySilentWithoutALimit(t *testing.T) {
	if err := AzCapsError(16, 25, softMax, 0); err != nil {
		t.Fatalf("AzCapsError with no hard cap and no GOMEMLIMIT = %v, want nil", err)
	}
}

// Below the calibration sims the per-worker estimate scales down, so a small
// belief-style smoke at sims 2 fits a cap the full search would not.
func TestAzCapsScaleWithSimsBelowTheCalibrationPoint(t *testing.T) {
	// 16 workers at sims 2: ~0.08 GiB/worker -> ~1.31 GiB total.
	if err := AzCapsError(16, 2, 2*gib, 4*gib); err != nil {
		t.Fatalf("AzCapsError(16, 2, 2GiB, 4G) = %v, want nil", err)
	}
	// The same workers at sims 25 need 16 GiB: the same 4G cap refuses.
	if err := AzCapsError(16, 25, 2*gib, 4*gib); err == nil {
		t.Fatal("AzCapsError(16, 25, 2GiB, 4G) = nil, want a refusal")
	}
}

// The refusal is actionable: it names the knobs and the proven pair, so the
// launcher can fix the launch instead of retrying into the next OOM kill.
func TestAzCapsRefusalNamesTheFix(t *testing.T) {
	err := AzCapsError(4, 25, 3*gib, 4*gib)
	if !strings.Contains(err.Error(), "ops/scripts/heavy.sh") || !strings.Contains(err.Error(), "-workers") {
		t.Fatalf("refusal does not name the fix: %v", err)
	}
}
