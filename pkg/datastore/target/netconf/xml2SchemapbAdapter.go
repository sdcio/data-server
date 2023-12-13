package netconf

import (
	"context"
	"strings"

	"github.com/beevik/etree"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/iptecharch/data-server/pkg/schema"
)

// XML2sdcpbConfigAdapter is used to transform the provided XML configuration data into the gnmi-like sdcpb.Notifications.
// This transformation is done via schema information acquired throughout the SchemaServerClient throughout the transformation process.
type XML2sdcpbConfigAdapter struct {
	schemaClient schema.Client
	schema       *sdcpb.Schema
}

// NewXML2sdcpbConfigAdapter constructs a new XML2sdcpbConfigAdapter
func NewXML2sdcpbConfigAdapter(ssc schema.Client, schema *sdcpb.Schema) *XML2sdcpbConfigAdapter {
	return &XML2sdcpbConfigAdapter{
		schemaClient: ssc,
		schema:       schema,
	}
}

// Transform takes an etree.Document and transforms the content into a sdcpb based Notification
func (x *XML2sdcpbConfigAdapter) Transform(ctx context.Context, doc *etree.Document) (*sdcpb.Notification, error) {
	result := &sdcpb.Notification{}
	err := x.transformRecursive(ctx, doc.Root(), []*sdcpb.PathElem{}, result, nil)

	return result, err
}

func (x *XML2sdcpbConfigAdapter) transformRecursive(ctx context.Context, e *etree.Element, pelems []*sdcpb.PathElem, result *sdcpb.Notification, tc *TransformationContext) error {
	// add the actual path element to the array of path elements that make up the actual abs path
	pelems = append(pelems, &sdcpb.PathElem{Name: e.Tag})

	// retrieve schema
	sr, err := x.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path: &sdcpb.Path{
			Elem: pelems,
		},
		Schema: x.schema,
	})
	if err != nil {
		return err
	}

	switch sr.GetSchema().Schema.(type) {
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
		err = x.transformField(ctx, e, pelems, sr.GetSchema().GetField(), result)
		if err != nil {
			return err
		}

	case *sdcpb.SchemaElem_Leaflist:
		// retrieved schema describes a yang LeafList
		log.Tracef("transforming leaflist %q", e.Tag)
		err = x.transformLeafList(ctx, e, sr, pelems, result, tc)
		if err != nil {
			return err
		}
	}

	return nil
}

// transformContainer transforms an etree.element of a configuration as an update into the provided *sdcpb.Notification.
func (x *XML2sdcpbConfigAdapter) transformContainer(ctx context.Context, e *etree.Element, sr *sdcpb.GetSchemaResponse, pelems []*sdcpb.PathElem, result *sdcpb.Notification) error {
	cs := sr.GetSchema().GetContainer()
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

// transformField transforms an etree.element of a configuration as an update into the provided *sdcpb.Notification.
func (x *XML2sdcpbConfigAdapter) transformField(ctx context.Context, e *etree.Element, pelems []*sdcpb.PathElem, ls *sdcpb.LeafSchema, result *sdcpb.Notification) error {
	// process terminal values

	tv, err := StringElementToTypedValue(e.Text(), ls)
	if err != nil {
		return err
	}

	// create sdcpb.update
	u := &sdcpb.Update{
		Path: &sdcpb.Path{
			Elem: pelems,
		},
		Value: tv,
	}
	result.Update = append(result.Update, u)
	return nil
}

// transformLeafList processes LeafList entries. These will be store in the TransformationContext.
// A new TransformationContext is created when entering a new container. And the appropriate actions are taken when a container is exited.
// Meaning the LeafLists will then be transformed into a single update with a sdcpb.TypedValue_LeaflistVal with all the values.
func (x *XML2sdcpbConfigAdapter) transformLeafList(ctx context.Context, e *etree.Element, sr *sdcpb.GetSchemaResponse, pelems []*sdcpb.PathElem, result *sdcpb.Notification, tc *TransformationContext) error {

	// process terminal values
	data := strings.TrimSpace(e.Text())

	typedval := &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: data}}

	name := pelems[len(pelems)-1].Name
	tc.AddLeafListEntry(name, typedval)
	return nil
}
