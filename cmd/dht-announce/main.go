package main

import (
	"log"
	"net"
	"sync"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/tagflag"
)

func main() {
	var flags = struct {
		tagflag.StartPos
		Infohash [][20]byte
	}{}
	tagflag.Parse(&flags)
	s, err := dht.NewServer(nil)
	if err != nil {
		log.Fatalf("error creating server: %s", err)
	}
	defer s.Close()
	wg := sync.WaitGroup{}
	addrs := make(map[[20]byte]map[string]struct{}, len(flags.Infohash))
	for _, ih := range flags.Infohash {
		a, err := s.Announce(ih, 0, true)
		if err != nil {
			log.Printf("error announcing %s: %s", ih, err)
			continue
		}
		wg.Add(1)
		addrs[ih] = make(map[string]struct{})
		go func(ih [20]byte) {
			defer wg.Done()
			for ps := range a.Peers {
				for _, p := range ps.Peers {
					s := p.String()
					if _, ok := addrs[ih][s]; !ok {
						log.Printf("got peer %s for %x from %s", p, ih, ps.NodeInfo)
						addrs[ih][s] = struct{}{}
					}
				}
			}
		}(ih)
	}
	wg.Wait()
	for _, ih := range flags.Infohash {
		ips := make(map[string]struct{}, len(addrs[ih]))
		for s := range addrs[ih] {
			ip, _, err := net.SplitHostPort(s)
			if err != nil {
				log.Printf("error parsing addr: %s", err)
			}
			ips[ip] = struct{}{}
		}
		log.Printf("%x: %d addrs %d distinct ips", ih, len(addrs[ih]), len(ips))
	}
}
