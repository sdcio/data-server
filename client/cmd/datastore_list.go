/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/olekukonko/tablewriter"
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
			printDataStoresTable(rsp)
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

func printDataStoresTable(rsp *sdcpb.ListDataStoreResponse) {
	tableData := make([][]string, 0, len(rsp.GetDatastores()))
	for _, r := range rsp.GetDatastores() {
		tableData = append(tableData, toTableData(r)...)
	}
	sort.Slice(tableData, func(i, j int) bool {
		return tableData[i][0] < tableData[j][0]
	})
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Schema", "Protocol", "Address", "State", "Candidate (C/O/P)"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	table.AppendBulk(tableData)
	table.Render()
}
