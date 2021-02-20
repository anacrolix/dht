package dht

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"runtime/pprof"
	"text/tabwriter"
	"time"

	peer_store "github.com/anacrolix/dht/v2/peer-store"
	"github.com/anacrolix/log"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/v2/conntrack"
	"github.com/anacrolix/sync"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/logonce"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/pkg/errors"

	"github.com/anacrolix/stm"

	"github.com/anacrolix/dht/v2/krpc"
)

// A Server defines parameters for a DHT node server that is able to send
// queries, and respond to the ones from the network. Each node has a globally
// unique identifier known as the "node ID." Node IDs are chosen at random
// from the same 160-bit space as BitTorrent infohashes and define the
// behaviour of the node. Zero valued Server does not have a valid ID and thus
// is unable to function properly. Use `NewServer(nil)` to initialize a
// default node.
type Server struct {
	id          int160
	socket      net.PacketConn
	resendDelay func() time.Duration

	mu           sync.RWMutex
	transactions map[transactionKey]*Transaction
	nextT        uint64 // unique "t" field for outbound queries
	table        table
	closed       missinggo.Event
	ipBlockList  iplist.Ranger
	tokenServer  tokenServer // Manages tokens we issue to our queriers.
	config       ServerConfig
	stats        ServerStats
	sendLimit    sendLimiter
}

type sendLimiter interface {
	Wait(ctx context.Context) error
	Allow() bool
	AllowStm(tx *stm.Tx) bool
}

