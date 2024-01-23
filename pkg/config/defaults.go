package config

import "time"

const (
	defaultGRPCAddress    = ":56000"
	defaultMaxRecvMsgSize = 4 * 1024 * 1024
	defaultRPCTimeout     = 30 * time.Minute

	defaultRemoteSchemaServerCacheTTL      = 300 * time.Second
	defaultRemoteSchemaServerCacheCapacity = 1000

	defaultNCPort             = 830
	defaultCacheType          = "local"
	defaultRemoteCacheAddress = "localhost:50100"
	defaultBufferSize         = 1000
	defaultStoreType          = "badgerdb"
	defaultCacheDir           = "./cached/caches"
	defaultWriteWorkers       = 16
	defaultTimeout            = 30 * time.Second

	defaultSchemaStorePath = "./schema-dir"
)
