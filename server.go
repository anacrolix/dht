package dht

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/pprof"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/anacrolix/chansync"
	"github.com/anacrolix/generics"
	"github.com/anacrolix/log"
	"github.com/anacrolix/sync"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/int160"
	"github.com/anacrolix/dht/v2/krpc"
	peer_store "github.com/anacrolix/dht/v2/peer-store"
	"github.com/anacrolix/dht/v2/transactions"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/dht/v2/types"
)

// A Server defines parameters for a DHT node server that is able to send
// queries, and respond to the ones from the network. Each node has a globally
// unique identifier known as the "node ID." Node IDs are chosen at random
// from the same 160-bit space as BitTorrent infohashes and define the
// behaviour of the node. Zero valued Server does not have a valid ID and thus
// is unable to function properly. Use `NewServer(nil)` to initialize a
// default node.
type Server struct {
	id          int160.T
	socket      net.PacketConn
	resendDelay func() time.Duration

	mu           sync.RWMutex
	transactions transactions.Dispatcher[*transaction]
	table        table
	closed       chansync.SetOnce
	ipBlockList  iplist.Ranger
	tokenServer  tokenServer // Manages tokens we issue to our queriers.
	config       ServerConfig
	stats        ServerStats

	lastBootstrap    time.Time
	bootstrappingNow bool

	store *bep44.Wrapper
}

func (s *Server) numGoodNodes() (num int) {
	s.table.forNodes(func(n *node) bool {
		if s.IsGood(n) {
			num++
		}
		return true
	})
	return
}

func prettySince(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	d /= time.Second
	d *= time.Second
	return fmt.Sprintf("%s ago", d)
}

func (s *Server) WriteStatus(w io.Writer) {
	fmt.Fprintf(w, "Listening on %s\n", s.Addr())
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(w, "Nodes in table: %d good, %d total\n", s.numGoodNodes(), s.numNodes())
	fmt.Fprintf(w, "Ongoing transactions: %d\n", s.transactions.NumActive())
	fmt.Fprintf(w, "Server node ID: %x\n", s.id.Bytes())
	buckets := &s.table.buckets
	for i := range s.table.buckets {
		b := &buckets[i]
		if b.Len() == 0 && b.lastChanged.IsZero() {
			continue
		}
		fmt.Fprintf(w,
			"b# %v: %v nodes, last updated: %v\n",
			i, b.Len(), prettySince(b.lastChanged))
		if b.Len() > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
			fmt.Fprintf(tw, "  node id\taddr\tlast query\tlast response\trecv\tdiscard\tflags\n")
			// Bucket nodes ordered by distance from server ID.
			nodes := slices.SortedFunc(b.NodeIter(), func(l *node, r *node) int {
				return l.Id.Distance(s.id).Cmp(r.Id.Distance(s.id))
			})
			for _, n := range nodes {
				var flags []string
				if s.IsQuestionable(n) {
					flags = append(flags, "q10e")
				}
				if s.nodeIsBad(n) {
					flags = append(flags, "bad")
				}
				if s.IsGood(n) {
					flags = append(flags, "good")
				}
				if n.IsSecure() {
					flags = append(flags, "sec")
				}
				fmt.Fprintf(tw, "  %x\t%s\t%s\t%s\t%d\t%v\t%v\n",
					n.Id.Bytes(),
					n.Addr,
					prettySince(n.lastGotQuery),
					prettySince(n.lastGotResponse),
					n.numReceivesFrom,
					n.failedLastQuestionablePing,
					strings.Join(flags, ","),
				)
			}
			tw.Flush()
		}
	}
	fmt.Fprintln(w)
}

func (s *Server) numNodes() (num int) {
	s.table.forNodes(func(n *node) bool {
		num++
		return true
	})
	return
}

// Stats returns statistics for the server.
func (s *Server) Stats() ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.stats
	ss.GoodNodes = s.numGoodNodes()
	ss.Nodes = s.numNodes()
	ss.OutstandingTransactions = s.transactions.NumActive()
	return ss
}

// Addr returns the listen address for the server. Packets arriving to this address
// are processed by the server (unless aliens are involved).
func (s *Server) Addr() net.Addr {
	return s.socket.LocalAddr()
}

func NewDefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		NoSecurity:    true,
		StartingNodes: func() ([]Addr, error) { return GlobalBootstrapAddrs("udp") },
		DefaultWant:   []krpc.Want{krpc.WantNodes, krpc.WantNodes6},
		Store:         bep44.NewMemory(),
		Exp:           2 * time.Hour,
		SendLimiter:   DefaultSendLimiter,
	}
}

// If the NodeId hasn't been specified, generate a suitable one. deterministic if c.Conn and
// c.PublicIP are non-nil.
func (c *ServerConfig) InitNodeId() (deterministic bool) {
	if c.NodeId.IsZero() {
		var secure bool
		if c.Conn != nil && c.PublicIP != nil {
			// Is this sufficient for a deterministic node ID?
			c.NodeId = HashTuple(
				[]byte(c.Conn.LocalAddr().Network()),
				[]byte(c.Conn.LocalAddr().String()),
				c.PublicIP,
			)
			// Since we have a public IP we can secure, and the choice must not be influenced by the
			// NoSecure configuration option.
			secure = true
			deterministic = true
		} else {
			c.NodeId = RandomNodeID()
			secure = !c.NoSecurity && c.PublicIP != nil
		}
		if secure {
			SecureNodeId(&c.NodeId, c.PublicIP)
		}
	}
	return
}

