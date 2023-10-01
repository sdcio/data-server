package config

import (
	"errors"
	"net"
	"time"
)

type DatastoreConfig struct {
	Name   string        `yaml:"name,omitempty" json:"name,omitempty"`
	Schema *SchemaConfig `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI    *SBI          `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	Sync   *Sync         `yaml:"sync,omitempty" json:"sync,omitempty"`
}

type SBI struct {
	// Southbound interface type, one of: gnmi, netconf
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// gNMI or netconf address
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	// netconf port
	Port int `yaml:"port,omitempty" json:"port,omitempty"`
	// TLS config
	TLS *TLS `yaml:"tls,omitempty" json:"tls,omitempty"`
	// Target SBI credentials
	Credentials *Creds `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	// if true, the namespace is included as an `xmlns` attribute in the netconf payloads
	IncludeNS bool `yaml:"include-ns,omitempty" json:"include-ns,omitempty"`
	// sets the preferred NC version: 1.0 or 1.1
	PreferredNCVersion string `yaml:"preferred-nc-version,omitempty" json:"preferred-nc-version,omitempty"`
	// ConnectRetry
	ConnectRetry time.Duration `yaml:"connect-retry,omitempty" json:"connect-retry,omitempty"`
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}

type Sync struct {
	Validate     bool        `yaml:"validate,omitempty" json:"validate,omitempty"`
	Buffer       int64       `yaml:"buffer,omitempty" json:"buffer,omitempty"`
	WriteWorkers int64       `yaml:"write-workers,omitempty" json:"write-workers,omitempty"`
	GNMI         []*GNMISync `yaml:"gnmi,omitempty" json:"gnmi,omitempty"`
}

type GNMISync struct {
	Name     string        `yaml:"name,omitempty" json:"name,omitempty"`
	Paths    []string      `yaml:"paths,omitempty" json:"paths,omitempty"`
	Mode     string        `yaml:"mode,omitempty" json:"mode,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty" json:"interval,omitempty"`
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
	return nil
}

func (s *SBI) validateSetDefaults() error {
	if s.Type == "noop" {
		return nil
	}
	if s.Address == "" {
		return errors.New("missing SBI address")
	}
	if s.ConnectRetry < time.Second {
		s.ConnectRetry = time.Second
	}
	switch s.Type {
	case "gnmi":
		return nil
	case "nc":
		if s.Port <= 0 {
			s.Port = defaultNCPort
		}
		return nil
	default:
		return nil
	}
}

func (s *Sync) validateSetDefaults() error {
	if s == nil || len(s.GNMI) == 0 {
		return nil
	}
	if s.Buffer <= 0 {
		s.Buffer = defaultBufferSize
	}
	if s.WriteWorkers <= 0 {
		s.WriteWorkers = defaultWriteWorkers
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
