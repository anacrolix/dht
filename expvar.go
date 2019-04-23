package dht

import (
	"expvar"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	readZeroPort       = expvar.NewInt("dhtReadZeroPort")
	readBlocked        = expvar.NewInt("dhtReadBlocked")
	readNotKRPCDict    = expvar.NewInt("dhtReadNotKRPCDict")
	readUnmarshalError = expvar.NewInt("dhtReadUnmarshalError")
	readAnnouncePeer   = expvar.NewInt("dhtReadAnnouncePeer")
	announceErrors     = expvar.NewInt("dhtAnnounceErrors")
	writeErrors        = expvar.NewInt("dhtWriteErrors")
	writes             = expvar.NewInt("dhtWrites")
	expvars            = expvar.NewMap("dht")
)

func init() {
	prometheus.MustRegister(prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"dht": prometheus.NewDesc("expvar_dht", "", []string{"key"}, nil),
	}))
}
