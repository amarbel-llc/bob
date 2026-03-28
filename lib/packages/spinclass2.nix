{
  pkgs,
  goWorkspaceSrc,
  goVendorHash,
  go,
}:

let
  mkGoModule = import ../mkGoWorkspaceModule.nix {
    inherit
      pkgs
      goWorkspaceSrc
      goVendorHash
      go
      ;
  };
in
mkGoModule {
  pname = "spinclass2";
  subPackages = [ "packages/spinclass2/cmd/spinclass" ];
  postInstall = ''
    mv $out/bin/spinclass $out/bin/spinclass2
    ln -s spinclass2 $out/bin/sc-dev
  '';
}
