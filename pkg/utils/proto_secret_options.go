// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Local copies of sdc.options secret extensions so redaction works when the
// pinned sdc-protos revision predates options.pb.go (see device-profile base pin).
var (
	protoSecretMessageExt = &protoimpl.ExtensionInfo{
		ExtendedType:  (*descriptorpb.MessageOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         51301,
		Name:          "sdc.options.secret",
		Tag:           "varint,51301,opt,name=secret",
		Filename:      "options.proto",
	}
	protoSecretFieldExt = &protoimpl.ExtensionInfo{
		ExtendedType:  (*descriptorpb.FieldOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         51302,
		Name:          "sdc.options.secret_field",
		Tag:           "varint,51302,opt,name=secret_field",
		Filename:      "options.proto",
	}
)

func secretFieldExtension() protoreflect.ExtensionType {
	return protoSecretFieldExt
}

func secretMessageExtension() protoreflect.ExtensionType {
	return protoSecretMessageExt
}
