package config

type DatastoreConfig struct {
	Name   string        `yaml:"name,omitempty" json:"name,omitempty"`
	Schema *SchemaConfig `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI    *SBI          `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	// SchemaServer *SchemaServer `yaml:"schema-server,omitempty" json:"schema-server,omitempty"`
}

type SBI struct {
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Address     string `yaml:"address,omitempty" json:"address,omitempty"`
	TLS         *TLS   `yaml:"tls,omitempty" json:"tls,omitempty"`
	Credentials *Creds `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type Creds struct {
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
}
