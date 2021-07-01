package main

import (
	"context"
	"crypto/sha1"
	"fmt"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/exts/putget"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/bencode"
	"github.com/davecgh/go-spew/spew"
)

type GetCmd struct {
	Target *krpc.ID `arg:"positional"`
	Put    *string
}

func get(cmd *GetCmd) (err error) {
	s, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	defer s.Close()
	var put *interface{}
	var target krpc.ID
	if cmd.Put != nil {
		putBytes := []byte(*cmd.Put)
		err = bencode.Unmarshal(putBytes, &put)
		if err != nil {
			return
		}
		h := sha1.Sum(putBytes)
		if cmd.Target != nil && h != *cmd.Target {
			err = fmt.Errorf("put value hash %x != target %x", h, cmd.Target)
			return
		}
		target = h
		if cmd.Target == nil {
			log.Printf("target is %x", h)
		}
	} else {
		target = *cmd.Target
	}
	spew.Dump("put is", put)
	v, err := putget.Get(context.Background(), target, s, put)
	if err != nil {
		return
	}
	spew.Dump(v)
	return nil
}
