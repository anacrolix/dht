// Command dht-secure-id outputs the node ID secured with the IP.
package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "dht-secure-id outputs the node ID secured with the IP.\n\nUsage: dht-secure-id <id> <ip>\n")
		os.Exit(2)
	}
	b, err := hex.DecodeString(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad id: %s\n", err)
		os.Exit(2)
	}
	var id krpc.ID
	if len(b) != len(id) {
		fmt.Fprintf(os.Stderr, "bad id: wrong length\n")
		os.Exit(2)
	}
	ip := net.ParseIP(os.Args[2])
	if ip == nil {
		fmt.Fprintf(os.Stderr, "bad ip\n")
		os.Exit(2)
	}
	copy(id[:], b)
	dht.SecureNodeId(&id, ip)
	fmt.Printf("%x\n", id)
}
