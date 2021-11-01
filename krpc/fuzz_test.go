//go:build go1.18

package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"
	qt "github.com/frankban/quicktest"
)

func Fuzz(f *testing.F) {
	f.Add([]byte("d1:rd2:id20:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01e1:t1:t1:y1:re"))
	f.Fuzz(func(t *testing.T, b []byte) {
		c := qt.New(t)
		var m Msg
		err := bencode.Unmarshal(b, &m)
		if err != nil || m.T == "" || m.Y == "" {
			t.Skip()
		}
		if m.R != nil {
			if m.R.ID == [20]byte{} {
				c.Skip()
			}
		}
		b0, err := bencode.Marshal(m)
		c.Logf("%q -> %q", b, b0)
		c.Assert(err, qt.IsNil)
		c.Assert(string(b0), qt.Equals, string(b))
	})
}
