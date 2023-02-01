/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

// schemaUploadCmd represents the upload command
var schemaUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "A brief description of your command",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		_ = schemaClient
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaUploadCmd)
}
