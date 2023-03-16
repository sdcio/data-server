package netconf

import (
	"context"
	"strings"

	"github.com/beevik/etree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

// XML2SchemapbConfigAdapter is used to transform the provided XML configuration data into the gnmi-like schemapb.Notifications.
// This transformation is done via schema information aquired throughout the SchemaServerClient throughout the transformation process.
type XML2SchemapbConfigAdapter struct {
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
}

// NewXML2SchemapbConfigAdapter constructs a new XML2SchemapbConfigAdapter
func NewXML2SchemapbConfigAdapter(ssc schemapb.SchemaServerClient, schema *schemapb.Schema) *XML2SchemapbConfigAdapter {
	return &XML2SchemapbConfigAdapter{
		schemaClient: ssc,
		schema:       schema,
	}
}

// Transform takes an etree.Document and transforms the content into a schemapb based Notification
func (x *XML2SchemapbConfigAdapter) Transform(ctx context.Context, doc *etree.Document) *schemapb.Notification {
	result := &schemapb.Notification{}
	x.transformRecursive(ctx, doc.Root(), []*schemapb.PathElem{}, result, nil)
	return result
}

func (x *XML2SchemapbConfigAdapter) transformRecursive(ctx context.Context, e *etree.Element, pelems []*schemapb.PathElem, result *schemapb.Notification, tc *TransformationContext) error {
	// add the actual path element to the array of path elements that make up the actual abs path
	pelems = append(pelems, &schemapb.PathElem{Name: e.Tag})

	// retrieve schema
	sr, err := x.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Path: &schemapb.Path{
			Elem: pelems,
		},
		Schema: x.schema,
	})
	if err != nil {
		return err
	}

	switch sr.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		// retrieved schema describes a yang container
		log.Tracef("transforming container %q", e.Tag)
		err = x.transformContainer(ctx, e, sr, pelems, result)
		if err != nil {
			return err
		}

	case *schemapb.GetSchemaResponse_Field:
		// retrieved schema describes a yang Field
		log.Tracef("transforming field %q", e.Tag)
		err = x.transformField(ctx, e, pelems, sr.GetField(), result)
		if err != nil {
			return err
		}

	case *schemapb.GetSchemaResponse_Leaflist:
		// retrieved schema describes a yang LeafList
		log.Tracef("transforming leaflist %q", e.Tag)
		err = x.transformLeafList(ctx, e, sr, pelems, result, tc)
		if err != nil {
			return err
		}
	}

	return nil
}

// transformContainer transforms an etree.element of a configuration as an update into the provided *schemapb.Notification.
func (x *XML2SchemapbConfigAdapter) transformContainer(ctx context.Context, e *etree.Element, sr *schemapb.GetSchemaResponse, pelems []*schemapb.PathElem, result *schemapb.Notification) error {
	cs := sr.GetContainer()
	for _, ls := range cs.Keys {
		pelem := pelems[len(pelems)-1]
		if pelem.Key == nil {
			pelem.Key = map[string]string{}
		}
		pelem.Key[ls.Name] = e.FindElement("./" + ls.Name).Text()
	}

	ntc := NewTransformationContext(pelems)

	// continue with all child
	for _, ce := range e.ChildElements() {
		err := x.transformRecursive(ctx, ce, pelems, result, ntc)
		if err != nil {
			return err
		}
	}

	leafListUpdates := ntc.Close()
	result.Update = append(result.Update, leafListUpdates...)

	return nil
}

// transformField transforms an etree.element of a configuration as an update into the provided *schemapb.Notification.
func (x *XML2SchemapbConfigAdapter) transformField(ctx context.Context, e *etree.Element, pelems []*schemapb.PathElem, ls *schemapb.LeafSchema, result *schemapb.Notification) error {
	// process terminal values
	//data := strings.TrimSpace(e.Text())

	tv, err := StringElementToTypedValue(e.Text(), ls)
	if err != nil {
		return err
	}

	// create schemapb.update
	u := &schemapb.Update{
		Path: &schemapb.Path{
			Elem: pelems,
		},
		Value: tv,
	}
	result.Update = append(result.Update, u)
	return nil
}

// transformLeafList processes LeafList entries. These will be store in the TransformationContext.
// A new TransformationContext is created when entering a new container. And the appropriate actions are taken when a container is exited.
// Meaning the LeafLists will then be transformed into a single update with a schemapb.TypedValue_LeaflistVal with all the values.
func (x *XML2SchemapbConfigAdapter) transformLeafList(ctx context.Context, e *etree.Element, sr *schemapb.GetSchemaResponse, pelems []*schemapb.PathElem, result *schemapb.Notification, tc *TransformationContext) error {

	// process terminal values
	data := strings.TrimSpace(e.Text())

	typedval := &schemapb.TypedValue{Value: &schemapb.TypedValue_StringVal{StringVal: data}}

	name := pelems[len(pelems)-1].Name
	tc.AddLeafListEntry(name, typedval)
	return nil
}
