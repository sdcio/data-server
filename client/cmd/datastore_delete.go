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

// datastoreDeleteCmd represents the delete command
var datastoreDeleteCmd = &cobra.Command{
	Use:          "delete",
	Short:        "delete datastore",
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
		req := &schemapb.DeleteDataStoreRequest{
			Name: datastoreName,
		}
		if candidate != "" {
			req.Datastore = &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			}
		}

		rsp, err := dataClient.DeleteDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreDeleteCmd)
}
