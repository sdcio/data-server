package target

import (
	"context"
	"time"

	"github.com/iptecharch/data-server/config"
	"github.com/iptecharch/data-server/datastore/target/netconf"
	"github.com/iptecharch/data-server/datastore/target/netconf/driver/scrapligo"
	"github.com/iptecharch/data-server/schema"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type ncTarget struct {
	name         string
	driver       netconf.Driver
	schemaClient schema.Client
	schema       *sdcpb.Schema
	sbi          *config.SBI
}

func newNCTarget(_ context.Context, name string, cfg *config.SBI, schemaClient schema.Client, schema *sdcpb.Schema) (*ncTarget, error) {

	// create a new
	d, err := scrapligo.NewScrapligoNetconfTarget(cfg)
	if err != nil {
		return nil, err
	}

	return &ncTarget{
		name:         name,
		driver:       d,
		schemaClient: schemaClient,
		schema:       schema,
		sbi:          cfg,
	}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
	var source string

	switch req.Datastore.Type {
	case sdcpb.Type_MAIN:
		source = "running"
	case sdcpb.Type_CANDIDATE:
		source = "candidate"
	}

	// init a new XMLConfigBuilder for the pathfilter
	pathfilterXmlBuilder := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

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

	// init an XML2sdcpbConfigAdapter used to convert the netconf xml config to a sdcpb.Notification
	data := netconf.NewXML2sdcpbConfigAdapter(t.schemaClient, t.schema)

	// start transformation, which yields the sdcpb_Notification
	noti := data.Transform(ctx, ncResponse.Doc)

	// building the resulting sdcpb.GetDataResponse struct
	result := &sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{
			noti,
		},
	}
	return result, nil
}

func (t *ncTarget) Set(ctx context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {

	xmlCBDelete := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

	// iterate over the delete array
	for _, d := range req.Delete {
		xmlCBDelete.Delete(ctx, d)
	}

	// iterate over the replace array
	// ATTENTION: This is not implemented intentionally, since it is expected,
	//  	that the datastore will only come up with deletes and updates.
	// 		actual replaces will be resolved to deletes and updates by the datastore
	// 		also replaces would only really make sense with jsonIETF encoding, where
	// 		an entire branch is replaces, on single values this is covered via an
	// 		update.
	//
	// for _, r := range req.Replace {
	// }
	//

	xmlCBAdd := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

	// iterate over the update array
	for _, u := range req.Update {
		xmlCBAdd.Add(ctx, u.Path, u.Value)
	}

	// first apply the deletes before the adds
	for _, xml := range []*netconf.XMLConfigBuilder{xmlCBDelete, xmlCBAdd} {
		// finally retrieve the xml config as string
		xdoc, err := xml.GetDoc()
		if err != nil {
			return nil, err
		}

		// if there was no data in the xml document, continue
		if len(xdoc) == 0 {
			continue
		}

		log.Debugf("datastore %s XML:\n%s\n", t.name, xdoc)

		// edit the config
		_, err = t.driver.EditConfig("candidate", xdoc)
		if err != nil {
			log.Errorf("datastore %s failed edit-config: %v", t.name, err)
			err2 := t.driver.Discard()
			if err != nil {
				// log failed discard
				log.Errorf("failed with %v while discarding pending changes after error %v", err2, err)
			}
			return nil, err
		}

	}
	log.Infof("datastore %s: committing changes on target", t.name)
	// commit the config
	err := t.driver.Commit()
	if err != nil {
		return nil, err
	}
	return &sdcpb.SetDataResponse{
		Timestamp: time.Now().UnixNano(),
	}, nil
}

func (t *ncTarget) Subscribe() {}

func (t *ncTarget) Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.sbi.Address)
	log.Infof("sync still is a NOOP on netconf targets")
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {
	t.driver.Close()
}
