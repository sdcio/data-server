package target

import (
	"context"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/target/netconf"
	"github.com/iptecharch/schema-server/datastore/target/netconf/driver/scrapligo"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
)

type ncTarget struct {
	driver       netconf.Driver
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
	sbi          *config.SBI
}

func newNCTarget(_ context.Context, cfg *config.SBI, schemaClient schemapb.SchemaServerClient, schema *schemapb.Schema) (*ncTarget, error) {

	// create a new
	d, err := scrapligo.NewScrapligoNetconfTarget(cfg)
	if err != nil {
		return nil, err
	}

	return &ncTarget{
		driver:       d,
		schemaClient: schemaClient,
		schema:       schema,
		sbi:          cfg,
	}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	source := "running"

	// init a new XMLConfigBuilder for the pathfilter
	pathfilterXmlBuilder := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, false)

	// add all the requested paths to the document
	for _, p := range req.Path {
		_, err := pathfilterXmlBuilder.AddElement(ctx, p)
		if err != nil {
			return nil, err
		}
	}

	// retrieve the xml filter as string
	filterDoc, err := pathfilterXmlBuilder.GetDoc()
	if err != nil {
		return nil, err
	}
	log.Debugf("netconf filter:\n%s", filterDoc)

	// execute the GetConfig rpc
	ncResponse, err := t.driver.GetConfig(source, filterDoc)
	if err != nil {
		return nil, err
	}

	log.Debugf("netconf response:\n%s", ncResponse.DocAsString())

	// init an XML2SchemapbConfigAdapter used to convert the netconf xml config to a schemapb.Notification
	data := netconf.NewXML2SchemapbConfigAdapter(t.schemaClient, t.schema)

	// start transformation, which yields the schemapb_Notificatio
	noti := data.Transform(ctx, ncResponse.Doc)

	// building the resulting schemapb.GetDataResponse struct
	result := &schemapb.GetDataResponse{
		Notification: []*schemapb.Notification{
			noti,
		},
	}
	return result, nil
}

func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {

	xmlCB := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, false)

	// iterate over the update array
	for _, u := range req.Update {
		xmlCB.Add(ctx, u.Path, u.Value)
	}

	// iterate over the delete array
	for _, d := range req.Delete {
		xmlCB.Delete(ctx, d)
	}

	// iterate over the replace array
	for _, r := range req.Replace {
		// TODO: Needs Implementation
		_ = r
	}

	// TODO: take care on interferrance of Delete vs. Update

	// finally retrieve the xml config as string
	xdoc, err := xmlCB.GetDoc()
	if err != nil {
		return nil, err
	}

	// edit the config
	_, err = t.driver.EditConfig("candidate", xdoc)
	if err != nil {
		return nil, err
	}

	// commit the config
	err = t.driver.Commit()
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.sbi.Address)
	log.Infof("sync still is a NOOP on netconf targets")
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {
	t.driver.Close()
}
