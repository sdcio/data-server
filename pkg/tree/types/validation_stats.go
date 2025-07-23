package types

import (
	"fmt"
	"strings"
)

type StatType int

const (
	StatTypeMandatory StatType = iota
	StatTypeMustStatement
	StatTypeMinMax
	StatTypeRange
	StatTypePattern
	StatTypeLength
	StatTypeLeafRef
	StatTypeMaxElements
	StatTypeEnums
)

func (s StatType) String() string {
	switch s {
	case StatTypeMandatory:
		return "mandatory"
	case StatTypeMustStatement:
		return "must-statement"
	case StatTypeMinMax:
		return "min/max"
	case StatTypeRange:
		return "range"
	case StatTypePattern:
		return "pattern"
	case StatTypeLength:
		return "length"
	case StatTypeLeafRef:
		return "leafref"
	case StatTypeMaxElements:
		return "max-elements"
	case StatTypeEnums:
		return "enums"
	}
	return ""
}

type ValidationStat struct {
	statType StatType
	count    uint
}

func NewValidationStat(statType StatType) *ValidationStat {
	return &ValidationStat{
		statType: statType,
	}
}

func (v *ValidationStat) PlusOne() *ValidationStat {
	v.count++
	return v
}

func (v *ValidationStat) Set(count uint) *ValidationStat {
	v.count = count
	return v
}

type ValidationStatOverall struct {
	counter map[StatType]uint
}

func NewValidationStatOverall() *ValidationStatOverall {
	return &ValidationStatOverall{
		counter: map[StatType]uint{},
	}
}

func (v *ValidationStatOverall) String() string {
	result := make([]string, 0, len(v.counter))
	for typ, count := range v.counter {
		result = append(result, fmt.Sprintf("%s: %d", typ.String(), count))
	}
	return strings.Join(result, ", ")
}

func (v *ValidationStatOverall) MergeStat(vs *ValidationStat) {
	v.counter[vs.statType] = v.counter[vs.statType] + vs.count
}

func (v *ValidationStatOverall) GetCounter() map[StatType]uint {
	return v.counter
}
