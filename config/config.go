package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	GRPCServer *GRPCServer `yaml:"grpc-server,omitempty" json:"grpc-server,omitempty"`
	// Address      string             `yaml:"address,omitempty" json:"address,omitempty"`
	// TLS          *TLS               `yaml:"tls,omitempty" json:"tls,omitempty"`
	Schemas      []*SchemaConfig    `yaml:"schemas,omitempty" json:"schemas,omitempty"`
	Datastores   []*DatastoreConfig `yaml:"datastores,omitempty" json:"datastores,omitempty"`
	SchemaServer *SchemaServer      `yaml:"schema-server,omitempty" json:"schema-server,omitempty"`
	Prometheus   *PromConfig        `yaml:"prometheus,omitempty" json:"prometheus,omitempty"`
}

type TLS struct {
	CA         string `yaml:"ca,omitempty" json:"ca,omitempty"`
	Cert       string `yaml:"cert,omitempty" json:"cert,omitempty"`
	Key        string `yaml:"key,omitempty" json:"key,omitempty"`
	SkipVerify bool   `yaml:"skip-verify,omitempty" json:"skip-verify,omitempty"`
}

func New(file string) (*Config, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = yaml.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}
	err = c.validateSetDefaults()
	return c, err
}

func (c *Config) validateSetDefaults() error {
	if c.GRPCServer == nil {
		return errors.New("grpc-server definition is required")
	}
	if c.GRPCServer.Address == "" {
		c.GRPCServer.Address = ":55000"
	}
	return nil
}

type SchemaServer struct {
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
}

type GRPCServer struct {
	Address      string
	TLS          *TLS
	SchemaServer bool
	DataServer   bool
}
