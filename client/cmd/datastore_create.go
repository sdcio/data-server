/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
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
		var req *sdcpb.CreateDataStoreRequest
		switch {
		// create a candidate datastore
		case candidate != "":
			req = &sdcpb.CreateDataStoreRequest{
				Name: datastoreName,
				Datastore: &sdcpb.DataStore{
					Type: sdcpb.Type_CANDIDATE,
					Name: candidate,
				},
			}
			//create a main datastore
		default:
			req = &sdcpb.CreateDataStoreRequest{
				Name: datastoreName,
				Schema: &sdcpb.Schema{
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
