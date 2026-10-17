package divergence

import "github.com/adams-shaun/gorge/engine/state"

// ShadowSeed derives the per-decision seed of the shadow policy's fresh
// bot instance (design §3, §5.4): the same splitmix-avalanche construction
// as the retired hindsight tool's Seed, with its own constants, so the two streams never
// coincide. The answer B at intent n of seat s in a match whose seed is
// matchSeed is a pure function of (matchSeed, s, n, policy): the shadow
// bot gets no wall-clock deadline, so triage can recompute B and compare
// it with the recorded one.
func ShadowSeed(matchSeed uint64, seat state.PlayerID, n int) uint64 {
	x := matchSeed ^ mix64(uint64(seat)+0x736861646f777365) ^ mix64(uint64(uint32(n))+0x646976657267656e)
	return mix64(x)
}

func mix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
