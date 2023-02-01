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

// schemaToPathCmd represents the to-path command
var schemaToPathCmd = &cobra.Command{
	Use:   "to-path",
	Short: "convert a list of path elements and key values to a valid path",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.ToPathRequest{
			PathElement: pathItems,
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := schemaClient.ToPath(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaToPathCmd)
	schemaToPathCmd.Flags().StringSliceVarP(&pathItems, "cp", "", nil, "path items")
}

var pathItems []string
