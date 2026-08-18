package peer_store

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
)

type comparableNodeAddr struct {
	ip   string
	port int
}

func makeComparableNodeAddr(na krpc.NodeAddr) comparableNodeAddr {
	return comparableNodeAddr{string(na.IP), na.Port}
}

func requireExactNodeAddr(t *testing.T, got, want krpc.NodeAddr) {
	t.Helper()
	if !bytes.Equal(got.IP, want.IP) || got.Port != want.Port {
		t.Fatalf("NodeAddr = {IP: %x, Port: %d}, want {IP: %x, Port: %d}", got.IP, got.Port, want.IP, want.Port)
	}
}

func requireNodeAddrSet(t *testing.T, got, want []krpc.NodeAddr) {
	t.Helper()
	gotCounts := make(map[comparableNodeAddr]int, len(got))
	wantCounts := make(map[comparableNodeAddr]int, len(want))
	for _, na := range got {
		gotCounts[makeComparableNodeAddr(na)]++
	}
	for _, na := range want {
		wantCounts[makeComparableNodeAddr(na)]++
	}
	if len(gotCounts) != len(wantCounts) {
		t.Fatalf("GetPeers returned %v, want unordered set %v", got, want)
	}
	for key, count := range wantCounts {
		if gotCounts[key] != count {
			t.Fatalf("GetPeers returned %v, want unordered set %v", got, want)
		}
	}
}

func TestInMemoryPeerStoreRoundTripsNodeAddr(t *testing.T) {
	tests := []struct {
		name string
		addr krpc.NodeAddr
	}{
		{"four-byte IPv4 with zero port", krpc.NodeAddr{IP: net.IP{192, 0, 2, 1}, Port: 0}},
		{"mapped IPv4 with port one", krpc.NodeAddr{IP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 0, 2, 1}, Port: 1}},
		{"global IPv6", krpc.NodeAddr{IP: net.IP{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Port: 6881}},
		{"link-local IPv6", krpc.NodeAddr{IP: net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Port: 65535}},
		{"unspecified IPv4", krpc.NodeAddr{IP: net.IP{0, 0, 0, 0}, Port: 6881}},
		{"unspecified IPv6", krpc.NodeAddr{IP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Port: 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var store InMemory
			var ih InfoHash
			store.AddPeer(ih, test.addr)

			got := store.GetPeers(ih)
			if len(got) != 1 {
				t.Fatalf("GetPeers returned %d peers, want 1", len(got))
			}
			requireExactNodeAddr(t, got[0], test.addr)
		})
	}
}

func TestInMemoryPeerStoreSameIPDifferentPorts(t *testing.T) {
	var store InMemory
	var ih InfoHash
	ip := net.IP{203, 0, 113, 9}
	store.AddPeer(ih, krpc.NodeAddr{IP: ip, Port: 1})
	want := krpc.NodeAddr{IP: ip, Port: 65535}
	store.AddPeer(ih, want)

	got := store.GetPeers(ih)
	if len(got) != 1 {
		t.Fatalf("GetPeers returned %d peers, want 1", len(got))
	}
	requireExactNodeAddr(t, got[0], want)
}

func TestInMemoryPeerStoreKeepsIPv4RepresentationsDistinct(t *testing.T) {
	var store InMemory
	var ih InfoHash
	fourByte := krpc.NodeAddr{IP: net.IP{192, 0, 2, 1}, Port: 6881}
	mapped := krpc.NodeAddr{IP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 192, 0, 2, 1}, Port: 6881}
	store.AddPeer(ih, fourByte)
	store.AddPeer(ih, mapped)

	requireNodeAddrSet(t, store.GetPeers(ih), []krpc.NodeAddr{fourByte, mapped})
}

func TestInMemoryPeerStoreRefreshesDuplicate(t *testing.T) {
	var ih InfoHash
	ip := net.IP{198, 51, 100, 7}
	oldTime := time.Unix(1, 0)
	store := InMemory{index: map[InfoHash]indexValue{
		ih: {
			string(ip): {NodeAddr: krpc.NodeAddr{IP: ip, Port: 1}, Time: oldTime},
		},
	}}
	want := krpc.NodeAddr{IP: ip, Port: 6881}

	store.AddPeer(ih, want)

	got := store.GetAll()[ih]
	if len(got) != 1 {
		t.Fatalf("GetAll returned %d peers, want 1", len(got))
	}
	requireExactNodeAddr(t, got[0].NodeAddr, want)
	if !got[0].Time.After(oldTime) {
		t.Fatalf("refreshed time = %v, want after %v", got[0].Time, oldTime)
	}
}

