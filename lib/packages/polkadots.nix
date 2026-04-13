{ pkgs, src }:

pkgs.stdenvNoCC.mkDerivation {
  pname = "polkadots";
  version = "0.1.0";
  inherit src;
  nativeBuildInputs = [ pkgs.scdoc ];
  dontUnpack = true;
  dontBuild = true;
  installPhase = ''
    mkdir -p $out/share/man/man7
    for f in $src/doc/*.7.scd; do
      scdoc < "$f" > "$out/share/man/man7/$(basename "$f" .scd)"
    done
  '';
}
