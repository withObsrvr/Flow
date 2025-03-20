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
            # Ensure proper Go module proxy configuration
            proxyVendor = false; # Don't use the built-in proxy behavior
            flags = [ "-mod=mod" ];
            # Set environment variables for go module downloads
            env = {
              GOPROXY = "https://proxy.golang.org,direct";
              GO111MODULE = "on";
              GOSUMDB = "sum.golang.org";
            };
            # Add a pre-build step to remove the vendor directory
            preBuild = ''
              echo "Removing vendor directory to avoid conflicts..."
              rm -rf vendor/
            '';
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
          # Also set the Go proxy environment variables in the dev shell
          shellHook = ''
            export GOPROXY="https://proxy.golang.org,direct"
            export GO111MODULE="on"
            export GOSUMDB="sum.golang.org"
          '';
        };
      }
    );
}
