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

package schema

import (
	"strings"

	ssSchema "github.com/sdcio/schema-server/pkg/schema"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RootNameAmbiguity is a local name that appears under more than one top-level YANG module.
type RootNameAmbiguity struct {
	LocalName string
	Modules   []string
}

// RootAmbiguitiesFromDetails reads the root ambiguity registry from GetSchemaDetailsResponse.exclude.
func RootAmbiguitiesFromDetails(details *sdcpb.GetSchemaDetailsResponse) []RootNameAmbiguity {
	if details == nil {
		return nil
	}
	return ParseAmbiguityRegistryExclude(details.GetExclude())
}

// ParseAmbiguityRegistryExclude parses machine-readable ambiguity entries encoded in schema-server exclude lines.
func ParseAmbiguityRegistryExclude(exclude []string) []RootNameAmbiguity {
	prefix := ssSchema.AmbiguousNameRegistryExcludePrefix
	out := make([]RootNameAmbiguity, 0)
	for _, entry := range exclude {
		if !strings.HasPrefix(entry, prefix) {
			continue
		}
		rest := strings.TrimPrefix(entry, prefix)
		if !strings.HasPrefix(rest, "/") {
			continue
		}
		rest = rest[1:]
		localName, modsStr, ok := strings.Cut(rest, "=")
		if !ok || localName == "" || modsStr == "" {
			continue
		}
		mods := strings.Split(modsStr, ",")
		out = append(out, RootNameAmbiguity{LocalName: localName, Modules: mods})
	}
	return out
}

// ModulesForAmbiguousRootLocal returns owning modules when localName is ambiguous at schema root.
func ModulesForAmbiguousRootLocal(ambiguities []RootNameAmbiguity, localName string) []string {
	for _, a := range ambiguities {
		if a.LocalName == localName {
			return a.Modules
		}
	}
	return nil
}
