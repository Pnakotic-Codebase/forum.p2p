package digest

import (
	"encoding/hex"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

func TestVectors(t *testing.T) {
	var f struct {
		Vectors []struct {
			ID      string `json:"id"`
			Message string `json:"message_hex"`
			Digest  string `json:"digest_hex"`
		} `json:"vectors"`
	}
	vectors.Load(t, "digest-skein512-512.json", &f)
	if len(f.Vectors) != 13 {
		t.Fatalf("expected 13 vectors, got %d", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		d := Sum(vectors.Hex(t, v.Message))
		if hex.EncodeToString(d[:]) != v.Digest {
			t.Errorf("%s: digest mismatch", v.ID)
		}
	}
}
