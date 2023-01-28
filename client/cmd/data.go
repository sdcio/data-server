/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// dataCmd represents the data command
var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "",
}

func init() {
	rootCmd.AddCommand(dataCmd)

	dataCmd.PersistentFlags().StringVarP(&candidate, "candidate", "", "", "datastore (candidate) name")
	dataCmd.PersistentFlags().StringVarP(&datastoreName, "ds", "", "", "datastore target name")
}
