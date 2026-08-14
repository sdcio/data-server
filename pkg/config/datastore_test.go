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

package config

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

const datastoreYAML = `
name: dev1
sbi:
  type: gnmi
  address: 10.0.0.1
  credentials:
    username: admin
    password: s3cr3t
    token: t0k3n
`

func TestDatastoreConfigCredentialsRoundTrip(t *testing.T) {
	cfg := &DatastoreConfig{}
	if err := yaml.Unmarshal([]byte(datastoreYAML), cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	creds := cfg.SBI.Credentials
	if got := creds.Password; got != "s3cr3t" {
		t.Errorf("password = %q, want %q", got, "s3cr3t")
	}
	if got := creds.Token; got != "t0k3n" {
		t.Errorf("token = %q, want %q", got, "t0k3n")
	}
}

// The config dump main.go writes to the log on startup.
func TestDatastoreConfigDumpHasNoSecrets(t *testing.T) {
	cfg := &DatastoreConfig{}
	if err := yaml.Unmarshal([]byte(datastoreYAML), cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	dump := string(b)
	for _, secret := range []string{"s3cr3t", "t0k3n"} {
		if strings.Contains(dump, secret) {
			t.Errorf("secret %q leaked into the config dump: %s", secret, dump)
		}
	}
	if !strings.Contains(dump, `"username": "admin"`) {
		t.Errorf("username should be kept, got %s", dump)
	}
}
