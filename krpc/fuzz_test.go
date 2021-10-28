//go:build go1.18

package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"
	qt "github.com/frankban/quicktest"
)

func Fuzz(f *testing.F) {
	//for _, ret := range random_encode_tests {
	//	f.Add([]byte(ret.expected))
	//}
	f.Fuzz(func(t *testing.T, b []byte) {
		c := qt.New(t)
		var m Msg
		err := bencode.Unmarshal(b, &m)
		if err != nil || m.T == "" || m.Q == "" {
			t.Skip()
		}
		b0, err := bencode.Marshal(m)
		c.Assert(err, qt.IsNil)
		c.Assert(b0, qt.DeepEquals, b)
	})
}
