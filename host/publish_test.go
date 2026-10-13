package host

import (
	"testing"

	"github.com/adams-shaun/gorge/protocol"
)

func TestPublishPushesToSession(t *testing.T) {
	t.Parallel()
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := r.OpenSession()
	f1 := protocol.Frame{T: protocol.TEvent}
	f2 := protocol.Frame{T: protocol.TEvent, Table: "e1t"}
	if !r.Publish(s, f1) || !r.Publish(s, f2) {
		t.Fatal("Publish returned false for an open session")
	}
	got1, got2 := <-s.Out(), <-s.Out()
	if got1.ID != 1 || got2.ID != 2 {
		t.Fatalf("frame ids %d, %d, want 1, 2", got1.ID, got2.ID)
	}
	if got1.Table != "" || got2.Table != "e1t" {
		t.Fatalf("frames %+v %+v, want pushed payloads", got1, got2)
	}
	since, ok := s.Since(0)
	if !ok || len(since) != 2 || since[0].ID != 1 || since[1].ID != 2 {
		t.Fatalf("Since(0) = %+v, %v", since, ok)
	}
	r.CloseSession(s.ID)
	if r.Publish(s, protocol.Frame{T: protocol.TEvent}) {
		t.Fatal("Publish to a closed session returned true")
	}
}
