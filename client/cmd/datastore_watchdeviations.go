// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
)

// datastoreWatchDeviationCmd represents the watch deviation command
var datastoreWatchDeviationCmd = &cobra.Command{
	Use:          "watch-dev",
	Short:        "watch deviation datastores",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &sdcpb.WatchDeviationRequest{Name: []string{datastoreName}}
		stream, err := dataClient.WatchDeviations(ctx, req)
		if err != nil {
			return err
		}
		count := 0
		for {
			rsp, err := stream.Recv()
			if err != nil {
				return err
			}
			switch format {
			case "":
				switch rsp.Event {
				case sdcpb.DeviationEvent_START:
					fmt.Printf("%s: %s: %s\n",
						rsp.GetName(), rsp.GetIntent(), rsp.GetEvent())
				case sdcpb.DeviationEvent_END:
					fmt.Printf("%s: %s: %s\n",
						rsp.GetName(), rsp.GetIntent(), rsp.GetEvent())

					fmt.Printf("Received %d deviations\n", count)
					count = 0
				default:
					count++
					fmt.Printf("%s: %s: %s: %s: %s : %s -> %s\n",
						rsp.GetName(), rsp.GetIntent(), rsp.GetEvent(),
						rsp.GetReason(), rsp.GetPath().ToXPath(false),
						rsp.GetExpectedValue(), rsp.GetCurrentValue())
				}
			case "json":
				b, err := json.MarshalIndent(rsp, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(b))
			}
		}
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreWatchDeviationCmd)
}
