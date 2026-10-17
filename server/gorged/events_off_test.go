package gorged

import (
	"errors"
	"testing"
)

// TestEventsNeedLabBuild: the platform gorged refuses -events rather than
// starting a server whose /api/events silently does not exist.
func TestEventsNeedLabBuild(t *testing.T) {
	_, _, err := config{events: true}.prepareEvents(nil, nil)
	if !errors.Is(err, errEventsNeedLab) {
		t.Fatalf("prepareEvents with -events on = %v, want errEventsNeedLab", err)
	}
	if _, _, err := (config{}).prepareEvents(nil, nil); err != nil {
		t.Fatalf("prepareEvents with -events off = %v, want nil", err)
	}
}
