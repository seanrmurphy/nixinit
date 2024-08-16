// Package cmd provides the command-line interface for the nixinit-server application.
package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

// removeBootstrapsCmd represents the removeBootstraps command
var removeBootstrapsCmd = &cobra.Command{
	Use:   "remove-bootstraps",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: removeBootstraps,
}

var removeInstanceID string

func init() {
	rootCmd.AddCommand(removeBootstrapsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// removeBootstrapsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	removeBootstrapsCmd.Flags().StringVarP(&removeInstanceID, "instance-id", "i", "", "ID of instance to be removed")
}

func removeBootstraps(cmd *cobra.Command, args []string) {
	if removeInstanceID == "" {
		log.Println("Instance ID is required to remove bootstrap - exiting... ")
		return
	}

	err := removeInstance(removeInstanceID)
	if err != nil {
		log.Printf("Error removing bootstrap VM: %v", err)
	}
}
