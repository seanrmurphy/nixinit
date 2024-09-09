{

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-unstable";
    nixinit-server.url = "github:seanrmurphy/nixinit/add-flake-for-service?dir=service-flake";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, nixos-generators, nixinit-server, ... }: {
    packages.x86_64-linux = {
      qcow = nixos-generators.nixosGenerate {
        system = "x86_64-linux";
        modules = [
          # you can include your own nixos configuration here, i.e.
          ./configuration.nix
          nixinit-server.nixosModules.nixinit
        ({ pkgs, ... }: {
          nixpkgs.overlays = [
            nixinit-server.overlays.default
          ];
          # Add the packages from nixinit-server here
          environment.systemPackages = with pkgs; [
            # Add the packages you want from nixinit-server
            # For example:
            nixinit-server.packages.x86_64-linux.nixinit-server
            # Add more packages as needed
          ];
        })
        ];
        format = "qcow";

        # pkgs = import nixpkgs { overlays = [ nixinit-server.overlays.default ]; };

        # optional arguments:
        # explicit nixpkgs and lib:
        # pkgs = nixpkgs.legacyPackages.x86_64-linux;
        # lib = nixpkgs.legacyPackages.x86_64-linux.lib;
        # additional arguments to pass to modules:
        # specialArgs = { myExtraArg = "foobar"; };

        # you can also define your own custom formats
        # customFormats = { "myFormat" = <myFormatModule>; ... };
        # format = "myFormat";
      };
    };
  };
}
