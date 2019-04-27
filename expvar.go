package dht

import (
	"expvar"
)

var (
	expvars = expvar.NewMap("dht")
)
