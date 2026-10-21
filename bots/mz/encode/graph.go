package encode

import (
	"encoding/binary"
	"math"
	"math/bits"
	"slices"
	"strconv"
)

// A Key names a graph node: an object, a zone, an ability, or a feature
// (keyed by its name, so every parent with that feature shares the node).
// It is the fork's node UUID.
type Key uint64

// A Graph is the mage fork's FeatureGraph: nodes carry the bucket of their
// name's hash and a number, and edges to their children carry a label. It is
// the network's input and, through Hash, the functional hash.
type Graph struct {
	keys   []Key
	ids    []int32 // Node.id: the bucket of the name's hash
	values []int32 // Node.value
	old    []uint64
	index  map[Key]int32
	edges  []edge
	last   []int32 // per node, its newest edge as a parent, -1 for none
	hash   uint64
	hashed bool
}

type edge struct {
	parent, child int32
	prev          int32 // the parent's previous edge, -1 for none
	label         string
}

// NewGraph is an empty graph with its root.
func NewGraph() *Graph {
	g := &Graph{index: map[Key]int32{}}
	g.Reset()
	return g
}

// Reset empties g to its root, keeping its storage.
func (g *Graph) Reset() {
	g.keys, g.ids, g.values, g.old, g.edges, g.last = g.keys[:0], g.ids[:0], g.values[:0], g.old[:0], g.edges[:0], g.last[:0]
	clear(g.index)
	g.hashed = false
	// The root is new Node("root", 0, 0): id 0, not a name's bucket.
	g.index[RootKey] = 0
	g.keys, g.ids, g.values, g.old = append(g.keys, RootKey), append(g.ids, 0), append(g.values, 0), append(g.old, Mix64(0))
	g.last = append(g.last, -1)
}

// Len is the number of nodes.
func (g *Graph) Len() int { return len(g.keys) }

// Has reports whether k is a node.
func (g *Graph) Has(k Key) bool { _, ok := g.index[k]; return ok }

// Keys are g's node keys in insertion order (the root first).
func (g *Graph) Keys() []Key { return g.keys }

func (g *Graph) put(k Key, name string, value int32, numeric bool) {
	id := IndexFor(Hash64(name))
	g.index[k] = int32(len(g.keys))
	g.keys = append(g.keys, k)
	g.ids = append(g.ids, id)
	g.values = append(g.values, value)
	g.last = append(g.last, -1)
	if numeric {
		g.old = append(g.old, Mix64(uint64(int64(id)))^uint64(int64(value)))
	} else {
		g.old = append(g.old, uint64(int64(id)))
	}
}

// Add is StateEncoder.addFeature(name, id, parent, edge): node k named name
// unless it exists, and an edge to it from parent (no edge when parent is
// not a node). It reports whether the node is new.
func (g *Graph) Add(name string, k, parent Key, label string) bool {
	_, ok := g.index[k]
	if !ok {
		g.put(k, name, 0, false)
	}
	g.link(parent, k, label)
	return !ok
}

// AddNumeric is StateEncoder.addNumericFeature: the node name@num, carrying
// num, shared by every parent with that value.
func (g *Graph) AddNumeric(name string, num int32, parent Key, label string) {
	k := NameKey(name + "@" + strconv.Itoa(int(num)))
	if _, ok := g.index[k]; !ok {
		g.put(k, name, num, true)
	}
	g.link(parent, k, label)
}

func (g *Graph) link(parent, child Key, label string) {
	p, ok := g.index[parent]
	if !ok {
		return
	}
	g.hashed = false
	c := g.index[child]
	for i := g.last[p]; i >= 0; i = g.edges[i].prev {
		if g.edges[i].child == c {
			g.edges[i].label = label
			return
		}
	}
	g.edges = append(g.edges, edge{parent: p, child: c, prev: g.last[p], label: label})
	g.last[p] = int32(len(g.edges) - 1)
}

