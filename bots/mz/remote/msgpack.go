package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// The few MessagePack forms the model server's wire uses: maps of arrays of
// integers out, arrays of maps of floats back.

type packer struct{ b []byte }

func (p *packer) mapHeader(n int) {
	switch {
	case n < 16:
		p.b = append(p.b, 0x80|byte(n))
	case n <= math.MaxUint16:
		p.b = binary.BigEndian.AppendUint16(append(p.b, 0xde), uint16(n))
	default:
		p.b = binary.BigEndian.AppendUint32(append(p.b, 0xdf), uint32(n))
	}
}

func (p *packer) arrayHeader(n int) {
	switch {
	case n < 16:
		p.b = append(p.b, 0x90|byte(n))
	case n <= math.MaxUint16:
		p.b = binary.BigEndian.AppendUint16(append(p.b, 0xdc), uint16(n))
	default:
		p.b = binary.BigEndian.AppendUint32(append(p.b, 0xdd), uint32(n))
	}
}

func (p *packer) str(s string) {
	switch n := len(s); {
	case n < 32:
		p.b = append(p.b, 0xa0|byte(n))
	case n <= math.MaxUint8:
		p.b = append(p.b, 0xd9, byte(n))
	default:
		p.b = binary.BigEndian.AppendUint32(append(p.b, 0xdb), uint32(n))
	}
	p.b = append(p.b, s...)
}

// int writes v in its smallest form, as msgpack-java's packInt does.
func (p *packer) int(v int64) {
	switch {
	case v >= 0 && v < 128, v < 0 && v >= -32:
		p.b = append(p.b, byte(v))
	case v >= math.MinInt32 && v <= math.MaxInt32:
		p.b = binary.BigEndian.AppendUint32(append(p.b, 0xd2), uint32(int32(v)))
	default:
		p.b = binary.BigEndian.AppendUint64(append(p.b, 0xd3), uint64(v))
	}
}

func (p *packer) ints(key string, vs []int32) {
	p.str(key)
	p.arrayHeader(len(vs))
	for _, v := range vs {
		p.int(int64(v))
	}
}

func (p *packer) longs(key string, vs []int64) {
	p.str(key)
	p.arrayHeader(len(vs))
	for _, v := range vs {
		p.int(v)
	}
}

type unpacker struct {
	b []byte
	i int
}

var errShort = errors.New("msgpack: truncated")

func (u *unpacker) byte() (byte, error) {
	if u.i >= len(u.b) {
		return 0, errShort
	}
	c := u.b[u.i]
	u.i++
	return c, nil
}

func (u *unpacker) take(n int) ([]byte, error) {
	if n < 0 || u.i+n > len(u.b) {
		return nil, errShort
	}
	s := u.b[u.i : u.i+n]
	u.i += n
	return s, nil
}

func (u *unpacker) size(n int) (int, error) {
	s, err := u.take(n)
	if err != nil {
		return 0, err
	}
	switch n {
	case 1:
		return int(s[0]), nil
	case 2:
		return int(binary.BigEndian.Uint16(s)), nil
	}
	return int(binary.BigEndian.Uint32(s)), nil
}

func (u *unpacker) header(fix, mask byte, w16, w32 byte) (int, error) {
	c, err := u.byte()
	if err != nil {
		return 0, err
	}
	switch {
	case c&^mask == fix:
		return int(c & mask), nil
	case c == w16:
		return u.size(2)
	case c == w32:
		return u.size(4)
	}
	return 0, fmt.Errorf("msgpack: unexpected 0x%02x", c)
}

func (u *unpacker) arrayHeader() (int, error) { return u.header(0x90, 0x0f, 0xdc, 0xdd) }
func (u *unpacker) mapHeader() (int, error)   { return u.header(0x80, 0x0f, 0xde, 0xdf) }

