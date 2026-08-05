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
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func createReq() *sdcpb.CreateDataStoreRequest {
	return &sdcpb.CreateDataStoreRequest{
		DatastoreName: "dev1",
		Target: &sdcpb.Target{
			Type:    "gnmi",
			Address: "10.0.0.1",
			Port:    57400,
			Credentials: &sdcpb.Credentials{
				Username: "admin",
				Password: "s3cr3t",
				Token:    "t0k3n",
			},
		},
	}
}

// jsonLogger builds a logger wired like the one in main.go.
func jsonLogger(buf *bytes.Buffer) logr.Logger {
	opts := &slog.HandlerOptions{ReplaceAttr: RedactAttr}
	return logr.FromSlogHandler(slog.NewJSONHandler(buf, opts))
}

func TestRedactProtoCredentials(t *testing.T) {
	req := createReq()

	redacted, ok := RedactProto(req).(*sdcpb.CreateDataStoreRequest)
	if !ok {
		t.Fatalf("expected a *sdcpb.CreateDataStoreRequest")
	}

	creds := redacted.GetTarget().GetCredentials()
	if got := creds.GetPassword(); got != Redacted {
		t.Errorf("password not redacted, got %q", got)
	}
	if got := creds.GetToken(); got != Redacted {
		t.Errorf("token not redacted, got %q", got)
	}
	if got := creds.GetUsername(); got != "admin" {
		t.Errorf("username should be kept, got %q", got)
	}
	if got := redacted.GetTarget().GetAddress(); got != "10.0.0.1" {
		t.Errorf("address should be kept, got %q", got)
	}
	if got := req.GetTarget().GetCredentials().GetPassword(); got != "s3cr3t" {
		t.Errorf("the passed message must not be modified, got password %q", got)
	}
}

func TestRedactProtoUnsetFieldsStayUnset(t *testing.T) {
	req := &sdcpb.CreateDataStoreRequest{
		Target: &sdcpb.Target{
			Credentials: &sdcpb.Credentials{Username: "admin"},
		},
	}

	redacted := RedactProto(req).(*sdcpb.CreateDataStoreRequest)
	if got := redacted.GetTarget().GetCredentials().GetPassword(); got != "" {
		t.Errorf("an unset password should stay unset, got %q", got)
	}
}

// A secret below a repeated field: the walk has to reach every element.
func TestRedactProtoInsideRepeatedField(t *testing.T) {
	rsp := &sdcpb.ListDataStoreResponse{}
	for _, password := range []string{"s3cr3t", "an0ther"} {
		rsp.Datastores = append(rsp.Datastores, &sdcpb.GetDataStoreResponse{
			Target: &sdcpb.Target{
				Credentials: &sdcpb.Credentials{Username: "admin", Password: password},
				Tls:         &sdcpb.TLS{Ca: "ca.pem", Key: "private"},
			},
		})
	}

	redacted := RedactProto(rsp).(*sdcpb.ListDataStoreResponse)
	for i, ds := range redacted.GetDatastores() {
		if got := ds.GetTarget().GetCredentials().GetPassword(); got != Redacted {
			t.Errorf("password of element %d not redacted, got %q", i, got)
		}
		if got := ds.GetTarget().GetTls().GetKey(); got != Redacted {
			t.Errorf("tls key of element %d not redacted, got %q", i, got)
		}
		if got := ds.GetTarget().GetTls().GetCa(); got != "ca.pem" {
			t.Errorf("tls ca of element %d should be kept, got %q", i, got)
		}
	}
	if got := rsp.GetDatastores()[1].GetTarget().GetCredentials().GetPassword(); got != "an0ther" {
		t.Errorf("the passed message must not be modified, got password %q", got)
	}
}

func TestRedactProtoWithoutSensitiveContent(t *testing.T) {
	req := &sdcpb.GetDataStoreRequest{DatastoreName: "dev1"}

	if redacted := RedactProto(req); redacted != req {
		t.Errorf("a message without sensitive fields should be returned as it is")
	}
}

// The message should be a JSON object in the record, not escaped into a string.
func TestProtoJSONNestsIntoTheRecord(t *testing.T) {
	buf := &bytes.Buffer{}
	jsonLogger(buf).Info("received request", "raw-request", ProtoJSON(createReq()))

	out := buf.String()
	if strings.Contains(out, `\"`) {
		t.Errorf("the message should not be escaped into a string: %s", out)
	}
	if !strings.Contains(out, `"raw-request":{"datastoreName":"dev1"`) {
		t.Errorf("the message should be nested into the record: %s", out)
	}
	if !strings.Contains(out, `"password":"`+Redacted+`"`) {
		t.Errorf("expected the redacted placeholder in %s", out)
	}
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("password leaked into the log output: %s", out)
	}
}

// Loggers built without RedactAttr, such as the fallback logger of logf.
func TestProtoJSONRedactsWithoutTheHandlerHook(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logr.FromSlogHandler(slog.NewJSONHandler(buf, nil))
	log.Info("received request", "raw-request", ProtoJSON(createReq()))

	if out := buf.String(); strings.Contains(out, "s3cr3t") {
		t.Errorf("password leaked through an unhooked logger: %s", out)
	}
}

