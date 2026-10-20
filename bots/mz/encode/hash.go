// Package encode is mz-gorge's state encoder: what the mage fork's
// StateEncoder and FeatureGraph are to MageZero. It reads only the view
// (principle 1 of mz-gorge-design.md). For now it gives a state's two
// hashes; the network's graph is stage 3.
package encode

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

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
	x := hasher{objs: map[state.ObjID]*record{}}
	var root record
	x.walk(&root, reflect.ValueOf(v).Elem())

	color := make(map[state.ObjID]uint64, len(x.objs))
	for id, r := range x.objs {
		color[id] = r.h
	}
	next := make(map[state.ObjID]uint64, len(x.objs))
	for range rounds {
		for id, r := range x.objs {
			next[id] = mix(r.h ^ refsHash(r.refs, color))
		}
		color, next = next, color
	}
	f := mix(root.h ^ refsHash(root.refs, color))
	return Hashes{Functional: f, Logical: mix(f ^ x.ids)}
}

// A record is one object's content, or the rest of the view's, with every
// object it names left as a reference.
type record struct {
	h    uint64
	refs []state.ObjID
}

func (r *record) add(h uint64) { r.h = mix(r.h ^ h) }

type hasher struct {
	objs map[state.ObjID]*record
	ids  uint64 // every ID in walk order: the logical part
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

func (x *hasher) walk(r *record, v reflect.Value) {
	if v.Type() == objIDType {
		if id := state.ObjID(v.Uint()); id != 0 {
			x.ref(r, id)
		} else {
			r.add(0)
		}
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			r.add(1)
		} else {
			r.add(2)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		r.add(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		r.add(v.Uint())
	case reflect.Float32, reflect.Float64:
		r.add(uint64(v.Float() * 1e6))
	case reflect.String:
		r.add(str(v.String()))
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			r.add(nilToken)
			return
		}
		if v.Kind() == reflect.Pointer {
			if name, ok := view.CardDataName(v); ok {
				r.add(str(name))
				return
			}
		}
		x.walk(r, v.Elem())
	case reflect.Slice, reflect.Array:
		r.add(uint64(v.Len()))
		for i := 0; i < v.Len(); i++ {
			x.walk(r, v.Index(i))
		}
	case reflect.Map:
		x.walkMap(r, v)
	case reflect.Struct:
		x.walkStruct(r, v)
	}
}

// walkStruct hashes a struct field by field. An object (a struct whose ID
// field is an object ID: a card or stack object) is hashed into its own
// record and referenced here.
func (x *hasher) walkStruct(r *record, v reflect.Value) {
	t := v.Type()
	first := 0
	if t.NumField() > 0 && t.Field(0).Name == "ID" && t.Field(0).Type == objIDType {
		id := state.ObjID(v.Field(0).Uint())
		if id != 0 {
			x.ref(r, id)
			if x.objs[id] == nil {
				x.objs[id] = &record{}
			}
			r = x.objs[id]
		}
		first = 1
	}
	for i := first; i < t.NumField(); i++ {
		if view.OpaqueField(t, t.Field(i).Name) {
			continue
		}
		r.add(uint64(i))
		if t == viewType && t.Field(i).Name == "Engine" {
			x.walkText(r, v.Field(i))
			continue
		}
		x.walk(r, v.Field(i))
	}
}

// walkMap hashes a map's entries in key order. The view's map keys are
// names and seats, so the order depends on no ID.
func (x *hasher) walkMap(r *record, v reflect.Value) {
	keys := v.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int {
		if a.Kind() == reflect.String {
			return strings.Compare(a.String(), b.String())
		}
		return strings.Compare(sortKey(a), sortKey(b))
	})
	r.add(uint64(len(keys)))
	for _, k := range keys {
		x.walk(r, k)
		x.walk(r, v.MapIndex(k))
	}
}

func sortKey(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int()+1<<62, 16)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 16)
	}
	return ""
}

// walkText hashes View.Engine: the engine's state as text, its object IDs
// written view.IDRef and the number, outside quoted strings.
func (x *hasher) walkText(r *record, v reflect.Value) {
	keys := v.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
	r.add(uint64(len(keys)))
	for _, k := range keys {
		r.add(str(k.String()))
		s := v.MapIndex(k).String()
		start, quoted := 0, false
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
				r.add(str(s[start:i]))
				x.ref(r, state.ObjID(id))
				start, i = j, j-1
			}
		}
		r.add(str(s[start:]))
	}
}

// refsHash hashes the colors of refs in order. A reference to no object
// is a player smuggled in an ID (state.PlayerRef) or a gone object.
func refsHash(refs []state.ObjID, color map[state.ObjID]uint64) uint64 {
	h := uint64(len(refs))
	for _, id := range refs {
		c, ok := color[id]
		if !ok {
			c = goneToken
			if p, isPlayer := id.PlayerRef(); isPlayer {
				c = mix(playerToken ^ uint64(p))
			}
		}
		h = mix(h ^ c)
	}
	return h
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
	v := view.ProjectDynamic(e.G, e, view.NoSeat, view.Omniscient, nil, dc)
	return Hash(&v)
}
