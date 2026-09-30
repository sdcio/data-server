# Shared, refcounted schema-bound registry per (vendor, name, version)

We deduplicate schema lookup caching across datastores that share the same YANG schema identity by pooling `SchemaClientBoundImpl` instances in a small schema client registry keyed by (name, vendor, version), instead of each datastore building its own private cache. `Datastore.New` acquires a schema-bound handle from the registry; `Datastore.Delete` calls `Close()` on that handle, which releases the registry's refcount and evicts the pooled instance once no datastore uses that schema identity anymore. The pooled instance's internal structure (path-keyed lookup map, root-ambiguity cache, per-entry coalescing) is unchanged—only its construction and lifecycle move from "one per datastore" to "one per schema identity, refcounted."

As part of the same change we remove `remoteClient`'s separate `ttlcache` and the `RemoteSchemaCache` config it was driven by, so the registry becomes the single schema-caching mechanism in the process. This includes the northbound `GetSchema`/`GetSchemaDetails` passthrough RPC handlers, which lose caching as an accepted simplification. No TTL is introduced anywhere: the registry keeps an entry alive exactly as long as at least one datastore holds a handle to it.

**Considered options:**
- Per-datastore cache (today's behavior)—rejected, duplicates memory across datastores on the same schema identity.
- A flat map keyed by (name, vendor, version, path) inside `schema.Client`—rejected in favor of pooling whole `SchemaClientBoundImpl` instances per identity, which requires no change to the existing per-path coalescing/storage code and gives a single coordination point per schema identity for future work (e.g. reload).
- Weak references from the registry with strong references held by tree nodes, so entries free automatically once no config references a path—deferred; a larger change bundled with reload/generation invalidation and per-path (not per-identity) eviction, tracked separately.

**Deferred to a follow-up:** invalidation on `ReloadSchema`/`DeleteSchema`, and any eviction of individual schema paths (e.g. when a config branch is deleted)—the registry evicts only whole pooled instances when a schema identity has no remaining datastore.
