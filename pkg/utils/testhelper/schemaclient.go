package testhelper

import (
	"os"
	"path"
	"runtime"
	"strings"

	dConfig "github.com/sdcio/data-server/pkg/config"
	dataschema "github.com/sdcio/data-server/pkg/schema"
	sConfig "github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store/memstore"
)

var (
	// location of the test schema from the projects root
	SDCIO_SCHEMA_LOCATION = "tests/schema"
)

func InitSDCIOSchema() (dataschema.Client, *dConfig.SchemaConfig, error) {

	// create an in memory schema store
	schemaMemStore := memstore.New()

	// HT: workaround to fixed paths here. Considering all unit tests are executed in pkg, split the paths on pkg.
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	project := strings.Split(dir, "pkg")[0]

	// define the schema config
	sc := &sConfig.SchemaConfig{
		Name:    "testschema",
		Vendor:  "sdcio",
		Version: "v0.0.0",
		Files: []string{
			path.Join(project, SDCIO_SCHEMA_LOCATION),
		},
	}

	dsc := &dConfig.SchemaConfig{
		Name:    sc.Name,
		Vendor:  sc.Vendor,
		Version: sc.Version,
	}

	// init new schema definition to be read by the schema component
	schema, err := schema.NewSchema(sc)
	if err != nil {
		return nil, nil, err
	}

	// add the schema to the in memory schema store
	err = schemaMemStore.AddSchema(schema)
	if err != nil {
		return nil, nil, err
	}

	return dataschema.NewLocalClient(schemaMemStore), dsc, nil
}

func GetTestFilename() string {
	_, filename, _, _ := runtime.Caller(0)
	return path.Dir(filename)
}