func (s *Server) numGoodNodes() (num int) {
	s.table.forNodes(func(n *node) bool {
		if n.IsGood() {
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
	fmt.Fprintf(w, "Ongoing transactions: %d\n", len(s.transactions))
	fmt.Fprintf(w, "Server node ID: %x\n", s.id.Bytes())
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
	fmt.Fprintf(tw, "b#\tnode id\taddr\tanntok\tlast query\tlast response\tcf\tro\n")
	for i, b := range s.table.buckets {
		b.EachNode(func(n *node) bool {
			fmt.Fprintf(tw, "%d\t%x\t%s\t%v\t%s\t%s\t%d\t%v\n",
				i,
				n.id.Bytes(),
				n.addr,
				func() int {
					if n.announceToken == nil {
						return -1
					}
					return len(*n.announceToken)
				}(),
				prettySince(n.lastGotQuery),
				prettySince(n.lastGotResponse),
				n.consecutiveFailures,
				n.readOnly,
			)
			return true
		})
	}
	tw.Flush()
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
	ss.OutstandingTransactions = len(s.transactions)
	return ss
}

// Addr returns the listen address for the server. Packets arriving to this address
// are processed by the server (unless aliens are involved).
func (s *Server) Addr() net.Addr {
	return s.socket.LocalAddr()
}

func NewDefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Conn:               mustListen(":0"),
		NoSecurity:         true,
		StartingNodes:      func() ([]Addr, error) { return GlobalBootstrapAddrs("udp") },
		ConnectionTracking: conntrack.NewInstance(),
	}
}

// NewServer initializes a new DHT node server.
func NewServer(c *ServerConfig) (s *Server, err error) {
	if c == nil {
		c = NewDefaultServerConfig()
	}
	if c.Conn == nil {
		return nil, errors.New("non-nil Conn required")
	}
	if missinggo.IsZeroValue(c.NodeId) {
		c.NodeId = RandomNodeID()
		if !c.NoSecurity && c.PublicIP != nil {
			SecureNodeId(&c.NodeId, c.PublicIP)
		}
	}
	// If Logger is empty, emulate the old behaviour: Everything is logged to the default location,
	// and there are no debug messages.
	if c.Logger.LoggerImpl == nil {
		c.Logger = log.Default.FilterLevel(log.Info)
	}
	// Add log.Debug by default.
	c.Logger = c.Logger.WithDefaultLevel(log.Debug)

	s = &Server{
		config:      *c,
		ipBlockList: c.IPBlocklist,
		tokenServer: tokenServer{
			maxIntervalDelta: 2,
			interval:         5 * time.Minute,
			secret:           make([]byte, 20),
		},
		transactions: make(map[transactionKey]*Transaction),
		table: table{
			k: 8,
		},
		sendLimit: defaultSendLimiter,
	}
	if s.config.ConnectionTracking == nil {
		s.config.ConnectionTracking = conntrack.NewInstance()
	}
	rand.Read(s.tokenServer.secret)
	s.socket = c.Conn
	s.id = int160FromByteArray(c.NodeId)
	s.table.rootID = s.id
	s.resendDelay = s.config.QueryResendDelay
	if s.resendDelay == nil {
		s.resendDelay = defaultQueryResendDelay
	}
	go s.questionableNodePinger()
	go s.serveUntilClosed()
	return
}

func (s *Server) serveUntilClosed() {
	err := s.serve()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.IsSet() {
		return
	}
	if err != nil {
		panic(err)
	}
}

// Returns a description of the Server.
func (s *Server) String() string {
	return fmt.Sprintf("dht server on %s", s.socket.LocalAddr())
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
	if _, ok := err.(bencode.ErrUnusedTrailingBytes); ok {
		// log.Printf("%s: received message packet with %d trailing bytes: %q", s, _err.NumUnusedBytes, b[len(b)-_err.NumUnusedBytes:])
		expvars.Add("processed packets with trailing bytes", 1)
	} else if err != nil {
		readUnmarshalError.Add(1)
		func() {
			if se, ok := err.(*bencode.SyntaxError); ok {
				// The message was truncated.
				if int(se.Offset) == len(b) {
					return
				}
				// Some messages seem to drop to nul chars abrubtly.
				if int(se.Offset) < len(b) && b[se.Offset] == 0 {
					return
				}
				// The message isn't bencode from the first.
				if se.Offset == 0 {
					return
				}
			}
			// if missinggo.CryHeard() {
			// 	log.Printf("%s: received bad krpc message from %s: %s: %+q", s, addr, err, b)
			// }
		}()
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
	t, ok := s.transactions[tk]
	if !ok {
		s.logger().Printf("received response for untracked transaction %q from %v", d.T, addr)
		return
	}
	//s.logger().Printf("received response for transaction %q from %v", d.T, addr)
	go t.handleResponse(d)
	s.updateNode(addr, d.SenderID(), true, func(n *node) {
		n.lastGotResponse = time.Now()
		n.consecutiveFailures = 0
		n.readOnly = d.ReadOnly
	})
	// Ensure we don't provide more than one response to a transaction.
	s.deleteTransaction(tk)
}

func (s *Server) serve() error {
	var b [0x10000]byte
	for {
		n, addr, err := s.socket.ReadFrom(b[:])
		if err != nil {
			return err
		}
		expvars.Add("packets read", 1)
		if n == len(b) {
			logonce.Stderr.Printf("received dht packet exceeds buffer size")
			continue
		}
		if missinggo.AddrPort(addr) == 0 {
			readZeroPort.Add(1)
			continue
		}
		s.mu.Lock()
		blocked := s.ipBlocked(missinggo.AddrIP(addr))
		s.mu.Unlock()
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
	id := int160FromByteArray(ni.ID)
	if id.IsZero() {
		return s.Ping(ni.Addr.UDP(), nil)
	}
	return s.updateNode(NewAddr(ni.Addr.UDP()), (*krpc.ID)(&ni.ID), true, func(*node) {})
}

func wantsContain(ws []krpc.Want, w krpc.Want) bool {
	for _, _w := range ws {
		if _w == w {
			return true
		}
	}
	return false
}

func shouldReturnNodes(queryWants []krpc.Want, querySource net.IP) bool {
	if len(queryWants) != 0 {
		return wantsContain(queryWants, krpc.WantNodes)
	}
	// Is it possible to be over IPv6 with IPv4 endpoints?
	return querySource.To4() != nil
}

func shouldReturnNodes6(queryWants []krpc.Want, querySource net.IP) bool {
	if len(queryWants) != 0 {
		return wantsContain(queryWants, krpc.WantNodes6)
	}
	return querySource.To4() == nil
}

func (s *Server) makeReturnNodes(target int160, filter func(krpc.NodeAddr) bool) []krpc.NodeInfo {
	return s.closestGoodNodeInfos(8, target, filter)
}

var krpcErrMissingArguments = krpc.Error{
	Code: krpc.ErrorCodeProtocolError,
	Msg:  "missing arguments dict",
}

// Filters peers per BEP 32 to return in the values field to a get_peers query.
func filterPeers(querySourceIp net.IP, queryWants []krpc.Want, allPeers []krpc.NodeAddr) (filtered []krpc.NodeAddr) {
	// The logic here is common with nodes, see BEP 32.
	retain4 := shouldReturnNodes(queryWants, querySourceIp)
	retain6 := shouldReturnNodes6(queryWants, querySourceIp)
	for _, peer := range allPeers {
		if ip, ok := func(ip net.IP) (net.IP, bool) {
			as4 := peer.IP.To4()
			as16 := peer.IP.To16()
			switch {
			case retain4 && len(ip) == net.IPv4len:
				return ip, true
			case retain6 && len(ip) == net.IPv6len:
				return ip, true
			case retain4 && as4 != nil:
				// Is it possible that we're converting to an IPv4 address when the transport in use
				// is IPv6?
				return as4, true
			case retain6 && as16 != nil:
				// Couldn't any IPv4 address be converted to IPv6, but isn't listening over IPv6?
				return as16, true
			default:
				return nil, false
			}
		}(peer.IP); ok {
			filtered = append(filtered, krpc.NodeAddr{ip, peer.Port})
		}
	}
	return
}

func (s *Server) setReturnNodes(r *krpc.Return, queryMsg krpc.Msg, querySource Addr) *krpc.Error {
	if queryMsg.A == nil {
		return &krpcErrMissingArguments
	}
	target := int160FromByteArray(queryMsg.A.InfoHash)
	if shouldReturnNodes(queryMsg.A.Want, querySource.IP()) {
		r.Nodes = s.makeReturnNodes(target, func(na krpc.NodeAddr) bool { return na.IP.To4() != nil })
	}
	if shouldReturnNodes6(queryMsg.A.Want, querySource.IP()) {
		r.Nodes6 = s.makeReturnNodes(target, func(krpc.NodeAddr) bool { return true })
	}
	return nil
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
	s.updateNode(source, m.SenderID(), true, func(n *node) {
		n.lastGotQuery = time.Now()
		n.readOnly = m.ReadOnly
	})
	if s.config.OnQuery != nil {
		propagate := s.config.OnQuery(&m, source.Raw())
		if !propagate {
			return
		}
	}
	// Don't respond.
	if s.config.Passive {
		return
	}
	// TODO: Should we disallow replying to ourself?
	args := m.A
	switch m.Q {
	case "ping":
		s.reply(source, m.T, krpc.Return{})
	case "get_peers":
		// Check for the naked m.A.Want deref below.
		if m.A == nil {
			s.sendError(source, m.T, krpcErrMissingArguments)
			break
		}
		var r krpc.Return
		if ps := s.config.PeerStore; ps != nil {
			r.Values = filterPeers(source.IP(), m.A.Want, ps.GetPeers(peer_store.InfoHash(args.InfoHash)))
		}
		if len(r.Values) == 0 {
			if err := s.setReturnNodes(&r, m, source); err != nil {
				s.sendError(source, m.T, *err)
				break
			}
		}
		// I wonder if we could choose not to return a token here, if we don't want an announce_peer
		// from the querier.
		r.Token = func() *string {
			t := s.createToken(source)
			return &t
		}()
		s.reply(source, m.T, r)
	case "find_node":
		var r krpc.Return
		if err := s.setReturnNodes(&r, m, source); err != nil {
			s.sendError(source, m.T, *err)
			break
		}
		s.reply(source, m.T, r)
	case "announce_peer":
		readAnnouncePeer.Add(1)

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
				krpc.NodeAddr{source.IP(), port},
			)
		}

		s.reply(source, m.T, krpc.Return{})
	default:
		s.sendError(source, m.T, krpc.ErrorMethodUnknown)
	}
}

func (s *Server) sendError(addr Addr, t string, e krpc.Error) {
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
}

func (s *Server) reply(addr Addr, t string, r krpc.Return) {
	r.ID = s.id.AsByteArray()
	m := krpc.Msg{
		T:  t,
		Y:  "r",
		R:  &r,
		IP: addr.KRPC(),
	}
	b, err := bencode.Marshal(m)
	if err != nil {
		panic(err)
	}
	log.Fmsg("replying to %q", addr).Log(s.logger())
	wrote, err := s.writeToNode(context.Background(), b, addr, false, true)
	if err != nil {
		s.config.Logger.Printf("error replying to %s: %s", addr, err)
	}
	if wrote {
		expvars.Add("replied to peer", 1)
	}
}

// Adds a node if appropriate.
func (s *Server) addNode(n *node) error {
	b := s.table.bucketForID(n.id)
	if b.Len() >= s.table.k {
		if b.EachNode(func(n *node) bool {
			if s.nodeIsBad(n) {
				s.table.dropNode(n)
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

// Updates the node, adding it if appropriate.
func (s *Server) updateNode(addr Addr, id *krpc.ID, tryAdd bool, update func(*node)) error {
	if id == nil {
		return errors.New("id is nil")
	}
	int160Id := int160FromByteArray(*id)
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
			id:   int160Id,
			addr: addr,
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
	if n.id == s.id {
		return errors.New("is self")
	}
	if n.id.IsZero() {
		return errors.New("has zero id")
	}
	if !s.config.NoSecurity && !n.IsSecure() {
		return errors.New("not secure")
	}
	if n.readOnly {
		return errors.New("is read-only")
	}
	if n.lastGotQuery.IsZero() && n.lastGotResponse.IsZero() {
		return errors.New("has never communicated")
	}
	if n.consecutiveFailures >= 3 {
		return fmt.Errorf("has %d consecutive failures", n.consecutiveFailures)
	}
	return nil
}

func (s *Server) writeToNode(ctx context.Context, b []byte, node Addr, wait, rate bool) (wrote bool, err error) {
	if list := s.ipBlockList; list != nil {
		if r, ok := list.Lookup(node.IP()); ok {
			err = fmt.Errorf("write to %v blocked by %v", node, r)
			return
		}
	}
	//s.config.Logger.WithValues(log.Debug).Printf("writing to %s: %q", node.String(), b)
	if rate {
		if wait {
			err = s.sendLimit.Wait(ctx)
			if err != nil {
				return false, err
			}
		} else {
			if !s.sendLimit.Allow() {
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
		err = fmt.Errorf("error writing %d bytes to %s: %s", len(b), node, err)
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
	var b [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(b[:], s.nextT)
	s.nextT++
	return string(b[:n])
}

func (s *Server) deleteTransaction(k transactionKey) {
	delete(s.transactions, k)
}

func (s *Server) addTransaction(k transactionKey, t *Transaction) {
	if _, ok := s.transactions[k]; ok {
		panic("transaction not unique")
	}
	s.transactions[k] = t
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

func (s *Server) connTrackEntryForAddr(a Addr) conntrack.Entry {
	return conntrack.Entry{
		s.socket.LocalAddr().Network(),
		s.socket.LocalAddr().String(),
		a.String(),
	}
}

type numWrites int

// Returns an STM operation that returns a func() when the Server's connection tracking and send
// rate-limiting allow, that executes `f`, where `f` returns the number of send operations actually
// performed. After `f` completes, the func rectifies any rate-limiting against the number of writes
// reported. If the operation returns, the *first* write has been accounted for already (See
// QueryRateLimiting.NotFirst).
func (s *Server) beginQuery(addr Addr, reason string, f func() numWrites) stm.Operation {
	return func(tx *stm.Tx) interface{} {
		tx.Assert(s.sendLimit.AllowStm(tx))
		cteh := s.config.ConnectionTracking.Allow(tx, s.connTrackEntryForAddr(addr), reason, -1)
		tx.Assert(cteh != nil)
		return func() {
			writes := f()
			finalizeCteh(cteh, writes)
		}
	}
}

func (s *Server) query(addr Addr, q string, a krpc.MsgArgs, callback func(krpc.Msg, error)) error {
	if callback == nil {
		callback = func(krpc.Msg, error) {}
	}
	go func() {
		stm.Atomically(
			s.beginQuery(addr, fmt.Sprintf("send dht query %q", q),
				func() numWrites {
					res := s.Query(context.Background(), addr, q, QueryInput{
						MsgArgs: a, RateLimiting: QueryRateLimiting{NotFirst: true}})
					callback(res.Reply, res.Err)
					return res.writes
				},
			),
		).(func())()
	}()
	return nil
}

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
	writes numWrites
	Err    error
}

// Rate-limiting to be applied to writes for a given query. Queries occur inside transactions that
// will attempt to send several times. If the STM rate-limiting helpers are used, the first send is
// often already accounted for in the rate-limiting machinery before the query method that does the
// IO is invoked.
type QueryRateLimiting struct {
	// Don't rate-limit the first send for a query.
	NotFirst bool
	// Don't rate-limit any sends for a query. Note that there's still built-in waits before retries.
	NotAny bool
}

type QueryInput struct {
	MsgArgs      krpc.MsgArgs
	RateLimiting QueryRateLimiting
}

// Performs an arbitrary query. `q` is the query value, defined by the DHT BEP. `a` should contain
// the appropriate argument values, if any. `a.ID` is clobbered by the Server. Responses to queries
// made this way are not interpreted by the Server. More specific methods like FindNode and GetPeers
// may make use of the response internally before passing it back to the caller.
func (s *Server) Query(ctx context.Context, addr Addr, q string, input QueryInput) (ret QueryResult) {
	defer func(started time.Time) {
		s.logger().WithDefaultLevel(log.Debug).WithValues(q).Printf(
			"Query(%v) returned after %v (err=%v, reply.Y=%v, reply.E=%v, writes=%v)",
			q, time.Since(started), ret.Err, ret.Reply.Y, ret.Reply.E, ret.writes)
	}(time.Now())
	replyChan := make(chan krpc.Msg, 1)
	t := &Transaction{
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
	// Receives a non-nil error from the sender, and closes when the sender completes.
	sendErr := make(chan error, 1)
	sendCtx, cancelSend := context.WithCancel(pprof.WithLabels(ctx, pprof.Labels("q", q)))
	go func() {
		err := s.transactionQuerySender(
			sendCtx,
			s.makeQueryBytes(q, input.MsgArgs, tid),
			&ret.writes,
			addr,
			input.RateLimiting)
		if err != nil {
			sendErr <- err
		}
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
	if ret.Err != nil {
		for _, n := range s.table.addrNodes(addr) {
			n.consecutiveFailures++
		}
	}
	s.mu.Unlock()
	return
}

func (s *Server) transactionQuerySender(
	sendCtx context.Context, b []byte, writes *numWrites, addr Addr, rateLimiting QueryRateLimiting,
) error {
	err := transactionSender(
		sendCtx,
		func() error {
			wrote, err := s.writeToNode(sendCtx, b, addr,
				// We only wait for the first write if rate-limiting is enabled for this query.
				// Retries will defer to other queries.
				*writes == 0,
				!rateLimiting.NotAny && !(rateLimiting.NotFirst && *writes == 0))
			if wrote {
				*writes++
			}
			return err
		},
		s.resendDelay,
		maxTransactionSends,
	)
	if err != nil {
		return err
	}
	select {
	case <-sendCtx.Done():
		return sendCtx.Err()
	case <-time.After(s.resendDelay()):
		return errors.New("timed out")
	}

}

// Sends a ping query to the address given.
func (s *Server) Ping(node *net.UDPAddr, callback func(krpc.Msg, error)) error {
	return s.ping(node, callback)
}

// This method is old, and probably needs a new signature.
func (s *Server) ping(node *net.UDPAddr, callback func(krpc.Msg, error)) error {
	return s.query(NewAddr(node), "ping", krpc.MsgArgs{}, callback)
}

func (s *Server) announcePeer(node Addr, infoHash int160, port int, token string, impliedPort bool, rl QueryRateLimiting) (ret QueryResult) {
	if port == 0 && !impliedPort {
		ret.Err = errors.New("no port specified")
		return
	}
	ret = s.Query(
		context.TODO(), node, "announce_peer",
		QueryInput{
			MsgArgs: krpc.MsgArgs{
				ImpliedPort: impliedPort,
				InfoHash:    infoHash.AsByteArray(),
				Port:        &port,
				Token:       token,
			},
			RateLimiting: rl})
	if ret.Err != nil {
		return
	}
	if ret.Err = ret.Reply.Error(); ret.Err != nil {
		announceErrors.Add(1)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.SuccessfulOutboundAnnouncePeerQueries++
	return
}

// Add response nodes to node table.
func (s *Server) addResponseNodes(d krpc.Msg) {
	if d.R == nil {
		return
	}
	d.R.ForAllNodes(func(ni krpc.NodeInfo) {
		s.updateNode(NewAddr(ni.Addr.UDP()), (*krpc.ID)(&ni.ID), true, func(*node) {})
	})
}

// Sends a find_node query to addr. targetID is the node we're looking for. The Server makes use of
// some of the response fields.
func (s *Server) FindNode(addr Addr, targetID int160, rl QueryRateLimiting) (ret QueryResult) {
	ret = s.Query(context.TODO(), addr, "find_node", QueryInput{
		MsgArgs: krpc.MsgArgs{
			Target: targetID.AsByteArray(),
			Want:   []krpc.Want{krpc.WantNodes, krpc.WantNodes6},
		},
		RateLimiting: rl})
	// Scrape peers from the response to put in the server's table before
	// handing the response back to the caller.
	s.mu.Lock()
	s.addResponseNodes(ret.Reply)
	s.mu.Unlock()
	return
}

// Returns how many nodes are in the node table.
func (s *Server) NumNodes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.numNodes()
}

// Exports the current node table.
func (s *Server) Nodes() (nis []krpc.NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.table.forNodes(func(n *node) bool {
		nis = append(nis, krpc.NodeInfo{
			Addr: n.addr.KRPC(),
			ID:   n.id.AsByteArray(),
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
	s.socket.Close()
}

func (s *Server) GetPeers(ctx context.Context, addr Addr, infoHash int160, scrape bool, rl QueryRateLimiting) (ret QueryResult) {
	args := krpc.MsgArgs{
		InfoHash: infoHash.AsByteArray(),
		// TODO: Maybe IPv4-only Servers won't want IPv6 nodes?
		Want: []krpc.Want{krpc.WantNodes, krpc.WantNodes6},
	}
	if scrape {
		args.Scrape = 1
	}
	ret = s.Query(ctx, addr, "get_peers", QueryInput{
		MsgArgs:      args,
		RateLimiting: rl,
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	m := ret.Reply
	s.addResponseNodes(m)
	if m.R != nil {
		if m.R.Token == nil {
			expvars.Add("get_peers responses with no token", 1)
		} else if len(*m.R.Token) == 0 {
			expvars.Add("get_peers responses with empty token", 1)
		} else {
			expvars.Add("get_peers responses with token", 1)
		}
		if m.R.Token != nil {
			s.updateNode(addr, m.SenderID(), false, func(n *node) {
				n.announceToken = m.R.Token
			})
		}
	}
	return
}

func (s *Server) closestGoodNodeInfos(
	k int,
	targetID int160,
	filter func(krpc.NodeAddr) bool,
) (
	ret []krpc.NodeInfo,
) {
	for _, n := range s.closestNodes(k, targetID, func(n *node) bool {
		return n.IsGood() && filter(n.NodeInfo().Addr)
	}) {
		ret = append(ret, n.NodeInfo())
	}
	return
}

func (s *Server) closestNodes(k int, target int160, filter func(*node) bool) []*node {
	return s.table.closestNodes(k, target, filter)
}

func (s *Server) traversalStartingNodes() (nodes []addrMaybeId, err error) {
	s.mu.RLock()
	s.table.forNodes(func(n *node) bool {
		nodes = append(nodes, addrMaybeId{n.addr.KRPC(), &n.id})
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
		s.logger().WithValues(log.Warning).Printf("falling back on starting nodes")
		addrs, err := s.config.StartingNodes()
		if err != nil {
			return nil, errors.Wrap(err, "getting starting nodes")
		} else {
			//log.Printf("resolved %v addresses", len(addrs))
		}
		for _, a := range addrs {
			nodes = append(nodes, addrMaybeId{a.KRPC(), nil})
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

func (s *Server) getQuestionableNode() (ret *node) {
	s.table.forNodes(func(n *node) bool {
		if n.IsQuestionable() {
			ret = n
			return false
		}
		return true
	})
	return
}

func (s *Server) questionableNodePinger() {
tryPing:
	for {
		s.mu.RLock()
		n := s.getQuestionableNode()
		if n != nil {
			addr := n.addr.Raw().(*net.UDPAddr)
			s.mu.RUnlock()
			done := make(chan struct{})
			err := s.Ping(addr, func(krpc.Msg, error) {
				close(done)
			})
			if err == nil {
				<-done
				goto tryPing
			}
			s.logger().Printf("error pinging questionable node: %v", err)
		} else {
			s.mu.RUnlock()
		}
		select {
		case <-time.After(time.Second):
		case <-s.closed.LockedChan(&s.mu):
		}
	}
}
