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

import "testing"

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
			name:    "config-server type requires an address",
			in:      &CacheConfig{Type: "config-server", Namespace: "sdcio"},
			wantErr: true,
		},
		{
			name:    "config-server type rejects an invalid address",
			in:      &CacheConfig{Type: "config-server", Address: "not-a-valid-address", Namespace: "sdcio"},
			wantErr: true,
		},
		{
			name:    "config-server type requires a namespace",
			in:      &CacheConfig{Type: "config-server", Address: "localhost:50051"},
			wantErr: true,
		},
		{
			name: "config-server type keeps explicit connection settings",
			in:   &CacheConfig{Type: "config-server", Address: "localhost:50051", Namespace: "sdcio"},
			check: func(t *testing.T, c *CacheConfig) {
				if c.Type != "config-server" {
					t.Errorf("Type = %q, want %q", c.Type, "config-server")
				}
				if c.Address != "localhost:50051" {
					t.Errorf("Address = %q, want %q", c.Address, "localhost:50051")
				}
				if c.Namespace != "sdcio" {
					t.Errorf("Namespace = %q, want %q", c.Namespace, "sdcio")
				}
				if c.StoreType != "" || c.Dir != "" {
					t.Errorf("expected no local-cache defaults to be applied for config-server type, got %+v", c)
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
