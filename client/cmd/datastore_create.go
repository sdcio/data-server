/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
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
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
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
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.CreateDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreCreateCmd)
}
