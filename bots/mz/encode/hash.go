// Package encode is mz-gorge's state encoder: what the mage fork's
// StateEncoder and FeatureGraph are to MageZero. It reads only the view
// (principle 1 of mz-gorge-design.md). For now it gives a state's two
// hashes; the network's graph is stage 3.
package encode

import (
	"cmp"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
)

// Hashes are a state's functional and logical hashes (the design doc's
// glossary). Functional is the same for functionally identical states: it
// hashes the view as a graph of objects, refining each object by what refers
// to what (Weisfeiler-Lehman, 16 rounds, as FeatureGraph.getStateHash), so
// it depends on no object ID. Logical adds the IDs.
type Hashes struct {
	Functional, Logical uint64
}

const rounds = 16

// Hash hashes v. Over the omniscient view it is the perfect-information
// pair; over a seat's view, that seat's hidden-information pair.
func Hash(v *view.View) Hashes {
	x := pool.Get().(*hasher)
	defer pool.Put(x)
	x.reset()
	x.record(0)
	x.walk(0, reflect.ValueOf(v).Elem())

	// A reference to no record is a player smuggled in an ID
	// (state.PlayerRef) or a gone object: a record of its own, colored by
	// that alone.
	for i := 0; i < len(x.recs); i++ {
		for _, id := range x.recs[i].refs {
			if _, ok := x.index[id]; !ok {
				g := x.record(id)
				g.h = goneToken
				if p, isPlayer := id.PlayerRef(); isPlayer {
					g.h = mix(playerToken ^ uint64(p))
				}
			}
		}
	}
	n := len(x.recs)
	color, next := grow(x.color, n), grow(x.next, n)
	for i := range n {
		color[i] = x.recs[i].h
	}
	for range rounds {
		for i := 1; i < n; i++ {
			next[i] = mix(x.recs[i].h ^ x.refsHash(i, color))
		}
		color, next = next, color
	}
	x.color, x.next = color, next
	f := mix(x.recs[0].h ^ x.refsHash(0, color))
	return Hashes{Functional: f, Logical: mix(f ^ x.ids)}
}

var views = sync.Pool{New: func() any { return new(view.View) }}

var pool = sync.Pool{New: func() any { return &hasher{index: map[state.ObjID]int{}} }}

func grow(s []uint64, n int) []uint64 {
	if cap(s) < n {
		return make([]uint64, n)
	}
	return s[:n]
}

// A record is one object's content, or (record 0) the rest of the view's,
// with every object it names left as a reference.
type record struct {
	h    uint64
	refs []state.ObjID
}

func (r *record) add(h uint64) { r.h = mix(r.h ^ h) }

type hasher struct {
	recs        []record
	index       map[state.ObjID]int
	ids         uint64 // every ID in walk order: the logical part
	keys        []reflect.Value
	strs        []string
	color, next []uint64
}

func (x *hasher) reset() {
	x.recs = x.recs[:0]
	clear(x.index)
	x.ids = 0
	x.keys = x.keys[:0]
	x.strs = x.strs[:0]
}

// record returns the record of object id (0: the view's), making it.
func (x *hasher) record(id state.ObjID) *record {
	if i, ok := x.index[id]; ok {
		return &x.recs[i]
	}
	x.index[id] = len(x.recs)
	if len(x.recs) < cap(x.recs) {
		x.recs = x.recs[:len(x.recs)+1]
		r := &x.recs[len(x.recs)-1]
		r.h, r.refs = 0, r.refs[:0]
		return r
	}
	x.recs = append(x.recs, record{})
	return &x.recs[len(x.recs)-1]
}

func (x *hasher) refsHash(i int, color []uint64) uint64 {
	refs := x.recs[i].refs
	h := uint64(len(refs))
	for _, id := range refs {
		h = mix(h ^ color[x.index[id]])
	}
	return h
}

// A plan is what walk needs of a type, worked out once.
type plan struct {
	fields   []int // a struct's fields that carry state
	object   bool  // a struct whose first field is its object ID
	engine   int   // View.Engine's field index, else -1
	cardData bool  // a pointer to card data
}

