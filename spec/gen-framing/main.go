package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Vector struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	FieldsHex   []string `json:"fields_hex,omitempty"`
	EncodedHex  string   `json:"encoded_hex"`
	FieldCount  int      `json:"expected_field_count"`
	Expect      bool     `json:"expect_valid"`
}

type File struct {
	Suite   string   `json:"suite"`
	Rule    string   `json:"rule"`
	Vectors []Vector `json:"vectors"`
}

func encode(fields [][]byte) []byte {
	var out []byte
	for _, f := range fields {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(f)))
		out = append(out, l[:]...)
		out = append(out, f...)
	}
	return out
}

func decode(b []byte, n int) ([][]byte, error) {
	var fields [][]byte
	for i := 0; i < n; i++ {
		if len(b) < 4 {
			return nil, errors.New("ends inside length prefix")
		}
		l := binary.BigEndian.Uint32(b[:4])
		b = b[4:]
		if uint64(len(b)) < uint64(l) {
			return nil, errors.New("ends inside field body")
		}
		fields = append(fields, b[:l])
		b = b[l:]
	}
	if len(b) != 0 {
		return nil, errors.New("trailing bytes")
	}
	return fields, nil
}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func hexes(fs [][]byte) []string {
	var s []string
	for _, f := range fs {
		s = append(s, hex.EncodeToString(f))
	}
	return s
}

func main() {
	var vs []Vector
	valid := func(id, desc string, fs ...[]byte) {
		enc := encode(fs)
		got, err := decode(enc, len(fs))
		if err != nil || len(got) != len(fs) {
			fmt.Fprintln(os.Stderr, "self-check failed", id, err)
			os.Exit(1)
		}
		vs = append(vs, Vector{ID: id, Description: desc, FieldsHex: hexes(fs), EncodedHex: hex.EncodeToString(enc), FieldCount: len(fs), Expect: true})
	}
	invalid := func(id, desc string, enc []byte, n int) {
		if _, err := decode(enc, n); err == nil {
			fmt.Fprintln(os.Stderr, "self-check accepted invalid", id)
			os.Exit(1)
		}
		vs = append(vs, Vector{ID: id, Description: desc, EncodedHex: hex.EncodeToString(enc), FieldCount: n, Expect: false})
	}

	valid("enc-001", "single short field", []byte("forum"))
	valid("enc-002", "two empty fields", []byte{}, []byte{})
	valid("enc-003", "mixed lengths including an empty last field", []byte("a"), []byte("bc"), []byte{})
	valid("enc-004", "field body that itself looks like a length prefix", []byte{0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'})
	valid("enc-005", "300-byte field, length uses the second byte", pattern(300))
	valid("enc-006", "70000-byte field, length uses the third byte", pattern(70000))
	valid("enc-007", "UTF-8 text is framed as raw bytes", []byte("Grøstl → Skein"), []byte("# Hello\n"))

	invalid("enc-101", "input ends inside a length prefix", []byte{0, 0, 0}, 1)
	invalid("enc-102", "length says 5, only 3 body bytes follow", []byte{0, 0, 0, 5, 'a', 'b', 'c'}, 1)
	invalid("enc-103", "valid field followed by trailing bytes", append(encode([][]byte{[]byte("ok")}), 0x00), 1)
	invalid("enc-104", "two well-formed fields where the object defines one", encode([][]byte{[]byte("a"), []byte("b")}), 1)
	invalid("enc-105", "one field where the object defines two", encode([][]byte{[]byte("a")}), 2)
	invalid("enc-106", "length prefix of 0xFFFFFFFF with no body", []byte{0xff, 0xff, 0xff, 0xff}, 1)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(File{Suite: "forum-spec/encoding-framing", Rule: "field = u32 big-endian length || bytes; object = fields in fixed order; no trailing bytes", Vectors: vs}); err != nil {
		os.Exit(1)
	}
}