// Hash is FeatureGraph.getStateHash: 16 rounds of mixing each node with its
// children's hashes and edge labels, then the mixed sum of every node.
func (g *Graph) Hash() uint64 {
	if g.hashed {
		return g.hash
	}
	n := len(g.keys)
	old := slices.Clone(g.old)
	next := make([]uint64, n)
	labels := make([]uint64, len(g.edges))
	for i, e := range g.edges {
		labels[i] = Hash64(e.label)
	}
	for range 16 {
		clear(next)
		for i, e := range g.edges {
			next[e.parent] += old[e.child] ^ labels[i]
		}
		for i := range next {
			next[i] = Mix64(next[i] ^ old[i])
		}
		old, next = next, old
	}
	var sum uint64
	for _, h := range old {
		sum += h
	}
	g.hash, g.hashed = Mix64(sum), true
	return g.hash
}

// Arrays is FeatureGraph.GraphArrays: the graph for serialization, nodes in
// key order and edges by their nodes' local indices.
type Arrays struct {
	Order                            []Key
	IDs, Values                      []int32
	EdgeChild, EdgeParent, EdgeLabel []int32
}

// Arrays serializes g.
func (g *Graph) Arrays() Arrays {
	n := len(g.keys)
	byKey := make([]int32, n)
	for i := range byKey {
		byKey[i] = int32(i)
	}
	slices.SortFunc(byKey, func(a, b int32) int { return cmpKey(g.keys[a], g.keys[b]) })
	local := make([]int32, n)
	a := Arrays{Order: make([]Key, n), IDs: make([]int32, n), Values: make([]int32, n)}
	for li, i := range byKey {
		local[i] = int32(li)
		a.Order[li], a.IDs[li], a.Values[li] = g.keys[i], g.ids[i], g.values[i]
	}
	es := slices.Clone(g.edges)
	slices.SortStableFunc(es, func(x, y edge) int { return int(local[x.parent]) - int(local[y.parent]) })
	for _, e := range es {
		a.EdgeChild = append(a.EdgeChild, local[e.child])
		a.EdgeParent = append(a.EdgeParent, local[e.parent])
		a.EdgeLabel = append(a.EdgeLabel, IndexFor(Hash64(e.label)))
	}
	return a
}

func cmpKey(a, b Key) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// The fork's fixed nodes.
var (
	RootKey        = NameKey("GameRootId")
	BattlefieldKey = NameKey("BattlefieldId")
	GraveyardKey   = NameKey("GraveyardId")
	HandKey        = NameKey("HandId")
	ExileKey       = NameKey("ExileId")
	StackKey       = NameKey("StackId")
	CommandKey     = NameKey("CommandZoneId")
	PassKey        = passKey()
	UseTrueKey     = NameKey("UseTrueId")
	UseFalseKey    = NameKey("UseFalseId")
	StopKey        = NameKey("StopChoosing")
)

// passKey is FeatureGraph.PASS_ABILITY_ID, new UUID(0, "Pass".hashCode()).
func passKey() Key {
	h := int32(0)
	for _, c := range "Pass" {
		h = 31*h + c
	}
	return Key(uint64(int64(h)))
}

// NameKey is StateEncoder.stringToUUID: a feature's key, from its name.
func NameKey(s string) Key { return Key(Hash64(s)) }

const globalSeed = 0x9E3779B185EBCA87

// Hash64 is StateEncoder.hash64.
func Hash64(s string) uint64 {
	data := []byte(s)
	h := Mix64(globalSeed ^ (uint64(len(data)) * 0x9E3779B185EBCA87))
	for len(data) >= 8 {
		h ^= Mix64(binary.LittleEndian.Uint64(data))
		h = bits.RotateLeft64(h, 27)*0x9E3779B185EBCA87 + 0x165667B19E3779F9
		data = data[8:]
	}
	var k uint64
	for i, b := range data {
		k ^= uint64(b) << (8 * i)
	}
	h ^= Mix64(k)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

// Mix64 is StateEncoder.mix64 (splitmix64's finalizer).
func Mix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// IndexFor is StateEncoder.indexFor: a hash's bucket.
func IndexFor(h uint64) int32 {
	x := int64(h)
	if x < 0 {
		x = -x
	}
	return int32(x % math.MaxInt32)
}
