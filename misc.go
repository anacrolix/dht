package dht

import "net"

func mustListen(addr string) net.PacketConn {
	ret, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	return ret
}

func mustResolveAddr(addr string) Addr {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	return NewAddr(ua)
}
