package config

import (
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type SchemaConfig struct {
	Name        string   `json:"name,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Version     string   `json:"version,omitempty"`
	Files       []string `json:"files,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
}

func (sc *SchemaConfig) GetSchema() *schemapb.Schema {
	return &schemapb.Schema{
		Name:    sc.Name,
		Vendor:  sc.Vendor,
		Version: sc.Version,
	}
}
