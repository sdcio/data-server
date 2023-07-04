package main

import (
	"context"
	"fmt"

	"github.com/iptecharch/data-server/config"
	"github.com/iptecharch/data-server/datastore/target"
	"github.com/iptecharch/data-server/schema"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {

	log.SetLevel(log.TraceLevel)

	ctx := context.TODO()

	targetName := "testbox"

	sbi := &config.SBI{
		Type:    "nc",
		Address: "172.20.20.2",
		Credentials: &config.Creds{
			Username: "root",
			Password: "clab123",
		},
	}

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	}

	cc, err := grpc.DialContext(ctx, "127.0.0.1:55000", opts...)
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	}

	ssc := schema.NewRemoteClient(cc)
	schema := &schemapb.Schema{
		Name:    "junos",
		Vendor:  "Juniper",
		Version: "22.3R1",
	}

	t, err := target.New(ctx, targetName, sbi, ssc, schema, nil)
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	}

	gdr, err := t.Get(ctx, &schemapb.GetDataRequest{
		Name: "foobar",
		Path: []*schemapb.Path{
			//{Elem: []*schemapb.PathElem{{Name: "configuration"}, {Name: "system"}}},
			//{Elem: []*schemapb.PathElem{{Name: "configuration"}, {Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "ge5"}}, {Name: "mtu"}}},
			//{Elem: []*schemapb.PathElem{{Name: "configuration"}, {Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "ge5"}}}},
			// {Elem: []*schemapb.PathElem{{Name: "configuration"}, {Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "ge5"}}}},
		},
		Datastore: &schemapb.DataStore{
			Type: schemapb.Type_MAIN,
		},
	})
	if err != nil {
		fmt.Printf("ERROR: %v", err)
	}

	for _, n := range gdr.GetNotification() {
		for _, u := range n.Update {
			log.Debug(u.String())
		}
		for _, u := range n.Delete {
			log.Debug(u.String())
		}
	}
}
