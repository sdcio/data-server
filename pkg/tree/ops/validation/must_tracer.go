package validation

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
)

// TraceEventKind values emitted by the data-server must validator and entry
// adapter alongside yang-parser VM events.
const (
	TraceEventMustProgram          xpath.TraceEventKind = "must_program"
	TraceEventAdapterNavigate      xpath.TraceEventKind = "adapter_navigate"
	TraceEventAdapterGetValue      xpath.TraceEventKind = "adapter_get_value"
	TraceEventAdapterBreadthSearch xpath.TraceEventKind = "adapter_breadth_search"
	TraceEventAdapterFollowLeafRef xpath.TraceEventKind = "adapter_follow_leafref"
)

// MustTraceStatus classifies the terminal outcome of a single must check.
type MustTraceStatus string

const (
	MustTraceStatusPassed         MustTraceStatus = "passed"
	MustTraceStatusFailed         MustTraceStatus = "failed"
	MustTraceStatusCompileError   MustTraceStatus = "compile_error"
	MustTraceStatusRuntimeError   MustTraceStatus = "runtime_error"
	MustTraceStatusStackUnderflow MustTraceStatus = "stack_underflow_ignored"
	MustTraceStatusSkipped        MustTraceStatus = "skipped"
)

// MustTraceMeta describes the tree entry and schema must statement for one
// evaluation check.
type MustTraceMeta struct {
	Path         string `json:"path"`
	KeylessPath  string `json:"keyless_path"`
	SchemaKind   string `json:"schema_kind"`
	Owner        string `json:"owner,omitempty"`
	Value        string `json:"value,omitempty"`
	Statement    string `json:"statement"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// MustTraceOutcome records the terminal status of a traced must check.
type MustTraceOutcome struct {
	Status     MustTraceStatus `json:"status"`
	Result     *bool           `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	SkipReason string          `json:"skip_reason,omitempty"`
}

// MustTracer selects which must-statement checks to trace and returns an
// isolated per-evaluation sink safe for concurrent validation workers.
// Returning nil from BeginMustTrace skips tracing for that evaluation.
type MustTracer interface {
	BeginMustTrace(meta MustTraceMeta) MustEvalTracer
}

// MustEvalTracer receives ordered VM and adapter events for a single must
// check and finalizes the trace on End.
type MustEvalTracer interface {
	xpath.Tracer
	End(outcome MustTraceOutcome)
}

// ValidateOption configures optional runtime hooks for Validate and
// NewValidateProcessorConfig.
type ValidateOption func(*validateOptions)

type validateOptions struct {
	mustTracer MustTracer
}

// WithMustTracer attaches mt to record must-statement evaluations during
// validation. Passing nil leaves tracing disabled.
func WithMustTracer(mt MustTracer) ValidateOption {
	return func(o *validateOptions) {
		o.mustTracer = mt
	}
}

func resolveValidateOptions(opts []ValidateOption) validateOptions {
	var cfg validateOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func formatEntryXPath(p *sdcpb.Path, keyless bool) string {
	if p == nil {
		return ""
	}
	s := p.ToXPath(keyless)
	if s == "" {
		return "/"
	}
	return s
}

func entryOwner(e api.Entry) string {
	if e == nil {
		return "unknown"
	}
	lvs := e.GetLeafVariants()
	if lvs == nil || lvs.Length() == 0 {
		return "unknown"
	}
	if highest := lvs.GetHighestPrecedence(false, false, false); highest != nil {
		return highest.Owner()
	}
	if highest := lvs.GetHighestPrecedence(false, true, true); highest != nil {
		return highest.Owner()
	}
	return "unknown"
}

func extractEntryMusts(e api.Entry) ([]*sdcpb.MustStatement, string) {
	if e == nil || e.GetSchema() == nil {
		return nil, ""
	}
	switch schem := e.GetSchema().GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		if len(schem.Container.GetKeys()) > 0 {
			return schem.Container.GetMustStatements(), "list"
		}
		return schem.Container.GetMustStatements(), "container"
	case *sdcpb.SchemaElem_Leaflist:
		return schem.Leaflist.GetMustStatements(), "leaflist"
	case *sdcpb.SchemaElem_Field:
		return schem.Field.GetMustStatements(), "leaf"
	default:
		return nil, ""
	}
}

func buildMustTraceMeta(e api.Entry, schemaKind string, must *sdcpb.MustStatement) MustTraceMeta {
	var owner, valStr string
	if lvs := e.GetLeafVariants(); lvs != nil && lvs.Length() > 0 {
		if lv := lvs.GetHighestPrecedence(false, true, false); lv != nil {
			owner = lv.Owner()
			if lv.Value() != nil {
				valStr = lv.Value().ToString()
			}
		} else if lv := lvs.GetHighestPrecedence(false, true, true); lv != nil {
			owner = lv.Owner()
		}
	}

	p := e.SdcpbPath()
	return MustTraceMeta{
		Path:         formatEntryXPath(p, false),
		KeylessPath:  formatEntryXPath(p, true),
		SchemaKind:   schemaKind,
		Owner:        owner,
		Value:        valStr,
		Statement:    must.GetStatement(),
		ErrorMessage: must.GetError(),
	}
}

func traceSkippedEntryMusts(e api.Entry, mt MustTracer, reason string) {
	if mt == nil || e == nil {
		return
	}
	musts, schemaKind := extractEntryMusts(e)
	for _, must := range musts {
		if et := mt.BeginMustTrace(buildMustTraceMeta(e, schemaKind, must)); et != nil {
			et.End(MustTraceOutcome{
				Status:     MustTraceStatusSkipped,
				SkipReason: reason,
			})
		}
	}
}

func traceSkippedSubtreeMusts(e api.Entry, mt MustTracer, reason string) {
	if mt == nil || e == nil {
		return
	}
	traceSkippedEntryMusts(e, mt, reason)
	for _, child := range e.GetChilds(types.DescendMethodAll) {
		traceSkippedSubtreeMusts(child, mt, reason)
	}
}