func (u *unpacker) str() (string, error) {
	c, err := u.byte()
	if err != nil {
		return "", err
	}
	var n int
	switch {
	case c&0xe0 == 0xa0:
		n = int(c & 0x1f)
	case c == 0xd9:
		n, err = u.size(1)
	case c == 0xda:
		n, err = u.size(2)
	case c == 0xdb:
		n, err = u.size(4)
	default:
		return "", fmt.Errorf("msgpack: want a string, have 0x%02x", c)
	}
	if err != nil {
		return "", err
	}
	s, err := u.take(n)
	return string(s), err
}

// float reads a float or an integer as a float64 (unpackDouble).
func (u *unpacker) float() (float64, error) {
	c, err := u.byte()
	if err != nil {
		return 0, err
	}
	switch {
	case c == 0xca:
		s, err := u.take(4)
		if err != nil {
			return 0, err
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(s))), nil
	case c == 0xcb:
		s, err := u.take(8)
		if err != nil {
			return 0, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(s)), nil
	case c < 0x80:
		return float64(c), nil
	case c >= 0xe0:
		return float64(int8(c)), nil
	}
	u.i--
	v, err := u.integer()
	return float64(v), err
}

func (u *unpacker) integer() (int64, error) {
	c, err := u.byte()
	if err != nil {
		return 0, err
	}
	widths := map[byte]int{0xcc: 1, 0xcd: 2, 0xce: 4, 0xcf: 8, 0xd0: 1, 0xd1: 2, 0xd2: 4, 0xd3: 8}
	w, ok := widths[c]
	if !ok {
		return 0, fmt.Errorf("msgpack: want a number, have 0x%02x", c)
	}
	s, err := u.take(w)
	if err != nil {
		return 0, err
	}
	var x uint64
	for _, b := range s {
		x = x<<8 | uint64(b)
	}
	if c >= 0xd0 { // signed: sign-extend
		shift := 64 - 8*w
		return int64(x<<shift) >> shift, nil
	}
	return int64(x), nil
}

func (u *unpacker) floats() ([]float32, error) {
	n, err := u.arrayHeader()
	if err != nil {
		return nil, err
	}
	out := make([]float32, n)
	for i := range out {
		f, err := u.float()
		if err != nil {
			return nil, err
		}
		out[i] = float32(f)
	}
	return out, nil
}

// skip passes over one value of any form (skipValue).
func (u *unpacker) skip() error {
	c, err := u.byte()
	if err != nil {
		return err
	}
	n, items := 0, 0
	switch {
	case c < 0x80 || c >= 0xe0 || c == 0xc0 || c == 0xc2 || c == 0xc3:
		return nil
	case c&0xf0 == 0x80:
		items = 2 * int(c&0x0f)
	case c&0xf0 == 0x90:
		items = int(c & 0x0f)
	case c&0xe0 == 0xa0:
		n = int(c & 0x1f)
	case c == 0xc4 || c == 0xd9:
		n, err = u.size(1)
	case c == 0xc5 || c == 0xda:
		n, err = u.size(2)
	case c == 0xc6 || c == 0xdb:
		n, err = u.size(4)
	case c == 0xcc || c == 0xd0:
		n = 1
	case c == 0xcd || c == 0xd1:
		n = 2
	case c == 0xca || c == 0xce || c == 0xd2:
		n = 4
	case c == 0xcb || c == 0xcf || c == 0xd3:
		n = 8
	case c == 0xdc:
		items, err = u.size(2)
	case c == 0xdd:
		items, err = u.size(4)
	case c == 0xde:
		items, err = u.size(2)
		items *= 2
	case c == 0xdf:
		items, err = u.size(4)
		items *= 2
	default:
		return fmt.Errorf("msgpack: cannot skip 0x%02x", c)
	}
	if err != nil {
		return err
	}
	if _, err := u.take(n); err != nil {
		return err
	}
	for range items {
		if err := u.skip(); err != nil {
			return err
		}
	}
	return nil
}
