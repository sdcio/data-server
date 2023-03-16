package netconf

import "github.com/iptecharch/schema-server/datastore/target/netconf/types"

type Driver interface {
	// GetConfig
	GetConfig(source string, filter string) (*types.NetconfResponse, error)
	// EditConfig applies to the provided configuration target (candidate|running) the xml config provided in the config parameter
	EditConfig(target string, config string) (*types.NetconfResponse, error)
	// Commit applies the candidate changes to the running config
	Commit() error
	// Close the connection to the device
	Close() error
	// discard a candidate config
	Discard() error
}
