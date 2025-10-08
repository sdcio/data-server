package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openconfig/gnmi/proto/gnmi"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// sdcpbDataTypeToGNMIType helper to convert the sdcpb data type to the gnmi data type
func SdcpbDataTypeToGNMIType(x sdcpb.DataType) (gnmi.GetRequest_DataType, error) {
	switch x {
	case sdcpb.DataType_ALL:
		return gnmi.GetRequest_ALL, nil
	case sdcpb.DataType_CONFIG:
		return gnmi.GetRequest_CONFIG, nil
	case sdcpb.DataType_STATE:
		return gnmi.GetRequest_STATE, nil
	}
	return 9999, fmt.Errorf("unable to convert sdcpb DataType %s to gnmi DataType", x)
}

// sdcpbEncodingToGNMIENcoding helper to convert sdcpb encoding to gnmi encoding
func SdcpbEncodingToGNMIENcoding(x sdcpb.Encoding) (gnmi.Encoding, error) {
	switch x {
	case sdcpb.Encoding_JSON:
		return gnmi.Encoding_JSON, nil
	case sdcpb.Encoding_JSON_IETF:
		return gnmi.Encoding_JSON_IETF, nil
	case sdcpb.Encoding_PROTO:
		return gnmi.Encoding_PROTO, nil
	case sdcpb.Encoding_STRING:
		return gnmi.Encoding_ASCII, nil
	}
	return 9999, fmt.Errorf("unable to convert sdcpb encoding %s to gnmi encoding", x)
}

func ParseGnmiEncoding(e string) int {
	enc, ok := gnmi.Encoding_value[strings.ToUpper(e)]
	if ok {
		return int(enc)
	}
	en, err := strconv.Atoi(e)
	if err != nil {
		return 0
	}
	return en
}

func ParseSdcpbEncoding(e string) int {
	enc, ok := sdcpb.Encoding_value[strings.ToUpper(e)]
	if ok {
		return int(enc)
	}
	en, err := strconv.Atoi(e)
	if err != nil {
		return 0
	}
	return en
}
