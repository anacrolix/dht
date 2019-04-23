package dht

import (
	"expvar"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	expvars = expvar.NewMap("dht")
)

func init() {
	prometheus.MustRegister(prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"dht": prometheus.NewDesc("expvar_dht", "", []string{"key"}, nil),
	}))
}
