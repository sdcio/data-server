package config

type SchemaConfig struct {
	Name        string   `json:"name,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Version     string   `json:"version,omitempty"`
	Files       []string `json:"files,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
}
