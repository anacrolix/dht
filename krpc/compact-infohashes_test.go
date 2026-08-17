package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"
	qt "github.com/frankban/quicktest"
)

func TestUnmarshalCompactInfohashes(t *testing.T) {
	c := qt.New(t)
	var cihs CompactInfohashes
	c.Check(bencode.Unmarshal([]byte("40:HELLOHELLOHELLOHELLOworldworldworldworld"), &cihs), qt.IsNil)
	var expected [2][20]byte
	copy(expected[0][:], "HELLOHELLOHELLOHELLO")
	copy(expected[1][:], "worldworldworldworld")
	c.Assert(cihs, qt.DeepEquals, CompactInfohashes(expected[:]))
}

func TestMarshalCompactInfohashes(t *testing.T) {
	c := qt.New(t)
	var cihs CompactInfohashes
	c.Assert(bencode.Unmarshal([]byte("40:HELLOHELLOHELLOHELLOworldworldworldworld"), &cihs), qt.IsNil)
	b, err := cihs.MarshalBinary()
	c.Assert(err, qt.IsNil)
	c.Check(string(b), qt.Equals, "HELLOHELLOHELLOHELLOworldworldworldworld")
	bb, err := cihs.MarshalBencode()
	c.Assert(err, qt.IsNil)
	c.Check(string(bb), qt.Equals, "40:HELLOHELLOHELLOHELLOworldworldworldworld")
}
