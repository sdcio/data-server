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

	"github.com/iptecharch/data-server/pkg/utils"
)

// dataDiffCmd represents the diff command
var dataDiffCmd = &cobra.Command{
	Use:          "diff",
	Short:        "diff candidate and its baseline",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &sdcpb.DiffRequest{
			Name: datastoreName,
			Datastore: &sdcpb.DataStore{
				Type: sdcpb.Type_CANDIDATE,
				Name: candidate,
			},
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.Diff(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		printDiffResponse(rsp)
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataDiffCmd)
}

func printDiffResponse(rsp *sdcpb.DiffResponse) {
	if rsp == nil {
		return
	}
	prefix := fmt.Sprintf("%s: %s/%s/%d:", rsp.GetName(), rsp.GetDatastore().GetName(), rsp.GetDatastore().GetOwner(), rsp.GetDatastore().GetPriority())
	for _, diff := range rsp.GetDiff() {
		p := utils.ToXPath(diff.GetPath(), false)
		fmt.Printf("%s\n\tpath=%s:\n\tmain->candidate: %s -> %s\n",
			prefix, p,
			diff.GetMainValue(), diff.GetCandidateValue())
	}
}
