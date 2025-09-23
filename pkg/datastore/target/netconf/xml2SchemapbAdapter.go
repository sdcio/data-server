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
	"strings"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
)

// XML2sdcpbConfigAdapter is used to transform the provided XML configuration data into the gnmi-like sdcpb.Notifications.
// This transformation is done via schema information acquired throughout the SchemaServerClient throughout the transformation process.
type XML2sdcpbConfigAdapter struct {
	schemaClient schemaClient.SchemaClientBound
}

// NewXML2sdcpbConfigAdapter constructs a new XML2sdcpbConfigAdapter
func NewXML2sdcpbConfigAdapter(ssc schemaClient.SchemaClientBound) *XML2sdcpbConfigAdapter {
	return &XML2sdcpbConfigAdapter{
		schemaClient: ssc,
	}
}

// Transform takes an etree.Document and transforms the content into a sdcpb based Notification
func (x *XML2sdcpbConfigAdapter) Transform(ctx context.Context, doc *etree.Document) ([]*sdcpb.Notification, error) {
	result := make([]*sdcpb.Notification, 0, len(doc.ChildElements()))
	if doc.Root() == nil {
		return nil, nil
	}

	for _, e := range doc.Root().ChildElements() {
		r := &sdcpb.Notification{}
		err := x.transformRecursive(ctx, e, []*sdcpb.PathElem{}, r, nil)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}

	return result, nil
}

func (x *XML2sdcpbConfigAdapter) transformRecursive(ctx context.Context, e *etree.Element, pelems []*sdcpb.PathElem, result *sdcpb.Notification, tc *TransformationContext) error {
	// add the current tag to the array of path elements that make up the actual abs path
	pelems = append(pelems, &sdcpb.PathElem{Name: e.Tag})

	// retrieve schema
	sr, err := x.schemaClient.GetSchemaSdcpbPath(ctx,
		&sdcpb.Path{
			Elem: pelems,
		},
	)
	if err != nil {
		return err
	}

	switch schema := sr.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		// retrieved schema describes a yang container
		log.Tracef("transforming container %q", e.Tag)
		err = x.transformContainer(ctx, e, sr, pelems, result)
		if err != nil {
			return err
		}

	case *sdcpb.SchemaElem_Field:
		// retrieved schema describes a yang Field
		log.Tracef("transforming field %q", e.Tag)
		err = x.transformField(ctx, e, pelems, schema.Field, result)
		if err != nil {
			return err
		}

	case *sdcpb.SchemaElem_Leaflist:
		// retrieved schema describes a yang LeafList
		log.Tracef("transforming leaflist %q", e.Tag)
		err = x.transformLeafList(ctx, e, pelems, schema.Leaflist, tc)
		if err != nil {
			return err
		}
	}

	return nil
}

// transformContainer transforms an etree.element of a configuration as an update into the provided *sdcpb.Notification.
func (x *XML2sdcpbConfigAdapter) transformContainer(ctx context.Context, e *etree.Element, sr *sdcpb.GetSchemaResponse, pelems []*sdcpb.PathElem, result *sdcpb.Notification) error {
	// copy pelems
	cPElem := make([]*sdcpb.PathElem, 0, len(pelems))
	for _, pe := range pelems {
		npe := &sdcpb.PathElem{
			Name: pe.Name,
			Key:  make(map[string]string),
		}
		for k, v := range pe.GetKey() {
			npe.Key[k] = v
		}
		cPElem = append(cPElem, npe)
	}

	cs := sr.GetSchema().GetContainer()
	// add keys to path elem
	for _, ls := range cs.GetKeys() {
		if cPElem[len(cPElem)-1].Key == nil {
			cPElem[len(cPElem)-1].Key = map[string]string{}
		}
		tv, err := utils.Convert(e.FindElement("./"+ls.Name).Text(), ls.Type)
		if err != nil {
			return err
		}

		cPElem[len(cPElem)-1].Key[ls.Name] = tv.ToString()
	}

	ntc := NewTransformationContext(cPElem)

	// continue with all children
	for _, ce := range e.ChildElements() {
		err := x.transformRecursive(ctx, ce, cPElem, result, ntc)
		if err != nil {
			return err
		}
	}

	leafListUpdates := ntc.Close()
	result.Update = append(result.Update, leafListUpdates...)

	return nil
}

