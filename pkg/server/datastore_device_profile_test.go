// Copyright 2026 Nokia
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

package server

import (
	"testing"

	"github.com/sdcio/data-server/pkg/config"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestSdcpbDeviceProfileToConfig(t *testing.T) {
	tests := []struct {
		name  string
		input sdcpb.DeviceProfile
		want  config.DeviceProfile
	}{
		{
			name:  "generic maps to none",
			input: sdcpb.DeviceProfile_DEVICE_PROFILE_GENERIC,
			want:  config.DeviceProfileNone,
		},
		{
			name:  "cisco-ios-xr maps to CiscoIOSXR",
			input: sdcpb.DeviceProfile_DEVICE_PROFILE_CISCO_IOS_XR,
			want:  config.DeviceProfileCiscoIOSXR,
		},
		{
			name:  "sonic maps to Sonic",
			input: sdcpb.DeviceProfile_DEVICE_PROFILE_SONIC,
			want:  config.DeviceProfileSonic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sdcpbDeviceProfileToConfig(tt.input)
			if got != tt.want {
				t.Errorf("sdcpbDeviceProfileToConfig(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfigDeviceProfileToSdcpb(t *testing.T) {
	tests := []struct {
		name  string
		input config.DeviceProfile
		want  sdcpb.DeviceProfile
	}{
		{
			name:  "none maps to generic",
			input: config.DeviceProfileNone,
			want:  sdcpb.DeviceProfile_DEVICE_PROFILE_GENERIC,
		},
		{
			name:  "CiscoIOSXR maps to cisco-ios-xr",
			input: config.DeviceProfileCiscoIOSXR,
			want:  sdcpb.DeviceProfile_DEVICE_PROFILE_CISCO_IOS_XR,
		},
		{
			name:  "Sonic maps to sonic",
			input: config.DeviceProfileSonic,
			want:  sdcpb.DeviceProfile_DEVICE_PROFILE_SONIC,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configDeviceProfileToSdcpb(tt.input)
			if got != tt.want {
				t.Errorf("configDeviceProfileToSdcpb(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
