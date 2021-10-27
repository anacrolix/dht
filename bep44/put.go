package bep44

type Put struct {
	V    interface{}
	K    [32]byte
	Salt []byte
	Sig  [64]byte
	Cas  int64
	Seq  int64
}

func (p *Put) ToItem() *Item {
	return &Item{
		V:    p.V,
		K:    p.K,
		Salt: p.Salt,
		Sig:  p.Sig,
		Cas:  p.Cas,
		Seq:  p.Seq,
	}
}
