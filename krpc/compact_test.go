package krpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalSlice(t *testing.T) {
	var data CompactIPv4NodeInfo
	err := data.UnmarshalBencode([]byte("52:" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x02\x03\x04\x05\x06" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x03\x04\x05\x06\x07"))
	require.NoError(t, err)
	require.Len(t, data, 2)
	assert.Equal(t, "1.2.3.4", data[0].Addr.IP.String())
	assert.Equal(t, "2.3.4.5", data[1].Addr.IP.String())
}
