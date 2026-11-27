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

package schema_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdcio/data-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestLocalClient_GetSchema_IsMutateOnWrite(t *testing.T) {
	yangDir := t.TempDir()

	files := map[string]string{
		"sdcio-ext.yang": `module sdcio-ext {
  yang-version 1.1;
  namespace "urn:sdcio-ext";
  prefix "sdcio-ext";
  extension mutate-on-write;
}
`,
		"iana-crypt-hash.yang": `module iana-crypt-hash {
  namespace "urn:ietf:params:xml:ns:yang:iana-crypt-hash";
  prefix "ianach";
  typedef crypt-hash {
    type string;
  }
}
`,
		"device.yang": `module ds-mow-test {
  yang-version 1.1;
  namespace "urn:ds:mow:test";
  prefix "dsmow";

  import iana-crypt-hash { prefix "ianach"; }

  container system {
    leaf hashed-password {
      type ianach:crypt-hash;
    }
    leaf overlay-password {
      type string;
    }
    leaf hostname {
      type string;
    }
  }
}
`,
		"overlay.yang": `module ds-mow-overlay {
  yang-version 1.1;
  namespace "urn:ds:mow:overlay";
  prefix "dsov";

  import ds-mow-test { prefix "dsmow"; }
  import sdcio-ext   { prefix "mowext"; }

  deviation /dsmow:system/dsmow:overlay-password {
    deviate add {
      mowext:mutate-on-write;
    }
  }
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(yangDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	ctx := context.Background()
	st := memstore.New()
	t.Cleanup(func() { _ = st.Close() })

	client := schema.NewLocalClient(st)
	scRef := &sdcpb.Schema{
		Name:    "ds-mow",
		Vendor:  "test",
		Version: "1.0.0",
	}

	if _, err := client.CreateSchema(ctx, &sdcpb.CreateSchemaRequest{
		Schema: scRef,
		File:   []string{yangDir},
	}); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	tests := []struct {
		name  string
		elems []string
		want  bool
	}{
		{name: "container returns false", elems: []string{"system"}, want: false},
		{name: "iana-crypt-hash leaf returns true", elems: []string{"system", "hashed-password"}, want: true},
		{name: "custom-prefix deviate add overlay leaf returns true", elems: []string{"system", "overlay-password"}, want: true},
		{name: "plain string hostname returns false", elems: []string{"system", "hostname"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toPathRsp, err := client.ToPath(ctx, &sdcpb.ToPathRequest{
				Schema:      scRef,
				PathElement: tt.elems,
			})
			if err != nil {
				t.Fatalf("ToPath(%v): %v", tt.elems, err)
			}
			rsp, err := client.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Schema: scRef,
				Path:   toPathRsp.GetPath(),
			})
			if err != nil {
				t.Fatalf("GetSchema(%v): %v", tt.elems, err)
			}
			if got := rsp.GetSchema().IsMutateOnWrite(); got != tt.want {
				t.Errorf("GetSchema(%v).IsMutateOnWrite() = %v, want %v", tt.elems, got, tt.want)
			}
		})
	}
}
