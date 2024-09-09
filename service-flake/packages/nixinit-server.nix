{
  lib,
  buildGoModule,
  fetchFromGitHub,
}:

buildGoModule rec {
  pname = "nixinit-server";
  # this package currently has no tags
  version = "0.0.0";

  src = fetchFromGitHub {
    owner = "seanrmurphy";
    repo = "nixinit";
    rev = "3516473";
    hash = "sha256-lYKydutQvvK2h2pFTojLS5as4xIP7HA2W3qL3P8pK/I=";
  };

  vendorHash = "sha256-rbQPfp5rEwUV5GQKw2HDZvI9dO1axOX23U7Ko6pCQxc=";

  subCommands = [ "cmd/nixinit-server" ];

  doCheck = false;

  meta = with lib; {
    description = "nixinit-server - a service for bringing up nix";
    homepage = "https://github.com/seanrmurphy/nixinit";
    mainProgram = "nixinit-server";
  };
}
