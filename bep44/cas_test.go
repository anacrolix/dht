package bep44

import (
	"crypto/ed25519"
	"errors"
	"sync"
	"testing"
	"time"
)

// BEP 44: when cas is present it is the sequence number being overwritten, compared to the
// sequence currently stored. It is not compared to the cas stored with the previous write.
// A zero cas is absent on the wire (bencode omitempty).
func TestCheckIncomingCASComparesStoredSequence(t *testing.T) {
	stored := &Item{Seq: 1, V: "a"}
	if err := CheckIncoming(stored, &Item{Seq: 2, Cas: 1, V: "b"}); err != nil {
		t.Fatalf("cas equal to the stored sequence was rejected: %v", err)
	}
	if err := CheckIncoming(stored, &Item{Seq: 2, Cas: 9, V: "b"}); !errors.Is(err, ErrCasHashMismatched) {
		t.Fatalf("cas 9 against stored seq 1: got %v, want mismatch", err)
	}
	if err := CheckIncoming(stored, &Item{Seq: 2, V: "b"}); err != nil {
		t.Fatalf("absent cas was rejected: %v", err)
	}
	// The previous writer sent cas 4. The current sequence is 5. A correct replacement
	// names cas 5, not 4.
	stored.Cas = 4
	stored.Seq = 5
	if err := CheckIncoming(stored, &Item{Seq: 6, Cas: 5, V: "c"}); err != nil {
		t.Fatalf("cas was compared to stored.Cas (%d) instead of stored.Seq: %v", stored.Cas, err)
	}
}

// Wrapper.Put reads the current item and writes the new one as two store calls. Overlapping
// calls can both observe the old sequence and then write out of order.
type overlapStore struct {
	mu         sync.Mutex
	depth      int
	overlapped bool
	item       *Item
}

func (s *overlapStore) begin() {
	s.mu.Lock()
	s.depth++
	if s.depth > 1 {
		s.overlapped = true
	}
	s.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
}

func (s *overlapStore) end() {
	s.mu.Lock()
	s.depth--
	s.mu.Unlock()
}

func (s *overlapStore) Get(Target) (*Item, error) {
	s.begin()
	defer s.end()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.item == nil {
		return nil, ErrItemNotFound
	}
	return s.item, nil
}

func (s *overlapStore) Put(i *Item) error {
	s.begin()
	defer s.end()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.item = i
	return nil
}

func (s *overlapStore) Del(Target) error { return nil }

func TestWrapperPutDoesNotOverlap(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	i1, err := NewItem("one", nil, 1, 0, priv)
	if err != nil {
		t.Fatal(err)
	}
	i2, err := NewItem("two", nil, 2, 0, priv)
	if err != nil {
		t.Fatal(err)
	}
	i3, err := NewItem("three", nil, 3, 0, priv)
	if err != nil {
		t.Fatal(err)
	}
	st := &overlapStore{item: i1}
	w := NewWrapper(st, time.Hour)
	var wg sync.WaitGroup
	wg.Go(func() { _ = w.Put(i2) })
	wg.Go(func() { _ = w.Put(i3) })
	wg.Wait()
	if st.overlapped {
		t.Fatal("two Wrapper.Put calls overlapped inside the store; a lower sequence can overwrite a higher one")
	}
	if st.item.Seq != 3 {
		t.Fatalf("stored seq %d, want 3", st.item.Seq)
	}
}
