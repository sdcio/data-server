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

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "delete schema",

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.DeleteSchemaRequest{
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := schemaClient.DeleteSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(deleteCmd)
}
