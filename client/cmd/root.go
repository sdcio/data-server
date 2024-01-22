/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"os"
	"time"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var schemaName string
var schemaVendor string
var schemaVersion string

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
	cc, err := grpc.DialContext(ctx, addr,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		return nil, err
	}
	return sdcpb.NewDataServerClient(cc), nil
}
