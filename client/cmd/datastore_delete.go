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

// datastoreDeleteCmd represents the delete command
var datastoreDeleteCmd = &cobra.Command{
	Use:          "delete",
	Short:        "delete datastore",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		req := &sdcpb.DeleteDataStoreRequest{
			Name: datastoreName,
		}
		if candidate != "" {
			req.Datastore = &sdcpb.DataStore{
				Type: sdcpb.Type_CANDIDATE,
				Name: candidate,
			}
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.DeleteDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreDeleteCmd)
}
