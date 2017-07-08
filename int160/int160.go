package int160

import (
	"github.com/anacrolix/torrent/bencode"
)

type T struct {
	b [20]byte
}

var (
	_ bencode.Marshaler   = (*T)(nil)
	_ bencode.Unmarshaler = (*T)(nil)
)

func FromString(s string) (ret T) {
	if len(s) != 20 {
		panic(len(s))
	}
	n := copy(ret.b[:], s)
	if n != 20 {
		panic(n)
	}
	return
}

func (me *T) MarshalBencode() ([]byte, error) {
	return append([]byte("20:"), me.b[:]...), nil
}

func (me *T) UnmarshalBencode(b []byte) error {

}
