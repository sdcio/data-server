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

var paths []string
var dataType string

// dataGetCmd represents the get command
var dataGetCmd = &cobra.Command{
	Use:          "get",
	Short:        "get data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		var dt schemapb.DataType
		switch dataType {
		case "ALL":
		case "CONFIG":
			dt = schemapb.DataType_CONFIG
		case "STATE":
			dt = schemapb.DataType_STATE
		default:
			return fmt.Errorf("invalid flag value --type %s", dataType)
		}

		req := &schemapb.GetDataRequest{
			Name:     datastoreName,
			DataType: dt,
		}
		for _, p := range paths {
			xp, err := utils.ParsePath(p)
			if err != nil {
				return err
			}
			req.Path = append(req.Path, xp)
		}
		if candidate != "" {
			req.Datastore = &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			}
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.GetData(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataGetCmd)
	dataGetCmd.Flags().StringArrayVarP(&paths, "path", "", []string{}, "get path(s)")
	dataGetCmd.Flags().StringVarP(&dataType, "type", "", "ALL", "data type, one of: ALL, CONFIG, STATE")
}
