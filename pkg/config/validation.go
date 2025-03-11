package config

type Validation struct {
	DisabledValidators Validators `yaml:"disabled-validators,omitempty" json:"disabled-validators,omitempty"`
	// ValidateConcurrently DO NOT USE THIS INTERNALLY! It is a user supplied setting and is migrated to Concurrent.
	ValidateConcurrently *bool `yaml:"validate-concurrently,omitempty" json:"validate-concurrently,omitempty"`
	Concurrent           bool
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

const (
	Mandatory               string = "mandatory"
	Leafref                 string = "leafref"
	LeafrefMinMaxAttributes string = "leafref-min-max-attributes"
	Pattern                 string = "pattern"
	MustStatement           string = "must-statement"
	Length                  string = "length"
	Range                   string = "range"
	MaxElements             string = "max-elements"
)

func (v *Validation) validateSetDefaults() error {
	if v.ValidateConcurrently != nil {
		// if given, set the value internally
		v.Concurrent = *v.ValidateConcurrently
	} else {
		// if unset, default true
		v.Concurrent = true
	}
	return nil
}
