// Pings DHT nodes with the given network addresses.
package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/anacrolix/dht/v2"
)

type pingArgs struct {
	Network  string
	Timeout  time.Duration `help:"sets a timeout for all queries"`
	Defaults bool          `help:"include all the default bootstrap nodes"`
	Nodes    []string      `arg:"positional" arity:"*" help:"nodes to ping e.g. router.bittorrent.com:6881"`
}

func ping(args pingArgs, s *dht.Server) error {
	nodes := args.Nodes
	if args.Defaults {
		nodes = append(nodes, dht.DefaultGlobalBootstrapHostPorts...)
	}
	var wg sync.WaitGroup
	for _, a := range nodes {
		ua, err := net.ResolveUDPAddr(args.Network, a)
		if err != nil {
			return err
		}
		started := time.Now()
		wg.Go(func() {
			res := s.Ping(ua)
			if res.Err != nil {
				fmt.Printf("%s: %s: %s\n", a, time.Since(started), res.Err)
				return
			}
			id := *res.Reply.SenderID()
			secure := '✘'
			if dht.NodeIdSecure(id, ua.IP) {
				secure = '✔'
			}
			fmt.Printf("%s: %x %c: %s\n", a, id, secure, time.Since(started))
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	var timeout <-chan time.Time
	if args.Timeout != 0 {
		timeout = time.After(args.Timeout)
	}
	select {
	case <-done:
		return nil
	case <-timeout:
		return errors.New("timed out")
	}
}
