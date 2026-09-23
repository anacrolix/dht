package bep44

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

// The current public API and KRPC Cas fields are int64, and KRPC encodes Cas with omitempty. Thus
// cas=0 cannot be distinguished from omission; this is a representation limitation, not a BEP 44
// rule that a present cas of zero is absent.
func TestCheckIncomingCASComparesStoredSequence(t *testing.T) {
	var key [32]byte
	key[0] = 1
	stored := &Item{K: key, Seq: 1, V: "a"}
	if err := CheckIncoming(stored, &Item{K: key, Seq: 2, Cas: 1, V: "b"}); err != nil {
		t.Fatalf("cas equal to the stored sequence was rejected: %v", err)
	}
	if err := CheckIncoming(stored, &Item{K: key, Seq: 2, Cas: 9, V: "b"}); !errors.Is(err, ErrCasHashMismatched) {
		t.Fatalf("cas 9 against stored seq 1: got %v, want mismatch", err)
	}
	if err := CheckIncoming(stored, &Item{K: key, Seq: 2, V: "b"}); err != nil {
		t.Fatalf("absent cas was rejected: %v", err)
	}
	// The previous writer sent cas 4. The current sequence is 5. A correct replacement
	// names cas 5, not 4.
	stored.Cas = 4
	stored.Seq = 5
	if err := CheckIncoming(stored, &Item{K: key, Seq: 6, Cas: 5, V: "c"}); err != nil {
		t.Fatalf("cas was compared to stored.Cas (%d) instead of stored.Seq: %v", stored.Cas, err)
	}
}

func TestCheckIncomingCASMismatchPrecedesSequenceCheck(t *testing.T) {
	var key [32]byte
	key[0] = 1
	stored := &Item{K: key, Seq: 5, V: "same"}
	for _, incoming := range []*Item{
		{K: key, Seq: 5, Cas: 4, V: "same"},
		{K: key, Seq: 5, Cas: 4, V: "different"},
		{K: key, Seq: 4, Cas: 4, V: "older"},
	} {
		if err := CheckIncoming(stored, incoming); !errors.Is(err, ErrCasHashMismatched) {
			t.Errorf("CheckIncoming(%+v) = %v, want CAS mismatch", incoming, err)
		}
	}
}

func TestWrapperConcurrentCASAllowsOneUpdate(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := NewItem("initial", nil, 1, 0, priv)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewItem("first", nil, 2, 1, priv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewItem("second", nil, 2, 1, priv)
	if err != nil {
		t.Fatal(err)
	}

	memory := NewMemory()
	wrapper := NewWrapper(memory, time.Hour)
	if err := wrapper.Put(initial); err != nil {
		t.Fatal(err)
	}

	type result struct {
		item *Item
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, item := range []*Item{first, second} {
		go func(item *Item) {
			<-start
			results <- result{item: item, err: wrapper.Put(item)}
		}(item)
	}
	close(start)

	var winner *Item
	successes := 0
	for range 2 {
		got := <-results
		if got.err == nil {
			winner = got.item
			successes++
		} else if !errors.Is(got.err, ErrCasHashMismatched) {
			t.Errorf("Wrapper.Put(%q) error = %v, want CAS mismatch", got.item.V, got.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent CAS writes = %d, want exactly one", successes)
	}

	stored, err := memory.Get(initial.Target())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Seq != 2 || stored.V != winner.V {
		t.Fatalf("stored item = (seq %d, value %q), want the accepted write (seq 2, value %q)",
			stored.Seq, stored.V, winner.V)
	}
}
