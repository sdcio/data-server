# data-server

Control-plane service that holds intended and running configuration, validates YANG-backed trees, and applies changes to southbound targets (gNMI, NETCONF, noop).

## Language

**Device profile**:
A named NOS-specific mode on a southbound interface (SBI) that selects how intent is materialized and how gNMI Get/Set are shaped for that vendor stack.
_Avoid_: Vendor flag, target flavor (when meaning this field specifically)

**Device-profile base (shared plumbing)**:
Config and API exposure of `device_profile`, southbound plan types, materialization entry points, and dispatch seams—without a given NOS encoder or profile-specific validation.
_Avoid_: Machinery (informal shorthand)

**NOS implementation PR**:
A change set that enables one device profile end-to-end: profile-specific validation, Set encoding, Get shaping (where needed), and tests for that profile only.
_Avoid_: Vendor PR

**Profile enablement**:
The moment a NOS implementation PR lifts the base-layer rejection for that profile and adds that profile’s validation and encoding behavior.
_Avoid_: Turning the feature on (too informal)

**Parallel NOS stacks**:
Two pull requests that both target the same device-profile base branch (or `main` after that base merges), neither stacked on the other—SONiC and Cisco IOS-XR do not depend on each other in Git or in runtime.
_Avoid_: Stacked vendors

**Root module**:
The YANG module that owns the first configuration node in an update or path—the namespace half of root-level identity when sibling modules share the same local name (e.g. multiple IOS-XR `router` containers).
_Avoid_: Path origin (without saying gNMI Origin field), namespace anchor

**Root module resolution (GET ingress)**:
When disambiguation is required, infer root module only from the southbound response: module-qualified first path element, gNMI notification prefix origin, then module-qualified top-level JSON-IETF keys—never from sync configuration or other out-of-band hints. If the response remains ambiguous, fail closed.
_Avoid_: Lexicographic schema winner, sync path as module source, device-wide default module table
