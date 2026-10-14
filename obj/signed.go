package obj

import (
	"crypto/mldsa"
	"errors"

	"github.com/Pnakotic-Codebase/forum.p2p/frame"
	"github.com/Pnakotic-Codebase/forum.p2p/tag"
)

const SignatureSize = 3309

var ErrSignature = errors.New("obj: signature does not verify")

func Seal(sk *mldsa.PrivateKey, object []byte) ([]byte, error) {
	sig, err := sk.Sign(nil, object, &mldsa.Options{})
	if err != nil {
		return nil, err
	}
	return frame.Encode([]byte(tag.Signed), object, sig), nil
}

func SealDeterministic(sk *mldsa.PrivateKey, object []byte) ([]byte, error) {
	sig, err := sk.SignDeterministic(object, &mldsa.Options{})
	if err != nil {
		return nil, err
	}
	return frame.Encode([]byte(tag.Signed), object, sig), nil
}

func Unseal(envelope []byte) (object, signature []byte, err error) {
	f, err := open(envelope, tag.Signed, 3)
	if err != nil {
		return nil, nil, err
	}
	if len(f[2]) != SignatureSize {
		return nil, nil, decodeErr("signature length")
	}
	return f[1], f[2], nil
}

func Check(publicKey, object, signature []byte) error {
	pk, err := mldsa.NewPublicKey(mldsa.MLDSA65(), publicKey)
	if err != nil {
		return ErrSignature
	}
	if mldsa.Verify(pk, object, signature, &mldsa.Options{}) != nil {
		return ErrSignature
	}
	return nil
}

type Bundle struct {
	Head      []byte
	Revisions [][]byte
	Assets    [][]byte
}

func (b *Bundle) Encode() []byte {
	return frame.Encode([]byte(tag.Bundle), b.Head, list(b.Revisions), list(b.Assets))
}

func DecodeBundle(b []byte) (*Bundle, error) {
	f, err := open(b, tag.Bundle, 4)
	if err != nil {
		return nil, err
	}
	out := &Bundle{Head: f[1]}
	if out.Revisions, err = readList(f[2], "bundle revisions"); err != nil {
		return nil, err
	}
	if out.Assets, err = readList(f[3], "bundle assets"); err != nil {
		return nil, err
	}
	return out, nil
}
