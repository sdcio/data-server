package api

// SchemaState is the lifecycle state of a schema.
type SchemaState int

const (
	SchemaReady SchemaState = iota
	SchemaReloading
	SchemaInitializing
	SchemaFailed
)

// SchemaInfo is a schema with its state and source locations.
type SchemaInfo struct {
	Schema      SchemaRef
	State       SchemaState
	Files       []string
	Directories []string
	Excludes    []string
}

// CreateSchemaSpec is the input for creating a schema.
type CreateSchemaSpec struct {
	Schema      SchemaRef
	Files       []string
	Directories []string
	Excludes    []string
}

// DeleteSchemaSpec identifies the schema to delete.
type DeleteSchemaSpec struct {
	Schema SchemaRef
}
