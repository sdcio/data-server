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

package netconf

import (
	"errors"
	"io"
	"net"
	"strings"
)

// isTransportError reports whether err represents a transport-level connection
// drop (e.g. the southbound device restarting mid-RPC) rather than a
// semantic/validation failure returned by the device itself.
//
// It relies only on the standard io/net error vocabulary (io.EOF, net.Error)
// plus a substring fallback for drivers that don't preserve that wrapping.
// Driver implementations are responsible for normalizing their own
// library-specific transport sentinels into io.EOF (or a net.Error) at the
// Driver interface boundary, so this function stays driver-agnostic.
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// Substring fallback: a safety net for drivers other than scrapligo (or a
	// future scrapligo version) that don't preserve io.EOF/net.Error wrapping
	// through their own error paths. Driver implementations should still
	// prefer normalizing at their own boundary (see driver/scrapligo's
	// normalizeTransportError) so this fallback ideally never fires in
	// practice.
	msg := err.Error()
	for _, sub := range []string{"EOF", "broken pipe", "connection reset", "i/o timeout"} {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}
