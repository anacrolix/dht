package bep44

import (
	"crypto/ed25519"
	"testing"

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