// A message that prints as <nil> must not break the record with a marshal error.
func TestProtoJSONHandlesUnsetMessages(t *testing.T) {
	buf := &bytes.Buffer{}
	jsonLogger(buf).Info("creating datastore", "datastore-target", ProtoJSON((*sdcpb.Target)(nil)))

	out := buf.String()
	if strings.Contains(out, "!ERROR") {
		t.Errorf("an unset message broke the record: %s", out)
	}
	if !strings.Contains(out, `"datastore-target":"<nil>"`) {
		t.Errorf("an unset message should render as <nil>: %s", out)
	}
}

// Messages that reach the logger directly, without ProtoJSON at the call site.
func TestRedactAttrRedactsProtoPassedToTheLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	jsonLogger(buf).Info("received request", "raw-request", createReq())

	out := buf.String()
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("password leaked through a directly logged proto: %s", out)
	}
	if !strings.Contains(out, "10.0.0.1") {
		t.Errorf("the rest of the message should still be logged: %s", out)
	}
	if !strings.Contains(out, `"raw-request":{`) {
		t.Errorf("the message should be nested into the record, not quoted into a string: %s", out)
	}
}

// A slice of messages reaches ReplaceAttr as one opaque value, not as a message.
func TestRedactAttrRedactsSliceOfProtos(t *testing.T) {
	buf := &bytes.Buffer{}
	targets := []*sdcpb.Target{
		createReq().GetTarget(),
		{Address: "10.0.0.2", Tls: &sdcpb.TLS{Ca: "ca.pem", Key: "private"}},
	}
	jsonLogger(buf).Info("creating datastores", "datastore-targets", targets)

	out := buf.String()
	for _, secret := range []string{"s3cr3t", "t0k3n", "private"} {
		if strings.Contains(out, secret) {
			t.Errorf("%q leaked through a slice of protos: %s", secret, out)
		}
	}
	if !strings.Contains(out, `"datastore-targets":[{`) {
		t.Errorf("the slice should be nested into the record as a list: %s", out)
	}
	if !strings.Contains(out, "10.0.0.2") {
		t.Errorf("the rest of the messages should still be logged: %s", out)
	}
}

// The hook sees every attribute of every record, so it must touch only messages.
func TestRedactAttrLeavesOtherValuesAlone(t *testing.T) {
	for _, value := range []any{nil, "plain", int64(42), []string{"a", "b"}, [2]int{1, 2}, struct{ A int }{1}} {
		got := RedactAttr(nil, slog.Any("key", value)).Value.Any()
		if !reflect.DeepEqual(got, value) {
			t.Errorf("RedactAttr rewrote %#v into %#v", value, got)
		}
	}
}

// The option plumbing by itself: an option whose generated code is not linked in
// lands in the unknown fields and reads back as absent instead of failing.
func TestSecretOptionsAreReadable(t *testing.T) {
	md := (&sdcpb.Credentials{}).ProtoReflect().Descriptor()

	msgOpts, ok := md.Options().(*descriptorpb.MessageOptions)
	if !ok {
		t.Fatalf("Credentials options are a %T", md.Options())
	}
	if value, set := boolOption(msgOpts, sdcpb.E_Secret); !set || !value {
		t.Errorf("secret option on data.Credentials read as set=%v value=%v, want set=true value=true", set, value)
	}

	fieldOpts, ok := md.Fields().ByName("username").Options().(*descriptorpb.FieldOptions)
	if !ok {
		t.Fatalf("username options are a %T", md.Fields().ByName("username").Options())
	}
	if value, set := boolOption(fieldOpts, sdcpb.E_SecretField); !set || value {
		t.Errorf("secret_field option on data.Credentials.username read as set=%v value=%v, want set=true value=false", set, value)
	}
}

// Dropping one of these markers in the protos would silently stop redaction here.
func TestProtoOptionsDriveRedaction(t *testing.T) {
	secret := map[protoreflect.FullName]bool{
		"data.Credentials.password": true,
		"data.Credentials.token":    true,
		"data.TLS.key":              true,
		// exempted from the secret option on the Credentials message
		"data.Credentials.username": false,
		"data.TLS.ca":               false,
		"data.Target.address":       false,
	}

	for name, want := range secret {
		d, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		if err != nil {
			t.Errorf("field %q does not exist: %v", name, err)
			continue
		}
		fd, ok := d.(protoreflect.FieldDescriptor)
		if !ok {
			t.Errorf("%q is a %T, not a field", name, d)
			continue
		}
		if got := isSecret(fd); got != want {
			t.Errorf("isSecret(%q) = %v, want %v", name, got, want)
		}
	}
}

// Deny by default: a field added to Credentials is redacted without touching this
// package.
func TestSecretOptionCoversFieldsAddedLater(t *testing.T) {
	fields := (&sdcpb.Credentials{}).ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Name() == "username" {
			continue // the proto exempts it explicitly
		}
		if !isSecret(fd) {
			t.Errorf("field %q of the Credentials message is not redacted", fd.FullName())
		}
	}
}
