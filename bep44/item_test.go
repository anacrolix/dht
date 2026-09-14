package bep44

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckIncomingCas(t *testing.T) {
	for _, c := range []struct {
		name        string
		storedSeq   int64
		storedCas   int64
		incomingSeq int64
		incomingCas int64
		err         error
	}{
		{name: "absent cas", storedSeq: 1, incomingSeq: 2},
		{name: "cas equal to stored seq", storedSeq: 1, incomingSeq: 2, incomingCas: 1},
		{name: "stored cas is not consulted", storedSeq: 1, storedCas: 7, incomingSeq: 2, incomingCas: 1},
		{
			name: "cas below stored seq", storedSeq: 2, incomingSeq: 3, incomingCas: 1,
			err: ErrCasHashMismatched,
		},
		{
			name: "cas above stored seq", storedSeq: 1, incomingSeq: 2, incomingCas: 2,
			err: ErrCasHashMismatched,
		},
		{
			name: "seq not greater than stored", storedSeq: 2, incomingSeq: 2, incomingCas: 2,
			err: ErrSequenceNumberLessThanCurrent,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			require := require.New(t)
			stored := &Item{V: "stored", Seq: c.storedSeq, Cas: c.storedCas}
			incoming := &Item{V: "incoming", Seq: c.incomingSeq, Cas: c.incomingCas}
			err := CheckIncoming(stored, incoming)
			if c.err == nil {
				require.NoError(err)
			} else {
				require.ErrorIs(err, c.err)
			}
		})
	}
}

func TestWrapperCas(t *testing.T) {
	require := require.New(t)
	w := NewWrapper(NewMemory(), 10*time.Hour)

	_, k, err := ed25519.GenerateKey(nil)
	require.NoError(err)

	first, err := NewItem("first", nil, 1, 0, k)
	require.NoError(err)
	require.NoError(w.Put(first))

	second, err := NewItem("second", nil, 2, 1, k)
	require.NoError(err)
	require.NoError(w.Put(second))

	// A writer that still believes seq 1 is current must not overwrite seq 2.
	stale, err := NewItem("stale", nil, 3, 1, k)
	require.NoError(err)
	require.ErrorIs(w.Put(stale), ErrCasHashMismatched)

	kept, err := w.Get(second.Target())
	require.NoError(err)
	require.Equal("second", kept.V)
}
