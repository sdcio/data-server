package tree

import (
	"context"
	"encoding/json"
	"runtime"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/pool"

	"github.com/sdcio/data-server/pkg/tree/importer"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type RootTreeImport interface {
	ImportConfig(ctx context.Context, basePath *sdcpb.Path, importer importer.ImportConfigAdapter, flags *types.UpdateInsertFlags, poolFactory pool.VirtualPoolFactory) (*types.ImportStats, error)
}

func loadYgotStructIntoTreeRoot(ctx context.Context, gs ygot.GoStruct, root *RootEntry, owner string, prio int32, nonRevertive bool, flags *types.UpdateInsertFlags) (*types.ImportStats, error) {
	jconfStr, err := ygot.EmitJSON(gs, &ygot.EmitJSONConfig{
		Format:         ygot.RFC7951,
		SkipValidation: true,
	})
	if err != nil {
		return nil, err
	}

	var jsonConfAny any
	err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
	if err != nil {
		return nil, err
	}

	stp := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	importProcessor := NewImportConfigProcessor(jsonImporter.NewJsonTreeImporter(jsonConfAny, owner, prio, nonRevertive), flags)
	err = importProcessor.Run(ctx, root.sharedEntryAttributes, stp)

	if err != nil {
		return nil, err
	}
	return importProcessor.GetStats(), nil
}
