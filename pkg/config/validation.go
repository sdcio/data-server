package config

type Validation struct {
	DisabledValidators []Validator `yaml:"disabled-validators,omitempty" json:"disabled-validators,omitempty"`
	enabledMap         map[Validator]bool
}

type Validator string

const (
	Mandatory               Validator = "mandatory"
	Leafref                 Validator = "leafref"
	LeafrefMinMaxAttributes Validator = "leafref-min-max-attributes"
	Pattern                 Validator = "pattern"
	MustStatements          Validator = "must-statements"
	Length                  Validator = "length"
	Range                   Validator = "range"
	MaxElements             Validator = "max-elements"
)

var AvailableValidators = [...]Validator{Mandatory, Leafref, LeafrefMinMaxAttributes, Pattern, MustStatements, Length, Range, MaxElements}

func (v *Validation) IsEnabled(validator Validator) bool {
	return v.enabledMap[validator]
}

func (v *Validation) validateSetDefaults() error {
	// Generate enabled map
	v.enabledMap = make(map[Validator]bool, len(AvailableValidators))
	for _, validator := range AvailableValidators {
		v.enabledMap[validator] = true
	}
	for _, validator := range v.DisabledValidators {
		v.enabledMap[Validator(validator)] = false
	}
	return nil
}
