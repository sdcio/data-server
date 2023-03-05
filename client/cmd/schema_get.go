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
var withDesc bool
var all bool

// schemaGetCmd represents the get command
var schemaGetCmd = &cobra.Command{
	Use:   "get",
	Short: "get schema",
	// SilenceErrors: true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
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
			WithDescription: withDesc,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		if all {
			return handleGetSchemaElems(ctx, schemaClient, req)
		}
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
	schemaGetCmd.PersistentFlags().BoolVarP(&all, "all", "", false, "return all path elems schemas")
	schemaGetCmd.PersistentFlags().BoolVarP(&withDesc, "with-desc", "", false, "include YANG entries descriptions")
}

func handleGetSchemaElems(ctx context.Context, scc schemapb.SchemaServerClient, req *schemapb.GetSchemaRequest) error {
	stream, err := scc.GetSchemaElements(ctx, req)
	if err != nil {
		return err
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
	}
}
