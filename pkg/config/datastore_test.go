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

func TestCacheConfigValidateSetDefaults(t *testing.T) {
	tests := []struct {
		name    string
		in      *CacheConfig
		wantErr bool
		check   func(t *testing.T, c *CacheConfig)
	}{
		{
			name: "empty type defaults to local",
			in:   &CacheConfig{},
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type != "local" {
					t.Errorf("Type = %q, want %q", c.Type, "local")
				}
				if c.StoreType != defaultStoreType {
					t.Errorf("StoreType = %q, want %q", c.StoreType, defaultStoreType)
				}
				if c.Dir != defaultCacheDir {
					t.Errorf("Dir = %q, want %q", c.Dir, defaultCacheDir)
				}
			},
		},
		{
			name: "local type keeps its defaults",
			in:   &CacheConfig{Type: "local"},
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type != "local" {
					t.Errorf("Type = %q, want %q", c.Type, "local")
				}
				if c.StoreType != defaultStoreType {
					t.Errorf("StoreType = %q, want %q", c.StoreType, defaultStoreType)
				}
				if c.Dir != defaultCacheDir {
					t.Errorf("Dir = %q, want %q", c.Dir, defaultCacheDir)
				}
			},
		},
		{
			name: "local type respects explicit store settings",
			in:   &CacheConfig{Type: "local", StoreType: "memory", Dir: "/tmp/x"},
			check: func(t *testing.T, c *CacheConfig) {
				if c.StoreType != "memory" {
					t.Errorf("StoreType = %q, want %q", c.StoreType, "memory")
				}
				if c.Dir != "/tmp/x" {
					t.Errorf("Dir = %q, want %q", c.Dir, "/tmp/x")
				}
			},
		},
		{
			name: "remote type defaults address",
			in:   &CacheConfig{Type: "remote"},
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type != "remote" {
					t.Errorf("Type = %q, want %q", c.Type, "remote")
				}
				if c.Address != defaultRemoteCacheAddress {
					t.Errorf("Address = %q, want %q", c.Address, defaultRemoteCacheAddress)
				}
			},
		},
		{
			name:    "remote type rejects invalid address",
			in:      &CacheConfig{Type: "remote", Address: "not-a-valid-address"},
			wantErr: true,
		},
		{
			name: "config-server type survives unchanged",
			in:   &CacheConfig{Type: "config-server"},
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type != "config-server" {
					t.Errorf("Type = %q, want %q", c.Type, "config-server")
				}
				if c.StoreType != "" || c.Dir != "" || c.Address != "" {
					t.Errorf("expected no defaults to be applied for config-server type, got %+v", c)
				}
			},
		},
		{
			name:    "unknown type fails loudly instead of being coerced to local",
			in:      &CacheConfig{Type: "bogus"},
			wantErr: true,
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type == "local" {
					t.Errorf("unknown type must not be silently coerced to %q", "local")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.validateSetDefaults()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSetDefaults() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, tt.in)
			}
		})
	}
}
