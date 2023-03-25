package config

import (
	"errors"
	"time"
)

type DatastoreConfig struct {
	Name   string        `yaml:"name,omitempty" json:"name,omitempty"`
	Schema *SchemaConfig `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI    *SBI          `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	Sync   *Sync         `yaml:"sync,omitempty" json:"sync,omitempty"`
}

type SBI struct {
	Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	// netconf port
	Port        int    `yaml:"port,omitempty" json:"port,omitempty"`
	TLS         *TLS   `yaml:"tls,omitempty" json:"tls,omitempty"`
	Credentials *Creds `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}

type Sync struct {
	Validate bool        `yaml:"validate,omitempty" json:"validate,omitempty"`
	GNMI     []*GNMISync `yaml:"gnmi,omitempty" json:"gnmi,omitempty"`
	// NATS     *NATSSync
}

type GNMISync struct {
	Name           string        `yaml:"name,omitempty" json:"name,omitempty"`
	Paths          []string      `yaml:"paths,omitempty" json:"paths,omitempty"`
	Mode           string        `yaml:"mode,omitempty" json:"mode,omitempty"`
	SampleInterval time.Duration `yaml:"sample-interval,omitempty" json:"sample-interval,omitempty"`
	Encoding       string        `yaml:"encoding,omitempty" json:"encoding,omitempty"`
}

// type NATSSync struct {
// 	Address string
// 	Subject string
// }

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
	return nil
}
