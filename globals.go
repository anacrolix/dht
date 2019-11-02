package dht

import (
	"github.com/lukechampine/stm/rate"
)

var defaultSendLimiter = rate.NewLimiter(25, 25)
