{
  lib
, buildGoModule
, fetchFromGitHub
}:

buildGoModule rec {
  pname = "nixinit";
  #version = "2.1.0";
  version = "0.0.1";

  # src = fetchFromGitHub {
  #   owner = "seanrmurphy";
  #   repo = pname;
  #   rev = "refs/tags/v${version}";
  #   #rev = "${version}";
  #   hash = "sha256-UbcYgAlyd0vD/MvGEA4YryygrWC13glslZW9fe8e+xw=";
  # } ;

  src = ./.;

  vendorHash = "sha256-ZjeA9ogyMsoByBzdvikut93JT6s+8m1AyyPFtwEcYwY=";

  # Tests use network
  doCheck = false;

  subPackages = [
    "cmd/nixinit"
    "cmd/nixinit-server"
  ];

  #ldflags = [ "-X main.noVCSVersionOverride=${version}" ] ;

  meta = with lib; {
    description = "test";
    homepage = "https://github.com/test/test";
    changelog = "https://github.com/test/test";
    license = licenses.asl20;
    maintainers = with maintainers; [ seanrmurphy ];
    mainProgram = "nixinit";
  };
}
