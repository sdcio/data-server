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

package types

import (
	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// GnmiSetPlan holds a pre-encoded gNMI Set payload.
// Replace intent is expressed as module-level delete entries in Deletes
// followed by the corresponding update entries in Updates, all in one plan.
// There is no Replaces field.
type GnmiSetPlan struct {
	Updates []*sdcpb.Update
	Deletes []*sdcpb.Path
}

// NetconfSetPlan holds a pre-built XML document ready for an EditConfig call.
type NetconfSetPlan struct {
	Doc *etree.Document
}

// SouthboundSetPlan is a discriminated wrapper carrying exactly one of
// GnmiSetPlan or NetconfSetPlan. Callers check which variant is populated
// via the GnmiPlan / NetconfPlan accessors.
type SouthboundSetPlan struct {
	Gnmi    *GnmiSetPlan
	Netconf *NetconfSetPlan
}

// GnmiPlan returns the embedded GnmiSetPlan and true if this plan targets a
// gNMI driver, or nil and false otherwise.
func (p SouthboundSetPlan) GnmiPlan() (*GnmiSetPlan, bool) {
	return p.Gnmi, p.Gnmi != nil
}

// NetconfPlan returns the embedded NetconfSetPlan and true if this plan
// targets a NETCONF driver, or nil and false otherwise.
func (p SouthboundSetPlan) NetconfPlan() (*NetconfSetPlan, bool) {
	return p.Netconf, p.Netconf != nil
}
