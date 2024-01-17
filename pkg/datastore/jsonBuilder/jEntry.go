package jsonbuilder

import (
	"encoding/json"
	"errors"
	"fmt"
)

type EntryType string

const (
	ETArray    EntryType = "array"
	ETMap      EntryType = "map"
	ETString   EntryType = "string"
	ETNumber   EntryType = "number"
	ETLeafList EntryType = "leaflist"
)

type JEntry struct {
	etype     EntryType
	name      string
	arrayVal  []*JEntry
	mapVal    map[string]*JEntry
	stringVal string
}

var (
	ErrNotMap    = errors.New("JEntry type is not map")
	ErrNotString = errors.New("JEntry type is not string")
	ErrNotArray  = errors.New("JEntry type is not array")
	ErrNotFound  = errors.New("not found")
)

// NewJEntryArray initializes as Array typed JEntry
func NewJEntryArray(name string) *JEntry {
	return &JEntry{
		etype:    ETArray,
		arrayVal: []*JEntry{},
		name:     name,
	}
}

// NewJEntryMap initializes as Map typed JEntry
func NewJEntryMap(name string) *JEntry {
	return &JEntry{
		etype:  ETMap,
		mapVal: map[string]*JEntry{},
		name:   name,
	}
}

// NewJEntryString initializes as String typed JEntry
func NewJEntryString(s string) *JEntry {
	return &JEntry{
		etype:     ETString,
		stringVal: s,
		name:      s,
	}
}

// GetType returns the underlaying type of the JEntry
func (j *JEntry) GetType() EntryType {
	return j.etype
}

// GetMap returns the Map vlaue
func (j *JEntry) GetMap() map[string]*JEntry {
	return j.mapVal
}

// GetArray returns the Array value
func (j *JEntry) GetArray() []*JEntry {
	return j.arrayVal
}

// GetString returns the String value
func (j *JEntry) GetString() string {
	return j.stringVal
}

func (j *JEntry) GetSubElem(name string, key map[string]string, create bool, nextKeys map[string]string, lastElem bool) (*JEntry, error) {

	switch j.etype {
	case ETArray:
		return j.getSubElemArray(name, key, create, nextKeys, lastElem)
	case ETMap:
		return j.GetSubElemMap(name, key, create, nextKeys, lastElem)
	}
	return nil, fmt.Errorf("ERROR")
}

func (j *JEntry) GetSubElemMap(name string, key map[string]string, create bool, nextKeys map[string]string, lastElem bool) (*JEntry, error) {
	val, exists := j.mapVal[name]
	if exists {
		if len(key) == 0 {
			return val, nil
		}
	OUTER:
		for _, elem := range val.arrayVal {
			for k, v := range key {
				if !(elem.mapVal[k].etype == ETString && elem.mapVal[k].GetString() == v) {
					// we can check the next Array entry
					continue OUTER
				}
			}
			return elem, nil
		}
	}
	if !create {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	var newEntry *JEntry
	if len(key) > 0 {
		var arr *JEntry
		if exists {
			arr = val
		} else {
			arr = NewJEntryArray(name)
			j.mapVal[name] = arr
		}
		newEntry = NewJEntryMap("Item")
		for k, v := range key {
			newEntry.mapVal[k] = NewJEntryString(v)
		}
		arr.arrayVal = append(arr.arrayVal, newEntry)
		return newEntry, nil
	}
	if lastElem {
		return j, nil
	}

	newEntry = NewJEntryMap(name)
	j.mapVal[name] = newEntry

	return newEntry, nil
}

// getsubElemArray is the GetSubElem implementation for the Array type
func (j *JEntry) getSubElemArray(name string, key map[string]string, create bool, nextKeys map[string]string, lastElem bool) (*JEntry, error) {
OUTER:
	for _, elem := range j.arrayVal {
		for k, v := range key {
			if elem.mapVal[k].etype != ETString && elem.mapVal[k].GetString() != v {
				// we can check the next Array entry
				continue OUTER
			}
		}
		// return the found element
		return elem, nil
	}
	// if create is not set,
	if !create {
		return nil, fmt.Errorf("Not Found")
	}
	var newEntry *JEntry
	if create {
		_ = ""
	}

	return newEntry, nil
}

// AddMapEntry adds the given JEntry to the Map
// returns an error if this JEntry is not of the type ETArray
func (j *JEntry) AddToMap(key string, je *JEntry) error {
	if err := j.AssureMap(); err != nil {
		return err
	}
	j.mapVal[key] = je
	return nil
}

// AddArrayEntry adds the given JEntry to the Array
// returns an error if this JEntry is not of the type ETArray
func (j *JEntry) AddToArray(je ...*JEntry) error {
	if err := j.AssureArray(); err != nil {
		return err
	}
	j.arrayVal = append(j.arrayVal, je...)
	return nil
}

// IsMap returns true if the JEntry
// is of type ETMap
func (j *JEntry) IsMap() bool {
	return j.etype == ETMap
}

// AssureMap returns an error if this JEntry is not of type ETMap
func (j *JEntry) AssureMap() error {
	if j.etype != ETMap {
		return fmt.Errorf("%w: %v", ErrNotMap, j)
	}
	return nil
}

// IsArray returns true if the JEntry
// is of type ETArray
func (j *JEntry) IsArray() bool {
	return j.etype == ETArray
}

// AssureArray returns an error if this JEntry is not of type ETArray
func (j *JEntry) AssureArray() error {
	if j.etype != ETArray {
		return fmt.Errorf("%w: %v", ErrNotArray, j)
	}
	return nil
}

// IsString returns true if the JEntry
// is of type ETString
func (j *JEntry) IsString() bool {
	return j.etype == ETString
}

// AssureString returns an error if this JEntry is not of type ETString
func (j *JEntry) AssureString() error {
	if j.etype != ETString {
		return fmt.Errorf("%w: %v", ErrNotString, j)
	}
	return nil
}

func (j *JEntry) String() string {
	return fmt.Sprintf("%s", j.etype)
}

// MarshalJSON custom marshaller, that returns the JEntry in the
// correct form (Array, Map or String value)
func (j *JEntry) MarshalJSON() ([]byte, error) {
	switch j.etype {
	case ETString:
		return json.Marshal(j.stringVal)
	case ETMap:
		return json.Marshal(j.mapVal)
	case ETArray:
		return json.Marshal(j.arrayVal)
	}
	return []byte{}, nil
}
