package main

import (
	"crypto/mldsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

type Vector struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	SeedHex     string `json:"seed_hex"`
	PublicKey   string `json:"public_key_hex"`
	MessageHex  string `json:"message_hex"`
	Context     string `json:"context"`
	Signature   string `json:"signature_hex"`
	VerifyMsg   string `json:"verify_message_hex"`
	VerifyCtx   string `json:"verify_context"`
	VerifySig   string `json:"verify_signature_hex"`
	Expect      bool   `json:"expect_valid"`
}

type File struct {
	Suite      string   `json:"suite"`
	Parameters string   `json:"parameters"`
	Standard   string   `json:"standard"`
	Signing    string   `json:"signing_mode"`
	Vectors    []Vector `json:"vectors"`
}

func seq(start byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = start + byte(i)
	}
	return b
}

func fill(v byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = v
	}
	return b
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sign(seed, msg []byte, ctx string) (string, []byte) {
	sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
	must(err)
	sig, err := sk.SignDeterministic(msg, &mldsa.Options{Context: ctx})
	must(err)
	return hex.EncodeToString(sk.PublicKey().Bytes()), sig
}

func check(pkHex string, msg []byte, ctx string, sig []byte) bool {
	raw, err := hex.DecodeString(pkHex)
	must(err)
	pk, err := mldsa.NewPublicKey(mldsa.MLDSA65(), raw)
	must(err)
	return mldsa.Verify(pk, msg, sig, &mldsa.Options{Context: ctx}) == nil
}

func main() {
	var out []Vector
	add := func(id, desc string, seed, msg []byte, ctx string, vmsg []byte, vctx string, mutate func([]byte) []byte) {
		pk, sig := sign(seed, msg, ctx)
		vsig := append([]byte(nil), sig...)
		if mutate != nil {
			vsig = mutate(vsig)
		}
		got := check(pk, vmsg, vctx, vsig)
		out = append(out, Vector{
			ID: id, Description: desc,
			SeedHex: hex.EncodeToString(seed), PublicKey: pk,
			MessageHex: hex.EncodeToString(msg), Context: ctx,
			Signature: hex.EncodeToString(sig),
			VerifyMsg: hex.EncodeToString(vmsg), VerifyCtx: vctx,
			VerifySig: hex.EncodeToString(vsig), Expect: got,
		})
	}

	m1 := []byte("forum://")
	m2 := []byte("")
	m3 := []byte("# Hello\n\nFirst post on a publication with no domain.\n")

	add("sig-001", "valid: sequential seed, short message, empty context", seq(0x00), m1, "", m1, "", nil)
	add("sig-002", "valid: empty message", seq(0x00), m2, "", m2, "", nil)
	add("sig-003", "valid: markdown body, second key", fill(0x42), m3, "", m3, "", nil)
	add("sig-004", "valid: non-empty context string", seq(0x20), m1, "forum:test", m1, "forum:test", nil)
	add("sig-101", "invalid: message altered by one byte", seq(0x00), m1, "", []byte("forum:/"), "", nil)
	add("sig-102", "invalid: signature first byte flipped", seq(0x00), m1, "", m1, "", func(s []byte) []byte { s[0] ^= 0x01; return s })
	add("sig-103", "invalid: context mismatch at verify", seq(0x20), m1, "forum:test", m1, "forum:other", nil)
	add("sig-104", "invalid: signature truncated by one byte", seq(0x00), m1, "", m1, "", func(s []byte) []byte { return s[:len(s)-1] })

	f := File{
		Suite:      "forum-spec/signature",
		Parameters: "ML-DSA-65",
		Standard:   "FIPS 204",
		Signing:    "deterministic (rnd = 0^32) so vectors reproduce; verification is mode-independent",
		Vectors:    out,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(f))
}
