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
	"errors"
	"testing"
)

// validGNMISBI returns an SBI configured for gNMI with the given encoding and
// device-profile, with address/port filled in so validateSetDefaults passes all
// other checks (when the profile is enabled).
func validGNMISBI(encoding string, deviceProfile DeviceProfile) *SBI {
	return &SBI{
		Type:          sbiGNMI,
		Address:       "192.0.2.1",
		Port:          57400,
		GnmiOptions:   &SBIGnmiOptions{Encoding: encoding},
		DeviceProfile: deviceProfile,
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_UnknownProfileIsRejected(t *testing.T) {
	sbi := validGNMISBI("json_ietf", DeviceProfile("not-a-valid-profile"))
	if err := sbi.validateSetDefaults(); err == nil {
		t.Fatal("expected error for unknown device-profile, got nil")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRIsNotEnabled(t *testing.T) {
	sbi := validGNMISBI("json_ietf", DeviceProfileCiscoIOSXR)
	err := sbi.validateSetDefaults()
	if err == nil {
		t.Fatal("expected error for cisco-ios-xr on base branch, got nil")
	}
	if !errors.Is(err, ErrDeviceProfileNotEnabled) {
		t.Fatalf("expected ErrDeviceProfileNotEnabled, got %v", err)
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_SonicGNMIJSONIETFIsAccepted(t *testing.T) {
	sbi := validGNMISBI("JSON_IETF", DeviceProfileSonic)
	if err := sbi.validateSetDefaults(); err != nil {
		t.Fatalf("unexpected error for sonic + gnmi + JSON_IETF: %v", err)
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_SonicGNMIJSONIsRejected(t *testing.T) {
	sbi := validGNMISBI("JSON", DeviceProfileSonic)
	if err := sbi.validateSetDefaults(); err == nil {
		t.Fatal("expected error for sonic + gnmi + JSON, got nil")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_SonicGNMIProtoIsRejected(t *testing.T) {
	sbi := validGNMISBI("PROTO", DeviceProfileSonic)
	if err := sbi.validateSetDefaults(); err == nil {
		t.Fatal("expected error for sonic + gnmi + PROTO, got nil")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_SonicNetconfIsRejected(t *testing.T) {
	sbi := &SBI{
		Type:           sbiNETCONF,
		Address:        "192.0.2.1",
		Port:           830,
		NetconfOptions: &SBINetconfOptions{},
		DeviceProfile:  DeviceProfileSonic,
	}
	if err := sbi.validateSetDefaults(); err == nil {
		t.Fatal("expected error for sonic + netconf, got nil")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_GenericProfileAccepted(t *testing.T) {
	for _, enc := range []string{"json_ietf", "json", "proto"} {
		if err := validGNMISBI(enc, DeviceProfileNone).validateSetDefaults(); err != nil {
			t.Fatalf("unexpected error for none + gnmi + %s: %v", enc, err)
		}
	}
}
