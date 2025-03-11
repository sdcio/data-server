package config

type Validation struct {
	DisabledValidators Validators `yaml:"disabled-validators,omitempty" json:"disabled-validators,omitempty"`
	DisableConcurrency bool       `yaml:"disable-concurrency,omitempty" json:"disable-concurrency,omitempty"`
}

type Validators struct {
	Mandatory               bool `yaml:"mandatory,omitempty" json:"mandatory,omitempty"`
	Leafref                 bool `yaml:"leafref,omitempty" json:"leafref,omitempty"`
	LeafrefMinMaxAttributes bool `yaml:"leafref-min-max,omitempty" json:"leafref-min-max,omitempty"`
	Pattern                 bool `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	MustStatement           bool `yaml:"must-statement,omitempty" json:"must-statement,omitempty"`
	Length                  bool `yaml:"length,omitempty" json:"length,omitempty"`
	Range                   bool `yaml:"range,omitempty" json:"range,omitempty"`
	MaxElements             bool `yaml:"max-elements,omitempty" json:"max-elements,omitempty"`
}

func (v *Validation) validateSetDefaults() error {
	return nil
}
