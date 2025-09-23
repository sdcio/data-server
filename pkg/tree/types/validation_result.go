package types

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// ValidationResults is map[string]*ValidationResultIntent so consider iterating via range
type ValidationResults map[string]*ValidationResultIntent

func (v ValidationResults) AddIntent(intentName string) {
	_, exists := v[intentName]
	if exists {
		return
	}
	v[intentName] = NewValidationResultIntent(intentName)
}

func (v ValidationResults) AddEntry(e *ValidationResultEntry) error {
	r, exists := v[e.intentName]
	if !exists {
		r = NewValidationResultIntent(e.intentName)
		v[e.intentName] = r
	}
	r.AddEntry(e)
	return nil
}

func (v ValidationResults) HasErrors() bool {
	for _, intent := range v {
		if len(intent.errors) > 0 {
			return true
		}
	}
	return false
}

func (v ValidationResults) HasWarnings() bool {
	for _, intent := range v {
		if len(intent.warnings) > 0 {
			return true
		}
	}
	return false
}

func (v ValidationResults) WarningsStr() []string {
	result := []string{}
	for _, intent := range v {
		result = append(result, intent.WarningsString()...)
	}
	return result
}

func (v ValidationResults) ErrorsStr() []string {
	result := []string{}
	for _, intent := range v {
		result = append(result, intent.ErrorsString()...)
	}
	return result
}

func (v ValidationResults) JoinErrors() error {
	var result error

	for _, intent := range v {
		result = errors.Join(result, errors.Join(intent.errors...))
	}
	return result
}

func (v ValidationResults) JoinWarnings() error {
	var result error

	for _, intent := range v {
		result = errors.Join(result, errors.Join(intent.warnings...))
	}
	return result
}

func (v ValidationResults) String() string {
	sb := &strings.Builder{}
	for _, e := range v {
		sb.WriteString(e.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

type ValidationResultIntent struct {
	intentName    string
	errors        []error
	errorsMutex   sync.Mutex
	warnings      []error
	warningsMutex sync.Mutex
}

func NewValidationResultIntent(intentName string) *ValidationResultIntent {
	return &ValidationResultIntent{
		intentName: intentName,
		errors:     []error{},
		warnings:   []error{},
	}
}

func (v *ValidationResultIntent) String() string {
	sb := &strings.Builder{}
	newLine := ""
	for _, e := range v.errors {
		sb.WriteString(fmt.Sprintf("%s%s: %s - %s", newLine, v.intentName, "error", e.Error()))
		newLine = "\n"
	}
	for _, e := range v.warnings {
		sb.WriteString(fmt.Sprintf("%s%s: %s - %s", newLine, v.intentName, "warning", e.Error()))
		newLine = "\n"
	}
	return sb.String()
}

func (v *ValidationResultIntent) AddEntry(x *ValidationResultEntry) {
	switch x.typ {
	case ValidationResultEntryTypeError:
		v.AddError(x.message)
	case ValidationResultEntryTypeWarning:
		v.AddWarning(x.message)
	}
}

func (v *ValidationResultIntent) AddError(err error) {
	v.errorsMutex.Lock()
	defer v.errorsMutex.Unlock()
	v.errors = append(v.errors, err)
}

func (v *ValidationResultIntent) AddWarning(warn error) {
	v.warningsMutex.Lock()
	defer v.warningsMutex.Unlock()
	v.warnings = append(v.warnings, warn)
}

func (v *ValidationResultIntent) Errors() []error {
	v.errorsMutex.Lock()
	defer v.errorsMutex.Unlock()
	return slices.Clone(v.errors)
}

func (v *ValidationResultIntent) ErrorsString() []string {
	v.errorsMutex.Lock()
	defer v.errorsMutex.Unlock()
	result := make([]string, 0, len(v.errors))
	for _, e := range v.errors {
		result = append(result, e.Error())
	}
	return result
}

func (v *ValidationResultIntent) Warnings() []error {
	v.warningsMutex.Lock()
	defer v.warningsMutex.Unlock()
	return slices.Clone(v.warnings)
}

func (v *ValidationResultIntent) WarningsString() []string {
	v.warningsMutex.Lock()
	defer v.warningsMutex.Unlock()
	result := make([]string, 0, len(v.warnings))
	for _, e := range v.warnings {
		result = append(result, e.Error())
	}
	return result
}

type ValidationResultEntry struct {
	intentName string
	message    error
	typ        ValidationResultEntryType
}

func (v *ValidationResultEntry) String() string {
	return fmt.Sprintf("Intent: %s, Type: %s, Message: %s", v.intentName, v.typ.String(), v.message)
}

func NewValidationResultEntry(intentName string, message error, typ ValidationResultEntryType) *ValidationResultEntry {
	return &ValidationResultEntry{
		intentName: intentName,
		message:    message,
		typ:        typ,
	}
}

type ValidationResultEntryType int8

const (
	ValidationResultEntryTypeError ValidationResultEntryType = iota
	ValidationResultEntryTypeWarning
)

func (v ValidationResultEntryType) String() string {
	switch v {
	case ValidationResultEntryTypeError:
		return "ERROR"
	case ValidationResultEntryTypeWarning:
		return "WARNING"
	}
	return "UNKNOWN"
}
