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

// dataDiffCmd represents the diff command
var dataDiffCmd = &cobra.Command{
	Use:          "diff",
	Short:        "diff candidate and its baseline",
	SilenceUsage: true,
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
		dataClient := schemapb.NewDataServerClient(cc)
		diffReq := &schemapb.DiffRequest{
			Name: datastoreName,
			DataStore: &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			},
		}
		rsp, err := dataClient.Diff(ctx, diffReq)
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataDiffCmd)
}
