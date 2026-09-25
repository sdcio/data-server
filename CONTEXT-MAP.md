# Context Map

## Contexts

- [Cache](./pkg/cache/CONTEXT.md) — Client abstraction for per-target Intent and device-state storage
- [Tree](./pkg/tree/CONTEXT.md) — In-process config tree, northbound render, and sensitivity redaction

## Relationships

- **Cache → Tree**: Cache supplies Intent documents (values, deletes, sensitive path markers, …); Tree materializes and renders them
- **Tree → Cache**: Tree does not own persistence; Intent path-marker updates flow back through datastore/cache write paths
