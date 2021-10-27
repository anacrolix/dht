package main

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/traversal"
	"github.com/anacrolix/torrent/bencode"
)

type PutCmd struct {
	Strings bool
	Data    []string `arg:"positional"`
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
	for _, data := range cmd.Data {
		putBytes := []byte(data)
		var put *interface{}
		if cmd.Strings {
			var s interface{} = string(putBytes)
			put = &s
			putBytes, err = bencode.Marshal(*put)
			if err != nil {
				return fmt.Errorf("marshalling string arg to bytes: %w", err)
			}
		} else {
			err = bencode.Unmarshal(putBytes, &put)
			if err != nil {
				return
			}
		}
		target := sha1.Sum(putBytes)
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
