package validation

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/xutils"
)

const maxTracedBreadthPaths = 64

type yangParserEntryAdapter struct {
	e      api.Entry
	ctx    context.Context
	tracer xpath.Tracer
}

func newYangParserEntryAdapterWithTracer(ctx context.Context, e api.Entry, tracer xpath.Tracer) *yangParserEntryAdapter {
	return &yangParserEntryAdapter{
		e:      e,
		ctx:    ctx,
		tracer: tracer,
	}
}

func (y *yangParserEntryAdapter) pathString() string {
	if y.e == nil {
		return ""
	}
	return formatEntryXPath(y.e.SdcpbPath(), false)
}

func formatNavTarget(p *sdcpb.Path) string {
	if p == nil {
		return ""
	}
	if len(p.GetElem()) == 0 {
		if p.GetIsRootBased() {
			return "/"
		}
		return "."
	}
	return p.ToXPath(false)
}

func (y *yangParserEntryAdapter) Copy() xpath.Entry {
	return newYangParserEntryAdapterWithTracer(y.ctx, y.e, y.tracer)
}

func (y *yangParserEntryAdapter) valueToDatum(tv *sdcpb.TypedValue) xpath.Datum {
	switch ttv := tv.Value.(type) {
	case *sdcpb.TypedValue_BoolVal:
		return xpath.NewBoolDatum(tv.GetBoolVal())
	case *sdcpb.TypedValue_UintVal:
		return xpath.NewNumDatum(float64(tv.GetUintVal()))
	case *sdcpb.TypedValue_LeaflistVal:
		datums := make([]xpath.Datum, 0, len(ttv.LeaflistVal.GetElement()))
		for _, e := range ttv.LeaflistVal.GetElement() {
			datum := y.valueToDatum(e)
			datums = append(datums, datum)
		}
		return xpath.NewDatumSliceDatum(datums)
	case *sdcpb.TypedValue_EmptyVal:
		return xpath.NewBoolDatum(true)
	case *sdcpb.TypedValue_IdentityrefVal:
		return xpath.NewLiteralDatum(tv.GetIdentityrefVal().YangString())
	default:
		return xpath.NewLiteralDatum(tv.GetStringVal())
	}
}

func (y *yangParserEntryAdapter) GetValue() (xpath.Datum, error) {
	if y.e.GetSchema() == nil {
		d := xpath.NewBoolDatum(true)
		y.emitGetValue(d, nil, "no_schema", "")
		return d, nil
	}

	if cs := y.e.GetSchema().GetContainer(); cs != nil {
		if len(cs.Keys) == 0 {
			d := xpath.NewBoolDatum(true)
			y.emitGetValue(d, nil, "container", "")
			return d, nil
		}
		childs, err := ops.GetListChilds(y.e)
		if err != nil {
			y.emitGetValue(nil, err, "list", "")
			return nil, err
		}
		datums := make([]xutils.XpathNode, 0, len(childs))
		for x := 0; x < len(childs); x++ {
			// Placeholder nil nodes preserve list length for count().
			datums = append(datums, nil)
		}
		d := xpath.NewNodesetDatum(datums)
		y.emitGetValue(d, nil, "list", "")
		return d, nil
	}

	lv := y.e.GetLeafVariants().GetHighestPrecedence(false, true, false)
	if lv == nil {
		d := xpath.NewNodesetDatum([]xutils.XpathNode{})
		y.emitGetValue(d, nil, "unset_leaf", "")
		return d, nil
	}
	d := y.valueToDatum(lv.Value())
	y.emitGetValue(d, nil, "leaf_or_leaflist", lv.Owner())
	return d, nil
}

func (y *yangParserEntryAdapter) emitGetValue(d xpath.Datum, err error, entryKind, owner string) {
	if y.tracer == nil {
		return
	}
	details := map[string]any{
		"entry_kind": entryKind,
	}
	if owner != "" {
		details["owner"] = owner
	}
	ev := xpath.TraceEvent{
		Kind:    TraceEventAdapterGetValue,
		Node:    y.pathString(),
		Details: details,
	}
	if d != nil {
		ev.Result = xpath.FormatDatum(d)
	}
	if err != nil {
		ev.Error = err.Error()
	}
	y.tracer.TraceEvent(ev)
}

func (y *yangParserEntryAdapter) BreadthSearch(ctx context.Context, path *sdcpb.Path) ([]xpath.Entry, error) {
	entries, err := breadthSearch(ctx, y.e, path)
	if err != nil {
		if y.tracer != nil {
			y.tracer.TraceEvent(xpath.TraceEvent{
				Kind:  TraceEventAdapterBreadthSearch,
				Node:  y.pathString(),
				Error: err.Error(),
				Details: map[string]any{
					"target": formatNavTarget(path),
				},
			})
		}
		return nil, err
	}

	result := make([]xpath.Entry, 0, len(entries))
	for _, x := range entries {
		result = append(result, newYangParserEntryAdapterWithTracer(ctx, x, y.tracer))
	}

	if y.tracer != nil {
		capLen := min(len(entries), maxTracedBreadthPaths)
		matched := make([]string, 0, capLen)
		for i := range capLen {
			matched = append(matched, formatEntryXPath(entries[i].SdcpbPath(), false))
		}
		details := map[string]any{
			"target":        formatNavTarget(path),
			"match_count":   len(entries),
			"matched_paths": matched,
		}
		if len(entries) > maxTracedBreadthPaths {
			details["truncated"] = true
		}
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind:    TraceEventAdapterBreadthSearch,
			Node:    y.pathString(),
			Details: details,
		})
	}

	return result, nil
}

