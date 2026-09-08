package bep44

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/go-quicktest/qt"
)

func TestWrapper(t *testing.T) {
	w := NewWrapper(NewMemory(), 10*time.Hour)

	i, err := NewItem([]byte("Hello World!"), nil, 0, 0, nil)
	qt.Assert(t, qt.IsNil(err))

	err = w.Put(i)
	qt.Assert(t, qt.IsNil(err))

	target := i.Target()

	targetStr := hex.EncodeToString(target[:])
	qt.Assert(t, qt.Equals(targetStr, "e5f96f6f38320f0f33959cb4d3d656452117aadb"))

	i2, err := w.Get(target)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(i2, i))
}

func TestWrapperTimeout(t *testing.T) {
	w := NewWrapper(NewMemory(), 0*time.Second)

	i, err := NewItem([]byte("Hello World!"), nil, 0, 0, nil)
	qt.Assert(t, qt.IsNil(err))

	err = w.Put(i)
	qt.Assert(t, qt.IsNil(err))
	_, err = w.Get(i.Target())
	qt.Assert(t, qt.Equals(err, ErrItemNotFound))
}
