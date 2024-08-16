// Package cmd provides the command-line interface for the nixinit-server application.
package cmd

import (
	"html/template"
	"log"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// NixConfigurationParams contains the parameters needed to generate a basic nix configuration file.
type NixConfigurationParams struct {
	Hostname          string
	SSHPublicKey      string
	NixOSStateVersion string
}

// generateConfigCmd represents the generateConfig command
var generateConfigCmd = &cobra.Command{
	Use:   "generate-config",
	Short: "Generate a nix configuration which will be applied to your nix instance",
	Long: `generate-config creates a configuration.nix based on a template with simple
	substitutions for your context.`,
	Run: generateConfig,
}

var (
	configurationNixFilename = "configuration.nix"
	defaultNixOSStateVersion = "24.05"
)

var configurationNixTemplate = `
# This is your system's configuration file.
# Use this to configure your system environment
# For more information, see: https://nixos.org/manual/nixpkgs/stable/configuration.html
{
  inputs,
  lib,
  config,
  pkgs,
}: {

  imports = [
    ./hardware-configuration.nix
  ];

  nixpkgs = {
    config = {
      allowUnfree = true;
    };
  };

  nix.settings.experimental-features = [ "nix-command" "flakes" ];
	nix.settings.channel.enable = false;

  networking.hostName = "{{ .Hostname }}";

  users.users = {
    nixos = {
      isNormalUser = true;
      openssh.authorizedKeys.keys = [
				"{{ .SSHPublicKey }}"
      ];
      extraGroups = ["wheel"];
    };
  };

  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = "no";
      PasswordAuthentication = false;
    };
  };

  # https://nixos.wiki/wiki/FAQ/When_do_I_update_stateVersion
  system.stateVersion = "{{ .NixOSStateVersion }}";
}
`

func init() {
	rootCmd.AddCommand(generateConfigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generateConfigCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// generateConfigCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func generateConfig(cmd *cobra.Command, args []string) {
	// check if configuration.nix already exists
	pterm.Info.Printf("Checking if configuration.nix exists...\n")
	if _, err := os.Stat(configurationNixFilename); err == nil {
		pterm.Error.Printf("configuration.nix already exists in current directory - will not overwrite - exiting...\n")
		return
	}
	pterm.Info.Println("configuration.nix does not exist. Creating...")

	hostname, err := pterm.DefaultInteractiveTextInput.Show("Enter hostname")
	sshPublicKey, err := pterm.DefaultInteractiveTextInput.Show("Enter SSH public key")

	nixosConfigParams := NixConfigurationParams{
		Hostname:          hostname,
		SSHPublicKey:      sshPublicKey,
		NixOSStateVersion: defaultNixOSStateVersion,
	}

	// render the template with the provided parameters using the go tmpl library
	tmpl, err := template.New("configuration.nix").Parse(configurationNixTemplate)
	if err != nil {
		log.Printf("Error parsing template: %v\n", err)
		return
	}

	var renderedTemplate strings.Builder
	err = tmpl.Execute(&renderedTemplate, nixosConfigParams)
	if err != nil {
		log.Printf("Error executing template: %v\n", err)
		return
	}

	// write the rendered template to configuration.nix
	err = os.WriteFile(configurationNixFilename, []byte(renderedTemplate.String()), 0600)
	if err != nil {
		log.Printf("Error writing configuration.nix: %v\n", err)
		return
	}

	pterm.Success.Println("Successfully created configuration.nix")
	pterm.Info.Println("You can customize configuration.nix according to your needs.")
	pterm.Info.Println("When ready, you can upload the config to the bootstrapping instance with nixinit upload-config.")
}