func (y *yangParserEntryAdapter) FollowLeafRef() (xpath.Entry, error) {
	entries, err := navigateLeafRef(y.ctx, y.e)
	if err != nil {
		if y.tracer != nil {
			y.tracer.TraceEvent(xpath.TraceEvent{
				Kind:  TraceEventAdapterFollowLeafRef,
				Node:  y.pathString(),
				Error: err.Error(),
			})
		}
		return nil, err
	}

	if len(entries) == 0 {
		notFoundErr := fmt.Errorf("error resolving leafref for %s", y.e.SdcpbPath().ToXPath(false))
		if y.tracer != nil {
			y.tracer.TraceEvent(xpath.TraceEvent{
				Kind:  TraceEventAdapterFollowLeafRef,
				Node:  y.pathString(),
				Error: notFoundErr.Error(),
			})
		}
		return nil, notFoundErr
	}

	resolved := newYangParserEntryAdapterWithTracer(y.ctx, entries[0], y.tracer)
	if y.tracer != nil {
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind: TraceEventAdapterFollowLeafRef,
			Node: y.pathString(),
			Details: map[string]any{
				"resolved_path":   resolved.pathString(),
				"candidate_count": len(entries),
			},
		})
	}
	return resolved, nil
}

func (y *yangParserEntryAdapter) GetSdcpbPath() *sdcpb.Path {
	return y.e.SdcpbPath()
}

func (y *yangParserEntryAdapter) Navigate(p *sdcpb.Path) (xpath.Entry, error) {
	if len(p.GetElem()) == 0 {
		if y.tracer != nil {
			y.tracer.TraceEvent(xpath.TraceEvent{
				Kind: TraceEventAdapterNavigate,
				Node: y.pathString(),
				Details: map[string]any{
					"target":        formatNavTarget(p),
					"resolved_path": y.pathString(),
				},
			})
		}
		return y, nil
	}

	entry, err := ops.NavigateSdcpbPath(y.ctx, y.e, p)
	if err != nil {
		if y.tracer != nil {
			y.tracer.TraceEvent(xpath.TraceEvent{
				Kind:  TraceEventAdapterNavigate,
				Node:  y.pathString(),
				Error: err.Error(),
				Details: map[string]any{
					"target":   formatNavTarget(p),
					"fallback": "empty_nodeset",
				},
			})
		}
		return newYangParserValueEntry(xpath.NewNodesetDatum([]xutils.XpathNode{}), err, y.tracer), nil
	}
	resolved := newYangParserEntryAdapterWithTracer(y.ctx, entry, y.tracer)
	if y.tracer != nil {
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind: TraceEventAdapterNavigate,
			Node: y.pathString(),
			Details: map[string]any{
				"target":        formatNavTarget(p),
				"resolved_path": resolved.pathString(),
			},
		})
	}
	return resolved, nil
}

type yangParserValueEntry struct {
	d      xpath.Datum
	e      error
	tracer xpath.Tracer
}

func newYangParserValueEntry(d xpath.Datum, err error, tracer xpath.Tracer) *yangParserValueEntry {
	return &yangParserValueEntry{
		d:      d,
		e:      err,
		tracer: tracer,
	}
}

func (y *yangParserValueEntry) Copy() xpath.Entry {
	return y
}

func (y *yangParserValueEntry) FollowLeafRef() (xpath.Entry, error) {
	err := fmt.Errorf("yangParserValueEntry navigation impossible")
	if y.tracer != nil {
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind:  TraceEventAdapterFollowLeafRef,
			Node:  "<unresolved>",
			Error: err.Error(),
		})
	}
	return nil, err
}

func (y *yangParserValueEntry) Navigate(p *sdcpb.Path) (xpath.Entry, error) {
	err := fmt.Errorf("yangParserValueEntry navigation impossible")
	if y.tracer != nil {
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind:  TraceEventAdapterNavigate,
			Node:  "<unresolved>",
			Error: err.Error(),
			Details: map[string]any{
				"target": formatNavTarget(p),
			},
		})
	}
	return nil, err
}

func (y *yangParserValueEntry) GetValue() (xpath.Datum, error) {
	if y.tracer != nil {
		details := map[string]any{
			"entry_kind": "unresolved_fallback",
		}
		if y.e != nil {
			details["from_error"] = y.e.Error()
		}
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind:    TraceEventAdapterGetValue,
			Node:    "<unresolved>",
			Result:  xpath.FormatDatum(y.d),
			Details: details,
		})
	}
	return y.d, nil
}

func (y *yangParserValueEntry) GetSdcpbPath() *sdcpb.Path {
	return nil
}

func (y *yangParserValueEntry) BreadthSearch(ctx context.Context, path *sdcpb.Path) ([]xpath.Entry, error) {
	if y.tracer != nil {
		y.tracer.TraceEvent(xpath.TraceEvent{
			Kind: TraceEventAdapterBreadthSearch,
			Node: "<unresolved>",
			Details: map[string]any{
				"target":      formatNavTarget(path),
				"match_count": 0,
			},
		})
	}
	return nil, nil
}
