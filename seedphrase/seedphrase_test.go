package seedphrase

import (
	"encoding/hex"
	"slices"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/internal/vectors"
)

func TestPhraseVectors(t *testing.T) {
	var f struct {
		Phrases []struct {
			ID    string   `json:"id"`
			Seed  string   `json:"seed_hex"`
			Words []string `json:"words"`
			Valid bool     `json:"expect_valid"`
			Error string   `json:"expected_error"`
			Canon []string `json:"canonical_words"`
		} `json:"phrase_vectors"`
		Texts []struct {
			ID    string `json:"id"`
			Text  string `json:"text"`
			Seed  string `json:"seed_hex"`
			Valid bool   `json:"expect_valid"`
			Error string `json:"expected_error"`
		} `json:"phrase_text_vectors"`
	}
	vectors.Load(t, "identifiers.json", &f)
	if len(f.Texts) == 0 {
		t.Fatal("no text vectors")
	}
	for _, v := range f.Texts {
		seed, err := RestoreText(v.Text)
		if v.Valid {
			if err != nil || hex.EncodeToString(seed[:]) != v.Seed {
				t.Errorf("%s: text restore failed: %v", v.ID, err)
			}
			continue
		}
		if err == nil || err.Error() != v.Error || seed != ([SeedSize]byte{}) {
			t.Errorf("%s: want %q and zero seed, got %v", v.ID, v.Error, err)
		}
	}
	if len(f.Phrases) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range f.Phrases {
		seed, err := Restore(v.Words)
		if !v.Valid {
			if err == nil || err.Error() != v.Error {
				t.Errorf("%s: want %q, got %v", v.ID, v.Error, err)
			}
			if seed != ([SeedSize]byte{}) {
				t.Errorf("%s: failed restore returned non-zero seed", v.ID)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", v.ID, err)
			continue
		}
		if hex.EncodeToString(seed[:]) != v.Seed {
			t.Errorf("%s: restored seed mismatch", v.ID)
		}
		want := v.Words
		if v.Canon != nil {
			want = v.Canon
		}
		if !slices.Equal(Encode(seed), want) {
			t.Errorf("%s: encode mismatch", v.ID)
		}
	}
}

func TestRoundTripEveryIndex(t *testing.T) {
	for i := 0; i < ListSize; i++ {
		var seed [SeedSize]byte
		seed[0], seed[1] = byte(i>>4), byte(i<<4)
		seed[31] = byte(i)
		got, err := Restore(Encode(seed))
		if err != nil || got != seed {
			t.Fatalf("index %d: round trip failed", i)
		}
	}
}
