/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// schemaListCmd represents the list command
var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "list schemas",

	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.ListSchemaRequest{}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		schemaList, err := schemaClient.ListSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(schemaList))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)
}
