/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iptecharch/schema-server/pkg/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
	"gopkg.in/yaml.v2"
)

var sampleInterval time.Duration
var subscriptionFile string

// dataSubscribeCmd represents the subscribe command
var dataSubscribeCmd = &cobra.Command{
	Use:          "subscribe",
	Aliases:      []string{"sub"},
	Short:        "subscribe to data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		var err error
		var req *sdcpb.SubscribeRequest
		if subscriptionFile != "" {
			req, err = subscribeRequestFromFile(subscriptionFile)
			if err != nil {
				return err
			}
		} else {
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

			xps := make([]*sdcpb.Path, 0, len(paths))
			for _, p := range paths {
				xp, err := utils.ParsePath(p)
				if err != nil {
					return err
				}
				xps = append(xps, xp)
			}

			req = &sdcpb.SubscribeRequest{
				Name: datastoreName,
				Subscription: []*sdcpb.Subscription{
					{
						Path:           xps,
						SampleInterval: uint64(sampleInterval),
						DataType:       dt,
					},
				},
			}
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		stream, err := dataClient.Subscribe(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("responses:")
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
	dataSubscribeCmd.Flags().StringVarP(&dataType, "type", "", "ALL", "data type, one of: ALL, CONFIG, STATE")
	dataSubscribeCmd.Flags().StringVarP(&subscriptionFile, "file", "", "", "file with a subscription definition")
	dataSubscribeCmd.Flags().StringVarP(&format, "format", "", "", "print format, '', 'flat' or 'json'")
}

type subscribeDefinition struct {
	Subscription []*subscription `yaml:"subscription,omitempty"`
}

type subscription struct {
	Paths             []string      `yaml:"paths,omitempty"`
	SampleInterval    time.Duration `yaml:"sample-interval,omitempty"`
	DataType          string        `yaml:"data-type,omitempty"`
	SuppressRedundant bool          `yaml:"suppress-redundant,omitempty"`
}

func subscribeRequestFromFile(file string) (*sdcpb.SubscribeRequest, error) {
	b, err := os.ReadFile(subscriptionFile)
	if err != nil {
		return nil, err
	}
	subDef := new(subscribeDefinition)
	err = yaml.Unmarshal(b, subDef)
	if err != nil {
		return nil, err
	}
	req := &sdcpb.SubscribeRequest{
		Name:         datastoreName,
		Subscription: make([]*sdcpb.Subscription, 0, len(subDef.Subscription)),
	}
	for _, subcd := range subDef.Subscription {
		if subcd.DataType == "" {
			subcd.DataType = "ALL"
		}
		var dt sdcpb.DataType
		switch strings.ToUpper(subcd.DataType) {
		case "ALL":
		case "CONFIG":
			dt = sdcpb.DataType_CONFIG
		case "STATE":
			dt = sdcpb.DataType_STATE
		default:
			return nil, fmt.Errorf("invalid data type %s", subcd.DataType)
		}
		subsc := &sdcpb.Subscription{
			Path:              make([]*sdcpb.Path, 0, len(subcd.Paths)),
			DataType:          dt,
			SampleInterval:    uint64(subcd.SampleInterval),
			SuppressRedundant: subcd.SuppressRedundant,
		}
		for _, p := range subcd.Paths {
			xp, err := utils.ParsePath(p)
			if err != nil {
				return nil, err
			}
			subsc.Path = append(subsc.Path, xp)
		}
		req.Subscription = append(req.Subscription, subsc)
	}
	return req, nil
}
