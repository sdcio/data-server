/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
)

// datastoreCommitCmd represents the commit command
var datastoreCommitCmd = &cobra.Command{
	Use:          "commit",
	Short:        "commit candidate datastore to target",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		cc, err := grpc.DialContext(ctx, addr,
			grpc.WithBlock(),
			grpc.WithTransportCredentials(
				insecure.NewCredentials(),
			),
		)
		if err != nil {
			return err
		}
		dataClient := schemapb.NewDataServerClient(cc)
		req := &schemapb.CommitRequest{
			Name: datastoreName,
			Datastore: &schemapb.DataStore{
				Type: schemapb.Type_CANDIDATE,
				Name: candidate,
			},
			Rebase: rebase,
			Stay:   stay,
		}

		rsp, err := dataClient.Commit(ctx, req)
		if err != nil {
			return err
		}
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
