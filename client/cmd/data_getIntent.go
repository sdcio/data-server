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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unsafe"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var intentName string

// dataGetIntentCmd represents the get-intent command
var dataGetIntentCmd = &cobra.Command{
	Use:          "get-intent",
	Short:        "get intent data",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		req := &sdcpb.GetIntentRequest{
			DatastoreName: datastoreName,
			Intent:        intentName,
			Format:        sdcpb.Format_Intent_Format_JSON,
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "request:")
		fmt.Fprintln(os.Stderr, prototext.Format(req))
		startTime := time.Now()
		rsp, err := dataClient.GetIntent(ctx, req)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "GetIntent took: %s\n", time.Since(startTime))

		var buf bytes.Buffer
		if err := json.Indent(&buf, rsp.GetBlob(), "", "  "); err != nil {
			return err
		}

		if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
			return err
		}

		return nil
	},
}

func BytesToString(b []byte) string {
	return unsafe.String(&b[0], len(b))
}

func init() {
	dataCmd.AddCommand(dataGetIntentCmd)
	dataGetIntentCmd.Flags().StringVarP(&intentName, "intent", "", "", "intent name")
	dataGetIntentCmd.Flags().Int32VarP(&priority, "priority", "", 0, "intent priority")
}
