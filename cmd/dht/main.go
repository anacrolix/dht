package main

import (
	"context"
	"fmt"
	"log"

	"github.com/anacrolix/args"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/publicip"
	"github.com/anacrolix/torrent"
)

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	var s *dht.Server
	args.Main{
		Params: []args.Param{
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
