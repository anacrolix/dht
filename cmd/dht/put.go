package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"math"

	"github.com/anacrolix/args/targets"
	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent/bencode"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/traversal"
)

type PutCmd struct {
	Strings bool
	Data    []string `arg:"positional"`
	Key     targets.Hex
	Seq     int64
	Cas     int64
	Salt    string
	Mutable bool
	AutoSeq bool
}

func validateMutableKey(mutable bool, key []byte) error {
	if mutable && len(key) != ed25519.SeedSize {
		return fmt.Errorf("mutable key must be %d bytes, got %d", ed25519.SeedSize, len(key))
	}
	return nil
}

func makeSeqToPut(autoSeq, mutable bool, put bep44.Put, privKey ed25519.PrivateKey) getput.SeqToPut {
	return func(seq int64) bep44.Put {
		if autoSeq && seq != math.MaxInt64 {
			put.Seq = seq + 1
		} else if autoSeq {
			// BEP 44 sequence numbers must not exceed MaxInt64. Reusing the maximum lets
			// identical content refresh its timeout; a changed value is rejected remotely.
			put.Seq = seq
		}
		if mutable {
			put.Sign(privKey)
		}
		return put
	}
}

func put(ctx context.Context, cmd *PutCmd) (err error) {
	s, err := dht.NewServer(nil)
	if err != nil {
		return
	}
	defer s.Close()
	if len(cmd.Data) == 0 {
		return errors.New("no payloads given")
	}
	mutable := cmd.Mutable || len(cmd.Key.Bytes) != 0 || cmd.Cas != 0 || len(cmd.Salt) != 0
	if err := validateMutableKey(mutable, cmd.Key.Bytes); err != nil {
		return err
	}
	for _, data := range cmd.Data {
		var v any
		if cmd.Strings {
			v = data
		} else if err = bencode.Unmarshal([]byte(data), &v); err != nil {
			return fmt.Errorf("parsing value bencode: %w", err)
		}
		put := bep44.Put{
			V:    v,
			Salt: []byte(cmd.Salt),
			Cas:  cmd.Cas,
			Seq:  cmd.Seq,
		}
		var privKey g.Option[ed25519.PrivateKey]
		if mutable {
			privKey.Set(ed25519.NewKeyFromSeed(cmd.Key.Bytes))
			put.K = (*[32]byte)(privKey.Value.Public().(ed25519.PublicKey))
		}
		target := put.Target()
		log.Printf("putting %q to %x", v, target)
		var stats *traversal.Stats
		stats, err = getput.Put(
			ctx,
			target,
			s,
			put.Salt,
			makeSeqToPut(cmd.AutoSeq, mutable, put, privKey.Value),
		)
		if err != nil {
			err = fmt.Errorf("in traversal: %w", err)
			return
		}
		log.Printf("%+v", stats)
		fmt.Printf("%x\n", target)
	}
	return nil
}
