package dht

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/logonce"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/tylertreat/BoomFilters"

	"github.com/anacrolix/dht/krpc"
)

const (
	maxInterval  = time.Minute * 3
	stepInterval = time.Second * 30
)

// A Server defines parameters for a DHT node server that is able to send
// queries, and respond to the ones from the network. Each node has a globally
// unique identifier known as the "node ID." Node IDs are chosen at random
// from the same 160-bit space as BitTorrent infohashes and define the
// behaviour of the node. Zero valued Server does not have a valid ID and thus
// is unable to function properly. Use `NewServer(nil)` to initialize a
// default node.
type Server struct {
	id               int160
	socket           net.PacketConn
	transactions     map[transactionKey]*Transaction
	transactionIDInt uint64
	table            table
	mu               sync.Mutex
	closed           missinggo.Event
	ipBlockList      iplist.Ranger
	badNodes         *boom.BloomFilter
	tokenServer      tokenServer

	numConfirmedAnnounces int
	config                ServerConfig
}

func (s *Server) numGoodNodes() (num int) {
	s.table.forNodes(func(n *node) bool {
		if n.DefinitelyGood() {
			num++
		}
		return true
	})
	return
}

func (s *Server) numNodes() (num int) {
	s.table.forNodes(func(n *node) bool {
		num++
		return true
	})
	return
}

// Stats returns statistics for the server.
func (s *Server) Stats() (ss ServerStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss.GoodNodes = s.numGoodNodes()
	ss.Nodes = s.numNodes()
	ss.OutstandingTransactions = len(s.transactions)
	ss.ConfirmedAnnounces = s.numConfirmedAnnounces
	ss.BadNodes = s.badNodes.Count()
	return
}

// Addr returns the listen address for the server. Packets arriving to this address
// are processed by the server (unless aliens are involved).
func (s *Server) Addr() net.Addr {
	return s.socket.LocalAddr()
}

// NewServer initializes a new DHT node server.
func NewServer(c *ServerConfig) (s *Server, err error) {
	if c == nil {
		c = &ServerConfig{
			Conn:       mustListen(":0"),
			NoSecurity: true,
		}
	}
	if missinggo.IsZeroValue(c.NodeId) {
		c.NodeId = RandomNodeID()
		if !c.NoSecurity && c.PublicIP != nil {
			SecureNodeId(&c.NodeId, c.PublicIP)
		}
	}
	s = &Server{
		config:      *c,
		ipBlockList: c.IPBlocklist,
		badNodes:    boom.NewBloomFilter(1000, 0.1),
		tokenServer: tokenServer{
			maxIntervalDelta: 2,
			interval:         5 * time.Minute,
			secret:           make([]byte, 20),
		},
		transactions: make(map[transactionKey]*Transaction),
	}
	rand.Read(s.tokenServer.secret)
	s.socket = c.Conn
	s.id = int160FromByteArray(c.NodeId)
	s.table.rootID = s.id
	go func() {
		err := s.serve()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed.IsSet() {
			return
		}
		if err != nil {
			panic(err)
		}
	}()
	return
}

// Returns a description of the Server. Python repr-style.
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
	if len(b) < 2 || b[0] != 'd' || b[len(b)-1] != 'e' {
		// KRPC messages are bencoded dicts.
		readNotKRPCDict.Add(1)
		return
	}
	var d krpc.Msg
	err := bencode.Unmarshal(b, &d)
	if err != nil {
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
	if sid := d.SenderID(); sid != nil && *sid == s.id.AsByteArray() {
		// Ignore messages that could be from us. This would trigger very
		// undesirable circumstances.
		return
	}
	if d.Y == "q" {
		readQuery.Add(1)
		s.handleQuery(addr, d)
		return
	}
	t := s.findResponseTransaction(d.T, addr)
	if t == nil {
		//log.Printf("unexpected message: %#v", d)
		return
	}
	node := s.getNode(addr, int160FromByteArray(*d.SenderID()))
	node.lastGotResponse = time.Now()
	// TODO: Update node ID as this is an authoritative packet.
	go t.handleResponse(d)
	s.deleteTransaction(t)
}

