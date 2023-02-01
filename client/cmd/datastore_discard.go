/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

// datastoreDiscardCmd represents the discard command
var datastoreDiscardCmd = &cobra.Command{
	Use:          "discard",
	Short:        "discard changes made to a candidate datastore",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schema_server.DiscardRequest{
			Name: datastoreName,
			Datastore: &schema_server.DataStore{
				Type: schema_server.Type_CANDIDATE,
				Name: candidate,
			},
			Stay: stay,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.Discard(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreDiscardCmd)
}
