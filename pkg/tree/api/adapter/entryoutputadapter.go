package adapter

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type EntryOutputAdapter struct {
	entry api.Entry
}

func NewEntryOutputAdapter(e api.Entry) *EntryOutputAdapter {
	return &EntryOutputAdapter{
		entry: e,
	}
}

func (t *EntryOutputAdapter) ToJson(ctx context.Context, onlyNewOrUpdated bool) (any, error) {
	return ops.ToJson(ctx, t.entry, onlyNewOrUpdated)
}

func (t *EntryOutputAdapter) ToJsonIETF(ctx context.Context, onlyNewOrUpdated bool) (any, error) {
	return ops.ToJsonIETF(ctx, t.entry, onlyNewOrUpdated)
}

func (t *EntryOutputAdapter) ToXML(ctx context.Context, onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (*etree.Document, error) {
	return ops.ToXML(ctx, t.entry, onlyNewOrUpdated, honorNamespace, operationWithNamespace, useOperationRemove)
}

func (t *EntryOutputAdapter) ToProtoUpdates(ctx context.Context, onlyNewOrUpdated bool) ([]*sdcpb.Update, error) {
	return ops.ToProtoUpdates(ctx, t.entry, onlyNewOrUpdated)
}

func (t *EntryOutputAdapter) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	return ops.ToProtoDeletes(ctx, t.entry)
}

func (t *EntryOutputAdapter) ContainsChanges(ctx context.Context) (bool, error) {
	// TODO: needs to be implemented properly, for now we assume it contains changes
	return true, nil
}
