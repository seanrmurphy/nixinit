= nixinit

nixinit is a project to make initialization of cloud based nix systems easier.

It comprises of three components:
- a specific server side component which supports ssh key based login with any
  key which is registered on github
- a lightweight nix image which runs the above service by default
- a client which can be used to launch the above image, generate a sensible
  default configuration.nix and upload this to the launched instance.
