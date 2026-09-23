package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/anacrolix/args"
	"github.com/anacrolix/args/targets"
	"github.com/anacrolix/log"
	"github.com/anacrolix/publicip"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/types/infohash"
	"github.com/davecgh/go-spew/spew"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
)

type serverParams struct {
	Network          string
	Secure           bool
	BootstrapAddr    []string
	QueryResendDelay time.Duration
}

func main() {
	if code := runMain(); code != 0 {
		os.Exit(code)
	}
}

func runMain() int {
	logger := log.Default.WithNames("main")
	ctx := log.ContextWithLogger(context.Background(), logger)
	ctx, stopSignalNotify := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignalNotify()
	var s *dht.Server
	serverArgs := serverParams{
		Network: "udp",
		Secure:  false,
	}
	params := append(
		args.FromStruct(&serverArgs),
		args.Subcommand("derive-put-target", func(sub args.SubCmdCtx) (err error) {
			var put bep44.Put
			if !sub.Parse(
				args.Subcommand("mutable", func(sub args.SubCmdCtx) (err error) {
					var subArgs struct {
						Private bool
						// We want "required" but it's not supported I think.
						Key  targets.Hex `arity:"+"`
						Salt string
					}
					sub.Parse(args.FromStruct(&subArgs)...)
					put.Salt = []byte(subArgs.Salt)
					put.K = (*[32]byte)(subArgs.Key.Bytes)
					if subArgs.Private {
						privKey := ed25519.NewKeyFromSeed(subArgs.Key.Bytes)
						pubKey := privKey.Public().(ed25519.PublicKey)
						log.Printf("public key: %x", pubKey)
						put.K = (*[32]byte)(pubKey)
					}
					return nil
				}),
				args.Subcommand("immutable", func(sub args.SubCmdCtx) (err error) {
					var subArgs struct {
						String bool
						Value  string `arg:"positional"`
					}
					sub.Parse(args.FromStruct(&subArgs)...)
					if subArgs.String {
						put.V = subArgs.Value
					} else {
						err = bencode.Unmarshal([]byte(subArgs.Value), &put.V)
						if err != nil {
							err = fmt.Errorf("parsing value bencode: %w", err)
						}
					}
					return
				}),
			).RanSubCmd {
				return errors.New("expected subcommand")
			}
			fmt.Printf("%x\n", put.Target())
			return nil
		}),
		args.Subcommand("put", func(sub args.SubCmdCtx) (err error) {
			var putOpt PutCmd
			sub.Parse(args.FromStruct(&putOpt)...)
			switch len(putOpt.Key.Bytes) {
			case 0, 32:
			default:
				return fmt.Errorf("key has bad length %v", len(putOpt.Key.Bytes))
			}
			sub.Defer(func() error { return put(ctx, &putOpt) })
			return nil
		}),
		args.Subcommand("put-mutable-infohash", func(sub args.SubCmdCtx) (err error) {
			var putOpt PutMutableInfohash
			var ih infohash.T
			sub.Parse(append(
				args.FromStruct(&putOpt),
				args.Opt(args.OptOpt{
					Long:     "info hash",
					Target:   &ih,
					Required: true,
				}))...)
			sub.Defer(func() error { return putMutableInfohash(ctx, &putOpt, ih) })
			return nil
		}),
		args.Subcommand("get", func(sub args.SubCmdCtx) error {
			var getOpt GetCmd
			sub.Parse(args.FromStruct(&getOpt)...)
			sub.Defer(func() error { return get(ctx, &getOpt) })
			return nil
		}),
		args.Subcommand("ping", func(sub args.SubCmdCtx) (err error) {
			var pa pingArgs
			pa.Network = serverArgs.Network
			sub.Parse(args.FromStruct(&pa)...)
			sub.Defer(func() error {
				return ping(ctx, pa, s)
			})
			return nil
		}),
		args.Subcommand("get-peers", func(sub args.SubCmdCtx) (err error) {
			var subArgs = struct {
				AnnouncePort int
				Scrape       bool
				InfoHash     infohash.T `arity:"+"`
			}{}
			sub.Parse(args.FromStruct(&subArgs)...)
			var announceOpts []dht.AnnounceOpt
			if subArgs.AnnouncePort != 0 {
				announceOpts = append(announceOpts, dht.AnnouncePeer(dht.AnnouncePeerOpts{
					Port: subArgs.AnnouncePort,
				}))
			}
			if subArgs.Scrape {
				announceOpts = append(announceOpts, dht.Scrape())
			}
			sub.Defer(func() error {
				return GetPeers(ctx, s, subArgs.InfoHash, announceOpts...)
			})
			return nil
		}),
		args.Subcommand("query", func(sub args.SubCmdCtx) (err error) {
			var subArgs = struct {
				Addr   string `arg:"positional"`
				Q      string `arg:"positional"`
				Target krpc.ID
			}{}
			sub.Parse(args.FromStruct(&subArgs)...)
			sub.Defer(func() error {
				addr, err := net.ResolveUDPAddr(serverArgs.Network, subArgs.Addr)
				if err != nil {
					return err
				}
				input := dht.QueryInput{}
				input.MsgArgs.Target = subArgs.Target
				res := s.Query(ctx, dht.NewAddr(addr), subArgs.Q, input)
				spew.Dump(res)
				return nil
			})
			return nil
		}),
		args.Subcommand("ping-nodes", func(sub args.SubCmdCtx) (err error) {
			var subArgs = struct {
				Timeout time.Duration
				Addr    string `arg:"positional"`
				Q       string `arg:"positional"`
				Target  krpc.ID
			}{}
			sub.Parse(args.FromStruct(&subArgs)...)
			sub.Defer(func() error {
				addr, err := net.ResolveUDPAddr(serverArgs.Network, subArgs.Addr)
				if err != nil {
					return err
				}
				input := dht.QueryInput{}
				input.MsgArgs.Target = subArgs.Target
				res := s.Query(ctx, dht.NewAddr(addr), subArgs.Q, input)
				err = res.ToError()
				if err != nil {
					return err
				}
				return ping(ctx, pingArgs{
					Network: serverArgs.Network,
					Nodes: func() (ret []string) {
						res.Reply.R.ForAllNodes(func(info krpc.NodeInfo) {
							ret = append(ret, info.Addr.String())
						})
						return
					}(),
					Timeout: subArgs.Timeout,
				}, s)
			})
			return nil
		}),
	)
	parsed := args.Parse(os.Args[1:], params...)
	if errors.Is(parsed.Err, args.ErrHelped) {
		return 0
	}
	if parsed.Err != nil {
		log.Printf("error parsing args in main: %v", parsed.Err)
		return 2
	}
	if !parsed.RanSubCmd {
		parsed.Parser.PrintChoices(os.Stderr)
		return 2
	}

	cfg := dht.NewDefaultServerConfig()
	if serverArgs.QueryResendDelay != 0 {
		cfg.QueryResendDelay = func() time.Duration { return serverArgs.QueryResendDelay }
	}
	conn, err := net.ListenPacket(serverArgs.Network, ":0")
	if err != nil {
		log.Printf("listening: %v", err)
		return 1
	}
	cfg.Conn = conn
	all, err := publicip.Get(ctx, serverArgs.Network)
	if err == nil {
		cfg.PublicIP = all[0]
		log.Printf("public ip: %q", cfg.PublicIP)
		cfg.NoSecurity = !serverArgs.Secure
	}
	if len(serverArgs.BootstrapAddr) != 0 {
		cfg.StartingNodes = func() ([]dht.Addr, error) {
			return dht.ResolveHostPorts(serverArgs.BootstrapAddr)
		}
	}
	s, err = dht.NewServer(cfg)
	if err != nil {
		conn.Close()
		log.Printf("creating server: %v", err)
		return 1
	}
	defer s.Close()
	log.Printf("dht server on %s with id %x", s.Addr(), s.ID())

	if err := parsed.Run(); err != nil {
		log.Printf("error running main parse result: %v", err)
		return 2
	}
	return 0
}
