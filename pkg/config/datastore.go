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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	"github.com/sdcio/data-server/pkg/utils"
)

type DatastoreConfig struct {
	Name       string           `yaml:"name,omitempty" json:"name,omitempty"`
	Schema     *SchemaConfig    `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI        *SBI             `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	Sync       *Sync            `yaml:"sync,omitempty" json:"sync,omitempty"`
	Validation *Validation      `yaml:"validation,omitempty" json:"validation,omitempty"`
	Deviation  *DeviationConfig `yaml:"deviation,omitempty" json:"deviation,omitempty"`
}

type DeviationConfig struct {
	Interval time.Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
}

func (d *DeviationConfig) validateSetDefaults() error {
	switch {
	case d.Interval < 0:
		return errors.New("deviation interval cannot be negative")
	case d.Interval > 0:
		// use configured value
	case os.Getenv("DEVIATION_INTERVAL") != "":
		envInterval, err := time.ParseDuration(os.Getenv("DEVIATION_INTERVAL"))
		if err != nil {
			return fmt.Errorf("invalid DEVIATION_INTERVAL: %w", err)
		}
		d.Interval = envInterval
	default:
		d.Interval = 30 * time.Second
	}

	return nil
}

func (d *DeviationConfig) DeepCopy() *DeviationConfig {
	result := &DeviationConfig{
		Interval: d.Interval,
	}
	return result
}

type SBI struct {
	// Southbound interface type, one of: gnmi, netconf
	Type api.TargetType `yaml:"type,omitempty" json:"type,omitempty"`
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
	Encoding   api.GNMIEncoding `yaml:"encoding,omitempty" json:"encoding,omitempty"`
	TargetName string           `yaml:"target,omitempty" json:"target,omitempty"`
}

type SBINetconfOptions struct {
	// if true, the namespace is included as an `xmlns` attribute in the netconf payloads
	IncludeNS bool `yaml:"include-ns,omitempty" json:"include-ns,omitempty"`
	// sets the preferred NC version: 1.0 or 1.1
	PreferredNCVersion NCVersion `yaml:"preferred-nc-version,omitempty" json:"preferred-nc-version,omitempty"`
	// add a namespace when specifying a netconf operation such as 'delete' or 'remove'
	OperationWithNamespace bool `yaml:"operation-with-namespace,omitempty" json:"operation-with-namespace,omitempty"`
	// use 'remove' operation instead of 'delete'
	UseOperationRemove bool `yaml:"use-operation-remove,omitempty" json:"use-operation-remove,omitempty"`
	// for netconf targets: defines whether to commit to running or use a candidate.
	CommitDatastore CommitDatastore `yaml:"commit-datastore,omitempty" json:"commit-datastore,omitempty"`
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}

// MarshalJSON redacts credentials from config dumps.
func (c Creds) MarshalJSON() ([]byte, error) {
	type creds Creds // sheds the marshaler to avoid recursing into this method
	redacted := creds(c)
	if redacted.Password != "" {
		redacted.Password = utils.Redacted
	}
	if redacted.Token != "" {
		redacted.Token = utils.Redacted
	}
	return json.Marshal(redacted)
}

type Sync struct {
	Config []*SyncProtocol `yaml:"config,omitempty" json:"config,omitempty"`
}

type SyncProtocol struct {
	Name     string           `yaml:"name,omitempty" json:"name,omitempty"`
	Protocol api.TargetType   `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Paths    []string         `yaml:"paths,omitempty" json:"paths,omitempty"`
	Interval time.Duration    `yaml:"interval,omitempty" json:"interval,omitempty"`
	Mode     SyncMode         `yaml:"mode,omitempty" json:"mode,omitempty"`
	Encoding api.GNMIEncoding `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

type CacheConfig struct {
	// cache type: "local" or "remote"
	Type CacheType `yaml:"type,omitempty" json:"type,omitempty"`
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
		if err = ds.Sync.resolveSyncs(ds.SBI); err != nil {
			return err
		}
	}

	if ds.Validation == nil {
		ds.Validation = &Validation{}
	}
	if err := ds.Validation.validateSetDefaults(); err != nil {
		return err
	}
	if ds.Deviation == nil {
		ds.Deviation = &DeviationConfig{}
	}
	if err = ds.Deviation.validateSetDefaults(); err != nil {
		return err
	}
	return nil
}

