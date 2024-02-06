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
	"context"
	"fmt"

	"github.com/beevik/etree"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"

	"github.com/iptecharch/data-server/pkg/schema"
)

const (
	//
	ncBase1_0 = "urn:ietf:params:xml:ns:netconf:base:1.0"
	//
	operationDelete = "delete"
	operationRemove = "remove"
)

// XMLConfigBuilder is used to builds XML configuration or XML Filter documents
// Via the use of a sdcpb.SchemaServerClient and the *sdcpb.Schema Namespace, Key and Type information
// and a valid configuration or filter document can be crafted.
type XMLConfigBuilder struct {
	cfg          *XMLConfigBuilderOpts
	doc          *etree.Document
	schemaClient schema.Client
	schema       *sdcpb.Schema
}

type XMLConfigBuilderOpts struct {
	// HonorNamespace if true, XML tags incorporate their namespace as an attribute.
	HonorNamespace bool
	// OperationWithNamespace if true, enables proper namespacing for the edit-config RPC operation attribute.
	OperationWithNamespace bool
	// UseOperationRemove if true, use NETCONF operation `remove` rather than `delete` in edit-config RPC.
	UseOperationRemove bool
}

// NewXMLConfigBuilder returns a new XMLConfigBuilder instance
func NewXMLConfigBuilder(ssc schema.Client, schema *sdcpb.Schema, cfgOpts *XMLConfigBuilderOpts) *XMLConfigBuilder {
	return &XMLConfigBuilder{
		cfg:          cfgOpts,
		doc:          etree.NewDocument(),
		schemaClient: ssc,
		schema:       schema,
	}
}

// GetDoc returns the XMLConfigBuilder generated XML document in string format.
func (x *XMLConfigBuilder) GetDoc() (string, error) {
	x.doc.Indent(2)
	xdoc, err := x.doc.WriteToString()
	if err != nil {
		return "", err
	}
	return xdoc, nil
}

// Delete adds the given path to the XMLConfigDocument and adds the delete operation
// attribute ( operation="delete" ) to the last element of path p.
func (x *XMLConfigBuilder) Delete(ctx context.Context, p *sdcpb.Path) error {
	// fastForward the XML to the element defined in the path p
	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}
	operName := operationDelete
	operKey := "operation"
	if x.cfg.UseOperationRemove {
		operName = operationRemove
	}
	// add base1.0 as xmlns:nc attr
	if x.cfg.OperationWithNamespace {
		elem.CreateAttr("xmlns:nc", ncBase1_0)
		operKey = "nc:" + operKey
	}
	// add the delete operation attribute
	elem.CreateAttr(operKey, operName)

	return nil
}

// fastForward takes the *sdcpb.Path p and iterates through the xml document along this path.
// It will create all the missing elements along the path in the document, as well as creating the provided
// key elements. Finally the element that represents the last part of the path is returned to the caller.
// If x.cfg.honorNamespace is set to true, it will also add "xmlns" attributes.
func (x *XMLConfigBuilder) fastForward(ctx context.Context, p *sdcpb.Path) (*etree.Element, error) {
	parent := &x.doc.Element
	actualNamespace := ""
	for peIdx, pe := range p.Elem {

		// generate an etree.Path from the path element
		// this is to find the next level xml element
		path, err := pathElem2EtreePath(pe)
		if err != nil {
			return nil, err
		}
		var newChild *etree.Element
		if newChild = parent.FindElementPath(path); newChild == nil {
			namespaceUri, err := x.resolveNamespace(ctx, p, peIdx)
			if err != nil {
				return nil, err
			}

			// if there is no such element, create it
			newChild = parent.CreateElement(pe.Name)
			if x.cfg.HonorNamespace && namespaceUri != actualNamespace {
				newChild.CreateAttr("xmlns", namespaceUri)
			}
			// with all its keys
			for k, v := range pe.Key {
				keyElem := newChild.CreateElement(k)
				keyElem.CreateText(v)
			}
		}
		//// prepare next iteration
		// get default namespace definition of actual element, if unset default to actualNamespace
		actualNamespace = newChild.SelectAttrValue("xmlns", actualNamespace)

		// newChild will be parent in next iteration
		parent = newChild
	}
	return parent, nil
}

// AddValue adds the given *sdcpb.TypedValue v under the given *sdcpb.Path p into the xml document
func (x *XMLConfigBuilder) AddValue(ctx context.Context, p *sdcpb.Path, v *sdcpb.TypedValue) error {
	// fastForward the XML to the element defined in the path p
	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}
	// get the string representation of the value
	// cause xml is all string
	value, err := valueAsString(v)
	if err != nil {
		return err
	}
	// set the respective value
	// use SetText instead of CreateText to properly handle paths
	// with a key as leaf.
	elem.SetText(value)

	return nil
}

// AddElements add a given *sdcpb.Path p to the xml document. This will not define a terminal value
// under the given path. This is usefull when creating Netconf Filters where you provide an xml document
// pointing to branches that you're intrested in receiving.
func (x *XMLConfigBuilder) AddElements(ctx context.Context, p *sdcpb.Path) error {
	_, err := x.fastForward(ctx, p)
	return err
}

// resolveNamespace takes a *sdcpb.Path and a pathElementIndex (peIdx). It returns the namespace of
// the element on position peIdx of the *sdcpb.path p
func (x *XMLConfigBuilder) resolveNamespace(ctx context.Context, p *sdcpb.Path, peIdx int) (string, error) {

	if peIdx+1 > len(p.Elem) {
		return "", fmt.Errorf("peIdx exceeds limit %d for path %s", len(p.Elem), p.String())
	}

	// Perform schema queries
	sr, err := x.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path: &sdcpb.Path{
			Elem:   p.Elem[:peIdx+1],
			Origin: p.Origin,
			Target: p.Target,
		},
		Schema: x.schema,
	})
	if err != nil {
		return "", err
	}

	// deduce namespace from SchemaRequest
	return getNamespaceFromGetSchemaResponse(sr), nil
}
