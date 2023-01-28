/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list schemas",

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
		schemaList, err := schemaClient.ListSchema(ctx, &schemapb.ListSchemaRequest{})
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(schemaList))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(listCmd)
}
