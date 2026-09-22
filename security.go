package dht

import (
	"hash/crc32"
	"net"

	"github.com/anacrolix/dht/v2/krpc"
)

var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

func maskForIP(ip net.IP) []byte {
	if ip.To4() != nil {
		return []byte{0x03, 0x0f, 0x3f, 0xff}
	}
	return []byte{0x01, 0x03, 0x07, 0x0f, 0x1f, 0x3f, 0x7f, 0xff}
}

// Generate the CRC used to make or validate secure node ID.
func crcIP(ip net.IP, rand uint8) uint32 {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	mask := maskForIP(ip)
	masked := make([]byte, len(mask))
	for i := range mask {
		masked[i] = ip[i] & mask[i]
	}
	masked[0] |= (rand & 7) << 5
	return crc32.Checksum(masked, castagnoliTable)
}

// Makes a node ID secure, in-place. The ID is 20 raw bytes.
// http://www.libtorrent.org/dht_sec.html
func SecureNodeId(id *krpc.ID, ip net.IP) {
	crc := crcIP(ip, id[19])
	id[0] = byte(crc >> 24 & 0xff)
	id[1] = byte(crc >> 16 & 0xff)
	id[2] = byte(crc>>8&0xf8) | id[2]&7
}

// Returns whether the node ID is considered secure. The id is the 20 raw
// bytes. http://www.libtorrent.org/dht_sec.html
func NodeIdSecure(id [20]byte, ip net.IP) bool {
	if isLocalNetwork(ip) {
		return true
	}
	crc := crcIP(ip, id[19])
	return id[0] == byte(crc>>24&0xff) &&
		id[1] == byte(crc>>16&0xff) &&
		id[2]&0xf8 == byte(crc>>8&0xf8)
}

var localNetworks = []*net.IPNet{
	mustParseCIDRIPNet("10.0.0.0/8"),
	mustParseCIDRIPNet("172.16.0.0/12"),
	mustParseCIDRIPNet("192.168.0.0/16"),
}

func mustParseCIDRIPNet(s string) *net.IPNet {
	_, ret, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return ret
}

// Per http://www.libtorrent.org/dht_sec.html#enforcement, the IP is
// considered a local network address and should be exempted from node ID
// verification.
func isLocalNetwork(ip net.IP) bool {
	for _, n := range localNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return ip.IsLinkLocalUnicast() || ip.IsLoopback()
}
