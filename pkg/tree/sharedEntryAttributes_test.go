package tree

import (
	"context"
	"fmt"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	"go.uber.org/mock/gomock"
)

func Test_sharedEntryAttributes_checkAndCreateKeysAsLeafs(t *testing.T) {
	controller := gomock.NewController(t)
	defer controller.Finish()

	sc, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	scb := schemaClient.NewSchemaClientBound(schema, sc)

	tc := NewTreeContext(scb, "intent1")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	flags := types.NewUpdateInsertFlags()
	flags.SetNewFlag()

	prio := int32(5)
	intentName := "intent1"

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate(types.PathSlice{"interface", "ethernet-1/1", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	_, err = root.AddUpdateRecursive(ctx, types.NewUpdate([]string{"doublekey", "k1.1", "k1.3", "mandato"}, testhelper.GetStringTvProto(t, "TheMandatoryValue1"), prio, intentName, 0), flags)
	if err != nil {
		t.Error(err)
	}

	t.Log(root.String())

	fmt.Println(root.String())
	err = root.FinishInsertionPhase(ctx)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(root.String())

	// TODO: check the result
}
