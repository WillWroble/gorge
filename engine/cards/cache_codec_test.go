package cards

import (
	"fmt"
	"reflect"
	"testing"
)

// fillExported sets every exported field of the struct v points to a
// distinct non-zero value, recursing into the cache's nested types, so a
// field the face codec forgets to write cannot survive a round trip. A field
// of a type the filler does not know fails the test: a new field kind needs
// the codec (and this filler) extended.
func fillExported(t *testing.T, v reflect.Value, seed *int, depth int) {
	t.Helper()
	typ := v.Type()
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)
		if !sf.IsExported() {
			continue
		}
		fillValue(t, v.Field(i), sf.Name, seed, depth)
	}
}

func fillValue(t *testing.T, f reflect.Value, name string, seed *int, depth int) {
	t.Helper()
	*seed++
	switch f.Kind() {
	case reflect.String:
		f.SetString(fmt.Sprintf("%s-%d", name, *seed))
	case reflect.Map:
		m := reflect.MakeMap(f.Type())
		for k := 0; k < 3; k++ {
			m.SetMapIndex(reflect.ValueOf(fmt.Sprintf("%s-k%d-%d", name, k, *seed)), reflect.ValueOf(fmt.Sprintf("v%d", k)))
		}
		f.Set(m)
	case reflect.Slice:
		s := reflect.MakeSlice(f.Type(), 2, 2)
		for k := 0; k < 2; k++ {
			fillValue(t, s.Index(k), name, seed, depth)
		}
		f.Set(s)
	case reflect.Pointer:
		if depth >= 2 {
			return // leave deep chains nil: nil pointers round-trip too
		}
		p := reflect.New(f.Type().Elem())
		fillExported(t, p.Elem(), seed, depth+1)
		f.Set(p)
	case reflect.Struct:
		fillExported(t, f, seed, depth+1)
	default:
		t.Fatalf("face codec test: field %s has unhandled kind %s -- extend the codec and this filler", name, f.Kind())
	}
}

// TestFaceCodecRoundTrips pins the face codec (cache_codec.go) to every
// exported field of Face and the SA/Trigger/Static/Repl it nests: a field
// added to one of those types but not to the codec fails here instead of
// being silently dropped from the cache.
func TestFaceCodecRoundTrips(t *testing.T) {
	var in Face
	seed := 0
	fillExported(t, reflect.ValueOf(&in).Elem(), &seed, 0)
	b, err := in.GobEncode()
	if err != nil {
		t.Fatal(err)
	}
	var out Face
	if err := out.GobDecode(b); err != nil {
		t.Fatalf("GobDecode: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("face codec round trip differs:\n in: %+v\nout: %+v", in, out)
	}
	again, _ := out.GobEncode()
	if string(again) != string(b) {
		t.Fatal("re-encoding the decoded face changed its bytes")
	}
	// Every truncation is rejected, never a panic or a silent short face.
	for n := 0; n < len(b); n++ {
		var f Face
		if err := f.GobDecode(b[:n]); err == nil {
			t.Fatalf("GobDecode accepted a face truncated to %d of %d bytes", n, len(b))
		}
	}
}

// TestFaceCodecEmptyIsNil keeps gob's old decoding of empty collections: a
// zero-length slice or map comes back nil.
func TestFaceCodecEmptyIsNil(t *testing.T) {
	in := Face{Name: "x", Types: []string{}, SVars: map[string]string{}, Abilities: []*SA{{Kind: "SP", Params: map[string]string{}}}}
	b, _ := in.GobEncode()
	var out Face
	if err := out.GobDecode(b); err != nil {
		t.Fatal(err)
	}
	if out.Types != nil || out.SVars != nil || out.Abilities[0].Params != nil {
		t.Fatalf("empty collections decoded non-nil: %+v", out)
	}
}
