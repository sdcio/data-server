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

import (
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	sbiNOOP    = "noop"
	sbiNETCONF = "netconf"
	sbiGNMI    = "gnmi"

	ncCommitDatastoreRunning   = "running"
	ncCommitDatastoreCandidate = "candidate"
)

type DatastoreConfig struct {
	Name       string        `yaml:"name,omitempty" json:"name,omitempty"`
	Schema     *SchemaConfig `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI        *SBI          `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	Sync       *Sync         `yaml:"sync,omitempty" json:"sync,omitempty"`
	Validation *Validation   `yaml:"validation,omitempty" json:"validation,omitempty"`
}

type SBI struct {
	// Southbound interface type, one of: gnmi, netconf
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// gNMI or netconf address
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	Port    uint32 `yaml:"port,omitempty" json:"port,omitempty"`
	// TLS config
	TLS *TLS `yaml:"tls,omitempty" json:"tls,omitempty"`
	// Target SBI credentials
	Credentials    *Creds             `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	NetconfOptions *SBINetconfOptions `yaml:"netconf-options,omitempty" json:"netconf-options,omitempty"`
	GnmiOptions    *SBIGnmiOptions    `yaml:"gnmi-options,omitempty" json:"gnmi-options,omitempty"`
	// ConnectRetry
	ConnectRetry time.Duration `yaml:"connect-retry,omitempty" json:"connect-retry,omitempty"`
	// Timeout
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type SBIGnmiOptions struct {
	Encoding string `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

type SBINetconfOptions struct {
	// if true, the namespace is included as an `xmlns` attribute in the netconf payloads
	IncludeNS bool `yaml:"include-ns,omitempty" json:"include-ns,omitempty"`
	// sets the preferred NC version: 1.0 or 1.1
	PreferredNCVersion string `yaml:"preferred-nc-version,omitempty" json:"preferred-nc-version,omitempty"`
	// add a namespace when specifying a netconf operation such as 'delete' or 'remove'
	OperationWithNamespace bool `yaml:"operation-with-namespace,omitempty" json:"operation-with-namespace,omitempty"`
	// use 'remove' operation instead of 'delete'
	UseOperationRemove bool `yaml:"use-operation-remove,omitempty" json:"use-operation-remove,omitempty"`
	// for netconf targets: defines whether to commit to running or use a candidate.
	CommitDatastore string `yaml:"commit-datastore,omitempty" json:"commit-datastore,omitempty"`
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}

type Sync struct {
	Validate     bool            `yaml:"validate,omitempty" json:"validate,omitempty"`
	Buffer       int64           `yaml:"buffer,omitempty" json:"buffer,omitempty"`
	WriteWorkers int64           `yaml:"write-workers,omitempty" json:"write-workers,omitempty"`
	Config       []*SyncProtocol `yaml:"config,omitempty" json:"config,omitempty"`
}

type SyncProtocol struct {
	Name     string        `yaml:"name,omitempty" json:"name,omitempty"`
	Protocol string        `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Paths    []string      `yaml:"paths,omitempty" json:"paths,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
	Mode     string        `yaml:"mode,omitempty" json:"mode,omitempty"`
	Encoding string        `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

type CacheConfig struct {
	// cache type: "local" or "remote"
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// Local cache attr
	StoreType string `yaml:"store-type,omitempty" json:"store-type,omitempty"`
	Dir       string `yaml:"dir,omitempty" json:"dir,omitempty"`
	// Remote cache attr
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
}

func (ds *DatastoreConfig) ValidateSetDefaults() error {
	var err error
	if err = ds.SBI.validateSetDefaults(); err != nil {
		return err
	}
	if ds.Sync != nil {
		if err = ds.Sync.validateSetDefaults(); err != nil {
			return err
		}
	}

	if ds.Validation == nil {
		ds.Validation = &Validation{}
	}
	if err := ds.Validation.validateSetDefaults(); err != nil {
		return err
	}

	return nil
}

func (s *SBI) validateSetDefaults() error {
	switch s.Type {
	case sbiNOOP:
		return nil
	case sbiNETCONF:
		switch s.NetconfOptions.CommitDatastore {
		case "":
			s.NetconfOptions.CommitDatastore = ncCommitDatastoreCandidate
		case ncCommitDatastoreRunning:
		case ncCommitDatastoreCandidate:
		default:
			return fmt.Errorf("unknown commit-datastore: %s. Must be one of %s, %s",
				s.NetconfOptions.CommitDatastore, ncCommitDatastoreCandidate, ncCommitDatastoreRunning)
		}
	case sbiGNMI:
		if s.GnmiOptions.Encoding == "" {
			return errors.New("no encoding defined")
		}
	default:
		return fmt.Errorf("unknown sbi type: %q", s.Type)
	}

	if s.Address == "" {
		return errors.New("missing SBI address")
	}

	if s.Port == 0 {
		return errors.New("missing sbi port")
	}

	if s.ConnectRetry < time.Second {
		s.ConnectRetry = time.Second
	}

	if s.Timeout <= 0 {
		s.Timeout = defaultTimeout
	}
	return nil
}

func (s *Sync) validateSetDefaults() error {
	// no sync
	if s == nil || len(s.Config) == 0 {
		return nil
	}
	if s.Buffer <= 0 {
		s.Buffer = defaultBufferSize
	}
	if s.WriteWorkers <= 0 {
		s.WriteWorkers = defaultWriteWorkers
	}

	var errs []error
	for _, sp := range s.Config {
		err := sp.validateSetDefaults()
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
	if err != nil {
		return err
	}

	return nil
}

func (s *SyncProtocol) validateSetDefaults() error {
	if s.Interval <= 0 {
		s.Interval = defaultSyncInterval
	}
	return nil
}

func (c *CacheConfig) validateSetDefaults() error {
	switch c.Type {
	case "remote":
		if c.Address == "" {
			c.Address = defaultRemoteCacheAddress
		}
		_, _, err := net.SplitHostPort(c.Address)
		if err != nil {
			return err
		}
	default:
		if c.Type != defaultCacheType {
			c.Type = defaultCacheType
		}
		if c.StoreType == "" {
			c.StoreType = defaultStoreType
		}
		if c.Dir == "" {
			c.Dir = defaultCacheDir
		}
	}
	return nil
}
