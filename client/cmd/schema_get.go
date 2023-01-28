/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
)

var xpath string

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "get schema",
	// SilenceErrors: true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := utils.ParsePath(xpath)
		if err != nil {
			return err
		}
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
		rsp, err := schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
			Path: p,
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
	schemaCmd.AddCommand(getCmd)

	getCmd.PersistentFlags().StringVarP(&xpath, "path", "p", "", "xpath")
}
