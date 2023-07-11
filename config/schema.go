package config

import (
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

type SchemaConfig struct {
	Name        string   `json:"name,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Version     string   `json:"version,omitempty"`
	Files       []string `json:"files,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
}

func (sc *SchemaConfig) GetSchema() *sdcpb.Schema {
	return &sdcpb.Schema{
		Name:    sc.Name,
		Vendor:  sc.Vendor,
		Version: sc.Version,
	}
}
