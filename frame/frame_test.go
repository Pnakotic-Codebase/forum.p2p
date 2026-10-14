package frame

import (
	"bytes"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

func TestVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			ID      string   `json:"id"`
			Fields  []string `json:"fields_hex"`
			Encoded string   `json:"encoded_hex"`
			Count   int      `json:"expected_field_count"`
			Valid   bool     `json:"expect_valid"`
		} `json:"vectors"`
	}
	vectors.Load(t, "encoding-framing.json", &f)
	if len(f.Vectors) != 13 {
		t.Fatalf("expected 13 vectors, got %d", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		enc := vectors.Hex(t, v.Encoded)
		got, err := Decode(enc, v.Count)
		if !v.Valid {
			if err == nil {
				t.Errorf("%s: malformed input accepted", v.ID)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", v.ID, err)
			continue
		}
		var want [][]byte
		for _, h := range v.Fields {
			want = append(want, vectors.Hex(t, h))
		}
		if !bytes.Equal(Encode(want...), enc) {
			t.Errorf("%s: encode mismatch", v.ID)
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Errorf("%s: field %d mismatch", v.ID, i)
			}
		}
	}
}
