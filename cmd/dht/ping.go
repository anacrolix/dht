// Pings DHT nodes with the given network addresses.
package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
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

func resolveUDPAddr(ctx context.Context, network, address string) (*net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(host); err == nil || host == "" {
		// The standard resolver does not perform DNS for literals. Keep its zone,
		// family, service-port and wildcard-address handling intact.
		return net.ResolveUDPAddr(network, address)
	}
	ipNetwork := "ip"
	switch network {
	case "", "udp":
	case "udp4":
		ipNetwork = "ip4"
	case "udp6":
		ipNetwork = "ip6"
	default:
		return nil, net.UnknownNetworkError(network)
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, ipNetwork, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses for %q", host)
	}
	ip := ips[0]
	if ipNetwork == "ip" {
		for _, candidate := range ips {
			if candidate.Is4() {
				ip = candidate
				break
			}
		}
	}
	return net.ResolveUDPAddr(network, net.JoinHostPort(ip.String(), port))
}

func ping(ctx context.Context, args pingArgs, s *dht.Server) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if args.Timeout != 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, args.Timeout)
		defer cancelTimeout()
	}
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
		ua, err := resolveUDPAddr(ctx, args.Network, a)
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
			if err := res.ToError(); err != nil {
				fmt.Printf("%s: %s: %s\n", a, time.Since(started), err)
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
	}
}
