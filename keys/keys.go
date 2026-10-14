package keys

import (
	"crypto/mldsa"
	"encoding/binary"
	"errors"
	"unicode/utf8"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/tag"
)

const (
	MasterSize  = 32
	kindCounter = 0x00
	kindCustom  = 0x01
)

var ErrSalt = errors.New("keys: custom salt must be non-empty UTF-8")

type Salt struct {
	kind  byte
	value []byte
}

func Counter(n uint64) Salt {
	return Salt{kind: kindCounter, value: binary.BigEndian.AppendUint64(nil, n)}
}

func Custom(s string) (Salt, error) {
	if !utf8.ValidString(s) {
		return Salt{}, ErrSalt
	}
	s = NormalizeSalt(s)
	if s == "" {
		return Salt{}, ErrSalt
	}
	return Salt{kind: kindCustom, value: []byte(s)}, nil
}

func Seed(master *[MasterSize]byte, s Salt) [32]byte {
	d := digest.Tagged(tag.PublicationKey, master[:], []byte{s.kind}, s.value)
	var seed [32]byte
	copy(seed[:], d[:32])
	clear(d[:])
	return seed
}

func Derive(master *[MasterSize]byte, s Salt) (*mldsa.PrivateKey, error) {
	seed := Seed(master, s)
	defer clear(seed[:])
	return mldsa.NewPrivateKey(mldsa.MLDSA65(), seed[:])
}

func IdentityID(pk *mldsa.PublicKey) digest.Digest {
	return digest.Tagged(tag.IdentityID, pk.Bytes())
}

func PublicationDigest(pk *mldsa.PublicKey) digest.Digest {
	return digest.Tagged(tag.PublicationID, pk.Bytes())
}

func Address(pk *mldsa.PublicKey) addr.ID {
	return addr.FromDigest(PublicationDigest(pk))
}
