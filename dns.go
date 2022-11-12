package dht

import (
	"sync"
	"time"

	"github.com/rs/dnscache"
)

var (
	// A cache to prevent wasteful/excessive use of DNS when trying to bootstrap.
	dnsResolver     *dnscache.Resolver
	dnsResolverInit sync.Once
)

func dnsResolverRefresher() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		<-ticker.C
		dnsResolver.Refresh(false)
	}
}

// https://github.com/anacrolix/dht/issues/43
func initDnsResolver(resolver dnscache.DNSResolver) {
	dnsResolverInit.Do(func() {
		dnsResolver = &dnscache.Resolver{
			Resolver: resolver,
		}
		go dnsResolverRefresher()
	})
}
