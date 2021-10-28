module github.com/anacrolix/dht/v2

require (
	github.com/anacrolix/args v0.1.1-0.20211020052733-53ed238acbd4
	github.com/anacrolix/chansync v0.3.0-0.0.20211007004133-3f72684c4a93
	github.com/anacrolix/confluence v1.9.0
	github.com/anacrolix/envpprof v1.1.1
	github.com/anacrolix/log v0.10.0
	github.com/anacrolix/missinggo v1.3.0
	github.com/anacrolix/missinggo/v2 v2.5.2
	github.com/anacrolix/multiless v0.2.0
	github.com/anacrolix/stm v0.3.0
	github.com/anacrolix/sync v0.4.0
	github.com/anacrolix/tagflag v1.3.0
	github.com/anacrolix/torrent v1.33.0
	github.com/benbjohnson/immutable v0.3.0
	github.com/bradfitz/iter v0.0.0-20191230175014-e8f45d346db8
	github.com/docopt/docopt-go v0.0.0-20180111231733-ee0de3bc6815
	github.com/frankban/quicktest v1.14.0
	github.com/pkg/errors v0.9.1
	github.com/rs/dnscache v0.0.0-20210201191234-295bba877686
	github.com/stretchr/testify v1.7.0
	github.com/willf/bloom v2.0.3+incompatible
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
)

require (
	github.com/anacrolix/missinggo/perf v1.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/edsrzf/mmap-go v1.0.0 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/willf/bitset v1.1.11 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20211023085530-d6a326fbbf70 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

go 1.18

exclude github.com/willf/bitset v1.2.0

replace github.com/anacrolix/args => ../args

replace github.com/anacrolix/torrent => ../torrent
