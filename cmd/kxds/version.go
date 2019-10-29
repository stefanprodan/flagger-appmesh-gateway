package main

import (
	"fmt"
	"github.com/spf13/cobra"
)

var VERSION = "0.0.3-beta.1"

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   `version`,
	Short: "Prints podcli version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(VERSION)
		return nil
	},
}
