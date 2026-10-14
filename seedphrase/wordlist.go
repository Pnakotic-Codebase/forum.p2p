package seedphrase

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"strconv"
	"strings"
)

const (
	ListSize       = 4096
	WordlistSHA256 = "e15a4c733d461f1730fb879e2b88bb4de18a4e4b845f0e1dc322b34394dd5229"
	slot           = 32
)

//go:embed forum-wordlist-v0.tsv
var wordlistTSV []byte

var (
	words [ListSize]string
	table [ListSize][slot]byte
)

func init() {
	sum := sha256.Sum256(wordlistTSV)
	if hex.EncodeToString(sum[:]) != WordlistSHA256 {
		panic("seedphrase: embedded word list does not match its recorded sha256")
	}
	sc := bufio.NewScanner(bytes.NewReader(wordlistTSV))
	sc.Scan()
	n := 0
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		i, err := strconv.Atoi(cols[0])
		if err != nil || i != n || n >= ListSize {
			panic("seedphrase: word list index out of order")
		}
		if n > 0 && cols[1] <= words[n-1] {
			panic("seedphrase: word list not in strict a-z order")
		}
		words[n] = cols[1]
		table[n] = pad(cols[1])
		n++
	}
	if n != ListSize {
		panic("seedphrase: word list is not 4096 words")
	}
}

func pad(w string) [slot]byte {
	var b [slot]byte
	l := len(w)
	if l > slot-1 {
		l = slot - 1
		b[0] = 0xff
	} else {
		b[0] = byte(l)
	}
	copy(b[1:], w[:l])
	for i := 1; i < slot; i++ {
		c := int(b[i])
		up := subtle.ConstantTimeLessOrEq('A', c) & subtle.ConstantTimeLessOrEq(c, 'Z')
		b[i] |= byte(up << 5)
	}
	return b
}

func Word(i int) string {
	return words[i]
}
