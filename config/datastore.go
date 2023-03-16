package config

type DatastoreConfig struct {
	Name   string        `yaml:"name,omitempty" json:"name,omitempty"`
	Schema *SchemaConfig `yaml:"schema,omitempty" json:"schema,omitempty"`
	SBI    *SBI          `yaml:"sbi,omitempty" json:"sbi,omitempty"`
	Sync   *Sync         `yaml:"sync,omitempty" json:"sync,omitempty"`
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

type Sync struct {
	Validate bool
	// GNMI     *[]GNMISync
	// NATS     *NATSSync
}

// type GNMISync struct {
// 	Name           string
// 	Paths          []string
// 	Mode           string
// 	SampleInterval time.Duration
// 	Encoding       string
// }

// type NATSSync struct {
// 	Address string
// 	Subject string
// }
