package dht

import (
	"golang.org/x/time/rate"
)

var defaultSendLimiter = rate.NewLimiter(100, 100)
