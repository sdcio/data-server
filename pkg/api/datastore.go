package api

// TargetType names the southbound protocol of a datastore target.
type TargetType string

const (
	TargetTypeGNMI    TargetType = "gnmi"
	TargetTypeNetconf TargetType = "netconf"
)

// ConnectionState reports the southbound connection state of a datastore.
type ConnectionState int

const (
	ConnectionUnknown ConnectionState = iota
	ConnectionConnected
	ConnectionNotConnected
)

// Credentials authenticate against a target.
type Credentials struct {
	Username string
	Password string
	Token    string
}

// TLS configures the client side of the target connection.
type TLS struct {
	CA         string
	Cert       string
	Key        string
	SkipVerify bool
}

// CommitCandidate selects which NETCONF datastore is committed.
type CommitCandidate int

const (
	CommitCandidateCandidate CommitCandidate = iota
	CommitCandidateRunning
)

// GNMIOptions are the gNMI-specific protocol options.
type GNMIOptions struct {
	Encoding   string
	TargetName string
}

// NetconfOptions are the NETCONF-specific protocol options.
type NetconfOptions struct {
	IncludeNs          bool
	OperationWithNs    bool
	UseOperationRemove bool
	CommitCandidate    CommitCandidate
}

// ProtocolOptions is a union: at most one of GNMI and Netconf is set.
type ProtocolOptions struct {
	GNMI    *GNMIOptions
	Netconf *NetconfOptions
}

// Target describes how to reach a device.
type Target struct {
	Type            TargetType
	Address         string
	Port            uint32
	Credentials     *Credentials
	TLS             *TLS
	ProtocolOptions ProtocolOptions
}

// SchemaRef identifies a schema by vendor and version.
type SchemaRef struct {
	Vendor  string
	Version string
}

// SyncMode selects how a sync strategy collects device state.
type SyncMode int

const (
	SyncModeOnChange SyncMode = iota
	SyncModeSample
	SyncModeOnce
	SyncModeGet
)

// SyncConfig is one sync strategy. Target, when set, overrides the datastore
// target for this strategy.
type SyncConfig struct {
	Name     string
	Target   *Target
	Paths    []string
	Mode     SyncMode
	Interval uint64
}

// Sync is the datastore sync strategy.
type Sync struct {
	Validate     bool
	Buffer       int64
	WriteWorkers int64
	Configs      []SyncConfig
}

// DatastoreSpec is the desired state used to create a datastore.
type DatastoreSpec struct {
	Name   string
	Target Target
	Schema SchemaRef
	Sync   Sync
}

// DatastoreInfo is the observed state of a datastore.
type DatastoreInfo struct {
	Name            string
	ConnectionState ConnectionState
	// StatusDetails explains a not-connected state when available.
	StatusDetails string
	Intents       []string
	// Spec is the effective spec the datastore runs with.
	Spec DatastoreSpec
}
