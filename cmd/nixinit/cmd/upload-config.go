/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"github.com/pterm/pterm"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// uploadConfigCmd represents the uploadConfig command
var uploadConfigCmd = &cobra.Command{
	Use:   "upload-config",
	Short: "uploads a nixos configuration to a remote bootstrapping nixos instance",
	Long:  `uploads a nixos configuration to a remote bootstrapping nixos instance`,
	Run:   uploadConfig,
}

var (
	addr                  string
	port                  int
	configurationFilename string
	instanceID            string
)

func init() {
	rootCmd.AddCommand(uploadConfigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// uploadConfigCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	uploadConfigCmd.Flags().StringVarP(&addr, "addr", "a", "localhost", "Remote address to upload the configuration to")
	uploadConfigCmd.Flags().IntVarP(&port, "port", "p", 2222, "Remote port to upload the configuration to")
	uploadConfigCmd.Flags().StringVarP(&configurationFilename, "file", "f", configurationNixFilename, "Name of nixOS configuration file to upload")
	uploadConfigCmd.Flags().StringVarP(&instanceID, "instance", "i", "", "ID of bootstrapping instance")
}

func uploadConfig(cmd *cobra.Command, args []string) {
	if instanceID == "" {
		pterm.Error.Println("Instance ID is required to upload configuration - exiting... ")
		return
	}

	// check if configuration.nix exists
	pterm.Info.Printf("Checking if %s exists...\n", configurationFilename)
	if _, err := os.Stat(configurationFilename); err != nil {
		pterm.Error.Printf("%s does not exist - exiting...\n", configurationFilename)
		return
	}
	pterm.Info.Printf("%s found - will attempt to upload\n", configurationFilename)

	// clientConfig, _ := auth.SshAgent("nixinit", ssh.InsecureIgnoreHostKey())

	config := &ssh.ClientConfig{
		User:            "nixinit",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For testing only, use ssh.FixedHostKey(publicKey) in production
	}

	socket := os.Getenv("SSH_AUTH_SOCK")

	conn, err := net.Dial("unix", socket)
	if err != nil {
		log.Fatalf("Failed to open SSH_AUTH_SOCK: %v", err)
	}

	agentClient := agent.NewClient(conn)
	config.Auth = []ssh.AuthMethod{
		ssh.PublicKeysCallback(agentClient.Signers),
	}

	sshAddr := fmt.Sprintf("%s:%d", addr, port)
	pterm.Info.Printf("Connecting to ssh server on %s...\n", sshAddr)
	sshClient, err := ssh.Dial("tcp", sshAddr, config)
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer sshClient.Close()

	uploadFilename := filepath.Join("/uploads/nixinit", instanceID, configurationNixFilename)

	pterm.Info.Printf("Uploading configuration file...\n")
	// open an SFTP session over an existing ssh connection.
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// read file into buffer
	configurationFileData, err := os.ReadFile(configurationFilename)
	if err != nil {
		log.Fatal(err)
	}

	f, err := client.Create(uploadFilename)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write(configurationFileData); err != nil {
		log.Fatal(err)
	}
	f.Close()
	pterm.Success.Printf("Configuration file uploaded...\n")
}
