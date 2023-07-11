/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/iptecharch/schema-server/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var updates []string
var replaces []string
var deletes []string

var updatePath string
var replacePath string
var updateFile string
var replaceFile string

// dataSetCmd represents the set command
var dataSetCmd = &cobra.Command{
	Use:          "set",
	Short:        "set data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if candidate == "" {
			return errors.New("a candidate datastore name must be specified with --candidate")
		}
		req, err := buildDataSetRequest()
		if err != nil {
			return err
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
	//
	dataSetCmd.Flags().StringVarP(&replacePath, "replace-path", "", "", "replace path to be used with --replace-file")
	dataSetCmd.Flags().StringVarP(&updatePath, "update-path", "", "", "update path to be used with --update-file")
	dataSetCmd.Flags().StringVarP(&replaceFile, "replace-file", "", "", "replace file containing a JSON or JSON_IETF value")
	dataSetCmd.Flags().StringVarP(&updateFile, "update-file", "", "", "update file containing a JSON or JSON_IETF value")
}

func buildDataSetRequest() (*sdcpb.SetDataRequest, error) {
	req := &sdcpb.SetDataRequest{
		Name: datastoreName,
		Datastore: &sdcpb.DataStore{
			Type: sdcpb.Type_CANDIDATE,
			Name: candidate,
		},
		Update:  make([]*sdcpb.Update, 0),
		Replace: make([]*sdcpb.Update, 0),
		Delete:  make([]*sdcpb.Path, 0),
	}
	if updatePath != "" {
		updPath, err := utils.ParsePath(updatePath)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(updateFile)
		if err != nil {
			return nil, err
		}
		req.Update = append(req.Update, &sdcpb.Update{
			Path: updPath,
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: b,
				},
			},
		})
	} else {
		for _, upd := range updates {
			updSplit := strings.SplitN(upd, ":::", 2)
			if len(updSplit) != 2 {
				return nil, fmt.Errorf("malformed update %q", upd)
			}
			updPath, err := utils.ParsePath(updSplit[0])
			if err != nil {
				return nil, err
			}
			req.Update = append(req.Update, &sdcpb.Update{
				Path: updPath,
				Value: &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_StringVal{
						StringVal: updSplit[1],
					},
				},
			})
		}
	}
	if replacePath != "" {
		repPath, err := utils.ParsePath(replacePath)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(replaceFile)
		if err != nil {
			return nil, err
		}
		req.Replace = append(req.Replace, &sdcpb.Update{
			Path: repPath,
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_JsonVal{
					JsonVal: b,
				},
			},
		})
	} else {
		for _, rep := range replaces {
			repSplit := strings.SplitN(rep, ":::", 2)
			if len(repSplit) != 2 {
				return nil, fmt.Errorf("malformed replace %q", rep)
			}
			repPath, err := utils.ParsePath(repSplit[0])
			if err != nil {
				return nil, err
			}
			req.Replace = append(req.Replace, &sdcpb.Update{
				Path: repPath,
				Value: &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_StringVal{
						StringVal: repSplit[1],
					},
				},
			})
		}
	}
	for _, del := range deletes {
		delPath, err := utils.ParsePath(del)
		if err != nil {
			return nil, err
		}
		req.Delete = append(req.Delete, delPath)
	}
	return req, nil
}
