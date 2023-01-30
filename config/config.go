package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

type Config struct {
	GRPCServer   *GRPCServer         `yaml:"grpc-server,omitempty" json:"grpc-server,omitempty"`
	Schemas      []*SchemaConfig     `yaml:"schemas,omitempty" json:"schemas,omitempty"`
	Datastores   []*DatastoreConfig  `yaml:"datastores,omitempty" json:"datastores,omitempty"`
	SchemaServer *RemoteSchemaServer `yaml:"schema-server,omitempty" json:"schema-server,omitempty"`
	Prometheus   *PromConfig         `yaml:"prometheus,omitempty" json:"prometheus,omitempty"`
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
	if c.GRPCServer.MaxRecvMsgSize <= 0 {
		c.GRPCServer.MaxRecvMsgSize = 4 * 1024 * 1024
	}
	return nil
}

type RemoteSchemaServer struct {
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	TLS     *TLS   `yaml:"tls,omitempty" json:"tls,omitempty"`
}

type GRPCServer struct {
	Address        string        `yaml:"address,omitempty" json:"address,omitempty"`
	TLS            *TLS          `yaml:"tls,omitempty" json:"tls,omitempty"`
	SchemaServer   *SchemaServer `yaml:"schema-server,omitempty" json:"schema-server,omitempty"`
	DataServer     *DataServer   `yaml:"data-server,omitempty" json:"data-server,omitempty"`
	MaxRecvMsgSize int           `yaml:"max-recv-msg-size,omitempty" json:"max-recv-msg-size,omitempty"`
}

func (t *TLS) NewConfig(ctx context.Context) (*tls.Config, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: t.SkipVerify}
	if t.CA != "" {
		ca, err := os.ReadFile(t.CA)
		if err != nil {
			return nil, fmt.Errorf("failed to read client CA cert: %w", err)
		}
		if len(ca) != 0 {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(ca)
			tlsCfg.RootCAs = caCertPool
		}
	}

	if t.Cert != "" && t.Key != "" {
		certWatcher, err := certwatcher.New(t.Cert, t.Key)
		if err != nil {
			return nil, err
		}

		go func() {
			if err := certWatcher.Start(ctx); err != nil {
				log.Errorf("certificate watcher error: %v", err)
			}
		}()
		tlsCfg.GetCertificate = certWatcher.GetCertificate
	}
	return tlsCfg, nil
}

type SchemaServer struct {
	Enabled          bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	SchemasDirectory string `yaml:"schemas-directory,omitempty" json:"schemas-directory,omitempty"`
}

type DataServer struct {
	Enabled       bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MaxCandidates int  `yaml:"max-candidates,omitempty" json:"max-candidates,omitempty"`
}