var plans sync.Map // reflect.Type → *plan

func planOf(t reflect.Type) *plan {
	if p, ok := plans.Load(t); ok {
		return p.(*plan)
	}
	p := &plan{engine: -1}
	switch t.Kind() {
	case reflect.Pointer:
		_, p.cardData = view.CardDataName(reflect.New(t.Elem()))
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			switch {
			case i == 0 && f.Name == "ID" && f.Type == objIDType:
				p.object = true
			case view.OpaqueField(t, f.Name):
			case t == viewType && f.Name == "Engine":
				p.engine = i
			default:
				p.fields = append(p.fields, i)
			}
		}
	}
	plans.Store(t, p)
	return p
}

var (
	objIDType = reflect.TypeOf(state.ObjID(0))
	viewType  = reflect.TypeOf(view.View{})
)

func (x *hasher) ref(r *record, id state.ObjID) {
	r.add(refToken)
	r.refs = append(r.refs, id)
	x.ids = mix(x.ids ^ uint64(id))
}

// walk hashes v into record ri. It takes an index, not a pointer: a new
// object's record may move the records.
func (x *hasher) walk(ri int, v reflect.Value) {
	t := v.Type()
	if t == objIDType {
		if id := state.ObjID(v.Uint()); id != 0 {
			x.ref(&x.recs[ri], id)
		} else {
			x.recs[ri].add(0)
		}
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			x.recs[ri].add(1)
		} else {
			x.recs[ri].add(2)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x.recs[ri].add(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		x.recs[ri].add(v.Uint())
	case reflect.Float32, reflect.Float64:
		x.recs[ri].add(uint64(v.Float() * 1e6))
	case reflect.String:
		x.recs[ri].add(str(v.String()))
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			x.recs[ri].add(nilToken)
			return
		}
		if v.Kind() == reflect.Pointer && planOf(t).cardData {
			name, _ := view.CardDataName(v)
			x.recs[ri].add(str(name))
			return
		}
		x.walk(ri, v.Elem())
	case reflect.Slice, reflect.Array:
		x.recs[ri].add(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			x.walk(ri, v.Index(i))
		}
	case reflect.Map:
		x.walkMap(ri, v)
	case reflect.Struct:
		x.walkStruct(ri, v, planOf(t))
	}
}

// walkStruct hashes a struct's fields that hold anything (an empty slice is
// nil's equal: a copy may hold either). An object (a card or stack object)
// is hashed into its own record and referenced here.
func (x *hasher) walkStruct(ri int, v reflect.Value, p *plan) {
	if p.object {
		if id := state.ObjID(v.Field(0).Uint()); id != 0 {
			x.ref(&x.recs[ri], id)
			x.record(id)
			ri = x.index[id]
		}
	}
	for _, i := range p.fields {
		f := v.Field(i)
		if f.IsZero() || (f.Kind() == reflect.Slice || f.Kind() == reflect.Map) && f.Len() == 0 {
			continue
		}
		x.recs[ri].add(uint64(i))
		x.walk(ri, f)
	}
	if p.engine >= 0 {
		x.walkText(ri, v.Field(p.engine))
	}
}

// walkMap hashes a map's entries in key order. The view's map keys are
// names and seats, so the order depends on no ID.
func (x *hasher) walkMap(ri int, v reflect.Value) {
	switch m := v.Interface().(type) {
	case view.Status:
		x.walkStatus(ri, m)
		return
	case view.Links:
		x.walkLinks(ri, m)
		return
	}
	start := len(x.keys)
	for it := v.MapRange(); it.Next(); {
		x.keys = append(x.keys, it.Key())
	}
	keys := x.keys[start:]
	slices.SortFunc(keys, compareKeys)
	x.recs[ri].add(uint64(len(keys)))
	for k := start; k < start+len(keys); k++ {
		x.walk(ri, x.keys[k])
		x.walk(ri, v.MapIndex(x.keys[k]))
	}
	x.keys = x.keys[:start]
}

