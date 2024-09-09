{
  config,
  pkgs,
  lib ? pkgs.lib,
  ...
}:

with lib;

let

  cfg = config.services.nixinit ;

in

{
  ###### interface
  options = {

    services.nixinit = rec {

      enable = mkOption {
        type = types.bool;
        default = false;
        description = ''
          Whether to run the nixinit-server
        '';
      };

      port = mkOption {
        type = types.int;
        default = 8080;
        description = ''
          The port to run the service on
        '';
      };
    };

  };

  ###### implementation

  config = mkIf cfg.enable {

    users.extraGroups.nixinit = { };

    users.extraUsers.nixinit = {
      description = "nixinit";
      group = "nixinit";
      home = "/home/nixinit";
      createHome = true;
      isSystemUser = true;
      useDefaultShell = true;
    };

    environment.systemPackages = [ pkgs.nixinit-server ];

    systemd.services.nixinit = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        # the binary generated in this repo is called simple-rest-api and not
        # simple-go-server.
        ExecStart = "+${pkgs.nixinit-server}/bin/nixinit-server";
        User = "nixinit";
        PermissionsStartOnly = true;
        Restart = "always";
        WorkingDirectory = "/home/nixinit";
      };
    };
  };
}
