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
	// Southbound: always send real values to the device, never redact.
	return ops.ToJson(ctx, t.entry, ops.RenderOpts{OnlyNewOrUpdated: onlyNewOrUpdated, IncludeSensitive: true})
}

func (t *EntryOutputAdapter) ToJsonIETF(ctx context.Context, onlyNewOrUpdated bool) (any, error) {
	// Southbound: always send real values to the device, never redact.
	return ops.ToJsonIETF(ctx, t.entry, ops.RenderOpts{OnlyNewOrUpdated: onlyNewOrUpdated, IncludeSensitive: true})
}

func (t *EntryOutputAdapter) ToXML(ctx context.Context, onlyNewOrUpdated bool, honorNamespace bool, operationWithNamespace bool, useOperationRemove bool) (*etree.Document, error) {
	// Southbound: always send real values to the device, never redact.
	return ops.ToXML(ctx, t.entry, ops.XMLRenderOpts{
		RenderOpts:             ops.RenderOpts{OnlyNewOrUpdated: onlyNewOrUpdated, IncludeSensitive: true},
		HonorNamespace:         honorNamespace,
		OperationWithNamespace: operationWithNamespace,
		UseOperationRemove:     useOperationRemove,
	})
}

func (t *EntryOutputAdapter) ToProtoUpdates(ctx context.Context, onlyNewOrUpdated bool) ([]*sdcpb.Update, error) {
	// Southbound: always send real values to the device, never redact.
	return ops.ToProtoUpdates(ctx, t.entry, ops.RenderOpts{OnlyNewOrUpdated: onlyNewOrUpdated, IncludeSensitive: true})
}

func (t *EntryOutputAdapter) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	return ops.ToProtoDeletes(ctx, t.entry)
}

func (t *EntryOutputAdapter) ContainsChanges(ctx context.Context) (bool, error) {
	// TODO: needs to be implemented properly, for now we assume it contains changes
	return true, nil
}
