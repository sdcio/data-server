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
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

// datastoreGetCmd represents the get command
var datastoreGetCmd = &cobra.Command{
	Use:          "get",
	Short:        "show datastore details",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &sdcpb.GetDataStoreRequest{
			Name: datastoreName,
		}
		// fmt.Println("request:")
		// fmt.Println(prototext.Format(req))
		rsp, err := dataClient.GetDataStore(ctx, req)
		if err != nil {
			return err
		}
		switch format {
		case "":
			fmt.Println(prototext.Format(rsp))
		case "table":
			printDataStoreTable(rsp)
		case "json":
			b, err := json.MarshalIndent(rsp, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
		}

		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreGetCmd)
}

func printDataStoreTable(rsp *sdcpb.GetDataStoreResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Schema", "Protocol", "Address", "State", "Candidate (C/O/P)"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	table.AppendBulk(toTableData(rsp))
	table.Render()
}

func toTableData(rsp *sdcpb.GetDataStoreResponse) [][]string {
	candidates := make([]string, 0, len(rsp.GetDatastore()))
	for _, ds := range rsp.GetDatastore() {
		if ds.GetType() == sdcpb.Type_MAIN {
			continue
		}
		candidateName := "- " + ds.GetName()
		if ds.Owner != "" {
			candidateName += "/" + ds.Owner
		}
		if ds.Priority != 0 {
			candidateName += "/" + strconv.Itoa(int(ds.Priority))
		}
		candidates = append(candidates, candidateName)
	}
	return [][]string{
		{
			rsp.GetName(),
			fmt.Sprintf("%s/%s/%s", rsp.GetSchema().GetName(), rsp.GetSchema().GetVendor(), rsp.GetSchema().GetVersion()),
			rsp.GetTarget().GetType(),
			rsp.GetTarget().GetAddress(),
			rsp.GetTarget().GetStatus().String(),
			strings.Join(candidates, "\n"),
		},
	}
}
