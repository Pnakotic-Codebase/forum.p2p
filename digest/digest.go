package digest

import (
	"github.com/deatil/go-hash/skein"

	"github.com/Pnakotic-Codebase/forum.p2p/frame"
)

const Size = 64

type Digest [Size]byte

func Sum(b []byte) Digest {
	return skein.Sum512(b, nil)
}

func Tagged(t string, fields ...[]byte) Digest {
	all := make([][]byte, 0, len(fields)+1)
	all = append(all, []byte(t))
	all = append(all, fields...)
	return Sum(frame.Encode(all...))
}
