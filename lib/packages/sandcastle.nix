{
  pkgs,
  src,
  sandcastle-seccomp ? null,
}:

pkgs.buildNpmPackage {
  pname = "sandcastle";
  version = "0.0.39";

  inherit src;

  npmDepsHash = "sha256-ny/nxyzf87HLluGIG7xrAz0Ev7c42R6FoSgPR5eAugk=";

  nativeBuildInputs = [
    pkgs.makeWrapper
    pkgs.scdoc
  ];

  buildPhase = ''
    runHook preBuild
    npm run build
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    mkdir -p $out/lib/sandcastle $out/bin

    cp -r dist/* $out/lib/sandcastle/
    cp -r node_modules $out/lib/sandcastle/
    cp package.json $out/lib/sandcastle/
    cp ${src}/sandcastle-cli.mjs $out/lib/sandcastle/sandcastle-cli.mjs

    ${pkgs.lib.optionalString (sandcastle-seccomp != null) ''
      # Install pre-generated seccomp BPF filters and apply-seccomp binary
      mkdir -p $out/lib/sandcastle/vendor
      cp -r ${sandcastle-seccomp}/share/seccomp $out/lib/sandcastle/vendor/seccomp
    ''}

    makeWrapper ${pkgs.nodejs_22}/bin/node $out/bin/sandcastle \
      --add-flags "$out/lib/sandcastle/sandcastle-cli.mjs" \
      --prefix PATH : ${
        pkgs.lib.makeBinPath (
          [
            pkgs.which
            pkgs.socat
            pkgs.ripgrep
          ]
          ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.bubblewrap ]
        )
      }

    mkdir -p $out/share/man/man1
    scdoc < ${src}/doc/sandcastle.1.scd > $out/share/man/man1/sandcastle.1

    runHook postInstall
  '';
}
