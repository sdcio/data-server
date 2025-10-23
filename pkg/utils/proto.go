package utils

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func FormatProtoJSON(m proto.Message) string {
	return protojson.MarshalOptions{Multiline: false}.Format(m)
}