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

// Redaction of the fields that the sdc protos mark as sensitive, see
// options.proto. A message reaches the log either through ProtoJSON or through RedactAttr.
package utils

import (
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Redacted is what a secret value looks like in the log.
const Redacted = "[REDACTED]"

// ProtoJSON wraps m for logging as a nested JSON object with the secret values
// replaced. Prefer it over passing m to the logger, which redacts only if the
// handler has RedactAttr installed.
func ProtoJSON(m proto.Message) any {
	return protoValue{m: m}
}

// RedactAttr is a ReplaceAttr hook for slog.HandlerOptions that runs every message
// reaching the logger through ProtoJSON, on its own or in a slice.
func RedactAttr(_ []string, a slog.Attr) slog.Attr {
	// Keep the reflection below off everything the logger already classified.
	if a.Value.Kind() != slog.KindAny {
		return a
	}
	if wrapped, ok := wrapMessages(a.Value.Any()); ok {
		a.Value = slog.AnyValue(wrapped)
	}
	return a
}

var messageType = reflect.TypeFor[proto.Message]()

// wrapMessages wraps a message, or a slice or array of messages, reporting false
// for anything else.
func wrapMessages(v any) (any, bool) {
	if m, ok := v.(proto.Message); ok {
		return ProtoJSON(m), true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	if !rv.Type().Elem().Implements(messageType) {
		return nil, false
	}
	wrapped := make([]any, rv.Len())
	for i := range wrapped {
		m, _ := rv.Index(i).Interface().(proto.Message)
		wrapped[i] = ProtoJSON(m)
	}
	return wrapped, true
}

// protoValue renders a message with the secret values replaced.
type protoValue struct {
	m proto.Message
}

func (p protoValue) MarshalJSON() ([]byte, error) {
	if p.m == nil || !p.m.ProtoReflect().IsValid() {
		return []byte(`"<nil>"`), nil
	}
	b, err := protojson.MarshalOptions{AllowPartial: true, Multiline: false}.Marshal(RedactProto(p.m))
	if err != nil {
		return json.Marshal("<" + err.Error() + ">")
	}
	return b, nil
}

func (p protoValue) String() string {
	b, err := p.MarshalJSON()
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

// RedactProto returns a copy of m with every secret field redacted.
func RedactProto(m proto.Message) proto.Message {
	if m == nil {
		return m
	}
	rm := m.ProtoReflect()
	if !rm.IsValid() || !typeHasSecret(rm.Descriptor()) {
		return m
	}
	redacted := proto.Clone(m)

	// Range hands over the chain of steps from the root down to each visited value,
	// so the last pair is the value and the one before it is the message declaring
	// it. Only a step into a field carries a descriptor; the root, list indexes and
	// map keys have none, and a repeated secret is already caught at the field step
	// above its elements.
	_ = protorange.Range(redacted.ProtoReflect(), func(path protopath.Values) error {
		visited, parent := path.Index(-1), path.Index(-2)
		fd := visited.Step.FieldDescriptor()
		if fd == nil || !isSecret(fd) {
			return nil
		}
		redactField(parent.Value.Message(), fd)
		return nil
	})
	return redacted
}

// redactField overwrites a single secret field. Only strings and bytes can carry
// the placeholder, so a secret of any other kind is dropped and reads back from
// the log as unset.
func redactField(m protoreflect.Message, fd protoreflect.FieldDescriptor) {
	switch {
	case fd.IsList() || fd.IsMap():
		m.Clear(fd)
	case fd.Kind() == protoreflect.StringKind:
		m.Set(fd, protoreflect.ValueOfString(Redacted))
	case fd.Kind() == protoreflect.BytesKind:
		m.Set(fd, protoreflect.ValueOfBytes([]byte(Redacted)))
	default:
		m.Clear(fd)
	}
}

// isSecret reports whether the proto marks fd as secret
func isSecret(fd protoreflect.FieldDescriptor) bool {
	if secret, set := boolOption(fd.Options(), sdcpb.E_SecretField); set {
		return secret
	}
	secret, _ := boolOption(fd.ContainingMessage().Options(), sdcpb.E_Secret)
	return secret
}

// boolOption reads an optional bool option from a descriptor
func boolOption(opts protoreflect.ProtoMessage, xt protoreflect.ExtensionType) (value, set bool) {
	if !proto.HasExtension(opts, xt) {
		return false, false
	}
	value, _ = proto.GetExtension(opts, xt).(bool)
	return value, true
}

// secretTypes caches typeHasSecret by message name.
var secretTypes sync.Map

// typeHasSecret reports whether md has a secret field, directly or below it.
func typeHasSecret(md protoreflect.MessageDescriptor) bool {
	if cached, ok := secretTypes.Load(md.FullName()); ok {
		return cached.(bool)
	}
	secret := scanType(md, map[protoreflect.FullName]bool{})
	secretTypes.Store(md.FullName(), secret)
	return secret
}

// scanType walks the types reachable from md, guarding against cycles.
func scanType(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) bool {
	if seen[md.FullName()] {
		return false
	}
	seen[md.FullName()] = true

	for i, fields := 0, md.Fields(); i < fields.Len(); i++ {
		fd := fields.Get(i)
		if isSecret(fd) {
			return true
		}
		if fmd := fd.Message(); fmd != nil && scanType(fmd, seen) {
			return true
		}
	}
	return false
}
