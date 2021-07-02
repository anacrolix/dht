package main

import (
	"context"
	"errors"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/exts/putget"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/davecgh/go-spew/spew"
)

type GetCmd struct {
	Target []krpc.ID `arg:"positional"`
}

func get(cmd *GetCmd) (err error) {
	s, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	defer s.Close()
	if len(cmd.Target) == 0 {
		return errors.New("no targets specified")
	}
	for _, t := range cmd.Target {
		var v interface{}
		v, _, err = putget.Get(context.Background(), t, s)
		if err != nil {
			return
		}
		spew.Dump(v)
	}
	return nil
}
