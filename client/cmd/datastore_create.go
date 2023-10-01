/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var candidate string
var target string
var syncFile string
var owner string
var priority int32

// datastoreCreateCmd represents the create command
var datastoreCreateCmd = &cobra.Command{
	Use:          "create",
	Short:        "create datastore",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		var tg *sdcpb.Target
		if target != "" {
			b, err := os.ReadFile(target)
			if err != nil {
				return err
			}
			tg = &sdcpb.Target{}
			err = json.Unmarshal(b, tg)
			if err != nil {
				return err
			}
		}
		req := &sdcpb.CreateDataStoreRequest{
			Name: datastoreName,
		}
		if syncFile != "" {
			b, err := os.ReadFile(syncFile)
			if err != nil {
				return err
			}
			sync := &sdcpb.Sync{}
			err = json.Unmarshal(b, sync)
			if err != nil {
				return err
			}
			req.Sync = sync
		}

		switch {
		// create a candidate datastore
		case candidate != "":
			req.Datastore = &sdcpb.DataStore{
				Type:     sdcpb.Type_CANDIDATE,
				Name:     candidate,
				Owner:    owner,
				Priority: priority,
			}
			req.Target = tg
			//create a main datastore
		default:
			req.Schema = &sdcpb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			}
			req.Target = tg
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.CreateDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreCreateCmd)

	datastoreCreateCmd.Flags().StringVarP(&target, "target", "", "", "target definition file")
	datastoreCreateCmd.Flags().StringVarP(&syncFile, "sync", "", "", "target sync definition file")
	datastoreCreateCmd.Flags().StringVarP(&owner, "owner", "", "", "candidate owner")
	datastoreCreateCmd.Flags().Int32VarP(&priority, "priority", "", 0, "candidate priority")

}
