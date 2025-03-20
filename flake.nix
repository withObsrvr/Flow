{
  description = "Obsrvr Flow Data Indexer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow";
            version = "0.1.0";
            src = ./.;
            # Set vendorHash to null to disable vendoring and fetch dependencies from the network
            vendorHash = null;
            # Ensure we're using -mod=mod to download modules directly from network
            proxyVendor = true;
            flags = [ "-mod=mod" ];
            # Specify the main packages to build
            subPackages = [ 
              "cmd/flow" 
              "cmd/graphql-api"
              "cmd/schema-registry"
            ];
          };
        };

        devShell = pkgs.mkShell {
          buildInputs = [ 
            pkgs.go_1_23
          ];
        };
      }
    );
}
