package obj

import (
	"regexp"

	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/frame"
	"github.com/Pnakotic-Codebase/forum.p2p/tag"
)

type Kind byte

const (
	Post Kind = 0x01
	Page Kind = 0x02
)

func (k Kind) Valid() bool { return k == Post || k == Page }

func (k Kind) String() string {
	switch k {
	case Post:
		return "post"
	case Page:
		return "page"
	}
	return "unknown"
}

const NonceSize = 16

type Nonce [NonceSize]byte

type Revision struct {
	Kind              Kind
	PublicationDigest digest.Digest
	DocumentNonce     Nonce
	Parent            *digest.Digest
	Created           int64
	Published         int64
	Title             string
	Body              string
	Assets            []digest.Digest
	References        []string
}

func (r *Revision) Encode() []byte {
	var parent []byte
	if r.Parent != nil {
		parent = r.Parent[:]
	}
	return frame.Encode(
		[]byte(tag.Revision),
		[]byte{byte(r.Kind)},
		r.PublicationDigest[:],
		r.DocumentNonce[:],
		parent,
		ms(r.Created),
		ms(r.Published),
		[]byte(r.Title),
		[]byte(r.Body),
		digestList(r.Assets),
		textList(r.References),
	)
}

func DecodeRevision(b []byte) (*Revision, error) {
	f, err := open(b, tag.Revision, 11)
	if err != nil {
		return nil, err
	}
	r := &Revision{}
	if len(f[1]) != 1 || !Kind(f[1][0]).Valid() {
		return nil, decodeErr("revision kind")
	}
	r.Kind = Kind(f[1][0])
	if r.PublicationDigest, err = readDigest(f[2], "revision publication_digest"); err != nil {
		return nil, err
	}
	if len(f[3]) != NonceSize {
		return nil, decodeErr("revision document_nonce")
	}
	copy(r.DocumentNonce[:], f[3])
	if len(f[4]) != 0 {
		p, err := readDigest(f[4], "revision parent_revision_id")
		if err != nil {
			return nil, err
		}
		r.Parent = &p
	}
	if r.Created, err = readMs(f[5], "revision created"); err != nil {
		return nil, err
	}
	if r.Published, err = readMs(f[6], "revision published"); err != nil {
		return nil, err
	}
	if r.Title, err = readLine(f[7], "revision title"); err != nil {
		return nil, err
	}
	if r.Body, err = readText(f[8], "revision body"); err != nil {
		return nil, err
	}
	if r.Assets, err = readDigestList(f[9], "revision assets"); err != nil {
		return nil, err
	}
	if r.References, err = readTextList(f[10], "revision references"); err != nil {
		return nil, err
	}
	return r, nil
}

func RevisionID(revisionBytes []byte) digest.Digest {
	return digest.Tagged(tag.RevisionID, revisionBytes)
}

func DocumentDigest(publication digest.Digest, nonce Nonce) digest.Digest {
	return digest.Tagged(tag.DocumentID, publication[:], nonce[:])
}

var (
	referenceRE = regexp.MustCompile(`forum://[0-9a-hjkmnp-tv-z]{22}(?:/[0-9a-hjkmnp-tv-z]{22})?`)
	assetRefRE  = regexp.MustCompile(`asset:([0-9a-f]{128})`)
)

func ExtractReferences(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range referenceRE.FindAllString(body, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func AssetRefs(body string) []string {
	var out []string
	for _, m := range assetRefRE.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

type Asset struct {
	MediaType string
	Content   []byte
}

func (a *Asset) Encode() []byte {
	return frame.Encode([]byte(tag.Asset), []byte(a.MediaType), a.Content)
}

func DecodeAsset(b []byte) (*Asset, error) {
	f, err := open(b, tag.Asset, 3)
	if err != nil {
		return nil, err
	}
	mt, err := readLine(f[1], "asset media_type")
	if err != nil {
		return nil, err
	}
	return &Asset{MediaType: mt, Content: f[2]}, nil
}

func AssetID(assetBytes []byte) digest.Digest {
	return digest.Tagged(tag.AssetID, assetBytes)
}

type Entry struct {
	Kind          Kind
	DocumentNonce Nonce
	Revision      digest.Digest
}

type Head struct {
	PublicKey   []byte
	Sequence    uint64
	Updated     int64
	Title       string
	Description string
	Entries     []Entry
	KnownCopies []string
}

func (h *Head) Encode() []byte {
	entries := make([][]byte, len(h.Entries))
	for i, e := range h.Entries {
		entries[i] = frame.Encode([]byte{byte(e.Kind)}, e.DocumentNonce[:], e.Revision[:])
	}
	return frame.Encode(
		[]byte(tag.Head),
		h.PublicKey,
		u64(h.Sequence),
		ms(h.Updated),
		[]byte(h.Title),
		[]byte(h.Description),
		list(entries),
		textList(h.KnownCopies),
	)
}

const PublicKeySize = 1952

func DecodeHead(b []byte) (*Head, error) {
	f, err := open(b, tag.Head, 8)
	if err != nil {
		return nil, err
	}
	h := &Head{}
	if len(f[1]) != PublicKeySize {
		return nil, decodeErr("head public_key")
	}
	h.PublicKey = f[1]
	if h.Sequence, err = readU64(f[2], "head sequence"); err != nil {
		return nil, err
	}
	if h.Updated, err = readMs(f[3], "head updated"); err != nil {
		return nil, err
	}
	if h.Title, err = readLine(f[4], "head title"); err != nil {
		return nil, err
	}
	if h.Description, err = readText(f[5], "head description"); err != nil {
		return nil, err
	}
	items, err := readList(f[6], "head entries")
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		p, err := frame.Decode(it, 3)
		if err != nil || len(p[0]) != 1 || !Kind(p[0][0]).Valid() || len(p[1]) != NonceSize {
			return nil, decodeErr("head entry")
		}
		e := Entry{Kind: Kind(p[0][0])}
		copy(e.DocumentNonce[:], p[1])
		if e.Revision, err = readDigest(p[2], "head entry revision"); err != nil {
			return nil, err
		}
		h.Entries = append(h.Entries, e)
	}
	if h.KnownCopies, err = readTextList(f[7], "head known_copies"); err != nil {
		return nil, err
	}
	return h, nil
}

func PublicationDigest(publicKey []byte) digest.Digest {
	return digest.Tagged(tag.PublicationID, publicKey)
}
