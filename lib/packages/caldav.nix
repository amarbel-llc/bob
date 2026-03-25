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
  pname = "caldav";
  subPackages = [ "packages/caldav/cmd/caldav" ];

  postInstall = ''
    $out/bin/caldav generate-plugin $out
  '';

  meta = with pkgs.lib; {
    description = "CalDAV MCP server — tasks, calendars, and VTODO management";
    homepage = "https://github.com/amarbel-llc/bob";
    license = licenses.mit;
  };
}