func (s *Server) serve() error {
	var b [0x10000]byte
	for {
		n, addr, err := s.socket.ReadFrom(b[:])
		if err != nil {
			return err
		}
		read.Add(1)
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
		s.processPacket(b[:n], NewAddr(addr.(*net.UDPAddr)))
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
	if s.getNode(NewAddr(ni.Addr), int160FromByteArray(ni.ID)).DefinitelyGood() {
		// We've already got it, and there's no point pinging it.
		return nil
	}
	return s.Ping(ni.Addr, nil)
}

// TODO: Probably should write error messages back to senders if something is
// wrong.
func (s *Server) handleQuery(source Addr, m krpc.Msg) {
	node := s.getNode(source, int160FromByteArray(*m.SenderID()))
	node.lastGotQuery = time.Now()
	if s.config.OnQuery != nil {
		propagate := s.config.OnQuery(&m, source.UDPAddr())
		if !propagate {
			return
		}
	}
	// Don't respond.
	if s.config.Passive {
		return
	}
	args := m.A
	switch m.Q {
	case "ping":
		s.reply(source, m.T, krpc.Return{})
	case "get_peers":
		if len(args.InfoHash) != 20 {
			break
		}
		s.reply(source, m.T, krpc.Return{
			Nodes: s.closestGoodNodeInfos(8, int160FromByteArray(args.InfoHash)),
			Token: s.createToken(source),
		})
	case "find_node": // TODO: Extract common behaviour with get_peers.
		if len(args.Target) != 20 {
			log.Printf("bad DHT query: %v", m)
			return
		}
		s.reply(source, m.T, krpc.Return{
			Nodes: s.closestGoodNodeInfos(8, int160FromByteArray(args.Target)),
		})
	case "announce_peer":
		readAnnouncePeer.Add(1)
		if !s.validToken(args.Token, source) {
			readInvalidToken.Add(1)
			return
		}
		if len(args.InfoHash) != 20 {
			readQueryBad.Add(1)
			return
		}
		if h := s.config.OnAnnouncePeer; h != nil {
			p := Peer{
				IP:   source.UDPAddr().IP,
				Port: args.Port,
			}
			if args.ImpliedPort {
				p.Port = source.UDPAddr().Port
			}
			go h(metainfo.Hash(args.InfoHash), p)
		}
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
	err = s.writeToNode(b, addr)
	if err != nil {
		log.Printf("error replying to %s: %s", addr, err)
	}
}

func (s *Server) reply(addr Addr, t string, r krpc.Return) {
	r.ID = s.id.AsByteArray()
	m := krpc.Msg{
		T: t,
		Y: "r",
		R: &r,
	}
	b, err := bencode.Marshal(m)
	if err != nil {
		panic(err)
	}
	err = s.writeToNode(b, addr)
	if err != nil {
		log.Printf("error replying to %s: %s", addr, err)
	}
}

// Returns a node struct, from the routing table if present.
func (s *Server) getNode(addr Addr, id int160) *node {
	if id != s.id {
		if n := s.table.getNode(addr, id); n != nil {
			return n
		}
	}
	return &node{
		addr: addr,
		id:   id,
	}
}

func (s *Server) addNode(n *node) {
	// if len(s.nodes) >= maxNodes {
	// 	return
	// }
	// // Exclude insecure nodes from the node table.
	// if !s.config.NoSecurity && !n.IsSecure() {
	// 	return
	// }
	// if s.badNodes.Test([]byte(addrStr)) {
	// 	return
	// }
	// s.nodes[addrStr] = n
}

func (s *Server) nodeTimedOut(addr Addr) {
	// node, ok := s.nodes[addr.String()]
	// if !ok {
	// 	return
	// }
	// if node.DefinitelyGood() {
	// 	return
	// }
	// if len(s.nodes) < maxNodes {
	// 	return
	// }
	// delete(s.nodes, addr.String())
}

func (s *Server) writeToNode(b []byte, node Addr) (err error) {
	if list := s.ipBlockList; list != nil {
		if r, ok := list.Lookup(missinggo.AddrIP(node.UDPAddr())); ok {
			err = fmt.Errorf("write to %s blocked: %s", node, r.Description)
			return
		}
	}
	log.Printf("writing to %s: %q", node.UDPAddr(), b)
	n, err := s.socket.WriteTo(b, node.UDPAddr())
	writes.Add(1)
	if err != nil {
		writeErrors.Add(1)
		err = fmt.Errorf("error writing %d bytes to %s: %s", len(b), node, err)
		return
	}
	if n != len(b) {
		err = io.ErrShortWrite
		return
	}
	return
}

func (s *Server) findResponseTransaction(transactionID string, sourceNode Addr) *Transaction {
	return s.transactions[transactionKey{
		sourceNode.String(),
		transactionID}]
}

func (s *Server) nextTransactionID() string {
	var b [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(b[:], s.transactionIDInt)
	s.transactionIDInt++
	return string(b[:n])
}

func (s *Server) deleteTransaction(t *Transaction) {
	delete(s.transactions, t.key())
}

func (s *Server) deleteTransactionUnlocked(t *Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteTransaction(t)
}

func (s *Server) addTransaction(t *Transaction) {
	if _, ok := s.transactions[t.key()]; ok {
		panic("transaction not unique")
	}
	s.transactions[t.key()] = t
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

func (s *Server) query(node Addr, q string, a *krpc.MsgArgs, callback func(krpc.Msg, error)) error {
	tid := s.nextTransactionID()
	if a == nil {
		a = &krpc.MsgArgs{}
	}
	a.ID = s.ID()
	m := krpc.Msg{
		T: tid,
		Y: "q",
		Q: q,
		A: a,
	}
	// BEP 43. Outgoing queries from passive nodes should contain "ro":1 in
	// the top level dictionary.
	if s.config.Passive {
		m.ReadOnly = true
	}
	b, err := bencode.Marshal(m)
	if err != nil {
		return err
	}
	var t *Transaction
	t = &Transaction{
		remoteAddr: node,
		t:          tid,
		querySender: func() error {
			return s.writeToNode(b, node)
		},
		onResponse: func(m krpc.Msg) {
			go callback(m, nil)
			go s.deleteTransactionUnlocked(t)
		},
		onTimeout: func() {
			go s.deleteTransactionUnlocked(t)
			go callback(krpc.Msg{}, errors.New("query timed out"))
		},
	}
	err = t.sendQuery()
	if err != nil {
		return err
	}
	// s.getNode(node, "").lastSentQuery = time.Now()
	t.mu.Lock()
	t.startResendTimer()
	t.mu.Unlock()
	s.addTransaction(t)
	return nil
}

// Sends a ping query to the address given.
func (s *Server) Ping(node *net.UDPAddr, callback func(krpc.Msg, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query(NewAddr(node), "ping", nil, callback)
}

func (s *Server) announcePeer(node Addr, infoHash int160, port int, token string, impliedPort bool) (err error) {
	if port == 0 && !impliedPort {
		return errors.New("nothing to announce")
	}
	s.query(node, "announce_peer", &krpc.MsgArgs{
		ImpliedPort: impliedPort,
		InfoHash:    infoHash.AsByteArray(),
		Port:        port,
		Token:       token,
	}, func(m krpc.Msg, err error) {
		if err != nil {
			log.Printf("error announcing peer: %s", err)
			return
		}
		if err := m.Error(); err != nil {
			announceErrors.Add(1)
			// log.Print(token)
			// logonce.Stderr.Printf("announce_peer response: %s", err)
			return
		}
		s.numConfirmedAnnounces++
	})
	return
}

// Add response nodes to node table.
func (s *Server) liftNodes(d krpc.Msg) {
	if d.Y != "r" {
		return
	}
	for _, cni := range d.R.Nodes {
		if cni.Addr.Port == 0 {
			// TODO: Why would people even do this?
			continue
		}
		n := s.getNode(NewAddr(cni.Addr), int160FromByteArray(cni.ID))
		s.addNode(n)
	}
}

// Sends a find_node query to addr. targetID is the node we're looking for.
func (s *Server) findNode(addr Addr, targetID int160, onResponse func(krpc.CompactIPv4NodeInfo, error)) (err error) {
	return s.query(addr, "find_node", &krpc.MsgArgs{Target: targetID.AsByteArray()}, func(d krpc.Msg, err error) {
		// Scrape peers from the response to put in the server's table before
		// handing the response back to the caller.
		s.liftNodes(d)
		onResponse(d.R.Nodes, err)
	})
}

// Populates the node table.
func (s *Server) Bootstrap() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	initialAddrs, err := s.traversalStartingAddrs()
	if err != nil {
		return
	}
	var outstanding sync.WaitGroup
	triedAddrs := newBloomFilterForTraversal()
	var onAddr func(addr Addr)
	onAddr = func(addr Addr) {
		if triedAddrs.Test([]byte(addr.String())) {
			return
		}
		outstanding.Add(1)
		s.findNode(addr, s.id, func(addrs krpc.CompactIPv4NodeInfo, err error) {
			defer outstanding.Done()
			if err != nil {
				log.Printf("error in find node to %q: %s", addr, err)
				return
			}
			for _, addr := range addrs {
				onAddr(NewAddr(addr.Addr))
			}
		})
	}
	for _, addr := range initialAddrs {
		onAddr(addr)
	}
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
			Addr: n.addr.UDPAddr(),
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

func (s *Server) getPeers(addr Addr, infoHash int160, onResponse func(m krpc.Msg)) (err error) {
	return s.query(addr, "get_peers", &krpc.MsgArgs{InfoHash: infoHash.AsByteArray()}, func(m krpc.Msg, err error) {
		s.liftNodes(m)
		if m.R != nil && m.R.Token != "" {
			s.getNode(addr, int160FromByteArray(*m.SenderID())).announceToken = m.R.Token
		}
		onResponse(m)
	})
}

func (s *Server) closestGoodNodeInfos(k int, targetID int160) (ret []krpc.NodeInfo) {
	for _, n := range s.closestNodes(k, targetID, func(n *node) bool { return n.DefinitelyGood() }) {
		ret = append(ret, n.NodeInfo())
	}
	return
}

func (s *Server) closestGoodNodes(k int, targetID int160) []*node {
	return s.closestNodes(k, targetID, func(n *node) bool { return n.DefinitelyGood() })
}

func (s *Server) closestNodes(k int, target int160, filter func(*node) bool) []*node {
	return s.table.closestNodes(k, target, filter)
}

func (s *Server) traversalStartingAddrs() (addrs []Addr, err error) {
	s.table.forNodes(func(n *node) bool {
		addrs = append(addrs, n.addr)
		return true
	})
	if len(addrs) > 0 {
		return
	}
	addrs = s.config.StartingNodes
	if len(addrs) == 0 {
		err = errors.New("no initial nodes")
	}
	return
}
