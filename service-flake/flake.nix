{
  description = "A simple flake which builds the nixinit-service package and adds it as a module";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.05-small";

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forEachSystem = nixpkgs.lib.genAttrs systems;

      overlayList = [ self.overlays.default ];

      pkgsBySystem = forEachSystem (
        system:
        import nixpkgs {
          inherit system;
          overlays = overlayList;
        }
      );

    in
    rec {

      # A Nixpkgs overlay that provides a 'simple-go-webserver' package.
      overlays.default = final: prev: { nixinit-server = final.callPackage ./packages/nixinit-server.nix { }; };

      packages = forEachSystem (system: {
        nixinit-server = pkgsBySystem.${system}.nixinit-server;
        default = pkgsBySystem.${system}.nixinit-server;
      });

      nixosModules = import ./nixos-modules { overlays = overlayList; };

    };
}
