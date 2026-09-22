package dht

import (
	"os"

	"github.com/anacrolix/dht/v2/krpc"
)

func WriteNodesToFile(ns []krpc.NodeInfo, fileName string) error {
	b, err := krpc.CompactIPv6NodeInfo(ns).MarshalBinary()
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, b, 0o640)
}

func ReadNodesFromFile(fileName string) ([]krpc.NodeInfo, error) {
	b, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	var cnis krpc.CompactIPv6NodeInfo
	err = cnis.UnmarshalBinary(b)
	return cnis, err
}
