package gorged

import (
	"errors"
	"testing"
)

// TestEventsNeedAPlugin: the platform gorged refuses -events rather than
// starting a server whose /api/events silently does not exist.
func TestEventsNeedAPlugin(t *testing.T) {
	_, _, err := config{events: true}.prepareEvents(nil, nil)
	if !errors.Is(err, errEventsNeedPlugin) {
		t.Fatalf("prepareEvents with -events on = %v, want errEventsNeedPlugin", err)
	}
	if _, _, err := (config{}).prepareEvents(nil, nil); err != nil {
		t.Fatalf("prepareEvents with -events off = %v, want nil", err)
	}
}
