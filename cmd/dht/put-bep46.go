package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/anacrolix/args/targets"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent"
	"github.com/multiformats/go-base36"
)

type PutMutableInfohash struct {
	Key  targets.Hex
	Seq  int64
	Cas  int64
	Salt string
}

func putMutableInfohash(cmd *PutMutableInfohash, ih torrent.InfoHash) (err error) {
	s, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	defer s.Close()
	put := bep44.Put{
		V:    krpc.Bep46Payload{Ih: ih},
		Salt: []byte(cmd.Salt),
		Cas:  cmd.Cas,
		Seq:  cmd.Seq,
	}
	privKey := ed25519.NewKeyFromSeed(cmd.Key.Bytes)
	put.K = (*[32]byte)(privKey.Public().(ed25519.PublicKey))
	put.Sign(privKey)
	target := put.Target()
	log.Printf("putting %q to %x", put.V, target)
	var stats *traversal.Stats
	stats, err = getput.Put(context.Background(), target, s, put)
	if err != nil {
		err = fmt.Errorf("in traversal: %w", err)
		return
	}
	log.Printf("%+v", stats)
	fmt.Printf("%x\n", target)
	link := fmt.Sprintf("magnet:?xs=urn:btpk:%x", *put.K)
	if len(put.Salt) != 0 {
		link += "&s=" + hex.EncodeToString(put.Salt)
	}
	fmt.Println(link)
	fmt.Println("base32hex", base32.HexEncoding.EncodeToString(put.K[:]))
	fmt.Println("base36lc", base36.EncodeToStringLc(put.K[:]))
	return nil
}
