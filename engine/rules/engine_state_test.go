package rules

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestEngineStateSkipIsCurrent pins engineStateSkip to fields that exist and
// are clone:"deep": a renamed or removed field must leave the list, so the
// list never hides a new field by accident.
func TestEngineStateSkipIsCurrent(t *testing.T) {
	deep := map[string]bool{}
	var walk func(typ reflect.Type)
	walk = func(typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Anonymous {
				walk(f.Type)
			} else if strings.HasPrefix(f.Tag.Get("clone"), "deep") {
				deep[f.Name] = true
			}
		}
	}
	walk(reflect.TypeOf(Engine{}))
	if !sort.StringsAreSorted(engineStateSkip) {
		t.Error("engineStateSkip is not sorted")
	}
	for _, name := range engineStateSkip {
		if !deep[name] {
			t.Errorf("engineStateSkip names %q, which is not a clone:\"deep\" Engine field", name)
		}
	}
}
