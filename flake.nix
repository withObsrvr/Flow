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
        
        # Custom source that explicitly excludes the vendor directory
        flowSrc = pkgs.lib.cleanSourceWith {
          src = ./.;
          filter = path: type:
            let baseName = baseNameOf path;
            in !(baseName == "vendor");
        };
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "flow";
            version = "0.1.0";
            src = flowSrc;
            # Set vendorHash to null to disable vendoring and fetch dependencies from the network
            vendorHash = null;
            # Don't try to use the vendor directory at all
            proxyVendor = false;
            allowGoReference = true;
            # Use these flags to ensure we don't use the vendor directory
            buildFlags = [ "-mod=mod" ];
            # Set environment variables for go module downloads
            env = {
              GOPROXY = "https://proxy.golang.org,direct";
              GO111MODULE = "on";
              GOSUMDB = "sum.golang.org";
            };
            # Additional pre-build checks to make sure the vendor directory is gone
            preBuild = ''
              if [ -d vendor ]; then
                echo "ERROR: vendor directory still exists!"
                exit 1
              fi
              echo "Building without vendor directory..."
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
