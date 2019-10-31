module github.com/anacrolix/dht/v2

require (
	github.com/anacrolix/envpprof v1.0.1
	github.com/anacrolix/log v0.3.1-0.20190913000754-831e4ffe0174
	github.com/anacrolix/missinggo v1.2.1
	github.com/anacrolix/sync v0.2.0
	github.com/anacrolix/tagflag v1.0.1
	github.com/anacrolix/torrent v1.7.1
	github.com/bradfitz/iter v0.0.0-20190303215204-33e6a9893b0c
	github.com/docopt/docopt-go v0.0.0-20180111231733-ee0de3bc6815
	github.com/lukechampine/stm v0.0.0-20191022212748-05486c32d236
	github.com/pkg/errors v0.8.1
	github.com/stretchr/testify v1.4.0
	github.com/willf/bloom v2.0.3+incompatible
	golang.org/x/sys v0.0.0-20190910064555-bbd175535a8b // indirect
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
)

go 1.13

replace github.com/lukechampine/stm => github.com/anacrolix/stm v0.0.0-20191031052127-d04075d6f23e
