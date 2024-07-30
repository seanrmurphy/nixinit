/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
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
	pterm.Info.Printf("%s exists\n", configurationFilename)

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

	sshClient, err := ssh.Dial("tcp", "127.0.0.1:2222", config)
	if err != nil {
		log.Fatalf("Failed to dial: %v", err)
	}
	defer sshClient.Close()

	// Create a new SCP client
	// remoteHost := fmt.Sprintf("%s:%d", addr, port)
	// client := scp.NewClient(remoteHost, &clientConfig)
	// pterm.Info.Printf("Connecting to remote host %s\n", remoteHost)
	//
	// // Connect to the remote server
	// err := client.Connect()
	// if err != nil {
	// 	pterm.Error.Printf("Couldn't establish a connection to the remote server %v - exiting...", err)
	// 	return
	// }
	//
	// // Open a file
	// f, _ := os.Open(configurationFilename)
	//
	// // Close client connection after the file has been copied
	// defer client.Close()
	//
	// // Close the file after it has been copied
	// defer f.Close()

	// Finally, copy the file over
	// Usage: CopyFromFile(context, file, remotePath, permission)

	// the context can be adjusted to provide time-outs or inherit from other contexts if this is embedded in a larger application.
	uploadFilename := filepath.Join("/uploads/nixinit", instanceID, configurationNixFilename)
	// err = client.CopyFromFile(context.Background(), *f, uploadFilename, "0655")
	// err = client.
	//
	// if err != nil {
	// 	pterm.Error.Printf("Error uploading file...%v", err)
	// }

	// ---

	// open an SFTP session over an existing ssh connection.
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// walk a directory
	// w := client.Walk("/home/user")
	// for w.Step() {
	// 	if w.Err() != nil {
	// 		continue
	// 	}
	// 	log.Println(w.Path())
	// }

	// leave your mark
	f, err := client.Create(uploadFilename)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write([]byte("Hello world!")); err != nil {
		log.Fatal(err)
	}
	f.Close()

	// check it's there
	fi, err := client.Lstat("hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(fi)
}
