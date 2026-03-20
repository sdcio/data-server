package adapter

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type IntentResponseAdapter struct {
	entry api.Entry
}

func NewIntentResponseAdapter(e api.Entry) *IntentResponseAdapter {
	return &IntentResponseAdapter{
		entry: e,
	}
}

func (t *IntentResponseAdapter) ToJson(ctx context.Context) (any, error) {
	return ops.ToJson(ctx, t.entry, false)
}

func (t *IntentResponseAdapter) ToJsonIETF(ctx context.Context) (any, error) {
	return ops.ToJsonIETF(ctx, t.entry, false)
}

func (t *IntentResponseAdapter) ToXML(ctx context.Context) (*etree.Document, error) {
	return ops.ToXML(ctx, t.entry, false, true, false, false)
}

func (t *IntentResponseAdapter) ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error) {
	return ops.ToProtoUpdates(ctx, t.entry, false)
}

func (t *IntentResponseAdapter) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	return ops.ToProtoDeletes(ctx, t.entry)
}
