{
  pkgs,
  src,
  go,
  # Auto-injected as -X main.version / -X main.commit by buildGoApplication
  # (see amarbel-llc/nixpkgs:pkgs/build-support/gomod2nix/default.nix).
  # Defaulted so direct `import ./caldav.nix {...}` consumers still work,
  # but release builds always override via flake.nix.
  version ? "dev",
  commit ? "unknown",
}:

pkgs.buildGoApplication {
  pname = "caldav";
  inherit version commit src;
  pwd = src;
  inherit go;
  modules = src + "/gomod2nix.toml";
  subPackages = [ "cmd/caldav" ];

  postInstall = ''
    $out/bin/caldav generate-plugin $out
  '';

  meta = with pkgs.lib; {
    description = "CalDAV MCP server — tasks, calendars, and VTODO management";
    homepage = "https://github.com/amarbel-llc/bob";
    license = licenses.mit;
  };
}