func (x *XML2sdcpbConfigAdapter) resolveSchemaLeafType(ctx context.Context, slt *sdcpb.SchemaLeafType, pelems []*sdcpb.PathElem) (*sdcpb.SchemaLeafType, error) {
	// TODO: can we swap out this logic for slt.LeafrefTargetType?
	schemaLeafType := slt
	for schemaLeafType.GetLeafref() != "" {
		path, err := utils.NormalizedAbsPath(schemaLeafType.Leafref, pelems)
		if err != nil {
			return nil, err
		}

		schema, err := x.schemaClient.GetSchemaSdcpbPath(ctx, path)
		if err != nil {
			return nil, err
		}

		switch se := schema.GetSchema().GetSchema().(type) {
		case *sdcpb.SchemaElem_Leaflist:
			schemaLeafType = se.Leaflist.GetType()
		case *sdcpb.SchemaElem_Field:
			schemaLeafType = se.Field.GetType()
		default:
			return nil, fmt.Errorf("leafref [%s] has non-field or leaflist target type [%T]", slt.GetLeafref(), se)
		}
	}
	return schemaLeafType, nil
}

// transformField transforms an etree.element of a configuration as an update into the provided *sdcpb.Notification.
func (x *XML2sdcpbConfigAdapter) transformField(ctx context.Context, e *etree.Element, pelems []*sdcpb.PathElem, ls *sdcpb.LeafSchema, result *sdcpb.Notification) error {
	schemaLeafType, err := x.resolveSchemaLeafType(ctx, ls.GetType(), pelems)
	if err != nil {
		return fmt.Errorf("failed to resolve type of node %s: %w", e.GetPath(), err)
	}

	// process terminal values
	tv, err := utils.Convert(e.Text(), schemaLeafType)
	if err != nil {
		return fmt.Errorf("unable to convert value [%s] at path [%s] according to SchemaLeafType [%+v]: %w", e.Text(), e.GetPath(), schemaLeafType, err)
	}
	// copy pathElems
	npelem := make([]*sdcpb.PathElem, 0, len(pelems))
	for _, pe := range pelems {
		npelem = append(npelem, &sdcpb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	// create sdcpb.update
	u := &sdcpb.Update{
		Path: &sdcpb.Path{
			Elem: npelem,
		},
		Value: tv,
	}
	result.Update = append(result.Update, u)
	return nil
}

// transformLeafList processes LeafList entries. These will be store in the TransformationContext.
// A new TransformationContext is created when entering a new container. And the appropriate actions are taken when a container is exited.
// Meaning the LeafLists will then be transformed into a single update with a sdcpb.TypedValue_LeaflistVal with all the values.
func (x *XML2sdcpbConfigAdapter) transformLeafList(ctx context.Context, e *etree.Element, pelems []*sdcpb.PathElem, lls *sdcpb.LeafListSchema, tc *TransformationContext) error {
	slt, err := x.resolveSchemaLeafType(ctx, lls.GetType(), pelems)
	if err != nil {
		return fmt.Errorf("failed to resolve type of node %s: %w", e.GetPath(), err)
	}

	// process terminal values
	data := strings.TrimSpace(e.Text())

	tv, err := utils.Convert(data, slt)
	if err != nil {
		return fmt.Errorf("failed to convert value %s to type %s: %w", data, slt.Type, err)
	}

	name := pelems[len(pelems)-1].Name
	err = tc.AddLeafListEntry(name, tv)
	return err
}
