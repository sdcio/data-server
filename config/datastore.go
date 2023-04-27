package config

import (
	"errors"
	"time"
)

const (
	defaultBufferSize = 1000
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
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}

type Sync struct {
	Validate bool        `yaml:"validate,omitempty" json:"validate,omitempty"`
	Buffer   int64       `yaml:"buffer,omitempty" json:"buffer,omitempty"`
	GNMI     []*GNMISync `yaml:"gnmi,omitempty" json:"gnmi,omitempty"`
}

type GNMISync struct {
	Name           string        `yaml:"name,omitempty" json:"name,omitempty"`
	Paths          []string      `yaml:"paths,omitempty" json:"paths,omitempty"`
	Mode           string        `yaml:"mode,omitempty" json:"mode,omitempty"`
	SampleInterval time.Duration `yaml:"sample-interval,omitempty" json:"sample-interval,omitempty"`
	Period         time.Duration `yaml:"period,omitempty" json:"period,omitempty"`
	Encoding       string        `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

func (ds *DatastoreConfig) validateSetDefaults() error {
	var err error
	if err = ds.Schema.validateSetDefaults(); err != nil {
		return err
	}
	if err = ds.SBI.validateSetDefaults(); err != nil {
		return err
	}
	if err = ds.Sync.validateSetDefaults(); err != nil {
		return err
	}
	return nil
}

func (s *SBI) validateSetDefaults() error {
	if s.Address == "" {
		return errors.New("missing SBI address")
	}
	switch s.Type {
	case "gnmi":
		return nil
	case "nc":
		if s.Port == 0 {
			s.Port = 830
		}
		return nil
	default:
		return nil
	}
}

func (s *Sync) validateSetDefaults() error {
	if s == nil {
		return nil
	}
	if s.Buffer <= 0 {
		s.Buffer = defaultBufferSize
	}
	return nil
}
