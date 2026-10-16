package divergence

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// TestShadowSeedStable pins ShadowSeed's values: the seed is a pure
// function of (matchSeed, seat, n), so a recorded divergence's shadow
// answer recomputes identically on every machine and run. Changing the
// constants changes every recorded B and is an operator decision, never a
// drive-by edit.
func TestShadowSeedStable(t *testing.T) {
	want := map[struct {
		seed uint64
		seat uint32
		n    int
	}]uint64{
		{0, 0, 0}:                     0xb64c23ada4d90418,
		{0xdeadbeef, 0, 118}:          0xb4174d80e1a6a8ac,
		{0xdeadbeef, 1, 118}:          0xa37f76259f70d167,
		{0xdeadbeef, 0, 119}:          0x51aa08c65b19ce9a,
		{1, 3, 2}:                     0xba854355fe77b5d2,
		{0xffffffffffffffff, 7, 4096}: 0x889c65b731447cc,
	}
	for in, w := range want {
		if got := ShadowSeed(in.seed, state.PlayerID(in.seat), in.n); got != w {
			t.Fatalf("ShadowSeed(%#x, %d, %d) = %#x, want %#x", in.seed, in.seat, in.n, got, w)
		}
	}
}
