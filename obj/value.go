package obj

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/frame"
)

var ErrDecode = errors.New("obj: malformed object")

func decodeErr(what string) error {
	return fmt.Errorf("%w: %s", ErrDecode, what)
}

func u64(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }

func ms(v int64) []byte { return binary.BigEndian.AppendUint64(nil, uint64(v)) }

func list(items [][]byte) []byte { return frame.Encode(items...) }

func digestList(ds []digest.Digest) []byte {
	items := make([][]byte, len(ds))
	for i := range ds {
		items[i] = ds[i][:]
	}
	return list(items)
}

func textList(ts []string) []byte {
	items := make([][]byte, len(ts))
	for i, t := range ts {
		items[i] = []byte(t)
	}
	return list(items)
}

func readU64(b []byte, what string) (uint64, error) {
	if len(b) != 8 {
		return 0, decodeErr(what)
	}
	return binary.BigEndian.Uint64(b), nil
}

func readMs(b []byte, what string) (int64, error) {
	v, err := readU64(b, what)
	return int64(v), err
}

func readDigest(b []byte, what string) (digest.Digest, error) {
	var d digest.Digest
	if len(b) != digest.Size {
		return d, decodeErr(what)
	}
	copy(d[:], b)
	return d, nil
}

func readText(b []byte, what string) (string, error) {
	if !utf8.Valid(b) {
		return "", decodeErr(what)
	}
	return string(b), nil
}

func readLine(b []byte, what string) (string, error) {
	s, err := readText(b, what)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(s, "\r\n") {
		return "", decodeErr(what)
	}
	return s, nil
}

func readList(b []byte, what string) ([][]byte, error) {
	var items [][]byte
	for len(b) > 0 {
		if len(b) < 4 {
			return nil, decodeErr(what)
		}
		l := uint64(binary.BigEndian.Uint32(b))
		b = b[4:]
		if uint64(len(b)) < l {
			return nil, decodeErr(what)
		}
		items = append(items, b[:l:l])
		b = b[l:]
	}
	return items, nil
}

func readDigestList(b []byte, what string) ([]digest.Digest, error) {
	items, err := readList(b, what)
	if err != nil {
		return nil, err
	}
	out := make([]digest.Digest, len(items))
	for i, it := range items {
		if out[i], err = readDigest(it, what); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readTextList(b []byte, what string) ([]string, error) {
	items, err := readList(b, what)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, it := range items {
		if out[i], err = readLine(it, what); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func open(b []byte, wantTag string, count int) ([][]byte, error) {
	f, err := frame.Decode(b, count)
	if err != nil {
		return nil, decodeErr(wantTag + " framing")
	}
	if string(f[0]) != wantTag {
		return nil, decodeErr("tag is not " + wantTag)
	}
	return f, nil
}
