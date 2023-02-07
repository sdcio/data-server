/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var updates []string
var replaces []string
var deletes []string

// dataSetCmd represents the set command
var dataSetCmd = &cobra.Command{
	Use:          "set",
	Short:        "set data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if candidate == "" {
			return errors.New("a candidate datastore name must be specified with --candidate")
		}
		req := &schemapb.SetDataRequest{
			Name: datastoreName,
			Datastore: &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			},
		}
		for _, upd := range updates {
			updSplit := strings.SplitN(upd, ":::", 2)
			if len(updSplit) != 2 {
				return fmt.Errorf("malformed update %q", upd)
			}
			updPath, err := utils.ParsePath(updSplit[0])
			if err != nil {
				return err
			}
			req.Update = append(req.Update, &schemapb.Update{
				Path: updPath,
				Value: &schemapb.TypedValue{
					Value: &schemapb.TypedValue_StringVal{
						StringVal: updSplit[1],
					},
				},
			})
		}
		for _, rep := range replaces {
			repSplit := strings.SplitN(rep, ":::", 2)
			if len(repSplit) != 2 {
				return fmt.Errorf("malformed replace %q", rep)
			}
			repPath, err := utils.ParsePath(repSplit[0])
			if err != nil {
				return err
			}
			req.Replace = append(req.Replace, &schemapb.Update{
				Path: repPath,
				Value: &schemapb.TypedValue{
					Value: &schemapb.TypedValue_StringVal{
						StringVal: repSplit[1],
					},
				},
			})
		}
		for _, del := range deletes {
			delPath, err := utils.ParsePath(del)
			if err != nil {
				return err
			}
			req.Delete = append(req.Delete, delPath)
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.SetData(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataSetCmd)
	dataSetCmd.Flags().StringArrayVarP(&updates, "update", "", []string{}, "update path and value separated by a ':::'")
	dataSetCmd.Flags().StringArrayVarP(&replaces, "replace", "", []string{}, "replace path and value separated by a ':::'")
	dataSetCmd.Flags().StringArrayVarP(&deletes, "delete", "", []string{}, "delete path")
}
