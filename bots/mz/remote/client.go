// Package remote is the mage fork's RemoteModelEvaluator: it sends encoded
// states to the MageZero model server in batches and returns the network's
// policies and value. Its wire is the fork's: a msgpack map of the batch's
// graphs in concatenated CSR form, answered by an array of result maps.
package remote

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/adams-shaun/gorge/bots/mz/encode"
)

// RemoteModelEvaluator's batching controls.
const (
	BatchInterval = 2500 * time.Microsecond // batchInterval
	MaxBatchSize  = 4                       // maxBatchSize
	MaxCalls      = 16                      // MAX_CONCURRENT_CALLS
)

// A Result is the network's output for one state (InferenceResult). The
// policies hold one score per node, in the state's Arrays order; Binary is
// the CHOOSE_USE pair (false, true).
type Result struct {
	Priority, Target []float32
	Binary           []float32
	Value            float32
	Err              error
}

// A Client evaluates states against one model server.
type Client struct {
	url     string
	http    *http.Client
	permits chan struct{}

	mu      sync.Mutex
	pending []*request
	stop    chan struct{}
}

type request struct {
	a    encode.Arrays
	done chan Result
}

// Dial connects to the model server at base (http://127.0.0.1:50052), which
// must answer its health check, and starts the batch flusher.
func Dial(base string) (*Client, error) {
	c := &Client{url: base + "/evaluate", http: &http.Client{}, permits: make(chan struct{}, MaxCalls), stop: make(chan struct{})}
	resp, err := c.http.Get(base + "/healthz")
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote: health check failed: HTTP %d", resp.StatusCode)
	}
	go func() {
		t := time.NewTicker(BatchInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.flush()
			case <-c.stop:
				return
			}
		}
	}()
	return c, nil
}

// Close stops the flusher.
func (c *Client) Close() { close(c.stop) }

// Infer queues a state; its result arrives on the returned channel
// (inferAsync).
func (c *Client) Infer(a encode.Arrays) <-chan Result {
	r := &request{a: a, done: make(chan Result, 1)}
	c.mu.Lock()
	c.pending = append(c.pending, r)
	full := len(c.pending) >= MaxBatchSize
	c.mu.Unlock()
	if full {
		c.flush()
	}
	return r.done
}

// flush sends up to MaxBatchSize queued states if a call slot is free
// (flushIfAny).
func (c *Client) flush() {
	select {
	case c.permits <- struct{}{}:
	default:
		return
	}
	c.mu.Lock()
	n := min(len(c.pending), MaxBatchSize)
	batch := append([]*request(nil), c.pending[:n]...)
	c.pending = c.pending[n:]
	c.mu.Unlock()
	if n == 0 {
		<-c.permits
		return
	}
	go func() {
		defer func() { <-c.permits }()
		results, err := c.call(batch)
		for i, r := range batch {
			if err != nil {
				r.done <- Result{Err: err}
			} else {
				r.done <- results[i]
			}
		}
	}()
}

func (c *Client) call(batch []*request) ([]Result, error) {
	var ids, values, child, parent, label []int32
	var offsets, edgeOffsets []int64
	for _, r := range batch {
		offsets = append(offsets, int64(len(ids)))
		edgeOffsets = append(edgeOffsets, int64(len(child)))
		ids, values = append(ids, r.a.IDs...), append(values, r.a.Values...)
		child, parent, label = append(child, r.a.EdgeChild...), append(parent, r.a.EdgeParent...), append(label, r.a.EdgeLabel...)
	}
	var p packer
	p.mapHeader(7)
	p.ints("indices", ids)
	p.ints("values", values)
	p.longs("offsets", offsets)
	p.ints("edge_child", child)
	p.ints("edge_parent", parent)
	p.ints("edge_label", label)
	p.longs("edge_offsets", edgeOffsets)

	resp, err := c.http.Post(c.url, "application/x-msgpack", bytes.NewReader(p.b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote: HTTP %d from model server", resp.StatusCode)
	}
	return decode(body, batch)
}

func decode(body []byte, batch []*request) ([]Result, error) {
	u := unpacker{b: body}
	n, err := u.arrayHeader()
	if err != nil {
		return nil, err
	}
	if n != len(batch) {
		return nil, fmt.Errorf("remote: batch size mismatch: sent %d got %d", len(batch), n)
	}
	out := make([]Result, n)
	for i := range out {
		if out[i], err = decodeOne(&u, len(batch[i].a.IDs)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func decodeOne(u *unpacker, nodes int) (Result, error) {
	var r Result
	n, err := u.mapHeader()
	if err != nil {
		return r, err
	}
	for range n {
		k, err := u.str()
		if err != nil {
			return r, err
		}
		switch k {
		case "policy_priority":
			r.Priority, err = u.floats()
		case "policy_target":
			r.Target, err = u.floats()
		case "policy_binary":
			r.Binary, err = u.floats()
		case "value":
			var v float64
			v, err = u.float()
			r.Value = float32(v)
		default:
			err = u.skip()
		}
		if err != nil {
			return r, err
		}
	}
	for _, p := range [][]float32{r.Priority, r.Target} {
		if p != nil && len(p) != nodes {
			return r, fmt.Errorf("remote: node count mismatch: sent %d got %d", nodes, len(p))
		}
	}
	if r.Binary != nil && len(r.Binary) != 2 {
		return r, errors.New("remote: policy_binary is not a pair")
	}
	return r, nil
}
