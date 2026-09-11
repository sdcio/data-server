package adapter

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/ops"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// IntentResponseAdapter wraps an api.Entry as a GetIntentResponse for GetIntent
// handling. It carries intent metadata (name, priority, orphan and non-revertive
// flags, explicit deletes) alongside the rendered tree. RenderOpts controls
// sensitive-path redaction and is set by datastore.GetIntent: running uses the
// cross-intent path-marker union; a regular intent uses that intent's own
// markers only. Schema-defined sensitivity from the YANG extension is still
// honored unconditionally via ShouldRedact.
type IntentResponseAdapter struct {
	Entry           api.Entry
	IntentName      string
	Priority        int32
	Orphan          bool
	NonRevertive    bool
	ExplicitDeletes []*sdcpb.Path
	RenderOpts      ops.RenderOpts
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
	return ops.ToJson(ctx, t.Entry, t.RenderOpts)
}

func (t *IntentResponseAdapter) ToJsonIETF(ctx context.Context) (any, error) {
	return ops.ToJsonIETF(ctx, t.Entry, t.RenderOpts)
}

func (t *IntentResponseAdapter) ToXML(ctx context.Context) (*etree.Document, error) {
	return ops.ToXML(ctx, t.Entry, ops.XMLRenderOpts{
		RenderOpts:     t.RenderOpts,
		HonorNamespace: true,
	})
}

func (t *IntentResponseAdapter) ToProtoUpdates(ctx context.Context) ([]*sdcpb.Update, error) {
	return ops.ToProtoUpdates(ctx, t.Entry, t.RenderOpts)
}

func (t *IntentResponseAdapter) ToProtoDeletes(ctx context.Context) ([]*sdcpb.Path, error) {
	return ops.ToProtoDeletes(ctx, t.Entry)
}

func (t *IntentResponseAdapter) ToXPath(ctx context.Context) (*sdcpb.PathValues, error) {
	return ops.ToXPath(ctx, t.Entry, ops.XPathRenderOpts{RenderOpts: t.RenderOpts})
}
