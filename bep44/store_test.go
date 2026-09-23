package bep44

import (
	"encoding/hex"
	"errors"
	"sync"
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

type reentrantMemoryValue struct {
	memory *Memory
	getErr chan error
	once   sync.Once
}

func (v *reentrantMemoryValue) MarshalBencode() ([]byte, error) {
	v.once.Do(func() {
		_, err := v.memory.Get(Target{})
		v.getErr <- err
	})
	return []byte("1:x"), nil
}

func TestMemoryPutAllowsMarshalerToCallGet(t *testing.T) {
	memory := NewMemory()
	value := &reentrantMemoryValue{
		memory: memory,
		getErr: make(chan error, 1),
	}
	item := &Item{V: value}
	putErr := make(chan error, 1)
	go func() {
		putErr <- memory.Put(item)
	}()

	select {
	case err := <-putErr:
		if err != nil {
			t.Fatalf("Memory.Put returned an error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Memory.Put did not complete when MarshalBencode called Memory.Get")
	}

	select {
	case err := <-value.getErr:
		if !errors.Is(err, ErrItemNotFound) {
			t.Fatalf("Memory.Get from MarshalBencode error = %v, want %v", err, ErrItemNotFound)
		}
	case <-time.After(time.Second):
		t.Fatal("MarshalBencode did not complete its Memory.Get call")
	}

	stored, err := memory.Get(item.Target())
	if err != nil {
		t.Fatal(err)
	}
	if stored != item {
		t.Fatalf("Memory.Get returned %p, want stored item %p", stored, item)
	}
}
