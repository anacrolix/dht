package dht

import (
	"net"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	peer_store "github.com/anacrolix/dht/v2/peer-store"
	"github.com/anacrolix/torrent/bencode"
	"golang.org/x/time/rate"
)

type announcedPortStore struct{ added chan krpc.NodeAddr }

func (s announcedPortStore) AddPeer(_ peer_store.InfoHash, addr krpc.NodeAddr) { s.added <- addr }
func (s announcedPortStore) GetPeers(peer_store.InfoHash) []krpc.NodeAddr      { return nil }

func TestAnnouncePeerPortValidation(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		port                    int
		present, implied, valid bool
	}{
		{name: "missing"},
		{name: "zero", present: true},
		{name: "too large", port: 65536, present: true},
		{name: "maximum", port: 65535, present: true, valid: true},
		{name: "implied overrides explicit", port: 65536, present: true, implied: true, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := mustListen("127.0.0.1:0")
			t.Cleanup(func() { _ = peer.Close() })
			store := announcedPortStore{added: make(chan krpc.NodeAddr, 1)}
			s, err := NewServer(&ServerConfig{Conn: mustListen("127.0.0.1:0"), NoSecurity: true, PeerStore: store, SendLimiter: rate.NewLimiter(rate.Inf, 0)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			source := NewAddr(peer.LocalAddr())
			s.mu.Lock()
			token := s.createToken(source)
			s.mu.Unlock()
			args := &krpc.MsgArgs{ID: krpc.ID{19: 1}, InfoHash: krpc.ID{1}, Token: token, ImpliedPort: tc.implied}
			if tc.present {
				args.Port = &tc.port
			}
			packet, err := bencode.Marshal(krpc.Msg{Y: krpc.YQuery, Q: "announce_peer", T: "test", A: args})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = peer.WriteTo(packet, s.Addr()); err != nil {
				t.Fatal(err)
			}
			if err = peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			var buf [1500]byte
			n, _, err := peer.ReadFrom(buf[:])
			if err != nil {
				t.Fatal(err)
			}
			var reply krpc.Msg
			if err = bencode.Unmarshal(buf[:n], &reply); err != nil {
				t.Fatal(err)
			}
			if !tc.valid {
				if reply.E == nil || reply.E.Code != krpc.ErrorCodeProtocolError {
					t.Fatalf("invalid port got response %+v, want protocol error", reply)
				}
				select {
				case addr := <-store.added:
					t.Fatalf("stored invalid peer %v", addr)
				default:
				}
				return
			}
			if reply.R == nil || reply.E != nil {
				t.Fatalf("valid announce failed: %+v", reply)
			}
			want := tc.port
			if tc.implied {
				want = peer.LocalAddr().(*net.UDPAddr).Port
			}
			select {
			case addr := <-store.added:
				if addr.Port != want {
					t.Fatalf("stored port %d, want %d", addr.Port, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("valid announcement was not stored")
			}
		})
	}
}
