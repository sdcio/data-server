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
	case *sdcpb.TypedValue_StringVal:
		if y.e.GetSchema().GetField().GetType().GetType() == "identityref" {
			idPrefixMap := y.e.GetSchema().GetField().GetType().GetIdentityPrefixesMap()
			if prefix, ok := idPrefixMap[tv.GetStringVal()]; ok {
				return xpath.NewLiteralDatum(fmt.Sprintf("%s:%s", prefix, tv.GetStringVal()))
			}
		}
		return xpath.NewLiteralDatum(tv.GetStringVal())
	case *sdcpb.TypedValue_UintVal:
		return xpath.NewNumDatum(float64(tv.GetUintVal()))
	case *sdcpb.TypedValue_LeaflistVal:
		datums := make([]xpath.Datum, 0, len(ttv.LeaflistVal.GetElement()))
		for _, e := range ttv.LeaflistVal.GetElement() {
			datum := y.valueToDatum(e)
			datums = append(datums, datum)
		}
		return xpath.NewDatumSliceDatum(datums)
	default:
		return xpath.NewLiteralDatum(tv.GetStringVal())
	}
}

func (y *yangParserEntryAdapter) GetValue() (xpath.Datum, error) {
	if y.e.GetSchema().GetContainer() != nil {
		return xpath.NewBoolDatum(true), nil
	}

	lv, err := y.e.getHighestPrecedenceLeafValue(y.ctx)
	if err != nil {
		return nil, err
	}
	if lv == nil {
		return xpath.NewNodesetDatum([]xutils.XpathNode{}), nil
	}
	tv, err := lv.Update.Value()
	if err != nil {
		return nil, err
	}

	return y.valueToDatum(tv), nil
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

	lookedUpEntry := y.e
	for idx, pelem := range p {
		// if we move up, on a .. we should just go up, staying in the branch that represents the instance
		// if there is another .. then we need to forward to the element with the schema and just then forward
		// to the parent. Thereby skipping the key levels that sit inbetween
		if pelem == ".." && lookedUpEntry.GetSchema().GetSchema() == nil {
			lookedUpEntry, _ = lookedUpEntry.GetFirstAncestorWithSchema()
		}

		// rootPath && idx == 0 => means only allow true on first index, for sure false on all other
		lookedUpEntry, err = lookedUpEntry.Navigate(y.ctx, []string{pelem}, rootPath && idx == 0)
		if err != nil {
			return newYangParserValueEntry(xpath.NewNodesetDatum([]xutils.XpathNode{}), err), nil
		}
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
