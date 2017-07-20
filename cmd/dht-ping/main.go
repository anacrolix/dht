// Pings DHT nodes with the given network addresses.
package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/dht/krpc"
	"github.com/anacrolix/tagflag"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var args = struct {
		Timeout time.Duration
		tagflag.StartPos
		Nodes []string `help:"nodes to ping e.g. router.bittorrent.com:6881"`
	}{}
	tagflag.Parse(&args)
	s, err := dht.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dht server on %s with id %x", s.Addr(), s.ID())
	var wg sync.WaitGroup
	for _, a := range args.Nodes {
		func(a string) {
			ua, err := net.ResolveUDPAddr("udp", a)
			if err != nil {
				log.Fatal(err)
			}
			started := time.Now()
			wg.Add(1)
			s.Ping(ua, func(m krpc.Msg, err error) {
				defer wg.Done()
				if err != nil {
					fmt.Printf("%s: %s: %s\n", a, time.Since(started), err)
					return
				}
				id := *m.SenderID()
				fmt.Printf("%s: %x %c: %s\n", a, id, func() rune {
					if dht.NodeIdSecure(id, ua.IP) {
						return '✔'
					} else {
						return '✘'
					}
				}(), time.Since(started))
			})
		}(a)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timeout := make(chan struct{})
	if args.Timeout != 0 {
		go func() {
			time.Sleep(args.Timeout)
			close(timeout)
		}()
	}
	select {
	case <-done:
	case <-timeout:
		log.Print("timed out")
	}
}
