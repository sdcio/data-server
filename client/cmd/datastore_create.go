/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
)

var candidate string

// datastoreCreateCmd represents the create command
var datastoreCreateCmd = &cobra.Command{
	Use:          "create",
	Short:        "create datastore",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		cc, err := grpc.DialContext(ctx, addr,
			grpc.WithBlock(),
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			),
		)
		if err != nil {
			return err
		}
		dataClient := schemapb.NewDataServerClient(cc)
		var req *schemapb.CreateDataStoreRequest
		switch {
		// create a candidate datastore
		case candidate != "":
			req = &schemapb.CreateDataStoreRequest{
				Name: datastoreName,
				Datastore: &schemapb.DataStore{
					Type: schemapb.Type_CANDIDATE,
					Name: candidate,
				},
			}
			//create a main datastore
		default:
			req = &schemapb.CreateDataStoreRequest{
				Name: datastoreName,
				Schema: &schemapb.Schema{
					Name:    schemaName,
					Vendor:  schemaVendor,
					Version: schemaVersion,
				},
			}
		}
		rsp, err := dataClient.CreateDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreCreateCmd)
}
