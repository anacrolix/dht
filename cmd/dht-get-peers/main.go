package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	_ "github.com/anacrolix/envpprof"

	"github.com/anacrolix/dht"
)

var (
	tableFileName = flag.String("tableFile", "", "name of file for storing node info")
	serveAddr     = flag.String("serveAddr", ":0", "local UDP address")
	infoHash      = flag.String("infoHash", "", "torrent infohash")
	once          = flag.Bool("once", false, "only do one scrape iteration")

	s        *dht.Server
	quitting = make(chan struct{})
)

func loadTable() error {
	ns, err := dht.ReadNodesFromFile(*tableFileName)
	if err != nil {
		return err
	}
	for _, ni := range ns {
		s.AddNode(ni)
	}
	return nil
}

func init() {
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
	s, err = dht.NewServer(&dht.ServerConfig{
		Conn: conn,
	})
	if err != nil {
		log.Fatal(err)
	}
	err = loadTable()
	if err != nil {
		log.Fatalf("error loading table: %s", err)
	}
	log.Printf("dht server on %s, ID is %x", s.Addr(), s.ID())
	setupSignals()
}

func saveTable() error {
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
						Port: int(p.Port),
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
