package schema

type LeafType string

const (
	BINARY               LeafType = "BINARY"
	BITS                 LeafType = "BITS"
	BOOLEAN              LeafType = "BOOLEAN"
	DECIMAL64            LeafType = "DECIMAL64"
	EMPTY                LeafType = "EMPTY"
	ENUM                 LeafType = "ENUM"
	IDENTITYREF          LeafType = "IDENTITYREF"
	INSTANCE_IDENTIFIER  LeafType = "INSTANCE_IDENTIFIER"
	INT16                LeafType = "INT16"
	INT32                LeafType = "INT32"
	INT64                LeafType = "INT64"
	INT8                 LeafType = "INT8"
	INTERFACE_NAME       LeafType = "INTERFACE_NAME"
	IP_ADDRESS           LeafType = "IP_ADDRESS"
	IP_ADDRESS_WITH_ZONE LeafType = "IP_ADDRESS_WITH_ZONE"
	IP_PREFIX            LeafType = "IP_PREFIX"
	MAC_ADDRESS          LeafType = "MAC_ADDRESS"
	STRING               LeafType = "STRING"
	SUBINTERFACE_NAME    LeafType = "SUBINTERFACE_NAME"
	UINT16               LeafType = "UINT16"
	UINT32               LeafType = "UINT32"
	UINT64               LeafType = "UINT64"
	UINT8                LeafType = "UINT8"
	UNION                LeafType = "UNION"
	ROUTE_DISTINGUISHER  LeafType = "ROUTE_DISTINGUISHER"
)
