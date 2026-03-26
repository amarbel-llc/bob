{ pkgs, src }:

let
  arch =
    if pkgs.stdenv.hostPlatform.isx86_64 then
      "x64"
    else if pkgs.stdenv.hostPlatform.isAarch64 then
      "arm64"
    else
      throw "sandcastle-seccomp: unsupported architecture ${pkgs.stdenv.hostPlatform.system}";
in
pkgs.stdenv.mkDerivation {
  pname = "sandcastle-seccomp";
  version = "0.0.1";

  inherit src;
  sourceRoot = ".";

  nativeBuildInputs = [ pkgs.libseccomp ];
  buildInputs = [ pkgs.libseccomp ];

  dontUnpack = true;

  buildPhase = ''
    runHook preBuild

    # Build the BPF generators
    $CC -o gen-unix-block ${src}/gen-unix-block.c -lseccomp
    $CC -o gen-bind-block ${src}/gen-bind-block.c -lseccomp

    # Build the apply-seccomp binary
    $CC -o apply-seccomp ${src}/apply-seccomp.c

    # Generate BPF bytecode
    ./gen-unix-block > unix-block.bpf
    ./gen-bind-block > bind-block.bpf

    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    mkdir -p $out/bin $out/share/seccomp/${arch}

    cp apply-seccomp $out/bin/apply-seccomp
    cp apply-seccomp $out/share/seccomp/${arch}/apply-seccomp
    cp unix-block.bpf $out/share/seccomp/${arch}/unix-block.bpf
    cp bind-block.bpf $out/share/seccomp/${arch}/bind-block.bpf

    runHook postInstall
  '';

  meta = {
    description = "Seccomp BPF filters and apply-seccomp binary for sandcastle";
    platforms = pkgs.lib.platforms.linux;
  };
}
