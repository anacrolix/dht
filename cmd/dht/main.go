package main

import (
	"fmt"
	"log"

	"github.com/anacrolix/args"
)

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	args.ParseMain(
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
		args.Subcommand("get", func(ctx args.SubCmdCtx) error {
			var getOpt GetCmd
			ctx.Parse(args.FromStruct(&getOpt)...)
			ctx.Defer(func() error { return get(&getOpt) })
			return nil
		}),
	)
}
