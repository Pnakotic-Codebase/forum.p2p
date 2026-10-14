package keys

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var letterMap = map[rune]string{
	'ø': "o", 'ł': "l", 'đ': "d", 'ħ': "h", 'ı': "i", 'ŀ': "l", 'ŧ': "t",
	'æ': "ae", 'œ': "oe", 'ß': "ss", 'þ': "th", 'ð': "d", 'ŋ': "n", 'ĸ': "k",
}

var folder = cases.Fold()

func NormalizeSalt(s string) string {
	s = norm.NFKD.String(folder.String(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		if m, ok := letterMap[r]; ok {
			b.WriteString(m)
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}
