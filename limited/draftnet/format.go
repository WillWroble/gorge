// Package draftnet is the draft-RL policy network (draft-RL spec §5): a small
// pre-LN transformer over one pick's tokens -- a context token, one token per
// other seat, the pool and the candidate pack cards -- that scores each
// candidate. Weights arrive as a .dpkw byte slice; the package never touches
// a file. The trainer (out of repo, draftrl/py/export.py) writes the format.
//
// The package is pure: it imports only the limited vocabulary, draft's
// PickView and draftfeat's features. Float reductions run in a fixed order.
package draftnet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// The .dpkw layout, all little endian:
//
//	magic   "DPKW"
//	u32     version (1)
//	u32     meta length, then that many bytes of free-form meta (the
//	        trainer writes JSON: set, set-table hash, iteration)
//	u32     tensor count
//	per tensor:
//	  u16   name length, name bytes
//	  u8    rank, then rank × u32 dims
//	  f32   prod(dims) values, row major
const (
	magic   = "DPKW"
	version = 1
)

// Tensor is one named float32 tensor of a weights file.
type Tensor struct {
	Name  string
	Shape []int
	Data  []float32
}

// File is a decoded .dpkw file.
type File struct {
	Meta    []byte
	Tensors []Tensor
}

// Decode parses a .dpkw byte slice without checking the tensor names; Load
// is the policy entry point that does.
func Decode(b []byte) (*File, error) {
	r := reader{b: b}
	if string(r.bytes(4)) != magic {
		return nil, errors.New("draftnet: not a .dpkw file")
	}
	if v := r.u32(); v != version && r.err == nil {
		return nil, fmt.Errorf("draftnet: unsupported .dpkw version %d", v)
	}
	f := &File{Meta: append([]byte(nil), r.bytes(int(r.u32()))...)}
	n := int(r.u32())
	for i := 0; i < n && r.err == nil; i++ {
		var t Tensor
		t.Name = string(r.bytes(int(r.u16())))
		rank := int(r.u8())
		size := 1
		for d := 0; d < rank; d++ {
			x := int(r.u32())
			t.Shape = append(t.Shape, x)
			size *= x
			if size > len(b) {
				r.err = errors.New("draftnet: tensor larger than the file")
			}
		}
		raw := r.bytes(4 * size)
		if r.err != nil {
			break
		}
		t.Data = make([]float32, size)
		for j := range t.Data {
			t.Data[j] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*j:]))
		}
		f.Tensors = append(f.Tensors, t)
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.off != len(b) {
		return nil, fmt.Errorf("draftnet: %d trailing bytes", len(b)-r.off)
	}
	return f, nil
}

// Encode writes f as .dpkw bytes.
func (f *File) Encode() []byte {
	var b []byte
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint32(b, version)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(f.Meta)))
	b = append(b, f.Meta...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(f.Tensors)))
	for _, t := range f.Tensors {
		b = binary.LittleEndian.AppendUint16(b, uint16(len(t.Name)))
		b = append(b, t.Name...)
		b = append(b, byte(len(t.Shape)))
		for _, d := range t.Shape {
			b = binary.LittleEndian.AppendUint32(b, uint32(d))
		}
		for _, x := range t.Data {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
		}
	}
	return b
}

type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = errors.New("draftnet: truncated .dpkw file")
		return nil
	}
	s := r.b[r.off : r.off+n]
	r.off += n
	return s
}

func (r *reader) u8() uint8 {
	s := r.bytes(1)
	if s == nil {
		return 0
	}
	return s[0]
}

func (r *reader) u16() uint16 {
	s := r.bytes(2)
	if s == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(s)
}

func (r *reader) u32() uint32 {
	s := r.bytes(4)
	if s == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(s)
}
