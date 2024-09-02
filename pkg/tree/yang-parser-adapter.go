package tree

import (
	"context"
	"fmt"

	schema_server "github.com/sdcio/sdc-protos/sdcpb"
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

func (y *yangParserEntryAdapter) GetValue() (xpath.Datum, error) {
	if !y.e.remainsToExist() {
		return xpath.NewNodesetDatum([]xutils.XpathNode{}), nil
	}

	if y.e.GetSchema().GetContainer() != nil {
		return xpath.NewBoolDatum(true), nil
	}

	lvs := LeafVariantSlice{}
	lvs = y.e.GetHighestPrecedence(lvs, false)

	tv, err := lvs[0].Value()
	if err != nil {
		return nil, err
	}

	var result xpath.Datum
	switch tv.Value.(type) {
	case *schema_server.TypedValue_BoolVal:
		result = xpath.NewBoolDatum(tv.GetBoolVal())
	default:
		result = xpath.NewLiteralDatum(tv.GetStringVal())
	}
	return result, nil
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

func (y *yangParserValueEntry) Navigate(p []string) (xpath.Entry, error) {
	return nil, fmt.Errorf("yangParserValueEntry navigation impossible")
}

func (y *yangParserValueEntry) GetValue() (xpath.Datum, error) {
	return y.d, nil
}
