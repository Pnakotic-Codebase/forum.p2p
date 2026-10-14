package frame

import (
	"encoding/binary"
	"errors"
	"math"
)

var ErrMalformed = errors.New("frame: malformed encoding")

func Encode(fields ...[]byte) []byte {
	n := 0
	for _, f := range fields {
		if uint64(len(f)) > math.MaxUint32 {
			panic("frame: field longer than 2^32-1 bytes")
		}
		n += 4 + len(f)
	}
	out := make([]byte, 0, n)
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}

func Decode(b []byte, count int) ([][]byte, error) {
	fields := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		if len(b) < 4 {
			return nil, ErrMalformed
		}
		l := uint64(binary.BigEndian.Uint32(b))
		b = b[4:]
		if uint64(len(b)) < l {
			return nil, ErrMalformed
		}
		fields = append(fields, b[:l:l])
		b = b[l:]
	}
	if len(b) != 0 {
		return nil, ErrMalformed
	}
	return fields, nil
}
