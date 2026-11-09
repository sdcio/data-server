package grpcserver

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sdcio/data-server/pkg/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func protoUnmarshal(b []byte, m proto.Message) error {
	return proto.Unmarshal(b, m)
}

func datastoreSpecFromCreate(req *sdcpb.CreateDataStoreRequest) (api.DatastoreSpec, error) {
	name := req.GetDatastoreName()
	if len(name) == 0 {
		return api.DatastoreSpec{}, fmt.Errorf("%w: missing datastore name attribute", api.ErrInvalidArgument)
	}
	if len(name) > math.MaxUint16 {
		return api.DatastoreSpec{}, fmt.Errorf("%w: datastore name attribute too long", api.ErrInvalidArgument)
	}
	target, err := targetFromSdcpb(req.GetTarget())
	if err != nil {
		return api.DatastoreSpec{}, err
	}
	spec := api.DatastoreSpec{
		Name:   name,
		Target: target,
		Schema: schemaRefFromSdcpb(req.GetSchema()),
	}
	if req.GetSync() != nil {
		sync, err := syncFromSdcpb(req.GetSync())
		if err != nil {
			return api.DatastoreSpec{}, err
		}
		spec.Sync = sync
	}
	return spec, nil
}

func schemaRefFromSdcpb(s *sdcpb.Schema) api.SchemaRef {
	if s == nil {
		return api.SchemaRef{}
	}
	return api.SchemaRef{Vendor: s.GetVendor(), Version: s.GetVersion()}
}

func syncFromSdcpb(s *sdcpb.Sync) (api.Sync, error) {
	out := api.Sync{
		Validate:     s.GetValidate(),
		Buffer:       s.GetBuffer(),
		WriteWorkers: s.GetWriteWorkers(),
	}
	for _, pSync := range s.GetConfig() {
		tgt, err := targetFromSdcpb(pSync.GetTarget())
		if err != nil {
			return api.Sync{}, err
		}
		out.Configs = append(out.Configs, api.SyncConfig{
			Name:     pSync.GetName(),
			Target:   &tgt,
			Paths:    pSync.GetPath(),
			Mode:     syncModeFromSdcpb(pSync.GetMode()),
			Interval: pSync.GetInterval(),
		})
	}
	return out, nil
}

func syncModeFromSdcpb(m sdcpb.SyncMode) api.SyncMode {
	switch m {
	case sdcpb.SyncMode_SM_SAMPLE:
		return api.SyncModeSample
	case sdcpb.SyncMode_SM_ONCE:
		return api.SyncModeOnce
	case sdcpb.SyncMode_SM_GET:
		return api.SyncModeGet
	default:
		return api.SyncModeOnChange
	}
}

func targetFromSdcpb(t *sdcpb.Target) (api.Target, error) {
	if t == nil {
		return api.Target{}, fmt.Errorf("%w: missing target", api.ErrInvalidArgument)
	}
	out := api.Target{
		Type:    api.TargetType(strings.ToLower(t.GetType())),
		Address: t.GetAddress(),
		Port:    t.GetPort(),
	}
	switch strings.ToLower(t.GetType()) {
	case "netconf":
		nc := t.GetNetconfOpts()
		commit := api.CommitCandidateCandidate
		if nc != nil && nc.GetCommitCandidate() == sdcpb.CommitCandidate_COMMIT_RUNNING {
			commit = api.CommitCandidateRunning
		}
		out.ProtocolOptions.Netconf = &api.NetconfOptions{
			IncludeNs:          nc.GetIncludeNs(),
			OperationWithNs:    nc.GetOperationWithNs(),
			UseOperationRemove: nc.GetUseOperationRemove(),
			CommitCandidate:    commit,
		}
	case "gnmi":
		g := t.GetGnmiOpts()
		out.ProtocolOptions.GNMI = &api.GNMIOptions{Encoding: g.GetEncoding(), TargetName: g.GetTargetName()}
	case "noop":
	default:
		return api.Target{}, fmt.Errorf("%w: unknown targetconnection protocol type %q", api.ErrInvalidArgument, t.GetType())
	}
	if tls := t.GetTls(); tls != nil {
		out.TLS = &api.TLS{CA: tls.GetCa(), Cert: tls.GetCert(), Key: tls.GetKey(), SkipVerify: tls.GetSkipVerify()}
	}
	if creds := t.GetCredentials(); creds != nil {
		out.Credentials = &api.Credentials{Username: creds.GetUsername(), Password: creds.GetPassword(), Token: creds.GetToken()}
	}
	return out, nil
}

