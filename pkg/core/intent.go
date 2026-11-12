package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sdcio/data-server/pkg/api"
	logf "github.com/sdcio/logger"
	"google.golang.org/protobuf/encoding/protojson"
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
	res.Deletes = append(res.Deletes, rsp.GetExplicitDeletes()...)

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
		res.Updates, err = rsp.ToProtoUpdates(ctx)
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
	return ds.BlameConfig(ctx, includeDefaults)
}
