package dht

import (
	"github.com/anacrolix/dht/v2/rate"
)

var defaultSendLimiter = rate.NewLimiter(25, 25)
