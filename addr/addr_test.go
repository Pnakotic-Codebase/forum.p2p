package addr

import (
	"encoding/hex"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
	"github.com/Pnakotic-Codebase/forum.p2p/tag"
)

type file struct {
	Address []struct {
		ID         string `json:"id"`
		PublicKey  string `json:"public_key_hex"`
		PubDigest  string `json:"publication_digest_hex"`
		PubID      string `json:"publication_id"`
		IdentityID string `json:"identity_id_hex"`
	} `json:"address_vectors"`
	Decode []struct {
		ID       string `json:"id"`
		Input    string `json:"input"`
		Valid    bool   `json:"expect_valid"`
		Expected string `json:"expected_id"`
	} `json:"address_decode_vectors"`
}

func TestAddressVectors(t *testing.T) {
	var f file
	vectors.Load(t, "identifiers.json", &f)
	if len(f.Address) == 0 || len(f.Decode) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range f.Address {
		var d digest.Digest
		if v.PublicKey != "" {
			pk := vectors.Hex(t, v.PublicKey)
			d = digest.Tagged(tag.PublicationID, pk)
			ident := digest.Tagged(tag.IdentityID, pk)
			if hex.EncodeToString(ident[:]) != v.IdentityID {
				t.Errorf("%s: identity id mismatch", v.ID)
			}
		} else {
			copy(d[:], vectors.Hex(t, v.PubDigest))
		}
		if hex.EncodeToString(d[:]) != v.PubDigest {
			t.Errorf("%s: publication digest mismatch", v.ID)
		}
		id := FromDigest(d)
		if id.String() != v.PubID {
			t.Errorf("%s: got %s want %s", v.ID, id, v.PubID)
		}
		if !Matches(id, d) {
			t.Errorf("%s: Matches rejected its own digest", v.ID)
		}
		d[13] ^= 0x04
		if Matches(id, d) {
			t.Errorf("%s: Matches accepted a digest differing inside the first 110 bits", v.ID)
		}
	}
	for _, v := range f.Decode {
		id, err := Parse(v.Input)
		switch {
		case v.Valid && err != nil:
			t.Errorf("%s: rejected valid input %q", v.ID, v.Input)
		case v.Valid && id.String() != v.Expected:
			t.Errorf("%s: decoded to %s want %s", v.ID, id, v.Expected)
		case !v.Valid && err == nil:
			t.Errorf("%s: accepted invalid input %q", v.ID, v.Input)
		}
	}
}
