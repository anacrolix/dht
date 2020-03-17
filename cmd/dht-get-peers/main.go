package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	_ "github.com/anacrolix/envpprof"

	"github.com/anacrolix/dht/v2"
)

var (
	tableFileName = flag.String("tableFile", "", "name of file for storing node info")
	serveAddr     = flag.String("serveAddr", ":0", "local UDP address")
	infoHash      = flag.String("infoHash", "", "torrent infohash")
	once          = flag.Bool("once", true, "only do one scrape iteration")

	s        *dht.Server
	quitting = make(chan struct{})
)

func saveTable() error {
	if *tableFileName == "" {
		return nil
	}
	return dht.WriteNodesToFile(s.Nodes(), *tableFileName)
}

func setupSignals() {
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		close(quitting)
	}()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	switch len(*infoHash) {
	case 20:
	case 40:
		_, err := fmt.Sscanf(*infoHash, "%x", infoHash)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("require 20 byte infohash")
	}
	conn, err := net.ListenPacket("udp", *serveAddr)
	if err != nil {
		log.Fatal(err)
	}
	sc := dht.ServerConfig{
		Conn: conn,
	}
	ns, err := dht.ReadNodesFromFile(*tableFileName)
	if os.IsNotExist(err) {
		sc.StartingNodes = func() ([]dht.Addr, error) { return dht.GlobalBootstrapAddrs("udp") }
	}
	s, err = dht.NewServer(&sc)
	if err != nil {
		log.Fatal(err)
	}
	for _, n := range ns {
		s.AddNode(n)
	}
	log.Printf("dht server on %s, ID is %x", s.Addr(), s.ID())
	setupSignals()

	seen := make(map[string]struct{})
getPeers:
	for {
		var ih [20]byte
		copy(ih[:], *infoHash)
		ps, err := s.Announce(ih, 0, false)
		if err != nil {
			log.Fatal(err)
		}
	values:
		for {
			select {
			case v, ok := <-ps.Peers:
				if !ok {
					break values
				}
				log.Printf("received %d peers from %x", len(v.Peers), v.NodeInfo.ID)
				for _, p := range v.Peers {
					if _, ok := seen[p.String()]; ok {
						continue
					}
					seen[p.String()] = struct{}{}
					fmt.Println((&net.UDPAddr{
						IP:   p.IP[:],
						Port: p.Port,
					}).String())
				}
			case <-quitting:
				break getPeers
			}
		}
		if *once {
			break
		}
	}
	if err := saveTable(); err != nil {
		log.Printf("error saving node table: %s", err)
	}
}
