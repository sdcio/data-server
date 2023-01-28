/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var datastoreName string

// datastoreCmd represents the datastore command
var datastoreCmd = &cobra.Command{
	Use:   "datastore",
	Short: "manipulate datastores",
}

func init() {
	rootCmd.AddCommand(datastoreCmd)
	datastoreCmd.PersistentFlags().StringVarP(&datastoreName, "ds", "", "", "datastore (main) name")
	datastoreCmd.PersistentFlags().StringVarP(&candidate, "candidate", "", "", "datastore candidate name")
}
