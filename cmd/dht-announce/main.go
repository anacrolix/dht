package main

import (
	"net"
	"os"
	"sync"

	"github.com/anacrolix/log"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/tagflag"

	"github.com/anacrolix/dht/v2"
)

func main() {
	code := mainErr()
	if code != 0 {
		os.Exit(code)
	}
}

func mainErr() int {
	var flags = struct {
		Port  int
		Debug bool
		tagflag.StartPos
		Infohash [][20]byte
	}{}
	tagflag.Parse(&flags)
	s, err := dht.NewServer(func() *dht.ServerConfig {
		sc := dht.NewDefaultServerConfig()
		if flags.Debug {
			sc.Logger = log.Default()
		}
		return sc
	}())
	if err != nil {
		log.Printf("error creating server: %s", err)
		return 1
	}
	defer s.Close()
	wg := sync.WaitGroup{}
	addrs := make(map[[20]byte]map[string]struct{}, len(flags.Infohash))
	for _, ih := range flags.Infohash {
		a, err := s.Announce(ih, flags.Port, false)
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
			log.Printf("%v contacted %v nodes", a, a.NumContacted())
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
	return 0
}
