package krpc

import (
	"testing"

	"github.com/anacrolix/torrent/bencode"

	"github.com/go-quicktest/qt"
)

// https://github.com/anacrolix/torrent/issues/166
func TestUnmarshalBadError(t *testing.T) {
	var e Error
	err := bencode.Unmarshal([]byte(`l5:helloe`), &e)
	qt.Assert(t, qt.IsNotNil(err))
}
