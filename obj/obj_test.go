package obj

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

type file struct {
	Assets []struct {
		MediaType string `json:"media_type"`
		Content   string `json:"content_hex"`
		Encoded   string `json:"encoded_hex"`
		ID        string `json:"asset_id_hex"`
	} `json:"assets"`
	Revisions []struct {
		Kind      byte     `json:"kind"`
		PubDigest string   `json:"publication_digest_hex"`
		Nonce     string   `json:"document_nonce_hex"`
		Parent    string   `json:"parent_revision_id_hex"`
		Created   int64    `json:"created"`
		Published int64    `json:"published"`
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Assets    []string `json:"assets"`
		Refs      []string `json:"references"`
		Encoded   string   `json:"encoded_hex"`
		ID        string   `json:"revision_id_hex"`
		DocDigest string   `json:"document_digest_hex"`
		DocAddr   string   `json:"document_address"`
	} `json:"revisions"`
	Head struct {
		PK          string   `json:"public_key_hex"`
		Sequence    uint64   `json:"sequence"`
		Updated     int64    `json:"updated"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		KnownCopies []string `json:"known_copies"`
		Entries     []struct {
			Kind  byte   `json:"kind"`
			Nonce string `json:"document_nonce_hex"`
			Rev   string `json:"revision_id_hex"`
		} `json:"entries"`
		Encoded string `json:"encoded_hex"`
		Address string `json:"publication_address"`
	} `json:"head"`
}

func dg(t *testing.T, s string) digest.Digest {
	var d digest.Digest
	copy(d[:], vectors.Hex(t, s))
	return d
}

func TestObjectVectors(t *testing.T) {
	var f file
	vectors.Load(t, "objects.json", &f)
	for i, a := range f.Assets {
		x := Asset{MediaType: a.MediaType, Content: vectors.Hex(t, a.Content)}
		enc := x.Encode()
		if hex.EncodeToString(enc) != a.Encoded {
			t.Errorf("asset %d: encoding mismatch", i)
		}
		if id := AssetID(enc); hex.EncodeToString(id[:]) != a.ID {
			t.Errorf("asset %d: id mismatch", i)
		}
		back, err := DecodeAsset(enc)
		if err != nil || back.MediaType != a.MediaType || !bytes.Equal(back.Content, x.Content) {
			t.Errorf("asset %d: decode mismatch", i)
		}
	}
	for i, v := range f.Revisions {
		r := Revision{Kind: Kind(v.Kind), PublicationDigest: dg(t, v.PubDigest), Created: v.Created,
			Published: v.Published, Title: v.Title, Body: v.Body, References: v.Refs}
		copy(r.DocumentNonce[:], vectors.Hex(t, v.Nonce))
		if v.Parent != "" {
			p := dg(t, v.Parent)
			r.Parent = &p
		}
		for _, a := range v.Assets {
			r.Assets = append(r.Assets, dg(t, a))
		}
		enc := r.Encode()
		if hex.EncodeToString(enc) != v.Encoded {
			t.Fatalf("revision %d: encoding mismatch", i)
		}
		if id := RevisionID(enc); hex.EncodeToString(id[:]) != v.ID {
			t.Errorf("revision %d: id mismatch", i)
		}
		dd := DocumentDigest(r.PublicationDigest, r.DocumentNonce)
		if hex.EncodeToString(dd[:]) != v.DocDigest || addr.FromDigest(dd).String() != v.DocAddr {
			t.Errorf("revision %d: document id mismatch", i)
		}
		back, err := DecodeRevision(enc)
		if err != nil {
			t.Fatalf("revision %d: %v", i, err)
		}
		if !bytes.Equal(back.Encode(), enc) {
			t.Errorf("revision %d: decode/encode round trip differs", i)
		}
		if got := ExtractReferences(v.Body); len(got) != len(v.Refs) {
			t.Errorf("revision %d: extracted %v want %v", i, got, v.Refs)
		}
	}
	h := Head{PublicKey: vectors.Hex(t, f.Head.PK), Sequence: f.Head.Sequence, Updated: f.Head.Updated,
		Title: f.Head.Title, Description: f.Head.Description, KnownCopies: f.Head.KnownCopies}
	for _, e := range f.Head.Entries {
		x := Entry{Kind: Kind(e.Kind), Revision: dg(t, e.Rev)}
		copy(x.DocumentNonce[:], vectors.Hex(t, e.Nonce))
		h.Entries = append(h.Entries, x)
	}
	enc := h.Encode()
	if hex.EncodeToString(enc) != f.Head.Encoded {
		t.Fatal("head: encoding mismatch")
	}
	back, err := DecodeHead(enc)
	if err != nil || !bytes.Equal(back.Encode(), enc) {
		t.Fatalf("head: decode round trip failed: %v", err)
	}
	if addr.FromDigest(PublicationDigest(h.PublicKey)).String() != f.Head.Address {
		t.Error("head: publication address mismatch")
	}
}

func TestDecodeRejects(t *testing.T) {
	r := Revision{Kind: Post, Title: "t", Body: "b"}
	enc := r.Encode()
	if _, err := DecodeRevision(append(enc, 0)); err == nil {
		t.Error("trailing byte accepted")
	}
	bad := Revision{Kind: Post, Title: "two\nlines"}
	if _, err := DecodeRevision(bad.Encode()); err == nil {
		t.Error("title with line break accepted")
	}
	bad = Revision{Kind: 7}
	if _, err := DecodeRevision(bad.Encode()); err == nil {
		t.Error("unknown kind accepted")
	}
	bad = Revision{Kind: Post, Body: "\xff"}
	if _, err := DecodeRevision(bad.Encode()); err == nil {
		t.Error("invalid UTF-8 accepted")
	}
	if _, err := DecodeHead(enc); err == nil {
		t.Error("revision accepted as head")
	}
}
