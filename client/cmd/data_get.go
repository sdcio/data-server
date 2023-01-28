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

var paths []string

// dataGetCmd represents the get command
var dataGetCmd = &cobra.Command{
	Use:          "get",
	Short:        "get data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
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
		req := &schemapb.GetDataRequest{
			Name: datastoreName,
			// DataStore: &schemapb.DataStore{
			// 	Type: schemapb.Type_MAIN,
			// 	Name: datastoreName,
			// },
		}
		for _, p := range paths {
			xp, err := utils.ParsePath(p)
			if err != nil {
				return err
			}
			req.Path = append(req.Path, xp)
		}
		if candidate != "" {
			req.DataStore = &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			}
		}

		rsp, err := dataClient.GetData(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataGetCmd)
	dataGetCmd.Flags().StringArrayVarP(&paths, "path", "", []string{}, "get path(s)")
}
