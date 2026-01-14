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
	"os"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var schemaName string
var schemaVendor string
var schemaVersion string
var format string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "datactl",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var addr string

func init() {
	rootCmd.PersistentFlags().StringVarP(&addr, "address", "a", "localhost:56000", "schema server address")
	rootCmd.PersistentFlags().StringVar(&schemaName, "name", "", "schema name")
	rootCmd.PersistentFlags().StringVar(&schemaVendor, "vendor", "", "schema vendor")
	rootCmd.PersistentFlags().StringVar(&schemaVersion, "version", "", "schema version")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "", "", "print format, '','table', 'flat' or 'json'")
}

func createDataClient(ctx context.Context, addr string) (sdcpb.DataServerClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr, //nolint:staticcheck
		grpc.WithBlock(), //nolint:staticcheck
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(200*1024*1024)),
	)
	if err != nil {
		return nil, err
	}
	return sdcpb.NewDataServerClient(cc), nil
}
