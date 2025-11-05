package types

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

var AllStatTypes = []StatType{
	StatTypeMandatory,
	StatTypeMustStatement,
	StatTypeMinMaxElementsLeaflist,
	StatTypeRange,
	StatTypePattern,
	StatTypeLength,
	StatTypeLeafRef,
	StatTypeMinMaxElementsList,
	StatTypeMinElements,
	StatTypeEnums,
}

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

type ValidationStats struct {
	Counter   map[StatType]*uint32 `json:"counters"`
	muCounter *sync.Mutex
}

func NewValidationStats() *ValidationStats {
	result := &ValidationStats{
		Counter:   map[StatType]*uint32{},
		muCounter: &sync.Mutex{},
	}
	for _, t := range AllStatTypes {
		result.Counter[t] = new(uint32)
	}
	return result
}

func (v *ValidationStats) Add(t StatType, i uint32) {
	v.muCounter.Lock()
	defer v.muCounter.Unlock()
	if counter, ok := v.Counter[t]; ok {
		atomic.AddUint32(counter, i)
	}
}

// String returns a string representation of all counters
func (v *ValidationStats) String() string {
	v.muCounter.Lock()
	defer v.muCounter.Unlock()
	result := make([]string, 0, len(v.Counter))
	for typ, count := range v.Counter {
		val := atomic.LoadUint32(count)
		result = append(result, fmt.Sprintf("%s: %d", typ, val))
	}
	return strings.Join(result, ", ")
}

// GetCounter returns a snapshot of the counters as a plain map
func (v *ValidationStats) GetCounter() map[StatType]uint32 {
	v.muCounter.Lock()
	defer v.muCounter.Unlock()
	snapshot := make(map[StatType]uint32, len(v.Counter))
	for typ, count := range v.Counter {
		snapshot[typ] = atomic.LoadUint32(count)
	}
	return snapshot
}
