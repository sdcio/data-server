package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// GetIntent reads an intent back in the requested format. It fails with
// api.ErrInvalidArgument for an empty datastore or intent name or an unknown
// format and api.ErrUnknownDatastore when the datastore does not exist.
func (c *Core) GetIntent(ctx context.Context, req api.IntentRequest) (*api.IntentResult, error) {
	log := logf.FromContext(ctx).WithName("GetIntent").WithValues(
		"intent-datastore", req.Datastore,
		"intent-name", req.Intent,
		"intent-origin", string(req.Origin),
	)
	ctx = logf.IntoContext(ctx, log)
	log.Info("GetIntent", "intent-format", req.Format)

	ds, err := c.getDatastore(req.Datastore)
	if err != nil {
		return nil, err
	}
	if req.Intent == "" {
		return nil, fmt.Errorf("%w: missing intent name", api.ErrInvalidArgument)
	}

	rsp, err := ds.GetIntent(ctx, req.Intent)
	if err != nil {
		return nil, err
	}

	res := &api.IntentResult{
		Name:         req.Intent,
		Priority:     rsp.GetPriority(),
		Orphan:       rsp.IsOrphan(),
		NonRevertive: rsp.IsNonRevertive(),
		Format:       req.Format,
	}
	for _, d := range rsp.GetExplicitDeletes() {
		res.Deletes = append(res.Deletes, d)
	}

	switch req.Format {
	case api.FormatXPath:
		pv, err := rsp.ToXPath(ctx)
		if err != nil {
			return nil, err
		}
		res.Content, err = protojson.Marshal(pv)
		return res, err
	case api.FormatJSON:
		j, err := rsp.ToJson(ctx)
		if err != nil {
			return nil, err
		}
		res.Content, err = json.Marshal(j)
		return res, err
	case api.FormatJSONIETF:
		j, err := rsp.ToJsonIETF(ctx)
		if err != nil {
			return nil, err
		}
		res.Content, err = json.Marshal(j)
		return res, err
	case api.FormatXML:
		doc, err := rsp.ToXML(ctx)
		if err != nil {
			return nil, err
		}
		res.Content, err = doc.WriteToBytes()
		return res, err
	case api.FormatProto:
		upds, err := rsp.ToProtoUpdates(ctx)
		if err != nil {
			return nil, err
		}
		res.Content, err = proto.Marshal(&sdcpb.Intent{Intent: rsp.GetIntentName(), Priority: rsp.GetPriority(), Update: upds})
		return res, err
	}
	return nil, fmt.Errorf("%w: unknown format %d", api.ErrInvalidArgument, req.Format)
}

// BlameConfig returns the blame tree of the datastore: for every value the
// intent that owns it. It fails with api.ErrInvalidArgument for an empty name
// and api.ErrUnknownDatastore when the datastore does not exist.
func (c *Core) BlameConfig(ctx context.Context, datastoreName string, includeDefaults bool) (*api.BlameNode, error) {
	ctx = logf.IntoContext(ctx, logf.FromContext(ctx).WithName("BlameConfig"))
	ds, err := c.getDatastore(datastoreName)
	if err != nil {
		return nil, err
	}
	tree, err := ds.BlameConfig(ctx, includeDefaults)
	if err != nil {
		return nil, err
	}
	return toBlameNode(tree), nil
}

func toBlameNode(e *sdcpb.BlameTreeElement) *api.BlameNode {
	if e == nil {
		return nil
	}
	n := &api.BlameNode{
		Name:           e.GetName(),
		Owner:          e.GetOwner(),
		Value:          e.GetValue(),
		DeviationValue: e.GetDeviationValue(),
		KeyName:        e.GetKeyName(),
	}
	for _, ch := range e.GetChilds() {
		n.Children = append(n.Children, toBlameNode(ch))
	}
	return n
}
