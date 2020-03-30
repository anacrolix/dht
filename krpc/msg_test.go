package krpc

import (
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMarshalUnmarshalMsg(t *testing.T, m Msg, expected string) {
	b, err := bencode.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, expected, string(b))
	var _m Msg
	err = bencode.Unmarshal([]byte(expected), &_m)
	assert.NoError(t, err)
	assert.EqualValues(t, m, _m)
	assert.EqualValues(t, m.A, _m.A)
	assert.EqualValues(t, m.R, _m.R)
}

func TestMarshalUnmarshalMsg(t *testing.T) {
	testMarshalUnmarshalMsg(t, Msg{}, "d1:t0:1:y0:e")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "q",
		Q: "ping",
		T: "hi",
	}, "d1:q4:ping1:t2:hi1:y1:qe")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "e",
		T: "42",
		E: &Error{Code: 200, Msg: "fuck"},
	}, "d1:eli200e4:fucke1:t2:421:y1:ee")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x8c%",
		R: &Return{},
	}, "d1:rd2:id20:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00e1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t,
		Msg{
			Y: "r",
			T: "\x8c%",
			R: &Return{
				Nodes: CompactIPv4NodeInfo{
					NodeInfo{
						Addr: NodeAddr{
							IP:   net.IPv4(1, 2, 3, 4).To4(),
							Port: 0x1234,
						},
					},
				},
			},
		},
		"d1:rd2:id20:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x005:nodes26:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x02\x03\x04\x124e1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x8c%",
		R: &Return{
			Values: []NodeAddr{
				{
					IP:   net.IPv4(1, 2, 3, 4).To4(),
					Port: 0x5678,
				},
			},
		},
	}, "d1:rd2:id20:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x006:valuesl6:\x01\x02\x03\x04\x56\x78ee1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x03",
		R: &Return{
			ID: IdFromString("\xeb\xff6isQ\xffJ\xec)ͺ\xab\xf2\xfb\xe3F|\xc2g"),
		},
		IP: NodeAddr{
			IP:   net.IPv4(124, 168, 180, 8).To4(),
			Port: 62844,
		},
	}, "d2:ip6:|\xa8\xb4\b\xf5|1:rd2:id20:\xeb\xff6isQ\xffJ\xec)ͺ\xab\xf2\xfb\xe3F|\xc2ge1:t1:\x031:y1:re")
}

func TestUnmarshalGetPeersResponse(t *testing.T) {
	var msg Msg
	err := bencode.Unmarshal([]byte("d1:rd6:valuesl6:\x01\x02\x03\x04\x05\x066:\x07\x08\x09\x0a\x0b\x0ce5:nodes52:\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07ee"), &msg)
	require.NoError(t, err)
	assert.Len(t, msg.R.Values, 2)
	assert.Len(t, msg.R.Nodes, 2)
	assert.Nil(t, msg.E)
}

func unprettifyHex(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "\n", "")
}

func TestBep33BloomFilter(t *testing.T) {
	var f ScrapeBloomFilter
	for i := 0; i <= 255; i++ {
		f.AddIp(net.IPv4(192, 0, 2, byte(i)).To4())
	}
	for i := 0; i <= 0x3e7; i++ {
		f.AddIp(net.ParseIP(fmt.Sprintf("2001:DB8::%x", i)))
	}
	expected, err := hex.DecodeString(unprettifyHex(`
F6C3F5EA A07FFD91 BDE89F77 7F26FB2B FF37BDB8 FB2BBAA2 FD3DDDE7 BACFFF75 EE7CCBAE
FE5EEDB1 FBFAFF67 F6ABFF5E 43DDBCA3 FD9B9FFD F4FFD3E9 DFF12D1B DF59DB53 DBE9FA5B
7FF3B8FD FCDE1AFB 8BEDD7BE 2F3EE71E BBBFE93B CDEEFE14 8246C2BC 5DBFF7E7 EFDCF24F
D8DC7ADF FD8FFFDF DDFFF7A4 BBEEDF5C B95CE81F C7FCFF1F F4FFFFDF E5F7FDCB B7FD79B3
FA1FC77B FE07FFF9 05B7B7FF C7FEFEFF E0B8370B B0CD3F5B 7F2BD93F EB4386CF DD6F7FD5
BFAF2E9E BFFFFEEC D67ADBF7 C67F17EF D5D75EBA 6FFEBA7F FF47A91E B1BFBB53 E8ABFB57
62ABE8FF 237279BF EFBFEEF5 FFC5FEBF DFE5ADFF ADFEE1FB 737FFFFB FD9F6AEF FEEE76B6
FD8F72EF
`))
	require.NoError(t, err)
	assert.EqualValues(t, expected, f[:])
	assert.EqualValues(t, 1224.9308, floorDecimals(f.EstimateCount(), 4))
}

func floorDecimals(f float64, decimals int) float64 {
	p := math.Pow10(decimals)
	return math.Floor(f*p) / p
}

func TestEmptyScrapeBloomFilterEstimatedCount(t *testing.T) {
	var f ScrapeBloomFilter
	assert.EqualValues(t, 0, math.Floor(f.EstimateCount()))
}