// NewServer initializes a new DHT node server.
func NewServer(c *ServerConfig) (s *Server, err error) {
	if c == nil {
		c = NewDefaultServerConfig()
	}
	if c.Conn == nil {
		c.Conn, err = net.ListenPacket("udp", ":0")
		if err != nil {
			return
		}
	}
	c.InitNodeId()
	// If Logger is empty, emulate the old behaviour: Everything is logged to the default location,
	// and there are no debug messages.
	if c.Logger.IsZero() {
		c.Logger = log.Default.FilterLevel(log.Info)
	}
	// Add log.Debug by default.
	c.Logger = c.Logger.WithDefaultLevel(log.Debug)

	if c.Store == nil {
		c.Store = bep44.NewMemory()
	}
	if c.SendLimiter == nil {
		c.SendLimiter = DefaultSendLimiter
	}

	s = &Server{
		config:      *c,
		ipBlockList: c.IPBlocklist,
		tokenServer: tokenServer{
			maxIntervalDelta: 2,
			interval:         5 * time.Minute,
			secret:           make([]byte, 20),
		},
		table: table{
			k: 8,
		},
		store: bep44.NewWrapper(c.Store, c.Exp),
	}
	rand.Read(s.tokenServer.secret)
	s.socket = c.Conn
	s.id = int160.FromByteArray(c.NodeId)
	s.table.rootID = s.id
	s.resendDelay = s.config.QueryResendDelay
	if s.resendDelay == nil {
		s.resendDelay = defaultQueryResendDelay
	}
	go s.serveUntilClosed()
	return
}

func (s *Server) serveUntilClosed() {
	err := s.serve()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.IsSet() {
		panic(err)
	}
}

// Returns a description of the Server.
func (s *Server) String() string {
	return fmt.Sprintf("dht server on %s (node id %v)", s.socket.LocalAddr(), s.id)
}

// Packets to and from any address matching a range in the list are dropped.
func (s *Server) SetIPBlockList(list iplist.Ranger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipBlockList = list
}

func (s *Server) IPBlocklist() iplist.Ranger {
	return s.ipBlockList
}