// validateSetDefaults resolves the target type and its protocol options to
// their canonical typed values and applies defaults.
func (s *SBI) validateSetDefaults() error {
	t, err := api.ParseTargetType(string(s.Type))
	if err != nil {
		return fmt.Errorf("unknown sbi type: %q", s.Type)
	}
	s.Type = t
	switch s.Type {
	case api.TargetTypeNoop:
		return nil
	case api.TargetTypeNetconf:
		if s.NetconfOptions == nil {
			s.NetconfOptions = &SBINetconfOptions{}
		}
		if err := s.NetconfOptions.validateSetDefaults(); err != nil {
			return err
		}
	case api.TargetTypeGNMI:
		if s.GnmiOptions == nil || s.GnmiOptions.Encoding == "" {
			return errors.New("no encoding defined")
		}
		enc, err := api.ParseGNMIEncoding(string(s.GnmiOptions.Encoding))
		if err != nil {
			return err
		}
		s.GnmiOptions.Encoding = enc
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

// resolveSyncs resolves protocol, mode and encoding of every sync to typed
// values. A gNMI sync without its own encoding inherits the datastore's gNMI
// target encoding, and without a mode defaults to on-change. Noop targets
// ignore syncs, so their syncs are left as given.
func (s *Sync) resolveSyncs(sbi *SBI) error {
	if sbi != nil && sbi.Type == api.TargetTypeNoop {
		return nil
	}
	for _, sp := range s.Config {
		if sp.Protocol != "" {
			p, err := api.ParseTargetType(string(sp.Protocol))
			if err != nil || p == api.TargetTypeNoop {
				return fmt.Errorf("sync %q: unknown protocol %q", sp.Name, sp.Protocol)
			}
			sp.Protocol = p
		}
		if sp.Mode != "" {
			m, err := ParseSyncMode(string(sp.Mode))
			if err != nil {
				return fmt.Errorf("sync %q: %w", sp.Name, err)
			}
			sp.Mode = m
		}
		if sp.Protocol != api.TargetTypeGNMI {
			continue
		}
		if sp.Mode == "" {
			sp.Mode = SyncModeOnChange
		}
		if sp.Encoding == "" && sbi != nil && sbi.GnmiOptions != nil {
			sp.Encoding = sbi.GnmiOptions.Encoding
		}
		if sp.Encoding == "" {
			return fmt.Errorf("sync %q: no encoding defined", sp.Name)
		}
		enc, err := api.ParseGNMIEncoding(string(sp.Encoding))
		if err != nil {
			return fmt.Errorf("sync %q: %w", sp.Name, err)
		}
		sp.Encoding = enc
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
	if c.Type == "" {
		c.Type = defaultCacheType
	}
	t, err := ParseCacheType(string(c.Type))
	if err != nil {
		return err
	}
	c.Type = t
	switch c.Type {
	case CacheTypeRemote:
		if c.Address == "" {
			c.Address = defaultRemoteCacheAddress
		}
		if _, _, err := net.SplitHostPort(c.Address); err != nil {
			return err
		}
	default:
		if c.StoreType == "" {
			c.StoreType = defaultStoreType
		}
		if c.Dir == "" {
			c.Dir = defaultCacheDir
		}
	}
	return nil
}

func (o *SBINetconfOptions) validateSetDefaults() error {
	if o.CommitDatastore == "" {
		o.CommitDatastore = NcCommitDatastoreCandidate
	} else {
		d, err := ParseCommitDatastore(string(o.CommitDatastore))
		if err != nil {
			return err
		}
		o.CommitDatastore = d
	}
	if o.PreferredNCVersion != "" {
		v, err := ParseNCVersion(string(o.PreferredNCVersion))
		if err != nil {
			return err
		}
		o.PreferredNCVersion = v
	}
	return nil
}
