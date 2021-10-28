package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"

	"github.com/anacrolix/args/targets"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
)

type PutCmd struct {
	Strings bool
	Data    []string `arg:"positional"`
	Key     targets.Hex
	Seq     int64
	Cas     int64
	Salt    string
	Mutable bool
}

func put(cmd *PutCmd) (err error) {
	s, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	defer s.Close()
	if len(cmd.Data) == 0 {
		return errors.New("no payloads given")
	}
	mutable := cmd.Mutable || len(cmd.Key.Bytes) != 0 || cmd.Cas != 0 || cmd.Seq != 0 || len(cmd.Salt) != 0
	for _, data := range cmd.Data {
		putBytes := []byte(data)
		var v interface{}
		if cmd.Strings {
			var s interface{} = string(putBytes)
			v = s
			putBytes, err = bencode.Marshal(v)
			if err != nil {
				return fmt.Errorf("marshalling string arg to bytes: %w", err)
			}
		} else {
			err = bencode.Unmarshal(putBytes, &v)
			if err != nil {
				return
			}
		}
		put := bep44.Put{
			V:    v,
			Salt: []byte(cmd.Salt),
			Cas:  cmd.Cas,
			Seq:  cmd.Seq,
		}
		if mutable {
			privKey := ed25519.NewKeyFromSeed(cmd.Key.Bytes)
			put.K = (*[32]byte)(privKey.Public().(ed25519.PublicKey))
			put.Sign(privKey)
		}
		target := put.Target()
		log.Printf("putting %q to %x", v, target)
		var stats *traversal.Stats
		stats, err = getput.Put(context.Background(), target, s, put)
		if err != nil {
			err = fmt.Errorf("in traversal: %w", err)
			return
		}
		log.Printf("%+v", stats)
		fmt.Printf("%x\n", target)
	}
	return nil
}
