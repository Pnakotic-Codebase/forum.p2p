package addr

import (
	"crypto/subtle"
	"errors"

	"github.com/Pnakotic-Codebase/forum.p2p/digest"
)

const (
	Bits     = 110
	Length   = 22
	alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
)

var ErrInvalid = errors.New("addr: invalid identifier")

type ID [Length]byte

var decodeTable = func() [256]int8 {
	var t [256]int8
	for i := range t {
		t[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		c := alphabet[i]
		t[c] = int8(i)
		if c >= 'a' && c <= 'z' {
			t[c-'a'+'A'] = int8(i)
		}
	}
	for _, c := range []byte{'i', 'I', 'l', 'L'} {
		t[c] = 1
	}
	t['o'], t['O'] = 0, 0
	return t
}()

func FromDigest(d digest.Digest) ID {
	var id ID
	for i := 0; i < Length; i++ {
		bit := i * 5
		w := uint16(d[bit/8])<<8 | uint16(d[bit/8+1])
		id[i] = alphabet[(w>>(11-bit%8))&31]
	}
	return id
}

func (id ID) String() string {
	return string(id[:])
}

func Parse(s string) (ID, error) {
	var id ID
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' {
			continue
		}
		v := decodeTable[c]
		if v < 0 || n == Length {
			return ID{}, ErrInvalid
		}
		id[n] = alphabet[v]
		n++
	}
	if n != Length {
		return ID{}, ErrInvalid
	}
	return id, nil
}

func Matches(id ID, d digest.Digest) bool {
	want := FromDigest(d)
	return subtle.ConstantTimeCompare(id[:], want[:]) == 1
}
