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
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
		req := &sdcpb.CommitRequest{
			Name: datastoreName,
			Datastore: &sdcpb.DataStore{
				Type: sdcpb.Type_CANDIDATE,
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
