package remote

import (
	"encoding/binary"
	"io"
	"math"
	"net/http"
)

// Handler serves the model server's side of the wire with f, which scores
// one state from its node buckets: a stand-in model server for tests and
// smoke runs (the real one is the Python side's).
func Handler(f func(ids []int32) Result) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req, err := readRequest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ids, offs := req["indices"], req["offsets"]
		var p packer
		p.arrayHeader(len(offs))
		for i, off := range offs {
			end := int64(len(ids))
			if i+1 < len(offs) {
				end = offs[i+1]
			}
			state := make([]int32, end-off)
			for j := range state {
				state[j] = int32(ids[off+int64(j)])
			}
			res := f(state)
			p.mapHeader(4)
			p.floatArray("policy_priority", res.Priority)
			p.floatArray("policy_target", res.Target)
			p.floatArray("policy_binary", res.Binary)
			p.str("value")
			p.float(float64(res.Value))
		}
		w.Header().Set("Content-Type", "application/x-msgpack")
		w.Write(p.b)
	})
}

func (p *packer) float(f float64) {
	p.b = binary.BigEndian.AppendUint64(append(p.b, 0xcb), math.Float64bits(f))
}

func (p *packer) floatArray(key string, fs []float32) {
	p.str(key)
	p.arrayHeader(len(fs))
	for _, f := range fs {
		p.float(float64(f))
	}
}

// readRequest decodes a request map of integer arrays.
func readRequest(body []byte) (map[string][]int64, error) {
	u := unpacker{b: body}
	n, err := u.mapHeader()
	if err != nil {
		return nil, err
	}
	out := map[string][]int64{}
	for range n {
		k, err := u.str()
		if err != nil {
			return nil, err
		}
		m, err := u.arrayHeader()
		if err != nil {
			return nil, err
		}
		vs := make([]int64, m)
		for i := range vs {
			f, err := u.float()
			if err != nil {
				return nil, err
			}
			vs[i] = int64(f)
		}
		out[k] = vs
	}
	return out, nil
}