// walkStatus and walkLinks are walkMap for the view's fact maps, the
// commonest, without reflection.
func (x *hasher) walkStatus(ri int, m view.Status) {
	keys := x.sortedKeys(len(m), func(add func(string)) {
		for k := range m {
			add(k)
		}
	})
	r := &x.recs[ri]
	r.add(uint64(len(keys)))
	for _, k := range keys {
		r.add(str(k))
		r.add(uint64(int64(m[k])))
	}
	x.strs = x.strs[:len(x.strs)-len(keys)]
}

func (x *hasher) walkLinks(ri int, m view.Links) {
	keys := x.sortedKeys(len(m), func(add func(string)) {
		for k := range m {
			add(k)
		}
	})
	x.recs[ri].add(uint64(len(keys)))
	for _, k := range keys {
		x.recs[ri].add(str(k))
		ids := m[k]
		x.recs[ri].add(uint64(len(ids)))
		for _, id := range ids {
			if id != 0 {
				x.ref(&x.recs[ri], id)
			} else {
				x.recs[ri].add(0)
			}
		}
	}
	x.strs = x.strs[:len(x.strs)-len(keys)]
}

// sortedKeys collects n keys onto x.strs and returns them sorted; the
// caller pops them.
func (x *hasher) sortedKeys(n int, each func(add func(string))) []string {
	start := len(x.strs)
	each(func(k string) { x.strs = append(x.strs, k) })
	keys := x.strs[start:]
	slices.Sort(keys)
	return keys
}

func compareKeys(a, b reflect.Value) int {
	switch a.Kind() {
	case reflect.String:
		return strings.Compare(a.String(), b.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmp.Compare(a.Int(), b.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cmp.Compare(a.Uint(), b.Uint())
	}
	return 0
}

// walkText hashes View.Engine: the engine's state as text, its object IDs
// written view.IDRef and the number, outside quoted strings.
func (x *hasher) walkText(ri int, v reflect.Value) {
	m := v.Interface().(map[string]string)
	start := len(x.keys)
	for k := range m {
		x.keys = append(x.keys, reflect.ValueOf(k))
	}
	keys := x.keys[start:]
	slices.SortFunc(keys, compareKeys)
	x.recs[ri].add(uint64(len(keys)))
	for _, k := range keys {
		r := &x.recs[ri]
		r.add(str(k.String()))
		s := m[k.String()]
		begin, quoted := 0, false
		for i := 0; i < len(s); i++ {
			switch c := s[i]; {
			case quoted && c == '\\':
				i++
			case c == '"':
				quoted = !quoted
			case !quoted && strings.HasPrefix(s[i:], view.IDRef):
				j := i + len(view.IDRef)
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					j++
				}
				id, _ := strconv.ParseUint(s[i+len(view.IDRef):j], 10, 32)
				r.add(str(s[begin:i]))
				x.ref(r, state.ObjID(id))
				begin, i = j, j-1
			}
		}
		r.add(str(s[begin:]))
	}
	x.keys = x.keys[:start]
}

const (
	refToken    = 0x9e3779b97f4a7c15
	nilToken    = 0xc2b2ae3d27d4eb4f
	goneToken   = 0x165667b19e3779f9
	playerToken = 0x27d4eb2f165667c5
)

// mix is splitmix64's finalizer (StateEncoder.mix64).
func mix(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// str is FNV-1a.
func str(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// Of hashes e's state at its pending decision over the omniscient view: the
// perfect-information pair. The decision's lazily built extensions (payment
// plans, the potential-mana walk's caches) are left out: they depend on
// which driver asked, not on the state.
func Of(e *rules.Engine) Hashes {
	var dc *decision.Decision
	if d := e.Pending(); d != nil {
		x := d.CloneValue()
		x.PaymentActions, x.PaymentActionsBuilt = nil, false
		x.PotentialCasts, x.PotentialUnpayableCasts, x.PotentialCastsKnown = nil, nil, false
		dc = &x
	}
	v := views.Get().(*view.View)
	defer views.Put(v)
	view.ProjectDynamicInto(v, e.G, e, view.NoSeat, view.Omniscient, nil, dc)
	return Hash(v)
}
