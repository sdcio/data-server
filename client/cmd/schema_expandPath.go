/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
)

// expandPathCmd represents the expand-path command
var expandPathCmd = &cobra.Command{
	Use:          "expand-path",
	Aliases:      []string{"expand"},
	Short:        "given a path returns all sub-paths",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if configOnly && stateOnly {
			return errors.New("either --config-only or --state-only can be set")
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		cc, err := grpc.DialContext(ctx, addr,
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(24*1024*1024)),
			grpc.WithBlock(),
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			),
		)
		if err != nil {
			return err
		}
		p, err := utils.ParsePath(xpath)
		if err != nil {
			return err
		}
		schemaClient := schemapb.NewSchemaServerClient(cc)
		dt := schemapb.DataType_ALL
		if configOnly {
			dt = schemapb.DataType_CONFIG
		}
		if stateOnly {
			dt = schemapb.DataType_STATE
		}
		req := &schemapb.ExpandPathRequest{
			Path:  p,
			Xpath: asXpath,
			Schema: &schemapb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
			DataType: dt,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := schemaClient.ExpandPath(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		fmt.Fprintf(os.Stderr, "path count: %d | %d\n", len(rsp.GetPath()), len(rsp.GetXpath()))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(expandPathCmd)
	expandPathCmd.Flags().StringVarP(&xpath, "path", "", "", "xpath to expand")
	expandPathCmd.Flags().BoolVarP(&asXpath, "xpath", "", false, "return paths in xpath format")
	expandPathCmd.Flags().BoolVarP(&configOnly, "config-only", "", false, "return paths from the config tree only")
	expandPathCmd.Flags().BoolVarP(&stateOnly, "state-only", "", false, "return paths from the config tree only")
}

var asXpath bool
var configOnly bool
var stateOnly bool
