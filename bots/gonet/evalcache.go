package gonet

import "sync"

// EvalCache maps state hashes to values and (state, options) hashes to option
// scores. Sharded; a full shard is cleared rather than evicted entry by entry.
type EvalCache struct {
	shards [64]cacheShard
	cap    int
}

type cacheShard struct {
	mu     sync.Mutex
	vals   map[uint64]float32
	scores map[uint64][]float32
}

func NewEvalCache(capacity int) *EvalCache {
	c := &EvalCache{cap: capacity / 64}
	for i := range c.shards {
		c.shards[i].vals = map[uint64]float32{}
		c.shards[i].scores = map[uint64][]float32{}
	}
	return c
}

func (c *EvalCache) Value(k uint64) (float32, bool) {
	s := &c.shards[k%64]
	s.mu.Lock()
	v, ok := s.vals[k]
	s.mu.Unlock()
	return v, ok
}

func (c *EvalCache) Scores(k uint64) ([]float32, bool) {
	s := &c.shards[k%64]
	s.mu.Lock()
	v, ok := s.scores[k]
	s.mu.Unlock()
	return v, ok
}

func (c *EvalCache) Put(sh, key uint64, v float32, sc []float32) {
	s := &c.shards[sh%64]
	s.mu.Lock()
	if len(s.vals) >= c.cap {
		s.vals = map[uint64]float32{}
	}
	s.vals[sh] = v
	s.mu.Unlock()
	if key == 0 {
		return
	}
	s = &c.shards[key%64]
	s.mu.Lock()
	if len(s.scores) >= c.cap {
		s.scores = map[uint64][]float32{}
	}
	s.scores[key] = sc
	s.mu.Unlock()
}
