package cmd

import (
	"context"
	"fmt"
	"os"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	includeDefaults = true
)

// dataBlameConfig represents the get-intent command
var dataBlameConfig = &cobra.Command{
	Use:          "blame",
	Short:        "blame",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		req := &sdcpb.BlameConfigRequest{
			DatastoreName:   datastoreName,
			IncludeDefaults: includeDefaults,
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		dataClient, err := createDataClient(ctx, addr)
		if err != nil {
			return err
		}
		rsp, err := dataClient.BlameConfig(ctx, req)
		if err != nil {
			return err
		}
		switch format {
		case "":
			fmt.Println(rsp.GetConfigTree().ToString())
		case "json":
			opts := protojson.MarshalOptions{
				Indent: "  ",
			}
			b, err := opts.Marshal(rsp.ConfigTree)
			if err != nil {
				fmt.Fprintf(os.Stdout, "%v", err)
			}
			fmt.Println(string(b))
		}
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataBlameConfig)
	dataBlameConfig.Flags().BoolVarP(&includeDefaults, "includeDefaults", "", false, "include defaults in the blame")
}
