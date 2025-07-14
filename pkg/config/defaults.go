// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	defaultSyncInterval       = 30 * time.Second

	defaultSchemaStorePath = "./schema-dir"
)
