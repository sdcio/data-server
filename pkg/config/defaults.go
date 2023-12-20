package config

import "time"

const (
	defaultNCPort             = 830
	defaultCacheType          = "local"
	defaultRemoteCacheAddress = "localhost:50100"
	defaultBufferSize         = 1000
	defaultStoreType          = "badgerdbsingle"
	defaultCacheDir           = "./cached/caches"
	defaultWriteWorkers       = 16
	defaultTimeout            = 30 * time.Second
)
