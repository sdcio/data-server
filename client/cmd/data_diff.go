/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// dataDiffCmd represents the diff command
var dataDiffCmd = &cobra.Command{
	Use:          "diff",
	Short:        "diff candidate and its baseline",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	dataCmd.AddCommand(dataDiffCmd)
}
