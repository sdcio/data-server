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

var schemaFiles []string
var schemaDirs []string
var schemaExcludes []string

// schemaCreateCmd represents the create command
var schemaCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "create a schema",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.CreateSchemaRequest{
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
			File:      schemaFiles,
			Directory: schemaDirs,
			Exclude:   schemaExcludes,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := schemaClient.CreateSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaCreateCmd)
	schemaCreateCmd.Flags().StringArrayVarP(&schemaFiles, "file", "", []string{}, "path to file containing a YANG module")
	schemaCreateCmd.Flags().StringArrayVarP(&schemaDirs, "dir", "", []string{}, "path to file containing a YANG module dependency")
	schemaCreateCmd.Flags().StringArrayVarP(&schemaExcludes, "exclude", "", []string{}, "regex of modules names to be excluded")
}
