{
  pkgs,
  goWorkspaceSrc,
  goVendorHash,
  go,
  src, # Original source for non-Go assets (completions)
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

  spinclass2 = mkGoModule {
    pname = "spinclass2";
    subPackages = [ "packages/spinclass2/cmd/spinclass" ];
    postInstall = ''
      mv $out/bin/spinclass $out/bin/spinclass2
      ln -s spinclass2 $out/bin/sc-dev
    '';
  };

  shellCompletions = pkgs.runCommand "spinclass2-completions" { } ''
    install -Dm644 ${src}/completions/spinclass2.bash-completion \
      $out/share/bash-completion/completions/spinclass2
    install -Dm644 ${src}/completions/spinclass2.fish \
      $out/share/fish/vendor_completions.d/spinclass2.fish
    install -Dm644 ${src}/completions/sc-dev.bash-completion \
      $out/share/bash-completion/completions/sc-dev
    install -Dm644 ${src}/completions/sc-dev.fish \
      $out/share/fish/vendor_completions.d/sc-dev.fish
  '';
in
pkgs.symlinkJoin {
  name = "spinclass2";
  paths = [
    spinclass2
    shellCompletions
  ];
}
