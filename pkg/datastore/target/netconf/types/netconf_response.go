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

import "github.com/beevik/etree"

type NetconfResponse struct {
	Doc *etree.Document
}

func NewNetconfResponse(doc *etree.Document) *NetconfResponse {
	return &NetconfResponse{
		Doc: doc,
	}
}

func (nr *NetconfResponse) DocAsString(indented bool) string {
	if nr.Doc == nil {
		return ""
	}
	doc := nr.Doc
	if !indented {
		doc = doc.Copy()
		doc.Unindent()
	}
	s, _ := doc.WriteToString()
	return s
}
