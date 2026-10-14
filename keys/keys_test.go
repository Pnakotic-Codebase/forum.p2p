package keys

import (
	"encoding/hex"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

type vec struct {
	ID     string `json:"id"`
	Master string `json:"master_seed_hex"`
	Kind   string `json:"salt_kind"`
	Value  string `json:"salt_value_hex"`
	Seed   string `json:"mldsa65_seed_hex"`
	Input  string `json:"salt_input"`
}

func salt(t *testing.T, v vec) Salt {
	b := vectors.Hex(t, v.Value)
	if v.Kind == "counter" {
		var n uint64
		for _, c := range b {
			n = n<<8 | uint64(c)
		}
		return Counter(n)
	}
	s, err := Custom(v.Input)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.value) != string(b) {
		t.Fatalf("%s: normalized salt %q, want %q", v.ID, s.value, b)
	}
	return s
}

func TestDerivationVectors(t *testing.T) {
	var f struct {
		Vectors []vec `json:"vectors"`
		Norm    []struct {
			ID    string `json:"id"`
			Input string `json:"input"`
			Want  string `json:"normalized"`
			Error string `json:"expect_error"`
		} `json:"normalization_vectors"`
	}
	vectors.Load(t, "derivation.json", &f)
	if len(f.Norm) == 0 {
		t.Fatal("no normalization vectors")
	}
	for _, v := range f.Norm {
		if got := NormalizeSalt(v.Input); got != v.Want {
			t.Errorf("%s: %q normalized to %q, want %q", v.ID, v.Input, got, v.Want)
		}
		_, err := Custom(v.Input)
		if (v.Error != "") != (err != nil) {
			t.Errorf("%s: error mismatch: %v", v.ID, err)
		}
	}
	if len(f.Vectors) != 14 {
		t.Fatalf("expected 14 vectors, got %d", len(f.Vectors))
	}
	seen := map[addr.ID]string{}
	for _, v := range f.Vectors {
		var m [MasterSize]byte
		copy(m[:], vectors.Hex(t, v.Master))
		s := salt(t, v)
		seed := Seed(&m, s)
		if hex.EncodeToString(seed[:]) != v.Seed {
			t.Errorf("%s: derived seed mismatch", v.ID)
			continue
		}
		sk, err := Derive(&m, s)
		if err != nil {
			t.Fatalf("%s: %v", v.ID, err)
		}
		pk := sk.PublicKey()
		id := Address(pk)
		if !addr.Matches(id, PublicationDigest(pk)) {
			t.Errorf("%s: address does not match its own digest", v.ID)
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("%s: address collides with %s", v.ID, prev)
		}
		seen[id] = v.ID
	}
}

func TestCustomSaltRules(t *testing.T) {
	if _, err := Custom(""); err == nil {
		t.Error("empty custom salt accepted")
	}
	if _, err := Custom("\xff\xfe"); err == nil {
		t.Error("invalid UTF-8 custom salt accepted")
	}
}
