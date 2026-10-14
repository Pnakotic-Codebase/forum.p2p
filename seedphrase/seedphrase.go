package seedphrase

import (
	"crypto/subtle"
	"errors"

	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/tag"
)

const (
	SeedSize     = 32
	Words        = 23
	ChecksumBits = 20
	packed       = 35
)

var ErrKeyNotFound = errors.New("the key was not found")

func checksum(seed *[SeedSize]byte) [3]byte {
	h := digest.Tagged(tag.SeedChecksum, seed[:])
	return [3]byte{h[0], h[1], h[2] & 0xf0}
}

func get12(b *[packed]byte, i int) int {
	o := i * 12 / 8
	if i%2 == 0 {
		return int(b[o])<<4 | int(b[o+1]>>4)
	}
	return int(b[o]&0x0f)<<8 | int(b[o+1])
}

func set12(b *[packed]byte, i, v int) {
	o := i * 12 / 8
	if i%2 == 0 {
		b[o] = byte(v >> 4)
		b[o+1] = b[o+1]&0x0f | byte(v<<4)
		return
	}
	b[o] = b[o]&0xf0 | byte(v>>8)
	b[o+1] = byte(v)
}

func Encode(seed [SeedSize]byte) []string {
	var buf [packed]byte
	copy(buf[:SeedSize], seed[:])
	c := checksum(&seed)
	copy(buf[SeedSize:], c[:])
	out := make([]string, Words)
	for i := range out {
		out[i] = words[get12(&buf, i)]
	}
	return out
}

func lookup(w string) (int, int) {
	p := pad(w)
	idx, found := 0, 0
	for i := 0; i < ListSize; i++ {
		eq := subtle.ConstantTimeCompare(p[:], table[i][:])
		idx |= subtle.ConstantTimeSelect(eq, i, 0)
		found |= eq
	}
	return idx, found
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

func trim(w string) string {
	i, j := 0, len(w)
	for i < j && isSpace(w[i]) {
		i++
	}
	for j > i && isSpace(w[j-1]) {
		j--
	}
	return w[i:j]
}

func split(text string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(text); i++ {
		if i == len(text) || isSpace(text[i]) {
			if start >= 0 {
				out = append(out, text[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

func RestoreText(text string) ([SeedSize]byte, error) {
	return Restore(split(text))
}

func Restore(phrase []string) ([SeedSize]byte, error) {
	ok := subtle.ConstantTimeEq(int32(len(phrase)), Words)
	var buf [packed]byte
	for i := 0; i < Words; i++ {
		w := ""
		if i < len(phrase) {
			w = phrase[i]
		}
		idx, found := lookup(trim(w))
		ok &= found
		set12(&buf, i, idx)
	}
	var seed [SeedSize]byte
	copy(seed[:], buf[:SeedSize])
	c := checksum(&seed)
	ok &= subtle.ConstantTimeCompare(c[:], buf[SeedSize:])
	clear(buf[:])
	if ok != 1 {
		clear(seed[:])
		return seed, ErrKeyNotFound
	}
	return seed, nil
}
