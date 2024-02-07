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

	"github.com/sdcio/data-server/pkg/utils"
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
