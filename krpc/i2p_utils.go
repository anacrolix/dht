package krpc

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
)

var (
	i2pB64enc *base64.Encoding = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~")
	i2pB32enc *base32.Encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567")
)

type I2PDest []byte

func (i2p I2PDest) String() string {
	if len(i2p) == 32 {
		b32addr := make([]byte, 56)
		i2pB32enc.Encode(b32addr, i2p[:])

		return string(b32addr[:52]) + ".b32.i2p"
	}

	buf := make([]byte, i2pB64enc.EncodedLen(len(i2p)))
	i2pB64enc.Encode(buf, i2p)

	return string(buf)
}

//Return a 32 byte representation of the destination
func (i2p I2PDest) Compact() I2PDest {
	if len(i2p) == 32 {
		return i2p
	}

	//invalid destination
	if len(i2p) < 387 || len(i2p) > 475 {
		return I2PDest{}
	}

	sum := sha256.Sum256(i2p)
	addr := make(I2PDest, 32)
	copy(addr[:], sum[:])

	return addr
}
