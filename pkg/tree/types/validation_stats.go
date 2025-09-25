package types

import (
	"fmt"
	"strings"
)

type StatType int

const (
	StatTypeMandatory StatType = iota
	StatTypeMustStatement
	StatTypeMinMaxElementsLeaflist
	StatTypeRange
	StatTypePattern
	StatTypeLength
	StatTypeLeafRef
	StatTypeMinMaxElementsList
	StatTypeMinElements
	StatTypeEnums
)

func (s StatType) String() string {
	switch s {
	case StatTypeMandatory:
		return "mandatory"
	case StatTypeMustStatement:
		return "must-statement"
	case StatTypeMinMaxElementsLeaflist:
		return "min/max elements leaflist"
	case StatTypeRange:
		return "range"
	case StatTypePattern:
		return "pattern"
	case StatTypeLength:
		return "length"
	case StatTypeLeafRef:
		return "leafref"
	case StatTypeMinMaxElementsList:
		return "min/max elements list"
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
	Counter map[string]uint `json:"counters"`
}

func NewValidationStatOverall() *ValidationStatOverall {
	return &ValidationStatOverall{
		Counter: map[string]uint{},
	}
}

func (v *ValidationStatOverall) String() string {
	result := make([]string, 0, len(v.Counter))
	for typ, count := range v.Counter {
		result = append(result, fmt.Sprintf("%s: %d", typ, count))
	}
	return strings.Join(result, ", ")
}

func (v *ValidationStatOverall) MergeStat(vs *ValidationStat) {
	v.Counter[vs.statType.String()] = v.Counter[vs.statType.String()] + vs.count
}

func (v *ValidationStatOverall) GetCounter() map[string]uint {
	return v.Counter
}
