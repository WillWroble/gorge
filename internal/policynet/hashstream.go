package policynet

import "strings"

// Streaming forms of hashID: FNV-1a is a left fold over the bytes, so the
// hash of a concatenation is the fold continued over each part in turn.
// hashID(a + b + c) == fnvRow(fnvAdd(fnvAdd(fnvAdd(fnvOffset, a), b), c)),
// with no string built. A token prefix's state can be kept and continued
// for every token that shares it.

const (
	fnvOffset uint64 = 14695981039346656037
	fnvPrime  uint64 = 1099511628211
)

// fnvAdd folds s into h.
func fnvAdd(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

// fnvAddLower folds strings.ToLower(s) into h, without building it for an
// ASCII s.
func fnvAddLower(h uint64, s string) uint64 {
	if !isASCII(s) {
		return fnvAdd(h, strings.ToLower(s))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		h ^= uint64(c)
		h *= fnvPrime
	}
	return h
}

// fnvAddInt folds formatInt(v) into h.
func fnvAddInt(h uint64, v int64) uint64 {
	if v == 0 {
		h ^= '0'
		return h * fnvPrime
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	for ; i < len(digits); i++ {
		h ^= uint64(digits[i])
		h *= fnvPrime
	}
	return h
}

// fnvRow is hashID's masking of a folded hash.
func fnvRow(h uint64) uint16 { return uint16(h & rowMask) }

// mzClamp is mzBucket's clamp: v into 0..hi (mzBucket(v, hi) ==
// formatInt(mzClamp(v, hi))).
func mzClamp(v, hi int32) int64 {
	if v < 0 {
		v = 0
	}
	if v > hi {
		v = hi
	}
	return int64(v)
}

// sink collects hashed feature rows.
type sink struct{ fs []Feature }

// h appends the row of folded hash h.
func (s *sink) h(h uint64, val float32) { s.fs = append(s.fs, Feature{Row: fnvRow(h), Value: val}) }

// str appends hashID(str).
func (s *sink) str(str string, val float32) {
	s.fs = append(s.fs, Feature{Row: hashID(str), Value: val})
}

// fnvCounterString folds counterString(m) into h: "-" when empty, else
// "Kind=v" in sorted key order joined by ",".
func fnvCounterString(h uint64, m map[string]int32) uint64 {
	if len(m) == 0 {
		return fnvAdd(h, "-")
	}
	first := true
	eachCounterSorted(m, func(k string, n int32) {
		if !first {
			h = fnvAdd(h, ",")
		}
		first = false
		h = fnvAddInt(fnvAdd(fnvAdd(h, k), "="), int64(n))
	})
	return h
}
