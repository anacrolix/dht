package dht

import (
	"golang.org/x/time/rate"
)

var defaultSendLimiter = rate.NewLimiter(25, 25)
