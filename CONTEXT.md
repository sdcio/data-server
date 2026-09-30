# data-server

Control-plane service that holds intended and running configuration, validates YANG-backed trees, and applies changes to southbound targets (gNMI, NETCONF, noop).

## Language

## Schema caching

**Schema client registry**:
A process-wide pool of one `SchemaClientBoundImpl` per YANG schema identity (name, vendor, version), shared by every datastore that uses that identity instead of each datastore building its own.
_Avoid_: Schema cache (too broad—this is the pooling/lifecycle layer, not the path-keyed lookup cache inside the pooled instance)

**Schema-bound handle**:
The value a datastore receives from the schema client registry: behaves as an ordinary `SchemaClientBound` for lookups, and releases the registry's hold on the pooled instance when the datastore closes it.
_Avoid_: Schema client bound (that name is reserved for the pooled instance itself, not the closeable handle to it)
