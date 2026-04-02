package adapter

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type IntentResponseAdapter struct {
	Entry           api.Entry
	IntentName      string
	Priority        int32
	Orphan          bool
	NonRevertive    bool
	ExplicitDeletes []*sdcpb.Path
}

func (t *IntentResponseAdapter) GetIntentName() string {
	return t.IntentName
}

func (t *IntentResponseAdapter) GetPriority() int32 {
	return t.Priority
}

func (t *IntentResponseAdapter) IsOrphan() bool {
	return t.Orphan
}

func (t *IntentResponseAdapter) IsNonRevertive() bool {
	return t.NonRevertive
}

func (t *IntentResponseAdapter) GetExplicitDeletes() []*sdcpb.Path {
	return t.ExplicitDeletes
}

func (t *IntentResponseAdapter) ToJson(ctx context.Context) (any, error) {
	return ops.ToJson(ctx, t.Entry, false)
}

func (t *IntentResponseAdapter) ToJsonIETF(ctx context.Context) (any, error) {
	return ops.ToJsonIETF(ctx, t.Entry, false)
}

func (t *IntentResponseAdapter) ToXML(ctx context.Context) (*etree.Document, error) {
	return ops.ToXML(ctx, t.Entry, false, true, false, false)
}

func (t *IntentResponseAdapter) ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error) {
	return ops.ToProtoUpdates(ctx, t.Entry, false)
}

func (t *IntentResponseAdapter) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	return ops.ToProtoDeletes(ctx, t.Entry)
}
