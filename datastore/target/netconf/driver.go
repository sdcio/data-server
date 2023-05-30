package netconf

import "github.com/iptecharch/data-server/datastore/target/netconf/types"

type Driver interface {
	// Get config or state
	Get(filter string) (*types.NetconfResponse, error)
	// GetConfig
	GetConfig(source string, filter string) (*types.NetconfResponse, error)
	// EditConfig applies to the provided configuration target (candidate|running) the xml config provided in the config parameter
	EditConfig(target string, config string) (*types.NetconfResponse, error)
	// lock a target datastore
	Lock(target string) (*types.NetconfResponse, error)
	// unlock a target datastore
	Unlock(target string) (*types.NetconfResponse, error)
	// validate a source datastore
	Validate(source string) (*types.NetconfResponse, error)
	// Commit applies the candidate changes to the running config
	Commit() error
	// discard a candidate config
	Discard() error
	// Close the connection to the device
	Close() error
}
