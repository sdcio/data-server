/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"

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
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.ListDataStore(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		printDataStoresTable(rsp)
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
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Schema", "Candidate(s)", "SBI", "Address"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	table.AppendBulk(tableData)
	table.Render()
}