func datastoreInfoToSdcpb(info *api.DatastoreInfo) (*sdcpb.GetDataStoreResponse, error) {
	rsp := &sdcpb.GetDataStoreResponse{
		DatastoreName: info.Name,
		Intents:       info.Intents,
		Target: &sdcpb.Target{
			Type:           string(info.Spec.Target.Type),
			Address:        info.Spec.Target.Address,
			Port:           info.Spec.Target.Port,
			Status:         targetStatusToSdcpb(info.ConnectionState),
			StatusDetails:  info.StatusDetails,
		},
		Schema: &sdcpb.Schema{
			Vendor:  info.Spec.Schema.Vendor,
			Version: info.Spec.Schema.Version,
		},
	}
	return rsp, nil
}

func targetStatusToSdcpb(s api.ConnectionState) sdcpb.TargetStatus {
	switch s {
	case api.ConnectionConnected:
		return sdcpb.TargetStatus_CONNECTED
	case api.ConnectionNotConnected:
		return sdcpb.TargetStatus_NOT_CONNECTED
	default:
		return sdcpb.TargetStatus_UNKNOWN
	}
}

func intentFromSdcpb(pb *sdcpb.TransactionIntent) (api.Intent, error) {
	if pb == nil {
		return api.Intent{}, fmt.Errorf("%w: nil intent", api.ErrInvalidArgument)
	}
	in := api.Intent{Name: pb.GetIntent()}
	if pb.GetDelete() {
		in.Spec.Delete = &api.Delete{IgnoreNoExist: pb.GetDeleteIgnoreNoExist(), Orphan: pb.GetOrphan()}
		return in, nil
	}
	u := &api.Update{
		Priority:          pb.GetPriority(),
		NonRevertive:      pb.GetNonRevertive(),
		PreviouslyApplied: pb.GetPreviouslyApplied(),
	}
	for _, d := range pb.GetDeletes() {
		u.Deletes = append(u.Deletes, d.ToXPath(false))
	}
	for _, d := range pb.GetRevertPaths() {
		u.RevertPaths = append(u.RevertPaths, d.ToXPath(false))
	}
	for _, upd := range pb.GetUpdate() {
		path := ""
		if upd.GetPath() != nil {
			path = upd.GetPath().ToXPath(false)
		}
		var content []byte
		if tv := upd.GetValue(); tv != nil {
			content = tv.GetJsonVal()
		}
		u.Entries = append(u.Entries, api.UpdateEntry{Path: path, Content: content})
	}
	in.Spec.Update = u
	return in, nil
}

func transactionResultToSdcpb(res *api.TransactionResult) *sdcpb.TransactionSetResponse {
	if res == nil {
		return &sdcpb.TransactionSetResponse{}
	}
	out := &sdcpb.TransactionSetResponse{Warnings: res.Warnings}
	for _, u := range res.Updates {
		path, _ := sdcpb.ParsePath(u.Path)
		out.Update = append(out.Update, &sdcpb.Update{
			Path:  path,
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: u.Value}},
		})
	}
	for _, d := range res.Deletes {
		if path, err := sdcpb.ParsePath(d); err == nil {
			out.Delete = append(out.Delete, path)
		}
	}
	out.Intents = make(map[string]*sdcpb.TransactionSetResponseIntent, len(res.Intents))
	for name, iv := range res.Intents {
		out.Intents[name] = &sdcpb.TransactionSetResponseIntent{Warnings: iv.Warnings, Errors: iv.Errors}
	}
	return out
}

func intentFormatFromSdcpb(f sdcpb.Format) (api.Format, error) {
	switch f {
	case sdcpb.Format_Intent_Format_JSON:
		return api.FormatJSON, nil
	case sdcpb.Format_Intent_Format_JSON_IETF:
		return api.FormatJSONIETF, nil
	case sdcpb.Format_Intent_Format_XML:
		return api.FormatXML, nil
	case sdcpb.Format_Intent_Format_PROTO:
		return api.FormatProto, nil
	case sdcpb.Format_Intent_Format_XPATH:
		return api.FormatXPath, nil
	default:
		return 0, fmt.Errorf("%w: unknown format", api.ErrInvalidArgument)
	}
}

