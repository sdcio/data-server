package netconf

import (
	"context"

	"github.com/beevik/etree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type XMLConfigBuilder struct {
	doc            *etree.Document
	schemaClient   schemapb.SchemaServerClient
	schema         *schemapb.Schema
	honorNamespace bool
}

func NewXMLConfigBuilder(ssc schemapb.SchemaServerClient, schema *schemapb.Schema, honorNamespace bool) *XMLConfigBuilder {
	return &XMLConfigBuilder{
		doc:            etree.NewDocument(),
		schemaClient:   ssc,
		schema:         schema,
		honorNamespace: honorNamespace,
	}
}

func (x *XMLConfigBuilder) GetDoc() (string, error) {
	//x.addNamespaceDefs()
	x.doc.Indent(2)
	xdoc, err := x.doc.WriteToString()
	if err != nil {
		return "", err
	}
	return xdoc, nil
}

func (x *XMLConfigBuilder) Delete(ctx context.Context, p *schemapb.Path) error {

	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}
	// add the delete operation
	elem.CreateAttr("operation", "delete")

	return nil
}

func (x *XMLConfigBuilder) fastForward(ctx context.Context, p *schemapb.Path) (*etree.Element, error) {
	elem := &x.doc.Element

	actualNamespace := ""

	for peIdx, pe := range p.Elem {

		//namespace := x.namespaces.Resolve(namespaceUri)

		// generate an xpath from the path element
		// this is to find the next level xml element
		path, err := pathElem2Xpath(pe, "")
		if err != nil {
			return nil, err
		}
		var nextElem *etree.Element
		if nextElem = elem.FindElementPath(path); nextElem == nil {

			namespaceUri, err := x.ResolveNamespace(ctx, p, peIdx)
			if err != nil {
				return nil, err
			}

			// if there is no such element, create it
			//elemName := toNamespacedName(pe.Name, namespace)
			nextElem = elem.CreateElement(pe.Name)
			if x.honorNamespace && namespaceUri != actualNamespace {
				nextElem.CreateAttr("xmlns", namespaceUri)
			}
			// with all its keys
			for k, v := range pe.Key {
				//keyNamespaced := toNamespacedName(k, namespace)
				keyElem := nextElem.CreateElement(k)
				keyElem.CreateText(v)
			}
		}
		// prepare next iteration
		elem = nextElem
		xmlns := elem.SelectAttrValue("xmlns", "")
		if xmlns != "" {
			actualNamespace = elem.Space
		}
	}
	return elem, nil
}

func (x *XMLConfigBuilder) Add(ctx context.Context, p *schemapb.Path, v *schemapb.TypedValue) error {

	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}

	value, err := valueAsString(v)
	if err != nil {
		return err
	}
	elem.CreateText(value)

	return nil
}

func (x *XMLConfigBuilder) AddElement(ctx context.Context, p *schemapb.Path) (*etree.Element, error) {
	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return nil, err
	}
	return elem, nil
}

func (x *XMLConfigBuilder) ResolveNamespace(ctx context.Context, p *schemapb.Path, peIdx int) (namespaceUri string, err error) {
	// Perform schema queries
	sr, err := x.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Path: &schemapb.Path{
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
