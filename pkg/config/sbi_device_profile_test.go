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

// validGNMISBI returns an SBI configured for gNMI with the given encoding and
// device-profile, with address/port filled in so validateSetDefaults passes all
// other checks.
func validGNMISBI(encoding string, deviceProfile DeviceProfile) *SBI {
	return &SBI{
		Type:          sbiGNMI,
		Address:       "192.0.2.1",
		Port:          57400,
		GnmiOptions:   &SBIGnmiOptions{Encoding: encoding},
		DeviceProfile: deviceProfile,
	}
}

func TestSBI_IsCiscoIOSXR(t *testing.T) {
	if (&SBI{DeviceProfile: DeviceProfileCiscoIOSXR}).IsCiscoIOSXR() != true {
		t.Fatal("expected true for cisco-ios-xr profile")
	}
	if (&SBI{DeviceProfile: DeviceProfileNone}).IsCiscoIOSXR() != false {
		t.Fatal("expected false for DeviceProfileNone")
	}
	if (&SBI{}).IsCiscoIOSXR() != false {
		t.Fatal("expected false for zero-value DeviceProfile (same as DeviceProfileNone)")
	}
	if (&SBI{DeviceProfile: DeviceProfile("other")}).IsCiscoIOSXR() != false {
		t.Fatal("expected false for unrelated profile")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_UnknownProfileIsRejected(t *testing.T) {
	sbi := validGNMISBI("json_ietf", DeviceProfile("not-a-valid-profile"))
	if err := sbi.validateSetDefaults(); err == nil {
		t.Fatal("expected error for unknown device-profile, got nil")
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRNetconfIsAccepted(t *testing.T) {
	sbi := &SBI{
		Type:           sbiNETCONF,
		Address:        "192.0.2.1",
		Port:           830,
		NetconfOptions: &SBINetconfOptions{},
		DeviceProfile:  DeviceProfileCiscoIOSXR,
	}
	if err := sbi.validateSetDefaults(); err != nil {
		t.Fatalf("unexpected error for cisco-ios-xr + netconf: %v", err)
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIJSONIsAccepted(t *testing.T) {
	sbi := validGNMISBI("json_ietf", DeviceProfileCiscoIOSXR)
	if err := sbi.validateSetDefaults(); err != nil {
		t.Fatalf("unexpected error for cisco-ios-xr + gnmi + json_ietf: %v", err)
	}
}

func TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIProtoIsAccepted(t *testing.T) {
	sbi := validGNMISBI("proto", DeviceProfileCiscoIOSXR)
	if err := sbi.validateSetDefaults(); err != nil {
		t.Fatalf("unexpected error for cisco-ios-xr + gnmi + proto: %v", err)
	}
}
