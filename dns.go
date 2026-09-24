package dht

import (
	"sync"
	"time"

	"github.com/rs/dnscache"
)

// A cache to prevent wasteful/excessive use of DNS when trying to bootstrap.
// https://github.com/anacrolix/dht/issues/43
var dnsResolver = sync.OnceValue(func() *dnscache.Resolver {
	r := &dnscache.Resolver{}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			r.Refresh(false)
		}
	}()
	return r
})