func TestInMemoryPeerStoreDifferentIPsSamePort(t *testing.T) {
	var store InMemory
	var ih InfoHash
	first := krpc.NodeAddr{IP: net.IP{192, 0, 2, 1}, Port: 6881}
	second := krpc.NodeAddr{IP: net.IP{192, 0, 2, 2}, Port: 6881}
	store.AddPeer(ih, first)
	store.AddPeer(ih, second)

	requireNodeAddrSet(t, store.GetPeers(ih), []krpc.NodeAddr{first, second})
}

func TestInMemoryPeerStoreUnknownInfoHash(t *testing.T) {
	var store InMemory
	var ih InfoHash
	if got := store.GetPeers(ih); len(got) != 0 {
		t.Fatalf("GetPeers returned %v, want no peers", got)
	}
}

func TestInMemoryPeerStoreOpaqueShortIPs(t *testing.T) {
	var store InMemory
	var peerStore Interface = &store
	var ih InfoHash
	want := []krpc.NodeAddr{
		{IP: nil, Port: 0},
		{IP: net.IP{1}, Port: 65535},
	}
	for _, addr := range want {
		peerStore.AddPeer(ih, addr)
	}

	requireNodeAddrSet(t, peerStore.GetPeers(ih), want)
}

func TestInMemoryPeerStoreDoesNotMutateCallerIP(t *testing.T) {
	var store InMemory
	var ih InfoHash
	ip := net.IP{192, 0, 2, 44}
	before := bytes.Clone(ip)

	store.AddPeer(ih, krpc.NodeAddr{IP: ip, Port: 6881})

	if !bytes.Equal(ip, before) {
		t.Fatalf("caller IP changed from %x to %x", before, ip)
	}
}

func TestInMemoryPeerStoreConcurrentAccess(t *testing.T) {
	var store InMemory
	var ih InfoHash
	endpoints := []krpc.NodeAddr{
		{IP: net.IP{192, 0, 2, 1}, Port: 6001},
		{IP: net.IP{192, 0, 2, 2}, Port: 6002},
		{IP: net.IP{192, 0, 2, 3}, Port: 6003},
		{IP: net.IP{192, 0, 2, 4}, Port: 6004},
		{IP: net.IP{192, 0, 2, 5}, Port: 6005},
		{IP: net.IP{192, 0, 2, 6}, Port: 6006},
		{IP: net.IP{192, 0, 2, 7}, Port: 6007},
		{IP: net.IP{192, 0, 2, 8}, Port: 6008},
	}
	known := make(map[comparableNodeAddr]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		known[makeComparableNodeAddr(endpoint)] = struct{}{}
	}

	start := make(chan struct{})
	writersDone := make(chan struct{})
	readerErrors := make(chan error, 4)
	var writers sync.WaitGroup
	var readers sync.WaitGroup

	for _, endpoint := range endpoints {
		endpoint := endpoint
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			for range 100 {
				store.AddPeer(ih, endpoint)
			}
		}()
	}
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for {
				select {
				case <-writersDone:
					return
				default:
				}
				got := store.GetPeers(ih)
				if len(got) > len(endpoints) {
					readerErrors <- fmt.Errorf("GetPeers returned %d peers, maximum is %d", len(got), len(endpoints))
					return
				}
				for _, peer := range got {
					if _, ok := known[makeComparableNodeAddr(peer)]; !ok {
						readerErrors <- fmt.Errorf("GetPeers returned unexpected peer %v", peer)
						return
					}
				}
			}
		}()
	}

	close(start)
	writers.Wait()
	close(writersDone)
	readers.Wait()
	close(readerErrors)
	for err := range readerErrors {
		t.Error(err)
	}
	requireNodeAddrSet(t, store.GetPeers(ih), endpoints)
}

func FuzzInMemoryPeerStoreRoundTrip(f *testing.F) {
	f.Add([]byte{192, 0, 2, 1}, uint16(0))
	f.Add([]byte{255, 255, 255, 255}, uint16(65535))
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint16(1))
	f.Add([]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, uint16(65535))

	f.Fuzz(func(t *testing.T, rawIP []byte, port uint16) {
		if len(rawIP) != net.IPv4len && len(rawIP) != net.IPv6len {
			t.Skip()
		}
		var store InMemory
		var ih InfoHash
		want := krpc.NodeAddr{IP: net.IP(bytes.Clone(rawIP)), Port: int(port)}

		store.AddPeer(ih, want)

		got := store.GetPeers(ih)
		if len(got) != 1 {
			t.Fatalf("GetPeers returned %d peers, want 1", len(got))
		}
		requireExactNodeAddr(t, got[0], want)
	})
}
