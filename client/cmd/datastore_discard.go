/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// datastoreDiscardCmd represents the discard command
var datastoreDiscardCmd = &cobra.Command{
	Use:          "discard",
	Short:        "discard changes made to a candidate datastore",
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// TODO:
	},
}

func init() {
	datastoreCmd.AddCommand(datastoreDiscardCmd)
}
