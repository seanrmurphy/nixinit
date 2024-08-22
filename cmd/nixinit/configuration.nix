
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

  networking.hostName = "nixos";

  users.users = {
    nixos = {
      isNormalUser = true;
      openssh.authorizedKeys.keys = [
				pubkey
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
  system.stateVersion = "24.05";
}
