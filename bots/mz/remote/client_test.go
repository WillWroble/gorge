package remote

import (
	"net/http/httptest"
	"testing"

	"github.com/adams-shaun/gorge/bots/mz/encode"
)

// TestBatchRoundTrip serves the wire with Handler: each state's value is
// its node count and its priority policy echoes its node buckets, so every
// result must come back to the state that asked, through batching.
func TestBatchRoundTrip(t *testing.T) {
	srv := httptest.NewServer(Handler(func(ids []int32) Result {
		r := Result{Value: float32(len(ids)), Binary: []float32{0, 1}}
		for _, id := range ids {
			r.Priority = append(r.Priority, float32(id))
		}
		r.Target = r.Priority
		return r
	}))
	defer srv.Close()
	c, err := Dial(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var chans []<-chan Result
	var states []encode.Arrays
	for n := 1; n <= 9; n++ {
		a := encode.Arrays{IDs: make([]int32, n), Values: make([]int32, n), EdgeChild: []int32{0}, EdgeParent: []int32{0}, EdgeLabel: []int32{-5}}
		for i := range a.IDs {
			a.IDs[i] = int32(100*n + i)
		}
		states = append(states, a)
		chans = append(chans, c.Infer(a))
	}
	for i, ch := range chans {
		r := <-ch
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if int(r.Value) != len(states[i].IDs) || len(r.Priority) != len(states[i].IDs) || r.Priority[0] != float32(states[i].IDs[0]) {
			t.Errorf("state %d: result %+v is not its own", i, r)
		}
	}
}
