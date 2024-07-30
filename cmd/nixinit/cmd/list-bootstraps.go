/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// listBootstrapsCmd represents the listBootstraps command
var listBootstrapsCmd = &cobra.Command{
	Use:   "list-bootstraps",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: listBootstraps,
}

func init() {
	rootCmd.AddCommand(listBootstrapsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// listBootstrapsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// listBootstrapsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func listBootstraps(cmd *cobra.Command, args []string) {
	bootstrapVMs, _ := getBootstrapVMs()
	for _, vm := range bootstrapVMs {
		fmt.Printf("Bootstrap VM ID: %s\n", vm)
	}
}
