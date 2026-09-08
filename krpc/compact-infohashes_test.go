package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/go-quicktest/qt"
)

func TestUnmarshalCompactInfohashes(t *testing.T) {
	var cihs CompactInfohashes
	qt.Check(t, qt.IsNil(bencode.Unmarshal([]byte("40:HELLOHELLOHELLOHELLOworldworldworldworld"), &cihs)))
	var expected [2][20]byte
	copy(expected[0][:], "HELLOHELLOHELLOHELLO")
	copy(expected[1][:], "worldworldworldworld")
	qt.Assert(t, qt.DeepEquals(cihs, CompactInfohashes(expected[:])))
}

func TestMarshalCompactInfohashes(t *testing.T) {
	var cihs CompactInfohashes
	qt.Assert(t, qt.IsNil(bencode.Unmarshal([]byte("40:HELLOHELLOHELLOHELLOworldworldworldworld"), &cihs)))
	b, err := cihs.MarshalBinary()
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(b), "HELLOHELLOHELLOHELLOworldworldworldworld"))
	bb, err := cihs.MarshalBencode()
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(string(bb), "40:HELLOHELLOHELLOHELLOworldworldworldworld"))
}
