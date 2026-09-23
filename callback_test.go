package dht

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/iplist"
	"golang.org/x/time/rate"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	peer_store "github.com/anacrolix/dht/v2/peer-store"
)

type callbackTestStore struct {
	gets  int
	puts  int
	item  *bep44.Item
	onGet func()
	onPut func(*bep44.Item)
}

func (s *callbackTestStore) Put(item *bep44.Item) error {
	s.puts++
	if s.onPut != nil {
		s.onPut(item)
	}
	s.item = item
	return nil
}
func (s *callbackTestStore) Get(bep44.Target) (*bep44.Item, error) {
	s.gets++
	if s.onGet != nil {
		s.onGet()
	}
	if s.item != nil {
		return s.item, nil
	}
	return nil, bep44.ErrItemNotFound
}
func (*callbackTestStore) Del(bep44.Target) error { return nil }

func newCallbackTestServer(t *testing.T, cfg *ServerConfig) *Server {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cfg.Conn = conn
	cfg.SendLimiter = rate.NewLimiter(rate.Inf, 0)
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func processCallbackTestPacket(t *testing.T, s *Server, query krpc.Msg) {
	t.Helper()
	packet := bencode.MustMarshal(query)
	processed := make(chan struct{})
	go func() {
		s.processPacket(packet, NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}))
		close(processed)
	}()
	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("packet processing deadlocked")
	}
}
func TestOnQueryCanReenterStatsAndClose(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	store := new(callbackTestStore)
	cfg := NewDefaultServerConfig()
	cfg.Conn = conn
	cfg.Store = store
	var s *Server
	var callbackCalls int
	callbackDone := make(chan struct{})
	cfg.OnQuery = func(query *krpc.Msg, _ net.Addr) bool {
		callbackCalls++
		query.Q = "get"
		_ = s.Stats()
		s.Close()
		close(callbackDone)
		return true
	}
	s, err = NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	packet := bencode.MustMarshal(krpc.Msg{
		T: "1",
		Y: "q",
		Q: "ping",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	processed := make(chan struct{})
	go func() {
		s.processPacket(packet, NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}))
		close(processed)
	}()

	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("packet processing deadlocked when OnQuery reentered Server.Stats and Server.Close")
	}
	select {
	case <-callbackDone:
	default:
		t.Fatal("packet processing returned before the OnQuery callback completed")
	}
	if callbackCalls != 1 {
		t.Fatalf("OnQuery called %d times, want 1", callbackCalls)
	}
	if got := store.gets; got != 0 {
		t.Fatalf("default handler accessed the store after callback closed the server: %d calls", got)
	}

	// A packet presented after Close must be discarded before the callback or default handlers run.
	s.processPacket(packet, NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}))
	if callbackCalls != 1 {
		t.Errorf("OnQuery called after server close: %d calls, want 1", callbackCalls)
	}
}

var _ bep44.Store = (*callbackTestStore)(nil)

func TestOnQueryMutationAndVeto(t *testing.T) {
	tests := []struct {
		name      string
		propagate bool
		wantGets  int
	}{
		{name: "mutation propagates", propagate: true, wantGets: 1},
		{name: "veto skips default handler", propagate: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := new(callbackTestStore)
			cfg := NewDefaultServerConfig()
			cfg.Store = store
			cfg.OnQuery = func(query *krpc.Msg, _ net.Addr) bool {
				query.Q = "get"
				return test.propagate
			}
			s := newCallbackTestServer(t, cfg)
			processCallbackTestPacket(t, s, krpc.Msg{
				T: "1",
				Y: "q",
				Q: "ping",
				A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
			})
			if got := store.gets; got != test.wantGets {
				t.Errorf("store Get calls = %d, want %d", got, test.wantGets)
			}
			s.Close()
		})
	}
}

type callbackTestLogHandler struct {
	onLog func()
}

func (h callbackTestLogHandler) Handle(log.Record) { h.onLog() }

