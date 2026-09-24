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
	"fmt"
)

// ErrDeviceProfileNotEnabled is returned when a known NOS device profile is
// recognized but not yet enabled in this build (base branch before the matching
// NOS PR merges).
var ErrDeviceProfileNotEnabled = errors.New("device profile is not enabled")

// ValidateDeviceProfileEnabled rejects non-generic profiles until the
// corresponding NOS stack PR enables them.
func ValidateDeviceProfileEnabled(profile DeviceProfile) error {
	switch profile {
	case DeviceProfileNone, DeviceProfileCiscoIOSXR:
		return nil
	case DeviceProfileSonic:
		return fmt.Errorf("%w: %q", ErrDeviceProfileNotEnabled, profile)
	default:
		return fmt.Errorf("unknown device-profile: %q", profile)
	}
}
