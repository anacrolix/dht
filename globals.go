package dht

var defaultSendLimiter = newRateLimiter(25, 25)