func intentResultToSdcpb(req *sdcpb.GetIntentRequest, res *api.IntentResult) (*sdcpb.GetIntentResponse, error) {
	out := &sdcpb.GetIntentResponse{
		DatastoreName: req.GetDatastoreName(),
		Format:        req.GetFormat(),
		IntentName:    res.Name,
		Orphan:        res.Orphan,
		Priority:      res.Priority,
		NonRevertive:  res.NonRevertive,
		Deletes:       make([]*sdcpb.Path, 0, len(res.Deletes)),
	}
	for _, d := range res.Deletes {
		if p, err := sdcpb.ParsePath(d); err == nil {
			out.Deletes = append(out.Deletes, p)
		}
	}
	switch req.GetFormat() {
	case sdcpb.Format_Intent_Format_PROTO:
		var intent sdcpb.Intent
		if err := protoUnmarshal(res.Content, &intent); err != nil {
			return nil, err
		}
		out.Intent = &sdcpb.GetIntentResponse_Proto{Proto: &intent}
	default:
		out.Intent = &sdcpb.GetIntentResponse_Blob{Blob: res.Content}
	}
	return out, nil
}

func blameNodeToSdcpb(n *api.BlameNode) *sdcpb.BlameTreeElement {
	if n == nil {
		return nil
	}
	e := &sdcpb.BlameTreeElement{
		Name:           n.Name,
		Owner:          n.Owner,
		Value:          &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: n.Value}},
		DeviationValue: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: n.DeviationValue}},
		KeyName:        n.KeyName,
	}
	for _, ch := range n.Children {
		e.Childs = append(e.Childs, blameNodeToSdcpb(ch))
	}
	return e
}

func schemaDetailsToSdcpb(info *api.SchemaInfo) *sdcpb.GetSchemaDetailsResponse {
	st := sdcpb.SchemaStatus_OK
	switch info.State {
	case api.SchemaReloading:
		st = sdcpb.SchemaStatus_RELOADING
	case api.SchemaInitializing:
		st = sdcpb.SchemaStatus_INITIALIZING
	case api.SchemaFailed:
		st = sdcpb.SchemaStatus_FAILED
	}
	return &sdcpb.GetSchemaDetailsResponse{
		Schema:    &sdcpb.Schema{Vendor: info.Schema.Vendor, Version: info.Schema.Version, Status: st},
		File:      info.Files,
		Directory: info.Directories,
		Exclude:   info.Excludes,
	}
}

func deviationEventToSdcpb(ev api.DeviationEvent) *sdcpb.WatchDeviationResponse {
	rsp := &sdcpb.WatchDeviationResponse{
		Name:   ev.Datastore,
		Intent: ev.Intent,
		Reason: sdcpb.DeviationReason(ev.Reason),
	}
	switch ev.Kind {
	case api.DeviationStart:
		rsp.Event = sdcpb.DeviationEvent_START
	case api.DeviationEnd:
		rsp.Event = sdcpb.DeviationEvent_END
	case api.DeviationClear:
		rsp.Event = sdcpb.DeviationEvent_CLEAR
	default:
		rsp.Event = sdcpb.DeviationEvent_UPDATE
		if len(ev.Entries) > 0 {
			ent := ev.Entries[0]
			if p, err := sdcpb.ParsePath(ent.Path); err == nil {
				rsp.Path = p
			}
			rsp.ExpectedValue = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: ent.Expected}}
			rsp.CurrentValue = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: ent.Current}}
		}
	}
	return rsp
}

func setOptionsFromRequest(req *sdcpb.TransactionSetRequest, origin api.Origin) api.SetOptions {
	opts := api.SetOptions{DryRun: req.GetDryRun(), Origin: origin}
	if req.Timeout != nil {
		opts.Timeout = time.Duration(*req.Timeout) * time.Second
	}
	if req.GetReplaceIntent() != nil {
		if rep, err := intentFromSdcpb(req.GetReplaceIntent()); err == nil {
			opts.Replace = &rep
		}
	}
	return opts
}
