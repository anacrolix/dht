package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"

	"github.com/go-quicktest/qt"
)

func TestMarshalID(t *testing.T) {
	var id ID
	copy(id[:], []byte("012345678901234567890"))
	qt.Check(t, qt.Equals(string(bencode.MustMarshal(id)), "20:01234567890123456789"))
}