func (s *Server) processPacket(b []byte, addr Addr) {
	if len(b) < 2 || b[0] != 'd' {
		// KRPC messages are bencoded dicts.
		readNotKRPCDict.Add(1)
		return
	}
	var d krpc.Msg
	err := bencode.Unmarshal(b, &d)
	if _, ok := errors.AsType[bencode.ErrUnusedTrailingBytes](err); ok {
		expvars.Add("processed packets with trailing bytes", 1)
	} else if err != nil {
		readUnmarshalError.Add(1)
		if !uninterestingUnmarshalError(err, b) {
			s.logger().Printf("received bad krpc message from %s: %s: %+q", addr, err, b)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.IsSet() {
		return
	}
	if d.Y == "q" {
		expvars.Add("received queries", 1)
		s.logger().Printf("received query %q from %v", d.Q, addr)
		s.handleQuery(addr, d)
		return
	}
	tk := transactionKey{
		RemoteAddr: addr.String(),
		T:          d.T,
	}
	if !s.transactions.Have(tk) {
		s.logger().Printf("received response for untracked transaction %q from %v", d.T, addr)
		return
	}
	t := s.transactions.Pop(tk)
	go t.handleResponse(d)
	_ = s.updateNode(addr, d.SenderID(), !d.ReadOnly, func(n *node) {
		n.lastGotResponse = time.Now()
		n.failedLastQuestionablePing = false
		n.numReceivesFrom++
	})
}

// Reports whether a bencode decoding error is common junk not worth logging: truncated messages,
// messages that drop to NUL bytes abruptly, or data that isn't bencode at all.
func uninterestingUnmarshalError(err error, b []byte) bool {
	se, ok := errors.AsType[*bencode.SyntaxError](err)
	if !ok {
		return false
	}
	off := int(se.Offset)
	return off == 0 || off == len(b) || off < len(b) && b[off] == 0
}

func (s *Server) serve() error {
	var b [0x10000]byte
	for {
		n, addr, err := s.socket.ReadFrom(b[:])
		if err != nil {
			if ignoreReadFromError(err) {
				continue
			}
			return err
		}
		expvars.Add("packets read", 1)
		if n == len(b) {
			expvars.Add("received dht packet exceeds buffer size", 1)
			continue
		}
		if addrPort(addr) == 0 {
			readZeroPort.Add(1)
			continue
		}
		blocked, err := func() (bool, error) {
			s.mu.RLock()
			defer s.mu.RUnlock()
			if s.closed.IsSet() {
				return false, errors.New("server is closed")
			}
			return s.ipBlocked(addrIP(addr)), nil
		}()
		if err != nil {
			return err
		}
		if blocked {
			readBlocked.Add(1)
			continue
		}
		s.processPacket(b[:n], NewAddr(addr))
	}
}

func (s *Server) ipBlocked(ip net.IP) (blocked bool) {
	if s.ipBlockList == nil {
		return
	}
	_, blocked = s.ipBlockList.Lookup(ip)
	return
}

// Adds directly to the node table.
func (s *Server) AddNode(ni krpc.NodeInfo) error {
	id := int160.FromByteArray(ni.ID)
	if id.IsZero() {
		go s.Ping(ni.Addr.UDP())
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateNode(NewAddr(ni.Addr.UDP()), &ni.ID, true, func(*node) {})
}

func shouldReturnNodes(queryWants []krpc.Want, querySource net.IP) bool {
	if len(queryWants) != 0 {
		return slices.Contains(queryWants, krpc.WantNodes)
	}
	return querySource.To4() != nil
}

func shouldReturnNodes6(queryWants []krpc.Want, querySource net.IP) bool {
	if len(queryWants) != 0 {
		return slices.Contains(queryWants, krpc.WantNodes6)
	}
	return querySource.To4() == nil
}

var krpcErrMissingArguments = krpc.Error{
	Code: krpc.ErrorCodeProtocolError,
	Msg:  "missing arguments dict",
}

// Filters peers per BEP 32 to return in the values field to a get_peers query.
func filterPeers(querySourceIp net.IP, queryWants []krpc.Want, allPeers []krpc.NodeAddr) (filtered []krpc.NodeAddr) {
	retain4 := shouldReturnNodes(queryWants, querySourceIp)
	retain6 := shouldReturnNodes6(queryWants, querySourceIp)
	for _, peer := range allPeers {
		ip := peer.IP
		switch {
		case retain4 && len(ip) == net.IPv4len, retain6 && len(ip) == net.IPv6len:
		case retain4 && ip.To4() != nil:
			ip = ip.To4()
		case retain6 && ip.To16() != nil:
			ip = ip.To16()
		default:
			continue
		}
		filtered = append(filtered, krpc.NodeAddr{IP: ip, Port: peer.Port})
	}
	return
}

// Sets the BEP 32 node fields of r to the good nodes closest to target.
func (s *Server) setReturnNodes(r *krpc.Return, target krpc.ID, wants []krpc.Want, querySource Addr) {
	targetInt160 := target.Int160()
	if shouldReturnNodes(wants, querySource.IP()) {
		r.Nodes = s.closestGoodNodeInfos(8, targetInt160, func(na krpc.NodeAddr) bool { return na.IP.To4() != nil })
	}
	if shouldReturnNodes6(wants, querySource.IP()) {
		r.Nodes6 = s.closestGoodNodeInfos(8, targetInt160, func(krpc.NodeAddr) bool { return true })
	}
}

// Converts an error from the BEP 44 store into one suitable for returning to the querying node.
func storeError(err error) krpc.Error {
	if kerr, ok := errors.AsType[krpc.Error](err); ok {
		return kerr
	}
	return krpc.Error{
		Code: krpc.ErrorCodeGenericError,
		Msg:  err.Error(),
	}
}

func (s *Server) handleQuery(source Addr, m krpc.Msg) {
	go func() {
		expvars.Add(fmt.Sprintf("received query %q", m.Q), 1)
		if a := m.A; a != nil {
			if a.NoSeed != 0 {
				expvars.Add("received argument noseed", 1)
			}
			if a.Scrape != 0 {
				expvars.Add("received argument scrape", 1)
			}
		}
	}()
	_ = s.updateNode(source, m.SenderID(), !m.ReadOnly, func(n *node) {
		n.lastGotQuery = time.Now()
		n.numReceivesFrom++
	})
	if s.config.OnQuery != nil && !s.config.OnQuery(&m, source.Raw()) {
		return
	}
	if s.config.Passive {
		return
	}
	var handle func(source Addr, t string, args *krpc.MsgArgs)
	switch m.Q {
	case "ping":
		s.reply(source, m.T, krpc.Return{})
		return
	case "get_peers":
		handle = s.handleGetPeers
	case "find_node":
		handle = s.handleFindNode
	case "announce_peer":
		handle = s.handleAnnouncePeer
	case "put":
		handle = s.handlePut
	case "get":
		handle = s.handleGet
	default:
		// TODO: http://libtorrent.org/dht_extensions.html#forward-compatibility
		s.sendError(source, m.T, krpc.ErrorMethodUnknown)
		return
	}
	if m.A == nil {
		s.sendError(source, m.T, krpcErrMissingArguments)
		return
	}
	handle(source, m.T, m.A)
}

func (s *Server) handleGetPeers(source Addr, t string, args *krpc.MsgArgs) {
	var r krpc.Return
	if ps := s.config.PeerStore; ps != nil {
		r.Values = filterPeers(source.IP(), args.Want, ps.GetPeers(peer_store.InfoHash(args.InfoHash)))
		token := s.createToken(source)
		r.Token = &token
	}
	if len(r.Values) == 0 {
		s.setReturnNodes(&r, args.InfoHash, args.Want, source)
	}
	s.reply(source, t, r)
}

func (s *Server) handleFindNode(source Addr, t string, args *krpc.MsgArgs) {
	var r krpc.Return
	s.setReturnNodes(&r, args.Target, args.Want, source)
	s.reply(source, t, r)
}

func (s *Server) handleAnnouncePeer(source Addr, t string, args *krpc.MsgArgs) {
	if !s.validToken(args.Token, source) {
		expvars.Add("received announce_peer with invalid token", 1)
		return
	}
	expvars.Add("received announce_peer with valid token", 1)
	var port int
	portOk := false
	if args.Port != nil {
		port = *args.Port
		portOk = true
	}
	if args.ImpliedPort {
		expvars.Add("received announce_peer with implied_port", 1)
		port = source.Port()
		portOk = true
	}
	if !portOk {
		expvars.Add("received announce_peer with no derivable port", 1)
	}
	if h := s.config.OnAnnouncePeer; h != nil {
		go h(metainfo.Hash(args.InfoHash), source.IP(), port, portOk)
	}
	if ps := s.config.PeerStore; ps != nil {
		go ps.AddPeer(
			peer_store.InfoHash(args.InfoHash),
			krpc.NodeAddr{IP: source.IP(), Port: port},
		)
	}
	s.reply(source, t, krpc.Return{})
}

func (s *Server) handlePut(source Addr, t string, args *krpc.MsgArgs) {
	if !s.validToken(args.Token, source) {
		expvars.Add("received put with invalid token", 1)
		return
	}
	expvars.Add("received put with valid token", 1)
	i := &bep44.Item{
		V:    args.V,
		K:    args.K,
		Salt: args.Salt,
		Sig:  args.Sig,
		Cas:  args.Cas,
	}
	if i.IsMutable() {
		if args.Seq == nil {
			s.sendError(source, t, krpc.Error{
				Code: krpc.ErrorCodeProtocolError,
				Msg:  "expected seq argument for mutable item",
			})
			return
		}
		i.Seq = *args.Seq
	}
	if err := s.store.Put(i); err != nil {
		s.sendError(source, t, storeError(err))
		return
	}
	s.reply(source, t, krpc.Return{})
}

func (s *Server) handleGet(source Addr, t string, args *krpc.MsgArgs) {
	var r krpc.Return
	s.setReturnNodes(&r, args.Target, args.Want, source)
	token := s.createToken(source)
	r.Token = &token
	item, err := s.store.Get(bep44.Target(args.Target))
	if errors.Is(err, bep44.ErrItemNotFound) {
		s.reply(source, t, r)
		return
	}
	if err != nil {
		s.sendError(source, t, storeError(err))
		return
	}
	r.Seq = &item.Seq
	if args.Seq == nil || item.Seq > *args.Seq {
		r.V = bencode.MustMarshal(item.V)
		r.K = item.K
		r.Sig = item.Sig
	}
	s.reply(source, t, r)
}

func (s *Server) sendError(addr Addr, t string, e krpc.Error) {
	go func() {
		m := krpc.Msg{
			T: t,
			Y: "e",
			E: &e,
		}
		b, err := bencode.Marshal(m)
		if err != nil {
			panic(err)
		}
		s.logger().Printf("sending error to %q: %v", addr, e)
		_, err = s.writeToNode(context.Background(), b, addr, false, true)
		if err != nil {
			s.logger().Printf("error replying to %q: %v", addr, err)
		}
	}()
}

func (s *Server) reply(addr Addr, t string, r krpc.Return) {
	go func() {
		r.ID = s.id.AsByteArray()
		m := krpc.Msg{
			T:  t,
			Y:  "r",
			R:  &r,
			IP: addr.KRPC(),
		}
		b := bencode.MustMarshal(m)
		log.Fmsg("replying to %q", addr).Log(s.logger())
		wrote, err := s.writeToNode(context.Background(), b, addr, s.config.WaitToReply, true)
		if err != nil {
			s.config.Logger.Printf("error replying to %s: %s", addr, err)
		}
		if wrote {
			expvars.Add("replied to peer", 1)
		}
	}()
}

// Adds a node if appropriate.
func (s *Server) addNode(n *node) error {
	if s.nodeIsBad(n) {
		return errors.New("node is bad")
	}
	b := s.table.bucketForID(n.Id)
	if b.Len() >= s.table.k {
		if b.EachNode(func(bn *node) bool {
			// Replace bad and untested nodes with a good one.
			if s.nodeIsBad(bn) || (s.IsGood(n) && bn.lastGotResponse.IsZero()) {
				s.table.dropNode(bn)
			}
			return b.Len() >= s.table.k
		}) {
			return errors.New("no room in bucket")
		}
	}
	if err := s.table.addNode(n); err != nil {
		panic(fmt.Sprintf("expected to add node: %s", err))
	}
	return nil
}

func (s *Server) NodeRespondedToPing(addr Addr, id int160.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == s.id {
		return
	}
	b := s.table.bucketForID(id)
	if b.GetNode(addr, id) == nil {
		return
	}
	b.lastChanged = time.Now()
}

// Updates the node, adding it if appropriate.
func (s *Server) updateNode(addr Addr, id *krpc.ID, tryAdd bool, update func(*node)) error {
	if id == nil {
		return errors.New("id is nil")
	}
	int160Id := int160.FromByteArray(*id)
	n := s.table.getNode(addr, int160Id)
	missing := n == nil
	if missing {
		if !tryAdd {
			return errors.New("node not present and add flag false")
		}
		if int160Id == s.id {
			return errors.New("can't store own id in routing table")
		}
		n = &node{nodeKey: nodeKey{
			Id:   int160Id,
			Addr: addr,
		}}
	}
	update(n)
	if !missing {
		return nil
	}
	return s.addNode(n)
}

func (s *Server) nodeIsBad(n *node) bool {
	return s.nodeErr(n) != nil
}

func (s *Server) nodeErr(n *node) error {
	if n.Id == s.id {
		return errors.New("is self")
	}
	if n.Id.IsZero() {
		return errors.New("has zero id")
	}
	if !s.config.NoSecurity && !n.IsSecure() {
		return errors.New("not secure")
	}
	if n.failedLastQuestionablePing {
		return errors.New("didn't respond to last questionable node ping")
	}
	return nil
}

func (s *Server) writeToNode(ctx context.Context, b []byte, node Addr, wait, rate bool) (wrote bool, err error) {
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.closed.IsSet() {
			err = errors.New("server is closed")
			return
		}
		if list := s.ipBlockList; list != nil {
			if r, ok := list.Lookup(node.IP()); ok {
				err = fmt.Errorf("write to %v blocked by %v", node, r)
				return
			}
		}
	}()
	if err != nil {
		return
	}
	if rate {
		if wait {
			err = s.config.SendLimiter.Wait(ctx)
			if err != nil {
				err = fmt.Errorf("waiting for rate-limit token: %w", err)
				return false, err
			}
		} else {
			if !s.config.SendLimiter.Allow() {
				return false, errors.New("rate limit exceeded")
			}
		}
	}
	n, err := s.socket.WriteTo(b, node.Raw())
	writes.Add(1)
	if rate {
		expvars.Add("rated writes", 1)
	} else {
		expvars.Add("unrated writes", 1)
	}
	if err != nil {
		writeErrors.Add(1)
		if rate {
			// Return the token consumed by the failed write.
			s.config.SendLimiter.AllowN(time.Now(), -1)
		}
		err = fmt.Errorf("writing %d bytes to %s: %w", len(b), node, err)
		return
	}
	wrote = true
	if n != len(b) {
		err = io.ErrShortWrite
		return
	}
	return
}

func (s *Server) nextTransactionID() string {
	return transactions.DefaultIdIssuer.Issue()
}

func (s *Server) deleteTransaction(k transactionKey) {
	s.transactions.Delete(k)
}

func (s *Server) addTransaction(k transactionKey, t *transaction) {
	s.transactions.Add(k, t)
}

// ID returns the 20-byte server ID. This is the ID used to communicate with the
// DHT network.
func (s *Server) ID() [20]byte {
	return s.id.AsByteArray()
}

func (s *Server) createToken(addr Addr) string {
	return s.tokenServer.CreateToken(addr)
}

func (s *Server) validToken(token string, addr Addr) bool {
	return s.tokenServer.ValidToken(token, addr)
}

type numWrites int

func (s *Server) makeQueryBytes(q string, a krpc.MsgArgs, t string) []byte {
	a.ID = s.ID()
	m := krpc.Msg{
		T: t,
		Y: "q",
		Q: q,
		A: &a,
	}
	// BEP 43. Outgoing queries from passive nodes should contain "ro":1 in the top level
	// dictionary.
	if s.config.Passive {
		m.ReadOnly = true
	}
	b, err := bencode.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

type QueryResult struct {
	Reply  krpc.Msg
	Writes numWrites
	Err    error
}

func (qr QueryResult) ToError() error {
	if qr.Err != nil {
		return qr.Err
	}
	e := qr.Reply.Error()
	if e != nil {
		return e
	}
	return nil
}

// Converts a Server QueryResult to a traversal.QueryResult.
func (qr QueryResult) TraversalQueryResult(addr krpc.NodeAddr) (ret traversal.QueryResult) {
	r := qr.Reply.R
	if r == nil {
		return
	}
	ret.ResponseFrom = &krpc.NodeInfo{
		Addr: addr,
		ID:   r.ID,
	}
	ret.Nodes = r.Nodes
	ret.Nodes6 = r.Nodes6
	if r.Token != nil {
		ret.ClosestData = *r.Token
	}
	return
}

// Rate-limiting to be applied to writes for a given query. Queries occur inside transactions that
// may send several times.
type QueryRateLimiting struct {
	// Don't rate-limit the first send for a query.
	NotFirst bool
	// Don't rate-limit any sends for a query. Note that there's still built-in waits before retries.
	NotAny        bool
	WaitOnRetries bool
	NoWaitFirst   bool
}

// The zero value for this uses reasonable/traditional defaults on Server methods.
type QueryInput struct {
	MsgArgs      krpc.MsgArgs
	RateLimiting QueryRateLimiting
	NumTries     int
}

// Performs an arbitrary query. `q` is the query value, defined by the DHT BEP. `a` should contain
// the appropriate argument values, if any. `a.ID` is clobbered by the Server. Responses to queries
// made this way are not interpreted by the Server. More specific methods like FindNode and GetPeers
// may make use of the response internally before passing it back to the caller.
func (s *Server) Query(ctx context.Context, addr Addr, q string, input QueryInput) (ret QueryResult) {
	if input.NumTries == 0 {
		input.NumTries = defaultMaxQuerySends
	}
	defer func(started time.Time) {
		s.logger().WithDefaultLevel(log.Debug).WithValues(q).Printf(
			"Query(%v) returned after %v (err=%v, reply.Y=%v, reply.E=%v, writes=%v)",
			q, time.Since(started), ret.Err, ret.Reply.Y, ret.Reply.E, ret.Writes)
	}(time.Now())
	replyChan := make(chan krpc.Msg, 1)
	t := &transaction{
		onResponse: func(m krpc.Msg) {
			replyChan <- m
		},
	}
	tk := transactionKey{
		RemoteAddr: addr.String(),
	}
	s.mu.Lock()
	tid := s.nextTransactionID()
	s.stats.OutboundQueriesAttempted++
	tk.T = tid
	s.addTransaction(tk, t)
	s.mu.Unlock()
	// Receives the sender's terminal error, and closes when the sender completes.
	sendErr := make(chan error, 1)
	sendCtx, cancelSend := context.WithCancel(pprof.WithLabels(ctx, pprof.Labels("q", q)))
	go func() {
		sendErr <- s.transactionQuerySender(
			sendCtx,
			s.makeQueryBytes(q, input.MsgArgs, tid),
			&ret.Writes,
			addr,
			input.RateLimiting,
			input.NumTries)
		close(sendErr)
	}()
	expvars.Add(fmt.Sprintf("outbound %s queries", q), 1)
	select {
	case ret.Reply = <-replyChan:
	case <-ctx.Done():
		ret.Err = ctx.Err()
	case ret.Err = <-sendErr:
	}
	// Make sure the query sender stops.
	cancelSend()
	// Make sure the query sender has returned, it will either send an error that we didn't catch
	// above, or the channel will be closed by the sender completing.
	<-sendErr
	s.mu.Lock()
	s.deleteTransaction(tk)
	s.mu.Unlock()
	return
}

func (s *Server) transactionQuerySender(
	sendCtx context.Context,
	b []byte,
	writes *numWrites,
	addr Addr,
	rateLimiting QueryRateLimiting,
	numTries int,
) error {
	err := transactionSender(
		sendCtx,
		func() error {
			first := *writes == 0
			wait := rateLimiting.WaitOnRetries
			if first {
				wait = !rateLimiting.NoWaitFirst
			}
			rate := !rateLimiting.NotAny && (!first || !rateLimiting.NotFirst)
			wrote, err := s.writeToNode(sendCtx, b, addr, wait, rate)
			if wrote {
				*writes++
			}
			return err
		},
		s.resendDelay,
		numTries,
	)
	if err != nil {
		return err
	}
	select {
	case <-sendCtx.Done():
		err = sendCtx.Err()
	case <-time.After(s.resendDelay()):
		err = TransactionTimeout
	}
	return fmt.Errorf("after %v tries: %w", numTries, err)
}

// Sends a ping query to the address given.
func (s *Server) PingQueryInput(node *net.UDPAddr, qi QueryInput) QueryResult {
	addr := NewAddr(node)
	res := s.Query(context.TODO(), addr, "ping", qi)
	if res.Err == nil {
		id := res.Reply.SenderID()
		if id != nil {
			s.NodeRespondedToPing(addr, id.Int160())
		}
	}
	return res
}

// Sends a ping query to the address given.
func (s *Server) Ping(node *net.UDPAddr) QueryResult {
	return s.PingQueryInput(node, QueryInput{})
}

// Put adds a new item to node. You need to call Get first for a write token.
func (s *Server) Put(ctx context.Context, node Addr, i bep44.Put, token string, rl QueryRateLimiting) QueryResult {
	if err := s.store.Put(i.ToItem()); err != nil {
		return QueryResult{
			Err: err,
		}
	}
	qi := QueryInput{
		MsgArgs: krpc.MsgArgs{
			Cas:   i.Cas,
			Salt:  i.Salt,
			Seq:   &i.Seq,
			Sig:   i.Sig,
			Token: token,
			V:     i.V,
		},
		RateLimiting: rl,
	}
	if i.K != nil {
		qi.MsgArgs.K = *i.K
	}
	return s.Query(ctx, node, "put", qi)
}

func (s *Server) announcePeer(
	ctx context.Context,
	node Addr, infoHash int160.T, port int, token string, impliedPort bool, rl QueryRateLimiting,
) (
	ret QueryResult,
) {
	if port == 0 && !impliedPort {
		ret.Err = errors.New("no port specified")
		return
	}
	ret = s.Query(
		ctx, node, "announce_peer",
		QueryInput{
			MsgArgs: krpc.MsgArgs{
				ImpliedPort: impliedPort,
				InfoHash:    infoHash.AsByteArray(),
				Port:        &port,
				Token:       token,
			},
			RateLimiting: rl,
		})
	if ret.Err != nil {
		return
	}
	if krpcError := ret.Reply.Error(); krpcError != nil {
		announceErrors.Add(1)
		ret.Err = krpcError
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.SuccessfulOutboundAnnouncePeerQueries++
	return
}

// Sends a find_node query to addr. targetID is the node we're looking for. The Server makes use of
// some of the response fields.
func (s *Server) FindNode(addr Addr, targetID int160.T, rl QueryRateLimiting) (ret QueryResult) {
	ret = s.Query(context.TODO(), addr, "find_node", QueryInput{
		MsgArgs: krpc.MsgArgs{
			Target: targetID.AsByteArray(),
			Want:   s.config.DefaultWant,
		},
		RateLimiting: rl,
	})
	return
}

// Returns how many nodes are in the node table.
func (s *Server) NumNodes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.numNodes()
}

// Returns non-bad nodes from the routing table.
func (s *Server) Nodes() (nis []krpc.NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notBadNodes()
}

// Returns non-bad nodes from the routing table.
func (s *Server) notBadNodes() (nis []krpc.NodeInfo) {
	s.table.forNodes(func(n *node) bool {
		if s.nodeIsBad(n) {
			return true
		}
		nis = append(nis, krpc.NodeInfo{
			Addr: n.Addr.KRPC(),
			ID:   n.Id.AsByteArray(),
		})
		return true
	})
	return
}

// Stops the server network activity. This is all that's required to clean-up a Server.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed.Set()
	go s.socket.Close()
}

func (s *Server) GetPeers(
	ctx context.Context,
	addr Addr,
	infoHash int160.T,
	// Be advised that if you set this, you might not get any "Return.values" back. That wasn't my
	// reading of BEP 33 but there you go.
	scrape bool,
	rl QueryRateLimiting,
) (ret QueryResult) {
	args := krpc.MsgArgs{
		InfoHash: infoHash.AsByteArray(),
		// TODO: Maybe IPv4-only Servers won't want IPv6 nodes?
		Want: s.config.DefaultWant,
	}
	if scrape {
		args.Scrape = 1
	}
	ret = s.Query(ctx, addr, "get_peers", QueryInput{
		MsgArgs:      args,
		RateLimiting: rl,
	})
	m := ret.Reply
	if m.R != nil {
		if m.R.Token == nil {
			expvars.Add("get_peers responses with no token", 1)
		} else if len(*m.R.Token) == 0 {
			expvars.Add("get_peers responses with empty token", 1)
		} else {
			expvars.Add("get_peers responses with token", 1)
		}
	}
	return
}

// Get gets item information from a specific target ID. If seq is set to a specific value,
// only items with seq bigger than the one provided will return a V, K and Sig, if any.
// Get must be used to get a Put write token, when you want to write an item instead of read it.
func (s *Server) Get(ctx context.Context, addr Addr, target bep44.Target, seq *int64, rl QueryRateLimiting) QueryResult {
	return s.Query(ctx, addr, "get", QueryInput{
		MsgArgs: krpc.MsgArgs{
			Target: target,
			Seq:    seq,
			Want:   []krpc.Want{krpc.WantNodes, krpc.WantNodes6},
		},
		RateLimiting: rl,
	})
}

func (s *Server) closestGoodNodeInfos(
	k int,
	targetID int160.T,
	filter func(krpc.NodeAddr) bool,
) (
	ret []krpc.NodeInfo,
) {
	for _, n := range s.closestNodes(k, targetID, func(n *node) bool {
		return s.IsGood(n) && filter(n.NodeInfo().Addr)
	}) {
		ret = append(ret, n.NodeInfo())
	}
	return
}

func (s *Server) closestNodes(k int, target int160.T, filter func(*node) bool) []*node {
	return s.table.closestNodes(k, target, filter)
}

func (s *Server) TraversalStartingNodes() (nodes []addrMaybeId, err error) {
	s.mu.RLock()
	s.table.forNodes(func(n *node) bool {
		nodes = append(nodes, addrMaybeId{
			Addr: n.Addr.KRPC().ToNodeAddrPort(),
			Id:   generics.Some(n.Id)})
		return true
	})
	s.mu.RUnlock()
	if len(nodes) > 0 {
		return
	}
	if s.config.StartingNodes != nil {
		// There seems to be floods on this call on occasion, which may cause a barrage of DNS
		// resolution attempts. This would require that we're unable to get replies because we can't
		// resolve, transmit or receive on the network. Nodes currently don't get expired from the
		// table, so once we have some entries, we should never have to fallback.
		s.logger().Levelf(log.Debug, "falling back on starting nodes")
		addrs, err := s.config.StartingNodes()
		if err != nil {
			return nil, fmt.Errorf("getting starting nodes: %w", err)
		}
		for _, a := range addrs {
			nodes = append(nodes, addrMaybeId{Addr: a.KRPC().ToNodeAddrPort()})
		}
	}
	if len(nodes) == 0 {
		err = errors.New("no initial nodes")
	}
	return
}

func (s *Server) AddNodesFromFile(fileName string) (added int, err error) {
	ns, err := ReadNodesFromFile(fileName)
	if err != nil {
		return
	}
	for _, n := range ns {
		if s.AddNode(n) == nil {
			added++
		}
	}
	return
}

func (s *Server) logger() log.Logger {
	return s.config.Logger
}

func (s *Server) PeerStore() peer_store.Interface {
	return s.config.PeerStore
}

func (s *Server) shouldStopRefreshingBucket(bucketIndex int) bool {
	if s.closed.IsSet() {
		return true
	}
	b := &s.table.buckets[bucketIndex]
	// Stop if the bucket is full, and none of the nodes are bad.
	return b.Len() == s.table.K() && b.EachNode(func(n *node) bool {
		return !s.nodeIsBad(n)
	})
}

func (s *Server) refreshBucket(bucketIndex int) *traversal.Stats {
	s.mu.RLock()
	id := s.table.randomIdForBucket(bucketIndex)
	op := traversal.Start(traversal.OperationInput{
		Target: id.AsByteArray(),
		Alpha:  3,
		// Running this to completion with K matching the full-bucket size should result in a good,
		// full bucket, since the Server will add nodes that respond to its table to replace the bad
		// ones we're presumably refreshing. It might be possible to terminate the traversal early
		// as soon as the bucket is good.
		K: s.table.K(),
		DoQuery: func(ctx context.Context, addr krpc.NodeAddr) traversal.QueryResult {
			res := s.FindNode(NewAddr(addr.UDP()), id, QueryRateLimiting{})
			err := res.Err
			if err != nil && !errors.Is(err, TransactionTimeout) {
				s.logger().Levelf(log.Debug, "error doing find node while refreshing bucket: %v", err)
			}
			return res.TraversalQueryResult(addr)
		},
		NodeFilter: s.TraversalNodeFilter,
	})
	defer func() {
		s.mu.RUnlock()
		op.Stop()
		<-op.Stopped()
	}()
	b := &s.table.buckets[bucketIndex]
wait:
	for {
		if s.shouldStopRefreshingBucket(bucketIndex) {
			break wait
		}
		op.AddNodes(types.AddrMaybeIdSliceFromNodeInfoSlice(s.notBadNodes()))
		bucketChanged := b.changed.Signaled()
		serverClosed := s.closed.Done()
		s.mu.RUnlock()
		select {
		case <-op.Stalled():
			s.mu.RLock()
			break wait
		case <-bucketChanged:
		case <-serverClosed:
		}
		s.mu.RLock()
	}
	return op.Stats()
}

func (s *Server) shouldBootstrap() bool {
	return s.lastBootstrap.IsZero() || time.Since(s.lastBootstrap) > 30*time.Minute
}

func (s *Server) shouldBootstrapUnlocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shouldBootstrap()
}

func (s *Server) pingQuestionableNodesInBucket(bucketIndex int) {
	b := &s.table.buckets[bucketIndex]
	var wg sync.WaitGroup
	b.EachNode(func(n *node) bool {
		if s.IsQuestionable(n) {
			wg.Go(func() {
				err := s.questionableNodePing(context.TODO(), n.Addr, n.Id.AsByteArray()).Err
				if err != nil {
					s.logger().WithDefaultLevel(log.Debug).Printf("error pinging questionable node in bucket %v: %v", bucketIndex, err)
				}
			})
		}
		return true
	})
	s.mu.RUnlock()
	wg.Wait()
	s.mu.RLock()
}

// A routine that maintains the Server's routing table, by pinging questionable nodes, and
// refreshing buckets. This should be invoked on a running Server when the caller is satisfied with
// having set it up. It is not necessary to explicitly Bootstrap the Server once this routine has
// started.
func (s *Server) TableMaintainer() {
	logger := s.logger()
	for {
		if s.shouldBootstrapUnlocked() {
			stats, err := s.Bootstrap()
			if err != nil {
				logger.Levelf(log.Error, "error bootstrapping during bucket refresh: %v", err)
			}
			logger.Levelf(log.Debug, "bucket refresh bootstrap stats: %v", stats)
		}
		s.mu.RLock()
		for i := range s.table.buckets {
			s.pingQuestionableNodesInBucket(i)
			if s.shouldStopRefreshingBucket(i) {
				continue
			}
			logger.Levelf(log.Debug, "refreshing bucket %v", i)
			s.mu.RUnlock()
			stats := s.refreshBucket(i)
			logger.Levelf(log.Debug, "finished refreshing bucket %v: %v", i, stats)
			s.mu.RLock()
			if !s.shouldStopRefreshingBucket(i) {
				// Presumably we couldn't fill the bucket anymore, so assume we're as deep in the
				// available node space as we can go.
				break
			}
		}
		s.mu.RUnlock()
		select {
		case <-s.closed.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func (s *Server) questionableNodePing(ctx context.Context, addr Addr, id krpc.ID) QueryResult {
	// A ping query that will be certain to try at least 3 times.
	res := s.Query(ctx, addr, "ping", QueryInput{
		RateLimiting: QueryRateLimiting{
			WaitOnRetries: true,
		},
		NumTries: 3,
	})
	if res.Err == nil && res.Reply.R != nil {
		s.NodeRespondedToPing(addr, res.Reply.R.ID.Int160())
	} else {
		s.mu.Lock()
		_ = s.updateNode(addr, &id, false, func(n *node) {
			n.failedLastQuestionablePing = true
		})
		s.mu.Unlock()
	}
	return res
}

// Whether we should consider a node for contact based on its address and possible ID.
func (s *Server) TraversalNodeFilter(node addrMaybeId) bool {
	if !validNodeAddr(node.Addr.UDP()) {
		return false
	}
	if s.ipBlocked(node.Addr.IP()) {
		return false
	}
	if !node.Id.Ok {
		return true
	}
	return s.config.NoSecurity || NodeIdSecure(node.Id.Value.AsByteArray(), node.Addr.IP())
}

func validNodeAddr(ua *net.UDPAddr) bool {
	if ua.Port == 0 {
		return false
	}
	// 0.0.0.0/8 addresses "this network" and can't be a destination (RFC 1122).
	if ip4 := ua.IP.To4(); ip4 != nil && ip4[0] == 0 {
		return false
	}
	return true
}

// func (s *Server) refreshBucket(bucketIndex int) {
//	targetId := s.table.randomIdForBucket(bucketIndex)
// }
