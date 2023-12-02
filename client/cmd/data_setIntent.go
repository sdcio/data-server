/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"

	"github.com/iptecharch/data-server/pkg/utils"
)

// var paths []string
// var dataType string
// var format string
// var intended bool
// var intentName string
var deleteFlag bool
var intentDefinition string

// dataSetIntentCmd represents the set-intent command
var dataSetIntentCmd = &cobra.Command{
	Use:          "set-intent",
	Short:        "set intent data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if deleteFlag && intentDefinition != "" {
			return errors.New("cannot set an intent body and the delete flag at the same time")
		}
		req := &sdcpb.SetIntentRequest{
			Name:     datastoreName,
			Intent:   intentName,
			Priority: priority,
			Update:   make([]*sdcpb.Update, 0),
		}
		if deleteFlag {
			req.Delete = true
		}
		if intentDefinition != "" {
			b, err := os.ReadFile(intentDefinition)
			if err != nil {
				return err
			}
			intentDefs := make([]*intentDef, 0)
			err = json.Unmarshal(b, &intentDefs)
			if err != nil {
				return err
			}
			for _, idef := range intentDefs {
				p, err := utils.ParsePath(idef.Path)
				if err != nil {
					return err
				}
				bb, err := json.Marshal(idef.Value)
				if err != nil {
					return err
				}
				req.Update = append(req.Update, &sdcpb.Update{
					Path: p,
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_JsonVal{JsonVal: bb},
					},
				})
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
		rsp, err := dataClient.SetIntent(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataSetIntentCmd)
	dataSetIntentCmd.Flags().StringVarP(&intentName, "intent", "", "", "intent name")
	dataSetIntentCmd.Flags().StringVarP(&intentDefinition, "body", "", "", "intent body")
	dataSetIntentCmd.Flags().Int32VarP(&priority, "priority", "", 0, "intent priority")
	dataSetIntentCmd.Flags().BoolVarP(&deleteFlag, "delete", "", false, "delete intent")
}

type intentDef struct {
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}
