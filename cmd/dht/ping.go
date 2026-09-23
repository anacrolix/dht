// Pings DHT nodes with the given network addresses.
package main

import (
	"context"
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

func ping(ctx context.Context, args pingArgs, s *dht.Server) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	nodes := args.Nodes
	if args.Defaults {
		nodes = append(nodes, dht.DefaultGlobalBootstrapHostPorts...)
	}
	var wg sync.WaitGroup
	for _, a := range nodes {
		if err := ctx.Err(); err != nil {
			cancel()
			wg.Wait()
			return err
		}
		ua, err := net.ResolveUDPAddr(args.Network, a)
		if err != nil {
			cancel()
			wg.Wait()
			return err
		}
		if err := ctx.Err(); err != nil {
			cancel()
			wg.Wait()
			return err
		}
		started := time.Now()
		wg.Go(func() {
			addr := dht.NewAddr(ua)
			res := s.Query(ctx, addr, "ping", dht.QueryInput{})
			if res.Err != nil {
				fmt.Printf("%s: %s: %s\n", a, time.Since(started), res.Err)
				return
			}
			id := res.Reply.SenderID()
			if id == nil {
				fmt.Printf("%s: response has no id: %s\n", a, time.Since(started))
				return
			}
			s.NodeRespondedToPing(addr, id.Int160())
			secure := '✘'
			if dht.NodeIdSecure(*id, ua.IP) {
				secure = '✔'
			}
			fmt.Printf("%s: %x %c: %s\n", a, *id, secure, time.Since(started))
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	var timeout <-chan time.Time
	if args.Timeout != 0 {
		timer := time.NewTimer(args.Timeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-done:
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		cancel()
		<-done
		return ctx.Err()
	case <-timeout:
		cancel()
		<-done
		return errors.New("timed out")
	}
}
