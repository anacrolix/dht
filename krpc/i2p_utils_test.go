package krpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	decode32 = i2pB32enc.DecodeString
)

const suffixb32 = ".b32.i2p"

func makeTestAddr32(addr string) I2PDest {
	b, err := i2pB32enc.DecodeString(addr + "====")
	if err != nil {
		panic(err)
	}

	var dest I2PDest = b
	return dest
}

func makeTestAddr64(addr string) I2PDest {
	b, err := i2pB64enc.DecodeString(addr)
	if err != nil {
		panic(err)
	}

	var dest I2PDest = b
	return dest
}

const (
	dest32_1 = "udhdrtrcetjm5sxzskjyr5ztpeszydbh4dpl3pl4utgqqw2v4jna"
	dest32_2 = "tmipbl5d7ctnz3cib4yd2yivlrssrtpmuuzyqdpqkelzmnqllhda"
	dest32_3 = "lhbd7ojcaiofbfku7ixh47qj537g572zmhdc4oilvugzxdpdghua"

	dest64_1 = "8ZAW~KzGFMUEj0pdchy6GQOOZbuzbqpWtiApEj8LHy2~O~58XKxRrA43cA23a9oDpNZDqWhRWEtehSnX5NoCwJcXWWdO1ksKEUim6cQLP-VpQyuZTIIqwSADwgoe6ikxZG0NGvy5FijgxF4EW9zg39nhUNKRejYNHhOBZKIX38qYyXoB8XCVJybKg89aMMPsCT884F0CLBKbHeYhpYGmhE4YW~aV21c5pebivvxeJPWuTBAOmYxAIgJE3fFU-fucQn9YyGUFa8F3t-0Vco-9qVNSEWfgrdXOdKT6orr3sfssiKo3ybRWdTpxycZ6wB4qHWgTSU5A-gOA3ACTCMZBsASN3W5cz6GRZCspQ0HNu~R~nJ8V06Mmw~iVYOu5lDvipmG6-dJky6XRxCedczxMM1GWFoieQ8Ysfuxq-j8keEtaYmyUQme6TcviCEvQsxyVirr~dTC-F8aZ~y2AlG5IJz5KD02nO6TRkI2fgjHhv9OZ9nskh-I2jxAzFP6Is1kyAAAA"
	dest64_2 = "Gw1kgEBbxZjENfiTdTQNRZYBwwyJVjXtF~t5D0-XMmeVeizW-s~90~XTtAqQ8n41roBCWtr9lrAhJ8S1drBivatp85G3bXH~eV0ZYhmcFTLd-6UUP2eFbG~0Fmmvf-Pb6UFH9J0yKBdkqLaQB82AHWbz9CTNIf~3xAMBit2AJ8XQZ8haLcIH1kxUYae1~mkgiFPPFXg1MxONOjjJ9vaTDLeYofyS8hG95s1hp60x5xNGG6gi2pmCGopQDX46ZrzpNcaZkGHey4uEZGcSiYTm7S1hycQApBYNvCv4QvV92E0eFYqCm6thUOV7K78mii5agaqpcumDBy2PXLnwR0XrqjZnKBxydCcS-HockXR7nVykJL3moQOKswoMEChXMzQqD~RUrrmzHE80oXwZjExGNnp1hI7jZevYg38voDE3TT-3IT84kuLeb-1yH0p-HbiKBk4VLOpRsFpLD9V-tl0w9j7GWOchWX78Xxq7NTWa~xaQdrrCw60Ztw4Zzu2taMekBQAEAAcAAA=="
	dest64_3 = "GKapJ8koUcBj~jmQzHsTYxDg2tpfWj0xjQTzd8BhfC9c3OS5fwPBNajgF-eOD6eCjFTqTlorlh7Hnd8kXj1qblUGXT-tDoR9~YV8dmXl51cJn9MVTRrEqRWSJVXbUUz9t5Po6Xa247Vr0sJn27R4KoKP8QVj1GuH6dB3b6wTPbOamC3dkO18vkQkfZWUdRMDXk0d8AdjB0E0864nOT~J9Fpnd2pQE5uoFT6P0DqtQR2jsFvf9ME61aqLvKPPWpkgdn4z6Zkm-NJOcDz2Nv8Si7hli94E9SghMYRsdjU-knObKvxiagn84FIwcOpepxuG~kFXdD5NfsH0v6Uri3usE3XWD7Pw6P8qVYF39jUIq4OiNMwPnNYzy2N4mDMQdsdHO3LUVh~DEppOy9AAmEoHDjjJxt2BFBbGxfdpZCpENkwvmZeYUyNCCzASqTOOlNzdpne8cuesn3NDXIpNnqEE6Oe5Qm5YOJykrX~Vx~cFFT3QzDGkIjjxlFBsjUJyYkFjBQAEAAcAAA=="
)

func TestStringB32(t *testing.T) {
	dest1 := makeTestAddr32(dest32_1)
	assert.EqualValues(t, dest32_1+suffixb32, dest1.String())

	dest2 := makeTestAddr32(dest32_2)
	assert.EqualValues(t, dest32_2+suffixb32, dest2.String())
}

func TestStringFullDestB64(t *testing.T) {
	dest1 := makeTestAddr64(dest64_1)
	assert.EqualValues(t, dest64_1, dest1.String())

	dest2 := makeTestAddr64(dest64_2)
	assert.EqualValues(t, dest64_2, dest2.String())
}

func TestCompact(t *testing.T) {
	dest1 := makeTestAddr32(dest32_1)
	dest1Compacted := dest1.Compact()
	assert.EqualValues(t, dest1.String(), dest1Compacted.String())

	dest2 := makeTestAddr64(dest64_2)
	dest2Compacted := dest2.Compact()
	dest2CompactB32 := makeTestAddr32(dest32_2)
	assert.EqualValues(t, dest2CompactB32.String(), dest2Compacted.String())

	dest3 := makeTestAddr64(dest64_3)
	dest3Compacted := dest3.Compact()
	assert.NotEqualValues(t, dest3.String(), dest3Compacted.String())

	dest3CompactB32 := makeTestAddr32(dest32_3)
	assert.EqualValues(t, dest3CompactB32.String(), dest3Compacted.String())
}
