# dht

[![CircleCI](https://circleci.com/gh/anacrolix/dht.svg?style=shield)](https://circleci.com/gh/anacrolix/dht)
[![GoDoc](https://godoc.org/github.com/anacrolix/dht?status.svg)](https://godoc.org/github.com/anacrolix/dht)
[![Join the chat at https://gitter.im/anacrolix/torrent](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/anacrolix/torrent?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

## Installation

Get the library package with `go get github.com/anacrolix/dht/v2`, or the provided cmds with `go install github.com/anacrolix/dht/v2/cmd/...@latest`.

## Commands

Here I'll describe what some of the provided commands in `./cmd` do.

### dht-ping

Pings DHT nodes with the given network addresses.

    $ go run ./cmd/dht-ping router.bittorrent.com:6881 router.utorrent.com:6881
    2015/04/01 17:21:23 main.go:33: dht server on [::]:60058
    32f54e697351ff4aec29cdbaabf2fbe3467cc267 (router.bittorrent.com:6881): 648.218621ms
    ebff36697351ff4aec29cdbaabf2fbe3467cc267 (router.utorrent.com:6881): 873.864706ms
    2/2 responses (100.000000%)
