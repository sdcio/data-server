# Index: sonic-device-profile

Ticket list for this effort, in dependency order. Each ticket's `Status:` line lives in its own file — this index is a read-only summary, not the source of truth for status.

| # | Ticket | Blocked by | Status |
|---|--------|-----------|--------|
| [01](01-device-profile-config-field-and-validation.md) | Sonic device-profile config field and closed-set validation | None | done |
| [02](02-sdc-protos-device-profile-sonic-enum.md) | sdc-protos: add `DEVICE_PROFILE_SONIC` to the `DeviceProfile` enum | None | done |
| [03](03-sonic-encoder-package.md) | Sonic `GnmiSetPlan` encoder package | None | ready-for-agent |
| [04](04-wire-sonic-dispatch-in-materialize.md) | Wire sonic dispatch into `materialize.BuildPlan` | 01, 03 | ready-for-agent |
| [05](05-expose-sonic-profile-on-grpc-target-cr.md) | Expose `DeviceProfileSonic` on the gRPC/Target-CR layer | 01, 02 | ready-for-agent |

Note: ticket 02 lands on the separate `sdc-protos` repo (`deviceprofile` branch), not `data-server` — track its status there but keep this row in sync.

## Working the frontier

1. Scan the table above (re-reading each ticket's own `Status:` line, since that's authoritative, not this table) for tickets that are `ready-for-agent` **and** unblocked (every ticket listed in its "Blocked by" is `Status: done`).
2. Among those, pick the lowest-numbered one — that's the frontier ticket. Right now 03 is unblocked and ready to pick up (02 landed on sdc-protos `deviceprofile` @ `40ed0bc`).
3. On completion, set that ticket's `Status:` line to `done`, update its row in the table above to match, and append a one-line pointer under a `## Comments` heading in the ticket file noting what landed (commit/branch if applicable).
4. If no ticket is both `ready-for-agent` and unblocked, report that the frontier is empty instead of guessing.
