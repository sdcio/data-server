package api

import "github.com/sdcio/sdc-protos/sdcpb"

type SdcpbPath interface {
	SdcpbPath() *sdcpb.Path
}
