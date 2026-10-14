package verify

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

type mapSource struct {
	revs, assets map[string]string
}

func (m mapSource) get(src map[string]string, id digest.Digest) ([]byte, error) {
	h, ok := src[hex.EncodeToString(id[:])]
	if !ok {
		return nil, errors.New("not found")
	}
	return hex.DecodeString(h)
}

func (m mapSource) Revision(id digest.Digest) ([]byte, error) { return m.get(m.revs, id) }
func (m mapSource) Asset(id digest.Digest) ([]byte, error)    { return m.get(m.assets, id) }

func TestVerificationVectors(t *testing.T) {
	var f struct {
		Cases []struct {
			ID        string            `json:"id"`
			Address   string            `json:"requested_address"`
			Head      string            `json:"head_envelope_hex"`
			Revisions map[string]string `json:"revision_envelopes"`
			Assets    map[string]string `json:"assets"`
			Held      *struct {
				PK   string `json:"public_key_hex"`
				Seq  uint64 `json:"sequence"`
				Head string `json:"head_object_hex"`
			} `json:"held"`
			Valid bool   `json:"expect_valid"`
			Check string `json:"expect_failed_check"`
		} `json:"cases"`
	}
	vectors.Load(t, "verification.json", &f)
	if len(f.Cases) < 10 {
		t.Fatalf("expected at least 10 cases, got %d", len(f.Cases))
	}
	for _, c := range f.Cases {
		want, err := addr.Parse(c.Address)
		if err != nil {
			t.Fatal(err)
		}
		var held *Held
		if c.Held != nil {
			held = &Held{PublicKey: vectors.Hex(t, c.Held.PK), Sequence: c.Held.Seq, HeadBytes: vectors.Hex(t, c.Held.Head)}
		}
		p, fail := Full(want, vectors.Hex(t, c.Head), held, mapSource{c.Revisions, c.Assets})
		switch {
		case c.Valid && fail != nil:
			t.Errorf("%s: %v", c.ID, fail)
		case c.Valid && len(p.Revisions) != 2:
			t.Errorf("%s: %d revisions verified", c.ID, len(p.Revisions))
		case !c.Valid && fail == nil:
			t.Errorf("%s: accepted, want failure at %q", c.ID, c.Check)
		case !c.Valid && fail.Check != c.Check:
			t.Errorf("%s: failed at %q, want %q", c.ID, fail.Check, c.Check)
		}
	}
}