func TestPacketLoggerCanReenterStats(t *testing.T) {
	cfg := NewDefaultServerConfig()
	cfg.Passive = true
	cfg.Logger = log.NewLogger()
	var s *Server
	cfg.Logger.SetHandlers(callbackTestLogHandler{onLog: func() { _ = s.Stats() }})
	s = newCallbackTestServer(t, cfg)
	processCallbackTestPacket(t, s, krpc.Msg{
		T: "1",
		Y: "q",
		Q: "ping",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	s.Close()
}

type callbackTestWriter struct {
	server *Server
	writes int
}

func (w *callbackTestWriter) Write(p []byte) (int, error) {
	_ = w.server.Stats()
	w.writes++
	return len(p), nil
}

func TestWriteStatusWriterCanReenterStats(t *testing.T) {
	s := newCallbackTestServer(t, NewDefaultServerConfig())
	writer := &callbackTestWriter{server: s}
	done := make(chan struct{})
	go func() {
		s.WriteStatus(writer)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WriteStatus deadlocked when its writer reentered Server.Stats")
	}
	if writer.writes == 0 {
		t.Error("WriteStatus did not write output")
	}
	s.Close()
}

type callbackTestPeerStore struct {
	get func(peer_store.InfoHash) []krpc.NodeAddr
}

func (*callbackTestPeerStore) AddPeer(peer_store.InfoHash, krpc.NodeAddr) {}
func (s *callbackTestPeerStore) GetPeers(ih peer_store.InfoHash) []krpc.NodeAddr {
	return s.get(ih)
}

func TestPeerStoreCanReenterStats(t *testing.T) {
	var s *Server
	cfg := NewDefaultServerConfig()
	cfg.PeerStore = &callbackTestPeerStore{get: func(peer_store.InfoHash) []krpc.NodeAddr {
		_ = s.Stats()
		return nil
	}}
	s = newCallbackTestServer(t, cfg)
	processCallbackTestPacket(t, s, krpc.Msg{
		T: "1",
		Y: "q",
		Q: "get_peers",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	s.Close()
}

func TestStoreGetCanReenterStats(t *testing.T) {
	var s *Server
	store := &callbackTestStore{onGet: func() { _ = s.Stats() }}
	cfg := NewDefaultServerConfig()
	cfg.Store = store
	s = newCallbackTestServer(t, cfg)
	processCallbackTestPacket(t, s, krpc.Msg{
		T: "1",
		Y: "q",
		Q: "get",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	if store.gets != 1 {
		t.Errorf("store Get calls = %d, want 1", store.gets)
	}
	s.Close()
}

func TestStorePutCanReenterStats(t *testing.T) {
	var s *Server
	store := &callbackTestStore{onPut: func(*bep44.Item) { _ = s.Stats() }}
	cfg := NewDefaultServerConfig()
	cfg.Store = store
	s = newCallbackTestServer(t, cfg)
	item, err := bep44.NewItem("callback store item", nil, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := NewAddr(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})
	processCallbackTestPacket(t, s, krpc.Msg{
		T: "1",
		Y: "q",
		Q: "put",
		A: &krpc.MsgArgs{
			ID:    krpc.ID{19: 1},
			Token: s.createToken(source),
			V:     item.V,
		},
	})
	if store.puts != 1 {
		t.Errorf("store Put calls = %d, want 1", store.puts)
	}
	s.Close()
}

type callbackTestRanger struct {
	lookup  func(net.IP)
	blocked bool
	calls   int
}

func (r *callbackTestRanger) Lookup(ip net.IP) (iplist.Range, bool) {
	r.calls++
	if r.lookup != nil {
		r.lookup(ip)
	}
	return iplist.Range{}, r.blocked
}

func (*callbackTestRanger) NumRanges() int { return 1 }

func TestBlocklistLookupOnWriteCanReenterStats(t *testing.T) {
	ranger := &callbackTestRanger{blocked: true}
	cfg := NewDefaultServerConfig()
	cfg.IPBlocklist = ranger
	var s *Server
	ranger.lookup = func(net.IP) { _ = s.Stats() }
	s = newCallbackTestServer(t, cfg)
	done := make(chan error, 1)
	go func() {
		_, err := s.writeToNode(context.Background(), nil, NewAddr(&net.UDPAddr{
			IP: net.IPv4(127, 0, 0, 1), Port: 1,
		}), false, false)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("writeToNode succeeded for an address blocked by the Ranger")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocklist Lookup deadlocked when it reentered Server.Stats during write")
	}
	if ranger.calls != 1 {
		t.Errorf("Ranger Lookup calls = %d, want 1", ranger.calls)
	}
	s.Close()
}

func TestBlocklistLookupOnReadCanReenterStats(t *testing.T) {
	ranger := &callbackTestRanger{}
	cfg := NewDefaultServerConfig()
	cfg.IPBlocklist = ranger
	var s *Server
	serverReady := make(chan struct{})
	ranger.lookup = func(net.IP) {
		<-serverReady
		_ = s.Stats()
	}
	queryReceived := make(chan struct{})
	cfg.OnQuery = func(*krpc.Msg, net.Addr) bool {
		close(queryReceived)
		return false
	}
	s = newCallbackTestServer(t, cfg)
	close(serverReady)
	sender, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	packet := bencode.MustMarshal(krpc.Msg{
		T: "1",
		Y: "q",
		Q: "ping",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	if _, err := sender.WriteTo(packet, s.Addr()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-queryReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("receive path deadlocked when blocklist Lookup reentered Server.Stats")
	}
	s.Close()
}

func TestTableMaintainerLoggerCanReenterStats(t *testing.T) {
	var s *Server
	logHandled := make(chan struct{}, 1)
	cfg := NewDefaultServerConfig()
	cfg.Logger = log.NewLogger()
	cfg.Logger.SetHandlers(callbackTestLogHandler{onLog: func() {
		_ = s.Stats()
		select {
		case logHandled <- struct{}{}:
		default:
		}
	}})
	s = newCallbackTestServer(t, cfg)
	s.mu.Lock()
	s.lastBootstrap = time.Now()
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.TableMaintainer()
		close(done)
	}()
	select {
	case <-logHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("TableMaintainer logger deadlocked when its handler reentered Server.Stats")
	}
	s.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TableMaintainer did not stop after Server.Close")
	}
}

func TestRefreshBucketNodeFilterCanReenterStats(t *testing.T) {
	var s *Server
	lookupStarted := make(chan struct{}, 1)
	ranger := &callbackTestRanger{lookup: func(net.IP) {
		select {
		case lookupStarted <- struct{}{}:
		default:
		}
		_ = s.Stats()
	}}
	cfg := NewDefaultServerConfig()
	cfg.IPBlocklist = ranger
	cfg.QueryResendDelay = func() time.Duration { return time.Millisecond }
	s = newCallbackTestServer(t, cfg)
	if err := s.AddNode(krpc.NodeInfo{
		Addr: krpc.NodeAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1},
		ID:   krpc.ID{19: 1},
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		s.refreshBucket(0)
		close(done)
	}()
	select {
	case <-lookupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("refreshBucket did not run its node filter")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refreshBucket's node filter deadlocked when Ranger.Lookup reentered Server.Stats")
	}
	s.Close()
}

type callbackTestMarshaler struct {
	server *Server
}

func (m callbackTestMarshaler) MarshalBencode() ([]byte, error) {
	_ = m.server.Stats()
	return []byte("i1e"), nil
}

func TestStoreValueMarshalerCanReenterStats(t *testing.T) {
	store := new(callbackTestStore)
	cfg := NewDefaultServerConfig()
	cfg.Store = store
	s := newCallbackTestServer(t, cfg)
	item, err := bep44.NewItem(callbackTestMarshaler{server: s}, nil, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.Put(item); err != nil {
		t.Fatal(err)
	}
	processCallbackTestPacket(t, s, krpc.Msg{
		T: "1",
		Y: "q",
		Q: "get",
		A: &krpc.MsgArgs{ID: krpc.ID{19: 1}},
	})
	s.Close()
}
