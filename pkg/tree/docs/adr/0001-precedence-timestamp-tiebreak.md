# Precedence tiebreak uses intent creation time when priorities are equal

When multiple owners set the same leaf at the same priority, winner selection must be deterministic across tree rebuilds. Insertion order (first in `LeafVariants.les`) is unstable because intent load order from cache is not guaranteed.

**Decision:** introduce `ComparePrecedence` used everywhere a winner is picked. Lower priority number still wins first. On equal priority, lower timestamp wins (earlier-created intent keeps precedence). On equal priority and equal timestamp, ascending owner-name lexicographic sort breaks the tie.

Timestamps are per-intent (all leaves from one intent share one value), in Unix seconds. config-server supplies K8s `metadata.creationTimestamp` via `config_read.ConfigEntry.created_at`; local cache stores `created_at` on `tree_persist.Intent`, set once on first write. Running and defaults use timestamp `0` and lose to any real intent timestamp.

Same priority with the same value is benign — no deviation is emitted. Same priority with different values: the loser is overruled. Choice-case resolution uses the same tiebreak on the winning leaf within each branch.

**Considered options:** insertion order (status quo, rejected — unstable); `metadata.generation` (rejected — changes on every edit); last-applied time (rejected — heavier, needs writeback plumbing).

**Consequences:** coordinated proto + config-server + data-server change. `ImportConfigAdapter` gains `GetTimestamp()`. Choice resolver `caseElement` carries tiebreak metadata beyond bare priority.
