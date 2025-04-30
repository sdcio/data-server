package tree

import (
	"context"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/xutils"
)

type yangParserEntryAdapter struct {
	e   Entry
	ctx context.Context
}

func newYangParserEntryAdapter(ctx context.Context, e Entry) *yangParserEntryAdapter {
	return &yangParserEntryAdapter{
		e:   e,
		ctx: ctx,
	}
}

func (y *yangParserEntryAdapter) Copy() xpath.Entry {
	return newYangParserEntryAdapter(y.ctx, y.e)
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
		return xpath.NewBoolDatum(true), nil
	}

	// if y.e is a container
	if cs := y.e.GetSchema().GetContainer(); cs != nil {
		// its a container
		if len(cs.Keys) == 0 {
			// regular container
			return xpath.NewBoolDatum(true), nil
		}
		// list
		childs, err := y.e.GetListChilds()
		if err != nil {
			return nil, err
		}
		datums := make([]xutils.XpathNode, 0, len(childs))
		for x := 0; x < len(childs); x++ {
			// this is a dirty fix, that will enable count() to evaluate the right value
			datums = append(datums, nil)
		}
		return xpath.NewNodesetDatum(datums), nil
	}

	// if y.e is anything else then a container
	lv, _ := y.e.getHighestPrecedenceLeafValue(y.ctx)
	if lv == nil {
		return xpath.NewNodesetDatum([]xutils.XpathNode{}), nil
	}
	return y.valueToDatum(lv.Value()), nil
}

func (y *yangParserEntryAdapter) BreadthSearch(ctx context.Context, path string) ([]xpath.Entry, error) {
	entries, err := y.e.BreadthSearch(ctx, path)
	if err != nil {
		return nil, err
	}

	result := make([]xpath.Entry, 0, len(entries))
	for _, x := range entries {
		result = append(result, newYangParserEntryAdapter(ctx, x))
	}

	return result, nil
}

func (y *yangParserEntryAdapter) FollowLeafRef() (xpath.Entry, error) {
	entries, err := y.e.NavigateLeafRef(y.ctx)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("error resolving leafref for %s", y.e.Path())
	}

	return newYangParserEntryAdapter(y.ctx, entries[0]), nil
}

func (y *yangParserEntryAdapter) GetPath() []string {
	return y.e.Path()
}

func (y *yangParserEntryAdapter) Navigate(p []string) (xpath.Entry, error) {
	var err error
	var rootPath = false

	if len(p) == 0 {
		return y, nil
	}

	// if the path slice starts with a / then it is a root based path.
	if p[0] == "/" {
		p = p[1:]
		rootPath = true
	}

	lookedUpEntry, err := y.e.Navigate(y.ctx, p, rootPath, true)
	if err != nil {
		return newYangParserValueEntry(xpath.NewNodesetDatum([]xutils.XpathNode{}), err), nil
	}
	return newYangParserEntryAdapter(y.ctx, lookedUpEntry), nil
}

type yangParserValueEntry struct {
	d xpath.Datum
	e error
}

func newYangParserValueEntry(d xpath.Datum, err error) *yangParserValueEntry {
	return &yangParserValueEntry{
		d: d,
		e: err,
	}
}

func (y *yangParserValueEntry) Copy() xpath.Entry {
	return y
}

func (y *yangParserValueEntry) FollowLeafRef() (xpath.Entry, error) {
	return nil, fmt.Errorf("yangParserValueEntry navigation impossible")
}

func (y *yangParserValueEntry) Navigate(p []string) (xpath.Entry, error) {
	return nil, fmt.Errorf("yangParserValueEntry navigation impossible")
}

func (y *yangParserValueEntry) GetValue() (xpath.Datum, error) {
	return y.d, nil
}

func (y *yangParserValueEntry) GetPath() []string {
	return nil
}

func (y *yangParserValueEntry) BreadthSearch(ctx context.Context, path string) ([]xpath.Entry, error) {
	return nil, nil
}
