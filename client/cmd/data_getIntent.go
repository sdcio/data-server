/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var intentName string

// dataGetIntentCmd represents the get-intent command
var dataGetIntentCmd = &cobra.Command{
	Use:          "get-intent",
	Short:        "get intent data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		req := &sdcpb.GetIntentRequest{
			Name:   datastoreName,
			Intent: intentName,
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.GetIntent(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataGetIntentCmd)
	dataGetIntentCmd.Flags().StringVarP(&intentName, "intent", "", "", "intent name")
}
