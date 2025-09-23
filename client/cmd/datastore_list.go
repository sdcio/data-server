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
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

// datastoreListCmd represents the list command
var datastoreListCmd = &cobra.Command{
	Use:          "list",
	Short:        "list datastores",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &sdcpb.ListDataStoreRequest{}
		rsp, err := dataClient.ListDataStore(ctx, req)
		if err != nil {
			return err
		}

		switch format {
		case "":
			fmt.Println(prototext.Format(rsp))
		case "table":
			err = printDataStoresTable(rsp)
			if err != nil {
				return err
			}
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
	datastoreCmd.AddCommand(datastoreListCmd)
}

func printDataStoresTable(rsp *sdcpb.ListDataStoreResponse) error {
	tableData := make([][]string, 0, len(rsp.GetDatastores()))
	for _, r := range rsp.GetDatastores() {
		tableData = append(tableData, toTableData(r)...)
	}
	sort.Slice(tableData, func(i, j int) bool {
		return tableData[i][0] < tableData[j][0]
	})
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Name", "Schema", "Protocol", "Address", "State", "Candidate (C/O/P)"})
	table.Options(tablewriter.WithAlignment(tw.Alignment{tw.AlignLeft}))
	err := table.Bulk(tableData)
	if err != nil {
		return err
	}
	err = table.Render()
	if err != nil {
		return err
	}
	return nil
}
