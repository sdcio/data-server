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

// toPathCmd represents the to-path command
var toPathCmd = &cobra.Command{
	Use:   "to-path",
	Short: "A brief description of your command",
	RunE: func(cmd *cobra.Command, args []string) error {
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
		schemaClient := schemapb.NewSchemaServerClient(cc)
		rsp, err := schemaClient.ToPath(ctx, &schemapb.ToPathRequest{
			PathElement: pathItems,
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
		})
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(toPathCmd)
	toPathCmd.Flags().StringSliceVarP(&pathItems, "cp", "", nil, "path items")
}

var pathItems []string
