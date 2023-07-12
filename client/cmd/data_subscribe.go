/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iptecharch/schema-server/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var sampleInterval time.Duration
var mode string
var streamMode string

// dataSubscribeCmd represents the subscribe command
var dataSubscribeCmd = &cobra.Command{
	Use:          "subscribe",
	Aliases:      []string{"sub"},
	Short:        "subscribe to data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		var dt sdcpb.DataType
		switch strings.ToUpper(dataType) {
		case "ALL":
		case "CONFIG":
			dt = sdcpb.DataType_CONFIG
		case "STATE":
			dt = sdcpb.DataType_STATE
		default:
			return fmt.Errorf("invalid flag value --type %s", dataType)
		}

		subscriptions := make([]*sdcpb.Subscription, 0, len(paths))

		for _, p := range paths {
			xp, err := utils.ParsePath(p)
			if err != nil {
				return err
			}

			subc := &sdcpb.Subscription{
				Path:           xp,
				SampleInterval: uint64(sampleInterval),
				Mode:           sdcpb.SubscriptionMode(sdcpb.SubscriptionMode_value[strings.ToUpper(streamMode)]),
			}
			subscriptions = append(subscriptions, subc)
		}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}

		req := &sdcpb.SubscribeRequest{
			Name: datastoreName,
			Subscribe: &sdcpb.SubscriptionList{
				Subscription: subscriptions,
				Mode:         sdcpb.SubscriptionListMode(sdcpb.SubscriptionListMode_value[strings.ToUpper(mode)]),
				DataType:     dt,
			},
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		stream, err := dataClient.Subscribe(ctx, req)
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
				for _, upd := range rsp.GetUpdate().GetUpdate() {
					p := utils.ToXPath(upd.GetPath(), false)
					fmt.Printf("%s: %s\n", p, upd.GetValue())
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
	dataCmd.AddCommand(dataSubscribeCmd)
	dataSubscribeCmd.Flags().StringArrayVarP(&paths, "path", "", []string{}, "get path(s)")
	dataSubscribeCmd.Flags().DurationVarP(&sampleInterval, "sample-interval", "", 10*time.Second, "sample interval")
	dataSubscribeCmd.Flags().StringVarP(&mode, "mode", "", "stream", "subscription mode")
	dataSubscribeCmd.Flags().StringVarP(&streamMode, "steam-mode", "", "sample", "stream subscription mode")
	dataSubscribeCmd.Flags().StringVarP(&dataType, "type", "", "ALL", "data type, one of: ALL, CONFIG, STATE")
	dataSubscribeCmd.Flags().StringVarP(&format, "format", "", "", "print format, '', 'flat' or 'json'")
}
