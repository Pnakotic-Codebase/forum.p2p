package verify

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
)

type Source interface {
	Revision(id digest.Digest) ([]byte, error)
	Asset(id digest.Digest) ([]byte, error)
}

type Held struct {
	PublicKey []byte
	Sequence  uint64
	HeadBytes []byte
}

type Failure struct {
	Check  string
	Detail string
}

func (f *Failure) Error() string {
	return "unverified: " + f.Check + ": " + f.Detail
}

func fail(check, format string, a ...any) *Failure {
	return &Failure{Check: check, Detail: fmt.Sprintf(format, a...)}
}

type Publication struct {
	Address     addr.ID
	Digest      digest.Digest
	HeadEnv     []byte
	Head        *obj.Head
	RevisionEnv map[digest.Digest][]byte
	Revisions   map[digest.Digest]*obj.Revision
	Assets      map[digest.Digest][]byte
}

func Head(want addr.ID, env []byte, held *Held) (*obj.Head, digest.Digest, *Failure) {
	var none digest.Digest
	object, sig, err := obj.Unseal(env)
	if err != nil {
		return nil, none, fail("head envelope", "%v", err)
	}
	h, err := obj.DecodeHead(object)
	if err != nil {
		return nil, none, fail("head decode", "%v", err)
	}
	if err := obj.Check(h.PublicKey, object, sig); err != nil {
		return nil, none, fail("head signature", "%v", err)
	}
	pd := obj.PublicationDigest(h.PublicKey)
	if !addr.Matches(want, pd) {
		return nil, none, fail("publication address", "key hashes to %s, requested %s", addr.FromDigest(pd), want)
	}
	if held != nil {
		if !bytes.Equal(held.PublicKey, h.PublicKey) {
			return nil, none, fail("key pinning", "public key differs from the one already held")
		}
		if h.Sequence < held.Sequence || (h.Sequence == held.Sequence && !bytes.Equal(held.HeadBytes, object)) {
			return nil, none, fail("head sequence", "sequence %d does not advance past held %d", h.Sequence, held.Sequence)
		}
	}
	return h, pd, nil
}

func Revision(pubKey []byte, pd digest.Digest, e obj.Entry, env []byte) (*obj.Revision, *Failure) {
	object, sig, err := obj.Unseal(env)
	if err != nil {
		return nil, fail("revision envelope", "%v", err)
	}
	if id := obj.RevisionID(object); id != e.Revision {
		return nil, fail("revision id", "content hashes to %s, head lists %s", short(id), short(e.Revision))
	}
	r, err := obj.DecodeRevision(object)
	if err != nil {
		return nil, fail("revision decode", "%v", err)
	}
	if err := obj.Check(pubKey, object, sig); err != nil {
		return nil, fail("revision signature", "%s: %v", short(e.Revision), err)
	}
	if r.PublicationDigest != pd || r.Kind != e.Kind || r.DocumentNonce != e.DocumentNonce {
		return nil, fail("revision binding", "%s does not match its head entry", short(e.Revision))
	}
	listed := map[string]bool{}
	for _, a := range r.Assets {
		listed[hex.EncodeToString(a[:])] = true
	}
	for _, ref := range obj.AssetRefs(r.Body) {
		if !listed[ref] {
			return nil, fail("revision assets", "body references asset %s… that is not listed", ref[:16])
		}
	}
	return r, nil
}

func Asset(id digest.Digest, b []byte) *Failure {
	if obj.AssetID(b) != id {
		return fail("asset id", "content of %s does not hash to its id", short(id))
	}
	if _, err := obj.DecodeAsset(b); err != nil {
		return fail("asset decode", "%v", err)
	}
	return nil
}

func Full(want addr.ID, headEnv []byte, held *Held, src Source) (*Publication, *Failure) {
	h, pd, f := Head(want, headEnv, held)
	if f != nil {
		return nil, f
	}
	p := &Publication{
		Address: want, Digest: pd, HeadEnv: headEnv, Head: h,
		RevisionEnv: map[digest.Digest][]byte{},
		Revisions:   map[digest.Digest]*obj.Revision{},
		Assets:      map[digest.Digest][]byte{},
	}
	for _, e := range h.Entries {
		env, err := src.Revision(e.Revision)
		if err != nil {
			return nil, fail("revision fetch", "%s: %v", short(e.Revision), err)
		}
		r, f := Revision(h.PublicKey, pd, e, env)
		if f != nil {
			return nil, f
		}
		p.RevisionEnv[e.Revision] = env
		p.Revisions[e.Revision] = r
		for _, a := range r.Assets {
			if _, ok := p.Assets[a]; ok {
				continue
			}
			b, err := src.Asset(a)
			if err != nil {
				return nil, fail("asset fetch", "%s: %v", short(a), err)
			}
			if f := Asset(a, b); f != nil {
				return nil, f
			}
			p.Assets[a] = b
		}
	}
	return p, nil
}

func short(d digest.Digest) string {
	return hex.EncodeToString(d[:8])
}
