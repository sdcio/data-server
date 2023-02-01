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
	"google.golang.org/protobuf/encoding/prototext"
)

var xpath string

// schemaGetCmd represents the get command
var schemaGetCmd = &cobra.Command{
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
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.GetSchemaRequest{
			Path: p,
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := schemaClient.GetSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaGetCmd)

	schemaGetCmd.PersistentFlags().StringVarP(&xpath, "path", "p", "", "xpath")
}
