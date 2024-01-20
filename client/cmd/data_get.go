/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iptecharch/schema-server/pkg/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var paths []string
var dataType string
var format string
var intended bool

// dataGetCmd represents the get command
var dataGetCmd = &cobra.Command{
	Use:          "get",
	Short:        "get data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if candidate != "" && intended {
			return fmt.Errorf("cannot set a candidate name and intended store at the same time")
		}
		var dt sdcpb.DataType
		switch dataType {
		case "ALL":
		case "CONFIG":
			dt = sdcpb.DataType_CONFIG
		case "STATE":
			dt = sdcpb.DataType_STATE
		default:
			return fmt.Errorf("invalid flag value --type %s", dataType)
		}

		req := &sdcpb.GetDataRequest{
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
			req.Datastore = &sdcpb.DataStore{
				Type: sdcpb.Type_CANDIDATE,
				Name: candidate,
			}
		}
		if intended {
			req.Datastore = &sdcpb.DataStore{
				Type:     sdcpb.Type_INTENDED,
				Owner:    owner,
				Priority: priority,
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
		stream, err := dataClient.GetData(ctx, req)
		if err != nil {
			return err
		}
		count := 0
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					break
				}
				return err
			}
			count++
			switch format {
			case "json":
				b, err := json.MarshalIndent(rsp, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(b))
			case "flat":
				for _, n := range rsp.GetNotification() {
					for _, upd := range n.GetUpdate() {
						p := utils.ToXPath(upd.GetPath(), false)
						// upd.GetValue()
						fmt.Printf("%s: %s\n", p, upd.GetValue())
					}
				}

			default:
				fmt.Println(prototext.Format(rsp))
			}

		}

		fmt.Println("num notifications:", count)
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataGetCmd)
	dataGetCmd.Flags().StringArrayVarP(&paths, "path", "", []string{}, "get path(s)")
	dataGetCmd.Flags().StringVarP(&dataType, "type", "", "ALL", "data type, one of: ALL, CONFIG, STATE")
	dataGetCmd.Flags().StringVarP(&format, "format", "", "", "print format, '', 'flat' or 'json'")
	// intended store
	dataGetCmd.Flags().BoolVarP(&intended, "intended", "", false, "get data from intended store")
	dataGetCmd.Flags().StringVarP(&owner, "owner", "", "", "intended store owner to query")
	dataGetCmd.Flags().Int32VarP(&priority, "priority", "", 0, "intended store priority")
}
