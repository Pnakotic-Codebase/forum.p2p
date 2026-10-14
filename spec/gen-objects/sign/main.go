package main

import (
	"crypto/mldsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

func fr(fields ...[]byte) []byte {
	var out []byte
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func unhex(s string) []byte {
	b, err := hex.DecodeString(s)
	must(err)
	return b
}

func seal(sk *mldsa.PrivateKey, object []byte) []byte {
	sig, err := sk.SignDeterministic(object, &mldsa.Options{})
	must(err)
	return fr([]byte("forum:v0:signed"), object, sig)
}

type Held struct {
	PublicKey string `json:"public_key_hex"`
	Sequence  uint64 `json:"sequence"`
	HeadBytes string `json:"head_object_hex"`
}

type Case struct {
	ID        string            `json:"id"`
	Desc      string            `json:"description"`
	Address   string            `json:"requested_address"`
	Head      string            `json:"head_envelope_hex"`
	Revisions map[string]string `json:"revision_envelopes"`
	Assets    map[string]string `json:"assets"`
	Held      *Held             `json:"held,omitempty"`
	Valid     bool              `json:"expect_valid"`
	Check     string            `json:"expect_failed_check,omitempty"`
}

func main() {
	var in struct {
		Assets []struct {
			ID  string `json:"asset_id_hex"`
			Enc string `json:"encoded_hex"`
		} `json:"assets"`
		Revisions []struct {
			ID  string `json:"revision_id_hex"`
			Enc string `json:"encoded_hex"`
		} `json:"revisions"`
		Head struct {
			Enc     string `json:"encoded_hex"`
			Seed    string `json:"signing_seed_hex"`
			PK      string `json:"public_key_hex"`
			Address string `json:"publication_address"`
		} `json:"head"`
	}
	raw, err := os.ReadFile(os.Args[1])
	must(err)
	must(json.Unmarshal(raw, &in))
	sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), unhex(in.Head.Seed))
	must(err)
	other, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), make([]byte, 32))
	must(err)

	head := seal(sk, unhex(in.Head.Enc))
	revs := map[string]string{}
	for _, r := range in.Revisions[1:] {
		revs[r.ID] = hex.EncodeToString(seal(sk, unhex(r.Enc)))
	}
	assets := map[string]string{}
	for _, a := range in.Assets[:1] {
		assets[a.ID] = a.Enc
	}
	clone := func(m map[string]string) map[string]string {
		c := map[string]string{}
		for k, v := range m {
			c[k] = v
		}
		return c
	}
	base := func(id, desc string) Case {
		return Case{ID: id, Desc: desc, Address: in.Head.Address, Head: hex.EncodeToString(head),
			Revisions: clone(revs), Assets: clone(assets), Valid: true}
	}
	var cases []Case
	cases = append(cases, base("ver-001", "complete publication verifies"))

	c := base("ver-002", "same publication, held head with lower sequence")
	c.Held = &Held{PublicKey: in.Head.PK, Sequence: 1, HeadBytes: ""}
	cases = append(cases, c)

	c = base("ver-003", "identical head re-fetched")
	c.Held = &Held{PublicKey: in.Head.PK, Sequence: 2, HeadBytes: in.Head.Enc}
	cases = append(cases, c)

	c = base("ver-101", "head signature bit flipped")
	hb := append([]byte(nil), head...)
	hb[len(hb)-100] ^= 0x01
	c.Head, c.Valid, c.Check = hex.EncodeToString(hb), false, "head signature"
	cases = append(cases, c)

	c = base("ver-102", "valid head requested under a different address")
	c.Address, c.Valid, c.Check = "0000000000000000000000", false, "publication address"
	cases = append(cases, c)

	c = base("ver-103", "held key differs (key pinning)")
	c.Held = &Held{PublicKey: hex.EncodeToString(other.PublicKey().Bytes()), Sequence: 1}
	c.Valid, c.Check = false, "key pinning"
	cases = append(cases, c)

	c = base("ver-104", "rollback: held sequence is higher")
	c.Held = &Held{PublicKey: in.Head.PK, Sequence: 3}
	c.Valid, c.Check = false, "head sequence"
	cases = append(cases, c)

	rid := in.Revisions[1].ID
	c = base("ver-105", "revision body altered after signing")
	rb := unhex(revs[rid])
	rb[len(rb)-3400] ^= 0x20
	c.Revisions[rid], c.Valid, c.Check = hex.EncodeToString(rb), false, "revision id"
	cases = append(cases, c)

	c = base("ver-106", "revision re-signed by another key")
	c.Revisions[rid] = hex.EncodeToString(seal(other, unhex(in.Revisions[1].Enc)))
	c.Valid, c.Check = false, "revision signature"
	cases = append(cases, c)

	c = base("ver-107", "asset replaced by another asset's bytes")
	c.Assets[in.Assets[0].ID] = in.Assets[1].Enc
	c.Valid, c.Check = false, "asset id"
	cases = append(cases, c)

	c = base("ver-108", "revision missing from the source")
	delete(c.Revisions, in.Revisions[2].ID)
	c.Valid, c.Check = false, "revision fetch"
	cases = append(cases, c)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(map[string]any{
		"suite":   "forum-spec/verification",
		"signing": "deterministic, so the file reproduces; producers sign hedged",
		"cases":   cases,
	}))
}
