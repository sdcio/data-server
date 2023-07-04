/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

// datastoreCommitCmd represents the commit command
var datastoreCommitCmd = &cobra.Command{
	Use:          "commit",
	Short:        "commit candidate datastore to target",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.CommitRequest{
			Name: datastoreName,
			Datastore: &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			},
			Rebase: rebase,
			Stay:   stay,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		rsp, err := dataClient.Commit(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreCommitCmd)

	datastoreCommitCmd.Flags().BoolVarP(&rebase, "rebase", "", false, "rebase before commit")
	datastoreCommitCmd.Flags().BoolVarP(&stay, "stay", "", false, "do not delete candidate after commit")
}

var rebase, stay bool
