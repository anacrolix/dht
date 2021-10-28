package bep44

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"testing"

	"filippo.io/edwards25519"
	qt "github.com/frankban/quicktest"
)

func TestPrivateKeyIsSeed(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	c := qt.New(t)
	c.Assert(err, qt.IsNil)
	t.Logf("generated private key %x", priv)
	seed := priv.Seed()
	t.Logf("it has seed %x", seed)
	seedPriv := ed25519.NewKeyFromSeed(seed)
	t.Logf("private key from seed: %x", seedPriv)
	c.Check(seedPriv.Equal(priv), qt.IsTrue)
	c.Check(
		seedPriv.Public().(ed25519.PublicKey),
		qt.ContentEquals,
		priv.Public().(ed25519.PublicKey))
	t.Logf("public keys:\n%x\n%x", seedPriv.Public(), priv.Public())
}

func TestPublicKeyCopy(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	c := qt.New(t)
	c.Assert(err, qt.IsNil)
	b := (*[32]byte)(pub)
	c.Check(b[:], qt.DeepEquals, []byte(pub))
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestVectorMutableWithSalt(t *testing.T) {
	c := qt.New(t)
	salt := []byte("foobar")
	bv := []byte("12:Hello World!")
	c.Check(
		bufferToSign(salt, bv, 1),
		qt.DeepEquals,
		[]byte("4:salt6:foobar3:seqi1e1:v12:Hello World!"))
	edwardsPrivateKey := mustDecodeHex(
		"e06d3183d14159228433ed599221b80bd0a5ce8352e4bdf0262f76786ef1c74d" +
			"b7e7a9fea2c0eb269d61e3b38e450a22e754941ac78479d6c54e1faf6037881d")
	sig := mustDecodeHex(
		"6834284b6b24c3204eb2fea824d82f88883a3d95e8b4a21b8c0ded553d17d17d" +
			"df9a8a7104b1258f30bed3787e6cb896fca78c58f8e03b5f18f14951a87d9a08")
	pubKey := mustDecodeHex("77ff84905a91936367c01360803104f92432fcd904a43511876df5cdf3e7e548")
	c.Check(
		EdwardsSignSha512(*(*[64]byte)(edwardsPrivateKey), pubKey, bufferToSign(salt, bv, 1)),
		qt.DeepEquals,
		sig)
}

// ed25519 sign with pre-sha512-summed private key. I believe this means we also need the public key
// since we can't derive it from the hash. See https://stackoverflow.com/a/43689064/149482.
func EdwardsSignSha512(privateKey [64]byte, publicKey []byte, message []byte) []byte {
	// Outline the function body so that the returned signature can be
	// stack-allocated.
	signature := make([]byte, ed25519.SignatureSize)
	edwardsSignSha512(signature, privateKey, publicKey, message)
	return signature
}

func edwardsSignSha512(signature []byte, h [64]byte, publicKey []byte, message []byte) {
	s, err := edwards25519.NewScalar().SetBytesWithClamping(h[:32])
	if err != nil {
		panic(err)
	}
	prefix := h[32:]

	mh := sha512.New()
	mh.Write(prefix)
	mh.Write(message)
	messageDigest := make([]byte, 0, sha512.Size)
	messageDigest = mh.Sum(messageDigest)
	r, err := edwards25519.NewScalar().SetUniformBytes(messageDigest)
	if err != nil {
		panic(err)
	}

	R := (&edwards25519.Point{}).ScalarBaseMult(r)

	kh := sha512.New()
	kh.Write(R.Bytes())
	kh.Write(publicKey)
	kh.Write(message)
	hramDigest := make([]byte, 0, sha512.Size)
	hramDigest = kh.Sum(hramDigest)
	k, err := edwards25519.NewScalar().SetUniformBytes(hramDigest)
	if err != nil {
		panic(err)
	}

	S := edwards25519.NewScalar().MultiplyAdd(k, s, r)

	copy(signature[:32], R.Bytes())
	copy(signature[32:], S.Bytes())
}
