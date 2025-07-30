package config

func NewValidationConfig() *Validation {
	return &Validation{
		DisabledValidators: &Validators{},
		DisableConcurrency: bool(false),
	}
}

type Validation struct {
	DisabledValidators *Validators `yaml:"disabled-validators,omitempty" json:"disabled-validators,omitempty"`
	DisableConcurrency bool        `yaml:"disable-concurrency,omitempty" json:"disable-concurrency,omitempty"`
}

func (v *Validation) validateSetDefaults() error {
	// no change required, all the bools default to false
	if v.DisabledValidators == nil {
		v.DisabledValidators = &Validators{}
	}
	return nil
}

func (v *Validation) SetDisableConcurrency(b bool) {
	v.DisableConcurrency = b
}

func (v *Validation) DeepCopy() *Validation {
	return &Validation{
		DisabledValidators: v.DisabledValidators.DeepCopy(),
		DisableConcurrency: v.DisableConcurrency,
	}
}

type Validators struct {
	Mandatory               bool `yaml:"mandatory,omitempty" json:"mandatory,omitempty"`
	Leafref                 bool `yaml:"leafref,omitempty" json:"leafref,omitempty"`
	LeafrefMinMaxAttributes bool `yaml:"leafref-min-max-attributes,omitempty" json:"leafref-min-max,omitempty"`
	Pattern                 bool `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	MustStatement           bool `yaml:"must-statement,omitempty" json:"must-statement,omitempty"`
	Length                  bool `yaml:"length,omitempty" json:"length,omitempty"`
	Range                   bool `yaml:"range,omitempty" json:"range,omitempty"`
	MaxElements             bool `yaml:"max-elements,omitempty" json:"max-elements,omitempty"`
}

func (v *Validators) DisableAll() *Validators {
	v.Leafref = true
	v.LeafrefMinMaxAttributes = true
	v.Length = true
	v.Mandatory = true
	v.MaxElements = true
	v.MustStatement = true
	v.Pattern = true
	v.Range = true
	return v
}

func (v *Validators) DeepCopy() *Validators {
	return &Validators{
		Mandatory:               v.Mandatory,
		Leafref:                 v.Leafref,
		LeafrefMinMaxAttributes: v.LeafrefMinMaxAttributes,
		Pattern:                 v.Pattern,
		MustStatement:           v.MustStatement,
		Length:                  v.Length,
		Range:                   v.Range,
		MaxElements:             v.MaxElements,
	}
}
