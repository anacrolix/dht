package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/anacrolix/args/targets"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/args"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/publicip"
	"github.com/anacrolix/torrent"
)

func main() {
	logger := log.Default.WithNames("main")
	ctx := log.ContextWithLogger(context.Background(), logger)
	ctx, stopSignalNotify := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignalNotify()
	var s *dht.Server
	args.Main{
		Params: []args.Param{
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
			args.Subcommand("put", func(ctx args.SubCmdCtx) (err error) {
				var putOpt PutCmd
				ctx.Parse(args.FromStruct(&putOpt)...)
				switch len(putOpt.Key.Bytes) {
				case 0, 32:
				default:
					return fmt.Errorf("key has bad length %v", len(putOpt.Key.Bytes))
				}
				ctx.Defer(func() error { return put(&putOpt) })
				return nil
			}),
			args.Subcommand("put-mutable-infohash", func(ctx args.SubCmdCtx) (err error) {
				var putOpt PutMutableInfohash
				var ih torrent.InfoHash
				ctx.Parse(append(
					args.FromStruct(&putOpt),
					args.Opt(args.OptOpt{
						Long:     "info hash",
						Target:   &ih,
						Required: true,
					}))...)
				ctx.Defer(func() error { return putMutableInfohash(&putOpt, ih) })
				return nil
			}),
			args.Subcommand("get", func(ctx args.SubCmdCtx) error {
				var getOpt GetCmd
				ctx.Parse(args.FromStruct(&getOpt)...)
				ctx.Defer(func() error { return get(&getOpt) })
				return nil
			}),
			args.Subcommand("ping", func(ctx args.SubCmdCtx) (err error) {
				var pa pingArgs
				ctx.Parse(args.FromStruct(&pa)...)
				ctx.Defer(func() error {
					return ping(pa, s)
				})
				return nil
			}),
			args.Subcommand("get-peers", func(sub args.SubCmdCtx) (err error) {
				var subArgs = struct {
					AnnouncePort int
					Scrape       bool
					InfoHash     torrent.InfoHash
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
		},
		AfterParse: func() (err error) {
			cfg := dht.NewDefaultServerConfig()
			all, err := publicip.GetAll(context.TODO())
			if err == nil {
				cfg.PublicIP = all[0].IP
				log.Printf("public ip: %q", cfg.PublicIP)
				cfg.NoSecurity = false
			}
			s, err = dht.NewServer(cfg)
			if err != nil {
				return err
			}
			log.Printf("dht server on %s with id %x", s.Addr(), s.ID())
			return
		},
	}.Do()
}
